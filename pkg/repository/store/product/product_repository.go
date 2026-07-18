// Package product is the read-only repository backing the storefront product
// browse surface. It reads the existing `products` table (written by the staff
// side) and returns the full models.Product, enriched on read with its catalog
// info (resolved categories from product_categories + brand) and review info
// (recent reviews; the rating/review_count aggregates are denormalized on the
// product row and maintained by the review repository).
package product

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// detailReviewLimit caps how many recent reviews are attached to a product
// detail response.
const detailReviewLimit = 5

// ProductRepository reads over the products table plus the catalog
// (categories, brands, product_categories), product_stock and reviews tables
// used to enrich a product on read.
type ProductRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *ProductRepository {
	return &ProductRepository{db: db}
}

// applyFilters translates list/search params into where-conditions on a product
// query. Category and price live in child tables (product_categories,
// product_stock) so those filters use correlated subqueries.
func (r *ProductRepository) applyFilters(q gorm.ChainInterface[models.Product], p models.ProductListParams) gorm.ChainInterface[models.Product] {
	if p.Query != "" {
		q = q.Where("name ILIKE ?", "%"+p.Query+"%")
	}
	if p.Category != "" {
		q = q.Where("EXISTS (SELECT 1 FROM product_categories pc WHERE pc.product_id = products.id AND pc.category_id = ?)", p.Category)
	}
	if p.Brand != "" {
		q = q.Where("brand_id = ?", p.Brand)
	}
	if p.SKU != "" {
		q = q.Where("sku = ?", p.SKU)
	}

	// Branch, price range and in-stock all constrain the same product_stock rows,
	// so collapse them into ONE correlated subquery. This is cheaper (a single
	// EXISTS instead of up to three) and more correct: with a branch AND a price
	// range set, the SAME stock row must satisfy both (priced in range AT that
	// branch), rather than matching two unrelated stock rows.
	if p.BranchID != "" || p.PriceMin != 0 || p.PriceMax != 0 || p.InStock {
		cond := "EXISTS (SELECT 1 FROM product_stock ps WHERE ps.product_id = products.id"
		args := []interface{}{}
		if p.BranchID != "" {
			cond += " AND ps.place_id = ?"
			args = append(args, p.BranchID)
		}
		if p.PriceMin != 0 {
			cond += " AND ps.price >= ?"
			args = append(args, p.PriceMin)
		}
		if p.PriceMax != 0 {
			cond += " AND ps.price <= ?"
			args = append(args, p.PriceMax)
		}
		if p.InStock {
			cond += " AND ps.quantity > 0"
		}
		cond += ")"
		q = q.Where(cond, args...)
	}
	return q
}

// sortExpr maps a sort token to an ORDER BY expression (default: newest first).
// Price sorting keys off the min/max stock price via a correlated subquery,
// mirroring how Mongo sorted on the quantity_distribution.price array.
func sortExpr(sort string) string {
	switch sort {
	case "newest":
		return "created_at DESC"
	case "name_asc":
		return "name ASC"
	case "name_desc":
		return "name DESC"
	case "rating_desc":
		return "rating DESC"
	case "price_asc":
		return "(SELECT MIN(price) FROM product_stock ps WHERE ps.product_id = products.id) ASC"
	case "price_desc":
		return "(SELECT MAX(price) FROM product_stock ps WHERE ps.product_id = products.id) DESC"
	default:
		return "created_at DESC"
	}
}

