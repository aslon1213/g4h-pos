// Package salecart is the repository for the POS-side carts: Scan & Go handoff
// carts (kind=handoff) and till carts (kind=pos). Both live in the shared
// `sale_carts` header with their lines in the shared `sale_cart_items` table;
// the handoff credentials and claim bookkeeping live in the
// `handoff_cart_sessions` satellite, which POS carts simply do not have.
//
// It owns the cart lifecycle and enforces the kind-aware state machine
// (models.CartState): a handoff cart is built by a customer authenticated by a
// session token, minted a short-lived handoff token at checkout, then claimed by
// a seller (freezing customer edits) and charged; a POS cart is opened, built
// and charged by one staff user. Charging delegates to the EXISTING journal
// operation (sale) flow — this package writes no new sale/journal logic — and
// moves stock through pkg/repository/inventory.
//
// Money is integer so'm; quantity is fractional. Every line total goes through
// models.RoundLineTotal, and a cart's subtotal is the exact integer sum of those
// already-rounded lines. Checkout re-derives prices from product_stock with the
// same helper, so the charged figure always reproduces what was displayed.
//
// The storefront basket (pkg/repository/store/cart) is a separate surface and is
// deliberately not served by this repository.
package salecart

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/inventory"
	journalsrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/journals"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// posCartTTL bounds how long an abandoned till cart lingers before the janitor
// reaps it. It replaces the Redis key expiry the old sales session relied on.
const posCartTTL = 12 * time.Hour

// SaleCartRepository owns sale_carts / sale_cart_items / handoff_cart_sessions
// and delegates the sale at checkout to the journals repository.
type SaleCartRepository struct {
	db       *gorm.DB
	journals *journalsrepo.JournalsRepository
}

func New(db *gorm.DB) *SaleCartRepository {
	return &SaleCartRepository{
		db:       db,
		journals: journalsrepo.New(db),
	}
}

// ---------------------------------------------------------------------------
// Handoff customer surface (authenticated by the session token)
// ---------------------------------------------------------------------------

// StartSession creates a new active handoff cart scoped to branchID (from the
// entry QR) and mints its session token. The plaintext token is returned once
// (for the entry-QR link); only its digest is stored. A missing branch is
// ErrNotFound.
func (r *SaleCartRepository) StartSession(ctx context.Context, branchID string) (*models.SaleCart, string, error) {
	if branchID == "" {
		return nil, "", repoerr.ErrInvalidInput
	}
	if err := r.branchMustExist(ctx, branchID); err != nil {
		return nil, "", err
	}

	token, hash, err := newSessionToken()
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	cart := &models.SaleCart{
		ID:        uuid.New().String(),
		Kind:      models.CartKindHandoff,
		BranchID:  branchID,
		State:     models.CartActive,
		CreatedAt: now,
		UpdatedAt: now,
	}
	session := &models.HandoffSession{
		CartID:           cart.ID,
		SessionTokenHash: hash,
		SessionExpiresAt: endOfDay(now),
	}

	// The cart and its credential must appear together — a cart with no session
	// row would be unreachable, and a session row with no cart would dangle.
	err = r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Create(cart).Error; err != nil {
			return err
		}
		return tx.WithContext(ctx).Create(session).Error
	})
	if err != nil {
		return nil, "", err
	}

	cart.Session = session
	cart.Items = []models.SaleCartItem{}
	return cart, token, nil
}

// GetBySessionToken loads a handoff cart by its session token, applying lazy
// expiry (a session past its day, or a lapsed handoff token) before returning it.
func (r *SaleCartRepository) GetBySessionToken(ctx context.Context, token string) (*models.SaleCart, error) {
	if token == "" {
		return nil, repoerr.ErrInvalidInput
	}
	session, err := gorm.G[models.HandoffSession](r.db).
		Where("session_token_hash = ?", hashToken(token)).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return r.loadCart(ctx, session.CartID, &session)
}

// AddItem adds (or increments a matching) scanned line to the session's cart.
func (r *SaleCartRepository) AddItem(ctx context.Context, token string, in models.AddSaleCartItemInput) (*models.SaleCart, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return r.addItem(ctx, cart, in)
}

// UpdateItem sets a line's quantity (<= 0 removes it) on the session's cart.
func (r *SaleCartRepository) UpdateItem(ctx context.Context, token, itemID string, quantity float64) (*models.SaleCart, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return r.updateItem(ctx, cart, itemID, quantity)
}

