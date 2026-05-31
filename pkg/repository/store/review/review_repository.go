// Package review is the repository for product reviews (the `reviews`
// collection). It backs both the customer write surface and the public review
// listing on the products controller.
package review

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ReviewRepository owns the reviews collection and keeps the product document's
// denormalized review aggregates (rating, review_count) in sync via products.
type ReviewRepository struct {
	coll     *mongo.Collection
	products *mongo.Collection
}

// New builds the repository and ensures one-review-per-customer-per-product.
func New(db *mongo.Database) *ReviewRepository {
	coll := db.Collection("reviews")
	_, _ = coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "product_id", Value: 1}, {Key: "customer_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{Keys: bson.D{{Key: "product_id", Value: 1}, {Key: "created_at", Value: -1}}},
	})
	return &ReviewRepository{coll: coll, products: db.Collection("products")}
}

// recalcProduct recomputes a product's denormalized rating + review_count from
// the reviews collection and writes them onto the product document. Best-effort:
// an aggregation/update error is returned for the caller to log but should not
// fail the review mutation itself.
func (r *ReviewRepository) recalcProduct(ctx context.Context, productID string) error {
	pipeline := mongo.Pipeline{
		bson.D{{Key: "$match", Value: bson.D{{Key: "product_id", Value: productID}}}},
		bson.D{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: nil},
			{Key: "avg", Value: bson.D{{Key: "$avg", Value: "$rating"}}},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
		}}},
	}
	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return err
	}
	var rows []struct {
		Avg   float64 `bson:"avg"`
		Count int     `bson:"count"`
	}
	if err := cursor.All(ctx, &rows); err != nil {
		return err
	}
	rating, count := 0.0, 0
	if len(rows) > 0 {
		rating, count = rows[0].Avg, rows[0].Count
	}
	_, err = r.products.UpdateOne(ctx,
		bson.M{"_id": productID},
		bson.M{"$set": bson.M{"rating": rating, "review_count": count}},
	)
	return err
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
	if _, err := r.coll.InsertOne(ctx, review); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	_ = r.recalcProduct(ctx, review.ProductID)
	return review, nil
}

// Update edits the caller's own review (rating/title/body).
func (r *ReviewRepository) Update(ctx context.Context, customerID, reviewID string, in models.UpdateReviewInput) (*models.Review, error) {
	review := &models.Review{}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": reviewID, "customer_id": customerID},
		bson.M{"$set": bson.M{
			"rating":     in.Rating,
			"title":      in.Title,
			"body":       in.Body,
			"updated_at": time.Now(),
		}},
		opts,
	).Decode(review)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	_ = r.recalcProduct(ctx, review.ProductID)
	return review, nil
}

// Delete removes the caller's own review.
func (r *ReviewRepository) Delete(ctx context.Context, customerID, reviewID string) error {
	// Read the review first so we know which product to recompute after deletion.
	deleted := &models.Review{}
	err := r.coll.FindOneAndDelete(ctx, bson.M{"_id": reviewID, "customer_id": customerID}).Decode(deleted)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return repoerr.ErrNotFound
	}
	if err != nil {
		return err
	}
	_ = r.recalcProduct(ctx, deleted.ProductID)
	return nil
}

// Vote records a helpful vote from a customer (idempotent per customer). The
// helpful_votes counter only changes the first time a customer votes.
func (r *ReviewRepository) Vote(ctx context.Context, customerID, reviewID string, helpful bool) (*models.Review, error) {
	delta := 1
	if !helpful {
		delta = -1
	}
	review := &models.Review{}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": reviewID, "voters": bson.M{"$ne": customerID}},
		bson.M{
			"$inc":      bson.M{"helpful_votes": delta},
			"$addToSet": bson.M{"voters": customerID},
			"$set":      bson.M{"updated_at": time.Now()},
		},
		opts,
	).Decode(review)
	if errors.Is(err, mongo.ErrNoDocuments) {
		// Either the review is missing or the customer already voted.
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return review, nil
}

// ListByProduct returns a product's reviews, newest first, paginated.
func (r *ReviewRepository) ListByProduct(ctx context.Context, productID string, page, count int) (*models.PagedReviews, error) {
	if page < 1 {
		page = 1
	}
	if count < 1 {
		count = 20
	}
	filter := bson.M{"product_id": productID}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * count)).
		SetLimit(int64(count))
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	reviews := []models.Review{}
	if err := cursor.All(ctx, &reviews); err != nil {
		return nil, err
	}
	return &models.PagedReviews{Reviews: reviews, Total: total, Page: page, Count: count}, nil
}
