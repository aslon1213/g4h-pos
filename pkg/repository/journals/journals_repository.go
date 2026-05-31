// Package journals is the repository for the admin journals domain — the
// accounting core. A journal is a per-branch daily shift holding operations
// (transactions); operations may only be mutated while the shift is open.
//
// This package owns every mongo/bson interaction for the journals and
// operations controllers, mirroring the suppliers/finance repositories: the
// controllers hold a *JournalsRepository and call these methods instead of
// touching collections directly. Multi-document writes (close/reopen shift,
// operation create/update/delete) run inside MongoDB transactions and therefore
// require a replica set; the session management is replicated here exactly as it
// lived in the controllers, with every filter, update document, $inc/$push and
// money/accounting calculation preserved byte-for-byte.
package journals

import (
	"context"
	"errors"
	"time"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	sales "github.com/aslon1213/g4h_pos_erp/pkg/repository/sales"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/suppliers"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"
	"github.com/aslon1213/g4h_pos_erp/platform/database"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// JournalsRepository owns the journals, transactions, finance and suppliers
// collections. transactions/finance are needed because journal mutations roll
// up into branch finance balances and the transactions collection; suppliers is
// needed so an operation can apply a supplier transaction (via the exported
// suppliers.NewSupplierTransaction) inside the journal's own session.
type JournalsRepository struct {
	journals     *mongo.Collection
	finance      *mongo.Collection
	transactions *mongo.Collection
	suppliers    *mongo.Collection
}

// New builds the repository and ensures the unique (date, branch) journal index
// exists, mirroring the controller constructor.
func New(db *mongo.Database) *JournalsRepository {
	journals := db.Collection("journals")
	indexModel := mongo.IndexModel{
		Keys: bson.D{
			{Key: "date", Value: -1},
			{Key: "branch", Value: -1},
		},
		Options: options.Index().SetUnique(true),
	}
	if _, err := journals.Indexes().CreateOne(context.Background(), indexModel); err != nil {
		log.Error().Err(err).Msg("Failed to create index for journals")
	}
	return &JournalsRepository{
		journals:     journals,
		finance:      db.Collection("finance"),
		transactions: db.Collection("transactions"),
		suppliers:    db.Collection("suppliers"),
	}
}

// GetByID returns a single journal by its id (hex ObjectID or raw string _id),
// optionally resolving its operations against the transactions collection.
// Returns repoerr.ErrNotFound when no journal matches.
func (r *JournalsRepository) GetByID(ctx context.Context, id string, fetchTransactions bool) (*models.Journal, error) {
	return r.fetchByID(ctx, id, fetchTransactions)
}

// fetchByID reproduces the controller's FetchJournalByID lookup pipeline. It
// accepts both an ObjectID hex and a raw string id so callers that pass either
// form resolve the same document.
func (r *JournalsRepository) fetchByID(ctx context.Context, id string, fetchTransactions bool) (*models.Journal, error) {
	journalID, err := bson.ObjectIDFromHex(id)
	if err != nil {
		return nil, repoerr.ErrInvalidInput
	}

	pipeline := mongo.Pipeline{}
	if fetchTransactions {
		pipeline = mongo.Pipeline{
			{{Key: "$match", Value: bson.D{
				{Key: "$or", Value: bson.A{
					bson.D{{Key: "_id", Value: journalID}},
					bson.D{{Key: "_id", Value: id}},
				}},
			}}},
			{
				{Key: "$lookup", Value: bson.M{
					"from":         "transactions",
					"localField":   "operations",
					"foreignField": "_id",
					"as":           "transactions",
				}},
			},
			{
				{Key: "$project", Value: bson.M{
					"date":            1,
					"total":           1,
					"shift_is_closed": 1,
					"terminal_income": 1,
					"cash_left":       1,
					"branch":          1,
					"operations":      "$transactions",
				}},
			},
		}
	} else {
		pipeline = mongo.Pipeline{
			{{Key: "$match", Value: bson.D{{Key: "_id", Value: journalID}}}},
		}
	}

	cursor, err := r.journals.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.Journal
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, repoerr.ErrNotFound
	}
	return &results[0], nil
}

