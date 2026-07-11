// Package review is the repository for product reviews (the `reviews` table plus
// the `review_votes` helpful-vote dedupe table). It backs both the customer
// write surface and the public review listing on the products controller, and
// keeps the product's denormalized review aggregates (rating, review_count) in
// sync.
package review

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// ReviewRepository owns the reviews/review_votes tables and maintains the
// products table's denormalized rating + review_count.
type ReviewRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *ReviewRepository {
	return &ReviewRepository{db: db}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint (23505)
// violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// recalcProduct recomputes a product's denormalized rating + review_count from
// the reviews table and writes them onto the product row. Best-effort: an error
// is returned for the caller to log but should not fail the review mutation.
func (r *ReviewRepository) recalcProduct(ctx context.Context, productID string) error {
	var agg struct {
		Avg float64
		Cnt int64
	}
	err := r.db.WithContext(ctx).Model(&models.Review{}).
		Select("COALESCE(AVG(rating), 0) AS avg, COUNT(*) AS cnt").
		Where("product_id = ?", productID).
		Scan(&agg).Error
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Model(&models.Product{}).Where("id = ?", productID).
		Updates(map[string]interface{}{"rating": agg.Avg, "review_count": agg.Cnt}).Error
}

// Create inserts a new review. Returns repoerr.ErrConflict if this customer has
// already reviewed the product.
func (r *ReviewRepository) Create(ctx context.Context, review *models.Review) (*models.Review, error) {
	now := time.Now()
	review.ID = uuid.New().String()
	review.HelpfulVotes = 0
	review.Voters = []string{}
	review.CreatedAt = now
	review.UpdatedAt = now
	if err := gorm.G[models.Review](r.db).Create(ctx, review); err != nil {
		if isUniqueViolation(err) {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	_ = r.recalcProduct(ctx, review.ProductID)
	return review, nil
}

// Update edits the caller's own review (rating/title/body).
func (r *ReviewRepository) Update(ctx context.Context, customerID, reviewID string, in models.UpdateReviewInput) (*models.Review, error) {
	res := r.db.WithContext(ctx).Model(&models.Review{}).
		Where("id = ? AND customer_id = ?", reviewID, customerID).
		Updates(map[string]interface{}{
			"rating":     in.Rating,
			"title":      in.Title,
			"body":       in.Body,
			"updated_at": time.Now(),
		})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, repoerr.ErrNotFound
	}
	review, err := gorm.G[models.Review](r.db).Where("id = ?", reviewID).First(ctx)
	if err != nil {
		return nil, err
	}
	_ = r.recalcProduct(ctx, review.ProductID)
	return &review, nil
}

// Delete removes the caller's own review.
func (r *ReviewRepository) Delete(ctx context.Context, customerID, reviewID string) error {
	// Read the review first so we know which product to recompute after deletion.
	deleted, err := gorm.G[models.Review](r.db).Where("id = ? AND customer_id = ?", reviewID, customerID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return repoerr.ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := gorm.G[models.Review](r.db).Where("id = ? AND customer_id = ?", reviewID, customerID).Delete(ctx); err != nil {
		return err
	}
	_ = r.recalcProduct(ctx, deleted.ProductID)
	return nil
}

// Vote records a helpful vote from a customer (idempotent per customer). The
// helpful_votes counter only changes the first time a customer votes; a repeat
// vote (or a missing review) yields repoerr.ErrNotFound, mirroring the original.
func (r *ReviewRepository) Vote(ctx context.Context, customerID, reviewID string, helpful bool) (*models.Review, error) {
	delta := 1
	if !helpful {
		delta = -1
	}

	// The review must exist first.
	if _, err := gorm.G[models.Review](r.db).Where("id = ?", reviewID).First(ctx); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, repoerr.ErrNotFound
		}
		return nil, err
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		vote := models.ReviewVote{ReviewID: reviewID, CustomerID: customerID}
		if err := gorm.G[models.ReviewVote](tx).Create(ctx, &vote); err != nil {
			if isUniqueViolation(err) {
				// The customer already voted — treat as not-found, as the Mongo
				// FindOneAndUpdate filter did.
				return repoerr.ErrNotFound
			}
			return err
		}
		res := tx.WithContext(ctx).Model(&models.Review{}).Where("id = ?", reviewID).Updates(map[string]interface{}{
			"helpful_votes": gorm.Expr("helpful_votes + ?", delta),
			"updated_at":    time.Now(),
		})
		return res.Error
	})
	if err != nil {
		return nil, err
	}

	review, err := gorm.G[models.Review](r.db).Where("id = ?", reviewID).First(ctx)
	if err != nil {
		return nil, err
	}
	return &review, nil
}

// ListByProduct returns a product's reviews, newest first, paginated.
func (r *ReviewRepository) ListByProduct(ctx context.Context, productID string, page, count int) (*models.PagedReviews, error) {
	if page < 1 {
		page = 1
	}
	if count < 1 {
		count = 20
	}
	q := gorm.G[models.Review](r.db).Where("product_id = ?", productID)
	total, err := q.Count(ctx, "*")
	if err != nil {
		return nil, err
	}
	reviews, err := q.Order("created_at DESC").Offset((page - 1) * count).Limit(count).Find(ctx)
	if err != nil {
		return nil, err
	}
	return &models.PagedReviews{Reviews: reviews, Total: total, Page: page, Count: count}, nil
}
