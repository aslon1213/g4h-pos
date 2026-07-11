// Package suppliers is the repository for the admin suppliers domain. It owns
// every gorm/Postgres interaction for suppliers, mirroring the other ported
// repositories: the controller holds a *SuppliersRepository and its handlers call
// these methods instead of touching the database directly.
package suppliers

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/ledger"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// SuppliersRepository owns the suppliers table. It also reads branch_finance to
// resolve branches; supplier-transaction money math lives in the ledger package.
type SuppliersRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *SuppliersRepository {
	return &SuppliersRepository{db: db}
}

// resolveBranchID looks a branch up by id or name in branch_finance and returns
// its branch id, or repoerr.ErrNotFound when no branch matches.
func (r *SuppliersRepository) resolveBranchID(ctx context.Context, branchOrName string) (string, error) {
	branch, err := gorm.G[models.BranchFinance](r.db).
		Where("branch_id = ? OR branch_name = ?", branchOrName, branchOrName).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", repoerr.ErrNotFound
	}
	if err != nil {
		return "", err
	}
	return branch.BranchID, nil
}

// Find returns suppliers matching the query. A branch filter (id or name) is
// resolved to a branch id first; an unknown branch yields repoerr.ErrNotFound.
func (r *SuppliersRepository) Find(ctx context.Context, q models.SupplierQueryParams) ([]models.Supplier, error) {
	// Seed the chain with an always-true predicate so `query` is a ChainInterface
	// from the start and the optional filters below can be reassigned onto it.
	query := gorm.G[models.Supplier](r.db).Where("1 = 1")
	if q.Name != "" {
		query = query.Where("name = ?", q.Name)
	}
	if q.INN != "" {
		query = query.Where("inn = ?", q.INN)
	}
	if q.Branch != "" {
		branchID, err := r.resolveBranchID(ctx, q.Branch)
		if err != nil {
			return nil, err
		}
		query = query.Where("branch_id = ?", branchID)
	}
	if q.Email != "" {
		query = query.Where("email = ?", q.Email)
	}
	if q.Phone != "" {
		query = query.Where("phone = ?", q.Phone)
	}
	if q.Address != "" {
		query = query.Where("address = ?", q.Address)
	}
	if q.Notes != "" {
		query = query.Where("notes = ?", q.Notes)
	}
	return query.Find(ctx)
}

// GetByID returns a single supplier, or repoerr.ErrNotFound.
func (r *SuppliersRepository) GetByID(ctx context.Context, id string) (*models.Supplier, error) {
	supplier, err := gorm.G[models.Supplier](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &supplier, nil
}

// Create inserts a new supplier (with zeroed financial data) under the branch
// named/identified by base.Branch. Returns repoerr.ErrNotFound if the branch does
// not exist. (Under Postgres there is no finance.suppliers[] array to maintain.)
func (r *SuppliersRepository) Create(ctx context.Context, base models.SupplierBase) (*models.Supplier, error) {
	branchID, err := r.resolveBranchID(ctx, base.Branch)
	if err != nil {
		return nil, err
	}

	now := time.Now().In(utils.GetTimeZone())
	base.Branch = branchID // associate with the resolved branch id
	supplier := models.Supplier{
		SupplierBase: base,
		ID:           uuid.New().String(),
		FinancialData: models.FinancialData{
			Balance:       0,
			Transactions:  []models.Transaction{},
			TotalIncome:   0,
			TotalExpenses: 0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}

	if err := gorm.G[models.Supplier](r.db).Create(ctx, &supplier); err != nil {
		return nil, err
	}
	return &supplier, nil
}

// Update patches the editable fields of a supplier. Returns repoerr.ErrNotFound
// when no supplier matches the id.
func (r *SuppliersRepository) Update(ctx context.Context, id string, base models.SupplierBase) error {
	set := map[string]interface{}{"updated_at": time.Now().In(utils.GetTimeZone())}
	if base.Email != "" {
		set["email"] = base.Email
	}
	if base.INN != "" {
		set["inn"] = base.INN
	}
	if base.Name != "" {
		set["name"] = base.Name
	}
	if base.Address != "" {
		set["address"] = base.Address
	}
	if base.Phone != "" {
		set["phone"] = base.Phone
	}
	if base.Notes != "" {
		set["notes"] = base.Notes
	}

	res := r.db.WithContext(ctx).Table("suppliers").Where("id = ?", id).Updates(set)
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// Delete removes a supplier. Returns repoerr.ErrNotFound when it does not exist.
func (r *SuppliersRepository) Delete(ctx context.Context, id string) error {
	affected, err := gorm.G[models.Supplier](r.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// CreateTransaction records a supplier transaction and reflects it on both the
// supplier's balances and the branch's finance balances, atomically in a single
// gorm transaction. The money math lives in ledger.ApplySupplierTransaction.
// Returns repoerr.ErrNotFound if the supplier does not exist.
func (r *SuppliersRepository) CreateTransaction(ctx context.Context, branchID, supplierID string, base models.TransactionBase) (*models.Transaction, error) {
	var transaction *models.Transaction
	err := r.db.Transaction(func(tx *gorm.DB) error {
		t, err := ledger.ApplySupplierTransaction(ctx, tx, base, supplierID, branchID)
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
