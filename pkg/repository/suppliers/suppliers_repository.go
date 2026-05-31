// Package suppliers is the repository for the admin suppliers domain. It owns
// every mongo/bson interaction for suppliers, mirroring the store repositories:
// the controller holds a *SuppliersRepository and its handlers call these
// methods instead of touching collections directly.
package suppliers

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"
	"github.com/aslon1213/g4h_pos_erp/platform/database"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SuppliersRepository owns the suppliers, transactions and finance collections.
// Finance is needed to resolve branches and to reflect supplier transactions on
// the owning branch's balances.
type SuppliersRepository struct {
	suppliers    *mongo.Collection
	transactions *mongo.Collection
	finance      *mongo.Collection
}

// New builds the repository.
func New(db *mongo.Database) *SuppliersRepository {
	return &SuppliersRepository{
		suppliers:    db.Collection("suppliers"),
		transactions: db.Collection("transactions"),
		finance:      db.Collection("finance"),
	}
}

// resolveBranchID looks a branch up by id or name and returns its branch id, or
// repoerr.ErrNotFound when no branch matches.
func (r *SuppliersRepository) resolveBranchID(ctx context.Context, branchOrName string) (string, error) {
	branch := models.BranchFinance{}
	err := r.finance.FindOne(ctx, bson.M{"$or": []bson.M{
		{"branch_id": branchOrName},
		{"branch_name": branchOrName},
	}}).Decode(&branch)
	if errors.Is(err, mongo.ErrNoDocuments) {
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
	filter := bson.M{}
	if q.Name != "" {
		filter["name"] = q.Name
	}
	if q.INN != "" {
		filter["inn"] = q.INN
	}
	if q.Branch != "" {
		branchID, err := r.resolveBranchID(ctx, q.Branch)
		if err != nil {
			return nil, err
		}
		filter["branch"] = branchID
	}
	if q.Email != "" {
		filter["email"] = q.Email
	}
	if q.Phone != "" {
		filter["phone"] = q.Phone
	}
	if q.Address != "" {
		filter["address"] = q.Address
	}
	if q.Notes != "" {
		filter["notes"] = q.Notes
	}

	cursor, err := r.suppliers.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	suppliers := []models.Supplier{}
	if err := cursor.All(ctx, &suppliers); err != nil {
		return nil, err
	}
	return suppliers, nil
}

// GetByID returns a single supplier, or repoerr.ErrNotFound.
func (r *SuppliersRepository) GetByID(ctx context.Context, id string) (*models.Supplier, error) {
	supplier := &models.Supplier{}
	err := r.suppliers.FindOne(ctx, bson.M{"_id": id}).Decode(supplier)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return supplier, nil
}

// Create inserts a new supplier (with zeroed financial data) under the branch
// named/identified by base.Branch, and registers it on that branch's finance
// document. Returns repoerr.ErrNotFound if the branch does not exist.
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

	if _, err := r.suppliers.InsertOne(ctx, supplier); err != nil {
		return nil, err
	}

	// register the supplier on the branch's finance document
	if _, err := r.finance.UpdateOne(ctx,
		bson.M{"branch_id": branchID},
		bson.M{"$push": bson.M{"suppliers": supplier.ID}},
	); err != nil {
		return nil, err
	}
	return &supplier, nil
}

