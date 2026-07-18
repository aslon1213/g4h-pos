// Package finance is the repository for the admin finance domain (per-branch
// balances in the `branch_finance` table). It owns all gorm/Postgres access for
// the finance controller, mirroring the other ported repositories.
package finance

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// FinanceRepository owns the branch_finance (and branches) tables.
type FinanceRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *FinanceRepository {
	return &FinanceRepository{db: db}
}

// GetAllBranches returns every branch finance row (possibly empty).
func (r *FinanceRepository) GetAllBranches(ctx context.Context) ([]models.BranchFinance, error) {
	return gorm.G[models.BranchFinance](r.db).Find(ctx)
}

// GetByBranchID returns one branch by its branch id, or repoerr.ErrNotFound.
func (r *FinanceRepository) GetByBranchID(ctx context.Context, branchID string) (*models.BranchFinance, error) {
	branch, err := gorm.G[models.BranchFinance](r.db).Where("branch_id = ?", branchID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &branch, nil
}

// GetByBranchName returns one branch by name (case-insensitive exact match), or
// repoerr.ErrNotFound. The caller is expected to have already URL-decoded name.
func (r *FinanceRepository) GetByBranchName(ctx context.Context, name string) (*models.BranchFinance, error) {
	branch, err := gorm.G[models.BranchFinance](r.db).Where("branch_name ILIKE ?", name).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &branch, nil
}

// GetByObjectID has no Postgres analog (there is no Mongo hex _id column). It is
// retained as an alias of GetByBranchID so the controller's legacy /finance/id/{id}
// route keeps working against the branch id.
func (r *FinanceRepository) GetByObjectID(ctx context.Context, id string) (*models.BranchFinance, error) {
	return r.GetByBranchID(ctx, id)
}

// CreateBranch creates a new branch: it inserts BOTH a branches row (id + name)
// and a branch_finance row (zeroed balances) atomically under a freshly generated
// branch id. A duplicate branch name surfaces as the raw write error (mapped to
// HTTP 500 by the controller), matching the documented behaviour.
func (r *FinanceRepository) CreateBranch(ctx context.Context, in models.NewBranchFinanceInput) (*models.BranchFinance, error) {
	branchID := uuid.New().String()
	branch := models.BranchFinance{
		BranchID:   branchID,
		BranchName: in.BranchName,
		Details:    in.Details,
		Finance: models.Finance{
			Balance:       models.Balance{Cash: 0, Bank: 0, Terminal: 0, MobileApps: 0},
			TotalIncome:   0,
			TotalExpenses: 0,
			Debt:          0,
		},
		Suppliers: []string{},
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		// branches row: the FK target for branch_finance.branch_id.
		if err := tx.WithContext(ctx).Table("branches").Create(map[string]interface{}{
			"id":   branchID,
			"name": in.BranchName,
		}).Error; err != nil {
			return err
		}
		// branch_finance row.
		if err := gorm.G[models.BranchFinance](tx).Create(ctx, &branch); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	log.Debug().Str("branch_id", branch.BranchID).Msg("Created new branch finance")
	return &branch, nil
}
