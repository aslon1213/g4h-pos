// Package transactions is the repository for the admin transactions domain. It
// owns every gorm/Postgres interaction with the `transactions` table, mirroring
// the other ported repositories.
package transactions

import (
	"context"
	"errors"
	"strconv"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"gorm.io/gorm"
)

// TransactionsRepository owns the transactions table.
type TransactionsRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *TransactionsRepository {
	return &TransactionsRepository{db: db}
}

// Find returns the transactions of a branch that match the query params, ordered
// newest-first and paginated. Filters:
//   - description  : case-insensitive substring (ILIKE)
//   - amount_min/max: amount >= / <=
//   - date_min/max : created_at >= / <= (a real range, unlike the Mongo original)
//   - payment_method / type_of_transaction (transaction_type) / initiator_type
//
// Pagination defaults to page 1, count 10 when unset.
func (r *TransactionsRepository) Find(ctx context.Context, branchID string, q models.TransactionQueryParams) ([]models.Transaction, error) {
	query := gorm.G[models.Transaction](r.db).Where("branch_id = ?", branchID)

	if q.Description != "" {
		query = query.Where("description ILIKE ?", "%"+q.Description+"%")
	}
	if q.AmountMin != 0 {
		query = query.Where("amount >= ?", q.AmountMin)
	}
	if q.AmountMax != 0 {
		query = query.Where("amount <= ?", q.AmountMax)
	}
	if q.PaymentMethod != "" {
		query = query.Where("payment_method = ?", q.PaymentMethod)
	}
	if q.TypeOfTransaction != "" {
		query = query.Where("transaction_type = ?", q.TypeOfTransaction)
	}
	if q.InitiatorType != "" {
		query = query.Where("initiator_type = ?", q.InitiatorType)
	}
	if !q.DateMin.IsZero() {
		query = query.Where("created_at >= ?", q.DateMin)
	}
	if !q.DateMax.IsZero() {
		query = query.Where("created_at <= ?", q.DateMax)
	}

	page, count := q.Page, q.Count
	if page == 0 {
		page = 1
	}
	if count == 0 {
		count = 10
	}

	return query.
		Order("created_at DESC").
		Limit(count).
		Offset(count * (page - 1)).
		Find(ctx)
}

// GetByID returns a single transaction, or repoerr.ErrNotFound.
func (r *TransactionsRepository) GetByID(ctx context.Context, id string) (*models.Transaction, error) {
	transaction, err := gorm.G[models.Transaction](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &transaction, nil
}

// Update patches the amount / description / initiator type of a transaction. Empty
// strings are left untouched. Returns repoerr.ErrInvalidInput when amount is not a
// number and repoerr.ErrNotFound when no transaction matches the id.
func (r *TransactionsRepository) Update(ctx context.Context, id, amount, description, initiatorType string) error {
	set := map[string]interface{}{}
	if amount != "" {
		n, err := strconv.Atoi(amount)
		if err != nil {
			return repoerr.ErrInvalidInput
		}
		set["amount"] = n
	}
	if description != "" {
		set["description"] = description
	}
	if initiatorType != "" {
		set["initiator_type"] = initiatorType
	}
	if len(set) == 0 {
		return nil
	}

	res := r.db.WithContext(ctx).Table("transactions").Where("id = ?", id).Updates(set)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}