// List returns a paginated page of storefront products for the given filters.
// Each product is enriched with its catalog info (categories + brand); reviews
// are attached only on detail reads, not in lists.
func (r *ProductRepository) List(ctx context.Context, p models.ProductListParams) (*models.PagedProducts, error) {
	// Logging: entrypoint and input params
	log.Info().
		Str("method", "ProductRepository.List").
		Interface("filters", p).
		Msg("Listing products with input filters")

	page, count := normalizePaging(p.Page, p.Count)

	q := r.applyFilters(gorm.G[models.Product](r.db).Where("1 = 1"), p)

	total, err := q.Count(ctx, "*")
	if err != nil {
		log.Error().
			Err(err).
			Str("method", "ProductRepository.List").
			Msg("Failed to count products")
		return nil, err
	}

	products, err := q.Order(sortExpr(p.Sort)).Offset((page - 1) * count).Limit(count).Find(ctx)
	if err != nil {
		log.Error().
			Err(err).
			Str("method", "ProductRepository.List").
			Msg("Failed to fetch products")
		return nil, err
	}

	for i := range products {
		r.attachCatalog(ctx, &products[i])
		r.attachDistribution(ctx, &products[i])
	}

	log.Info().
		Str("method", "ProductRepository.List").
		Int("returned", len(products)).
		Int64("total", total).
		Int("page", page).
		Int("count", count).
		Msg("Returning paged product list")

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
	product, err := gorm.G[models.Product](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.attachCatalog(ctx, &product)
	r.attachDistribution(ctx, &product)
	r.attachReviews(ctx, &product)
	return &product, nil
}

// GetImages returns the product's image URLs, or repoerr.ErrNotFound.
func (r *ProductRepository) GetImages(ctx context.Context, id string) ([]string, error) {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if product.Images == nil {
		return []string{}, nil
	}
	return product.Images, nil
}

// GetRelated returns up to `limit` products sharing a category (or, failing
// that, a brand) with the given product, excluding it. Returns
// repoerr.ErrNotFound if the product is missing.
func (r *ProductRepository) GetRelated(ctx context.Context, id string, limit int) ([]models.Product, error) {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.loadCategoryIDs(ctx, &product)
	if limit <= 0 {
		limit = 8
	}
	q := gorm.G[models.Product](r.db).Where("id <> ?", id)
	if len(product.Category) > 0 {
		q = q.Where("EXISTS (SELECT 1 FROM product_categories pc WHERE pc.product_id = products.id AND pc.category_id IN ?)", product.Category)
	} else if product.BrandID != "" {
		q = q.Where("brand_id = ?", product.BrandID)
	}
	related, err := q.Limit(limit).Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range related {
		r.attachCatalog(ctx, &related[i])
		r.attachDistribution(ctx, &related[i])
	}
	return related, nil
}

// GetAvailability reports per-branch stock derived from the product's stock
// rows. Returns repoerr.ErrNotFound if the product is missing.
func (r *ProductRepository) GetAvailability(ctx context.Context, id string) ([]models.BranchAvailability, error) {
	cnt, err := gorm.G[models.Product](r.db).Where("id = ?", id).Count(ctx, "*")
	if err != nil {
		return nil, err
	}
	if cnt == 0 {
		return nil, repoerr.ErrNotFound
	}
	stocks, err := gorm.G[models.ProductDistribution](r.db).Where("product_id = ?", id).Find(ctx)
	if err != nil {
		return nil, err
	}
	availability := make([]models.BranchAvailability, 0, len(stocks))
	for _, s := range stocks {
		availability = append(availability, models.BranchAvailability{
			BranchID: s.Place.ID,
			Quantity: int(s.Quantity),
			InStock:  s.Quantity > 0,
		})
	}
	return availability, nil
}

// GetReviews returns a paginated list of a product's reviews (newest first).
func (r *ProductRepository) GetReviews(ctx context.Context, productID string, page, count int) (*models.PagedReviews, error) {
	page, count = normalizePaging(page, count)
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

// loadCategoryIDs populates product.Category from the product_categories join
// table (best-effort; the field is not persisted directly on the product row).
func (r *ProductRepository) loadCategoryIDs(ctx context.Context, product *models.Product) {
	var ids []string
	_ = r.db.WithContext(ctx).Table("product_categories").
		Where("product_id = ?", product.ID).
		Pluck("category_id", &ids)
	product.Category = ids
}

// attachCatalog resolves the product's category ids and BrandID into full
// catalog documents and attaches them to the product. Lookup failures are
// non-fatal: enrichment is best-effort, so a missing category/brand simply
// leaves the field empty rather than failing the whole read.
func (r *ProductRepository) attachCatalog(ctx context.Context, product *models.Product) {
	r.loadCategoryIDs(ctx, product)
	if len(product.Category) > 0 {
		categories, err := gorm.G[models.Category](r.db).Where("id IN ?", product.Category).Find(ctx)
		if err == nil {
			product.Categories = categories
		}
	}
	if product.BrandID != "" {
		brand, err := gorm.G[models.Brand](r.db).Where("id = ?", product.BrandID).First(ctx)
		if err == nil {
			product.Brand = &brand
		}
	}
}

// attachDistribution loads the product's per-place stock rows (product_stock)
// into QuantityDistribution. That field is gorm:"-", so it is never auto-loaded;
// this mirrors the admin repo's attachChildren. Best-effort and never nil.
func (r *ProductRepository) attachDistribution(ctx context.Context, product *models.Product) {
	dists, err := gorm.G[models.ProductDistribution](r.db).
		Where("product_id = ?", product.ID).
		Order("id").
		Find(ctx)
	if err == nil {
		product.QuantityDistribution = dists
	}
	if product.QuantityDistribution == nil {
		product.QuantityDistribution = []models.ProductDistribution{}
	}
}

// attachReviews attaches a handful of recent reviews to a product (best-effort).
func (r *ProductRepository) attachReviews(ctx context.Context, product *models.Product) {
	reviews, err := gorm.G[models.Review](r.db).
		Where("product_id = ?", product.ID).
		Order("created_at DESC").
		Limit(detailReviewLimit).
		Find(ctx)
	if err == nil {
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
