package auth

import (
	"github.com/aslon1213/g4h_pos_erp/pkg/configs"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	activities_repo "github.com/aslon1213/g4h_pos_erp/pkg/repository/activities"
	users_repository "github.com/aslon1213/g4h_pos_erp/pkg/repository/users"
	"github.com/gofiber/fiber/v2"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"

	"github.com/rs/zerolog/log"
)

// AuthControllers handles authentication-related operations
type AuthControllers struct {
	AdminUserRepo           *users_repository.AdminUserRepository
	ActivitiesRepo          *activities_repo.ActivitiesRepo
	SecretSymmetricKey      string
	TokenExpiryHours        int
	RefreshTokenExpiryHours int
}

// New initializes a new AuthControllers instance
func New(db *gorm.DB) *AuthControllers {

	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	return &AuthControllers{
		AdminUserRepo:           users_repository.NewAdminUserRepository(db),
		ActivitiesRepo:          activities_repo.New(db),
		SecretSymmetricKey:      config.Server.SecretSymmetricKey,
		TokenExpiryHours:        config.Server.TokenExpiryHours,
		RefreshTokenExpiryHours: config.Server.RefreshTokenExpiryHours,
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
// @Description Authenticate user and return an access + refresh token pair
// @Tags auth
// @Accept json
// @Produce json
// @Param user body LoginInput true "User credentials"
// @Success 200 {object} models.TokenPairResponse "access + refresh tokens"
// @Failure 401 {object} models.ErrorOutput "Unauthorized"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/auth/login [post]
func (a *AuthControllers) Login(c *fiber.Ctx) error {
	var user_to_check LoginInput

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
	user_db := &models.User{}
	user_db, err := a.AdminUserRepo.GetByName(user_to_check.Username)

	if err != nil {
		log.Error().Err(err).Msg("Failed to find user")
		// no user found
		a.ActivitiesRepo.LogActivityWithCtx(c, activities_repo.ActivityTypeLogin, fiber.Map{
			"error": "User not found",
		})
		return c.SendStatus(fiber.StatusUnauthorized)
	}
	c.Locals("user", user_db.Username)

	equal := bcrypt.CompareHashAndPassword([]byte(user_db.Password), []byte(pass))
	if equal != nil {
		log.Warn().Msg("Unauthorized access attempt")
		a.ActivitiesRepo.LogActivityWithCtx(c, activities_repo.ActivityTypeLogin, fiber.Map{
			"error": "Invalid password",
		})
		return c.SendStatus(fiber.StatusUnauthorized)
	}

	// Issue a fresh access + refresh token pair for the user.
	tokens, err := a.createTokenPair(user_db.Username)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create token pair")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	log.Info().Str("username", user_to_check.Username).Msg("User logged in successfully")
	return c.JSON(tokens)
}

// RefreshInput is the request body for the refresh endpoint.
type RefreshInput struct {
	RefreshToken string `json:"refresh_token" validate:"required"`
}

// Refresh godoc
// @Summary Refresh the token pair
// @Description Exchange a valid refresh token for a brand-new access + refresh token pair (the old refresh token is rotated out). This endpoint is NOT guarded by the access-token middleware — it authenticates via the refresh token in the body. Once the refresh token itself expires the request is rejected with 401 and the user must log in again.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body RefreshInput true "Refresh token"
// @Success 200 {object} models.TokenPairResponse "new access + refresh tokens"
// @Failure 400 {object} models.ErrorOutput "Bad Request"
// @Failure 401 {object} models.ErrorOutput "Unauthorized"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/auth/refresh [post]
func (a *AuthControllers) Refresh(c *fiber.Ctx) error {
	var input RefreshInput

	// Accept the refresh token from the JSON body, falling back to the
	// Authorization header for convenience.
	if err := c.BodyParser(&input); err != nil {
		log.Error().Err(err).Msg("Failed to parse refresh request body")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewErrorOutput(models.NewError("Invalid request body", fiber.StatusBadRequest)))
	}
	if input.RefreshToken == "" {
		input.RefreshToken = c.Get(fiber.HeaderAuthorization)
	}
	if input.RefreshToken == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewErrorOutput(models.NewError("refresh_token is required", fiber.StatusBadRequest)))
	}

	// Validate the refresh token and extract the subject (username).
	username, err := a.parseRefreshToken(input.RefreshToken)
	if err != nil {
		// Both an invalid and an expired refresh token mean the client can no
		// longer refresh and must log in again (fully logged out).
		log.Warn().Err(err).Msg("Refresh token rejected")
		return c.Status(fiber.StatusUnauthorized).JSON(models.NewErrorOutput(models.NewError(err.Error(), fiber.StatusUnauthorized)))
	}

	// Ensure the user still exists (e.g. not deleted since the token was issued).
	user_db := &models.User{}
	user_db, err = a.AdminUserRepo.GetByName(username)
	if err != nil {
		log.Warn().Err(err).Str("username", username).Msg("Refresh for unknown user")
		return c.Status(fiber.StatusUnauthorized).JSON(models.NewErrorOutput(models.NewError("Unauthorized", fiber.StatusUnauthorized)))
	}

	// Rotate: issue a brand-new access + refresh pair.
	tokens, err := a.createTokenPair(user_db.Username)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create token pair")
		return c.SendStatus(fiber.StatusInternalServerError)
	}

	log.Info().Str("username", user_db.Username).Msg("Token pair refreshed successfully")
	return c.JSON(tokens)
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
	a.ActivitiesRepo.LogActivityWithCtx(c, activities_repo.ActivityTypeRegister, fiber.Map{
		"username": user.Username,
		"email":    user.Email,
		"role":     user.Role,
		"phone":    user.Phone,
		"branch":   user.Branch,
	})

	// Hash the password using bcrypt
	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(user.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("Failed to hash password")

		return c.SendStatus(fiber.StatusInternalServerError)
	}

	// Create a new user document
	newUser := models.NewUser(user.Username, string(hashedPassword), user.Email, user.Role, user.Phone, user.Branch)

	// Insert the new user into the database
	err = a.AdminUserRepo.Create(newUser)
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
// @Success 200 {object} models.Output[[]a.ActivitiesRepo.Activity]
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/activities/recent [get]
func (a *AuthControllers) GetRecentActivities(c *fiber.Ctx) error {
	log.Info().Msg("Getting recent activities")
	offset := c.QueryInt("offset", 0)
	limit := c.QueryInt("limit", 20)
	activities, err := a.ActivitiesRepo.ListRecentActivities(c.Context(), limit, offset)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get recent activities")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.NewError("Failed to get recent activities", fiber.StatusInternalServerError)))
	}
	return c.JSON(models.NewOutput(
		activities,
	))
}

// GetActivitesOfUser godoc
// @Summary Get activities of current user
// @Security BearerAuth
// @Description Get the 25 most recent activities for the authenticated user
// @Tags auth
// @Produce json
// @Success 200 {object} models.Output[[]a.ActivitiesRepo.Activity]
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/admin/activities/me [get]
func (a *AuthControllers) GetActivitesOfUser(c *fiber.Ctx) error {
	log.Info().Str("user", c.Locals("user").(string)).Msg("Getting activities of user")
	activities, err := a.ActivitiesRepo.ListActivities(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get recent activities")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]string{}, models.NewError("Failed to get recent activities", fiber.StatusInternalServerError)))
	}
	return c.JSON(models.NewOutput(
		activities,
	))
}
