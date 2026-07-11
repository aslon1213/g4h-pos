package storeproducts

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	productrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/product"
	reviewrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/review"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Controller handles the public storefront product browse surface (read-only
// views over the products collection). Product reads go through Products; the
// public review listing reuses the shared review repository.
type Controller struct {
	Products *productrepo.ProductRepository
	Reviews  *reviewrepo.ReviewRepository
}

func New(db *gorm.DB) *Controller {
	return &Controller{
		Products: productrepo.New(db),
		Reviews:  reviewrepo.New(db),
	}
}

// ListProducts godoc
// @Summary List storefront products
// @Description List products with filter/sort/pagination. Each product carries
// @Description its attached catalog info (categories + brand) and review aggregates.
// @Tags store-products
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
// @Router /api/v1/store/products [get]
func (ctrl *Controller) ListProducts(c *fiber.Ctx) error {
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

// GetProductByID godoc
// @Summary Get a storefront product by id
// @Description Returns the product with its catalog info (categories + brand) and
// @Description a handful of recent reviews attached.
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 200 {object} models.Output[models.Product]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id} [get]
func (ctrl *Controller) GetProductByID(c *fiber.Ctx) error {
	product, err := ctrl.Products.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(product))
}

// GetProductImages godoc
// @Summary Get a product's images
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 200 {object} models.Output[[]string]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/images [get]
func (ctrl *Controller) GetProductImages(c *fiber.Ctx) error {
	images, err := ctrl.Products.GetImages(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(images))
}

// GetRelatedProducts godoc
// @Summary Get related / recommended products
// @Tags store-products
// @Param id path string true "Product ID"
// @Param count query int false "Max related products"
// @Produce json
// @Success 200 {object} models.Output[[]models.Product]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/related [get]
func (ctrl *Controller) GetRelatedProducts(c *fiber.Ctx) error {
	limit := c.QueryInt("count", 8)
	related, err := ctrl.Products.GetRelated(c.Context(), c.Params("id"), limit)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(related))
}

// GetProductAvailability godoc
// @Summary Get a product's stock availability per branch
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 200 {object} models.Output[[]models.BranchAvailability]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/availability [get]
func (ctrl *Controller) GetProductAvailability(c *fiber.Ctx) error {
	availability, err := ctrl.Products.GetAvailability(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(availability))
}

// GetProductReviews godoc
// @Summary List reviews for a product (public)
// @Tags store-products
// @Param id path string true "Product ID"
// @Param page query int false "Page number"
// @Param count query int false "Items per page"
// @Produce json
// @Success 200 {object} models.Output[models.PagedReviews]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/reviews [get]
func (ctrl *Controller) GetProductReviews(c *fiber.Ctx) error {
	page := c.QueryInt("page", 1)
	count := c.QueryInt("count", 20)
	reviews, err := ctrl.Reviews.ListByProduct(c.Context(), c.Params("id"), page, count)
	if err != nil {
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.JSON(models.NewOutput(reviews))
}
