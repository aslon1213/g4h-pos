package storecatalog

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	catalogrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/catalog"
	productrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/product"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles the public catalog browse surface (categories, brands,
// search). Category/brand reads go through Catalog; product listing (category
// products, search) reuses the product repository.
type Controller struct {
	Catalog  *catalogrepo.CatalogRepository
	Products *productrepo.ProductRepository
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		Catalog:  catalogrepo.New(db),
		Products: productrepo.New(db),
	}
}

// GetCategories godoc
// @Summary List catalog categories
// @Tags store-catalog
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/categories [get]
func (ctrl *Controller) GetCategories(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetCategoryByID godoc
// @Summary Get a category by id
// @Tags store-catalog
// @Param id path string true "Category ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/categories/{id} [get]
func (ctrl *Controller) GetCategoryByID(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetCategoryProducts godoc
// @Summary List products in a category
// @Tags store-catalog
// @Param id path string true "Category ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/categories/{id}/products [get]
func (ctrl *Controller) GetCategoryProducts(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetBrands godoc
// @Summary List brands / manufacturers
// @Tags store-catalog
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/brands [get]
func (ctrl *Controller) GetBrands(c *fiber.Ctx) error { return models.NotImplemented(c) }

// Search godoc
// @Summary Search the catalog
// @Description Full-text search with filters, facets, sort and pagination
// @Tags store-catalog
// @Param q query string false "Search query"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/catalog/search [get]
func (ctrl *Controller) Search(c *fiber.Ctx) error { return models.NotImplemented(c) }
