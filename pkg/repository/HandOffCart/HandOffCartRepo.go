// Package handoffcart_repo is the repository for the Scan & Go handoff cart (the
// `handoff_carts` header table plus its `handoff_cart_lines` child table). It
// owns the cart lifecycle and enforces the state machine (models.HandoffState):
// customers build a cart authenticated by a high-entropy session token; at
// checkout a short-lived handoff token is minted; a seller claims the cart
// (freezing customer edits) and charges it, delegating to the EXISTING journal
// operation (sale) flow — this package writes no new sale/stock/journal logic.
//
// It follows the repo's GORM generics idiom (gorm.G[T]) and manages the child
// line table explicitly, mirroring pkg/repository/store/cart.
package handoffcart_repo

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	journalsrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/journals"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// HandOffCartRepository owns the handoff_carts / handoff_cart_lines tables and
// delegates the sale at checkout to the journals repository.
type HandOffCartRepository struct {
	db       *gorm.DB
	journals *journalsrepo.JournalsRepository
}

func New(db *gorm.DB) *HandOffCartRepository {
	return &HandOffCartRepository{
		db:       db,
		journals: journalsrepo.New(db),
	}
}

// ---------------------------------------------------------------------------
// Customer surface (authenticated by the session token)
// ---------------------------------------------------------------------------

