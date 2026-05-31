package storereviews

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	customerrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/customer"
	orderrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/order"
	reviewrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/review"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles writing and managing product reviews. Public review
// listing lives on the storefront products controller. Reviews are persisted
// through Reviews; Customers resolves the reviewer's display name; Orders is
// available to enforce verified-purchase rules.
type Controller struct {
	Reviews   *reviewrepo.ReviewRepository
	Customers *customerrepo.CustomerRepository
	Orders    *orderrepo.OrderRepository
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		Reviews:   reviewrepo.New(db),
		Customers: customerrepo.New(db),
		Orders:    orderrepo.New(db),
	}
}

// validRating reports whether a star rating is within the allowed 1..5 range.
func validRating(rating int) bool { return rating >= 1 && rating <= 5 }

// CreateReview godoc
// @Summary Create a review for a product
// @Tags store-reviews
// @Security BearerAuth
// @Accept json
// @Param id path string true "Product ID"
// @Param body body models.CreateReviewInput true "Review details"
// @Produce json
// @Success 201 {object} models.Output[models.Review]
// @Failure 400 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/store/products/{id}/reviews [post]
func (ctrl *Controller) CreateReview(c *fiber.Ctx) error {
	customerID := c.Locals("customer").(string)
	productID := c.Params("id")

	var in models.CreateReviewInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if !validRating(in.Rating) {
		return models.RespondError(c, fiber.StatusBadRequest, "rating must be between 1 and 5")
	}

	// Resolve the customer's display name (best-effort).
	name := ""
	if customer, err := ctrl.Customers.GetByID(c.Context(), customerID); err == nil {
		name = customer.Name
	}

	review := &models.Review{
		ProductID:    productID,
		CustomerID:   customerID,
		CustomerName: name,
		Rating:       in.Rating,
		Title:        in.Title,
		Body:         in.Body,
	}
	created, err := ctrl.Reviews.Create(c.Context(), review)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(created))
}

// UpdateReview godoc
// @Summary Update the caller's own review
// @Tags store-reviews
// @Security BearerAuth
// @Accept json
// @Param id path string true "Review ID"
// @Param body body models.UpdateReviewInput true "Review fields to update"
// @Produce json
// @Success 200 {object} models.Output[models.Review]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/reviews/{id} [put]
func (ctrl *Controller) UpdateReview(c *fiber.Ctx) error {
	customerID := c.Locals("customer").(string)
	var in models.UpdateReviewInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if !validRating(in.Rating) {
		return models.RespondError(c, fiber.StatusBadRequest, "rating must be between 1 and 5")
	}
	review, err := ctrl.Reviews.Update(c.Context(), customerID, c.Params("id"), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(review))
}

// DeleteReview godoc
// @Summary Delete the caller's own review
// @Tags store-reviews
// @Security BearerAuth
// @Param id path string true "Review ID"
// @Produce json
// @Success 200 {object} models.Output[models.MessageResponse]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/reviews/{id} [delete]
func (ctrl *Controller) DeleteReview(c *fiber.Ctx) error {
	customerID := c.Locals("customer").(string)
	if err := ctrl.Reviews.Delete(c.Context(), customerID, c.Params("id")); err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(models.MessageResponse{Message: "review deleted"}))
}

// VoteReview godoc
// @Summary Vote a review helpful / unhelpful
// @Tags store-reviews
// @Security BearerAuth
// @Accept json
// @Param id path string true "Review ID"
// @Param body body models.VoteReviewInput true "Vote"
// @Produce json
// @Success 200 {object} models.Output[models.Review]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/store/reviews/{id}/vote [post]
func (ctrl *Controller) VoteReview(c *fiber.Ctx) error {
	customerID := c.Locals("customer").(string)
	var in models.VoteReviewInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	review, err := ctrl.Reviews.Vote(c.Context(), customerID, c.Params("id"), in.Helpful)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(review))
}
