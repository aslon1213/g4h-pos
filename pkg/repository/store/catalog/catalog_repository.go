// Package catalog is the repository for the public catalog browse surface
// (the `categories` and `brands` tables). Product listing/search is served by
// the product repository, which this package does not duplicate.
package catalog

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

// CatalogRepository owns the categories and brands tables.
type CatalogRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *CatalogRepository {
	return &CatalogRepository{db: db}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint (23505)
// violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// GetCategories returns all active categories ordered by sort_order.
func (r *CatalogRepository) GetCategories(ctx context.Context) ([]models.Category, error) {
	return gorm.G[models.Category](r.db).Where("is_active = ?", true).Order("sort_order").Find(ctx)
}

// GetCategoryByID returns one category, or repoerr.ErrNotFound.
func (r *CatalogRepository) GetCategoryByID(ctx context.Context, id string) (*models.Category, error) {
	category, err := gorm.G[models.Category](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &category, nil
}

// GetBrands returns all brands ordered by name.
func (r *CatalogRepository) GetBrands(ctx context.Context) ([]models.Brand, error) {
	return gorm.G[models.Brand](r.db).Order("name").Find(ctx)
}

// GetBrandByID returns one brand, or repoerr.ErrNotFound.
func (r *CatalogRepository) GetBrandByID(ctx context.Context, id string) (*models.Brand, error) {
	brand, err := gorm.G[models.Brand](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &brand, nil
}

// ---- admin write surface (categories) ----

// ListAllCategories returns every category (including inactive ones) ordered by
// sort_order. Used by the admin catalog management surface.
func (r *CatalogRepository) ListAllCategories(ctx context.Context) ([]models.Category, error) {
	return gorm.G[models.Category](r.db).Order("sort_order").Find(ctx)
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
	if err := gorm.G[models.Category](r.db).Create(ctx, category); err != nil {
		if isUniqueViolation(err) {
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
	set := map[string]interface{}{"updated_at": time.Now()}
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
	res := r.db.WithContext(ctx).Model(&models.Category{}).Where("id = ?", id).Updates(set)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, repoerr.ErrNotFound
	}
	return r.GetCategoryByID(ctx, id)
}

// DeleteCategory removes a category. Returns repoerr.ErrNotFound when none
// matches.
func (r *CatalogRepository) DeleteCategory(ctx context.Context, id string) error {
	affected, err := gorm.G[models.Category](r.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
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
	if err := gorm.G[models.Brand](r.db).Create(ctx, brand); err != nil {
		if isUniqueViolation(err) {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	return brand, nil
}

// UpdateBrand patches the editable fields of a brand and returns the updated
// document. Returns repoerr.ErrNotFound when none matches.
func (r *CatalogRepository) UpdateBrand(ctx context.Context, id string, in models.BrandInput) (*models.Brand, error) {
	set := map[string]interface{}{}
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

	// Guard against an empty update set (no editable fields supplied): fall back
	// to a plain existence check so callers still get ErrNotFound / the row back.
	if len(set) == 0 {
		return r.GetBrandByID(ctx, id)
	}

	res := r.db.WithContext(ctx).Model(&models.Brand{}).Where("id = ?", id).Updates(set)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, repoerr.ErrNotFound
	}
	return r.GetBrandByID(ctx, id)
}

// DeleteBrand removes a brand. Returns repoerr.ErrNotFound when none matches.
func (r *CatalogRepository) DeleteBrand(ctx context.Context, id string) error {
	affected, err := gorm.G[models.Brand](r.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}
