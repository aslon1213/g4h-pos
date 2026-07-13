// Package journals is the repository for the admin journals domain — the
// accounting core. A journal is a per-branch daily shift holding operations
// (transactions); operations may only be mutated while the shift is open.
//
// This package owns every database interaction for the journals and operations
// controllers, mirroring the other ported repositories: the controllers hold a
// *JournalsRepository and call these methods instead of touching the database
// directly. Under Postgres a journal's operations are no longer an embedded array
// — they are rows in the `transactions` table whose `journal_id` points back at
// the journal — so "load a journal with its operations" is a journal row plus a
// query of its transactions. Every multi-row write (close/reopen shift, operation
// create/update/delete) runs inside one gorm transaction so all writes commit
// atomically. The shared money math (sales/supplier ledger writes) lives in
// pkg/repository/ledger; the journal total/terminal_income/cash_left accounting
// preserves the original MongoDB behaviour.
package journals

import (
	"context"
	"errors"
	"time"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/ledger"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// JournalsRepository owns journal and transaction persistence. Journal mutations
// roll up into the journal totals and, via the ledger primitives, into branch
// finance balances and the supplier balances.
type JournalsRepository struct {
	db *gorm.DB
}

// New builds the repository. The unique (date, branch_id) journal constraint is
// enforced by the schema (journals_date_branch_idx), so no index bootstrap is
// needed here.
func New(db *gorm.DB) *JournalsRepository {
	return &JournalsRepository{db: db}
}

// GetByID returns a single journal by its id, optionally resolving its operations
// (the transactions whose journal_id equals the journal id). Returns
// repoerr.ErrNotFound when no journal matches.
func (r *JournalsRepository) GetByID(ctx context.Context, id string, fetchTransactions bool) (*models.Journal, error) {
	return r.fetchByID(ctx, r.db, id, fetchTransactions)
}

// fetchByID loads a journal row and, when fetchTransactions is set, attaches its
// operations (transactions with journal_id = id). The db handle may be the base
// connection or a transaction handle so callers can reuse it inside a tx.
func (r *JournalsRepository) fetchByID(ctx context.Context, db *gorm.DB, id string, fetchTransactions bool) (*models.Journal, error) {
	journal, err := gorm.G[models.Journal](db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if fetchTransactions {
		ops, err := r.loadOperations(ctx, db, id)
		if err != nil {
			return nil, err
		}
		journal.Operations = ops
	}
	return &journal, nil
}

// loadOperations returns a journal's operations (the transactions that point at
// it), ordered oldest-first so positional close/reopen logic is deterministic.
func (r *JournalsRepository) loadOperations(ctx context.Context, db *gorm.DB, journalID string) ([]models.Transaction, error) {
	ops, err := gorm.G[models.Transaction](db).Where("journal_id = ?", journalID).Order("created_at ASC").Find(ctx)
	if err != nil {
		return nil, err
	}
	return ops, nil
}

// Query returns journals matching the query params, with pagination, branch
// filtering and the optional total-value range filter, each journal's operations
// attached. Mirrors the controller's original QueryJournals.
func (r *JournalsRepository) Query(ctx context.Context, queryParams models.JournalQueryParams) ([]models.Journal, error) {
	// Seed the chain with Order so the variable is a ChainInterface and the
	// optional filters below can be appended conditionally.
	q := gorm.G[models.Journal](r.db).Order("date DESC")
	if queryParams.BranchID != "" {
		q = q.Where("branch_id = ?", queryParams.BranchID)
	}
	if queryParams.Total.Use {
		q = q.Where("total >= ? AND total <= ?", queryParams.Total.Min, queryParams.Total.Max)
	}
	q = q.Offset((queryParams.Page - 1) * queryParams.PageSize).Limit(queryParams.PageSize)

	journals, err := q.Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range journals {
		ops, err := r.loadOperations(ctx, r.db, journals[i].ID)
		if err != nil {
			return nil, err
		}
		journals[i].Operations = ops
	}
	return journals, nil
}

// resolveBranch looks the branch finance row up by name (case-insensitive) or
// branch id, mirroring the controller's NewJournalEntry lookup. Returns
// repoerr.ErrNotFound when no branch matches.
func (r *JournalsRepository) resolveBranch(ctx context.Context, branchNameOrID string) (*models.BranchFinance, error) {
	branch, err := gorm.G[models.BranchFinance](r.db).
		Where("branch_name ILIKE ? OR branch_id = ?", "%"+branchNameOrID+"%", branchNameOrID).
		First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &branch, nil
}

// Create inserts a new journal (open shift, zeroed totals) for the branch named
// or identified by input.BranchNameOrID. The date is normalised to midnight in
// the configured timezone, matching the controller. Returns repoerr.ErrNotFound
// if the branch does not exist and repoerr.ErrConflict when a journal already
// exists for that (date, branch).
func (r *JournalsRepository) Create(ctx context.Context, input models.NewJournalEntryInput) (*models.JournalWithTransactionID, error) {
	financeBranch, err := r.resolveBranch(ctx, input.BranchNameOrID)
	if err != nil {
		return nil, err
	}

	// parse the date to the timezone first and set to midnight
	loc := utils.GetTimeZone()
	input.Date = input.Date.In(loc)
	input.Date = time.Date(input.Date.Year(), input.Date.Month(), input.Date.Day(), 0, 0, 0, 0, loc)

	branch := models.Branch_names[input.BranchNameOrID]

	base := models.JournalBase{
		Branch: models.Branch{
			Name:     branch.Name,
			Location: branch.Location,
			Phone:    branch.Phone,
			ID:       financeBranch.BranchID,
		},
		Date:            input.Date,
		Shift_is_closed: false,
		Terminal_income: 0,
		Cash_left:       0,
		Total:           0,
		ID:              uuid.New().String(),
	}

	// Insert through models.Journal so the "journals" table name is used; the
	// Operations field is gorm:"-" and therefore ignored on write.
	journal := models.Journal{JournalBase: base, Operations: []models.Transaction{}}
	if err := gorm.G[models.Journal](r.db).Create(ctx, &journal); err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}

	return &models.JournalWithTransactionID{JournalBase: base, Operations: []string{}}, nil
}

// Close closes a journal shift inside one gorm transaction: it creates the
// cash-left and terminal-income closing transactions (sales credits on the
// branch), attaches them to the journal (journal_id) so they become operations,
// and stamps the shift as closed with the new total. Money semantics are
// preserved exactly: total = previous total + cash_left + terminal_income.
func (r *JournalsRepository) Close(ctx context.Context, journalID string, input models.CloseJournalEntryInput) (*models.Journal, error) {
	var result *models.Journal

	err := r.db.Transaction(func(tx *gorm.DB) error {
		journal, err := r.fetchByID(ctx, tx, journalID, true)
		if err != nil {
			return err
		}
		branchID := journal.Branch.ID

		cashBase := models.TransactionBase{
			Amount:        input.CashLeft,
			Description:   "Cash left at the end of the day",
			PaymentMethod: models.PaymentMethodCash,
			Type:          models.TransactionTypeCredit,
		}
		terminalBase := models.TransactionBase{
			Amount:        input.TerminalIncome,
			Description:   "Terminal income at the end of the day",
			PaymentMethod: models.PaymentMethodTerminal,
			Type:          models.TransactionTypeCredit,
		}

		terminalTx, err := ledger.ApplySalesTransaction(ctx, tx, terminalBase, branchID)
		if err != nil {
			return err
		}
		cashTx, err := ledger.ApplySalesTransaction(ctx, tx, cashBase, branchID)
		if err != nil {
			return err
		}

		// Attach the two closing transactions to the journal (replaces the Mongo
		// $push onto operations) so they are loaded as operations on re-fetch and
		// can be identified on reopen.
		if _, err := gorm.G[models.Transaction](tx).
			Where("id IN ?", []string{terminalTx.ID, cashTx.ID}).
			Update(ctx, "journal_id", journalID); err != nil {
			return err
		}

		newTotal := journal.Total + input.CashLeft + input.TerminalIncome
		res := tx.Table("journals").Where("id = ?", journalID).Updates(map[string]interface{}{
			"cash_left":       input.CashLeft,
			"terminal_income": input.TerminalIncome,
			"shift_is_closed": true,
			"total":           newTotal,
		})
		if res.Error != nil {
			return res.Error
		}

		journal.Total = newTotal
		journal.Cash_left = input.CashLeft
		journal.Terminal_income = input.TerminalIncome
		result = journal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// ReOpen reverses a Close inside one gorm transaction: it deletes the two most
// recent (terminal, cash) closing sales transactions — reversing their branch
// balance effect via the ledger — reverts the journal's
// shift_is_closed/cash_left/terminal_income/total, and returns the freshly
// re-fetched journal. Mirrors the controller's "delete the last two operations"
// logic.
func (r *JournalsRepository) ReOpen(ctx context.Context, journalID string) (*models.Journal, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		journal, err := gorm.G[models.Journal](tx).Where("id = ?", journalID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return repoerr.ErrNotFound
		}
		if err != nil {
			return err
		}

		ops, err := r.loadOperations(ctx, tx, journalID)
		if err != nil {
			return err
		}
		if len(ops) < 2 {
			// Not a closed shift (nothing to reverse). The Mongo original would
			// have panicked on the missing operations; fail cleanly instead.
			return repoerr.ErrInvalidInput
		}

		// delete the two closing transactions (last two by creation order)
		if _, err := ledger.DeleteSalesTransaction(ctx, tx, ops[len(ops)-1].ID); err != nil {
			return err
		}
		if _, err := ledger.DeleteSalesTransaction(ctx, tx, ops[len(ops)-2].ID); err != nil {
			return err
		}

		newTotal := journal.Total - (journal.Cash_left + journal.Terminal_income)
		res := tx.Table("journals").Where("id = ?", journalID).Updates(map[string]interface{}{
			"shift_is_closed": false,
			"cash_left":       0,
			"terminal_income": 0,
			"total":           newTotal,
		})
		if res.Error != nil {
			return res.Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	return r.fetchByID(ctx, r.db, journalID, true)
}

// AddOperation creates a new operation transaction (a sales credit, or a supplier
// transaction when input.SupplierTransaction is set), attaches it to the journal
// and rolls its amount up onto the journal total, inside one gorm transaction.
// Returns the journal fetched at the start (matching the controller, which
// responds with the pre-update journal value). input.Type is forced to credit in
// both arms, preserving the original invariant.
func (r *JournalsRepository) AddOperation(ctx context.Context, journalID string, input models.JournalOperationInput) (*models.Journal, error) {
	var result *models.Journal
	err := r.db.Transaction(func(tx *gorm.DB) error {
		journal, err := r.AddOperationTx(ctx, tx, journalID, input)
		if err != nil {
			return err
		}
		result = journal
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

// AddOperationTx is AddOperation's body run against an existing transaction
// handle, so a caller (e.g. the Scan & Go handoff checkout) can record a sale
// operation and its own writes in the SAME gorm transaction — mirroring how the
// ledger primitives take a tx. It records the operation, links it to the journal
// and rolls its amount onto the journal total, and returns the journal fetched
// at the start. No new accounting logic: the behavior is identical to the
// original single-transaction AddOperation.
func (r *JournalsRepository) AddOperationTx(ctx context.Context, tx *gorm.DB, journalID string, input models.JournalOperationInput) (*models.Journal, error) {
	journal, err := r.fetchByID(ctx, tx, journalID, true)
	if err != nil {
		return nil, err
	}

	var operation *models.Transaction
	if !input.SupplierTransaction {
		input.TransactionBase.Type = models.TransactionTypeCredit
		operation, err = ledger.ApplySalesTransaction(ctx, tx, input.TransactionBase, journal.Branch.ID)
		if err != nil {
			return nil, err
		}
	} else {
		input.TransactionBase.Type = models.TransactionTypeCredit
		if input.SupplierID == "" {
			return nil, repoerr.ErrInvalidInput
		}
		operation, err = ledger.ApplySupplierTransaction(ctx, tx, input.TransactionBase, input.SupplierID, journal.Branch.ID)
		if err != nil {
			return nil, err
		}
	}

	// attach the new transaction to the journal (replaces the Mongo $push)
	if _, err := gorm.G[models.Transaction](tx).
		Where("id = ?", operation.ID).
		Update(ctx, "journal_id", journalID); err != nil {
		return nil, err
	}

	// roll the operation amount up onto the journal total
	res := tx.Table("journals").Where("id = ?", journalID).
		Update("total", gorm.Expr("total + ?", input.Amount))
	if res.Error != nil {
		return nil, res.Error
	}

	return journal, nil
}

// UpdateOperation edits the amount (and optionally description) of an operation
// on an already-loaded journal, inside one gorm transaction: the transaction row
// and the journal total are adjusted by the diff (old amount - new amount),
// preserving the controller's accounting exactly. The passed-in journal is
// mutated in place (total and operations) and returned. Returns
// repoerr.ErrInvalidInput for a non-positive amount and repoerr.ErrNotFound when
// the operation is not on the journal.
func (r *JournalsRepository) UpdateOperation(ctx context.Context, journal *models.Journal, operationID string, amount int, description string) (*models.Journal, error) {
	if amount <= 0 {
		return nil, repoerr.ErrInvalidInput
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		for _, operation := range journal.Operations {
			if operation.ID != operationID {
				continue
			}

			// update the transaction row
			set := map[string]interface{}{
				"amount":     uint32(amount),
				"updated_at": time.Now(),
			}
			if description != "" {
				set["description"] = description
			}
			if res := tx.Table("transactions").Where("id = ?", operation.ID).Updates(set); res.Error != nil {
				return res.Error
			}

			// adjust the journal total by the diff (old - new)
			diff := int32(int(operation.Amount) - amount)
			if res := tx.Table("journals").Where("id = ?", journal.ID).
				Update("total", gorm.Expr("total + ?", -diff)); res.Error != nil {
				return res.Error
			}

			// NOTE: the Mongo original also incremented a branch-finance "total"
			// field; the Postgres branch_finance table has no such column (it was
			// dead write-only data never read back), so that write is dropped.

			newOperation := models.Transaction{
				ID: operation.ID,
				TransactionBase: models.TransactionBase{
					Amount:      uint32(amount),
					Description: operation.Description,
				},
				CreatedAt: operation.CreatedAt,
				UpdatedAt: time.Now(),
			}
			if description != "" {
				newOperation.Description = description
			}
			journal.Total -= uint32(diff)
			journal.Operations = utils.ReplaceElement(journal.Operations, operation, newOperation)
			return nil
		}
		return repoerr.ErrNotFound
	})
	if err != nil {
		return nil, err
	}
	return journal, nil
}

// DeleteOperation removes an operation from an already-loaded journal inside one
// gorm transaction: it deletes the transaction row (which also clears its
// journal_id link) and decrements the journal total by the operation amount. The
// passed-in journal is mutated in place (operations and total) and returned.
// Returns repoerr.ErrNotFound when the operation is not on the journal.
func (r *JournalsRepository) DeleteOperation(ctx context.Context, journal *models.Journal, operationID string) (*models.Journal, error) {
	err := r.db.Transaction(func(tx *gorm.DB) error {
		for _, operation := range journal.Operations {
			if operation.ID != operationID {
				continue
			}

			// delete the transaction row (removes it from the journal's operations)
			if _, err := gorm.G[models.Transaction](tx).Where("id = ?", operation.ID).Delete(ctx); err != nil {
				return err
			}

			// NOTE: the Mongo original also decremented a branch-finance "total"
			// field; the Postgres branch_finance table has no such column, so that
			// write is dropped (see UpdateOperation).

			// decrement the journal total by the operation amount
			if res := tx.Table("journals").Where("id = ?", journal.ID).
				Update("total", gorm.Expr("total - ?", int32(operation.Amount))); res.Error != nil {
				return res.Error
			}

			journal.Operations = utils.RemoveElement(journal.Operations, operation)
			journal.Total -= operation.Amount
			return nil
		}
		return repoerr.ErrNotFound
	})
	if err != nil {
		return nil, err
	}
	return journal, nil
}
