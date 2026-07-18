// Package customers_repository is the repository for the admin customers domain.
// Under Postgres a customer's BNPLs are no longer embedded — they live in the
// `bnpls` table (with products in `bnpl_products` and payments in
// `transactions`). This repository owns every gorm/Postgres interaction for the
// customers controller, mirroring the other ported repositories: the controller
// holds a *CustomersRepository and its handlers call these methods instead of
// touching the database directly.
package customers_repository

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

// ErrActiveBNPL is returned by Delete when the customer still has an active BNPL
// and therefore may not be removed. The controller maps it to HTTP 400.
var ErrActiveBNPL = errors.New("cannot delete customer with active BNPL transactions")

// Sort modes for ListParams.Sort (mirrors the controller's query enum).
const (
	SortByActiveBNPLDesc = "max"
	SortByActiveBNPLAsc  = "min"
	SortByActiveBNPLNone = "none"
)

// ListParams captures the filters / sort / pagination for a customer listing.
type ListParams struct {
	Name    string
	Phone   string
	Address string
	Sort    string
	Page    int
	Count   int
}

// CustomersRepository owns the customers table (and reads bnpls to derive each
// customer's BNPLs and the active-BNPL total used for sorting).
type CustomersRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *CustomersRepository {
	return &CustomersRepository{db: db}
}

// Find returns a page of customers, sorted by their total active-BNPL amount,
// with each customer's BNPLs attached. The second return value is the total
// number of customers in the table (unfiltered), used by the controller to
// compute the page count — mirroring the original aggregation.
func (r *CustomersRepository) Find(ctx context.Context, p ListParams) ([]models.Customer, int64, error) {
	if p.Count <= 0 {
		p.Count = 10
	}
	if p.Page <= 0 {
		p.Page = 1
	}

	var total int64
	if err := r.db.WithContext(ctx).Model(&models.Customer{}).Count(&total).Error; err != nil {
		return nil, 0, err
	}

	// active_bnpl_total = sum of total_amount over the customer's active BNPLs.
	q := r.db.WithContext(ctx).
		Model(&models.Customer{}).
		Select("customers.*, COALESCE(SUM(bnpls.total_amount) FILTER (WHERE bnpls.status = ?), 0) AS active_bnpl_total", string(models.BNPLStatusActive)).
		Joins("LEFT JOIN bnpls ON bnpls.customer_id = customers.id").
		Group("customers.id")

	if p.Name != "" {
		q = q.Where("customers.name ILIKE ?", "%"+p.Name+"%")
	}
	if p.Phone != "" {
		q = q.Where("customers.phone ILIKE ?", "%"+p.Phone+"%")
	}
	if p.Address != "" {
		q = q.Where("customers.address ILIKE ?", "%"+p.Address+"%")
	}

	switch p.Sort {
	case SortByActiveBNPLAsc:
		q = q.Order("active_bnpl_total ASC")
	case SortByActiveBNPLNone:
		// no sort stage
	default:
		q = q.Order("active_bnpl_total DESC")
	}

	q = q.Limit(p.Count).Offset((p.Page - 1) * p.Count)

	customers := make([]models.Customer, 0)
	if err := q.Find(&customers).Error; err != nil {
		return nil, 0, err
	}

	for i := range customers {
		bnpls, err := r.loadBNPLs(ctx, customers[i].ID)
		if err != nil {
			return nil, 0, err
		}
		customers[i].BNPLs = bnpls
	}
	return customers, total, nil
}

