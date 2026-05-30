package products

import (
	"fmt"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	s3provider "github.com/aslon1213/g4h_pos_erp/platform/s3"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type ProductsController struct {
	ProductsCollection     *mongo.Collection
	TransactionsCollection *mongo.Collection
	FinanceCollection      *mongo.Collection
	SupplierCollection     *mongo.Collection
	ActivitiesCollection   *mongo.Collection
	S3Client               *s3provider.S3Client
}

func New(db *mongo.Database) *ProductsController {
	return &ProductsController{
		ProductsCollection:     db.Collection("products"),
		TransactionsCollection: db.Collection("transactions"),
		FinanceCollection:      db.Collection("finance"),
		SupplierCollection:     db.Collection("suppliers"),
		ActivitiesCollection:   db.Collection("activities"),
	}
}

// CreateProduct godoc
// @Security BearerAuth
// @Summary Create a new product
// @Description Creates a new product with the given details
// @Tags products
// @Accept json
// @Produce json
// @Param product body models.ProductBase true "Product details"
// @Success 201 {object} models.Output[[]models.Product]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/products [post]
func (p *ProductsController) CreateProduct(c *fiber.Ctx) error {
	_, span := otel.Tracer("products").Start(c.Context(), "create_product")
	defer span.End()

	log.Info().Msg("Creating new product")
	base := &models.ProductBase{}

	if err := c.BodyParser(base); err != nil {
		log.Error().Err(err).Msg("Failed to parse product body")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	product := models.NewProduct(base)
	// log activity

	span.AddEvent("Inserting product", trace.WithAttributes(attribute.String("product", fmt.Sprintf("%v", product))))
	log.Debug().Interface("product", product).Msg("Inserting product")

	_, err := p.ProductsCollection.InsertOne(c.Context(), product)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert product")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateProduct, product.ID, p.ActivitiesCollection)
	log.Info().Str("id", product.ID).Msg("Successfully created product")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(
		[]models.Product{*product},
	))
}

// EditProduct godoc
// @Security BearerAuth
// @Summary Edit a product
// @Description Updates an existing product with the given details
// @Tags products
// @Accept json
// @Produce json
// @Param id path string true "Product ID"
// @Param product body models.ProductBase true "Product details to update"
// @Success 200 {object} models.Output[[]models.Product]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/products/{id} [put]
func (p *ProductsController) EditProduct(c *fiber.Ctx) error {
	_, span := otel.Tracer("products").Start(c.Context(), "edit_product")
	defer span.End()

	id := c.Params("id")
	log.Info().Str("id", id).Msg("Editing product")

	product := &models.ProductBase{}
	if err := c.BodyParser(product); err != nil {
		log.Error().Err(err).Msg("Failed to parse product update body")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	update := bson.M{
		"$set": bson.M{},
	}

	if product.Name != "" {
		update["$set"].(bson.M)["name"] = product.Name
	}
	if product.Description != "" {
		update["$set"].(bson.M)["description"] = product.Description
	}
	if product.Manufacturer.Name != "" {
		update["$set"].(bson.M)["manufacturer.name"] = product.Manufacturer.Name
	}
	if product.Manufacturer.Country != "" {
		update["$set"].(bson.M)["manufacturer.country"] = product.Manufacturer.Country
	}
	if product.Manufacturer.Address != "" {
		update["$set"].(bson.M)["manufacturer.address"] = product.Manufacturer.Address
	}
	if product.Manufacturer.Phone != "" {
		update["$set"].(bson.M)["manufacturer.phone"] = product.Manufacturer.Phone
	}
	if product.Manufacturer.Email != "" {
		update["$set"].(bson.M)["manufacturer.email"] = product.Manufacturer.Email
	}
	if product.Category != nil {
		update["$set"].(bson.M)["category"] = product.Category
	}
	if product.SKU != "" {
		update["$set"].(bson.M)["sku"] = product.SKU
	}
	if product.MinimumStockAlert != 0 {
		update["$set"].(bson.M)["minimum_stock_alert"] = product.MinimumStockAlert
	}

	log.Debug().Interface("update", update).Msg("Updating product")
	_, err := p.ProductsCollection.UpdateOne(c.Context(), bson.M{"_id": id}, update)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update product")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	product_ := &models.Product{}
	err = p.ProductsCollection.FindOne(c.Context(), bson.M{"_id": id}).Decode(product_)
	if err != nil {

		log.Error().Err(err).Msg("Failed to fetch updated product")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeEditProduct, fiber.Map{
		"id":     id,
		"update": update,
		"user":   c.Locals("user").(string),
	}, p.ActivitiesCollection)

	log.Info().Str("id", id).Msg("Successfully updated product")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		[]models.Product{*product_},
	))
}

