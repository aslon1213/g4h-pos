package models

import (
	"math"
	"slices"
	"time"
)

// ---------------------------------------------------------------------------
// Money and quantity
//
// Money is INTEGER so'm and never fractional. Quantity IS fractional, because
// weighted goods sell by the kilogram (1.500 kg). The two meet in RoundLineTotal
// below, which is the single place a fractional quantity becomes integer money.
// ---------------------------------------------------------------------------

// RoundLineTotal returns the so'm total for a line: quantity * unitPrice rounded
// to the nearest whole so'm.
//
// This is deliberately the ONLY rounding in the cart pipeline, and every caller
// must go through it — both the display path (when an item is added or edited)
// and the authoritative repricing at checkout. A cart's total is then the exact
// integer SUM of already-rounded line totals, never a rounding of the sum.
//
// Rounding per line rather than on the total matters twice over:
//   - the printed line totals actually add up to the printed total, and
//   - checkout reproduces exactly the figure the customer was shown, so a
//     re-price at charge time can never disagree with the basket by a stray so'm.
func RoundLineTotal(quantity float64, unitPrice uint32) uint32 {
	if quantity <= 0 || unitPrice == 0 {
		return 0
	}
	return uint32(math.Round(quantity * float64(unitPrice)))
}

// ---------------------------------------------------------------------------
// Cart kind and lifecycle
// ---------------------------------------------------------------------------

// CartKind is the surface that owns a cart. It selects the legal state
// transitions and decides whether a handoff satellite row exists.
type CartKind string

const (
	// CartKindHandoff is a Scan & Go cart: created by a customer scanning a
	// branch entry QR, authenticated by a session token, handed to a seller.
	CartKindHandoff CartKind = "handoff"
	// CartKindPOS is a till cart: opened by an authenticated staff user, edited
	// and charged by that same user. It has no session or handoff token.
	CartKindPOS CartKind = "pos"
)

// CartState is the lifecycle shared by both cart kinds. Transitions are enforced
// by cartTransitions / CanTransitionTo and backed by a CHECK constraint on the
// state column.
type CartState string

const (
	// CartActive: the cart is being built; items may be added/edited/removed.
	CartActive CartState = "active"
	// CartReadyForHandoff (handoff only): the customer requested checkout and a
	// handoff token is minted. The cart is still customer-editable.
	CartReadyForHandoff CartState = "ready_for_handoff"
	// CartClaimed (handoff only): a seller claimed the cart; edit rights transfer
	// to the seller and customer writes are rejected (409).
	CartClaimed CartState = "claimed"
	// CartCompleted: charged, and a sale was recorded. Terminal.
	CartCompleted CartState = "completed"
	// CartExpired: a stale non-terminal cart was reaped. Terminal.
	CartExpired CartState = "expired"
	// CartCancelled: explicitly cancelled/released to a dead end. Terminal.
	CartCancelled CartState = "cancelled"
)

// cartTransitions is the per-kind whitelist of legal state moves. A POS cart
// never reaches ready_for_handoff or claimed: the seller who opened it already
// holds it, so there is nobody to hand it to. Item edits are NOT transitions —
// they are gated separately by AllowsEdit.
var cartTransitions = map[CartKind]map[CartState][]CartState{
	CartKindHandoff: {
		CartActive:          {CartReadyForHandoff, CartExpired, CartCancelled},
		CartReadyForHandoff: {CartClaimed, CartActive, CartExpired, CartCancelled},
		CartClaimed:         {CartCompleted, CartReadyForHandoff, CartCancelled},
		CartCompleted:       {},
		CartExpired:         {},
		CartCancelled:       {},
	},
	CartKindPOS: {
		CartActive:    {CartCompleted, CartExpired, CartCancelled},
		CartCompleted: {},
		CartExpired:   {},
		CartCancelled: {},
	},
}

// terminalStates are end states with no outgoing transitions, for either kind.
var terminalStates = []CartState{CartCompleted, CartExpired, CartCancelled}