// StartSession creates a new active cart scoped to branchID (from the entry QR)
// and mints its session token. The plaintext token is returned once (for the
// entry-QR link); only its digest is stored. A missing branch is ErrNotFound.
func (r *HandOffCartRepository) StartSession(ctx context.Context, branchID string) (*models.HandoffCart, string, error) {
	if branchID == "" {
		return nil, "", repoerr.ErrInvalidInput
	}
	var n int64
	if err := r.db.WithContext(ctx).Table("branches").Where("id = ?", branchID).Count(&n).Error; err != nil {
		return nil, "", err
	}
	if n == 0 {
		return nil, "", repoerr.ErrNotFound
	}

	token, hash, err := newSessionToken()
	if err != nil {
		return nil, "", err
	}
	now := time.Now()
	cart := &models.HandoffCart{
		ID:               uuid.New().String(),
		BranchID:         branchID,
		State:            models.HandoffActive,
		SessionTokenHash: hash,
		SessionExpiresAt: endOfDay(now),
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	if err := r.db.WithContext(ctx).Create(cart).Error; err != nil {
		return nil, "", err
	}
	cart.Lines = []models.HandoffCartLine{}
	return cart, token, nil
}

// GetBySessionToken loads a cart by its session token, applying lazy expiry
// (a session past its day, or a lapsed handoff token) before returning it.
func (r *HandOffCartRepository) GetBySessionToken(ctx context.Context, token string) (*models.HandoffCart, error) {
	if token == "" {
		return nil, repoerr.ErrInvalidInput
	}
	cart, err := gorm.G[models.HandoffCart](r.db).Where("session_token_hash = ?", hashToken(token)).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.lazilyExpire(ctx, &cart)
	r.attachLines(ctx, &cart)
	return &cart, nil
}

// AddLine adds (or increments a matching) scanned line. The product is resolved
// through the catalog and priced from product_stock at the cart's branch; the
// phone never supplies a price. Rejected with ErrConflict once editing is frozen.
func (r *HandOffCartRepository) AddLine(ctx context.Context, token string, in models.AddHandoffLineInput) (*models.HandoffCart, error) {
	if in.Quantity <= 0 {
		return nil, repoerr.ErrInvalidInput
	}
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !cart.State.AllowsCustomerEdit() {
		return nil, repoerr.ErrConflict
	}
	for i := range cart.Lines {
		if cart.Lines[i].ProductID == in.ProductID {
			cart.Lines[i].Quantity += in.Quantity
			return r.saveLines(ctx, cart)
		}
	}
	snap, err := r.productLineSnapshot(ctx, cart.BranchID, in.ProductID)
	if err != nil {
		return nil, err
	}
	snap.ID = uuid.New().String()
	snap.Quantity = in.Quantity
	cart.Lines = append(cart.Lines, *snap)
	return r.saveLines(ctx, cart)
}

// UpdateLine sets a line's quantity (<= 0 removes it). Frozen after claim.
func (r *HandOffCartRepository) UpdateLine(ctx context.Context, token, lineID string, quantity int) (*models.HandoffCart, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !cart.State.AllowsCustomerEdit() {
		return nil, repoerr.ErrConflict
	}
	found := false
	kept := cart.Lines[:0]
	for _, l := range cart.Lines {
		if l.ID == lineID {
			found = true
			if quantity <= 0 {
				continue
			}
			l.Quantity = quantity
		}
		kept = append(kept, l)
	}
	if !found {
		return nil, repoerr.ErrNotFound
	}
	cart.Lines = kept
	return r.saveLines(ctx, cart)
}

// RemoveLine drops a single line. Frozen after claim.
func (r *HandOffCartRepository) RemoveLine(ctx context.Context, token, lineID string) (*models.HandoffCart, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if !cart.State.AllowsCustomerEdit() {
		return nil, repoerr.ErrConflict
	}
	found := false
	kept := cart.Lines[:0]
	for _, l := range cart.Lines {
		if l.ID == lineID {
			found = true
			continue
		}
		kept = append(kept, l)
	}
	if !found {
		return nil, repoerr.ErrNotFound
	}
	cart.Lines = kept
	return r.saveLines(ctx, cart)
}

// RequestHandoff transitions the cart to ready_for_handoff and mints a fresh
// handoff token (re-minting if already ready). The plaintext code + QR ref are
// returned once. An empty cart, or a cart past the editable states, is rejected.
func (r *HandOffCartRepository) RequestHandoff(ctx context.Context, token string) (*models.HandoffCart, string, string, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, "", "", err
	}
	if cart.State != models.HandoffActive && cart.State != models.HandoffReadyForHandoff {
		return nil, "", "", repoerr.ErrConflict
	}
	if len(cart.Lines) == 0 {
		return nil, "", "", repoerr.ErrInvalidInput
	}
	if cart.State == models.HandoffActive && !cart.State.CanTransitionTo(models.HandoffReadyForHandoff) {
		return nil, "", "", repoerr.ErrConflict
	}

	code, ref, codeHash, refHash, err := newHandoffToken()
	if err != nil {
		return nil, "", "", err
	}
	now := time.Now()
	exp := now.Add(handoffTTL)
	res := r.db.WithContext(ctx).Model(&models.HandoffCart{}).
		Where("id = ? AND state IN ?", cart.ID, []models.HandoffState{models.HandoffActive, models.HandoffReadyForHandoff}).
		Updates(map[string]interface{}{
			"state":                models.HandoffReadyForHandoff,
			"handoff_code_hash":    codeHash,
			"handoff_ref_hash":     refHash,
			"handoff_expires_at":   exp,
			"handoff_attempts":     0,
			"handoff_locked_until": nil,
			"updated_at":           now,
		})
	if res.Error != nil {
		return nil, "", "", res.Error
	}
	if res.RowsAffected == 0 {
		return nil, "", "", repoerr.ErrConflict
	}
	cart.State = models.HandoffReadyForHandoff
	cart.HandoffExpiresAt = &exp
	return cart, code, ref, nil
}

// CancelBySession lets the customer cancel their own cart (active/ready → cancelled).
func (r *HandOffCartRepository) CancelBySession(ctx context.Context, token string) (*models.HandoffCart, error) {
	cart, err := r.GetBySessionToken(ctx, token)
	if err != nil {
		return nil, err
	}
	if err := r.transition(ctx, cart, models.HandoffCancelled, map[string]interface{}{"finalized_at": time.Now()}); err != nil {
		return nil, err
	}
	return cart, nil
}

// ---------------------------------------------------------------------------
// Seller surface (authenticated POS session, capability + branch checked)
// ---------------------------------------------------------------------------

