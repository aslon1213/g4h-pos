// Package wishlist is the repository for the customer's saved-for-later list
// (the `wishlists` table plus its `wishlist_items` child table, one per
// customer).
package wishlist

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// WishlistRepository owns the wishlists/wishlist_items tables and reads products
// for snapshots.
type WishlistRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *WishlistRepository {
	return &WishlistRepository{db: db}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint (23505)
// violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// GetByCustomer returns the customer's wishlist, creating an empty one on first use.
func (r *WishlistRepository) GetByCustomer(ctx context.Context, customerID string) (*models.Wishlist, error) {
	wl, err := gorm.G[models.Wishlist](r.db).Where("customer_id = ?", customerID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.ensure(ctx, customerID)
	}
	if err != nil {
		return nil, err
	}
	r.attachItems(ctx, &wl)
	return &wl, nil
}

// ensure find-or-creates an empty wishlist for the customer and returns it.
func (r *WishlistRepository) ensure(ctx context.Context, customerID string) (*models.Wishlist, error) {
	now := time.Now()
	wl := &models.Wishlist{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "customer_id"}}, DoNothing: true}).
		Create(wl).Error
	if err != nil {
		return nil, err
	}
	return r.GetByCustomer(ctx, customerID)
}

// attachItems loads the wishlist's saved products (best-effort).
func (r *WishlistRepository) attachItems(ctx context.Context, wl *models.Wishlist) {
	items, err := gorm.G[models.WishlistItem](r.db).Where("wishlist_id = ?", wl.ID).Order("id").Find(ctx)
	if err == nil {
		wl.Items = items
	}
	if wl.Items == nil {
		wl.Items = []models.WishlistItem{}
	}
}

// AddItem adds a product to the wishlist (idempotent — no duplicates). Returns
// repoerr.ErrNotFound if the product does not exist.
func (r *WishlistRepository) AddItem(ctx context.Context, customerID, productID string) (*models.Wishlist, error) {
	wl, err := r.GetByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	for _, it := range wl.Items {
		if it.ProductID == productID {
			return wl, nil // already present
		}
	}
	item, err := r.productSnapshot(ctx, productID)
	if err != nil {
		return nil, err
	}
	item.WishlistID = wl.ID
	if err := gorm.G[models.WishlistItem](r.db).Create(ctx, item); err != nil {
		// A concurrent add trips the unique(wishlist_id, product_id) index; the
		// product is already saved, so just return the current wishlist.
		if isUniqueViolation(err) {
			return r.GetByCustomer(ctx, customerID)
		}
		return nil, err
	}
	r.touch(ctx, wl.ID)
	return r.GetByCustomer(ctx, customerID)
}

// RemoveItem removes a product from the wishlist.
func (r *WishlistRepository) RemoveItem(ctx context.Context, customerID, productID string) (*models.Wishlist, error) {
	wl, err := gorm.G[models.Wishlist](r.db).Where("customer_id = ?", customerID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if _, err := gorm.G[models.WishlistItem](r.db).Where("wishlist_id = ? AND product_id = ?", wl.ID, productID).Delete(ctx); err != nil {
		return nil, err
	}
	r.touch(ctx, wl.ID)
	return r.GetByCustomer(ctx, customerID)
}

// touch bumps the wishlist's updated_at (best-effort).
func (r *WishlistRepository) touch(ctx context.Context, wishlistID string) {
	_, _ = gorm.G[models.Wishlist](r.db).Where("id = ?", wishlistID).Update(ctx, "updated_at", time.Now())
}

// productSnapshot reads a product and returns a wishlist line pre-filled with
// its display fields and a representative price.
func (r *WishlistRepository) productSnapshot(ctx context.Context, productID string) (*models.WishlistItem, error) {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", productID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := &models.WishlistItem{ProductID: productID, Name: product.Name, AddedAt: time.Now()}
	if len(product.Images) > 0 {
		item.Image = product.Images[0]
	}
	price, err := r.firstStockPrice(ctx, productID)
	if err != nil {
		return nil, err
	}
	item.Price = price
	return item, nil
}

// firstStockPrice returns the price of the product's first stock row (mirrors
// the old quantity_distribution[0].price), or 0 when the product has no stock.
func (r *WishlistRepository) firstStockPrice(ctx context.Context, productID string) (float64, error) {
	stock, err := gorm.G[models.ProductDistribution](r.db).Where("product_id = ?", productID).Order("id").First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return float64(stock.Price), nil
}