// CanTransitionTo reports whether moving from s to next is legal for this kind.
func (s CartState) CanTransitionTo(kind CartKind, next CartState) bool {
	return slices.Contains(cartTransitions[kind][s], next)
}

// IsTerminal reports whether s is an end state.
func (s CartState) IsTerminal() bool {
	return slices.Contains(terminalStates, s)
}

// AllowsEdit reports whether items may still be added/edited/removed in state s.
// A handoff cart freezes the moment a seller claims it; a POS cart is editable
// right up until it is charged.
func (s CartState) AllowsEdit(kind CartKind) bool {
	if kind == CartKindPOS {
		return s == CartActive
	}
	return s == CartActive || s == CartReadyForHandoff
}

// ---------------------------------------------------------------------------
// Tables
// ---------------------------------------------------------------------------

// SaleCartItem is a single scanned line, shared by handoff and POS carts (the
// `sale_cart_items` table). Quantity is fractional for weighted goods; UnitPrice
// is the authoritative price copied from product_stock at the cart's branch, and
// LineTotal is always RoundLineTotal(Quantity, UnitPrice).
//
// The storefront basket (models.CartItem / `cart_items`) is a separate,
// customer-facing table and is intentionally NOT unified with this one.
type SaleCartItem struct {
	ID        string  `json:"id" bson:"id" gorm:"column:id;primaryKey"`
	CartID    string  `json:"-" bson:"-" gorm:"column:cart_id"`
	ProductID string  `json:"product_id" bson:"product_id" gorm:"column:product_id"`
	Name      string  `json:"name" bson:"name" gorm:"column:name"`
	SKU       string  `json:"sku" bson:"sku" gorm:"column:sku"`
	UOM       string  `json:"uom" bson:"uom" gorm:"column:uom"`
	Quantity  float64 `json:"quantity" bson:"quantity" gorm:"column:quantity"`
	UnitPrice uint32  `json:"unit_price" bson:"unit_price" gorm:"column:unit_price"`
	LineTotal uint32  `json:"line_total" bson:"line_total" gorm:"column:line_total"`
}

func (SaleCartItem) TableName() string { return "sale_cart_items" }

// SaleCart is a server-side cart at one branch, for either surface (the
// `sale_carts` table). Subtotal is the exact integer sum of the items' already
// rounded line totals.
type SaleCart struct {
	ID       string    `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	Kind     CartKind  `json:"kind" bson:"kind" gorm:"column:kind"`
	BranchID string    `json:"branch_id" bson:"branch_id" gorm:"column:branch_id"`
	State    CartState `json:"state" bson:"state" gorm:"column:state"`

	// POS: the staff user who opened the cart. Handoff: the seller who claimed it.
	SellerID *string `json:"seller_id,omitempty" bson:"seller_id" gorm:"column:seller_id"`

	Subtotal uint32 `json:"subtotal" bson:"subtotal" gorm:"column:subtotal"`

	CheckoutIdempotencyKey *string `json:"-" bson:"-" gorm:"column:checkout_idempotency_key"`
	SaleJournalID          *string `json:"sale_journal_id,omitempty" bson:"sale_journal_id" gorm:"column:sale_journal_id"`

	FinalizedAt *time.Time `json:"finalized_at,omitempty" bson:"finalized_at" gorm:"column:finalized_at"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time  `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`

	Items []SaleCartItem `json:"items" bson:"items" gorm:"-"`

	// Session is the handoff satellite, attached only for Kind == CartKindHandoff.
	Session *HandoffSession `json:"session,omitempty" bson:"-" gorm:"-"`
}

func (SaleCart) TableName() string { return "sale_carts" }

