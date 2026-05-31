package products

import (
	"errors"
	"fmt"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	productsrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/products"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	s3provider "github.com/aslon1213/g4h_pos_erp/platform/s3"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// ProductsController exposes the admin products endpoints. All Mongo access goes
// through Repo; the controller parses requests, performs S3 image storage, logs
// audit activity, and renders the response envelope.
type ProductsController struct {
	Repo                 *productsrepo.ProductsRepository
	ActivitiesCollection *mongo.Collection
	S3Client             *s3provider.S3Client
}

func New(db *mongo.Database) *ProductsController {
	return &ProductsController{
		Repo:                 productsrepo.New(db),
		ActivitiesCollection: db.Collection("activities"),
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
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	span.AddEvent("Inserting product", trace.WithAttributes(attribute.String("product", fmt.Sprintf("%v", base))))
	log.Debug().Interface("product", base).Msg("Inserting product")

	product, err := p.Repo.Create(c.Context(), base)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert product")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
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

	base := &models.ProductBase{}
	if err := c.BodyParser(base); err != nil {
		log.Error().Err(err).Msg("Failed to parse product update body")
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	log.Debug().Interface("update", base).Msg("Updating product")
	product, err := p.Repo.Update(c.Context(), id, base)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update product")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeEditProduct, fiber.Map{
		"id":     id,
		"update": base,
		"user":   c.Locals("user").(string),
	}, p.ActivitiesCollection)

	log.Info().Str("id", id).Msg("Successfully updated product")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		[]models.Product{*product},
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

	// The original controller returned 200 even when no document matched, so a
	// missing document (repoerr.ErrNotFound) is intentionally ignored here; only
	// genuine errors surface as 500.
	if err := p.Repo.Delete(c.Context(), id); err != nil && !errors.Is(err, repoerr.ErrNotFound) {
		log.Error().Err(err).Msg("Failed to delete product")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
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

	product, err := p.Repo.GetByID(c.Context(), id)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Product not found")
		return models.RespondError(c, fiber.StatusNotFound, "Product not found")
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
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	products, err := p.Repo.Query(c.Context(), params)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query products")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().Int("count", len(products)).Msg("Successfully queried products")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		products,
	))
}
