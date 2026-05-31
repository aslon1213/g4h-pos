package models

import "time"

// CartItem is a single line in a customer's cart.
type CartItem struct {
	ID        string  `json:"id" bson:"id"`
	ProductID string  `json:"product_id" bson:"product_id"`
	Name      string  `json:"name" bson:"name"`
	Image     string  `json:"image" bson:"image"`
	Quantity  int     `json:"quantity" bson:"quantity"`
	UnitPrice float64 `json:"unit_price" bson:"unit_price"`
	LineTotal float64 `json:"line_total" bson:"line_total"`
}

// Cart is the customer's active shopping cart (one per customer). Stored in the
// `carts` collection, keyed uniquely by customer_id.
type Cart struct {
	ID         string     `json:"id" bson:"_id"`
	CustomerID string     `json:"customer_id" bson:"customer_id"`
	Items      []CartItem `json:"items" bson:"items"`
	CouponCode string     `json:"coupon_code" bson:"coupon_code"`
	Subtotal   float64    `json:"subtotal" bson:"subtotal"`
	Discount   float64    `json:"discount" bson:"discount"`
	Total      float64    `json:"total" bson:"total"`
	CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at" bson:"updated_at"`
}

// AddCartItemInput is the body for POST /api/v1/store/cart/items.
type AddCartItemInput struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// UpdateCartItemInput is the body for PUT /api/v1/store/cart/items/{item_id}.
type UpdateCartItemInput struct {
	Quantity int `json:"quantity"`
}

// ApplyPromoInput is the body for POST /api/v1/store/cart/promo.
type ApplyPromoInput struct {
	Code string `json:"code"`
}