// RemoveItem drops a single line from the session's cart.
func (r *SaleCartRepository) RemoveItem(ctx context.Context, token, itemID string) (*models.SaleCart, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	return r.removeItem(ctx, cart, itemID)
}

// RequestHandoff transitions the cart to ready_for_handoff and mints a fresh
// handoff token (re-minting if already ready). The plaintext code + QR ref are
// returned once. An empty cart, or a cart past the editable states, is rejected.
func (r *SaleCartRepository) RequestHandoff(ctx context.Context, token string) (*models.SaleCart, string, string, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, "", "", err
	}
	if cart.State != models.CartActive && cart.State != models.CartReadyForHandoff {
		return nil, "", "", repoerr.ErrConflict
	}
	if len(cart.Items) == 0 {
		return nil, "", "", repoerr.ErrInvalidInput
	}

	code, ref, codeHash, refHash, err := newHandoffToken()
	if err != nil {
		return nil, "", "", err
	}
	now := time.Now()
	exp := now.Add(handoffTTL)

	err = r.db.Transaction(func(tx *gorm.DB) error {
		// Flip the cart only from a state that still permits minting; the guard
		// doubles as the optimistic-concurrency check (RowsAffected == 0 loses).
		res := tx.WithContext(ctx).Model(&models.SaleCart{}).
			Where("id = ? AND kind = ? AND state IN ?", cart.ID, models.CartKindHandoff,
				[]models.CartState{models.CartActive, models.CartReadyForHandoff}).
			Updates(map[string]any{
				"state":      models.CartReadyForHandoff,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return repoerr.ErrConflict
		}
		return tx.WithContext(ctx).Model(&models.HandoffSession{}).
			Where("cart_id = ?", cart.ID).
			Updates(map[string]any{
				"handoff_code_hash":    codeHash,
				"handoff_ref_hash":     refHash,
				"handoff_expires_at":   exp,
				"handoff_attempts":     0,
				"handoff_locked_until": nil,
			}).Error
	})
	if err != nil {
		return nil, "", "", err
	}

	cart.State = models.CartReadyForHandoff
	if cart.Session != nil {
		cart.Session.HandoffExpiresAt = &exp
	}
	return cart, code, ref, nil
}

// CancelBySession lets the customer cancel their own cart (active/ready → cancelled).
func (r *SaleCartRepository) CancelBySession(ctx context.Context, token string) (*models.SaleCart, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := r.transition(ctx, r.db, cart, models.CartCancelled, map[string]any{"finalized_at": time.Now()}); err != nil {
		return nil, err
	}
	return cart, nil
}

// DeleteBySessionToken hard-deletes the cart behind a session token; its items
// and its handoff session row go with it via ON DELETE CASCADE. This is the only
// operation that destroys a cart rather than moving it to a terminal state —
// once the row is gone the session token digest is gone with it, so the header
// the phone cached can never resolve again (no state check: a customer may drop
// their session whatever state it is in).
//
// Note this is unconditional by design: deleting a completed cart also discards
// the cart_id back-link on its sale transaction (ON DELETE SET NULL). The
// recorded sale and its money are untouched, but the trail from that sale back
// to the item list is not recoverable. An unknown token is ErrNotFound.
func (r *SaleCartRepository) DeleteBySessionToken(ctx context.Context, token string) error {
	if token == "" {
		return repoerr.ErrInvalidInput
	}
	session, err := gorm.G[models.HandoffSession](r.db).
		Where("session_token_hash = ?", hashToken(token)).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return repoerr.ErrNotFound
	}
	if err != nil {
		return err
	}
	n, err := gorm.G[models.SaleCart](r.db).Where("id = ?", session.CartID).Delete(ctx)
	if err != nil {
		return err
	}
	if n == 0 {
		return repoerr.ErrNotFound
	}
	log.Info().Str("cart_id", session.CartID).Msg("handoff session: cart deleted by session token")
	return nil
}

// ---------------------------------------------------------------------------
// POS (till) surface — the seller owns the cart from open to charge
// ---------------------------------------------------------------------------