// ClaimByToken atomically claims a ready cart for a seller at the same branch,
// freezing customer edits. Idempotent: a repeat by the same seller returns the
// already-claimed cart. Invalid/expired tokens are ErrNotFound (and bump the
// per-cart failed-attempt counter / lockout); an already-claimed-by-another or
// locked cart is ErrConflict. Prefers the higher-entropy QR ref over the code.
func (r *HandOffCartRepository) ClaimByToken(ctx context.Context, in models.ClaimHandoffInput, sellerID string) (*models.HandoffCart, error) {
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

	// Single-statement atomic claim: the handoff token (hash) alone identifies the
	// cart — it is unique among ready_for_handoff carts — so there is no branch
	// scoping. Only a ready, unexpired, unlocked cart flips to claimed;
	// RowsAffected==1 means we won the race.
	res := r.db.WithContext(ctx).Model(&models.HandoffCart{}).
		Where(col+" = ? AND state = ?", val, models.HandoffReadyForHandoff).
		Where("handoff_expires_at IS NOT NULL AND handoff_expires_at > ?", now).
		Where("(handoff_locked_until IS NULL OR handoff_locked_until < ?)", now).
		Updates(map[string]interface{}{
			"state":                models.HandoffClaimed,
			"claimed_by":           sellerID,
			"claimed_at":           now,
			"handoff_attempts":     0,
			"handoff_locked_until": nil,
			"updated_at":           now,
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 1 {
		cart, err := gorm.G[models.HandoffCart](r.db).
			Where(col+" = ? AND claimed_by = ?", val, sellerID).
			Order("claimed_at DESC").First(ctx)
		if err != nil {
			return nil, err
		}
		r.attachLines(ctx, &cart)
		log.Info().Str("cart_id", cart.ID).Str("seller_id", sellerID).Msg("handoff claim: cart claimed")
		return &cart, nil
	}

	// Claim did not apply — explain why on a committed read (never inside the
	// failing path, so the attempt bump below actually persists).
	cart, err := gorm.G[models.HandoffCart](r.db).
		Where(col+" = ?", val).
		Order("created_at DESC").First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		log.Warn().Str("seller_id", sellerID).Msg("handoff claim: no cart matches the token")
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	// Idempotent double-tap: already claimed by this same seller.
	if cart.State == models.HandoffClaimed && cart.ClaimedBy != nil && *cart.ClaimedBy == sellerID {
		r.attachLines(ctx, &cart)
		return &cart, nil
	}
	if cart.HandoffLockedUntil != nil && now.Before(*cart.HandoffLockedUntil) {
		log.Warn().Str("cart_id", cart.ID).Msg("handoff claim: cart is locked out from repeated failures")
		return nil, repoerr.ErrConflict
	}
	if cart.State != models.HandoffReadyForHandoff {
		log.Warn().Str("cart_id", cart.ID).Str("state", string(cart.State)).Msg("handoff claim: cart not in ready_for_handoff")
		return nil, repoerr.ErrConflict // already claimed / terminal
	}
	// Ready but the token was expired: count the failed attempt (committed).
	log.Warn().Str("cart_id", cart.ID).Msg("handoff claim: handoff token expired")
	r.registerFailedClaim(ctx, &cart, now)
	return nil, repoerr.ErrNotFound
}

// Checkout charges a claimed cart: it reprices every line from product_stock at
// the cart's branch (authoritative — the display price is never trusted) and
// records the sale by delegating to the journals operation flow, all in one
// transaction with the cart's completion. idempotencyKey makes retries safe: a
// replay with the same key returns the completed cart; a different key on an
// already-completed cart is ErrConflict. The journal must be open and at the
// cart's branch.
func (r *HandOffCartRepository) Checkout(ctx context.Context, cartID, sellerID string, in models.CheckoutHandoffInput, idempotencyKey string) (*models.HandoffCart, error) {
	if idempotencyKey == "" || in.JournalID == "" {
		return nil, repoerr.ErrInvalidInput
	}
	if err := models.ValidatePaymentMethod(in.PaymentMethod); err != nil {
		return nil, repoerr.ErrInvalidInput
	}

	var result *models.HandoffCart
	err := r.db.Transaction(func(tx *gorm.DB) error {
		var cart models.HandoffCart
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", cartID).First(&cart).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return repoerr.ErrNotFound
			}
			return err
		}

		// Idempotency replay / conflict.
		if cart.State == models.HandoffCompleted {
			if cart.CheckoutIdempotencyKey != nil && *cart.CheckoutIdempotencyKey == idempotencyKey {
				result = &cart
				return nil
			}
			return repoerr.ErrConflict
		}
		if cart.State != models.HandoffClaimed || cart.ClaimedBy == nil || *cart.ClaimedBy != sellerID {
			return repoerr.ErrConflict
		}

		lines, err := gorm.G[models.HandoffCartLine](tx).Where("cart_id = ?", cart.ID).Find(ctx)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
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

		total, err := r.repriceTotal(ctx, tx, cart.BranchID, lines)
		if err != nil {
			return err
		}
		if total <= 0 {
			return repoerr.ErrInvalidInput
		}

		// Delegate to the EXISTING sale flow (records the sale + journal entry) in
		// this same transaction, so the sale and the cart completion commit together.
		opInput := models.JournalOperationInput{
			TransactionBase: models.TransactionBase{
				Amount:        uint32(total),
				Description:   "Scan & Go handoff " + cart.ID,
				PaymentMethod: in.PaymentMethod,
			},
		}
		if _, err := r.journals.AddOperationTx(ctx, tx, in.JournalID, opInput); err != nil {
			return err
		}

		now := time.Now()
		key, jid := idempotencyKey, in.JournalID
		if err := tx.WithContext(ctx).Model(&models.HandoffCart{}).Where("id = ?", cart.ID).Updates(map[string]interface{}{
			"state":                    models.HandoffCompleted,
			"checkout_idempotency_key": key,
			"sale_journal_id":          jid,
			"finalized_at":             now,
			"updated_at":               now,
		}).Error; err != nil {
			return err
		}
		cart.State = models.HandoffCompleted
		cart.CheckoutIdempotencyKey = &key
		cart.SaleJournalID = &jid
		cart.FinalizedAt = &now
		cart.Lines = lines
		result = &cart
		return nil
	})
	if err != nil {
		return nil, err
	}
	if result.Lines == nil {
		r.attachLines(ctx, result)
	}
	return result, nil
}

