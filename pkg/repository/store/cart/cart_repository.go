// Package cart is the repository for the authenticated customer's shopping cart
// (the `carts` table plus its `cart_items` child table, one cart per customer).
// It reads the products/product_stock tables to snapshot item name/price when
// lines are added.
package cart

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// CartRepository owns the carts/cart_items tables and reads products for line
// snapshots.
type CartRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *CartRepository {
	return &CartRepository{db: db}
}

// GetByCustomer returns the customer's cart, creating an empty one on first use.
func (r *CartRepository) GetByCustomer(ctx context.Context, customerID string) (*models.Cart, error) {
	cart, err := gorm.G[models.Cart](r.db).Where("customer_id = ?", customerID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r.ensure(ctx, customerID)
	}
	if err != nil {
		return nil, err
	}
	r.attachItems(ctx, &cart)
	return &cart, nil
}

// ensure find-or-creates an empty cart for the customer and returns it.
func (r *CartRepository) ensure(ctx context.Context, customerID string) (*models.Cart, error) {
	now := time.Now()
	cart := &models.Cart{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	// One cart per customer: on a concurrent insert the unique index fires and
	// we simply re-read the existing row.
	err := r.db.WithContext(ctx).
		Clauses(clause.OnConflict{Columns: []clause.Column{{Name: "customer_id"}}, DoNothing: true}).
		Create(cart).Error
	if err != nil {
		return nil, err
	}
	return r.GetByCustomer(ctx, customerID)
}

// attachItems loads the cart's line items (best-effort).
func (r *CartRepository) attachItems(ctx context.Context, cart *models.Cart) {
	items, err := gorm.G[models.CartItem](r.db).Where("cart_id = ?", cart.ID).Order("id").Find(ctx)
	if err == nil {
		cart.Items = items
	}
	if cart.Items == nil {
		cart.Items = []models.CartItem{}
	}
}

// AddItem adds (or increments, if already present) a product line, snapshotting
// the product's name/image/price. Returns repoerr.ErrInvalidInput on bad qty and
// repoerr.ErrNotFound if the product does not exist.
func (r *CartRepository) AddItem(ctx context.Context, customerID, productID string, quantity int) (*models.Cart, error) {
	if quantity <= 0 {
		return nil, repoerr.ErrInvalidInput
	}
	cart, err := r.GetByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}

	// Merge into an existing line for the same product if present.
	for i := range cart.Items {
		if cart.Items[i].ProductID == productID {
			cart.Items[i].Quantity += quantity
			r.recompute(cart)
			return r.save(ctx, cart)
		}
	}

	snap, err := r.productSnapshot(ctx, productID)
	if err != nil {
		return nil, err
	}
	snap.ID = uuid.New().String()
	snap.Quantity = quantity
	cart.Items = append(cart.Items, *snap)
	r.recompute(cart)
	return r.save(ctx, cart)
}

// UpdateItem sets a line's quantity (a quantity <= 0 removes the line).
func (r *CartRepository) UpdateItem(ctx context.Context, customerID, itemID string, quantity int) (*models.Cart, error) {
	cart, err := r.GetByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	found := false
	items := cart.Items[:0]
	for _, it := range cart.Items {
		if it.ID == itemID {
			found = true
			if quantity <= 0 {
				continue // drop the line
			}
			it.Quantity = quantity
		}
		items = append(items, it)
	}
	if !found {
		return nil, repoerr.ErrNotFound
	}
	cart.Items = items
	r.recompute(cart)
	return r.save(ctx, cart)
}

// RemoveItem removes a single line from the cart.
func (r *CartRepository) RemoveItem(ctx context.Context, customerID, itemID string) (*models.Cart, error) {
	cart, err := r.GetByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	found := false
	items := cart.Items[:0]
	for _, it := range cart.Items {
		if it.ID == itemID {
			found = true
			continue
		}
		items = append(items, it)
	}
	if !found {
		return nil, repoerr.ErrNotFound
	}
	cart.Items = items
	r.recompute(cart)
	return r.save(ctx, cart)
}

// Clear empties the cart (items and coupon).
func (r *CartRepository) Clear(ctx context.Context, customerID string) (*models.Cart, error) {
	cart, err := r.GetByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	cart.Items = []models.CartItem{}
	cart.CouponCode = ""
	r.recompute(cart)
	return r.save(ctx, cart)
}

// SetPromo stores a coupon code on the cart (validation happens elsewhere).
func (r *CartRepository) SetPromo(ctx context.Context, customerID, code string) (*models.Cart, error) {
	cart, err := r.GetByCustomer(ctx, customerID)
	if err != nil {
		return nil, err
	}
	cart.CouponCode = code
	r.recompute(cart)
	return r.save(ctx, cart)
}

// RemovePromo clears any coupon code from the cart.
func (r *CartRepository) RemovePromo(ctx context.Context, customerID string) (*models.Cart, error) {
	return r.SetPromo(ctx, customerID, "")
}

// save persists the cart's totals and replaces its line items (the whole item
// set is rewritten, mirroring the old "$set items" semantics), then returns the
// stored cart.
func (r *CartRepository) save(ctx context.Context, cart *models.Cart) (*models.Cart, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		res := tx.WithContext(ctx).Model(&models.Cart{}).Where("id = ?", cart.ID).Updates(map[string]interface{}{
			"coupon_code": cart.CouponCode,
			"subtotal":    cart.Subtotal,
			"discount":    cart.Discount,
			"total":       cart.Total,
			"updated_at":  time.Now(),
		})
		if res.Error != nil {
			return res.Error
		}
		if _, err := gorm.G[models.CartItem](tx).Where("cart_id = ?", cart.ID).Delete(ctx); err != nil {
			return err
		}
		if len(cart.Items) > 0 {
			items := make([]models.CartItem, len(cart.Items))
			copy(items, cart.Items)
			for i := range items {
				items[i].CartID = cart.ID
				if items[i].ID == "" {
					items[i].ID = uuid.New().String()
				}
			}
			if err := gorm.G[models.CartItem](tx).CreateInBatches(ctx, &items, len(items)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return r.GetByCustomer(ctx, cart.CustomerID)
}

// recompute refreshes each line total and the cart subtotal/total in place.
func (r *CartRepository) recompute(cart *models.Cart) {
	var subtotal float64
	for i := range cart.Items {
		cart.Items[i].LineTotal = cart.Items[i].UnitPrice * float64(cart.Items[i].Quantity)
		subtotal += cart.Items[i].LineTotal
	}
	cart.Subtotal = subtotal
	cart.Total = subtotal - cart.Discount
}

// productSnapshot reads a product and returns a cart line pre-filled with its
// display fields and a representative price.
func (r *CartRepository) productSnapshot(ctx context.Context, productID string) (*models.CartItem, error) {
	product, err := gorm.G[models.Product](r.db).Where("id = ?", productID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := &models.CartItem{ProductID: productID, Name: product.Name}
	if len(product.Images) > 0 {
		item.Image = product.Images[0]
	}
	price, err := r.firstStockPrice(ctx, productID)
	if err != nil {
		return nil, err
	}
	item.UnitPrice = price
	return item, nil
}

// firstStockPrice returns the price of the product's first stock row (mirrors
// the old quantity_distribution[0].price), or 0 when the product has no stock.
func (r *CartRepository) firstStockPrice(ctx context.Context, productID string) (float64, error) {
	stock, err := gorm.G[models.ProductDistribution](r.db).Where("product_id = ?", productID).Order("id").First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return float64(stock.Price), nil
}
