package models

import "time"

// WishlistItem is a saved product in a customer's wishlist.
type WishlistItem struct {
	ProductID string    `json:"product_id" bson:"product_id"`
	Name      string    `json:"name" bson:"name"`
	Image     string    `json:"image" bson:"image"`
	Price     float64   `json:"price" bson:"price"`
	AddedAt   time.Time `json:"added_at" bson:"added_at"`
}

// Wishlist is the customer's saved-for-later list (one per customer). Stored in
// the `wishlists` collection, keyed uniquely by customer_id.
type Wishlist struct {
	ID         string         `json:"id" bson:"_id"`
	CustomerID string         `json:"customer_id" bson:"customer_id"`
	Items      []WishlistItem `json:"items" bson:"items"`
	CreatedAt  time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at" bson:"updated_at"`
}

// AddWishlistItemInput is the body for POST /api/v1/store/wishlist/items.
type AddWishlistItemInput struct {
	ProductID string `json:"product_id"`
}
