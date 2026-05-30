package suppliers

import (
	"context"
	"errors"

	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/aslon1213/g4h_pos_erp/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// NewTransaction godoc
// @Security BearerAuth
// @Summary Create a new transaction for a supplier
// @Description Create a new transaction for a supplier and update financial records
// @Tags suppliers, transactions
// @Accept json
// @Produce json
// @Param branch_id path string true "Branch ID"
// @Param supplier_id path string true "Supplier ID"
// @Param transaction body models.TransactionBase true "Transaction data"
// @Success 200 {object} models.Output[models.Transaction]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{branch_id}/{supplier_id}/transactions [post]
func (s *SuppliersController) NewTransaction(c *fiber.Ctx) error {
	// if suppliers gets money, that is when we payed money to them,
	// so we need to add money to the supplier's balance and decrease the debt of the branch also decrease the balance of the branch
	// if suppliers pays money, that is when they have given us some product,
	// service or something valuable, so we need to decrease the balance of the supplier and increase the debt of the branch

	branch_id := c.Params("branch_id")

	supplier_id := c.Params("supplier_id")

	// Parse transaction data from request body
	var transactionBase models.TransactionBase
	if err := c.BodyParser(&transactionBase); err != nil {
		log.Error().Err(err).Msg("Failed to parse transaction data")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}
	log.Info().Str("branch_id", branch_id).Str("supplier_id", supplier_id).Str("TransactionType", string(transactionBase.Type)).Msg("Starting new transaction")

	// validate the transaction base
	if err := models.ValidateTransactionType(transactionBase.Type); err != nil {
		log.Error().Err(err).Str("transaction_type", string(transactionBase.Type)).Msg("Failed to validate transaction data")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	// Start a session
	sess, ctx, err := database.StartTransaction(s.transactionsCollection.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer sess.EndSession(ctx)

	transaction, err := NewSupplierTransaction(ctx, transactionBase, supplier_id, branch_id, s.transactionsCollection, s.financeCollection, s.suppliersCollection)
	if err != nil {
		sess.AbortTransaction(ctx)
		log.Error().Err(err).Msg("Failed to create new supplier transaction --- aborting")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	// Commit transaction
	err = sess.CommitTransaction(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	log.Info().Msg("Transaction committed successfully")

	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(transaction))
}

func NewSupplierTransaction(ctx context.Context, transactionBase models.TransactionBase, supplier_id string, branch_id string, transactionsCollection *mongo.Collection, financeCollection *mongo.Collection, suppliersCollection *mongo.Collection) (*models.Transaction, error) {
	// Create new transaction
	transaction := models.NewTransaction(&transactionBase, models.InitiatorTypeSupplier, branch_id)
	log.Info().Interface("transaction", transaction).Str("TransactionType", string(transaction.TransactionBase.Type)).Msg("Created new transaction")

	// Insert transaction
	res, err := transactionsCollection.InsertOne(ctx, transaction)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert transaction")
		return &models.Transaction{}, err
	}
	log.Info().Interface("res", res).Msg("Transaction inserted successfully")

	// Update supplier financial data
	update := bson.M{
		"$push": bson.M{
			"financial_data.transactions": transaction,
		},
		"$inc": bson.M{
			"financial_data.balance": func() int32 {
				if transaction.TransactionBase.Type == models.TransactionTypeCredit {
					return int32(transaction.Amount)
				}
				return -int32(transaction.Amount)
			}(),
			"financial_data.total_income": func() int32 {
				if transaction.TransactionBase.Type == models.TransactionTypeCredit {
					return int32(transaction.Amount)
				}
				return 0
			}(),
			"financial_data.total_expenses": func() int32 {
				if transaction.TransactionBase.Type == models.TransactionTypeDebit {
					return int32(transaction.Amount)
				}
				return 0
			}(),
		},
	}

	supplier_res, err := suppliersCollection.UpdateOne(ctx, bson.M{"_id": supplier_id}, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update supplier financial data")
		return &models.Transaction{}, err
	}
	if supplier_res.MatchedCount == 0 {
		log.Error().Msg("Supplier not found")
		return &models.Transaction{}, errors.New("supplier not found")
	}

	log.Info().Msg("Supplier financial data updated successfully")

	// change finance of branch

	// update finance of branch
	filter := bson.M{
		"branch_id": branch_id,
	}
	update_bson := bson.M{
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

	switch transaction.TransactionBase.PaymentMethod {
	case models.PaymentMethodCash:
		update_bson["finance.balance.cash"] = func() int32 {
			if transaction.TransactionBase.Type == models.TransactionTypeCredit {
				return -int32(transaction.Amount)
			}
			return 0
		}()
	case models.PaymentMethodBank, models.OnlineMobileAppPayment:
		update_bson["finance.balance.bank"] = func() int32 {
			if transaction.TransactionBase.Type == models.TransactionTypeCredit {
				return -int32(transaction.Amount)
			}
			return 0
		}()
	case models.OnlineTransfer:
		update_bson["finance.balance.mobile_apps"] = func() int32 {
			if transaction.TransactionBase.Type == models.TransactionTypeCredit {
				return -int32(transaction.Amount)
			}
			return 0
		}()
	}

	update = bson.M{
		"$inc": update_bson,
	}

	res_2, err := financeCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update branch finance")
		return &models.Transaction{}, err
	}
	log.Info().Interface("res", res_2).Msg("Branch finance updated successfully")

	return transaction, nil
}