// Query returns journals matching the query params, with pagination, branch
// filtering and the optional total-value range filter, resolving operations
// against the transactions collection. Mirrors the controller's QueryJournals.
func (r *JournalsRepository) Query(ctx context.Context, queryParams models.JournalQueryParams) ([]models.Journal, error) {
	match := bson.M{}
	if queryParams.BranchID != "" {
		match["branch._id"] = queryParams.BranchID
	}
	if queryParams.Total.Use {
		match["total"] = bson.M{"$gte": queryParams.Total.Min, "$lte": queryParams.Total.Max}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "date", Value: -1}}}},
		{{Key: "$skip", Value: (queryParams.Page - 1) * queryParams.PageSize}},
		{{Key: "$limit", Value: queryParams.PageSize}},
		{
			{Key: "$lookup", Value: bson.M{
				"from":         "transactions",
				"localField":   "operations",
				"foreignField": "_id",
				"as":           "transactions",
			}},
		},
		{
			{Key: "$project", Value: bson.M{
				"date":            1,
				"total":           1,
				"shift_is_closed": 1,
				"terminal_income": 1,
				"cash_left":       1,
				"branch":          1,
				"operations":      "$transactions",
			}},
		},
	}

	cursor, err := r.journals.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.Journal
	if err := cursor.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// resolveBranch looks the finance branch up by name (case-insensitive regex) or
// branch id, mirroring the controller's NewJournalEntry lookup. Returns
// repoerr.ErrNotFound when no branch matches.
func (r *JournalsRepository) resolveBranch(ctx context.Context, branchNameOrID string) (*models.BranchFinance, error) {
	financeBranch := models.BranchFinance{}
	err := r.finance.FindOne(ctx, bson.M{"$or": []bson.M{
		{"branch_name": bson.M{"$regex": branchNameOrID, "$options": "i"}},
		{"branch_id": branchNameOrID},
	}}).Decode(&financeBranch)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &financeBranch, nil
}

// Create inserts a new journal (open shift, zeroed totals) for the branch named
// or identified by input.BranchNameOrID. The date is normalised to midnight in
// the configured timezone, matching the controller. Returns repoerr.ErrNotFound
// if the branch does not exist.
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

	journal := models.JournalWithTransactionID{
		JournalBase: models.JournalBase{
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
			ID:              bson.NewObjectID(),
		},
		Operations: []string{},
	}

	if _, err := r.journals.InsertOne(ctx, journal); err != nil {
		return nil, err
	}
	return &journal, nil
}

// Close closes a journal shift inside a MongoDB transaction: it creates the
// cash-left and terminal-income closing transactions (sales credits on the
// branch), pushes them onto the journal's operations, and stamps the shift as
// closed with the new total. Returns the updated journal (with the looked-up
// operations and adjusted totals). Money semantics are preserved exactly from
// the controller: total = previous total + cash_left + terminal_income.
func (r *JournalsRepository) Close(ctx context.Context, journalID string, input models.CloseJournalEntryInput) (*models.Journal, error) {
	oid, err := bson.ObjectIDFromHex(journalID)
	if err != nil {
		return nil, repoerr.ErrInvalidInput
	}

	// start a db transaction
	ses, sessCtx, err := database.StartTransaction(r.journals.Database().Client())
	if err != nil {
		return nil, err
	}
	defer ses.EndSession(sessCtx)

	// get journal info
	journal, err := r.fetchByID(sessCtx, journalID, true)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}

	branchID := journal.Branch.ID

	cash_transaction_base := models.TransactionBase{
		Amount:        input.CashLeft,
		Description:   "Cash left at the end of the day",
		PaymentMethod: models.PaymentMethodCash,
		Type:          models.TransactionTypeCredit,
	}
	terminal_transaction_base := models.TransactionBase{
		Amount:        input.TerminalIncome,
		Description:   "Terminal income at the end of the day",
		PaymentMethod: models.PaymentMethodTerminal,
		Type:          models.TransactionTypeCredit,
	}
	terminal_transaction, err := sales.NewSalesTransaction(
		sessCtx,
		terminal_transaction_base,
		branchID,
		r.transactions,
		r.finance,
	)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}
	cash_transaction, err := sales.NewSalesTransaction(
		sessCtx,
		cash_transaction_base,
		branchID,
		r.transactions,
		r.finance,
	)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}

	// update the journal
	_, err = r.journals.UpdateOne(sessCtx,
		bson.M{"_id": oid},
		bson.M{
			"$set": bson.M{
				"cash_left": input.CashLeft, "terminal_income": input.TerminalIncome, "shift_is_closed": true, "total": journal.Total + input.CashLeft + input.TerminalIncome,
			},
			"$push": bson.M{"operations": bson.M{"$each": []string{terminal_transaction.ID, cash_transaction.ID}}},
		},
	)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}
	// commit the transaction
	if err := ses.CommitTransaction(sessCtx); err != nil {
		return nil, err
	}

	journal.Total = journal.Total + input.CashLeft + input.TerminalIncome
	journal.Cash_left = input.CashLeft
	journal.Terminal_income = input.TerminalIncome
	return journal, nil
}

