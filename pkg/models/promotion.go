package models

import "time"

// DiscountType describes how a coupon/promotion reduces the cart total.
type DiscountType string

const (
	DiscountTypePercentage DiscountType = "percentage" // Value is a percent (0..100)
	DiscountTypeFixed      DiscountType = "fixed"      // Value is an absolute amount
)

// Promotion is a storefront campaign/banner, optionally backed by a coupon
// code. Stored in the `promotions` collection.
type Promotion struct {
	ID           string       `json:"id" bson:"_id"`
	Title        string       `json:"title" bson:"title"`
	Description  string       `json:"description" bson:"description"`
	Banner       string       `json:"banner" bson:"banner"`
	Code         string       `json:"code" bson:"code"` // coupon code, empty for banner-only
	DiscountType DiscountType `json:"discount_type" bson:"discount_type"`
	Value        float64      `json:"value" bson:"value"`
	MinSubtotal  float64      `json:"min_subtotal" bson:"min_subtotal"`
	UsageLimit   int          `json:"usage_limit" bson:"usage_limit"`
	UsedCount    int          `json:"used_count" bson:"used_count"`
	IsActive     bool         `json:"is_active" bson:"is_active"`
	StartsAt     time.Time    `json:"starts_at" bson:"starts_at"`
	EndsAt       time.Time    `json:"ends_at" bson:"ends_at"`
	CreatedAt    time.Time    `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time    `json:"updated_at" bson:"updated_at"`
}

// CouponValidation is the result of validating a coupon against a cart.
type CouponValidation struct {
	Code     string  `json:"code" bson:"code"`
	Valid    bool    `json:"valid" bson:"valid"`
	Reason   string  `json:"reason" bson:"reason"` // why invalid (empty when valid)
	Discount float64 `json:"discount" bson:"discount"`
}

// ValidateCouponInput is the body for POST /api/v1/store/promotions/validate.
type ValidateCouponInput struct {
	Code string `json:"code"`
}
