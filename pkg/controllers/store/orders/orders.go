package storeorders

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	cartrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/cart"
	orderrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/order"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles checkout and the customer's orders. Orders are persisted
// through Orders; checkout/reorder reads the cart through Cart.
type Controller struct {
	Orders *orderrepo.OrderRepository
	Cart   *cartrepo.CartRepository
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		Orders: orderrepo.New(db),
		Cart:   cartrepo.New(db),
	}
}

// CheckoutPreview godoc
// @Summary Preview checkout totals (items, shipping, tax) before placing an order
// @Tags store-orders
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/checkout/preview [post]
func (ctrl *Controller) CheckoutPreview(c *fiber.Ctx) error { return models.NotImplemented(c) }

// PlaceOrder godoc
// @Summary Place an order from the current cart
// @Tags store-orders
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/orders [post]
func (ctrl *Controller) PlaceOrder(c *fiber.Ctx) error { return models.NotImplemented(c) }

// ListOrders godoc
// @Summary List the customer's orders
// @Tags store-orders
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/orders [get]
func (ctrl *Controller) ListOrders(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetOrderByID godoc
// @Summary Get an order by id
// @Tags store-orders
// @Security BearerAuth
// @Param id path string true "Order ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/orders/{id} [get]
func (ctrl *Controller) GetOrderByID(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetOrderStatus godoc
// @Summary Get an order's status / tracking
// @Tags store-orders
// @Security BearerAuth
// @Param id path string true "Order ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/orders/{id}/status [get]
func (ctrl *Controller) GetOrderStatus(c *fiber.Ctx) error { return models.NotImplemented(c) }

// CancelOrder godoc
// @Summary Cancel an order
// @Tags store-orders
// @Security BearerAuth
// @Param id path string true "Order ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/orders/{id}/cancel [post]
func (ctrl *Controller) CancelOrder(c *fiber.Ctx) error { return models.NotImplemented(c) }

// ReorderOrder godoc
// @Summary Re-add the items of a past order to the cart
// @Tags store-orders
// @Security BearerAuth
// @Param id path string true "Order ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/orders/{id}/reorder [post]
func (ctrl *Controller) ReorderOrder(c *fiber.Ctx) error { return models.NotImplemented(c) }
