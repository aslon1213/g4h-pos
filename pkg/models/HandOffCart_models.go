package models

import (
	"slices"
	"time"
)

// HandoffState is the lifecycle of a Scan & Go handoff cart. Transitions are
// enforced by handoffTransitions / CanTransitionTo; illegal moves are rejected
// by the repository (and backed by a CHECK constraint on the state column).
type HandoffState string

const (
	// HandoffActive: customer is building the cart; lines may be added/edited/removed.
	HandoffActive HandoffState = "active"
	// HandoffReadyForHandoff: customer requested checkout; a handoff token is minted;
	// the cart is still customer-editable.
	HandoffReadyForHandoff HandoffState = "ready_for_handoff"
	// HandoffClaimed: a seller claimed the cart; edit rights transfer to the seller
	// and customer writes are rejected (409).
	HandoffClaimed HandoffState = "claimed"
	// HandoffCompleted: seller charged it; a sale was recorded. Terminal.
	HandoffCompleted HandoffState = "completed"
	// HandoffExpired: a stale non-terminal cart was reaped. Terminal.
	HandoffExpired HandoffState = "expired"
	// HandoffCancelled: explicitly cancelled/released to a dead end. Terminal.
	HandoffCancelled HandoffState = "cancelled"
)

// handoffTransitions is the whitelist of legal state moves. Line edits are NOT
// transitions — they are gated separately (allowed only while active or
// ready_for_handoff); see HandoffState.AllowsCustomerEdit.
var handoffTransitions = map[HandoffState][]HandoffState{
	HandoffActive:          {HandoffReadyForHandoff, HandoffExpired, HandoffCancelled},
	HandoffReadyForHandoff: {HandoffClaimed, HandoffActive, HandoffExpired, HandoffCancelled},
	HandoffClaimed:         {HandoffCompleted, HandoffReadyForHandoff, HandoffCancelled},
	HandoffCompleted:       {},
	HandoffExpired:         {},
	HandoffCancelled:       {},
}

// CanTransitionTo reports whether moving from s to next is a legal transition.
func (s HandoffState) CanTransitionTo(next HandoffState) bool {
	return slices.Contains(handoffTransitions[s], next)
}

// IsTerminal reports whether s is an end state with no outgoing transitions.
func (s HandoffState) IsTerminal() bool {
	return len(handoffTransitions[s]) == 0
}

// AllowsCustomerEdit reports whether the customer may still add/edit/remove
// lines in state s. Editing is frozen the moment a seller claims the cart.
func (s HandoffState) AllowsCustomerEdit() bool {
	return s == HandoffActive || s == HandoffReadyForHandoff
}

// HandoffCartLine is a single scanned line in a handoff cart. display_price is a
// customer-facing snapshot only; the authoritative price is applied by the sale
// flow at checkout. POS money is integer (matches product_stock.price).
type HandoffCartLine struct {
	ID           string `json:"id" bson:"id" gorm:"column:id;primaryKey"`
	CartID       string `json:"-" bson:"-" gorm:"column:cart_id"`
	ProductID    string `json:"product_id" bson:"product_id" gorm:"column:product_id"`
	SKU          string `json:"sku" bson:"sku" gorm:"column:sku"`
	Name         string `json:"name" bson:"name" gorm:"column:name"`
	UOM          string `json:"uom" bson:"uom" gorm:"column:uom"`
	Quantity     int    `json:"quantity" bson:"quantity" gorm:"column:quantity"`
	DisplayPrice int    `json:"display_price" bson:"display_price" gorm:"column:display_price"`
	LineTotal    int    `json:"line_total" bson:"line_total" gorm:"column:line_total"`
}

func (HandoffCartLine) TableName() string { return "handoff_cart_lines" }

