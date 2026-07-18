// Package promotion is the repository for storefront promotions and coupons
// (the `promotions` table).
package promotion

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"gorm.io/gorm"
)

// PromotionRepository owns the promotions table.
type PromotionRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *PromotionRepository {
	return &PromotionRepository{db: db}
}

// List returns the currently-active promotions (active flag + within window).
func (r *PromotionRepository) List(ctx context.Context) ([]models.Promotion, error) {
	now := time.Now()
	return gorm.G[models.Promotion](r.db).
		Where("is_active = ?", true).
		Where("(starts_at IS NULL OR starts_at <= ?)", now).
		Where("(ends_at IS NULL OR ends_at >= ?)", now).
		Order("created_at DESC").
		Find(ctx)
}

// GetByID returns a single promotion, or repoerr.ErrNotFound.
func (r *PromotionRepository) GetByID(ctx context.Context, id string) (*models.Promotion, error) {
	return r.findOne(ctx, "id = ?", id)
}

// GetByCode returns the promotion backing a coupon code, or repoerr.ErrNotFound.
func (r *PromotionRepository) GetByCode(ctx context.Context, code string) (*models.Promotion, error) {
	return r.findOne(ctx, "code = ?", code)
}

func (r *PromotionRepository) findOne(ctx context.Context, query string, args ...interface{}) (*models.Promotion, error) {
	promo, err := gorm.G[models.Promotion](r.db).Where(query, args...).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &promo, nil
}

// ValidateCoupon checks a coupon code against a cart subtotal and returns the
// computed discount. A non-existent code yields a non-error invalid result so
// the handler can report "invalid coupon" without a 404.
func (r *PromotionRepository) ValidateCoupon(ctx context.Context, code string, subtotal float64) (*models.CouponValidation, error) {
	result := &models.CouponValidation{Code: code}
	promo, err := r.GetByCode(ctx, code)
	if errors.Is(err, repoerr.ErrNotFound) {
		result.Reason = "coupon not found"
		return result, nil
	}
	if err != nil {
		return nil, err
	}

	now := time.Now()
	switch {
	case !promo.IsActive:
		result.Reason = "coupon is not active"
	case !promo.StartsAt.IsZero() && now.Before(promo.StartsAt):
		result.Reason = "coupon is not yet valid"
	case !promo.EndsAt.IsZero() && now.After(promo.EndsAt):
		result.Reason = "coupon has expired"
	case promo.UsageLimit > 0 && promo.UsedCount >= promo.UsageLimit:
		result.Reason = "coupon usage limit reached"
	case subtotal < promo.MinSubtotal:
		result.Reason = "cart subtotal is below the coupon minimum"
	default:
		result.Valid = true
		result.Discount = discountFor(promo, subtotal)
	}
	return result, nil
}

// discountFor computes the discount a promotion applies to a subtotal, capped at
// the subtotal so a fixed coupon never makes the total negative.
func discountFor(promo *models.Promotion, subtotal float64) float64 {
	var discount float64
	switch promo.DiscountType {
	case models.DiscountTypePercentage:
		discount = subtotal * promo.Value / 100
	case models.DiscountTypeFixed:
		discount = promo.Value
	}
	if discount > subtotal {
		discount = subtotal
	}
	return discount
}
