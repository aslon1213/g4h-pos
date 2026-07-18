// Package sales is the repository for the sales (POS checkout) domain. It owns
// the persistent sales-transaction operations the sales controllers perform,
// mirroring the other ported repositories: a controller holds a *SalesRepository
// and its handlers call these methods instead of touching the database directly.
//
// Scope note: the sales SESSION flow (pkg/models/sales-models.go) is Redis /
// cache-backed and already carries its own methods on models.SalesSession — those
// are NOT database operations and are intentionally left out of this repository.
// The only persistent operations the sales controllers run are the two
// transaction flows below: recording a sales transaction (insert + branch balance
// increment) and deleting one (lookup + delete + balance decrement). The money
// math lives in the shared pkg/repository/ledger package so every domain applies
// it identically; each flow here wraps a ledger primitive in a single gorm
// transaction so all writes commit atomically.
package sales

import (
	"context"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/ledger"
	"gorm.io/gorm"
)

// SalesRepository records sales transactions against the transactions ledger and
// the owning branch's finance balances.
type SalesRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *SalesRepository {
	return &SalesRepository{db: db}
}

// CreateTransaction records a sales transaction for a branch and increments the
// branch's matching balance bucket, atomically in a single gorm transaction. The
// transaction type is forced to credit (income) and the payment method validated
// inside ledger.ApplySalesTransaction; an unknown branch surfaces as
// repoerr.ErrNotFound.
func (r *SalesRepository) CreateTransaction(ctx context.Context, branchID string, base models.TransactionBase) (*models.Transaction, error) {
	var transaction *models.Transaction
	err := r.db.Transaction(func(tx *gorm.DB) error {
		// No cart: this endpoint records an amount typed by staff, with no item
		// list behind it.
		t, err := ledger.ApplySalesTransaction(ctx, tx, base, branchID, "")
		if err != nil {
			return err
		}
		transaction = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return transaction, nil
}

// DeleteTransaction looks up a sales transaction by id, deletes it, and reverses
// its effect on the owning branch's balance, atomically in a single gorm
// transaction. A missing transaction surfaces as repoerr.ErrNotFound.
func (r *SalesRepository) DeleteTransaction(ctx context.Context, transactionID string) (*models.Transaction, error) {
	var transaction *models.Transaction
	err := r.db.Transaction(func(tx *gorm.DB) error {
		t, err := ledger.DeleteSalesTransaction(ctx, tx, transactionID)
		if err != nil {
			return err
		}
		transaction = t
		return nil
	})
	if err != nil {
		return nil, err
	}
	return transaction, nil
}
