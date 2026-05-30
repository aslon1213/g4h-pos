package storeauth

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles storefront customer authentication against the
// store_customers collection (separate from staff auth).
type Controller struct {
	StoreCustomersCollection *mongo.Collection
	ActivitiesCollection     *mongo.Collection
	DB                       *mongo.Database
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		StoreCustomersCollection: db.Collection("store_customers"),
		ActivitiesCollection:     db.Collection("activities"),
		DB:                       db,
	}
}

// Register godoc
// @Summary Register a storefront customer
// @Description Create a new storefront customer account
// @Tags store-auth
// @Accept json
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/register [post]
func (ctrl *Controller) Register(c *fiber.Ctx) error { return models.NotImplemented(c) }

// Login godoc
// @Summary Login a storefront customer
// @Description Authenticate a storefront customer and return a token
// @Tags store-auth
// @Accept json
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/login [post]
func (ctrl *Controller) Login(c *fiber.Ctx) error { return models.NotImplemented(c) }

// RefreshToken godoc
// @Summary Refresh a storefront access token
// @Tags store-auth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/refresh [post]
func (ctrl *Controller) RefreshToken(c *fiber.Ctx) error { return models.NotImplemented(c) }

// ForgotPassword godoc
// @Summary Request a password reset
// @Tags store-auth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/password/forgot [post]
func (ctrl *Controller) ForgotPassword(c *fiber.Ctx) error { return models.NotImplemented(c) }

// ResetPassword godoc
// @Summary Reset a password with a reset token
// @Tags store-auth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/password/reset [post]
func (ctrl *Controller) ResetPassword(c *fiber.Ctx) error { return models.NotImplemented(c) }

// Me godoc
// @Summary Get current storefront customer profile
// @Tags store-auth
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/me [get]
func (ctrl *Controller) Me(c *fiber.Ctx) error { return models.NotImplemented(c) }

// UpdateMe godoc
// @Summary Update current storefront customer profile
// @Tags store-auth
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/me [put]
func (ctrl *Controller) UpdateMe(c *fiber.Ctx) error { return models.NotImplemented(c) }

// Logout godoc
// @Summary Logout the current storefront customer
// @Tags store-auth
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/auth/logout [post]
func (ctrl *Controller) Logout(c *fiber.Ctx) error { return models.NotImplemented(c) }
