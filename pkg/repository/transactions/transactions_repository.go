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

// GetDetails returns a transaction together with its type-specific detail — the
// cart and its items for a sale, the supplier for a supplier transaction, the
// BNPL record for a BNPL one. This is what backs the staff drill-in from a
// journal operation.
//
// A type with no detail record of its own (salary, rent, utilities, other)
// resolves to a Details with only Kind set; so does a sale with no cart (a keyed
// amount, or one whose cart was hard-deleted). Neither is an error — the
// transaction itself is still the answer, there is simply nothing to expand.
//
// A dangling reference (the detail row was deleted out from under the FK) is
// likewise left nil rather than failing the read: staff should still be able to
// open the transaction and see its money.
func (r *TransactionsRepository) GetDetails(ctx context.Context, id string) (*models.TransactionWithDetails, error) {
	transaction, err := r.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	out := &models.TransactionWithDetails{
		Transaction: *transaction,
		Details:     models.TransactionDetails{Kind: transaction.Type},
	}

	switch transaction.Type {
	case models.InitiatorTypeSales:
		if transaction.CartID == nil || *transaction.CartID == "" {
			return out, nil
		}
		cart, err := gorm.G[models.SaleCart](r.db).Where("id = ?", *transaction.CartID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		items, err := gorm.G[models.SaleCartItem](r.db).
			Where("cart_id = ?", cart.ID).Order("id").Find(ctx)
		if err != nil {
			return nil, err
		}
		if items == nil {
			items = []models.SaleCartItem{}
		}
		cart.Items = items
		out.Details.Cart = &cart
		// Keep ItemCount honest on this surface too. It is normally filled by the
		// journals repository for list views; leaving it zero here while returning
		// a populated cart would have the two fields contradict each other.
		out.ItemCount = len(items)

	case models.InitiatorTypeSupplier:
		if transaction.SupplierID == nil || *transaction.SupplierID == "" {
			return out, nil
		}
		supplier, err := gorm.G[models.Supplier](r.db).Where("id = ?", *transaction.SupplierID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out.Details.Supplier = &supplier

	case models.InitiatorTypeBNPL:
		if transaction.BNPLID == nil || *transaction.BNPLID == "" {
			return out, nil
		}
		bnpl, err := gorm.G[models.BNPL](r.db).Where("id = ?", *transaction.BNPLID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return out, nil
		}
		if err != nil {
			return nil, err
		}
		out.Details.BNPL = &bnpl
	}

	return out, nil
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
