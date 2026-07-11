package models

import "time"

// WishlistItem is a saved product in a customer's wishlist.
type WishlistItem struct {
	ID         uint      `json:"-" bson:"-" gorm:"column:id;primaryKey"`
	WishlistID string    `json:"-" bson:"-" gorm:"column:wishlist_id"`
	ProductID  string    `json:"product_id" bson:"product_id" gorm:"column:product_id"`
	Name       string    `json:"name" bson:"name" gorm:"column:name"`
	Image      string    `json:"image" bson:"image" gorm:"column:image"`
	Price      float64   `json:"price" bson:"price" gorm:"column:price"`
	AddedAt    time.Time `json:"added_at" bson:"added_at" gorm:"column:added_at"`
}

func (WishlistItem) TableName() string { return "wishlist_items" }

// Wishlist is the customer's saved-for-later list (one per customer). Stored in
// the `wishlists` collection, keyed uniquely by customer_id.
type Wishlist struct {
	ID         string         `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	CustomerID string         `json:"customer_id" bson:"customer_id" gorm:"column:customer_id"`
	Items      []WishlistItem `json:"items" bson:"items" gorm:"-"`
	CreatedAt  time.Time      `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt  time.Time      `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (Wishlist) TableName() string { return "wishlists" }

// AddWishlistItemInput is the body for POST /api/v1/store/wishlist/items.
type AddWishlistItemInput struct {
	ProductID string `json:"product_id"`
}