// OpenPOSCart opens a new active till cart at the seller's branch. Unlike a
// handoff cart there is no session token and no QR: the staff PASETO guard plus
// the handoff.checkout capability already authenticate the seller, and the cart
// is addressed by its id from then on.
func (r *SaleCartRepository) OpenPOSCart(ctx context.Context, branchID, sellerID string) (*models.SaleCart, error) {
	if branchID == "" || sellerID == "" {
		return nil, repoerr.ErrInvalidInput
	}
	if err := r.branchMustExist(ctx, branchID); err != nil {
		return nil, err
	}
	now := time.Now()
	seller := sellerID
	cart := &models.SaleCart{
		ID:        uuid.New().String(),
		Kind:      models.CartKindPOS,
		BranchID:  branchID,
		State:     models.CartActive,
		SellerID:  &seller,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := r.db.WithContext(ctx).Create(cart).Error; err != nil {
		return nil, err
	}
	cart.Items = []models.SaleCartItem{}
	return cart, nil
}

// ListOpenPOSCarts returns the seller's own un-charged till carts, so a till can
// recover its state after a reload. This is what the Redis session scan used to
// provide, now backed by a real index.
func (r *SaleCartRepository) ListOpenPOSCarts(ctx context.Context, branchID, sellerID string) ([]models.SaleCart, error) {
	carts, err := gorm.G[models.SaleCart](r.db).
		Where("kind = ? AND state = ? AND branch_id = ? AND seller_id = ?",
			models.CartKindPOS, models.CartActive, branchID, sellerID).
		Order("created_at DESC").Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range carts {
		r.attachItems(ctx, &carts[i])
	}
	return carts, nil
}

// AddItemByID / UpdateItemByID / RemoveItemByID are the seller-driven edits,
// addressed by cart id. They serve POS carts, and a handoff cart a seller has
// claimed (where edit rights have transferred to that seller).
func (r *SaleCartRepository) AddItemByID(ctx context.Context, cartID string, in models.AddSaleCartItemInput) (*models.SaleCart, error) {
	cart, err := r.GetByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	return r.addItem(ctx, cart, in)
}

func (r *SaleCartRepository) UpdateItemByID(ctx context.Context, cartID, itemID string, quantity float64) (*models.SaleCart, error) {
	cart, err := r.GetByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	return r.updateItem(ctx, cart, itemID, quantity)
}

func (r *SaleCartRepository) RemoveItemByID(ctx context.Context, cartID, itemID string) (*models.SaleCart, error) {
	cart, err := r.GetByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	return r.removeItem(ctx, cart, itemID)
}

// CancelByID cancels a cart of either kind by id (POS: the seller abandons the
// till cart; handoff: see CancelClaim for the claim-scoped variant).
func (r *SaleCartRepository) CancelByID(ctx context.Context, cartID string) (*models.SaleCart, error) {
	cart, err := r.GetByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	if err := r.transition(ctx, r.db, cart, models.CartCancelled, map[string]any{"finalized_at": time.Now()}); err != nil {
		return nil, err
	}
	return cart, nil
}

// ---------------------------------------------------------------------------
// Handoff seller surface (staff PASETO, capability checked)
// ---------------------------------------------------------------------------

// ClaimByToken atomically claims a ready handoff cart for a seller, freezing
// customer edits. Idempotent: a repeat by the same seller returns the
// already-claimed cart. Invalid/expired tokens are ErrNotFound (and bump the
// per-cart failed-attempt counter / lockout); an already-claimed-by-another or
// locked cart is ErrConflict. Prefers the higher-entropy QR ref over the code.
func (r *SaleCartRepository) ClaimByToken(ctx context.Context, in models.ClaimHandoffInput, sellerID string) (*models.SaleCart, error) {
	var col, val string
	switch {
	case in.Ref != "":
		col, val = "handoff_ref_hash", hashToken(in.Ref)
	case in.Code != "":
		col, val = "handoff_code_hash", hashToken(in.Code)
	default:
		return nil, repoerr.ErrInvalidInput
	}

	now := time.Now()
	log.Info().Str("seller_id", sellerID).Str("lookup", col).Msg("handoff claim: attempting claim by token")

	// Single-statement atomic claim. The handoff token (hash) alone identifies
	// the cart — it is unique among live tokens — so there is no branch scoping.
	// Only a ready, unexpired, unlocked cart flips to claimed; RowsAffected == 1
	// means we won the race.
	eligible := r.db.Table("handoff_cart_sessions").Select("cart_id").
		Where(col+" = ?", val).
		Where("handoff_expires_at IS NOT NULL AND handoff_expires_at > ?", now).
		Where("(handoff_locked_until IS NULL OR handoff_locked_until < ?)", now)

	var claimedCartID string
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.WithContext(ctx).Model(&models.SaleCart{}).
			Where("kind = ? AND state = ?", models.CartKindHandoff, models.CartReadyForHandoff).
			Where("id IN (?)", eligible).
			Updates(map[string]any{
				"state":      models.CartClaimed,
				"seller_id":  sellerID,
				"updated_at": now,
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return nil // fall through to the explain path below
		}
		// Find the row we just claimed and stamp the satellite bookkeeping.
		cart, err := gorm.G[models.SaleCart](tx).
			Where("kind = ? AND state = ? AND seller_id = ?", models.CartKindHandoff, models.CartClaimed, sellerID).
			Where("id IN (?)", r.db.Table("handoff_cart_sessions").Select("cart_id").Where(col+" = ?", val)).
			Order("updated_at DESC").First(ctx)
		if err != nil {
			return err
		}
		claimedCartID = cart.ID
		return tx.WithContext(ctx).Model(&models.HandoffSession{}).
			Where("cart_id = ?", cart.ID).
			Updates(map[string]any{
				"claimed_at":           now,
				"handoff_attempts":     0,
				"handoff_locked_until": nil,
			}).Error
	})
	if err != nil {
		return nil, err
	}
	if claimedCartID != "" {
		cart, err := r.loadCart(ctx, claimedCartID, nil)
		if err != nil {
			return nil, err
		}
		log.Info().Str("cart_id", cart.ID).Str("seller_id", sellerID).Msg("handoff claim: cart claimed")
		return cart, nil
	}

	// The claim did not apply — explain why on a committed read, never inside the
	// failing transaction, so the attempt bump below actually persists.
	session, err := gorm.G[models.HandoffSession](r.db).Where(col+" = ?", val).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Warn().Str("seller_id", sellerID).Msg("handoff claim: no cart matches the token")
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	cart, err := r.loadCart(ctx, session.CartID, &session)
	if err != nil {
		return nil, err
	}
	// Idempotent double-tap: already claimed by this same seller.
	if cart.State == models.CartClaimed && cart.SellerID != nil && *cart.SellerID == sellerID {
		return cart, nil
	}
	if session.HandoffLockedUntil != nil && now.Before(*session.HandoffLockedUntil) {
		log.Warn().Str("cart_id", cart.ID).Msg("handoff claim: cart is locked out from repeated failures")
		return nil, repoerr.ErrConflict
	}
	if cart.State != models.CartReadyForHandoff {
		log.Warn().Str("cart_id", cart.ID).Str("state", string(cart.State)).Msg("handoff claim: cart not in ready_for_handoff")
		return nil, repoerr.ErrConflict // already claimed / terminal
	}
	// Ready but the token was expired: count the failed attempt (committed).
	log.Warn().Str("cart_id", cart.ID).Msg("handoff claim: handoff token expired")
	r.registerFailedClaim(ctx, &session, now)
	return nil, repoerr.ErrNotFound
}

