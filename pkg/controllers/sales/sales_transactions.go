package sales

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/aslon1213/g4h_pos_erp/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type SalesTransactionsController struct {
	transactions *mongo.Collection
	finances     *mongo.Collection
	products     *mongo.Collection
	activities   *mongo.Collection
}

func New(db *mongo.Database) *SalesTransactionsController {
	log.Info().Msg("Initializing SalesTransactionsController")
	return &SalesTransactionsController{
		transactions: db.Collection("transactions"),
		finances:     db.Collection("finance"),
		products:     db.Collection("products"),
		activities:   db.Collection("activities"),
	}
}

// CreateSalesTransaction godoc
// @Security BearerAuth
// @Summary Create a new sales transaction
// @Description Create a new sales transaction for a branch
// @Tags sales/transactions
// @Accept json
// @Produce json
// @Param branch_id path string true "Branch ID"
// @Param transaction body models.TransactionBase true "Transaction details"
// @Success 201 {object} models.Output[models.Transaction]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/sales/transactions/{branch_id} [post]
func (s *SalesTransactionsController) CreateSalesTransaction(c *fiber.Ctx) error {
	branch_id := c.Params("branch_id")
	transaction_base := models.TransactionBase{}
	if err := c.BodyParser(&transaction_base); err != nil {
		log.Error().Err(err).Msg("Failed to parse transaction base")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	log.Info().
		Str("branch_id", branch_id).
		Interface("transaction_base", transaction_base).
		Msg("Creating new sales transaction")

	ses, ctx, err := database.StartTransaction(s.transactions.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer ses.EndSession(ctx)

	// log activity

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateTransaction, transaction_base, s.activities)

	transaction, err := NewTransaction(ctx, transaction_base, branch_id, s.transactions, s.finances)
	if err != nil {

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	if err := ses.CommitTransaction(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().
		Str("transaction_id", transaction.ID).
		Str("branch_id", branch_id).
		Uint32("amount", transaction.Amount).
		Msg("Sales transaction created successfully")

	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(transaction))
}

func NewTransaction(ctx context.Context, transaction_base models.TransactionBase, branch_id string, transactionsCollection *mongo.Collection, financesCollection *mongo.Collection) (*models.Transaction, error) {
	transaction_base.Type = models.TransactionTypeCredit
	log.Info().
		Str("Collection", transactionsCollection.Name()).
		Str("Branch ID", branch_id).
		Interface("Transaction Base", transaction_base).
		Msg("Creating sales transaction")

	if err := models.ValidatePaymentMethod(transaction_base.PaymentMethod); err != nil {
		log.Error().Err(err).Msg("Invalid payment method")
		return nil, err
	}

	transaction := models.NewTransaction(
		&transaction_base,
		models.InitiatorTypeSales,
		branch_id,
	)

	_, err := transactionsCollection.InsertOne(ctx, transaction)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert transaction")
		return nil, err
	}

	if err := IncrementBalance(ctx, financesCollection, branch_id, transaction_base); err != nil {
		log.Error().Err(err).Msg("Failed to increment balance")
		return nil, err
	}

	log.Info().
		Str("transaction_id", transaction.ID).
		Uint32("amount", transaction.Amount).
		Str("payment_method", string(transaction.PaymentMethod)).
		Msg("Transaction created successfully")

	return transaction, nil
}

// DeleteSalesTransaction godoc
// @Security BearerAuth
// @Summary Delete a sales transaction
// @Description Delete a sales transaction by ID
// @Tags sales/transactions
// @Accept json
// @Produce json
// @Param transaction_id path string true "Transaction ID"
// @Success 200 {object} models.Output[models.Transaction]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/sales/transactions/{transaction_id} [delete]
func (s *SalesTransactionsController) DeleteSalesTransaction(c *fiber.Ctx) error {
	transaction_id := c.Params("transaction_id")
	log.Info().Str("transaction_id", transaction_id).Msg("Deleting sales transaction")

	ses, ctx, err := database.StartTransaction(s.transactions.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer ses.EndSession(ctx)

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeDeleteTransaction, transaction_id, s.activities)

	transaction, err := DeleteSalesTransaction(ctx, transaction_id, s.transactions, s.finances)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	if err := ses.CommitTransaction(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().
		Str("transaction_id", transaction_id).
		Msg("Transaction deleted successfully")

	return c.JSON(models.NewOutput(transaction))
}

func DeleteSalesTransaction(ctx context.Context, transactionID string, transactionsCollection *mongo.Collection, financesCollection *mongo.Collection) (models.Transaction, error) {
	transaction := models.Transaction{}
	err := transactionsCollection.FindOne(ctx, bson.M{"_id": transactionID}).Decode(&transaction)
	if err != nil {
		log.Error().Err(err).Str("transaction_id", transactionID).Msg("Failed to find transaction")
		return models.Transaction{}, err
	}

	_, err = transactionsCollection.DeleteOne(ctx, bson.M{"_id": transactionID})
	if err != nil {
		log.Error().Err(err).Str("transaction_id", transactionID).Msg("Failed to delete transaction")
		return models.Transaction{}, err
	}

	if err := DecrementBalance(ctx, financesCollection, transaction.BranchID, transaction); err != nil {
		log.Error().Err(err).Str("branch_id", transaction.BranchID).Msg("Failed to decrement balance")
		return models.Transaction{}, err
	}

	return transaction, nil
}

func IncrementBalance(ctx context.Context, finance *mongo.Collection, branch_id string, transaction_base models.TransactionBase) error {
	filter := bson.M{
		"branch_id": branch_id,
	}
	update := bson.M{
		"$inc": bson.M{},
	}
	switch transaction_base.PaymentMethod {
	case models.PaymentMethodCash:
		update["$inc"].(bson.M)["finance.balance.cash"] = int(transaction_base.Amount)
	case models.PaymentMethodBank:
		update["$inc"].(bson.M)["finance.balance.bank"] = int(transaction_base.Amount)
	case models.PaymentMethodTerminal:
		update["$inc"].(bson.M)["finance.balance.terminal"] = int(transaction_base.Amount)
	case models.OnlineMobileAppPayment:
		update["$inc"].(bson.M)["finance.balance.mobile_apps"] = int(transaction_base.Amount)
	case models.OnlineTransfer:
		update["$inc"].(bson.M)["finance.balance.mobile_apps"] = int(transaction_base.Amount)
	}

	log.Info().
		Interface("update", update).
		Interface("filter", filter).
		Str("Collection", finance.Name()).
		Msg("Updating finance of the branch")

	result, err := finance.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update finance")
		return err
	}
	if result.MatchedCount == 0 {
		log.Error().Str("branch_id", branch_id).Msg("No finance found for the branch")
		return errors.New("no finance found for the branch")
	}
	return nil
}

func DecrementBalance(ctx context.Context, finance *mongo.Collection, branch_id string, transaction models.Transaction) error {
	filter := bson.M{
		"branch_id": branch_id,
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

	log.Info().
		Interface("update", update).
		Interface("filter", filter).
		Str("Collection", finance.Name()).
		Msg("Decrementing branch balance")

	result, err := finance.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update finance")
		return err
	}
	if result.MatchedCount == 0 {
		log.Error().Str("branch_id", branch_id).Msg("No finance found for the branch")
		return errors.New("no finance found for the branch")
	}
	return nil
}
