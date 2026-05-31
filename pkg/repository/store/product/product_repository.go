// Package product is the read-only repository backing the storefront product
// browse surface. It reads the existing `products` collection (written by the
// staff side) and projects each document onto models.StoreProduct, plus a few
// helpers for images, related items, availability and review summaries.
package product

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// ProductRepository owns reads over the products collection (and the reviews
// collection for product review summaries).
type ProductRepository struct {
	products *mongo.Collection
	reviews  *mongo.Collection
}

// New builds the repository and ensures browse indexes exist.
func New(db *mongo.Database) *ProductRepository {
	products := db.Collection("products")
	_, _ = products.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "category", Value: 1}}},
		{Keys: bson.D{{Key: "sku", Value: 1}}},
	})
	return &ProductRepository{products: products, reviews: db.Collection("reviews")}
}

// buildFilter translates list/search params into a mongo filter.
func buildFilter(p models.ProductListParams) bson.M {
	filter := bson.M{}
	if p.Query != "" {
		filter["name"] = bson.M{"$regex": p.Query, "$options": "i"}
	}
	if p.Category != "" {
		filter["category"] = p.Category
	}
	if p.Brand != "" {
		filter["manufacturer.name"] = p.Brand
	}
	return filter
}

// sortDoc maps a sort token to a mongo sort document (default: newest first).
func sortDoc(sort string) bson.D {
	switch sort {
	case "newest":
		return bson.D{{Key: "created_at", Value: -1}}
	case "name_asc":
		return bson.D{{Key: "name", Value: 1}}
	case "name_desc":
		return bson.D{{Key: "name", Value: -1}}
	default:
		return bson.D{{Key: "created_at", Value: -1}}
	}
}

// List returns a paginated page of storefront products for the given filters.
func (r *ProductRepository) List(ctx context.Context, p models.ProductListParams) (*models.PagedProducts, error) {
	page, count := normalizePaging(p.Page, p.Count)
	filter := buildFilter(p)

	total, err := r.products.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}

	opts := options.Find().
		SetSort(sortDoc(p.Sort)).
		SetSkip(int64((page - 1) * count)).
		SetLimit(int64(count))
	cursor, err := r.products.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	products := []models.StoreProduct{}
	if err := cursor.All(ctx, &products); err != nil {
		return nil, err
	}
	return &models.PagedProducts{Products: products, Total: total, Page: page, Count: count}, nil
}

// GetByID returns a single storefront product, or repoerr.ErrNotFound.
func (r *ProductRepository) GetByID(ctx context.Context, id string) (*models.StoreProduct, error) {
	product := &models.StoreProduct{}
	err := r.products.FindOne(ctx, bson.M{"_id": id}).Decode(product)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return product, nil
}

// GetImages returns the product's image URLs.
func (r *ProductRepository) GetImages(ctx context.Context, id string) ([]string, error) {
	product, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return product.Images, nil
}

// GetRelated returns up to `limit` products sharing a category with the given
// product (excluding it). Returns repoerr.ErrNotFound if the product is missing.
func (r *ProductRepository) GetRelated(ctx context.Context, id string, limit int) ([]models.StoreProduct, error) {
	product, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 8
	}
	filter := bson.M{"_id": bson.M{"$ne": id}}
	if len(product.Category) > 0 {
		filter["category"] = bson.M{"$in": product.Category}
	}
	cursor, err := r.products.Find(ctx, filter, options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	related := []models.StoreProduct{}
	if err := cursor.All(ctx, &related); err != nil {
		return nil, err
	}
	return related, nil
}

// GetAvailability reports per-branch stock derived from the product's
// quantity_distribution. Returns repoerr.ErrNotFound if the product is missing.
func (r *ProductRepository) GetAvailability(ctx context.Context, id string) ([]models.BranchAvailability, error) {
	var doc struct {
		QuantityDistribution []struct {
			Quantity int `bson:"quantity"`
			Place    struct {
				ID string `bson:"id"`
			} `bson:"place"`
		} `bson:"quantity_distribution"`
	}
	err := r.products.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	availability := make([]models.BranchAvailability, 0, len(doc.QuantityDistribution))
	for _, d := range doc.QuantityDistribution {
		availability = append(availability, models.BranchAvailability{
			BranchID: d.Place.ID,
			Quantity: d.Quantity,
			InStock:  d.Quantity > 0,
		})
	}
	return availability, nil
}

// GetReviews returns a paginated list of a product's reviews (newest first).
func (r *ProductRepository) GetReviews(ctx context.Context, productID string, page, count int) (*models.PagedReviews, error) {
	page, count = normalizePaging(page, count)
	filter := bson.M{"product_id": productID}

	total, err := r.reviews.CountDocuments(ctx, filter)
	if err != nil {
		return nil, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * count)).
		SetLimit(int64(count))
	cursor, err := r.reviews.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	reviews := []models.Review{}
	if err := cursor.All(ctx, &reviews); err != nil {
		return nil, err
	}
	return &models.PagedReviews{Reviews: reviews, Total: total, Page: page, Count: count}, nil
}

// normalizePaging applies sane defaults/bounds to page and count.
func normalizePaging(page, count int) (int, int) {
	if page < 1 {
		page = 1
	}
	if count < 1 {
		count = 20
	}
	if count > 100 {
		count = 100
	}
	return page, count
}
