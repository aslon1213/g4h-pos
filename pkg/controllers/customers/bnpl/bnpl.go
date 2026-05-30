package bnpl

import (
	"context"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/transactions"
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/aslon1213/g4h_pos_erp/platform/database"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type BNPLController struct {
	activitiesCollection   *mongo.Collection
	customersCollection    *mongo.Collection
	transactionsCollection *mongo.Collection
	financeCollection      *mongo.Collection
}

func New(db *mongo.Database) *BNPLController {
	return &BNPLController{
		activitiesCollection:   db.Collection("activities"),
		customersCollection:    db.Collection("customers"),
		transactionsCollection: db.Collection("transactions"),
		financeCollection:      db.Collection("finance"),
	}
}

// NewBNPL godoc
// @Summary Create new BNPL
// @Security BearerAuth
// @Description Create a new Buy Now Pay Later transaction
// @Tags BNPL
// @Accept json
// @Produce json
// @Param input body models.NewBNPLInput true "BNPL input"
// @Success 200 {object} models.Output[[]models.BNPL]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/bnpl [post]
func (ctrl *BNPLController) NewBNPL(c *fiber.Ctx) error {
	log.Info().Msg("Creating new BNPL")

	new_bnpl_input := &models.NewBNPLInput{}
	if err := c.BodyParser(new_bnpl_input); err != nil {
		log.Error().Err(err).Msg("Failed to parse request body")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	if err := new_bnpl_input.Validate(); err != nil {
		log.Error().Err(err).Msg("Invalid input")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	// check if the customer exists
	customer := &models.Customer{}
	err := ctrl.customersCollection.FindOne(context.Background(), bson.M{"_id": new_bnpl_input.CustomerID}).Decode(customer)
	if err != nil {
		log.Error().Err(err).Str("customer_id", new_bnpl_input.CustomerID).Msg("Customer not found")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	// calculate total amount
	var total_amount int32
	if new_bnpl_input.CalculateTotalAmount {
		for _, product := range new_bnpl_input.Products {
			total_amount += product.Price * int32(product.Quantity)
		}
	} else {
		total_amount = new_bnpl_input.TotalAmount
	}

	bnpl := &models.BNPL{
		ID:           uuid.New().String(),
		CustomerID:   new_bnpl_input.CustomerID,
		TotalAmount:  total_amount,
		PaidAmount:   0,
		Products:     new_bnpl_input.Products,
		Status:       models.BNPLStatusActive,
		Transactions: []string{},
		BranchID:     new_bnpl_input.BranchID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err = ctrl.customersCollection.UpdateOne(context.Background(), bson.M{"_id": new_bnpl_input.CustomerID}, bson.M{"$push": bson.M{"bnpls": bnpl}})
	if err != nil {
		log.Error().Err(err).Msg("Failed to update customer with new BNPL")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Str("bnpl_id", bnpl.ID).Msg("Successfully created new BNPL")
	return c.JSON(models.NewOutput([]interface{}{}))
}

// CreditBNPL godoc
// @Summary Credit BNPL payment
// @Security BearerAuth
// @Description Add a credit payment to an existing BNPL
// @Tags BNPL
// @Accept json
// @Produce json
// @Param id path string true "BNPL ID"
// @Param amount query int true "Payment amount"
// @Param payment_method query string false "Payment method" default(cash)
// @Success 200 {object} models.Output[[]models.BNPL]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/bnpl/{id}/credit [post]
func (ctrl *BNPLController) CreditBNPL(c *fiber.Ctx) error {
	log.Info().Msg("Processing BNPL credit payment")

	bnpl_id := c.Params("id")
	amount := c.QueryInt("amount")
	payment_method := c.Query("payment_method")
	if payment_method == "" {
		payment_method = "cash"
	}

	log.Info().
		Str("bnpl_id", bnpl_id).
		Int("amount", amount).
		Str("payment_method", payment_method).
		Msg("BNPL credit payment details")

	// open database transaction
	db := ctrl.customersCollection.Database()
	session, ctx, err := database.StartTransaction(db.Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer session.EndSession(ctx)

	transaction := models.TransactionBase{
		Amount:        uint32(amount),
		Description:   "Credit BNPL",
		Type:          models.TransactionTypeCredit,
		PaymentMethod: models.PaymentMethod(payment_method),
	}

	// get the BNPL from the customers collection
	bnpl, err := GetBNPLByIDFromDB(ctx, bnpl_id, ctrl.customersCollection)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to find BNPL")
		session.AbortTransaction(ctx)
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	trx_id, err := transactions.NewTransaction(
		ctx,
		transaction,
		models.InitiatorTypeBNPL,
		bnpl.BranchID,
		ctrl.transactionsCollection,
	)

	if err != nil {
		log.Error().Err(err).Msg("Failed to create transaction")
		session.AbortTransaction(ctx)
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	bnpl.Transactions = append(bnpl.Transactions, trx_id)

	// update finance of branch
	log.Info().Str("branch_id", bnpl.BranchID).Msg("Updating branch finance")
	// TODO: Add other payment methods --- actually move finance increment and decrement
	// TODO: to a separate function in finance controller
	update_res, err := ctrl.financeCollection.UpdateOne(
		ctx,
		bson.M{"branch_id": bnpl.BranchID},
		bson.M{"$inc": bson.M{"finance.balance.cash": amount}},
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update branch finance")
		session.AbortTransaction(ctx)
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	if update_res.MatchedCount == 0 {
		log.Error().Str("branch_id", bnpl.BranchID).Msg("Branch not found")
		session.AbortTransaction(ctx)
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Branch not found",
			Code:    fiber.StatusInternalServerError,
		}))
	}

	total_paid_amount := bnpl.PaidAmount + int32(amount)
	bnpl.UpdatedAt = time.Now()
	bnpl.PaidAmount = total_paid_amount
	bnpl.Transactions = append(bnpl.Transactions, trx_id)

	if total_paid_amount >= bnpl.TotalAmount {
		log.Info().Msg("BNPL payment completed")
		bnpl.Status = models.BNPLStatusCompleted

		update_res, err = ctrl.customersCollection.UpdateOne(ctx, bson.M{"bnpls.id": bnpl_id}, bson.M{
			"$set":  bson.M{"bnpls.$.paid_amount": total_paid_amount, "bnpls.$.updated_at": time.Now(), "bnpls.$.status": models.BNPLStatusCompleted},
			"$push": bson.M{"bnpls.$.transactions": trx_id},
		})
		if err != nil || update_res.MatchedCount == 0 {
			log.Error().Err(err).Msg("Failed to update completed BNPL")
			session.AbortTransaction(ctx)
			return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: err.Error(),
				Code:    fiber.StatusInternalServerError,
			}))
		}
	} else {
		log.Info().Msg("Updating BNPL payment amount")
		update_res, err = ctrl.customersCollection.UpdateOne(ctx, bson.M{"bnpls.id": bnpl_id}, bson.M{
			"$set":  bson.M{"bnpls.$.paid_amount": total_paid_amount, "bnpls.$.updated_at": time.Now(), "bnpls.$.status": models.BNPLStatusActive},
			"$push": bson.M{"bnpls.$.transactions": trx_id},
		})
		if err != nil || update_res.MatchedCount == 0 {
			log.Error().Err(err).Msg("Failed to update BNPL payment")
			session.AbortTransaction(ctx)
			return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: err.Error(),
				Code:    fiber.StatusInternalServerError,
			}))
		}
	}

	session.CommitTransaction(ctx)
	log.Info().Str("bnpl_id", bnpl_id).Msg("Successfully processed BNPL payment")
	return c.JSON(models.NewOutput([]*models.BNPL{bnpl}))
}