// HandoffCart is a server-side "Scan & Go" cart scoped to one branch. The phone
// holds no state; the session token (entry-QR bearer) is the customer's
// credential. Token digests are never exposed (json:"-"); the plaintext tokens
// are returned exactly once at mint time via the response DTOs below.
type HandoffCart struct {
	ID       string       `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	BranchID string       `json:"branch_id" bson:"branch_id" gorm:"column:branch_id"`
	State    HandoffState `json:"state" bson:"state" gorm:"column:state"`

	// session token (entry-QR bearer) — hash only.
	SessionTokenHash string    `json:"-" bson:"-" gorm:"column:session_token_hash"`
	SessionExpiresAt time.Time `json:"session_expires_at" bson:"session_expires_at" gorm:"column:session_expires_at"`

	// handoff token (minted at checkout) — hashes only.
	HandoffCodeHash    string     `json:"-" bson:"-" gorm:"column:handoff_code_hash"`
	HandoffRefHash     string     `json:"-" bson:"-" gorm:"column:handoff_ref_hash"`
	HandoffExpiresAt   *time.Time `json:"handoff_expires_at,omitempty" bson:"handoff_expires_at" gorm:"column:handoff_expires_at"`
	HandoffAttempts    int        `json:"-" bson:"-" gorm:"column:handoff_attempts"`
	HandoffLockedUntil *time.Time `json:"-" bson:"-" gorm:"column:handoff_locked_until"`

	// claim / checkout bookkeeping.
	ClaimedBy              *string    `json:"claimed_by,omitempty" bson:"claimed_by" gorm:"column:claimed_by"`
	ClaimedAt              *time.Time `json:"claimed_at,omitempty" bson:"claimed_at" gorm:"column:claimed_at"`
	CheckoutIdempotencyKey *string    `json:"-" bson:"-" gorm:"column:checkout_idempotency_key"`
	SaleJournalID          *string    `json:"sale_journal_id,omitempty" bson:"sale_journal_id" gorm:"column:sale_journal_id"`

	FinalizedAt *time.Time `json:"finalized_at,omitempty" bson:"finalized_at" gorm:"column:finalized_at"`
	CreatedAt   time.Time  `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt   time.Time  `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`

	Lines []HandoffCartLine `json:"lines" bson:"lines" gorm:"-"`
}

func (HandoffCart) TableName() string { return "handoff_carts" }

// ---- request DTOs -------------------------------------------------------------

// StartHandoffSessionInput starts a session. BranchID comes from the entry QR.
type StartHandoffSessionInput struct {
	BranchID string `json:"branch_id"`
}

// AddHandoffLineInput adds a scanned line. The product is resolved through the
// existing catalog; the phone never supplies a price.
type AddHandoffLineInput struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// UpdateHandoffLineInput sets a line's quantity (<= 0 removes it).
type UpdateHandoffLineInput struct {
	Quantity int `json:"quantity"`
}

// ClaimHandoffInput is the seller's typed/scanned handoff token. Ref carries the
// higher-entropy QR payload; Code is the typed 8-digit fallback.
type ClaimHandoffInput struct {
	Code     string `json:"code"`
	Ref      string `json:"ref"`
	BranchID string `json:"branch_id"`
}

// CheckoutHandoffInput charges a claimed cart. JournalID is the seller's OPEN
// shift journal the sale is recorded on; PaymentMethod is validated by the sale
// flow. The Idempotency-Key is taken from the request header, not the body.
type CheckoutHandoffInput struct {
	JournalID     string        `json:"journal_id"`
	PaymentMethod PaymentMethod `json:"payment_method"`
}

// ---- response DTOs (plaintext tokens returned once, never persisted) ----------

// StartHandoffSessionResponse returns the freshly minted session token (shown in
// the entry-QR link) alongside the new cart.
type StartHandoffSessionResponse struct {
	SessionToken string      `json:"session_token"`
	Cart         HandoffCart `json:"cart"`
}

// RequestHandoffResponse returns the handoff token in both forms: the QR payload
// (higher entropy) and the typed 8-digit code (fallback), plus its expiry.
type RequestHandoffResponse struct {
	Code      string      `json:"code"`
	QRRef     string      `json:"qr_ref"`
	ExpiresAt time.Time   `json:"expires_at"`
	Cart      HandoffCart `json:"cart"`
}
