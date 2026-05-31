package transactions

import (
	"context"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type TransactionsController struct {
	collection *mongo.Collection
	logger     *zerolog.Logger
	// cache      cache
}

func New(db *mongo.Database) *TransactionsController {
	return &TransactionsController{
		collection: db.Collection("transactions"),
		logger:     &log.Logger,
	}
}

// GetTransactionsByQueryParams godoc
// @Security BearerAuth
// @Summary Get transactions by query parameters
// @Description Retrieve transactions based on various query parameters
// @Tags transactions
// @Accept json
// @Produce json
// @Param branch_id path string true "Branch ID"
// @Param description query string false "Transaction description"
// @Param amount_min query int false "Minimum transaction amount"
// @Param amount_max query int false "Maximum transaction amount"
// @Param payment_method query string false "Payment method"
// @Param type_of_transaction query string false "Type of transaction"
// @Param initiator_type query string false "Initiator type"
// @Param date_min query string false "Minimum date"
// @Param date_max query string false "Maximum date"
// @Param page query int false "Page number"
// @Param count query int false "Number of transactions per page"
// @Success 200 {object} models.TransactionOutput
// @Failure 400 {object} models.Error
// @Failure 500 {object} models.Error
// @Router /api/v1/admin/transactions/branch/{branch_id} [get]
func (s *TransactionsController) GetTransactionsByQueryParams(c *fiber.Ctx) error {
	s.logger.Info().Msg("GetTransactionsByQueryParams called")
	branch_id := c.Params("branch_id")
	if branch_id == "" {
		s.logger.Warn().Msg("branch_id is required but not provided")
		return c.Status(401).JSON(
			models.NewErrorOutput(
				models.NewError(
					"branch_id is required",
					fiber.StatusBadRequest,
				),
			),
		)
	}
	queryParams := models.TransactionQueryParams{}
	if err := c.QueryParser(&queryParams); err != nil {
		s.logger.Error().Err(err).Msg("Error parsing query params")
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewOutput([]interface{}{}, models.NewError(
				"invalid query params",
				fiber.StatusBadRequest,
			)),
		)
	}
	if err := queryParams.Validate(); err != nil {
		s.logger.Error().Err(err).Msg("Error validating query params")
		return c.Status(fiber.StatusBadRequest).JSON(
			models.NewOutput([]interface{}{}, models.NewError(err.Error(), fiber.StatusBadRequest)),
		)
	}

	query := bson.M{}
	if queryParams.Description != "" {
		query["description"] = queryParams.Description
	}
	if queryParams.AmountMin != 0 {
		query["amount"] = bson.M{"$gte": queryParams.AmountMin}
	}
	if queryParams.AmountMax != 0 {
		query["amount"] = bson.M{"$lte": queryParams.AmountMax}
	}
	if queryParams.PaymentMethod != "" {
		query["payment_method"] = queryParams.PaymentMethod
	}
	if queryParams.TypeOfTransaction != "" {
		query["transactionbase.type"] = queryParams.TypeOfTransaction
	}
	if queryParams.InitiatorType != "" {
		query["type"] = queryParams.InitiatorType
	}
	// if page and count are not provided the set default values
	// page = 1, count = 10
	if queryParams.Page == 0 {
		queryParams.Page = 1
	}
	if queryParams.Count == 0 {
		queryParams.Count = 10
	}
	if !queryParams.DateMax.IsZero() {
		query["created_at"] = bson.M{"$lte": queryParams.DateMax}
	}
	if !queryParams.DateMin.IsZero() {
		query["created_at"] = bson.M{"$gte": queryParams.DateMin}
	}

	// apply pagination --- move cursor to the correct page --- query.count * (query.page - 1): query.count * query.page
	options := options.Find().SetSkip(int64(queryParams.Count * (queryParams.Page - 1))).SetLimit(int64(queryParams.Count)).SetSort(bson.M{"created_at": -1})

	cursor, err := s.collection.Find(context.Background(), query, options)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error finding transactions")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer cursor.Close(context.Background())

	transactions := []models.Transaction{}
	err = cursor.All(context.Background(), &transactions)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error decoding transactions")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	s.logger.Info().Int("count", len(transactions)).Msg("Successfully retrieved transactions")
	return c.JSON(models.NewOutput(transactions))
}

