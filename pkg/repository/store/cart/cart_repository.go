// Package cart is the repository for the authenticated customer's shopping cart
// (the `carts` collection, one document per customer). It reads the products
// collection to snapshot item name/price when lines are added.
package cart

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

// CartRepository owns the carts collection and reads products for line snapshots.
type CartRepository struct {
	carts    *mongo.Collection
	products *mongo.Collection
}

// New builds the repository and ensures the unique customer index exists.
func New(db *mongo.Database) *CartRepository {
	carts := db.Collection("carts")
	_, _ = carts.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "customer_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &CartRepository{carts: carts, products: db.Collection("products")}
}

// GetByCustomer returns the customer's cart, creating an empty one on first use.
func (r *CartRepository) GetByCustomer(ctx context.Context, customerID string) (*models.Cart, error) {
	cart := &models.Cart{}
	err := r.carts.FindOne(ctx, bson.M{"customer_id": customerID}).Decode(cart)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return r.ensure(ctx, customerID)
	}
	if err != nil {
		return nil, err
	}
	return cart, nil
}

// ensure upserts an empty cart for the customer and returns it.
func (r *CartRepository) ensure(ctx context.Context, customerID string) (*models.Cart, error) {
	now := time.Now()
	cart := &models.Cart{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		Items:      []models.CartItem{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	opts := options.UpdateOne().SetUpsert(true)
	_, err := r.carts.UpdateOne(ctx,
		bson.M{"customer_id": customerID},
		bson.M{"$setOnInsert": cart},
		opts,
	)
	if err != nil {
		return nil, err
	}
	return r.GetByCustomer(ctx, customerID)
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

// save persists the cart's items/totals and returns the stored document.
func (r *CartRepository) save(ctx context.Context, cart *models.Cart) (*models.Cart, error) {
	_, err := r.carts.UpdateOne(ctx,
		bson.M{"customer_id": cart.CustomerID},
		bson.M{"$set": bson.M{
			"items":       cart.Items,
			"coupon_code": cart.CouponCode,
			"subtotal":    cart.Subtotal,
			"discount":    cart.Discount,
			"total":       cart.Total,
			"updated_at":  time.Now(),
		}},
	)
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
	var doc struct {
		Name                 string   `bson:"name"`
		Images               []string `bson:"images"`
		QuantityDistribution []struct {
			Price int32 `bson:"price"`
		} `bson:"quantity_distribution"`
	}
	err := r.products.FindOne(ctx, bson.M{"_id": productID}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	item := &models.CartItem{ProductID: productID, Name: doc.Name}
	if len(doc.Images) > 0 {
		item.Image = doc.Images[0]
	}
	if len(doc.QuantityDistribution) > 0 {
		item.UnitPrice = float64(doc.QuantityDistribution[0].Price)
	}
	return item, nil
}
