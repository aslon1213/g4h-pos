// Package wishlist is the repository for the customer's saved-for-later list
// (the `wishlists` collection, one document per customer).
package wishlist

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

// WishlistRepository owns the wishlists collection and reads products for snapshots.
type WishlistRepository struct {
	wishlists *mongo.Collection
	products  *mongo.Collection
}

// New builds the repository and ensures the unique customer index exists.
func New(db *mongo.Database) *WishlistRepository {
	wishlists := db.Collection("wishlists")
	_, _ = wishlists.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "customer_id", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &WishlistRepository{wishlists: wishlists, products: db.Collection("products")}
}

// GetByCustomer returns the customer's wishlist, creating an empty one on first use.
func (r *WishlistRepository) GetByCustomer(ctx context.Context, customerID string) (*models.Wishlist, error) {
	wl := &models.Wishlist{}
	err := r.wishlists.FindOne(ctx, bson.M{"customer_id": customerID}).Decode(wl)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return r.ensure(ctx, customerID)
	}
	if err != nil {
		return nil, err
	}
	return wl, nil
}

func (r *WishlistRepository) ensure(ctx context.Context, customerID string) (*models.Wishlist, error) {
	now := time.Now()
	wl := &models.Wishlist{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		Items:      []models.WishlistItem{},
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	opts := options.UpdateOne().SetUpsert(true)
	if _, err := r.wishlists.UpdateOne(ctx,
		bson.M{"customer_id": customerID},
		bson.M{"$setOnInsert": wl},
		opts,
	); err != nil {
		return nil, err
	}
	return r.GetByCustomer(ctx, customerID)
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
	_, err = r.wishlists.UpdateOne(ctx,
		bson.M{"customer_id": customerID},
		bson.M{
			"$push": bson.M{"items": item},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	if err != nil {
		return nil, err
	}
	return r.GetByCustomer(ctx, customerID)
}

// RemoveItem removes a product from the wishlist.
func (r *WishlistRepository) RemoveItem(ctx context.Context, customerID, productID string) (*models.Wishlist, error) {
	res, err := r.wishlists.UpdateOne(ctx,
		bson.M{"customer_id": customerID},
		bson.M{
			"$pull": bson.M{"items": bson.M{"product_id": productID}},
			"$set":  bson.M{"updated_at": time.Now()},
		},
	)
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, repoerr.ErrNotFound
	}
	return r.GetByCustomer(ctx, customerID)
}

func (r *WishlistRepository) productSnapshot(ctx context.Context, productID string) (*models.WishlistItem, error) {
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
	item := &models.WishlistItem{ProductID: productID, Name: doc.Name, AddedAt: time.Now()}
	if len(doc.Images) > 0 {
		item.Image = doc.Images[0]
	}
	if len(doc.QuantityDistribution) > 0 {
		item.Price = float64(doc.QuantityDistribution[0].Price)
	}
	return item, nil
}
