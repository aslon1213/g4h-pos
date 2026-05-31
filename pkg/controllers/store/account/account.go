package storeaccount

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	customerrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/customer"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles the storefront customer account surface (address book,
// etc). Address-book operations are served by the customer repository.
type Controller struct {
	Customers *customerrepo.CustomerRepository
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		Customers: customerrepo.New(db),
	}
}

// GetAddresses godoc
// @Summary List the customer's saved addresses
// @Tags store-account
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/account/addresses [get]
func (ctrl *Controller) GetAddresses(c *fiber.Ctx) error { return models.NotImplemented(c) }

// CreateAddress godoc
// @Summary Add a new address to the customer's address book
// @Tags store-account
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/account/addresses [post]
func (ctrl *Controller) CreateAddress(c *fiber.Ctx) error { return models.NotImplemented(c) }

// UpdateAddress godoc
// @Summary Update an address
// @Tags store-account
// @Security BearerAuth
// @Param address_id path string true "Address ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/account/addresses/{address_id} [put]
func (ctrl *Controller) UpdateAddress(c *fiber.Ctx) error { return models.NotImplemented(c) }

// DeleteAddress godoc
// @Summary Delete an address
// @Tags store-account
// @Security BearerAuth
// @Param address_id path string true "Address ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/account/addresses/{address_id} [delete]
func (ctrl *Controller) DeleteAddress(c *fiber.Ctx) error { return models.NotImplemented(c) }

// SetDefaultAddress godoc
// @Summary Mark an address as the default
// @Tags store-account
// @Security BearerAuth
// @Param address_id path string true "Address ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/account/addresses/{address_id}/default [put]
func (ctrl *Controller) SetDefaultAddress(c *fiber.Ctx) error { return models.NotImplemented(c) }