// Update patches the editable fields of a supplier. Returns repoerr.ErrNotFound
// when no supplier matches the id.
func (r *SuppliersRepository) Update(ctx context.Context, id string, base models.SupplierBase) error {
	set := bson.M{"updated_at": time.Now().In(utils.GetTimeZone())}
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

	res, err := r.suppliers.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// Delete removes a supplier. Returns repoerr.ErrNotFound when it does not exist.
func (r *SuppliersRepository) Delete(ctx context.Context, id string) error {
	res, err := r.suppliers.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// CreateTransaction records a supplier transaction and reflects it on both the
// supplier's financial data and the branch's finance balances, atomically in a
// MongoDB multi-document transaction (requires a replica set). Returns
// repoerr.ErrNotFound if the supplier does not exist.
func (r *SuppliersRepository) CreateTransaction(ctx context.Context, branchID, supplierID string, base models.TransactionBase) (*models.Transaction, error) {
	sess, sessCtx, err := database.StartTransaction(r.suppliers.Database().Client())
	if err != nil {
		return nil, err
	}
	defer sess.EndSession(sessCtx)

	transaction, err := NewSupplierTransaction(sessCtx, base, supplierID, branchID, r.transactions, r.finance, r.suppliers)
	if err != nil {
		_ = sess.AbortTransaction(sessCtx)
		return nil, err
	}
	if err := sess.CommitTransaction(sessCtx); err != nil {
		return nil, err
	}
	return transaction, nil
}

// NewSupplierTransaction performs the three writes (transaction insert, supplier
// financial-data update, branch finance update) within the caller's context.
// It is exported as a free function (taking collections explicitly) so other
// domains — journals operations, product income — can apply a supplier
// transaction inside their own multi-document session. Money semantics:
//   - credit (we paid the supplier): supplier balance +amount, branch debt -amount
//     and the matching branch balance bucket is reduced.
//   - debit (supplier delivered value): supplier balance -amount, branch debt +amount.
//
// Returns repoerr.ErrNotFound if the supplier does not exist.
func NewSupplierTransaction(ctx context.Context, base models.TransactionBase, supplierID, branchID string, transactions, finance, suppliers *mongo.Collection) (*models.Transaction, error) {
	transaction := models.NewTransaction(&base, models.InitiatorTypeSupplier, branchID)

	if _, err := transactions.InsertOne(ctx, transaction); err != nil {
		return nil, err
	}

	isCredit := transaction.TransactionBase.Type == models.TransactionTypeCredit
	signedAmount := func(when bool) int32 {
		if when {
			return int32(transaction.Amount)
		}
		return 0
	}

	supplierUpdate := bson.M{
		"$push": bson.M{"financial_data.transactions": transaction},
		"$inc": bson.M{
			"financial_data.balance": func() int32 {
				if isCredit {
					return int32(transaction.Amount)
				}
				return -int32(transaction.Amount)
			}(),
			"financial_data.total_income":   signedAmount(isCredit),
			"financial_data.total_expenses": signedAmount(!isCredit),
		},
	}
	supRes, err := suppliers.UpdateOne(ctx, bson.M{"_id": supplierID}, supplierUpdate)
	if err != nil {
		return nil, err
	}
	if supRes.MatchedCount == 0 {
		return nil, repoerr.ErrNotFound
	}

	// branch finance: debt and the matching balance bucket
	inc := bson.M{
		"finance.debt": func() int32 {
			switch transaction.TransactionBase.Type {
			case models.TransactionTypeDebit:
				return int32(transaction.Amount)
			case models.TransactionTypeCredit:
				return -int32(transaction.Amount)
			}
			return 0
		}(),
	}
	creditOutflow := func() int32 {
		if isCredit {
			return -int32(transaction.Amount)
		}
		return 0
	}()
	switch transaction.TransactionBase.PaymentMethod {
	case models.PaymentMethodCash:
		inc["finance.balance.cash"] = creditOutflow
	case models.PaymentMethodBank, models.OnlineMobileAppPayment:
		inc["finance.balance.bank"] = creditOutflow
	case models.OnlineTransfer:
		inc["finance.balance.mobile_apps"] = creditOutflow
	}

	if _, err := finance.UpdateOne(ctx, bson.M{"branch_id": branchID}, bson.M{"$inc": inc}); err != nil {
		return nil, err
	}
	log.Info().Str("supplier_id", supplierID).Str("branch_id", branchID).Msg("Supplier transaction applied")
	return transaction, nil
}
