package storeproducts

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	productrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/product"
	reviewrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/review"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles the public storefront product browse surface (read-only
// views over the products collection). Product reads go through Products; the
// public review listing reuses the shared review repository.
type Controller struct {
	Products *productrepo.ProductRepository
	Reviews  *reviewrepo.ReviewRepository
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		Products: productrepo.New(db),
		Reviews:  reviewrepo.New(db),
	}
}

// ListProducts godoc
// @Summary List storefront products
// @Description List products with filter/sort/pagination
// @Tags store-products
// @Param page query int false "Page number"
// @Param count query int false "Items per page"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/products [get]
func (ctrl *Controller) ListProducts(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetProductByID godoc
// @Summary Get a storefront product by id
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id} [get]
func (ctrl *Controller) GetProductByID(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetProductImages godoc
// @Summary Get a product's images
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/images [get]
func (ctrl *Controller) GetProductImages(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetRelatedProducts godoc
// @Summary Get related / recommended products
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/related [get]
func (ctrl *Controller) GetRelatedProducts(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetProductAvailability godoc
// @Summary Get a product's stock availability per branch
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/availability [get]
func (ctrl *Controller) GetProductAvailability(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetProductReviews godoc
// @Summary List reviews for a product (public)
// @Tags store-products
// @Param id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/reviews [get]
func (ctrl *Controller) GetProductReviews(c *fiber.Ctx) error { return models.NotImplemented(c) }
