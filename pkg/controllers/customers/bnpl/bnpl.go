package bnpl

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	bnplrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/bnpl"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// BNPLController exposes the admin BNPL endpoints. All database access goes
// through Repo; the controller only parses requests, validates input, and
// renders the response envelope.
type BNPLController struct {
	Repo *bnplrepo.BNPLRepository
}

func New(db *mongo.Database) *BNPLController {
	return &BNPLController{
		Repo: bnplrepo.New(db),
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
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	if err := new_bnpl_input.Validate(); err != nil {
		log.Error().Err(err).Msg("Invalid input")
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	// check if the customer exists (a missing customer is a 400 here)
	if err := ctrl.Repo.CustomerExists(c.Context(), new_bnpl_input.CustomerID); err != nil {
		log.Error().Err(err).Str("customer_id", new_bnpl_input.CustomerID).Msg("Customer not found")
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
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

	bnpl, err := ctrl.Repo.Create(c.Context(), new_bnpl_input.CustomerID, new_bnpl_input.BranchID, total_amount, new_bnpl_input.Products)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update customer with new BNPL")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
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

	bnpl, err := ctrl.Repo.Credit(c.Context(), bnpl_id, amount, payment_method)
	if err != nil {
		// Every failure path here is reported as a 500 (preserved behaviour).
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to process BNPL payment")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

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

	if err := ctrl.Repo.Delete(c.Context(), bnpl_id); err != nil {
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to delete BNPL")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().Str("bnpl_id", bnpl_id).Msg("Successfully deleted BNPL")
	return c.JSON(models.NewOutput(fiber.Map{
		"message": "BNPL deleted successfully",
	}))
}

// GetBNPLByID godoc
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
	bnpl, err := ctrl.Repo.GetByID(c.Context(), bnpl_id)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnpl_id).Msg("Failed to get BNPL")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().Str("bnpl_id", bnpl_id).Msg("Successfully retrieved BNPL details")
	return c.JSON(models.NewOutput(bnpl))
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
	bnpls, err := ctrl.Repo.GetCustomerBNPLs(c.Context(), customer_id, branch_id)
	if err != nil {
		log.Error().Err(err).Str("customer_id", customer_id).Msg("Failed to get customer BNPLs")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().Str("customer_id", customer_id).Msg("Successfully retrieved customer BNPLs")
	return c.JSON(models.NewOutput(bnpls))
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

	customer_name := c.Query("customer_name")
	customer_phone := c.Query("customer_phone")
	customer_address := c.Query("customer_address")

	branch_id := c.Params("branch_id")
	output, err := ctrl.Repo.GetBranchBNPLs(c.Context(), branch_id, customer_name, customer_phone, customer_address)
	if err != nil {
		log.Error().Err(err).Str("branch_id", branch_id).Msg("Failed to get BNPLs of branch")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().Str("branch_id", branch_id).Msg("Successfully retrieved BNPLs of branch")
	return c.JSON(models.NewOutput(output))
}
