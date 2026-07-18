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
	ID        uint    `json:"-" bson:"-" gorm:"column:id;primaryKey"`
	OrderID   string  `json:"-" bson:"-" gorm:"column:order_id"`
	ProductID string  `json:"product_id" bson:"product_id" gorm:"column:product_id"`
	Name      string  `json:"name" bson:"name" gorm:"column:name"`
	Image     string  `json:"image" bson:"image" gorm:"column:image"`
	Quantity  int     `json:"quantity" bson:"quantity" gorm:"column:quantity"`
	UnitPrice float64 `json:"unit_price" bson:"unit_price" gorm:"column:unit_price"`
	LineTotal float64 `json:"line_total" bson:"line_total" gorm:"column:line_total"`
}

func (OrderItem) TableName() string { return "order_items" }

// OrderTotals is the money breakdown for an order or checkout preview.
type OrderTotals struct {
	Subtotal float64 `json:"subtotal" bson:"subtotal" gorm:"column:subtotal"`
	Discount float64 `json:"discount" bson:"discount" gorm:"column:discount"`
	Shipping float64 `json:"shipping" bson:"shipping" gorm:"column:shipping"`
	Tax      float64 `json:"tax" bson:"tax" gorm:"column:tax"`
	Total    float64 `json:"total" bson:"total" gorm:"column:total"`
}

// Order is a placed storefront order. Stored in the `orders` collection.
type Order struct {
	ID         string      `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	Number     string      `json:"number" bson:"number" gorm:"column:number"` // human-friendly order number
	CustomerID string      `json:"customer_id" bson:"customer_id" gorm:"column:customer_id"`
	Items      []OrderItem `json:"items" bson:"items" gorm:"-"`
	Totals     OrderTotals `json:"totals" bson:"totals" gorm:"embedded"`
	Status     OrderStatus `json:"status" bson:"status" gorm:"column:status"`
	CouponCode string      `json:"coupon_code" bson:"coupon_code" gorm:"column:coupon_code"`
	Address    Address     `json:"address" bson:"address" gorm:"embedded;embeddedPrefix:address_"`
	Note       string      `json:"note" bson:"note" gorm:"column:note"`
	CreatedAt  time.Time   `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt  time.Time   `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (Order) TableName() string { return "orders" }

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
