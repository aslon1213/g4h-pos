// Package catalog exposes the admin catalog-management endpoints
// (/api/v1/admin/catalog): categories and brands that storefront products are
// attached to. All Mongo access goes through the shared catalog repository
// (pkg/repository/store/catalog), the same one the storefront browse surface
// reads from, so admin writes and storefront reads stay consistent.
package catalog

import (
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	catalogrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/catalog"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// CatalogController manages categories and brands for staff. Reads/writes go
// through Repo; the controller only parses requests, logs audit activity and
// renders the response envelope.
type CatalogController struct {
	Repo                 *catalogrepo.CatalogRepository
	ActivitiesCollection *mongo.Collection
}

func New(db *mongo.Database) *CatalogController {
	return &CatalogController{
		Repo:                 catalogrepo.New(db),
		ActivitiesCollection: db.Collection("activities"),
	}
}

// ---- categories ----

// ListCategories godoc
// @Security BearerAuth
// @Summary List all catalog categories (including inactive)
// @Tags catalog
// @Produce json
// @Success 200 {object} models.Output[[]models.Category]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/categories [get]
func (ctrl *CatalogController) ListCategories(c *fiber.Ctx) error {
	categories, err := ctrl.Repo.ListAllCategories(c.Context())
	if err != nil {
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(models.NewOutput(categories))
}

// GetCategory godoc
// @Security BearerAuth
// @Summary Get a catalog category by id
// @Tags catalog
// @Param id path string true "Category ID"
// @Produce json
// @Success 200 {object} models.Output[models.Category]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/categories/{id} [get]
func (ctrl *CatalogController) GetCategory(c *fiber.Ctx) error {
	category, err := ctrl.Repo.GetCategoryByID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(category))
}

// CreateCategory godoc
// @Security BearerAuth
// @Summary Create a catalog category
// @Tags catalog
// @Accept json
// @Produce json
// @Param body body models.CategoryInput true "Category details"
// @Success 201 {object} models.Output[models.Category]
// @Failure 400 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/categories [post]
func (ctrl *CatalogController) CreateCategory(c *fiber.Ctx) error {
	var in models.CategoryInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if in.Name == "" {
		return models.RespondError(c, fiber.StatusBadRequest, "name is required")
	}
	category, err := ctrl.Repo.CreateCategory(c.Context(), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateCategory, category.ID, ctrl.ActivitiesCollection)
	log.Info().Str("id", category.ID).Msg("Category created")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(category))
}

// UpdateCategory godoc
// @Security BearerAuth
// @Summary Update a catalog category
// @Tags catalog
// @Accept json
// @Produce json
// @Param id path string true "Category ID"
// @Param body body models.CategoryInput true "Category fields to update"
// @Success 200 {object} models.Output[models.Category]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/categories/{id} [put]
func (ctrl *CatalogController) UpdateCategory(c *fiber.Ctx) error {
	var in models.CategoryInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	category, err := ctrl.Repo.UpdateCategory(c.Context(), c.Params("id"), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeEditCategory, fiber.Map{"id": c.Params("id"), "update": in}, ctrl.ActivitiesCollection)
	return c.JSON(models.NewOutput(category))
}

// DeleteCategory godoc
// @Security BearerAuth
// @Summary Delete a catalog category
// @Tags catalog
// @Param id path string true "Category ID"
// @Produce json
// @Success 200 {object} models.Output[models.MessageResponse]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/categories/{id} [delete]
func (ctrl *CatalogController) DeleteCategory(c *fiber.Ctx) error {
	if err := ctrl.Repo.DeleteCategory(c.Context(), c.Params("id")); err != nil {
		return models.RespondRepoError(c, err)
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeDeleteCategory, c.Params("id"), ctrl.ActivitiesCollection)
	return c.JSON(models.NewOutput(models.MessageResponse{Message: "category deleted"}))
}

// ---- brands ----

// ListBrands godoc
// @Security BearerAuth
// @Summary List all catalog brands
// @Tags catalog
// @Produce json
// @Success 200 {object} models.Output[[]models.Brand]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/brands [get]
func (ctrl *CatalogController) ListBrands(c *fiber.Ctx) error {
	brands, err := ctrl.Repo.GetBrands(c.Context())
	if err != nil {
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(models.NewOutput(brands))
}

// GetBrand godoc
// @Security BearerAuth
// @Summary Get a catalog brand by id
// @Tags catalog
// @Param id path string true "Brand ID"
// @Produce json
// @Success 200 {object} models.Output[models.Brand]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/brands/{id} [get]
func (ctrl *CatalogController) GetBrand(c *fiber.Ctx) error {
	brand, err := ctrl.Repo.GetBrandByID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(brand))
}

// CreateBrand godoc
// @Security BearerAuth
// @Summary Create a catalog brand
// @Tags catalog
// @Accept json
// @Produce json
// @Param body body models.BrandInput true "Brand details"
// @Success 201 {object} models.Output[models.Brand]
// @Failure 400 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/brands [post]
func (ctrl *CatalogController) CreateBrand(c *fiber.Ctx) error {
	var in models.BrandInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if in.Name == "" {
		return models.RespondError(c, fiber.StatusBadRequest, "name is required")
	}
	brand, err := ctrl.Repo.CreateBrand(c.Context(), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateBrand, brand.ID, ctrl.ActivitiesCollection)
	log.Info().Str("id", brand.ID).Msg("Brand created")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(brand))
}

// UpdateBrand godoc
// @Security BearerAuth
// @Summary Update a catalog brand
// @Tags catalog
// @Accept json
// @Produce json
// @Param id path string true "Brand ID"
// @Param body body models.BrandInput true "Brand fields to update"
// @Success 200 {object} models.Output[models.Brand]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/brands/{id} [put]
func (ctrl *CatalogController) UpdateBrand(c *fiber.Ctx) error {
	var in models.BrandInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	brand, err := ctrl.Repo.UpdateBrand(c.Context(), c.Params("id"), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeEditBrand, fiber.Map{"id": c.Params("id"), "update": in}, ctrl.ActivitiesCollection)
	return c.JSON(models.NewOutput(brand))
}

// DeleteBrand godoc
// @Security BearerAuth
// @Summary Delete a catalog brand
// @Tags catalog
// @Param id path string true "Brand ID"
// @Produce json
// @Success 200 {object} models.Output[models.MessageResponse]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/catalog/brands/{id} [delete]
func (ctrl *CatalogController) DeleteBrand(c *fiber.Ctx) error {
	if err := ctrl.Repo.DeleteBrand(c.Context(), c.Params("id")); err != nil {
		return models.RespondRepoError(c, err)
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeDeleteBrand, c.Params("id"), ctrl.ActivitiesCollection)
	return c.JSON(models.NewOutput(models.MessageResponse{Message: "brand deleted"}))
}