// ReOpen reverses a Close inside a MongoDB transaction: it deletes the last two
// (cash, terminal) closing sales transactions, reverts the journal's
// shift_is_closed/cash_left/terminal_income/total and trims the two operation
// ids, then returns the freshly re-fetched journal. Logic preserved exactly from
// the controller, including operating on the last two operation entries.
func (r *JournalsRepository) ReOpen(ctx context.Context, journalID string) (*models.Journal, error) {
	oid, err := bson.ObjectIDFromHex(journalID)
	if err != nil {
		return nil, repoerr.ErrInvalidInput
	}

	// start a db transaction
	ses, sessCtx, err := database.StartTransaction(r.journals.Database().Client())
	if err != nil {
		return nil, err
	}
	defer ses.EndSession(sessCtx)

	// fetch journal Info
	journal := models.JournalWithTransactionID{}
	err = r.journals.FindOne(sessCtx, bson.M{"_id": oid}).Decode(&journal)
	if errors.Is(err, mongo.ErrNoDocuments) {
		_ = ses.AbortTransaction(sessCtx)
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}

	// delete two transactions - [cash, terminal by ID]
	_, err = sales.DeleteSalesTransaction(sessCtx, journal.Operations[len(journal.Operations)-1], r.transactions, r.finance)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}
	_, err = sales.DeleteSalesTransaction(sessCtx, journal.Operations[len(journal.Operations)-2], r.transactions, r.finance)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}
	_, err = r.journals.UpdateByID(sessCtx, journal.ID, bson.M{
		"$set": bson.M{"shift_is_closed": false, "cash_left": 0, "terminal_income": 0, "total": journal.Total - (journal.Cash_left + journal.Terminal_income), "operations": journal.Operations[:len(journal.Operations)-2]},
	})
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}
	// commit the transaction
	if err := ses.CommitTransaction(sessCtx); err != nil {
		return nil, err
	}

	journal_new, err := r.fetchByID(sessCtx, journalID, true)
	if err != nil {
		return nil, err
	}
	return journal_new, nil
}

// AddOperation creates a new operation transaction (a sales credit, or a
// supplier transaction when input.SupplierTransaction is set) for the given
// journal and rolls it up onto the journal total/operations, inside a MongoDB
// transaction. Returns the journal that was fetched at the start (matching the
// controller, which responds with the pre-update journal value). The supplier
// branch invariant — input.Type is forced to credit in both arms — is preserved.
func (r *JournalsRepository) AddOperation(ctx context.Context, journalID string, input models.JournalOperationInput) (*models.Journal, error) {
	// start a new session and transaction
	ses, sessCtx, err := database.StartTransaction(r.journals.Database().Client())
	if err != nil {
		return nil, err
	}
	defer ses.EndSession(sessCtx)

	ids := []string{}
	journal, err := r.fetchByID(sessCtx, journalID, true)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}

	if !input.SupplierTransaction {
		input.TransactionBase.Type = models.TransactionTypeCredit
		sales_transaction, err := sales.NewSalesTransaction(sessCtx, input.TransactionBase, journal.Branch.ID, r.transactions, r.finance)
		if err != nil {
			_ = ses.AbortTransaction(sessCtx)
			return nil, err
		}
		ids = append(ids, sales_transaction.ID)
	} else {
		input.TransactionBase.Type = models.TransactionTypeCredit
		if input.SupplierID == "" {
			_ = ses.AbortTransaction(sessCtx)
			return nil, repoerr.ErrInvalidInput
		}
		supplier_transaction, err := suppliers.NewSupplierTransaction(sessCtx, input.TransactionBase, input.SupplierID, journal.Branch.ID, r.transactions, r.journals, r.suppliers)
		if err != nil {
			_ = ses.AbortTransaction(sessCtx)
			return nil, err
		}
		ids = append(ids, supplier_transaction.ID)
	}

	_, err = r.journals.UpdateByID(
		sessCtx,
		journal.ID,
		bson.M{
			"$inc": bson.M{
				"total": input.Amount,
			},
			"$push": bson.M{
				"operations": bson.M{
					"$each": ids,
				},
			},
		},
	)
	if err != nil {
		_ = ses.AbortTransaction(sessCtx)
		return nil, err
	}

	// commit the transaction
	if err := ses.CommitTransaction(sessCtx); err != nil {
		return nil, err
	}

	return journal, nil
}

