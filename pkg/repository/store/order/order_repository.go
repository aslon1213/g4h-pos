// Package order is the repository for storefront orders (the `orders`
// collection). It owns order persistence and status transitions.
package order

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// OrderRepository owns the orders collection.
type OrderRepository struct {
	coll *mongo.Collection
}

// New builds the repository and ensures lookup indexes exist.
func New(db *mongo.Database) *OrderRepository {
	coll := db.Collection("orders")
	_, _ = coll.Indexes().CreateMany(context.Background(), []mongo.IndexModel{
		{Keys: bson.D{{Key: "customer_id", Value: 1}, {Key: "created_at", Value: -1}}},
		{Keys: bson.D{{Key: "number", Value: 1}}, Options: options.Index().SetUnique(true)},
	})
	return &OrderRepository{coll: coll}
}

// Create persists a new order, assigning its id, number, status and timestamps.
func (r *OrderRepository) Create(ctx context.Context, order *models.Order) (*models.Order, error) {
	now := time.Now()
	order.ID = uuid.New().String()
	order.Number = newOrderNumber(order.ID)
	if order.Status == "" {
		order.Status = models.OrderStatusPending
	}
	order.CreatedAt = now
	order.UpdatedAt = now
	if order.Items == nil {
		order.Items = []models.OrderItem{}
	}
	if _, err := r.coll.InsertOne(ctx, order); err != nil {
		return nil, err
	}
	return order, nil
}

// ListByCustomer returns the customer's orders, newest first, paginated.
func (r *OrderRepository) ListByCustomer(ctx context.Context, customerID string, page, count int) ([]models.Order, int64, error) {
	if page < 1 {
		page = 1
	}
	if count < 1 {
		count = 20
	}
	filter := bson.M{"customer_id": customerID}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetSkip(int64((page - 1) * count)).
		SetLimit(int64(count))
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, err
	}
	orders := []models.Order{}
	if err := cursor.All(ctx, &orders); err != nil {
		return nil, 0, err
	}
	return orders, total, nil
}

// GetByID returns one order owned by the customer, or repoerr.ErrNotFound.
func (r *OrderRepository) GetByID(ctx context.Context, customerID, orderID string) (*models.Order, error) {
	order := &models.Order{}
	err := r.coll.FindOne(ctx, bson.M{"_id": orderID, "customer_id": customerID}).Decode(order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return order, nil
}

// UpdateStatus transitions an order to a new status.
func (r *OrderRepository) UpdateStatus(ctx context.Context, customerID, orderID string, status models.OrderStatus) (*models.Order, error) {
	order := &models.Order{}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.coll.FindOneAndUpdate(ctx,
		bson.M{"_id": orderID, "customer_id": customerID},
		bson.M{"$set": bson.M{"status": status, "updated_at": time.Now()}},
		opts,
	).Decode(order)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return order, nil
}

// Cancel marks a pending/confirmed order cancelled. Returns repoerr.ErrInvalidInput
// if the order is already shipped/delivered/cancelled.
func (r *OrderRepository) Cancel(ctx context.Context, customerID, orderID string) (*models.Order, error) {
	order, err := r.GetByID(ctx, customerID, orderID)
	if err != nil {
		return nil, err
	}
	if order.Status != models.OrderStatusPending && order.Status != models.OrderStatusConfirmed {
		return nil, repoerr.ErrInvalidInput
	}
	return r.UpdateStatus(ctx, customerID, orderID, models.OrderStatusCancelled)
}

// newOrderNumber derives a short, human-friendly number from the order uuid.
func newOrderNumber(id string) string {
	short := strings.ReplaceAll(id, "-", "")
	if len(short) > 10 {
		short = short[:10]
	}
	return "ORD-" + strings.ToUpper(short)
}