// DeleteBNPL godoc
// @Summary Delete BNPL
// @Security BearerAuth
// @Description Delete an existing BNPL transaction
// @Tags BNPL
// @Produce json
// @Param id path string true "BNPL ID"
// @Success 200 {object} models.MessageResponse
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/bnpl/{id} [delete]
func (ctrl *BNPLController) DeleteBNPL(c *fiber.Ctx) error {
	log.Info().Msg("Deleting BNPL")

	bnpl_id := c.Params("id")

	_, err := ctrl.customersCollection.UpdateOne(
		context.Background(),
		bson.M{
			"bnpls.id": bnpl_id,
		},
		bson.M{
			"$pull": bson.M{
				"bnpls": bson.M{"id": bnpl_id},
			},
		},
	)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to delete BNPL")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Str("bnpl_id", bnpl_id).Msg("Successfully deleted BNPL")
	return c.JSON(models.NewOutput(fiber.Map{
		"message": "BNPL deleted successfully",
	}))
}

// GetBNPL godoc
// @Summary Get BNPL details
// @Security BearerAuth
// @Description Get details of a specific BNPL transaction
// @Tags BNPL
// @Produce json
// @Param id path string true "BNPL ID"
// @Success 200 {object} models.Output[models.BNPL]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/bnpl/{id} [get]
func (ctrl *BNPLController) GetBNPLByID(c *fiber.Ctx) error {
	log.Info().Msg("Getting BNPL details")

	bnpl_id := c.Params("id")
	bnpl, err := GetBNPLByIDFromDB(context.Background(), bnpl_id, ctrl.customersCollection)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to get BNPL")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Str("bnpl_id", bnpl_id).Msg("Successfully retrieved BNPL details")
	return c.JSON(models.NewOutput(bnpl))
}
func GetBNPLByIDFromDB(ctx context.Context, bnpl_id string, customersCollection *mongo.Collection) (*models.BNPL, error) {
	log.Debug().Str("bnpl_id", bnpl_id).Msg("Getting BNPL from database")

	pipeline := mongo.Pipeline{
		bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "bnpls.id", Value: bnpl_id},
			}},
		},
		bson.D{
			{Key: "$project", Value: bson.D{
				{Key: "bnpl", Value: bson.D{
					{Key: "$first", Value: bson.D{
						{Key: `$filter`, Value: bson.D{
							{Key: "input", Value: `$bnpls`},
							{Key: "as", Value: "b"},
							{Key: "cond", Value: bson.D{
								{Key: "$eq", Value: bson.A{"$$b.id", bnpl_id}},
							}},
						}},
					}},
				}},
				{Key: "_id", Value: 0},
			}},
		},
	}

	cursor, err := customersCollection.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to find BNPL in database")
		return nil, err
	}

	type PipelineResult struct {
		BNPL models.BNPL `bson:"bnpl"`
	}
	var output []PipelineResult

	err = cursor.All(ctx, &output)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to decode BNPL from database")
		return nil, err
	}

	log.Debug().Interface("output", output).Str("bnpl_id", bnpl_id).Msg("Successfully retrieved BNPL from database")
	return &output[0].BNPL, nil
}

