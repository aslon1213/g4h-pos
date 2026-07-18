// Package products is the repository for the admin products domain. It owns
// every GORM/Postgres interaction for products, mirroring the suppliers/finance
// repositories: the controller holds a *ProductsRepository and its handlers call
// these methods instead of touching the database directly. Non-DB I/O (S3 image
// storage) stays in the controller; this repository only persists the product's
// image list and its child tables.
//
// The product's array-shaped fields live in child tables rather than on the row:
//   - quantity_distribution[] -> product_stock       (models.ProductDistribution)
//   - income_history[]        -> product_income_history (models.IncomeHistory)
//   - category[]              -> product_categories   (junction: product_id, category_id)
//
// images[] stays inline as a jsonb column (gorm serializer). All three child
// tables are marked gorm:"-" on the parent and are therefore managed explicitly
// here (attached on read, written on create/update/income).
package products

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/ledger"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"

	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// productCategory is the row type for the product_categories junction table. It
// is local to the repository because the parent Product model only carries the
// category ids as a plain []string (gorm:"-").
type productCategory struct {
	ProductID  string `gorm:"column:product_id"`
	CategoryID string `gorm:"column:category_id"`
}

func (productCategory) TableName() string { return "product_categories" }

// ProductsRepository owns the products table and its child tables (product_stock,
// product_income_history, product_categories). The income/distribution flow also
// applies a supplier transaction, which reflects on branch finance and the
// supplier via the shared ledger primitives.
type ProductsRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *ProductsRepository {
	return &ProductsRepository{db: db}
}

// Query returns products matching the params. All filters are combined using AND;
// optional parameters are collected and applied in one go rather than chaining one by one.
func (r *ProductsRepository) Query(ctx context.Context, params models.ProductQueryParams) ([]models.Product, error) {
	log := log.Ctx(ctx).With().
		Str("method", "ProductsRepository.Query").
		Interface("params", params).
		Logger()

	log.Info().Msg("Building product query")

	query := r.db.WithContext(ctx).Model(&models.Product{})

	// To accumulate WHERE clauses and parameters
	var wheres []string
	var args []interface{}

	if params.ID != "" {
		log.Info().Str("id", params.ID).Msg("Adding ID filter")
		wheres = append(wheres, "id = ?")
		args = append(args, params.ID)
	}
	if params.Name != "" {
		log.Info().Str("name", params.Name).Msg("Adding Name filter")
		wheres = append(wheres, "name ILIKE ?")
		args = append(args, "%"+params.Name+"%")
	}
	if params.BranchID != "" {
		log.Info().Str("branch_id", params.BranchID).Msg("Adding BranchID filter")
		wheres = append(wheres, "EXISTS (SELECT 1 FROM product_stock ps WHERE ps.product_id = products.id AND ps.place_id = ?)")
		args = append(args, params.BranchID)
	}
	if params.SKU != "" {
		log.Info().Str("sku", params.SKU).Msg("Adding SKU filter")
		wheres = append(wheres, "sku = ?")
		args = append(args, params.SKU)
	}
	if params.PriceMin != 0 {
		log.Info().Float64("pricemin", params.PriceMin).Msg("Adding PriceMin filter")
		wheres = append(wheres, "EXISTS (SELECT 1 FROM product_stock ps WHERE ps.product_id = products.id AND ps.price >= ?)")
		args = append(args, params.PriceMin)
	}
	if params.PriceMax != 0 {
		log.Info().Float64("pricemax", params.PriceMax).Msg("Adding PriceMax filter")
		wheres = append(wheres, "EXISTS (SELECT 1 FROM product_stock ps WHERE ps.product_id = products.id AND ps.price <= ?)")
		args = append(args, params.PriceMax)
	}

	// Compose the full WHERE clause using AND for all filters
	if len(wheres) > 0 {
		whereClause := "(" + wheres[0]
		for i := 1; i < len(wheres); i++ {
			whereClause += " AND " + wheres[i]
		}
		whereClause += ")"
		query = query.Where(whereClause, args...)
	}

	products := []models.Product{}
	if err := query.Find(&products).Error; err != nil {
		log.Error().Err(err).Msg("Query failed")
		return nil, err
	}
	log.Info().Int("num_products", len(products)).Msg("Products found from query")

	for i := range products {
		if err := r.attachChildren(ctx, r.db, &products[i]); err != nil {
			log.Error().
				Str("product_id", products[i].ID).
				Err(err).
				Msg("Failed to attach children to product")
			return nil, err
		}
	}

	log.Info().Msg("Successfully fetched and attached children to products")
	return products, nil
}

// GetByID returns a single product with its child tables attached, or
// repoerr.ErrNotFound.
func (r *ProductsRepository) GetByID(ctx context.Context, id string) (*models.Product, error) {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.attachChildren(ctx, r.db, &product); err != nil {
		return nil, err
	}
	return &product, nil
}

