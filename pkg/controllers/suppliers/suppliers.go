package suppliers

import (
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	suppliersrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/suppliers"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// SuppliersController exposes the admin suppliers endpoints. All database access
// goes through Repo; the controller only parses requests, logs audit activity,
// and renders the response envelope.
type SuppliersController struct {
	Repo                 *suppliersrepo.SuppliersRepository
	activitiesCollection *mongo.Collection
}

func New(db *mongo.Database) *SuppliersController {
	return &SuppliersController{
		Repo:                 suppliersrepo.New(db),
		activitiesCollection: db.Collection("activities"),
	}
}

// GetSuppliers godoc
// @Security BearerAuth
// @Summary Get all suppliers
// @Description Get all suppliers from the database
// @Tags suppliers
// @Accept json
// @Produce json
// @Param name query string false "Supplier name"
// @Param inn query string false "Supplier INN"
// @Param branch query string false "Supplier branch"
// @Param email query string false "Supplier email"
// @Param phone query string false "Supplier phone"
// @Param address query string false "Supplier address"
// @Param notes query string false "Supplier notes"
// @Success 200 {object} models.Output[[]models.Supplier]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers [get]
func (s *SuppliersController) GetSuppliers(c *fiber.Ctx) error {
	var query models.SupplierQueryParams
	if err := c.QueryParser(&query); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	suppliers, err := s.Repo.Find(c.Context(), query)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(suppliers))
}

// GetSupplierByID godoc
// @Security BearerAuth
// @Summary Get a supplier by ID
// @Description Get a supplier by its ID
// @Tags suppliers
// @Accept json
// @Produce json
// @Param id path string true "Supplier ID"
// @Success 200 {object} models.Output[models.Supplier]
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{id} [get]
func (s *SuppliersController) GetSupplierByID(c *fiber.Ctx) error {
	supplier, err := s.Repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(supplier))
}

// CreateSupplier godoc
// @Security BearerAuth
// @Summary Create a new supplier
// @Description Create a new supplier in the database
// @Tags suppliers
// @Accept json
// @Produce json
// @Param supplier body models.SupplierBase true "Supplier data"
// @Success 201 {object} models.Output[models.Supplier]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers [post]
func (s *SuppliersController) CreateSupplier(c *fiber.Ctx) error {
	var base models.SupplierBase
	if err := c.BodyParser(&base); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	supplier, err := s.Repo.Create(c.Context(), base)
	if err != nil {
		return models.RespondRepoError(c, err)
	}

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateSupplier, supplier, s.activitiesCollection)
	log.Debug().Str("id", supplier.ID).Msg("Successfully created supplier")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(supplier))
}

// UpdateSupplier godoc
// @Security BearerAuth
// @Summary Update a supplier
// @Description Update a supplier's information
// @Tags suppliers
// @Accept json
// @Produce json
// @Param id path string true "Supplier ID"
// @Param supplier body models.SupplierBase true "Supplier data"
// @Success 200 {object} models.MessageResponse
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{id} [put]
func (s *SuppliersController) UpdateSupplier(c *fiber.Ctx) error {
	id := c.Params("id")
	var base models.SupplierBase
	if err := c.BodyParser(&base); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	if err := s.Repo.Update(c.Context(), id, base); err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(models.MessageResponse{Message: "Supplier updated successfully"}))
}

// DeleteSupplier godoc
// @Security BearerAuth
// @Summary Delete a supplier
// @Description Delete a supplier from the database
// @Tags suppliers
// @Accept json
// @Produce json
// @Param id path string true "Supplier ID"
// @Success 200 {object} models.MessageResponse
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{id} [delete]
func (s *SuppliersController) DeleteSupplier(c *fiber.Ctx) error {
	if err := s.Repo.Delete(c.Context(), c.Params("id")); err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(models.MessageResponse{Message: "Supplier deleted successfully"}))
}
