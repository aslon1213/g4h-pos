package journal_handlers

import (
	"context"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/sales"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/suppliers"
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"
	"github.com/aslon1213/g4h_pos_erp/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type OperationHandlers struct {
	ctx                    context.Context
	FinancesCollection     *mongo.Collection
	JournalsCollection     *mongo.Collection
	SuppliersCollections   *mongo.Collection
	TransactionsCollection *mongo.Collection
	ActivitiesCollection   *mongo.Collection
}

func NewOperationsHandler(db *mongo.Database) *OperationHandlers {
	ctx := context.Background()
	journalsCollection := db.Collection("journals")
	suppliersCollections := db.Collection("suppliers")
	financesCollection := db.Collection("finance")
	transactionsCollection := db.Collection("transactions")
	activitiesCollection := db.Collection("activities")
	return &OperationHandlers{
		ctx:                    ctx,
		JournalsCollection:     journalsCollection,
		FinancesCollection:     financesCollection,
		SuppliersCollections:   suppliersCollections,
		TransactionsCollection: transactionsCollection,
		ActivitiesCollection:   activitiesCollection,
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
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	log.Info().Interface("transaction", transaction).Msg("Transaction data")

	// transaction_created := &models.Transaction{}
	// start a new session and transaction
	ses, ctx, err := database.StartTransaction(o.JournalsCollection.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer ses.EndSession(ctx)
	// log activity

	ids := []string{}
	log.Info().Msg("Fetching journal by ID")
	journal, err := FetchJournalByID(ctx, c, true, o.JournalsCollection)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch journal by ID")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	if !transaction.SupplierTransaction {
		transaction.TransactionBase.Type = models.TransactionTypeCredit
		log.Info().Msg("Creating new sales transaction")
		sales_transaction, err := sales.NewTransaction(ctx, transaction.TransactionBase, journal.Branch.ID, o.TransactionsCollection, o.FinancesCollection)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create sales transaction")

			return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: err.Error(),
				Code:    fiber.StatusInternalServerError,
			}))
		}
		// idx := sales_transaction.ID
		ids = append(ids, sales_transaction.ID)
	} else {
		transaction.TransactionBase.Type = models.TransactionTypeCredit
		if transaction.SupplierID == "" {
			log.Error().Msg("Supplier ID is required")

			return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Supplier ID is required",
				Code:    fiber.StatusBadRequest,
			}))
		}
		log.Info().Msg("Creating new supplier transaction")
		supplier_transaction, err := suppliers.NewSupplierTransaction(ctx, transaction.TransactionBase, transaction.SupplierID, journal.Branch.ID, o.TransactionsCollection, o.JournalsCollection, o.SuppliersCollections)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create supplier transaction")

			return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: err.Error(),
				Code:    fiber.StatusInternalServerError,
			}))
		}
		ids = append(ids, supplier_transaction.ID)
	}

	log.Info().Msg("Updating journal total")
	_, err = o.JournalsCollection.UpdateByID(
		ctx,
		journal.ID,
		bson.M{
			"$inc": bson.M{
				"total": transaction.Amount,
			},
			"$push": bson.M{
				"operations": bson.M{
					"$each": ids,
				},
			},
		},
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update journal total")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	// commit the transaction
	if err := ses.CommitTransaction(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateTransaction, transaction, o.ActivitiesCollection)

	log.Info().Msg("Transaction created successfully")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(journal))
}

func (o *OperationHandlers) ShiftIsOpenMiddleware(c *fiber.Ctx) error {
	ctx := context.Background()
	journal, err := FetchJournalByID(
		ctx,
		c,
		true,
		o.JournalsCollection,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch journal by ID")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	if journal.Shift_is_closed {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Shift is closed",
			Code:    fiber.StatusBadRequest,
		}))
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
	// log handler
	log.Info().Str("operation_id", c.Params("operation_id")).Msg("Updating operation transaction by ID")

	journal := c.Locals("journal").(*models.Journal)
	operation_id := c.Params("operation_id")
	amount := c.QueryInt("amount")
	description := c.Query("description")
	if amount <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Amount must be greater than 0",
			Code:    fiber.StatusBadRequest,
		}))
	}

	// start a new session and transaction
	ses, ctx, err := database.StartTransaction(o.JournalsCollection.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer ses.EndSession(ctx)

	branch_id := journal.Branch.ID
	// check if shift is closed or not

	for _, operation := range journal.Operations {
		if operation.ID == operation_id {
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

			_, err = o.TransactionsCollection.UpdateOne(ctx, bson.M{"_id": operation.ID}, update_query)
			if err != nil {
				log.Error().Err(err).Msg("Failed to update transaction in transactions collection")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
			}
			// update journal total
			diff := int32(int(operation.Amount) - amount)
			_, err = o.JournalsCollection.UpdateOne(ctx, bson.M{"_id": journal.ID}, bson.M{"$inc": bson.M{"total": -diff}})
			if err != nil {
				log.Error().Err(err).Msg("Failed to update journal total, operations list")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
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
			_, err = o.FinancesCollection.UpdateOne(ctx, bson.M{"branch_id": branch_id}, bson.M{"$inc": bson.M{"total": -diff}})
			if err != nil {
				log.Error().Err(err).Msg("Failed to update finance of branch")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
			}
			// commit the transaction
			if err := ses.CommitTransaction(ctx); err != nil {
				log.Error().Err(err).Msg("Failed to commit transaction")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
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
	}
	return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
		Message: "Operation not found",
		Code:    fiber.StatusNotFound,
	}))
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
	// log activity
	log.Info().Str("operation_id", c.Params("operation_id")).Msg("Deleting operation transaction by ID")
	operation_id := c.Params("operation_id")

	// start a new session and transaction
	ses, ctx, err := database.StartTransaction(o.JournalsCollection.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer ses.EndSession(ctx)

	journal := c.Locals("journal").(*models.Journal)

	branch_id := journal.Branch.ID
	// delete transaction from transactions collection
	// check if the transaction exists
	for _, operation := range journal.Operations {
		if operation.ID == operation_id {
			// delete transaction from transactions collection

			_, err = o.TransactionsCollection.DeleteOne(ctx, bson.M{"_id": operation.ID})
			if err != nil {
				log.Error().Err(err).Msg("Failed to delete transaction from transactions collection")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
			}

			// update journal total, operations list
			// update finance of branch
			_, err = o.FinancesCollection.UpdateOne(ctx, bson.M{"branch_id": branch_id}, bson.M{"$inc": bson.M{"total": -1 * int32(operation.Amount)}})
			if err != nil {
				log.Error().Err(err).Msg("Failed to update finance of branch")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
			}

			// update journal total, operations list
			_, err = o.JournalsCollection.UpdateOne(ctx, bson.M{"_id": journal.ID}, bson.M{"$pull": bson.M{"operations": operation_id}, "$inc": bson.M{"total": -1 * int32(operation.Amount)}})
			if err != nil {
				log.Error().Err(err).Msg("Failed to update journal total, operations list")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
			}
			journal.Operations = utils.RemoveElement(journal.Operations, operation)
			journal.Total -= operation.Amount

			// commit the transaction
			if err := ses.CommitTransaction(ctx); err != nil {
				log.Error().Err(err).Msg("Failed to commit transaction")
				ses.AbortTransaction(ctx)
				return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
					Message: err.Error(),
					Code:    fiber.StatusInternalServerError,
				}))
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
	}

	log.Error().Msg("Transaction not found")
	ses.AbortTransaction(ctx)
	return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
		Message: "Transaction not found",
		Code:    fiber.StatusNotFound,
	}))

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
	// log activity

	panic("Not implemented")
}
