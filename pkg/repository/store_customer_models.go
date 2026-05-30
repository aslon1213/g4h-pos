package models

import "time"

// StoreCustomer is a storefront account (separate from the staff-managed POS
// `customers` collection). Stored in the `store_customers` collection.
//
// Phase 1: only the fields required by CustomerAuthMiddleware and the stubbed
// handlers are defined. The full account model (password hash, verification,
// addresses, etc.) lands in Phase 2.
type StoreCustomer struct {
	ID        string    `json:"id" bson:"_id"`
	Email     string    `json:"email" bson:"email"`
	Phone     string    `json:"phone" bson:"phone"`
	CreatedAt time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt time.Time `json:"updated_at" bson:"updated_at"`
}
