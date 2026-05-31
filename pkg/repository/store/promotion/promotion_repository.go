// Package promotion is the repository for storefront promotions and coupons
// (the `promotions` collection).
package promotion

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// PromotionRepository owns the promotions collection.
type PromotionRepository struct {
	coll *mongo.Collection
}

// New builds the repository and ensures the coupon-code index exists.
func New(db *mongo.Database) *PromotionRepository {
	coll := db.Collection("promotions")
	_, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "code", Value: 1}},
	})
	return &PromotionRepository{coll: coll}
}

// List returns the currently-active promotions (active flag + within window).
func (r *PromotionRepository) List(ctx context.Context) ([]models.Promotion, error) {
	now := time.Now()
	filter := bson.M{
		"is_active": true,
		"$and": []bson.M{
			{"$or": []bson.M{{"starts_at": bson.M{"$lte": now}}, {"starts_at": bson.M{"$exists": false}}}},
			{"$or": []bson.M{{"ends_at": bson.M{"$gte": now}}, {"ends_at": bson.M{"$exists": false}}}},
		},
	}
	cursor, err := r.coll.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}))
	if err != nil {
		return nil, err
	}
	promotions := []models.Promotion{}
	if err := cursor.All(ctx, &promotions); err != nil {
		return nil, err
	}
	return promotions, nil
}

// GetByID returns a single promotion, or repoerr.ErrNotFound.
func (r *PromotionRepository) GetByID(ctx context.Context, id string) (*models.Promotion, error) {
	return r.findOne(ctx, bson.M{"_id": id})
}

// GetByCode returns the promotion backing a coupon code, or repoerr.ErrNotFound.
func (r *PromotionRepository) GetByCode(ctx context.Context, code string) (*models.Promotion, error) {
	return r.findOne(ctx, bson.M{"code": code})
}

func (r *PromotionRepository) findOne(ctx context.Context, filter bson.M) (*models.Promotion, error) {
	promo := &models.Promotion{}
	err := r.coll.FindOne(ctx, filter).Decode(promo)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return promo, nil
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
