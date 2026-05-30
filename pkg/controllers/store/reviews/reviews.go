package storereviews

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles writing and managing product reviews. Public review
// listing lives on the storefront products controller.
type Controller struct {
	ReviewsCollection  *mongo.Collection
	ProductsCollection *mongo.Collection
	OrdersCollection   *mongo.Collection
	DB                 *mongo.Database
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		ReviewsCollection:  db.Collection("reviews"),
		ProductsCollection: db.Collection("products"),
		OrdersCollection:   db.Collection("orders"),
		DB:                 db,
	}
}

// CreateReview godoc
// @Summary Create a review for a product
// @Tags store-reviews
// @Security BearerAuth
// @Param id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/reviews [post]
func (ctrl *Controller) CreateReview(c *fiber.Ctx) error { return models.NotImplemented(c) }

// UpdateReview godoc
// @Summary Update the caller's own review
// @Tags store-reviews
// @Security BearerAuth
// @Param id path string true "Review ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/reviews/{id} [put]
func (ctrl *Controller) UpdateReview(c *fiber.Ctx) error { return models.NotImplemented(c) }

// DeleteReview godoc
// @Summary Delete the caller's own review
// @Tags store-reviews
// @Security BearerAuth
// @Param id path string true "Review ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/reviews/{id} [delete]
func (ctrl *Controller) DeleteReview(c *fiber.Ctx) error { return models.NotImplemented(c) }

// VoteReview godoc
// @Summary Vote a review helpful / unhelpful
// @Tags store-reviews
// @Security BearerAuth
// @Param id path string true "Review ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/reviews/{id}/vote [post]
func (ctrl *Controller) VoteReview(c *fiber.Ctx) error { return models.NotImplemented(c) }
