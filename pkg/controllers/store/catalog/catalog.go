package storecatalog

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	catalogrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/catalog"
	productrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/product"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Controller handles the public catalog browse surface (categories, brands,
// search). Category/brand reads go through Catalog; product listing (category
// products, search) reuses the product repository.
type Controller struct {
	Catalog  *catalogrepo.CatalogRepository
	Products *productrepo.ProductRepository
}

func New(db *gorm.DB) *Controller {
	return &Controller{
		Catalog:  catalogrepo.New(db),
		Products: productrepo.New(db),
	}
}

// GetCategories godoc
// @Summary List catalog categories
// @Tags store-catalog
// @Produce json
// @Success 200 {object} models.Output[[]models.Category]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/categories [get]
func (ctrl *Controller) GetCategories(c *fiber.Ctx) error {
	categories, err := ctrl.Catalog.GetCategories(c.Context())
	if err != nil {
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(models.NewOutput(categories))
}

// GetCategoryByID godoc
// @Summary Get a category by id
// @Tags store-catalog
// @Param id path string true "Category ID"
// @Produce json
// @Success 200 {object} models.Output[models.Category]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/categories/{id} [get]
func (ctrl *Controller) GetCategoryByID(c *fiber.Ctx) error {
	category, err := ctrl.Catalog.GetCategoryByID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(category))
}

// GetCategoryProducts godoc
// @Summary List products in a category
// @Tags store-catalog
// @Param id path string true "Category ID"
// @Param page query int false "Page number"
// @Param count query int false "Items per page"
// @Produce json
// @Success 200 {object} models.Output[models.PagedProducts]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/categories/{id}/products [get]
func (ctrl *Controller) GetCategoryProducts(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	count := c.QueryInt("count", 20)
	products, err := ctrl.Products.ListByCategory(c.Context(), c.Params("id"), page, count)
	if err != nil {
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(models.NewOutput(products))
}

// GetBrands godoc
// @Summary List brands / manufacturers
// @Tags store-catalog
// @Produce json
// @Success 200 {object} models.Output[[]models.Brand]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/brands [get]
func (ctrl *Controller) GetBrands(c *fiber.Ctx) error {
	brands, err := ctrl.Catalog.GetBrands(c.Context())
	if err != nil {
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(models.NewOutput(brands))
}

// Search godoc
// @Summary Search the catalog
// @Description Full-text-ish search with filters, sort and pagination over products
// @Tags store-catalog
// @Param q query string false "Search query"
// @Param category query string false "Category id"
// @Param brand query string false "Brand id"
// @Param price_min query number false "Minimum price"
// @Param price_max query number false "Maximum price"
// @Param sort query string false "Sort (newest, name_asc, name_desc, rating_desc, price_asc, price_desc)"
// @Param page query int false "Page number"
// @Param count query int false "Items per page"
// @Produce json
// @Success 200 {object} models.Output[models.PagedProducts]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/search [get]
func (ctrl *Controller) Search(c *fiber.Ctx) error {
	params := models.ProductListParams{}
	if err := c.QueryParser(&params); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	products, err := ctrl.Products.List(c.Context(), params)
	if err != nil {
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(models.NewOutput(products))
}
