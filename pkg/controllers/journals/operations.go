package journal_handlers

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	journalsrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/journals"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// OperationHandlers exposes the admin journal-operation endpoints. All database
// access goes through Repo; the controller only parses requests, logs audit
// activity, runs request-level validation, and renders the response envelope.
type OperationHandlers struct {
	ctx                  context.Context
	Repo                 *journalsrepo.JournalsRepository
	ActivitiesCollection *mongo.Collection
}

func NewOperationsHandler(db *mongo.Database) *OperationHandlers {
	ctx := context.Background()
	activitiesCollection := db.Collection("activities")
	return &OperationHandlers{
		ctx:                  ctx,
		Repo:                 journalsrepo.New(db),
		ActivitiesCollection: activitiesCollection,
	}
}

// NewOperationTransaction godoc
// @Security BearerAuth
// @Summary Create a new operation transaction
// @Description Create a new transaction and update the journal
// @Tags journals/operations
// @Accept json
// @Produce json
// @Param transaction body models.JournalOperationInput true "Transaction data"
// @Param journal_id path string true "Journal ID"
// @Success 201 {object} models.Output[models.Journal]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/{journal_id}/operations [post]
func (o *OperationHandlers) NewOperationTransaction(c *fiber.Ctx) error {
	transaction := models.JournalOperationInput{}
	if err := c.BodyParser(&transaction); err != nil {
		log.Error().Err(err).Msg("Failed to parse transaction data")
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	log.Info().Interface("transaction", transaction).Msg("Transaction data")

	// Request-level validation: a supplier transaction requires a supplier id.
	if transaction.SupplierTransaction && transaction.SupplierID == "" {
		log.Error().Msg("Supplier ID is required")
		return models.RespondError(c, fiber.StatusBadRequest, "Supplier ID is required")
	}

	journal, err := o.Repo.AddOperation(c.Context(), c.Params("id"), transaction)
	if err != nil {
		log.Error().Err(err).Msg("Failed to add operation to journal")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateTransaction, transaction, o.ActivitiesCollection)

	log.Info().Msg("Transaction created successfully")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(journal))
}

func (o *OperationHandlers) ShiftIsOpenMiddleware(c *fiber.Ctx) error {
	journal, err := o.Repo.GetByID(context.Background(), c.Params("id"), true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch journal by ID")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	if journal.Shift_is_closed {
		return models.RespondError(c, fiber.StatusBadRequest, "Shift is closed")
	}
	c.Locals("journal", journal)
	return c.Next()
}

// UpdateOperationTransactionByID godoc
// @Security BearerAuth
// @Summary Update an operation transaction
// @Description Update an operation transaction by ID
// @Tags journals/operations
// @Produce json
// @Param id path string true "Operation ID"
// @Param journal_id path string true "Journal ID"
// @Param amount query int true "Amount"
// @Param description query string false "Description"
// @Success 200 {object} models.Output[models.Journal]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/{journal_id}/operations/{id} [put]
func (o *OperationHandlers) UpdateOperationTransactionByID(c *fiber.Ctx) error {
	log.Info().Str("operation_id", c.Params("operation_id")).Msg("Updating operation transaction by ID")

	journal := c.Locals("journal").(*models.Journal)
	operation_id := c.Params("operation_id")
	amount := c.QueryInt("amount")
	description := c.Query("description")
	if amount <= 0 {
		return models.RespondError(c, fiber.StatusBadRequest, "Amount must be greater than 0")
	}

	journal, err := o.Repo.UpdateOperation(c.Context(), journal, operation_id, amount, description)
	if err != nil {
		// The original handler returned 404 when the operation was not on the
		// journal, and 500 for every other (db transaction) failure.
		if errors.Is(err, repoerr.ErrNotFound) {
			return models.RespondError(c, fiber.StatusNotFound, "Operation not found")
		}
		log.Error().Err(err).Msg("Failed to update operation transaction")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeEditOperation, fiber.Map{
		"journal_id":   journal.ID,
		"operation_id": operation_id,
		"amount":       amount,
		"description":  description,
		"activity":     "updated",
	}, o.ActivitiesCollection)
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(journal))
}

// DeleteOperationTransactionByID godoc
// @Security BearerAuth
// @Summary Delete an operation transaction
// @Description Delete an operation transaction by ID
// @Tags journals/operations
// @Produce json
// @Param id path string true "Operation ID"
// @Param journal_id path string true "Journal ID"
// @Success 200 {object} models.Output[models.Journal]
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/{journal_id}/operations/{id} [delete]
func (o *OperationHandlers) DeleteOperationTransactionByID(c *fiber.Ctx) error {
	log.Info().Str("operation_id", c.Params("operation_id")).Msg("Deleting operation transaction by ID")
	operation_id := c.Params("operation_id")

	journal := c.Locals("journal").(*models.Journal)

	journal, err := o.Repo.DeleteOperation(c.Context(), journal, operation_id)
	if err != nil {
		// The original handler returned 404 when the operation was not on the
		// journal, and 500 for every other (db transaction) failure.
		if errors.Is(err, repoerr.ErrNotFound) {
			log.Error().Msg("Transaction not found")
			return models.RespondError(c, fiber.StatusNotFound, "Transaction not found")
		}
		log.Error().Err(err).Msg("Failed to delete operation transaction")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeDeleteOperation, fiber.Map{
		"journal_id":   journal.ID,
		"operation_id": operation_id,
		"activity":     "deleted",
	}, o.ActivitiesCollection)

	log.Info().Msg("Transaction deleted successfully")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(journal))
}

// GetOperationTransactionByID godoc
// @Security BearerAuth
// @Summary Get an operation transaction by ID
// @Description Get an operation transaction by ID
// @Tags journals/operations
// @Produce json
// @Param id path string true "Operation ID"
// @Param journal_id path string true "Journal ID"
// @Success 200 {object} models.Output[models.Transaction]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/{journal_id}/operations/{id} [get]
func (o *OperationHandlers) GetOperationTransactionByID(c *fiber.Ctx) error {
	panic("Not implemented")
}
