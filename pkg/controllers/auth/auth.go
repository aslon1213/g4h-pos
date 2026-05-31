package auth

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/configs"
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"golang.org/x/crypto/bcrypt"

	pasetoware "github.com/gofiber/contrib/paseto"
	"github.com/rs/zerolog/log"
)

// AuthControllers handles authentication-related operations
type AuthControllers struct {
	UserCollection       *mongo.Collection
	ActivitiesCollection *mongo.Collection
	SecretSymmetricKey   string
	TokenExpiryHours     int
}

// New initializes a new AuthControllers instance
func New(db *mongo.Database) *AuthControllers {

	users_collection := db.Collection("users")
	users_collection.Indexes().CreateOne(
		context.Background(),
		mongo.IndexModel{
			Keys: bson.M{
				"username": 1,
			},
			Options: options.Index().SetUnique(true),
		},
	)

	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	return &AuthControllers{
		UserCollection:       users_collection,
		ActivitiesCollection: db.Collection("activities"),
		SecretSymmetricKey:   config.Server.SecretSymmetricKey,
		TokenExpiryHours:     config.Server.TokenExpiryHours,
	}
}

// User represents a user in the system
type LoginInput struct {
	Username string `json:"username" validate:"required"`
	Password string `json:"password" validate:"required"`
}

// InfoMe godoc
// @Summary Get user info
// @Description Get user info
// @Security BearerAuth
// @Tags auth
// @Produce json
// @Success 200 {object} models.MessageResponse "message"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/auth/me [get]
func (a *AuthControllers) InfoMe(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"message": "Hello, World!",
	})
}

// Login godoc
// @Summary Login a user
// @Description Authenticate user and return a token
// @Tags auth
// @Accept json
// @Produce json
// @Param user body LoginInput true "User credentials"
// @Success 200 {object} models.TokenResponse "token"
// @Failure 401 {object} models.ErrorOutput "Unauthorized"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/auth/login [post]
func (a *AuthControllers) Login(c *fiber.Ctx) error {
	var user_to_check LoginInput

	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	if err := c.BodyParser(&user_to_check); err != nil {
		log.Error().Err(err).Msg("Failed to parse request body")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Invalid request body",
		})
	}

	// Validate required fields
	if user_to_check.Username == "" || user_to_check.Password == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Username and password are required",
		})
	}

	pass := user_to_check.Password

	// check in the database if the user exists
	user_db := models.User{}
	err = a.UserCollection.FindOne(c.Context(), bson.M{"username": user_to_check.Username}).Decode(&user_db)

	if err != nil {
		log.Error().Err(err).Msg("Failed to find user")
		// no user found
		middleware.LogActivityWithCtx(c, middleware.ActivityTypeLogin, fiber.Map{
			"error": "User not found",
		}, a.ActivitiesCollection)

		return c.SendStatus(fiber.StatusUnauthorized)
	}
	c.Locals("user", user_db.Username)

	equal := bcrypt.CompareHashAndPassword([]byte(user_db.Password), []byte(pass))
	if equal != nil {
		log.Warn().Msg("Unauthorized access attempt")
		middleware.LogActivityWithCtx(c, middleware.ActivityTypeLogin, fiber.Map{
			"error": "Invalid password",
		}, a.ActivitiesCollection)
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// Create token and encrypt it
	keyBytes, err := base64.StdEncoding.DecodeString(a.SecretSymmetricKey)
	encryptedToken, err := pasetoware.CreateToken(keyBytes, user_to_check.Username, time.Duration(a.TokenExpiryHours)*time.Hour, pasetoware.PurposeLocal)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create token")

		return c.SendStatus(fiber.StatusInternalServerError)
	}

	payload, err := pasetoware.NewPayload(
		encryptedToken,
		time.Duration(config.Server.TokenExpiryHours)*time.Hour,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create payload")

		return c.SendStatus(fiber.StatusInternalServerError)
	}

	log.Info().Str("username", user_to_check.Username).Msg("User logged in successfully")
	return c.JSON(payload)
}

// Register godoc
// @Summary Register a new user
// @Description Create a new user account
// @Tags auth
// @Accept json
// @Produce json
// @Param user body models.UserRegisterInput true "User credentials"
// @Success 201 {string} string "Created"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// register endpoint disabled -- route commented out in routes.PublicAuthRoutes
// // @Router /api/v1/admin/auth/register [post]
func (a *AuthControllers) Register(c *fiber.Ctx) error {
	// Parse user input
	var user models.UserRegisterInput

	err := c.BodyParser(&user)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse user")
		return c.SendStatus(fiber.StatusInternalServerError)
	}
	// validate the user

	// log the action
	c.Locals("user", user.Username)
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeRegister, fiber.Map{
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
		"phone":    user.Phone,
		"branch":   user.Branch,
	}, a.ActivitiesCollection)

	// Hash the password using bcrypt
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("Failed to hash password")

		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// Create a new user document
	newUser := models.NewUser(user.Username, string(hashedPassword), user.Email, user.Role, user.Phone, user.Branch)

	// Insert the new user into the database
	_, err = a.UserCollection.InsertOne(c.Context(), newUser)
	if err != nil {
		log.Error().Err(err).Msg("Failed to register user")

		return c.SendStatus(fiber.StatusInternalServerError)
	}

	log.Info().Str("username", user.Username).Msg("User registered successfully")
	return c.SendStatus(fiber.StatusCreated)
}

// GetRecentActivities godoc
// @Summary Get recent activities
// @Security BearerAuth
// @Description Get the 25 most recent activities across all users
// @Tags auth
// @Produce json
// @Success 200 {object} models.Output[[]middleware.Activity]
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/activities/recent [get]
func (a *AuthControllers) GetRecentActivities(c *fiber.Ctx) error {
	log.Info().Msg("Getting recent activities")
	activities, err := a.ActivitiesCollection.Find(c.Context(), bson.M{}, options.Find().SetSort(bson.M{"date": -1}).SetLimit(25))
	if err != nil {
		log.Error().Err(err).Msg("Failed to get recent activities")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.NewError("Failed to get recent activities", fiber.StatusInternalServerError)))
	}
	var activities_output []middleware.Activity
	err = activities.All(c.Context(), &activities_output)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get recent activities")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.NewError("Failed to get recent activities", fiber.StatusInternalServerError)))
	}
	return c.JSON(models.NewOutput(
		activities_output,
	))
}

// GetActivitesOfUser godoc
// @Summary Get activities of current user
// @Security BearerAuth
// @Description Get the 25 most recent activities for the authenticated user
// @Tags auth
// @Produce json
// @Success 200 {object} models.Output[[]middleware.Activity]
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/activities/me [get]
func (a *AuthControllers) GetActivitesOfUser(c *fiber.Ctx) error {
	log.Info().Str("user", c.Locals("user").(string)).Msg("Getting activities of user")
	activities, err := a.ActivitiesCollection.Find(c.Context(), bson.M{"user_id": c.Locals("user").(string)}, options.Find().SetSort(bson.M{"date": -1}).SetLimit(25))
	if err != nil {
		log.Error().Err(err).Msg("Failed to get recent activities")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.NewError("Failed to get recent activities", fiber.StatusInternalServerError)))
	}
	var activities_output []middleware.Activity
	err = activities.All(c.Context(), &activities_output)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get recent activities")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.NewError("Failed to get recent activities", fiber.StatusInternalServerError)))
	}
	return c.JSON(models.NewOutput(
		activities_output,
	))
}
