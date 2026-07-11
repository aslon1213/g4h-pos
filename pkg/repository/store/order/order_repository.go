// Package order is the repository for storefront orders (the `orders` table plus
// its `order_items` child table). It owns order persistence and status
// transitions.
package order

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// OrderRepository owns the orders/order_items tables.
type OrderRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *OrderRepository {
	return &OrderRepository{db: db}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint (23505)
// violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Create persists a new order (row + line items atomically), assigning its id,
// number, status and timestamps.
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

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := gorm.G[models.Order](tx).Create(ctx, order); err != nil {
			if isUniqueViolation(err) {
				return repoerr.ErrConflict
			}
			return err
		}
		if len(order.Items) > 0 {
			items := make([]models.OrderItem, len(order.Items))
			copy(items, order.Items)
			for i := range items {
				items[i].ID = 0 // let the DB assign the bigserial id
				items[i].OrderID = order.ID
			}
			if err := gorm.G[models.OrderItem](tx).CreateInBatches(ctx, &items, len(items)); err != nil {
				return err
			}
			copy(order.Items, items)
		}
		return nil
	})
	if err != nil {
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
	q := gorm.G[models.Order](r.db).Where("customer_id = ?", customerID)
	total, err := q.Count(ctx, "*")
	if err != nil {
		return nil, 0, err
	}
	orders, err := q.Order("created_at DESC").Offset((page - 1) * count).Limit(count).Find(ctx)
	if err != nil {
		return nil, 0, err
	}
	for i := range orders {
		r.attachItems(ctx, &orders[i])
	}
	return orders, total, nil
}

// GetByID returns one order owned by the customer, or repoerr.ErrNotFound.
func (r *OrderRepository) GetByID(ctx context.Context, customerID, orderID string) (*models.Order, error) {
	order, err := gorm.G[models.Order](r.db).Where("id = ? AND customer_id = ?", orderID, customerID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.attachItems(ctx, &order)
	return &order, nil
}

// attachItems loads an order's line items (best-effort).
func (r *OrderRepository) attachItems(ctx context.Context, order *models.Order) {
	items, err := gorm.G[models.OrderItem](r.db).Where("order_id = ?", order.ID).Order("id").Find(ctx)
	if err == nil {
		order.Items = items
	}
	if order.Items == nil {
		order.Items = []models.OrderItem{}
	}
}

// UpdateStatus transitions an order to a new status.
func (r *OrderRepository) UpdateStatus(ctx context.Context, customerID, orderID string, status models.OrderStatus) (*models.Order, error) {
	res := r.db.WithContext(ctx).Model(&models.Order{}).
		Where("id = ? AND customer_id = ?", orderID, customerID).
		Updates(map[string]interface{}{"status": status, "updated_at": time.Now()})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, repoerr.ErrNotFound
	}
	return r.GetByID(ctx, customerID, orderID)
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
