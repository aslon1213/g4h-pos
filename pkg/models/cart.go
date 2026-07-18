package models

import "time"

// CartItem is a single line in a customer's cart.
type CartItem struct {
	ID        string  `json:"id" bson:"id" gorm:"column:id;primaryKey"`
	CartID    string  `json:"-" bson:"-" gorm:"column:cart_id"`
	ProductID string  `json:"product_id" bson:"product_id" gorm:"column:product_id"`
	Name      string  `json:"name" bson:"name" gorm:"column:name"`
	Image     string  `json:"image" bson:"image" gorm:"column:image"`
	Quantity  int     `json:"quantity" bson:"quantity" gorm:"column:quantity"`
	UnitPrice float64 `json:"unit_price" bson:"unit_price" gorm:"column:unit_price"`
	LineTotal float64 `json:"line_total" bson:"line_total" gorm:"column:line_total"`
}

func (CartItem) TableName() string { return "cart_items" }

// Cart is the customer's active shopping cart (one per customer). Stored in the
// `carts` collection, keyed uniquely by customer_id.
type Cart struct {
	ID         string     `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	CustomerID string     `json:"customer_id" bson:"customer_id" gorm:"column:customer_id"`
	Items      []CartItem `json:"items" bson:"items" gorm:"-"`
	CouponCode string     `json:"coupon_code" bson:"coupon_code" gorm:"column:coupon_code"`
	Subtotal   float64    `json:"subtotal" bson:"subtotal" gorm:"column:subtotal"`
	Discount   float64    `json:"discount" bson:"discount" gorm:"column:discount"`
	Total      float64    `json:"total" bson:"total" gorm:"column:total"`
	CreatedAt  time.Time  `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt  time.Time  `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (Cart) TableName() string { return "carts" }

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