// GetBNPLSofCustomer godoc
// @Summary Get customer BNPLs
// @Security BearerAuth
// @Description Get all BNPL transactions for a specific customer
// @Tags BNPL
// @Produce json
// @Param customer_id path string true "Customer ID"
// @Param branch_id query string false "Branch ID"
// @Success 200 {object} models.Output[[]models.BNPL]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/customers/{customer_id}/bnpls [get]
func (ctrl *BNPLController) GetBNPLSofCustomer(c *fiber.Ctx) error {
	log.Info().Msg("Getting customer BNPLs")
	branch_id := c.Query("branch_id")

	customer_id := c.Params("customer_id")
	customer := &models.Customer{}
	query := bson.M{"_id": customer_id}
	if branch_id != "" {
		query["bnpls.branch_id"] = branch_id
	}
	err := ctrl.customersCollection.FindOne(context.Background(), query).Decode(customer)
	if err != nil {
		log.Error().Err(err).Str("customer_id", customer_id).Msg("Failed to get customer BNPLs")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Str("customer_id", customer_id).Msg("Successfully retrieved customer BNPLs")
	return c.JSON(models.NewOutput(customer.BNPLs))
}

// GetBNPLsOfBranch godoc
// @Summary Get BNPLs of branch
// @Security BearerAuth
// @Description Get all BNPL transactions for a specific branch
// @Tags BNPL
// @Produce json
// @Param branch_id path string true "Branch ID"
// @Param customer_name query string false "Customer name"
// @Param customer_phone query string false "Customer phone"
// @Param customer_address query string false "Customer address"
// @Success 200 {object} models.Output[[]models.Customer]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/branches/{branch_id}/bnpls [get]
func (ctrl *BNPLController) GetBNPLsOfBranch(c *fiber.Ctx) error {
	log.Info().Msg("Getting BNPLs of branch")

	// customer name
	// customer phone
	// customer address

	customer_name := c.Query("customer_name")
	customer_phone := c.Query("customer_phone")
	customer_address := c.Query("customer_address")

	branch_id := c.Params("branch_id")
	query := mongo.Pipeline{
		bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "bnpls.branch_id", Value: branch_id},
			}},
		},
	}
	if customer_name != "" {
		query = append(query, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "name", Value: bson.D{
					{Key: "$regex", Value: customer_name},
					{Key: "$options", Value: "i"},
				}},
			}},
		})
	}
	if customer_phone != "" {
		query = append(query, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "phone", Value: bson.D{
					{Key: "$regex", Value: customer_phone},
					{Key: "$options", Value: "i"},
				}},
			}},
		})
	}
	if customer_address != "" {
		query = append(query, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "address", Value: bson.D{
					{Key: "$regex", Value: customer_address},
					{Key: "$options", Value: "i"},
				}},
			}},
		})
	}

	cursor, err := ctrl.customersCollection.Aggregate(context.Background(), query)
	var output []models.Customer
	if err != nil {
		log.Error().Err(err).Str("branch_id", branch_id).Msg("Failed to get BNPLs of branch")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	err = cursor.All(context.Background(), &output)
	if err != nil {
		log.Error().Err(err).Str("branch_id", branch_id).Msg("Failed to get BNPLs of branch")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Str("branch_id", branch_id).Msg("Successfully retrieved BNPLs of branch")
	return c.JSON(models.NewOutput(output))
}
