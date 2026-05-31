// Package catalog is the repository for the public catalog browse surface
// (categories and brands). Product listing/search is served by the product
// repository, which this package does not duplicate.
package catalog

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

// GetBrandByID returns one brand, or repoerr.ErrNotFound.
func (r *CatalogRepository) GetBrandByID(ctx context.Context, id string) (*models.Brand, error) {
	brand := &models.Brand{}
	err := r.brands.FindOne(ctx, bson.M{"_id": id}).Decode(brand)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return brand, nil
}

// ---- admin write surface (categories) ----

// ListAllCategories returns every category (including inactive ones) ordered by
// sort_order. Used by the admin catalog management surface.
func (r *CatalogRepository) ListAllCategories(ctx context.Context) ([]models.Category, error) {
	cursor, err := r.categories.Find(ctx,
		bson.M{},
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

// CreateCategory inserts a new category from admin input. is_active defaults to
// true when unset.
func (r *CatalogRepository) CreateCategory(ctx context.Context, in models.CategoryInput) (*models.Category, error) {
	now := time.Now()
	active := true
	if in.IsActive != nil {
		active = *in.IsActive
	}
	category := &models.Category{
		ID:          uuid.New().String(),
		Name:        in.Name,
		Slug:        in.Slug,
		ParentID:    in.ParentID,
		Description: in.Description,
		Image:       in.Image,
		SortOrder:   in.SortOrder,
		IsActive:    active,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if _, err := r.categories.InsertOne(ctx, category); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	return category, nil
}

// UpdateCategory patches the editable fields of a category and returns the
// updated document. Only non-zero fields of in are set (is_active is set when
// the pointer is non-nil). Returns repoerr.ErrNotFound when none matches.
func (r *CatalogRepository) UpdateCategory(ctx context.Context, id string, in models.CategoryInput) (*models.Category, error) {
	set := bson.M{"updated_at": time.Now()}
	if in.Name != "" {
		set["name"] = in.Name
	}
	if in.Slug != "" {
		set["slug"] = in.Slug
	}
	if in.ParentID != "" {
		set["parent_id"] = in.ParentID
	}
	if in.Description != "" {
		set["description"] = in.Description
	}
	if in.Image != "" {
		set["image"] = in.Image
	}
	if in.SortOrder != 0 {
		set["sort_order"] = in.SortOrder
	}
	if in.IsActive != nil {
		set["is_active"] = *in.IsActive
	}

	category := &models.Category{}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.categories.FindOneAndUpdate(ctx, bson.M{"_id": id}, bson.M{"$set": set}, opts).Decode(category)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return category, nil
}

// DeleteCategory removes a category. Returns repoerr.ErrNotFound when none
// matches.
func (r *CatalogRepository) DeleteCategory(ctx context.Context, id string) error {
	res, err := r.categories.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// ---- admin write surface (brands) ----

// CreateBrand inserts a new brand from admin input.
func (r *CatalogRepository) CreateBrand(ctx context.Context, in models.BrandInput) (*models.Brand, error) {
	brand := &models.Brand{
		ID:      uuid.New().String(),
		Name:    in.Name,
		Slug:    in.Slug,
		Logo:    in.Logo,
		Country: in.Country,
	}
	if _, err := r.brands.InsertOne(ctx, brand); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	return brand, nil
}

// UpdateBrand patches the editable fields of a brand and returns the updated
// document. Returns repoerr.ErrNotFound when none matches.
func (r *CatalogRepository) UpdateBrand(ctx context.Context, id string, in models.BrandInput) (*models.Brand, error) {
	set := bson.M{}
	if in.Name != "" {
		set["name"] = in.Name
	}
	if in.Slug != "" {
		set["slug"] = in.Slug
	}
	if in.Logo != "" {
		set["logo"] = in.Logo
	}
	if in.Country != "" {
		set["country"] = in.Country
	}

	brand := &models.Brand{}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.brands.FindOneAndUpdate(ctx, bson.M{"_id": id}, bson.M{"$set": set}, opts).Decode(brand)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return brand, nil
}

// DeleteBrand removes a brand. Returns repoerr.ErrNotFound when none matches.
func (r *CatalogRepository) DeleteBrand(ctx context.Context, id string) error {
	res, err := r.brands.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}
