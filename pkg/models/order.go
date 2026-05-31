package models

import "time"

// OrderStatus enumerates the lifecycle states of a storefront order.
type OrderStatus string

const (
	OrderStatusPending   OrderStatus = "pending"   // created, awaiting payment/confirmation
	OrderStatusConfirmed OrderStatus = "confirmed" // accepted, being prepared
	OrderStatusShipped   OrderStatus = "shipped"   // handed to delivery
	OrderStatusDelivered OrderStatus = "delivered" // received by customer
	OrderStatusCancelled OrderStatus = "cancelled" // cancelled by customer/staff
)

// OrderItem is a purchased line, captured at order time (price is frozen).
type OrderItem struct {
	ProductID string  `json:"product_id" bson:"product_id"`
	Name      string  `json:"name" bson:"name"`
	Image     string  `json:"image" bson:"image"`
	Quantity  int     `json:"quantity" bson:"quantity"`
	UnitPrice float64 `json:"unit_price" bson:"unit_price"`
	LineTotal float64 `json:"line_total" bson:"line_total"`
}

// OrderTotals is the money breakdown for an order or checkout preview.
type OrderTotals struct {
	Subtotal float64 `json:"subtotal" bson:"subtotal"`
	Discount float64 `json:"discount" bson:"discount"`
	Shipping float64 `json:"shipping" bson:"shipping"`
	Tax      float64 `json:"tax" bson:"tax"`
	Total    float64 `json:"total" bson:"total"`
}

// Order is a placed storefront order. Stored in the `orders` collection.
type Order struct {
	ID         string      `json:"id" bson:"_id"`
	Number     string      `json:"number" bson:"number"` // human-friendly order number
	CustomerID string      `json:"customer_id" bson:"customer_id"`
	Items      []OrderItem `json:"items" bson:"items"`
	Totals     OrderTotals `json:"totals" bson:"totals"`
	Status     OrderStatus `json:"status" bson:"status"`
	CouponCode string      `json:"coupon_code" bson:"coupon_code"`
	Address    Address     `json:"address" bson:"address"`
	Note       string      `json:"note" bson:"note"`
	CreatedAt  time.Time   `json:"created_at" bson:"created_at"`
	UpdatedAt  time.Time   `json:"updated_at" bson:"updated_at"`
}

// CheckoutPreview is the computed (un-persisted) summary returned before an
// order is placed.
type CheckoutPreview struct {
	Items  []OrderItem `json:"items" bson:"items"`
	Totals OrderTotals `json:"totals" bson:"totals"`
}

// CheckoutPreviewInput is the body for POST /api/v1/store/checkout/preview.
type CheckoutPreviewInput struct {
	AddressID  string `json:"address_id"`
	CouponCode string `json:"coupon_code"`
}

// PlaceOrderInput is the body for POST /api/v1/store/orders.
type PlaceOrderInput struct {
	AddressID  string `json:"address_id"`
	CouponCode string `json:"coupon_code"`
	Note       string `json:"note"`
}