// Create inserts a new product (built from base with zeroed distribution, images
// and income history) plus its category junction rows, and returns it. Runs in a
// transaction so the product row and its category rows commit atomically.
func (r *ProductsRepository) Create(ctx context.Context, base *models.ProductBase) (*models.Product, error) {
	product := models.NewProduct(base)
	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := gorm.G[models.Product](tx).Create(ctx, product); err != nil {
			return err
		}
		if len(base.Category) > 0 {
			if err := r.replaceCategories(ctx, tx, product.ID, base.Category); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return product, nil
}

// Update patches the editable fields of a product and returns the updated
// product (with child tables attached). Only non-zero fields of base are set,
// matching the original controller. A non-nil base.Category replaces the product's
// category junction rows. Returns repoerr.ErrNotFound when no product matches.
func (r *ProductsRepository) Update(ctx context.Context, id string, base *models.ProductBase) (*models.Product, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		set := map[string]interface{}{}
		if base.Name != "" {
			set["name"] = base.Name
		}
		if base.Description != "" {
			set["description"] = base.Description
		}
		if base.Manufacturer.Name != "" {
			set["manufacturer_name"] = base.Manufacturer.Name
		}
		if base.Manufacturer.Country != "" {
			set["manufacturer_country"] = base.Manufacturer.Country
		}
		if base.Manufacturer.Address != "" {
			set["manufacturer_address"] = base.Manufacturer.Address
		}
		if base.Manufacturer.Phone != "" {
			set["manufacturer_phone"] = base.Manufacturer.Phone
		}
		if base.Manufacturer.Email != "" {
			set["manufacturer_email"] = base.Manufacturer.Email
		}
		if base.BrandID != "" {
			set["brand_id"] = base.BrandID
		}
		if base.SKU != "" {
			set["sku"] = base.SKU
		}
		if base.MinimumStockAlert != 0 {
			set["minimum_stock_alert"] = base.MinimumStockAlert
		}

		if len(set) > 0 {
			res := tx.WithContext(ctx).Table("products").Where("id = ?", id).Updates(set)
			if res.Error != nil {
				return res.Error
			}
		}
		if base.Category != nil {
			if err := r.replaceCategories(ctx, tx, id, base.Category); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	// Re-read the product (with children) so the caller gets the persisted state.
	return r.GetByID(ctx, id)
}

// Delete removes a product. Its child rows (product_stock, product_income_history,
// product_categories) are removed by ON DELETE CASCADE. Reports
// repoerr.ErrNotFound when no product matched; the controller historically
// ignores that and returns 200.
func (r *ProductsRepository) Delete(ctx context.Context, id string) error {
	affected, err := gorm.G[models.Product](r.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// AddImage appends an S3 image key to the product's images jsonb column. S3
// upload itself stays in the controller; this only mutates the DB. Returns
// repoerr.ErrNotFound when no product matches the id.
func (r *ProductsRepository) AddImage(ctx context.Context, productID, key string) error {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", productID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return repoerr.ErrNotFound
	}
	if err != nil {
		return err
	}
	images := append(product.Images, key)
	affected, err := gorm.G[models.Product](r.db).Where("id = ?", productID).Update(ctx, "images", images)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// RemoveImage pulls an S3 image key from the product's images jsonb column. S3
// deletion stays in the controller. Returns repoerr.ErrNotFound when no product
// matches.
func (r *ProductsRepository) RemoveImage(ctx context.Context, productID, key string) error {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", productID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return repoerr.ErrNotFound
	}
	if err != nil {
		return err
	}
	images := make([]string, 0, len(product.Images))
	for _, img := range product.Images {
		if img != key {
			images = append(images, img)
		}
	}
	affected, err := gorm.G[models.Product](r.db).Where("id = ?", productID).Update(ctx, "images", images)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// GetIncomeHistory returns the recorded income history of a product, or
// repoerr.ErrNotFound when the product does not exist.
func (r *ProductsRepository) GetIncomeHistory(ctx context.Context, productID string) ([]models.IncomeHistory, error) {
	// Confirm the product exists so a missing product still yields ErrNotFound
	// (an existing product with no income simply returns an empty slice).
	_, err := gorm.G[models.Product](r.db).Select("id").Where("id = ?", productID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	history, err := gorm.G[models.IncomeHistory](r.db).Where("product_id = ?", productID).Find(ctx)
	if err != nil {
		return nil, err
	}
	return history, nil
}

// AddIncome records new income for a product inside a single GORM transaction:
// it upserts the per-branch stock row (setting the selling price and incrementing
// the quantity on a matching place, or inserting a new stock row otherwise),
// appends an income_history row, and applies a supplier debit via
// ledger.ApplySupplierTransaction (reflecting it on branch finance and the
// supplier). The price/quantity money math mirrors the original controller. The
// returned product is the pre-write snapshot (with children attached), matching
// the controller's response. Returns repoerr.ErrNotFound when the product does
// not exist.
func (r *ProductsRepository) AddIncome(ctx context.Context, productID string, input *models.IncomeHistory, sellingPrice int32) (*models.Product, error) {
	var product *models.Product

	err := r.db.Transaction(func(tx *gorm.DB) error {
		// Read the product (and its pre-write children) up front.
		p, err := gorm.G[models.Product](tx).Where("id = ?", productID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return repoerr.ErrNotFound
		}
		if err != nil {
			return err
		}
		if err := r.attachChildren(ctx, tx, &p); err != nil {
			return err
		}
		product = &p

		// Upsert the stock row for the target place: update if it exists,
		// otherwise insert a new distribution row. A UNIQUE(product_id, place_id)
		// backs the one-row-per-place invariant.
		_, err = gorm.G[models.ProductDistribution](tx).
			Where("product_id = ? AND place_id = ?", productID, input.UploadedTo.ID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if err := r.appendQuantityDistribution(ctx, tx, productID, input, sellingPrice); err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			res := tx.WithContext(ctx).Table("product_stock").
				Where("product_id = ? AND place_id = ?", productID, input.UploadedTo.ID).
				Updates(map[string]interface{}{
					"price":    sellingPrice,
					"quantity": gorm.Expr("quantity + ?", input.Quantity),
				})
			if res.Error != nil {
				return res.Error
			}
		}

		// Append the income-history record.
		income := *input
		income.ID = 0
		income.ProductID = productID
		if err := gorm.G[models.IncomeHistory](tx).Create(ctx, &income); err != nil {
			return err
		}

		// Apply the supplier debit (branch is the upload place id, mirroring the
		// original Mongo call) — but only when the income actually names a supplier.
		// Supplier-less income just adds stock; there is no supplier to owe, so no
		// supplier transaction is recorded (and no supplier_id FK to satisfy).
		if input.SupplierID != "" {
			transactionBase := models.TransactionBase{
				// Quantity is fractional, money is whole so'm — round through the
				// same helper the cart lines use, so every price*quantity in the
				// system rounds identically.
				Amount:        models.RoundLineTotal(input.Quantity, uint32(input.Price)),
				Description:   "Income from " + input.SupplierID,
				Type:          models.TransactionTypeDebit,
				PaymentMethod: models.PaymentMethodUndefined,
			}
			supplierTransaction, err := ledger.ApplySupplierTransaction(ctx, tx, transactionBase, input.SupplierID, input.UploadedTo.ID)
			if err != nil {
				return err
			}
			log.Debug().Interface("supplier_transaction", supplierTransaction).Msg("Supplier transaction created")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	log.Info().Str("product_id", productID).Msg("Successfully processed new income")
	return product, nil
}

// appendQuantityDistribution inserts a new per-place stock row for the product.
func (r *ProductsRepository) appendQuantityDistribution(ctx context.Context, tx *gorm.DB, productID string, input *models.IncomeHistory, sellingPrice int32) error {
	dist := models.ProductDistribution{
		ProductID: productID,
		ProductQuantityInfo: models.ProductQuantityInfo{
			Quantity: input.Quantity,
		},
		Place: input.UploadedTo,
		Price: sellingPrice,
	}
	if err := gorm.G[models.ProductDistribution](tx).Create(ctx, &dist); err != nil {
		log.Error().Err(err).Msg("Failed to create new distribution")
		return err
	}
	return nil
}

// attachChildren loads the product's child tables (stock, income history and
// category ids) and attaches them onto the product. db may be r.db or a
// transaction handle.
func (r *ProductsRepository) attachChildren(ctx context.Context, db *gorm.DB, product *models.Product) error {
	stock, err := gorm.G[models.ProductDistribution](db).Where("product_id = ?", product.ID).Find(ctx)
	if err != nil {
		return err
	}
	product.QuantityDistribution = stock

	income, err := gorm.G[models.IncomeHistory](db).Where("product_id = ?", product.ID).Find(ctx)
	if err != nil {
		return err
	}
	product.IncomeHistory = income

	cats, err := r.loadCategoryIDs(ctx, db, product.ID)
	if err != nil {
		return err
	}
	product.Category = cats
	return nil
}

// loadCategoryIDs returns the category ids linked to a product via the
// product_categories junction table.
func (r *ProductsRepository) loadCategoryIDs(ctx context.Context, db *gorm.DB, productID string) ([]string, error) {
	rows, err := gorm.G[productCategory](db).Where("product_id = ?", productID).Find(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(rows))
	for _, row := range rows {
		ids = append(ids, row.CategoryID)
	}
	return ids, nil
}

// replaceCategories rewrites the product_categories rows for a product to exactly
// the given set of category ids (delete-then-insert). An empty/nil slice clears
// them. Runs on the supplied tx so it participates in the caller's transaction.
func (r *ProductsRepository) replaceCategories(ctx context.Context, tx *gorm.DB, productID string, categoryIDs []string) error {
	if _, err := gorm.G[productCategory](tx).Where("product_id = ?", productID).Delete(ctx); err != nil {
		return err
	}
	if len(categoryIDs) == 0 {
		return nil
	}
	rows := make([]productCategory, 0, len(categoryIDs))
	for _, cid := range categoryIDs {
		rows = append(rows, productCategory{ProductID: productID, CategoryID: cid})
	}
	return gorm.G[productCategory](tx).CreateInBatches(ctx, &rows, len(rows))
}
