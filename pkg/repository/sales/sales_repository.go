// Package sales is the repository for the sales (POS checkout) domain. It owns
// every mongo/bson interaction the sales controllers perform, mirroring the
// suppliers/finance repositories: a controller holds a *SalesRepository and its
// handlers call these methods instead of touching collections directly.
//
// Scope note: the sales SESSION flow (pkg/models/sales-models.go) is Redis /
// cache-backed and already carries its own methods on models.SalesSession — those
// are NOT database/Mongo operations and are intentionally left out of this
// repository. The only persistent (Mongo) operations the sales controllers run
// are the two transaction flows below: recording a sales transaction (insert +
// branch balance increment) and deleting one (lookup + delete + balance
// decrement), each inside a MongoDB multi-document transaction (requires a
// replica set).
package sales

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/aslon1213/g4h_pos_erp/platform/database"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SalesRepository owns the transactions and finance collections. A sales
// transaction is inserted into `transactions` and reflected on the owning
// branch's `finance` balances.
type SalesRepository struct {
	transactions *mongo.Collection
	finance      *mongo.Collection
}

// New builds the repository.
func New(db *mongo.Database) *SalesRepository {
	return &SalesRepository{
		transactions: db.Collection("transactions"),
		finance:      db.Collection("finance"),
	}
}

// CreateTransaction records a sales transaction for a branch and increments the
// branch's matching balance bucket, atomically in a MongoDB multi-document
// transaction (requires a replica set). The transaction type is forced to credit
// (income), the payment method is validated, and an unknown branch surfaces as
// repoerr.ErrNotFound.
func (r *SalesRepository) CreateTransaction(ctx context.Context, branchID string, base models.TransactionBase) (*models.Transaction, error) {
	sess, sessCtx, err := database.StartTransaction(r.transactions.Database().Client())
	if err != nil {
		return nil, err
	}
	defer sess.EndSession(sessCtx)

	transaction, err := NewSalesTransaction(sessCtx, base, branchID, r.transactions, r.finance)
	if err != nil {
		_ = sess.AbortTransaction(sessCtx)
		return nil, err
	}
	if err := sess.CommitTransaction(sessCtx); err != nil {
		return nil, err
	}
	return transaction, nil
}

// DeleteTransaction looks up a sales transaction by id, deletes it, and reverses
// its effect on the owning branch's balance, atomically in a MongoDB
// multi-document transaction (requires a replica set). A missing transaction
// surfaces as repoerr.ErrNotFound.
func (r *SalesRepository) DeleteTransaction(ctx context.Context, transactionID string) (*models.Transaction, error) {
	sess, sessCtx, err := database.StartTransaction(r.transactions.Database().Client())
	if err != nil {
		return nil, err
	}
	defer sess.EndSession(sessCtx)

	transaction, err := DeleteSalesTransaction(sessCtx, transactionID, r.transactions, r.finance)
	if err != nil {
		_ = sess.AbortTransaction(sessCtx)
		return nil, err
	}
	if err := sess.CommitTransaction(sessCtx); err != nil {
		return nil, err
	}
	return transaction, nil
}

// NewSalesTransaction performs the two writes (transaction insert + branch
// balance increment) within the caller's context. It is exported as a free
// function (taking collections explicitly) so other flows can apply a sales
// transaction inside their own multi-document session. The transaction type is
// forced to credit (income) and the payment method is validated before any
// write. Returns repoerr.ErrNotFound if the branch finance document does not
// exist.
func NewSalesTransaction(ctx context.Context, base models.TransactionBase, branchID string, transactions, finance *mongo.Collection) (*models.Transaction, error) {
	base.Type = models.TransactionTypeCredit

	if err := models.ValidatePaymentMethod(base.PaymentMethod); err != nil {
		return nil, err
	}

	transaction := models.NewTransaction(&base, models.InitiatorTypeSales, branchID)

	if _, err := transactions.InsertOne(ctx, transaction); err != nil {
		return nil, err
	}

	if err := incrementBalance(ctx, finance, branchID, base); err != nil {
		return nil, err
	}

	log.Info().
		Str("transaction_id", transaction.ID).
		Uint32("amount", transaction.Amount).
		Str("payment_method", string(transaction.PaymentMethod)).
		Msg("Sales transaction created successfully")

	return transaction, nil
}

// DeleteSalesTransaction looks up, deletes and reverses the balance effect of a
// sales transaction within the caller's context. Exported as a free function so
// it can run inside an existing multi-document session. Returns
// repoerr.ErrNotFound when no transaction matches the id.
func DeleteSalesTransaction(ctx context.Context, transactionID string, transactions, finance *mongo.Collection) (*models.Transaction, error) {
	transaction := models.Transaction{}
	err := transactions.FindOne(ctx, bson.M{"_id": transactionID}).Decode(&transaction)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	res, err := transactions.DeleteOne(ctx, bson.M{"_id": transactionID})
	if err != nil {
		return nil, err
	}
	if res.DeletedCount == 0 {
		return nil, repoerr.ErrNotFound
	}

	if err := decrementBalance(ctx, finance, transaction.BranchID, transaction); err != nil {
		return nil, err
	}

	return &transaction, nil
}

// incrementBalance adds the transaction amount to the branch's matching balance
// bucket. Returns repoerr.ErrNotFound when no finance document matches the branch.
func incrementBalance(ctx context.Context, finance *mongo.Collection, branchID string, base models.TransactionBase) error {
	filter := bson.M{
		"branch_id": branchID,
	}
	update := bson.M{
		"$inc": bson.M{},
	}
	switch base.PaymentMethod {
	case models.PaymentMethodCash:
		update["$inc"].(bson.M)["finance.balance.cash"] = int(base.Amount)
	case models.PaymentMethodBank:
		update["$inc"].(bson.M)["finance.balance.bank"] = int(base.Amount)
	case models.PaymentMethodTerminal:
		update["$inc"].(bson.M)["finance.balance.terminal"] = int(base.Amount)
	case models.OnlineMobileAppPayment:
		update["$inc"].(bson.M)["finance.balance.mobile_apps"] = int(base.Amount)
	case models.OnlineTransfer:
		update["$inc"].(bson.M)["finance.balance.mobile_apps"] = int(base.Amount)
	}

	result, err := finance.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// decrementBalance subtracts the transaction amount from the branch's matching
// balance bucket. Returns repoerr.ErrNotFound when no finance document matches
// the branch.
func decrementBalance(ctx context.Context, finance *mongo.Collection, branchID string, transaction models.Transaction) error {
	filter := bson.M{
		"branch_id": branchID,
	}
	update := bson.M{
		"$inc": bson.M{},
	}
	switch transaction.PaymentMethod {
	case models.PaymentMethodCash:
		update["$inc"].(bson.M)["finance.balance.cash"] = -int32(transaction.Amount)
	case models.PaymentMethodBank:
		update["$inc"].(bson.M)["finance.balance.bank"] = -int32(transaction.Amount)
	case models.PaymentMethodTerminal:
		update["$inc"].(bson.M)["finance.balance.terminal"] = -int32(transaction.Amount)
	case models.OnlineMobileAppPayment:
		update["$inc"].(bson.M)["finance.balance.mobile_apps"] = -int32(transaction.Amount)
	case models.OnlineTransfer:
		update["$inc"].(bson.M)["finance.balance.mobile_apps"] = -int32(transaction.Amount)
	}

	result, err := finance.UpdateOne(ctx, filter, update)
	if err != nil {
		return err
	}
	if result.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}