// UpdateOperation edits the amount (and optionally description) of an operation
// on an already-loaded journal, inside a MongoDB transaction: the transaction
// document, journal total and branch finance total are all adjusted by the diff
// (old amount - new amount), preserving the controller's accounting exactly. The
// passed-in journal is mutated in place (total and operations) and returned.
// Returns repoerr.ErrInvalidInput for a non-positive amount and
// repoerr.ErrNotFound when the operation is not on the journal.
func (r *JournalsRepository) UpdateOperation(ctx context.Context, journal *models.Journal, operationID string, amount int, description string) (*models.Journal, error) {
	if amount <= 0 {
		return nil, repoerr.ErrInvalidInput
	}

	// start a new session and transaction
	ses, sessCtx, err := database.StartTransaction(r.journals.Database().Client())
	if err != nil {
		return nil, err
	}
	defer ses.EndSession(sessCtx)

	branch_id := journal.Branch.ID

	for _, operation := range journal.Operations {
		if operation.ID == operationID {
			// update transaction in transactions collection
			update_query := bson.M{
				"$set": bson.M{
					"transactionbase.amount": uint32(amount),
					"updated_at":             time.Now(),
				},
			}
			if description != "" {
				update_query["$set"].(bson.M)["transactionbase.description"] = description
			}

			_, err = r.transactions.UpdateOne(sessCtx, bson.M{"_id": operation.ID}, update_query)
			if err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}
			// update journal total
			diff := int32(int(operation.Amount) - amount)
			_, err = r.journals.UpdateOne(sessCtx, bson.M{"_id": journal.ID}, bson.M{"$inc": bson.M{"total": -diff}})
			if err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}
			new_operation := models.Transaction{
				ID: operation.ID,
				TransactionBase: models.TransactionBase{
					Amount:      uint32(amount),
					Description: operation.Description,
				},
				CreatedAt: operation.CreatedAt,
				UpdatedAt: time.Now(),
			}
			if description != "" {
				new_operation.Description = description
			}
			journal.Total -= uint32(diff)
			journal.Operations = utils.ReplaceElement(journal.Operations, operation, new_operation)

			// update finance of branch
			_, err = r.finance.UpdateOne(sessCtx, bson.M{"branch_id": branch_id}, bson.M{"$inc": bson.M{"total": -diff}})
			if err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}
			// commit the transaction
			if err := ses.CommitTransaction(sessCtx); err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}
			return journal, nil
		}
	}
	return nil, repoerr.ErrNotFound
}

// DeleteOperation removes an operation from an already-loaded journal inside a
// MongoDB transaction: it deletes the transaction document, decrements the
// branch finance total and the journal total by the operation amount, and pulls
// the operation id off the journal. The passed-in journal is mutated in place
// (operations and total) and returned. Returns repoerr.ErrNotFound when the
// operation is not on the journal. Accounting preserved exactly from the
// controller.
func (r *JournalsRepository) DeleteOperation(ctx context.Context, journal *models.Journal, operationID string) (*models.Journal, error) {
	// start a new session and transaction
	ses, sessCtx, err := database.StartTransaction(r.journals.Database().Client())
	if err != nil {
		return nil, err
	}
	defer ses.EndSession(sessCtx)

	branch_id := journal.Branch.ID
	// delete transaction from transactions collection
	// check if the transaction exists
	for _, operation := range journal.Operations {
		if operation.ID == operationID {
			// delete transaction from transactions collection
			_, err = r.transactions.DeleteOne(sessCtx, bson.M{"_id": operation.ID})
			if err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}

			// update journal total, operations list
			// update finance of branch
			_, err = r.finance.UpdateOne(sessCtx, bson.M{"branch_id": branch_id}, bson.M{"$inc": bson.M{"total": -1 * int32(operation.Amount)}})
			if err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}

			// update journal total, operations list
			_, err = r.journals.UpdateOne(sessCtx, bson.M{"_id": journal.ID}, bson.M{"$pull": bson.M{"operations": operationID}, "$inc": bson.M{"total": -1 * int32(operation.Amount)}})
			if err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}
			journal.Operations = utils.RemoveElement(journal.Operations, operation)
			journal.Total -= operation.Amount

			// commit the transaction
			if err := ses.CommitTransaction(sessCtx); err != nil {
				_ = ses.AbortTransaction(sessCtx)
				return nil, err
			}
			return journal, nil
		}
	}

	_ = ses.AbortTransaction(sessCtx)
	return nil, repoerr.ErrNotFound
}