// GetByIDForSeller loads a cart by id. It is not branch-scoped: a seller may
// operate across stores, so the cart is identified by its id alone (the seller
// reached it via a claim, which is gated by the handoff token).
func (r *HandOffCartRepository) GetByIDForSeller(ctx context.Context, cartID string) (*models.HandoffCart, error) {
	cart, err := gorm.G[models.HandoffCart](r.db).Where("id = ?", cartID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.lazilyExpire(ctx, &cart)
	r.attachLines(ctx, &cart)
	return &cart, nil
}

// ReleaseClaim returns a seller's claimed cart to ready_for_handoff (e.g. handed
// to a different till). CancelClaim ends it (claimed → cancelled). Both require
// the caller to be the claiming seller.
func (r *HandOffCartRepository) ReleaseClaim(ctx context.Context, cartID, sellerID string) (*models.HandoffCart, error) {
	return r.sellerTerminate(ctx, cartID, sellerID, models.HandoffReadyForHandoff, map[string]interface{}{
		"claimed_by": nil, "claimed_at": nil,
	})
}

func (r *HandOffCartRepository) CancelClaim(ctx context.Context, cartID, sellerID string) (*models.HandoffCart, error) {
	return r.sellerTerminate(ctx, cartID, sellerID, models.HandoffCancelled, map[string]interface{}{
		"finalized_at": time.Now(),
	})
}

// ---------------------------------------------------------------------------
// Lifecycle (janitor)
// ---------------------------------------------------------------------------

// SweepExpired reaps stale carts: it reverts ready_for_handoff carts whose
// handoff token lapsed back to active (clearing the token), then expires
// active/ready_for_handoff carts past their session day. Claimed carts are left
// alone (a seller owns them). Returns the number of carts expired.
func (r *HandOffCartRepository) SweepExpired(ctx context.Context) (int64, error) {
	now := time.Now()
	if err := r.db.WithContext(ctx).Model(&models.HandoffCart{}).
		Where("state = ? AND handoff_expires_at IS NOT NULL AND handoff_expires_at < ?", models.HandoffReadyForHandoff, now).
		Updates(map[string]interface{}{
			"state":                models.HandoffActive,
			"handoff_code_hash":    nil,
			"handoff_ref_hash":     nil,
			"handoff_expires_at":   nil,
			"handoff_locked_until": nil,
			"updated_at":           now,
		}).Error; err != nil {
		return 0, err
	}
	res := r.db.WithContext(ctx).Model(&models.HandoffCart{}).
		Where("state IN ? AND session_expires_at < ?", []models.HandoffState{models.HandoffActive, models.HandoffReadyForHandoff}, now).
		Updates(map[string]interface{}{
			"state":        models.HandoffExpired,
			"finalized_at": now,
			"updated_at":   now,
		})
	return res.RowsAffected, res.Error
}

// ---------------------------------------------------------------------------
// internals
// ---------------------------------------------------------------------------

// sellerTerminate moves a seller's own claimed cart to next (release/cancel).
func (r *HandOffCartRepository) sellerTerminate(ctx context.Context, cartID, sellerID string, next models.HandoffState, extra map[string]interface{}) (*models.HandoffCart, error) {
	cart, err := gorm.G[models.HandoffCart](r.db).Where("id = ?", cartID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if cart.ClaimedBy == nil || *cart.ClaimedBy != sellerID {
		return nil, repoerr.ErrConflict
	}
	if err := r.transition(ctx, &cart, next, extra); err != nil {
		return nil, err
	}
	r.attachLines(ctx, &cart)
	return &cart, nil
}

// transition enforces the state machine and persists the move plus any extra
// column writes. An illegal move is ErrConflict.
func (r *HandOffCartRepository) transition(ctx context.Context, cart *models.HandoffCart, next models.HandoffState, extra map[string]interface{}) error {
	if !cart.State.CanTransitionTo(next) {
		return repoerr.ErrConflict
	}
	updates := map[string]interface{}{"state": next, "updated_at": time.Now()}
	for k, v := range extra {
		updates[k] = v
	}
	if err := r.db.WithContext(ctx).Model(&models.HandoffCart{}).
		Where("id = ? AND state = ?", cart.ID, cart.State).Updates(updates).Error; err != nil {
		return err
	}
	cart.State = next
	return nil
}

// lazilyExpire applies time-based transitions on read: a stale non-terminal cart
// becomes expired; a ready cart whose handoff token lapsed reverts to active.
func (r *HandOffCartRepository) lazilyExpire(ctx context.Context, cart *models.HandoffCart) {
	now := time.Now()
	if !cart.State.IsTerminal() && now.After(cart.SessionExpiresAt) {
		_ = r.transition(ctx, cart, models.HandoffExpired, map[string]interface{}{"finalized_at": now})
		return
	}
	if cart.State == models.HandoffReadyForHandoff && cart.HandoffExpiresAt != nil && now.After(*cart.HandoffExpiresAt) {
		_ = r.transition(ctx, cart, models.HandoffActive, map[string]interface{}{
			"handoff_code_hash":  nil,
			"handoff_ref_hash":   nil,
			"handoff_expires_at": nil,
		})
		cart.HandoffExpiresAt = nil
	}
}

// registerFailedClaim bumps the per-cart failed-attempt counter and locks the
// cart out once it crosses maxHandoffAttempts. Best-effort (committed on r.db).
func (r *HandOffCartRepository) registerFailedClaim(ctx context.Context, cart *models.HandoffCart, now time.Time) {
	attempts := cart.HandoffAttempts + 1
	updates := map[string]interface{}{"handoff_attempts": attempts, "updated_at": now}
	if attempts >= maxHandoffAttempts {
		updates["handoff_locked_until"] = now.Add(handoffLockout)
	}
	_ = r.db.WithContext(ctx).Model(&models.HandoffCart{}).Where("id = ?", cart.ID).Updates(updates).Error
}

// saveLines recomputes line totals and rewrites the child rows (delete-all +
// insert, mirroring the store cart), touching the cart's updated_at.
func (r *HandOffCartRepository) saveLines(ctx context.Context, cart *models.HandoffCart) (*models.HandoffCart, error) {
	now := time.Now()
	for i := range cart.Lines {
		cart.Lines[i].LineTotal = cart.Lines[i].DisplayPrice * cart.Lines[i].Quantity
	}
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Model(&models.HandoffCart{}).Where("id = ?", cart.ID).Update("updated_at", now).Error; err != nil {
			return err
		}
		if _, err := gorm.G[models.HandoffCartLine](tx).Where("cart_id = ?", cart.ID).Delete(ctx); err != nil {
			return err
		}
		if len(cart.Lines) > 0 {
			lines := make([]models.HandoffCartLine, len(cart.Lines))
			copy(lines, cart.Lines)
			for i := range lines {
				lines[i].CartID = cart.ID
				if lines[i].ID == "" {
					lines[i].ID = uuid.New().String()
				}
			}
			if err := gorm.G[models.HandoffCartLine](tx).CreateInBatches(ctx, &lines, len(lines)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.reload(ctx, cart.ID)
}

// reload re-reads a cart (with lines) by id.
func (r *HandOffCartRepository) reload(ctx context.Context, cartID string) (*models.HandoffCart, error) {
	cart, err := gorm.G[models.HandoffCart](r.db).Where("id = ?", cartID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.attachLines(ctx, &cart)
	return &cart, nil
}

// attachLines loads a cart's lines (best-effort; never nil).
func (r *HandOffCartRepository) attachLines(ctx context.Context, cart *models.HandoffCart) {
	if cart == nil {
		return
	}
	lines, err := gorm.G[models.HandoffCartLine](r.db).Where("cart_id = ?", cart.ID).Order("id").Find(ctx)
	if err == nil {
		cart.Lines = lines
	}
	if cart.Lines == nil {
		cart.Lines = []models.HandoffCartLine{}
	}
}

// productLineSnapshot resolves a product through the catalog and prices it from
// product_stock at the branch, returning a pre-filled line. ErrNotFound if the
// product does not exist or is not stocked at that branch.
func (r *HandOffCartRepository) productLineSnapshot(ctx context.Context, branchID, productID string) (*models.HandoffCartLine, error) {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", productID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	stock, err := gorm.G[models.ProductDistribution](r.db).Where("product_id = ? AND place_id = ?", productID, branchID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &models.HandoffCartLine{
		ProductID:    productID,
		SKU:          product.SKU,
		Name:         product.Name,
		UOM:          stock.Unit,
		DisplayPrice: int(stock.Price),
	}, nil
}

// repriceTotal sums the authoritative price (product_stock at the branch) of
// every line — the total charged at checkout, ignoring any display price.
func (r *HandOffCartRepository) repriceTotal(ctx context.Context, tx *gorm.DB, branchID string, lines []models.HandoffCartLine) (int, error) {
	total := 0
	for _, l := range lines {
		stock, err := gorm.G[models.ProductDistribution](tx).Where("product_id = ? AND place_id = ?", l.ProductID, branchID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return 0, repoerr.ErrInvalidInput
		}
		if err != nil {
			return 0, err
		}
		total += int(stock.Price) * l.Quantity
	}
	return total, nil
}