// GetTransactionByID godoc
// @Security BearerAuth
// @Summary Get a transaction by ID
// @Description Retrieve a single transaction by its ID
// @Tags transactions
// @Accept json
// @Produce json
// @Param transaction_id path string true "Transaction ID"
// @Success 200 {object} models.TransactionOutputSingle
// @Failure 500 {object} models.Error
// @Router /api/v1/admin/transactions/{transaction_id} [get]
func (t *TransactionsController) GetTransactionByID(c *fiber.Ctx) error {
	t.logger.Info().Msg("GetTransactionByID called")
	transaction_id := c.Params("id")
	transaction := models.Transaction{}
	err := t.collection.FindOne(context.Background(), bson.M{"_id": transaction_id}).Decode(&transaction)
	if err != nil {
		t.logger.Error().Err(err).Str("transaction_id", transaction_id).Msg("Error finding transaction by ID")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	t.logger.Info().Str("transaction_id", transaction_id).Msg("Successfully retrieved transaction")
	return c.JSON(models.NewOutput(transaction))
}

// UpdateTransactionByID godoc
// @Security BearerAuth
// @Summary Update a transaction by ID
// @Description Update transaction details by its ID
// @Tags transactions
// @Accept json
// @Produce json
// @Param id path string true "Transaction ID"
// @Param amount query string false "Transaction amount"
// @Param description query string false "Transaction description"
// @Param type query string false "Type of transaction"
// @Success 200 {object} map[string]string "message" : "transaction was succesfully updated"
// @Failure 500 {object} models.Error
// @Router /api/v1/admin/transactions/{id} [put]
func (t *TransactionsController) UpdateTransactionByID(c *fiber.Ctx) error {
	t.logger.Info().Msg("UpdateTransactionByID called")
	idx := c.Params("id")
	amount := c.Query("amount", "")
	description := c.Query("description", "")
	typeOfTransaction := c.Query("type", "")

	query := bson.M{
		"_id": idx,
	}
	set := bson.M{
		"$set": bson.M{},
	}
	if amount != "" {
		set["$set"].(bson.M)["amount"] = amount
	}
	if description != "" {
		set["$set"].(bson.M)["description"] = description
	}
	if typeOfTransaction != "" {
		set["$set"].(bson.M)["type"] = typeOfTransaction
	}

	_, err := t.collection.UpdateOne(context.Background(), query, set)
	if err != nil {
		t.logger.Error().Err(err).Str("transaction_id", idx).Msg("Error updating transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	t.logger.Info().Str("transaction_id", idx).Msg("Successfully updated transaction")
	return c.JSON(models.NewOutput(fiber.Map{
		"message": "transaction was succesfully updated",
	}))
}

// DeleteTransaction godoc
// @Security BearerAuth
// @Summary Delete a transaction by ID
// @Description Delete a transaction from the database by its ID
// @Tags transactions
// @Accept json
// @Produce json
// @Param id path string true "Transaction ID"
// @Success 200 {object} map[string]string "message" : "transaction was succesfully deleted"
// @Failure 500 {object} models.Error
// @Router /api/v1/admin/transactions/{id} [delete]
func (t *TransactionsController) DeleteTransactionByID(c *fiber.Ctx) error {
	t.logger.Info().Msg("DeleteTransactionByID called")
	panic("not implemented")
	idx := c.Params("id")
	query := bson.M{
		"_id": idx,
	}
	_, err := t.collection.DeleteOne(context.Background(), query)
	if err != nil {
		t.logger.Error().Err(err).Str("transaction_id", idx).Msg("Error deleting transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	t.logger.Info().Str("transaction_id", idx).Msg("Successfully deleted transaction")
	return c.JSON(models.NewOutput(
		fiber.Map{
			"message": "transaction was succesfully deleted",
		}))

}

// GetInitiatorType godoc
// @Security BearerAuth
// @Summary Get all initiator types
// @Description Retrieve a list of all possible initiator types for transactions
// @Tags transactions
// @Accept json
// @Produce json
// @Success 200 {array} models.InitiatorType
// @Router /api/v1/admin/transactions/docs/initiator_type [get]
func (t *TransactionsController) GetInitiatorType(c *fiber.Ctx) error {
	t.logger.Info().Msg("GetInitiatorType called")
	types := []models.InitiatorType{
		models.InitiatorTypeSalary,
		models.InitiatorTypeRent,
		models.InitiatorTypeUtilities,
		models.InitiatorTypeOther,
		models.InitiatorTypeSales,
		models.InitiatorTypeSupplier,
	}
	return c.JSON(models.NewOutput(types))
}

// GetTransactionType godoc
// @Security BearerAuth
// @Summary Get all transaction types
// @Description Retrieve a list of all possible transaction types
// @Tags transactions
// @Accept json
// @Produce json
// @Success 200 {array} models.TransactionType
// @Router /api/v1/admin/transactions/docs/type [get]
func (t *TransactionsController) GetTransactionType(c *fiber.Ctx) error {
	t.logger.Info().Msg("GetTransactionType called")
	types := []models.TransactionType{
		models.TransactionTypeCredit,
		models.TransactionTypeDebit,
	}
	return c.JSON(models.NewOutput(types))
}

// GetPaymentMethod godoc
// @Security BearerAuth
// @Summary Get all payment methods
// @Description Retrieve a list of all possible payment methods
// @Tags transactions
// @Accept json
// @Produce json
// @Success 200 {array} models.PaymentMethod
// @Router /api/v1/admin/transactions/docs/payment_method [get]
func (t *TransactionsController) GetPaymentMethod(c *fiber.Ctx) error {
	t.logger.Info().Msg("GetPaymentMethod called")
	methods := []models.PaymentMethod{
		models.PaymentMethodCash,
		models.PaymentMethodBank,
		models.PaymentMethodTerminal,
		models.OnlineMobileAppPayment,
		models.Cheque,
		models.OnlineTransfer,
	}
	return c.JSON(models.NewOutput(methods))
}

func NewTransaction(ctx context.Context, transaction models.TransactionBase, initiatorType models.InitiatorType, branchID string, transactionsCollection *mongo.Collection) (string, error) {
	trx := models.NewTransaction(&transaction, initiatorType, branchID)
	_, err := transactionsCollection.InsertOne(ctx, trx)
	if err != nil {
		return "", err
	}
	return trx.ID, nil

}
