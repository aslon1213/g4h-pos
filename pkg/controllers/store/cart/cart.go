package storecart

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles the authenticated customer's shopping cart.
type Controller struct {
	CartsCollection    *mongo.Collection
	ProductsCollection *mongo.Collection
	DB                 *mongo.Database
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		CartsCollection:    db.Collection("carts"),
		ProductsCollection: db.Collection("products"),
		DB:                 db,
	}
}

// GetCart godoc
// @Summary Get the current customer's cart
// @Tags store-cart
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/cart [get]
func (ctrl *Controller) GetCart(c *fiber.Ctx) error { return models.NotImplemented(c) }

// AddItem godoc
// @Summary Add an item to the cart
// @Tags store-cart
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/cart/items [post]
func (ctrl *Controller) AddItem(c *fiber.Ctx) error { return models.NotImplemented(c) }

// UpdateItem godoc
// @Summary Update a cart item's quantity
// @Tags store-cart
// @Security BearerAuth
// @Param item_id path string true "Cart item ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/cart/items/{item_id} [put]
func (ctrl *Controller) UpdateItem(c *fiber.Ctx) error { return models.NotImplemented(c) }

// RemoveItem godoc
// @Summary Remove an item from the cart
// @Tags store-cart
// @Security BearerAuth
// @Param item_id path string true "Cart item ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/cart/items/{item_id} [delete]
func (ctrl *Controller) RemoveItem(c *fiber.Ctx) error { return models.NotImplemented(c) }

// ClearCart godoc
// @Summary Empty the cart
// @Tags store-cart
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/cart [delete]
func (ctrl *Controller) ClearCart(c *fiber.Ctx) error { return models.NotImplemented(c) }

// ApplyPromo godoc
// @Summary Apply a coupon code to the cart
// @Tags store-cart
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/cart/promo [post]
func (ctrl *Controller) ApplyPromo(c *fiber.Ctx) error { return models.NotImplemented(c) }

// RemovePromo godoc
// @Summary Remove the applied coupon from the cart
// @Tags store-cart
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/cart/promo [delete]
func (ctrl *Controller) RemovePromo(c *fiber.Ctx) error { return models.NotImplemented(c) }