// GetByID returns a single customer (with its BNPLs attached), or
// repoerr.ErrNotFound.
func (r *CustomersRepository) GetByID(ctx context.Context, id string) (*models.Customer, error) {
	customer, err := gorm.G[models.Customer](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	bnpls, err := r.loadBNPLs(ctx, customer.ID)
	if err != nil {
		return nil, err
	}
	customer.BNPLs = bnpls
	return &customer, nil
}

// Create inserts a new customer. A pre-existing customer with the same phone
// yields repoerr.ErrConflict.
func (r *CustomersRepository) Create(ctx context.Context, base models.CustomerBase) (*models.Customer, error) {
	if _, err := r.getByPhone(ctx, base.Phone); err == nil {
		return nil, repoerr.ErrConflict
	} else if !errors.Is(err, repoerr.ErrNotFound) {
		return nil, err
	}

	now := time.Now()
	customer := &models.Customer{
		ID:              uuid.New().String(),
		CustomerBase:    base,
		PurchaseHistory: []models.SalesSession{},
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	if err := gorm.G[models.Customer](r.db).Create(ctx, customer); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	return customer, nil
}

// Update patches the editable fields of a customer and returns the refreshed
// row (with BNPLs attached). Returns repoerr.ErrNotFound when the customer does
// not exist, or repoerr.ErrConflict when the new phone belongs to someone else.
func (r *CustomersRepository) Update(ctx context.Context, id string, base models.CustomerBase) (*models.Customer, error) {
	existing, err := gorm.G[models.Customer](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if base.Phone != existing.Phone {
		if _, perr := r.getByPhone(ctx, base.Phone); perr == nil {
			return nil, repoerr.ErrConflict
		} else if !errors.Is(perr, repoerr.ErrNotFound) {
			return nil, perr
		}
	}

	updates := map[string]interface{}{
		"updated_at": time.Now(),
		"created_at": time.Now(),
	}
	if base.Name != "" {
		updates["name"] = base.Name
	}
	if base.Phone != "" {
		updates["phone"] = base.Phone
	}
	if base.Address != "" {
		updates["address"] = base.Address
	}

	res := r.db.WithContext(ctx).Table("customers").Where("id = ?", id).Updates(updates)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, repoerr.ErrNotFound
	}

	return r.GetByID(ctx, id)
}

// Delete removes a customer. Returns ErrActiveBNPL when the customer still has an
// active BNPL, or repoerr.ErrNotFound when the customer does not exist. The
// customer's bnpls (and their bnpl_products) cascade via the foreign keys.
func (r *CustomersRepository) Delete(ctx context.Context, id string) error {
	count, err := gorm.G[models.Customer](r.db).Where("id = ?", id).Count(ctx, "id")
	if err != nil {
		return err
	}
	if count == 0 {
		return repoerr.ErrNotFound
	}

	active, err := gorm.G[models.BNPL](r.db).
		Where("customer_id = ? AND status = ?", id, string(models.BNPLStatusActive)).Count(ctx, "id")
	if err != nil {
		return err
	}
	if active > 0 {
		return ErrActiveBNPL
	}

	affected, err := gorm.G[models.Customer](r.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// getByPhone returns the customer with the given phone, or repoerr.ErrNotFound.
func (r *CustomersRepository) getByPhone(ctx context.Context, phone string) (*models.Customer, error) {
	customer, err := gorm.G[models.Customer](r.db).Where("phone = ?", phone).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &customer, nil
}

// loadBNPLs returns all BNPLs of a customer, each with its products (from
// bnpl_products) and transaction ids (from transactions) attached.
func (r *CustomersRepository) loadBNPLs(ctx context.Context, customerID string) ([]models.BNPL, error) {
	bnpls, err := gorm.G[models.BNPL](r.db).Where("customer_id = ?", customerID).Order("created_at").Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range bnpls {
		products, err := gorm.G[models.BnplProduct](r.db).Where("bnpl_id = ?", bnpls[i].ID).Find(ctx)
		if err != nil {
			return nil, err
		}
		bnpls[i].Products = make(map[string]models.SalesSessionItem, len(products))
		for _, p := range products {
			bnpls[i].Products[p.ProductID] = models.SalesSessionItem{Quantity: p.Quantity, Price: p.Price}
		}

		txns, err := gorm.G[models.Transaction](r.db).Select("id").Where("bnpl_id = ?", bnpls[i].ID).Order("created_at").Find(ctx)
		if err != nil {
			return nil, err
		}
		bnpls[i].Transactions = make([]string, 0, len(txns))
		for _, t := range txns {
			bnpls[i].Transactions = append(bnpls[i].Transactions, t.ID)
		}
	}
	return bnpls, nil
}
