package middleware

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	pasetoware "github.com/gofiber/contrib/paseto"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Middlewares struct {
	UserCollection           *mongo.Collection
	ActivitiesCollection     *mongo.Collection
	StoreCustomersCollection *mongo.Collection
}

func New(db *mongo.Database) *Middlewares {
	return &Middlewares{
		UserCollection:           db.Collection("users"),
		ActivitiesCollection:     db.Collection("activities"),
		StoreCustomersCollection: db.Collection("store_customers"),
	}
}

func (m *Middlewares) AuthMiddleware(c *fiber.Ctx) error {

	values := c.Locals(
		pasetoware.DefaultContextKey,
	).(string)

	// got username
	username := values

	// retreive the user
	user := &models.User{}
	err := m.UserCollection.FindOne(c.Context(), bson.M{"username": username}).Decode(user)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find user")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	// add the user to the context
	c.Locals("user", user.Username)

	return c.Next()
}

// CustomerAuthMiddleware guards the protected storefront routes (/api/v1/store/*).
// It mirrors AuthMiddleware but validates the PASETO token subject against the
// store_customers collection (storefront accounts), not the staff users collection.
func (m *Middlewares) CustomerAuthMiddleware(c *fiber.Ctx) error {

	subject := c.Locals(
		pasetoware.DefaultContextKey,
	).(string)

	// retrieve the storefront customer by token subject (customer id)
	customer := &models.StoreCustomer{}
	err := m.StoreCustomersCollection.FindOne(c.Context(), bson.M{"_id": subject}).Decode(customer)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find store customer")
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	// add the customer id to the context for downstream handlers
	c.Locals("customer", customer.ID)

	return c.Next()
}
