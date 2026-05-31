package sales

import (
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	salesrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/sales"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SalesTransactionsController exposes the sales (POS checkout) transaction
// endpoints. All database access goes through Repo; the controller only parses
// requests, logs audit activity, and renders the response envelope.
type SalesTransactionsController struct {
	Repo       *salesrepo.SalesRepository
	activities *mongo.Collection
}

func New(db *mongo.Database) *SalesTransactionsController {
	log.Info().Msg("Initializing SalesTransactionsController")
	return &SalesTransactionsController{
		Repo:       salesrepo.New(db),
		activities: db.Collection("activities"),
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
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	log.Info().
		Str("branch_id", branch_id).
		Interface("transaction_base", transaction_base).
		Msg("Creating new sales transaction")

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateTransaction, transaction_base, s.activities)

	transaction, err := s.Repo.CreateTransaction(c.Context(), branch_id, transaction_base)
	if err != nil {
		// StartTransaction / commit / business failures all surface as 500.
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().
		Str("transaction_id", transaction.ID).
		Str("branch_id", branch_id).
		Uint32("amount", transaction.Amount).
		Msg("Sales transaction created successfully")

	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(transaction))
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

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeDeleteTransaction, transaction_id, s.activities)

	transaction, err := s.Repo.DeleteTransaction(c.Context(), transaction_id)
	if err != nil {
		// StartTransaction / commit / lookup / business failures all surface as 500.
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().
		Str("transaction_id", transaction_id).
		Msg("Transaction deleted successfully")

	return c.JSON(models.NewOutput(transaction))
}
