package suppliers

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
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
// @Success 201 {object} models.Output[models.Transaction]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{branch_id}/{supplier_id}/transactions [post]
func (s *SuppliersController) NewTransaction(c *fiber.Ctx) error {
	// if a supplier gets money, that is when we paid money to them, so the
	// supplier's balance rises and the branch's debt/balance drop; if a supplier
	// pays money (delivers value), the supplier's balance drops and the branch's
	// debt rises. The repository applies all of this atomically.
	branchID := c.Params("branch_id")
	supplierID := c.Params("supplier_id")

	var base models.TransactionBase
	if err := c.BodyParser(&base); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	if err := models.ValidateTransactionType(base.Type); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	log.Info().Str("branch_id", branchID).Str("supplier_id", supplierID).Str("type", string(base.Type)).Msg("Starting new supplier transaction")
	transaction, err := s.Repo.CreateTransaction(c.Context(), branchID, supplierID, base)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(transaction))
}
