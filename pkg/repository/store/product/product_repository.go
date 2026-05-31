// Package product is the read-only repository backing the storefront product
// browse surface. It reads the existing `products` collection (written by the
// staff side) and returns the full models.Product, enriched on read with its
// catalog info (resolved categories + brand) and review info (recent reviews;
// the rating/review_count aggregates are denormalized on the product document
// and maintained by the review repository).
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

// detailReviewLimit caps how many recent reviews are attached to a product
// detail response.
const detailReviewLimit = 5

// ProductRepository owns reads over the products collection plus the catalog
// (categories, brands) and reviews collections used to enrich a product on read.
type ProductRepository struct {
	products   *mongo.Collection
	categories *mongo.Collection
	brands     *mongo.Collection
	reviews    *mongo.Collection
}

// New builds the repository and ensures browse indexes exist.
func New(db *mongo.Database) *ProductRepository {
	products := db.Collection("products")
	_, _ = products.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "category", Value: 1}}},
		{Keys: bson.D{{Key: "brand_id", Value: 1}}},
		{Keys: bson.D{{Key: "sku", Value: 1}}},
	})
	return &ProductRepository{
		products:   products,
		categories: db.Collection("categories"),
		brands:     db.Collection("brands"),
		reviews:    db.Collection("reviews"),
	}
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
		filter["brand_id"] = p.Brand
	}
	if p.PriceMin != 0 || p.PriceMax != 0 {
		price := bson.M{}
		if p.PriceMin != 0 {
			price["$gte"] = p.PriceMin
		}
		if p.PriceMax != 0 {
			price["$lte"] = p.PriceMax
		}
		filter["quantity_distribution.price"] = price
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
	case "rating_desc":
		return bson.D{{Key: "rating", Value: -1}}
	case "price_asc":
		return bson.D{{Key: "quantity_distribution.price", Value: 1}}
	case "price_desc":
		return bson.D{{Key: "quantity_distribution.price", Value: -1}}
	default:
		return bson.D{{Key: "created_at", Value: -1}}
	}
}

// List returns a paginated page of storefront products for the given filters.
// Each product is enriched with its catalog info (categories + brand); reviews
// are attached only on detail reads, not in lists.
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
	products := []models.Product{}
	if err := cursor.All(ctx, &products); err != nil {
		return nil, err
	}
	for i := range products {
		r.attachCatalog(ctx, &products[i])
	}
	return &models.PagedProducts{Products: products, Total: total, Page: page, Count: count}, nil
}

// ListByCategory returns a paginated page of products in a category (by category
// id), enriched with catalog info.
func (r *ProductRepository) ListByCategory(ctx context.Context, categoryID string, page, count int) (*models.PagedProducts, error) {
	return r.List(ctx, models.ProductListParams{Category: categoryID, Page: page, Count: count})
}

// GetByID returns a single storefront product, enriched with its catalog info
// and a handful of recent reviews, or repoerr.ErrNotFound.
func (r *ProductRepository) GetByID(ctx context.Context, id string) (*models.Product, error) {
	product := &models.Product{}
	err := r.products.FindOne(ctx, bson.M{"_id": id}).Decode(product)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.attachCatalog(ctx, product)
	r.attachReviews(ctx, product)
	return product, nil
}

// GetImages returns the product's image URLs, or repoerr.ErrNotFound.
func (r *ProductRepository) GetImages(ctx context.Context, id string) ([]string, error) {
	var doc struct {
		Images []string `bson:"images"`
	}
	err := r.products.FindOne(ctx, bson.M{"_id": id}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return doc.Images, nil
}

// GetRelated returns up to `limit` products sharing a category (or, failing
// that, a brand) with the given product, excluding it. Returns
// repoerr.ErrNotFound if the product is missing.
func (r *ProductRepository) GetRelated(ctx context.Context, id string, limit int) ([]models.Product, error) {
	product := &models.Product{}
	err := r.products.FindOne(ctx, bson.M{"_id": id}).Decode(product)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 8
	}
	filter := bson.M{"_id": bson.M{"$ne": id}}
	if len(product.Category) > 0 {
		filter["category"] = bson.M{"$in": product.Category}
	} else if product.BrandID != "" {
		filter["brand_id"] = product.BrandID
	}
	cursor, err := r.products.Find(ctx, filter, options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	related := []models.Product{}
	if err := cursor.All(ctx, &related); err != nil {
		return nil, err
	}
	for i := range related {
		r.attachCatalog(ctx, &related[i])
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

// attachCatalog resolves the product's Category ids and BrandID into full
// catalog documents and attaches them to the product. Lookup failures are
// non-fatal: enrichment is best-effort, so a missing category/brand simply
// leaves the field empty rather than failing the whole read.
func (r *ProductRepository) attachCatalog(ctx context.Context, product *models.Product) {
	if len(product.Category) > 0 {
		cursor, err := r.categories.Find(ctx, bson.M{"_id": bson.M{"$in": product.Category}})
		if err == nil {
			categories := []models.Category{}
			if cursor.All(ctx, &categories) == nil {
				product.Categories = categories
			}
		}
	}
	if product.BrandID != "" {
		brand := &models.Brand{}
		if r.brands.FindOne(ctx, bson.M{"_id": product.BrandID}).Decode(brand) == nil {
			product.Brand = brand
		}
	}
}

// attachReviews attaches a handful of recent reviews to a product (best-effort).
func (r *ProductRepository) attachReviews(ctx context.Context, product *models.Product) {
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(detailReviewLimit)
	cursor, err := r.reviews.Find(ctx, bson.M{"product_id": product.ID}, opts)
	if err != nil {
		return
	}
	reviews := []models.Review{}
	if cursor.All(ctx, &reviews) == nil {
		product.Reviews = reviews
	}
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
