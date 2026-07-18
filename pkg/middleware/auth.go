package middleware

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	pasetoware "github.com/gofiber/contrib/paseto"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Middlewares holds the auth guards. It resolves staff users and storefront
// customers from PostgreSQL via GORM (the generics API).
type Middlewares struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Middlewares {
	return &Middlewares{db: db}
}

func (m *Middlewares) AuthMiddleware(c *fiber.Ctx) error {

	values := c.Locals(
		pasetoware.DefaultContextKey,
	).(string)

	// The token data encodes its type: only access tokens may reach protected
	// routes. A refresh token (or a legacy raw-subject token) is rejected here.
	tokenType, username, ok := models.DecodeTokenData(values)
	if !ok || tokenType != models.TokenTypeAccess {
		log.Warn().Msg("Non-access token used on a protected route")
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// retrieve the user
	user, err := gorm.G[models.User](m.db).Where("username = ?", username).First(c.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to find user")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	// add the user to the context. "user_id" is what the activities repo reads;
	// "user" is kept for handlers that still reference it.
	c.Locals("user", user.Username)
	c.Locals("user_id", user.Username)

	return c.Next()
}

// CustomerAuthMiddleware guards the protected storefront routes (/api/v1/store/*).
// It mirrors AuthMiddleware but validates the PASETO token subject against the
// store_customers table (storefront accounts), not the staff users table.
func (m *Middlewares) CustomerAuthMiddleware(c *fiber.Ctx) error {

	subject := c.Locals(
		pasetoware.DefaultContextKey,
	).(string)

	// retrieve the storefront customer by token subject (customer id)
	customer, err := gorm.G[models.StoreCustomer](m.db).Where("id = ?", subject).First(c.Context())
	if err != nil {
		log.Error().Err(err).Msg("Failed to find store customer")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	// add the customer id to the context for downstream handlers
	c.Locals("customer", customer.ID)

	return c.Next()
}
