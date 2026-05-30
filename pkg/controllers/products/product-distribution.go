package products

import (
	"context"

	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/suppliers"
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/aslon1213/g4h_pos_erp/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// to add new income to the product distribution

type NewIncomeInput struct {
	models.IncomeHistory
	SellingPrice int32 `json:"selling_price"`
}

// NewIncome godoc
// @Security BearerAuth
// @Summary Add new income for a product
// @Description Adds new income entry for a product with quantity and price updates
// @Tags products
// @Accept json
// @Produce json
// @Param id path string true "Product ID"
// @Param input body NewIncomeInput true "Income details"
// @Success 200 {object} models.Output[[]models.Product]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/products/{id}/income [post]
func (p *ProductsController) NewIncome(c *fiber.Ctx) error {
	log.Info().Msg("Starting new income process")

	product_id := c.Params("id")
	input := NewIncomeInput{}
	if err := c.BodyParser(&input); err != nil {
		log.Error().Err(err).Msg("Failed to parse income input")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Invalid request body format",
			Code:    fiber.StatusBadRequest,
		}))
	}

	// Validate required fields
	if input.Quantity <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Quantity must be greater than 0",
			Code:    fiber.StatusBadRequest,
		}))
	}

	if input.Price <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Price must be greater than 0",
			Code:    fiber.StatusBadRequest,
		}))
	}

	if input.SellingPrice <= 0 {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Selling price must be greater than 0",
			Code:    fiber.StatusBadRequest,
		}))
	}

	if input.UploadedTo.ID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Upload location ID is required",
			Code:    fiber.StatusBadRequest,
		}))
	}

	log.Debug().Str("product_id", product_id).Interface("input", input).Msg("Processing income for product")

	// start a db transactions
	session, ctx, err := database.StartTransaction(p.ProductsCollection.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")

		return models.ReturnError(c, err)
	}
	defer session.EndSession(ctx)

	// get the product
	product := &models.Product{}
	res := p.ProductsCollection.FindOne(c.Context(), bson.M{"_id": product_id})
	if res.Err() != nil {
		log.Error().Err(res.Err()).Str("product_id", product_id).Msg("Product not found")

		return models.ReturnError(c, res.Err())
	}
	err = res.Decode(product)
	if err != nil {
		log.Error().Err(err).Msg("Failed to decode product")

		return models.ReturnError(c, err)
	}

	log.Debug().Str("product_id", product_id).Msg("Updating quantity distribution")

	// update the price distribution
	if len(product.QuantityDistribution) > 0 {
		for _, distribution := range product.QuantityDistribution {
			if distribution.Place.ID == input.UploadedTo.ID {
				// update the quantity
				distribution.Quantity += input.Quantity
				distribution.Price = input.SellingPrice
				// update the database
				filter := bson.M{"_id": product_id, "quantity_distribution.place._id": input.UploadedTo.ID}
				update := bson.M{"$set": bson.M{"quantity_distribution.$.price": input.SellingPrice}, "$inc": bson.M{"quantity_distribution.$.quantity": input.Quantity}}
				_, err = p.ProductsCollection.UpdateOne(ctx, filter, update)
				if err != nil {
					log.Error().Err(err).Msg("Failed to update quantity distribution")

					return models.AbortTransactionAndReturnError(ctx, session, c, err)
				}
			} else {
				err = AppendQuantityDistribution(ctx, input, product, session, p)
				if err != nil {
					log.Error().Err(err).Msg("Failed to create new distribution")

					return models.AbortTransactionAndReturnError(ctx, session, c, err)
				}
			}
		}
	} else {
		err = AppendQuantityDistribution(ctx, input, product, session, p)
		if err != nil {
			log.Error().Err(err).Msg("Failed to create new distribution")

			return models.AbortTransactionAndReturnError(ctx, session, c, err)
		}
	}

	log.Debug().Msg("Creating supplier transaction")
	// create supplier transaction
	transaction_base := models.TransactionBase{
		Amount:        uint32(input.Price * input.Quantity),
		Description:   "Income from " + input.SupplierID,
		Type:          models.TransactionTypeDebit,
		PaymentMethod: models.PaymentMethodUndefined,
	}

	supplier_transaction, err := suppliers.NewSupplierTransaction(
		ctx,
		transaction_base,
		input.SupplierID,
		input.UploadedTo.ID,
		p.TransactionsCollection,
		p.FinanceCollection,
		p.SupplierCollection,
	)
	log.Debug().Interface("supplier_transaction", supplier_transaction).Msg("Supplier transaction created")
	if err != nil {
		log.Error().Err(err).Msg("Failed to create supplier transaction")

		return models.AbortTransactionAndReturnError(ctx, session, c, err)
	}

	// commit the transaction
	err = session.CommitTransaction(c.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")

		return models.AbortTransactionAndReturnError(ctx, session, c, err)
	}

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeProductIncome, fiber.Map{
		"product_id": product_id,
		"input":      input,
	}, p.ActivitiesCollection)

	log.Info().Str("product_id", product_id).Msg("Successfully processed new income")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		[]models.Product{*product},
	))
}

func AppendQuantityDistribution(ctx context.Context, input NewIncomeInput, product *models.Product, session *mongo.Session, p *ProductsController) error {
	// create new distribution
	product.QuantityDistribution = append(product.QuantityDistribution, models.ProductDistribution{
		Place: input.UploadedTo,
		Price: input.SellingPrice,
	})
	filter := bson.M{"_id": product.ID}
	update := bson.M{"$push": bson.M{"quantity_distribution": models.ProductDistribution{
		ProductQuantityInfo: models.ProductQuantityInfo{
			Quantity: input.Quantity,
		},
		Place: input.UploadedTo,
		Price: input.SellingPrice,
	}}}
	_, err := p.ProductsCollection.UpdateOne(ctx, filter, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create new distribution")
		return err
	}
	return nil
}

// NewTransfer godoc
// @Security BearerAuth
// @Summary Transfer product between locations
// @Description Transfers product quantity from one location to another
// @Tags products
// @Produce json
// @Success 200 {object} models.Output[[]models.Product]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/products/transfer [post]
func (p *ProductsController) NewTransfer(c *fiber.Ctx) error {

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeProductTransfer, fiber.Map{}, p.ActivitiesCollection)

	panic("Not implemented")
	return nil
}