// HandoffSession holds the Scan & Go credentials and claim bookkeeping for a
// handoff cart (the `handoff_cart_sessions` satellite). Token digests are never
// exposed (json:"-"); the plaintext tokens are returned exactly once at mint
// time via the response DTOs below. A POS cart has no row here.
type HandoffSession struct {
	CartID string `json:"-" bson:"-" gorm:"column:cart_id;primaryKey"`

	// session token (entry-QR bearer) — hash only.
	SessionTokenHash string    `json:"-" bson:"-" gorm:"column:session_token_hash"`
	SessionExpiresAt time.Time `json:"session_expires_at" bson:"session_expires_at" gorm:"column:session_expires_at"`

	// handoff token (minted at checkout) — hashes only.
	//
	// POINTERS, deliberately: an un-minted token must be SQL NULL, not the empty
	// string. The uniqueness of a live handoff code is enforced by a partial
	// unique index that keys on presence, and '' is present as far as Postgres is
	// concerned — so a plain string here would make every second cart collide on
	// '' the moment it was created.
	HandoffCodeHash    *string    `json:"-" bson:"-" gorm:"column:handoff_code_hash"`
	HandoffRefHash     *string    `json:"-" bson:"-" gorm:"column:handoff_ref_hash"`
	HandoffExpiresAt   *time.Time `json:"handoff_expires_at,omitempty" bson:"handoff_expires_at" gorm:"column:handoff_expires_at"`
	HandoffAttempts    int        `json:"-" bson:"-" gorm:"column:handoff_attempts"`
	HandoffLockedUntil *time.Time `json:"-" bson:"-" gorm:"column:handoff_locked_until"`

	ClaimedAt *time.Time `json:"claimed_at,omitempty" bson:"claimed_at" gorm:"column:claimed_at"`
}

func (HandoffSession) TableName() string { return "handoff_cart_sessions" }

// ---- request DTOs -------------------------------------------------------------
//
// Named with a SaleCart prefix to stay distinct from the storefront basket DTOs
// (AddCartItemInput / UpdateCartItemInput in cart.go), which are a different
// surface entirely.

// StartHandoffSessionInput starts a handoff session. BranchID comes from the
// entry QR.
type StartHandoffSessionInput struct {
	BranchID string `json:"branch_id"`
}

// OpenPOSCartInput opens a till cart. The branch is the seller's own; it is
// taken from the authenticated staff user, so only an explicit override is
// accepted here.
type OpenPOSCartInput struct {
	BranchID string `json:"branch_id"`
}

// AddSaleCartItemInput adds a scanned line. The product is resolved through the
// existing catalog and priced from product_stock; the client never supplies a
// price. Quantity is fractional to support weighed goods.
type AddSaleCartItemInput struct {
	ProductID string  `json:"product_id"`
	Quantity  float64 `json:"quantity"`
}

// UpdateSaleCartItemInput sets a line's quantity (<= 0 removes it).
type UpdateSaleCartItemInput struct {
	Quantity float64 `json:"quantity"`
}

// ClaimHandoffInput is the seller's typed/scanned handoff token. Ref carries the
// higher-entropy QR payload; Code is the typed 8-digit fallback.
type ClaimHandoffInput struct {
	Code     string `json:"code"`
	Ref      string `json:"ref"`
	BranchID string `json:"branch_id"`
}

// CheckoutCartInput charges a cart of either kind. JournalID is the seller's
// OPEN shift journal the sale is recorded on; PaymentMethod is validated by the
// sale flow. The Idempotency-Key is taken from the request header, not the body.
type CheckoutCartInput struct {
	JournalID     string        `json:"journal_id"`
	PaymentMethod PaymentMethod `json:"payment_method"`
}

// ---- response DTOs (plaintext tokens returned once, never persisted) ----------

// StartHandoffSessionResponse returns the freshly minted session token (shown in
// the entry-QR link) alongside the new cart.
type StartHandoffSessionResponse struct {
	SessionToken string   `json:"session_token"`
	Cart         SaleCart `json:"cart"`
}

// RequestHandoffResponse returns the handoff token in both forms: the QR payload
// (higher entropy) and the typed 8-digit code (fallback), plus its expiry.
type RequestHandoffResponse struct {
	Code      string    `json:"code"`
	QRRef     string    `json:"qr_ref"`
	ExpiresAt time.Time `json:"expires_at"`
	Cart      SaleCart  `json:"cart"`
}
