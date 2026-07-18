package transactions

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	transactionsrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/transactions"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// TransactionsController exposes the admin transactions endpoints. All database
// access goes through Repo; the controller parses requests and renders the
// response envelope.
type TransactionsController struct {
	Repo   *transactionsrepo.TransactionsRepository
	logger *zerolog.Logger
}

func New(db *gorm.DB) *TransactionsController {
	return &TransactionsController{
		Repo:   transactionsrepo.New(db),
		logger: &log.Logger,
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

	transactions, err := s.Repo.Find(c.Context(), branch_id, queryParams)
	if err != nil {
		s.logger.Error().Err(err).Msg("Error finding transactions")
		return models.RespondRepoError(c, err)
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
	transaction, err := t.Repo.GetByID(c.Context(), transaction_id)
	if err != nil {
		t.logger.Error().Err(err).Str("transaction_id", transaction_id).Msg("Error finding transaction by ID")
		return models.RespondRepoError(c, err)
	}
	t.logger.Info().Str("transaction_id", transaction_id).Msg("Successfully retrieved transaction")
	return c.JSON(models.NewOutput(transaction))
}

// GetTransactionDetailsByID godoc
// @Security BearerAuth
// @Summary Get a transaction with its type-specific details
// @Description Returns the transaction plus a discriminated `details` object resolved from its initiator type: the cart and its items for a sale, the supplier for a supplier transaction, the BNPL record for a BNPL one. Types with no detail record of their own (salary, rent, utilities, other) return `details` with only `kind` set — as does a sale that has no cart, such as a keyed amount. Switch on `details.kind` in the client.
// @Tags transactions
// @Produce json
// @Param id path string true "Transaction ID"
// @Success 200 {object} models.Output[models.TransactionWithDetails]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/transactions/{id}/details [get]
func (t *TransactionsController) GetTransactionDetailsByID(c *fiber.Ctx) error {
	transactionID := c.Params("id")
	details, err := t.Repo.GetDetails(c.Context(), transactionID)
	if err != nil {
		t.logger.Error().Err(err).Str("transaction_id", transactionID).Msg("Error finding transaction details by ID")
		return models.RespondRepoError(c, err)
	}
	t.logger.Info().Str("transaction_id", transactionID).Str("kind", string(details.Details.Kind)).
		Msg("Successfully retrieved transaction details")
	return c.JSON(models.NewOutput(*details))
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

	if err := t.Repo.Update(c.Context(), idx, amount, description, typeOfTransaction); err != nil {
		t.logger.Error().Err(err).Str("transaction_id", idx).Msg("Error updating transaction")
		return models.RespondRepoError(c, err)
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
	// Deleting a transaction must also reverse its balance effects; that path is
	// not yet ported, so the endpoint stays a 501 (was an unimplemented panic).
	return models.NotImplemented(c)
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