// DeleteProduct godoc
// @Security BearerAuth
// @Summary Delete a product
// @Description Deletes a product and its related data
// @Tags products
// @Produce json
// @Param id path string true "Product ID"
// @Success 200 {object} models.Output[[]models.Product]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/products/{id} [delete]
func (p *ProductsController) DeleteProduct(c *fiber.Ctx) error {
	_, span := otel.Tracer("products").Start(c.Context(), "delete_product")
	defer span.End()

	id := c.Params("id")
	log.Info().Str("id", id).Msg("Deleting product")

	_, err := p.ProductsCollection.DeleteOne(c.Context(), bson.M{"_id": id})
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete product")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Str("id", id).Msg("Successfully deleted product")
	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeDeleteProduct, fiber.Map{
		"id": id,
	}, p.ActivitiesCollection)
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		[]models.Product{
			{
				ID: id,
			},
		},
	))
}

// GetProductByID godoc
// @Security BearerAuth
// @Summary Get a product by ID
// @Description Retrieves a product by its ID
// @Tags products
// @Produce json
// @Param id path string true "Product ID"
// @Success 200 {object} models.Output[[]models.Product]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/products/{id} [get]
func (p *ProductsController) GetProductByID(c *fiber.Ctx) error {
	_, span := otel.Tracer("products").Start(c.Context(), "get_product_by_id")
	defer span.End()

	id := c.Params("id")
	log.Info().Str("id", id).Msg("Getting product by ID")

	product := &models.Product{}
	err := p.ProductsCollection.FindOne(c.Context(), bson.M{"_id": id}).Decode(product)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Product not found")
		return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Product not found",
			Code:    fiber.StatusNotFound,
		}))
	}

	log.Info().Str("id", id).Msg("Successfully retrieved product")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		[]models.Product{*product},
	))
}

// QueryProducts godoc
// @Security BearerAuth
// @Summary Query products
// @Description Query products based on various parameters
// @Tags products
// @Produce json
// @Param branch_id query string false "Branch ID"
// @Param sku query string false "SKU"
// @Param price_min query number false "Minimum price"
// @Param price_max query number false "Maximum price"
// @Success 200 {object} models.Output[[]models.Product]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/products [get]
func (p *ProductsController) QueryProducts(c *fiber.Ctx) error {
	_, span := otel.Tracer("products").Start(c.Context(), "query_products")
	defer span.End()

	log.Info().Msg("Querying products")
	params := models.ProductQueryParams{}

	if err := c.QueryParser(&params); err != nil {
		log.Error().Err(err).Msg("Failed to parse query parameters")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	pipeline := bson.D{}
	if params.Name != "" {
		pipeline = append(pipeline, bson.E{Key: "name", Value: bson.M{"$regex": params.Name, "$options": "i"}})
	}

	if params.BranchID != "" {
		pipeline = append(pipeline, bson.E{Key: "quantity_distribution.place.id", Value: params.BranchID})
	}
	if params.SKU != "" {
		pipeline = append(pipeline, bson.E{Key: "sku", Value: params.SKU})
	}
	if params.PriceMin != 0 {
		pipeline = append(pipeline, bson.E{Key: "quantity_distribution.price", Value: bson.E{Key: "$gte", Value: params.PriceMin}})
	}
	if params.PriceMax != 0 {
		pipeline = append(pipeline, bson.E{Key: "quantity_distribution.price", Value: bson.E{Key: "$lte", Value: params.PriceMax}})
	}

	log.Debug().Interface("pipeline", pipeline).Msg("Executing Find query")
	cursor, err := p.ProductsCollection.Find(c.Context(), pipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to execute Find query")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	products := []models.Product{}
	if err := cursor.All(c.Context(), &products); err != nil {
		log.Error().Err(err).Msg("Failed to decode products")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Int("count", len(products)).Msg("Successfully queried products")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		products,
	))
}
