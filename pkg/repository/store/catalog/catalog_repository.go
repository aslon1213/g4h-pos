// Package catalog is the repository for the public catalog browse surface
// (categories and brands). Product listing/search is served by the product
// repository, which this package does not duplicate.
package catalog

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// CatalogRepository owns the categories and brands collections.
type CatalogRepository struct {
	categories *mongo.Collection
	brands     *mongo.Collection
}

// New builds the repository and ensures lookup indexes exist.
func New(db *mongo.Database) *CatalogRepository {
	categories := db.Collection("categories")
	brands := db.Collection("brands")
	_, _ = categories.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "slug", Value: 1}}},
		{Keys: bson.D{{Key: "parent_id", Value: 1}}},
	})
	_, _ = brands.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "slug", Value: 1}},
	})
	return &CatalogRepository{categories: categories, brands: brands}
}

// GetCategories returns all active categories ordered by sort_order.
func (r *CatalogRepository) GetCategories(ctx context.Context) ([]models.Category, error) {
	cursor, err := r.categories.Find(ctx,
		bson.M{"is_active": true},
		options.Find().SetSort(bson.D{{Key: "sort_order", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	categories := []models.Category{}
	if err := cursor.All(ctx, &categories); err != nil {
		return nil, err
	}
	return categories, nil
}

// GetCategoryByID returns one category, or repoerr.ErrNotFound.
func (r *CatalogRepository) GetCategoryByID(ctx context.Context, id string) (*models.Category, error) {
	category := &models.Category{}
	err := r.categories.FindOne(ctx, bson.M{"_id": id}).Decode(category)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return category, nil
}

// GetBrands returns all brands ordered by name.
func (r *CatalogRepository) GetBrands(ctx context.Context) ([]models.Brand, error) {
	cursor, err := r.brands.Find(ctx,
		bson.M{},
		options.Find().SetSort(bson.D{{Key: "name", Value: 1}}),
	)
	if err != nil {
		return nil, err
	}
	brands := []models.Brand{}
	if err := cursor.All(ctx, &brands); err != nil {
		return nil, err
	}
	return brands, nil
}
