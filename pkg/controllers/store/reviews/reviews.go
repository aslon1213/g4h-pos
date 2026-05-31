package storereviews

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	orderrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/order"
	reviewrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/review"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles writing and managing product reviews. Public review
// listing lives on the storefront products controller. Reviews are persisted
// through Reviews; Orders is available to enforce verified-purchase rules.
type Controller struct {
	Reviews *reviewrepo.ReviewRepository
	Orders  *orderrepo.OrderRepository
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		Reviews: reviewrepo.New(db),
		Orders:  orderrepo.New(db),
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
