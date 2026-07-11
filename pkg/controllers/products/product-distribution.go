package products

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	activities_repo "github.com/aslon1213/g4h_pos_erp/pkg/repository/activities"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
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
		return models.RespondError(c, fiber.StatusBadRequest, "Invalid request body format")
	}

	// Validate required fields
	if input.Quantity <= 0 {
		return models.RespondError(c, fiber.StatusBadRequest, "Quantity must be greater than 0")
	}

	if input.Price <= 0 {
		return models.RespondError(c, fiber.StatusBadRequest, "Price must be greater than 0")
	}

	if input.SellingPrice <= 0 {
		return models.RespondError(c, fiber.StatusBadRequest, "Selling price must be greater than 0")
	}

	if input.UploadedTo.ID == "" {
		return models.RespondError(c, fiber.StatusBadRequest, "Upload location ID is required")
	}

	log.Debug().Str("product_id", product_id).Interface("input", input).Msg("Processing income for product")

	// The repository runs the full income flow (quantity-distribution update +
	// supplier transaction) inside a single MongoDB multi-document transaction.
	product, err := p.Repo.AddIncome(c.Context(), product_id, &input.IncomeHistory, input.SellingPrice)
	if err != nil {
		// The original controller surfaced every failure here (including a
		// missing product) as HTTP 500 via models.ReturnError /
		// AbortTransactionAndReturnError; preserve that status exactly.
		log.Error().Err(err).Str("product_id", product_id).Msg("Failed to process new income")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	// log activity
	p.ActivitiesRepo.LogActivityWithCtx(c, activities_repo.ActivityTypeProductIncome, fiber.Map{
		"product_id": product_id,
		"input":      input,
	})

	log.Info().Str("product_id", product_id).Msg("Successfully processed new income")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		[]models.Product{*product},
	))
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
	p.ActivitiesRepo.LogActivityWithCtx(c, activities_repo.ActivityTypeProductTransfer, fiber.Map{})

	return models.NotImplemented(c)
}