// Checkout charges a cart of either kind. It reprices every line from
// product_stock at the cart's branch (authoritative — the stored unit price is
// re-derived, never trusted), records the sale by delegating to the journals
// operation flow, decrements stock, and completes the cart — all in one
// transaction, so the money, the stock and the cart state commit together.
//
// idempotencyKey makes retries safe: a replay with the same key returns the
// completed cart WITHOUT re-charging or re-decrementing stock; a different key
// on an already-completed cart is ErrConflict. The journal must be open and at
// the cart's branch.
//
// A handoff cart must have been claimed by this seller; a POS cart must be
// active and owned by them.
func (r *SaleCartRepository) Checkout(ctx context.Context, cartID, sellerID string, in models.CheckoutCartInput, idempotencyKey string) (*models.SaleCart, error) {
	if idempotencyKey == "" || in.JournalID == "" {
		return nil, repoerr.ErrInvalidInput
	}
	if err := models.ValidatePaymentMethod(in.PaymentMethod); err != nil {
		return nil, repoerr.ErrInvalidInput
	}

	var result *models.SaleCart
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var cart models.SaleCart
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", cartID).First(&cart).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repoerr.ErrNotFound
			}
			return err
		}

		// Idempotency replay / conflict. This MUST stay ahead of every write
		// below: a retried checkout that fell through here would charge the
		// customer twice and decrement stock twice.
		if cart.State == models.CartCompleted {
			if cart.CheckoutIdempotencyKey != nil && *cart.CheckoutIdempotencyKey == idempotencyKey {
				result = &cart
				return nil
			}
			return repoerr.ErrConflict
		}
		if err := checkoutAuthorized(&cart, sellerID); err != nil {
			return err
		}

		items, err := gorm.G[models.SaleCartItem](tx).Where("cart_id = ?", cart.ID).Find(ctx)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			return repoerr.ErrInvalidInput
		}

		// Journal must exist, be open, and belong to the cart's branch.
		journal, err := gorm.G[models.Journal](tx).Where("id = ?", in.JournalID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return repoerr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if journal.Shift_is_closed {
			return repoerr.ErrConflict
		}
		if journal.Branch.ID != cart.BranchID {
			return repoerr.ErrInvalidInput
		}

		total, err := r.repriceTotal(ctx, tx, cart.BranchID, items)
		if err != nil {
			return err
		}
		if total == 0 {
			return repoerr.ErrInvalidInput
		}

		// Delegate to the EXISTING sale flow (records the sale + journal entry) in
		// this same transaction. The cart total is already integer so'm, so this
		// hands over an exact figure — no rounding happens at the boundary.
		opInput := models.JournalOperationInput{
			TransactionBase: models.TransactionBase{
				Amount:        total,
				Description:   checkoutDescription(&cart),
				PaymentMethod: in.PaymentMethod,
			},
			CartID: cart.ID,
		}
		if _, err := r.journals.AddOperationTx(ctx, tx, in.JournalID, opInput); err != nil {
			return err
		}

		// Move the stock the sale consumed. Overselling is logged, not blocked —
		// the goods are already leaving the shop.
		if err := inventory.ApplyStockDecrement(ctx, tx, cart.BranchID, items); err != nil {
			return err
		}

		now := time.Now()
		key, jid := idempotencyKey, in.JournalID
		if err := tx.WithContext(ctx).Model(&models.SaleCart{}).Where("id = ?", cart.ID).
			Updates(map[string]any{
				"state":                    models.CartCompleted,
				"checkout_idempotency_key": key,
				"sale_journal_id":          jid,
				"subtotal":                 total,
				"finalized_at":             now,
				"updated_at":               now,
			}).Error; err != nil {
			return err
		}
		cart.State = models.CartCompleted
		cart.CheckoutIdempotencyKey = &key
		cart.SaleJournalID = &jid
		cart.Subtotal = total
		cart.FinalizedAt = &now
		cart.Items = items
		result = &cart
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Items == nil {
		r.attachItems(ctx, result)
	}
	return result, nil
}

// GetByID loads a cart by id (either kind), with its items and — for a handoff
// cart — its session satellite.
func (r *SaleCartRepository) GetByID(ctx context.Context, cartID string) (*models.SaleCart, error) {
	return r.loadCart(ctx, cartID, nil)
}

// ReleaseClaim returns a seller's claimed handoff cart to ready_for_handoff (e.g.
// handed to a different till). CancelClaim ends it (claimed → cancelled). Both
// require the caller to be the claiming seller.
func (r *SaleCartRepository) ReleaseClaim(ctx context.Context, cartID, sellerID string) (*models.SaleCart, error) {
	return r.sellerTerminate(ctx, cartID, sellerID, models.CartReadyForHandoff, map[string]any{
		"seller_id": nil,
	})
}

func (r *SaleCartRepository) CancelClaim(ctx context.Context, cartID, sellerID string) (*models.SaleCart, error) {
	return r.sellerTerminate(ctx, cartID, sellerID, models.CartCancelled, map[string]any{
		"finalized_at": time.Now(),
	})
}

// ---------------------------------------------------------------------------
// Lifecycle (janitor)
// ---------------------------------------------------------------------------

// SweepExpired reaps stale carts of both kinds. It reverts ready_for_handoff
// carts whose handoff token lapsed back to active (clearing the token), then
// expires handoff carts past their session day and POS carts idle beyond
// posCartTTL. Claimed carts are left alone (a seller owns them). Returns the
// number of carts expired.
//
// For POS this replaces the Redis TTL that used to drop abandoned till sessions.
func (r *SaleCartRepository) SweepExpired(ctx context.Context) (int64, error) {
	now := time.Now()

	// 1. Lapsed handoff tokens: back to active, token cleared.
	lapsed := r.db.Table("handoff_cart_sessions").Select("cart_id").
		Where("handoff_expires_at IS NOT NULL AND handoff_expires_at < ?", now)
	if err := r.db.WithContext(ctx).Model(&models.SaleCart{}).
		Where("kind = ? AND state = ?", models.CartKindHandoff, models.CartReadyForHandoff).
		Where("id IN (?)", lapsed).
		Updates(map[string]any{"state": models.CartActive, "updated_at": now}).Error; err != nil {
		return 0, err
	}
	if err := r.db.WithContext(ctx).Model(&models.HandoffSession{}).
		Where("handoff_expires_at IS NOT NULL AND handoff_expires_at < ?", now).
		Updates(map[string]any{
			"handoff_code_hash":    nil,
			"handoff_ref_hash":     nil,
			"handoff_expires_at":   nil,
			"handoff_locked_until": nil,
		}).Error; err != nil {
		return 0, err
	}

	// 2. Handoff carts past their session day.
	stale := r.db.Table("handoff_cart_sessions").Select("cart_id").Where("session_expires_at < ?", now)
	res := r.db.WithContext(ctx).Model(&models.SaleCart{}).
		Where("kind = ? AND state IN ?", models.CartKindHandoff,
			[]models.CartState{models.CartActive, models.CartReadyForHandoff}).
		Where("id IN (?)", stale).
		Updates(map[string]any{"state": models.CartExpired, "finalized_at": now, "updated_at": now})
	if res.Error != nil {
		return 0, res.Error
	}
	expired := res.RowsAffected

	// 3. Abandoned POS carts.
	res = r.db.WithContext(ctx).Model(&models.SaleCart{}).
		Where("kind = ? AND state = ? AND updated_at < ?", models.CartKindPOS, models.CartActive, now.Add(-posCartTTL)).
		Updates(map[string]any{"state": models.CartExpired, "finalized_at": now, "updated_at": now})
	if res.Error != nil {
		return expired, res.Error
	}
	return expired + res.RowsAffected, nil
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

// checkoutDescription is the journal operation description for a cart. It keeps
// the cart id human-visible in the ledger even though the structured link now
// lives in transactions.cart_id — and it preserves the exact 'Scan & Go handoff
// <id>' wording that migration 00004 parses when backfilling older sales.
func checkoutDescription(cart *models.SaleCart) string {
	if cart.Kind == models.CartKindPOS {
		return "POS sale " + cart.ID
	}
	return "Scan & Go handoff " + cart.ID
}

// checkoutAuthorized reports whether sellerID may charge this cart: a handoff
// cart must be claimed by them, a POS cart must be active and theirs.
func checkoutAuthorized(cart *models.SaleCart, sellerID string) error {
	switch cart.Kind {
	case models.CartKindHandoff:
		if cart.State != models.CartClaimed || cart.SellerID == nil || *cart.SellerID != sellerID {
			return repoerr.ErrConflict
		}
	case models.CartKindPOS:
		if cart.State != models.CartActive || cart.SellerID == nil || *cart.SellerID != sellerID {
			return repoerr.ErrConflict
		}
	default:
		return repoerr.ErrInvalidInput
	}
	return nil
}

// branchMustExist returns ErrNotFound when the branch is unknown.
func (r *SaleCartRepository) branchMustExist(ctx context.Context, branchID string) error {
	var n int64
	if err := r.db.WithContext(ctx).Table("branches").Where("id = ?", branchID).Count(&n).Error; err != nil {
		return err
	}
	if n == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// loadCart reads a cart with its items and (for handoff) its session satellite,
// applying lazy expiry. session may be supplied by the caller to avoid a second
// read when it already has it.
func (r *SaleCartRepository) loadCart(ctx context.Context, cartID string, session *models.HandoffSession) (*models.SaleCart, error) {
	cart, err := gorm.G[models.SaleCart](r.db).Where("id = ?", cartID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if cart.Kind == models.CartKindHandoff {
		if session == nil {
			s, err := gorm.G[models.HandoffSession](r.db).Where("cart_id = ?", cartID).First(ctx)
			if err == nil {
				session = &s
			}
		}
		cart.Session = session
	}
	r.lazilyExpire(ctx, &cart)
	r.attachItems(ctx, &cart)
	return &cart, nil
}

// addItem adds (or increments a matching) line on an already-loaded cart.
func (r *SaleCartRepository) addItem(ctx context.Context, cart *models.SaleCart, in models.AddSaleCartItemInput) (*models.SaleCart, error) {
	if in.Quantity <= 0 {
		return nil, repoerr.ErrInvalidInput
	}
	if !cart.State.AllowsEdit(cart.Kind) {
		return nil, repoerr.ErrConflict
	}
	for i := range cart.Items {
		if cart.Items[i].ProductID == in.ProductID {
			cart.Items[i].Quantity += in.Quantity
			return r.saveItems(ctx, cart)
		}
	}
	snap, err := r.productItemSnapshot(ctx, cart.BranchID, in.ProductID)
	if err != nil {
		return nil, err
	}
	snap.ID = uuid.New().String()
	snap.Quantity = in.Quantity
	cart.Items = append(cart.Items, *snap)
	return r.saveItems(ctx, cart)
}

// updateItem sets a line's quantity (<= 0 removes it) on an already-loaded cart.
func (r *SaleCartRepository) updateItem(ctx context.Context, cart *models.SaleCart, itemID string, quantity float64) (*models.SaleCart, error) {
	if !cart.State.AllowsEdit(cart.Kind) {
		return nil, repoerr.ErrConflict
	}
	found := false
	kept := cart.Items[:0]
	for _, it := range cart.Items {
		if it.ID == itemID {
			found = true
			if quantity <= 0 {
				continue
			}
			it.Quantity = quantity
		}
		kept = append(kept, it)
	}
	if !found {
		return nil, repoerr.ErrNotFound
	}
	cart.Items = kept
	return r.saveItems(ctx, cart)
}

// removeItem drops a single line from an already-loaded cart.
func (r *SaleCartRepository) removeItem(ctx context.Context, cart *models.SaleCart, itemID string) (*models.SaleCart, error) {
	if !cart.State.AllowsEdit(cart.Kind) {
		return nil, repoerr.ErrConflict
	}
	found := false
	kept := cart.Items[:0]
	for _, it := range cart.Items {
		if it.ID == itemID {
			found = true
			continue
		}
		kept = append(kept, it)
	}
	if !found {
		return nil, repoerr.ErrNotFound
	}
	cart.Items = kept
	return r.saveItems(ctx, cart)
}

// sellerTerminate moves a seller's own claimed cart to next (release/cancel).
func (r *SaleCartRepository) sellerTerminate(ctx context.Context, cartID, sellerID string, next models.CartState, extra map[string]any) (*models.SaleCart, error) {
	cart, err := r.GetByID(ctx, cartID)
	if err != nil {
		return nil, err
	}
	if cart.SellerID == nil || *cart.SellerID != sellerID {
		return nil, repoerr.ErrConflict
	}
	if err := r.transition(ctx, r.db, cart, next, extra); err != nil {
		return nil, err
	}
	r.attachItems(ctx, cart)
	return cart, nil
}

// transition enforces the kind-aware state machine and persists the move plus
// any extra column writes. An illegal move is ErrConflict.
func (r *SaleCartRepository) transition(ctx context.Context, db *gorm.DB, cart *models.SaleCart, next models.CartState, extra map[string]any) error {
	if !cart.State.CanTransitionTo(cart.Kind, next) {
		return repoerr.ErrConflict
	}
	updates := map[string]any{"state": next, "updated_at": time.Now()}
	for k, v := range extra {
		updates[k] = v
	}
	if err := db.WithContext(ctx).Model(&models.SaleCart{}).
		Where("id = ? AND state = ?", cart.ID, cart.State).Updates(updates).Error; err != nil {
		return err
	}
	cart.State = next
	return nil
}

// lazilyExpire applies time-based transitions on read: a stale non-terminal cart
// becomes expired; a ready handoff cart whose token lapsed reverts to active.
func (r *SaleCartRepository) lazilyExpire(ctx context.Context, cart *models.SaleCart) {
	now := time.Now()
	if cart.State.IsTerminal() {
		return
	}

	if cart.Kind == models.CartKindPOS {
		if now.Sub(cart.UpdatedAt) > posCartTTL {
			_ = r.transition(ctx, r.db, cart, models.CartExpired, map[string]any{"finalized_at": now})
		}
		return
	}

	if cart.Session == nil {
		return
	}
	if now.After(cart.Session.SessionExpiresAt) {
		_ = r.transition(ctx, r.db, cart, models.CartExpired, map[string]any{"finalized_at": now})
		return
	}
	if cart.State == models.CartReadyForHandoff &&
		cart.Session.HandoffExpiresAt != nil && now.After(*cart.Session.HandoffExpiresAt) {
		if err := r.transition(ctx, r.db, cart, models.CartActive, nil); err == nil {
			_ = r.db.WithContext(ctx).Model(&models.HandoffSession{}).Where("cart_id = ?", cart.ID).
				Updates(map[string]any{
					"handoff_code_hash":  nil,
					"handoff_ref_hash":   nil,
					"handoff_expires_at": nil,
				}).Error
			cart.Session.HandoffExpiresAt = nil
		}
	}
}

// registerFailedClaim bumps the per-cart failed-attempt counter and locks the
// cart out once it crosses maxHandoffAttempts. Best-effort (committed on r.db).
func (r *SaleCartRepository) registerFailedClaim(ctx context.Context, session *models.HandoffSession, now time.Time) {
	attempts := session.HandoffAttempts + 1
	updates := map[string]any{"handoff_attempts": attempts}
	if attempts >= maxHandoffAttempts {
		updates["handoff_locked_until"] = now.Add(handoffLockout)
	}
	_ = r.db.WithContext(ctx).Model(&models.HandoffSession{}).
		Where("cart_id = ?", session.CartID).Updates(updates).Error
}

// saveItems recomputes every line total through models.RoundLineTotal, rewrites
// the child rows (delete-all + insert) and rolls the integer subtotal onto the
// cart header, in one transaction.
func (r *SaleCartRepository) saveItems(ctx context.Context, cart *models.SaleCart) (*models.SaleCart, error) {
	now := time.Now()
	var subtotal uint32
	for i := range cart.Items {
		cart.Items[i].LineTotal = models.RoundLineTotal(cart.Items[i].Quantity, cart.Items[i].UnitPrice)
		subtotal += cart.Items[i].LineTotal
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Model(&models.SaleCart{}).Where("id = ?", cart.ID).
			Updates(map[string]any{"updated_at": now, "subtotal": subtotal}).Error; err != nil {
			return err
		}
		if _, err := gorm.G[models.SaleCartItem](tx).Where("cart_id = ?", cart.ID).Delete(ctx); err != nil {
			return err
		}
		if len(cart.Items) > 0 {
			items := make([]models.SaleCartItem, len(cart.Items))
			copy(items, cart.Items)
			for i := range items {
				items[i].CartID = cart.ID
				if items[i].ID == "" {
					items[i].ID = uuid.New().String()
				}
			}
			if err := gorm.G[models.SaleCartItem](tx).CreateInBatches(ctx, &items, len(items)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.loadCart(ctx, cart.ID, cart.Session)
}

// attachItems loads a cart's items (best-effort; never nil).
func (r *SaleCartRepository) attachItems(ctx context.Context, cart *models.SaleCart) {
	if cart == nil {
		return
	}
	items, err := gorm.G[models.SaleCartItem](r.db).Where("cart_id = ?", cart.ID).Order("id").Find(ctx)
	if err == nil {
		cart.Items = items
	}
	if cart.Items == nil {
		cart.Items = []models.SaleCartItem{}
	}
}

// productItemSnapshot resolves a product through the catalog and prices it from
// product_stock at the branch, returning a pre-filled line. ErrNotFound if the
// product does not exist or is not stocked at that branch.
func (r *SaleCartRepository) productItemSnapshot(ctx context.Context, branchID, productID string) (*models.SaleCartItem, error) {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", productID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	stock, err := gorm.G[models.ProductDistribution](r.db).
		Where("product_id = ? AND place_id = ?", productID, branchID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &models.SaleCartItem{
		ProductID: productID,
		SKU:       product.SKU,
		Name:      product.Name,
		UOM:       stock.Unit,
		UnitPrice: uint32(stock.Price),
	}, nil
}

// repriceTotal sums the authoritative price (product_stock at the branch) of
// every line — the total charged at checkout. It rounds per line with the SAME
// helper the display path uses, so the charged figure reproduces the basket the
// customer was shown rather than re-rounding a float sum.
func (r *SaleCartRepository) repriceTotal(ctx context.Context, tx *gorm.DB, branchID string, items []models.SaleCartItem) (uint32, error) {
	var total uint32
	for _, it := range items {
		stock, err := gorm.G[models.ProductDistribution](tx).
			Where("product_id = ? AND place_id = ?", it.ProductID, branchID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, repoerr.ErrInvalidInput
		}
		if err != nil {
			return 0, err
		}
		total += models.RoundLineTotal(it.Quantity, uint32(stock.Price))
	}
	return total, nil
}
