package storeauth

import (
	"encoding/base64"
	"errors"
	"strings"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/configs"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	customerrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/customer"
	pasetoware "github.com/gofiber/contrib/paseto"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

// Controller handles storefront customer authentication against the
// store_customers collection (separate from staff auth). All database access
// goes through the customer repository held on Customers.
type Controller struct {
	Customers          *customerrepo.CustomerRepository
	SecretSymmetricKey string
	TokenExpiryHours   int
}

func New(db *gorm.DB) *Controller {
	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	return &Controller{
		Customers:          customerrepo.New(db),
		SecretSymmetricKey: config.Server.SecretSymmetricKey,
		TokenExpiryHours:   config.Server.TokenExpiryHours,
	}
}

// issueToken mints a PASETO for the customer. The token subject (dataInfo) is
// the customer id, which CustomerAuthMiddleware looks up in store_customers.
func (ctrl *Controller) issueToken(customerID string) (any, error) {
	keyBytes, err := base64.StdEncoding.DecodeString(ctrl.SecretSymmetricKey)
	if err != nil {
		return nil, err
	}
	ttl := time.Duration(ctrl.TokenExpiryHours) * time.Hour
	encrypted, err := pasetoware.CreateToken(keyBytes, customerID, ttl, pasetoware.PurposeLocal)
	if err != nil {
		return nil, err
	}
	return pasetoware.NewPayload(encrypted, ttl)
}

// Register godoc
// @Summary Register a storefront customer
// @Description Create a new storefront customer account and return an access token
// @Tags store-auth
// @Accept json
// @Produce json
// @Param body body models.RegisterInput true "Registration details"
// @Success 201 {object} models.TokenResponse "token"
// @Failure 400 {object} models.ErrorOutput "Invalid request"
// @Failure 409 {object} models.ErrorOutput "Email already registered"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/store/auth/register [post]
func (ctrl *Controller) Register(c *fiber.Ctx) error {
	var in models.RegisterInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if msg := validateRegister(in); msg != "" {
		return models.RespondError(c, fiber.StatusBadRequest, msg)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(in.Password), bcrypt.DefaultCost)
	if err != nil {
		log.Error().Err(err).Msg("Failed to hash password")
		return models.RespondError(c, fiber.StatusInternalServerError, "failed to process password")
	}

	customer := &models.StoreCustomer{
		Email:        strings.ToLower(strings.TrimSpace(in.Email)),
		Phone:        in.Phone,
		Name:         in.Name,
		PasswordHash: string(hash),
	}
	created, err := ctrl.Customers.Create(c.Context(), customer)
	if err != nil {
		return models.RespondRepoError(c, err)
	}

	payload, err := ctrl.issueToken(created.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create token")
		return models.RespondError(c, fiber.StatusInternalServerError, "failed to create token")
	}
	log.Info().Str("customer_id", created.ID).Msg("Storefront customer registered")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(payload))
}

// Login godoc
// @Summary Login a storefront customer
// @Description Authenticate a storefront customer and return an access token
// @Tags store-auth
// @Accept json
// @Produce json
// @Param body body models.LoginInput true "Login credentials"
// @Success 200 {object} models.TokenResponse "token"
// @Failure 400 {object} models.ErrorOutput "Invalid request"
// @Failure 401 {object} models.ErrorOutput "Invalid email or password"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/store/auth/login [post]
func (ctrl *Controller) Login(c *fiber.Ctx) error {
	var in models.LoginInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	if in.Email == "" || in.Password == "" {
		return models.RespondError(c, fiber.StatusBadRequest, "email and password are required")
	}

	customer, err := ctrl.Customers.GetByEmail(c.Context(), strings.ToLower(strings.TrimSpace(in.Email)))
	if err != nil {
		// Do not reveal whether the email exists.
		if errors.Is(err, repoerr.ErrNotFound) {
			return models.RespondError(c, fiber.StatusUnauthorized, "invalid email or password")
		}
		return models.RespondRepoError(c, err)
	}

	if bcrypt.CompareHashAndPassword([]byte(customer.PasswordHash), []byte(in.Password)) != nil {
		return models.RespondError(c, fiber.StatusUnauthorized, "invalid email or password")
	}

	payload, err := ctrl.issueToken(customer.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create token")
		return models.RespondError(c, fiber.StatusInternalServerError, "failed to create token")
	}
	log.Info().Str("customer_id", customer.ID).Msg("Storefront customer logged in")
	return c.JSON(models.NewOutput(payload))
}

// Me godoc
// @Summary Get current storefront customer profile
// @Tags store-auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} models.Output[models.StoreCustomer]
// @Failure 404 {object} models.ErrorOutput "Customer not found"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/store/auth/me [get]
func (ctrl *Controller) Me(c *fiber.Ctx) error {
	customerID := c.Locals("customer").(string)
	customer, err := ctrl.Customers.GetByID(c.Context(), customerID)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(customer))
}

// UpdateMe godoc
// @Summary Update current storefront customer profile
// @Tags store-auth
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body models.UpdateProfileInput true "Profile fields to update"
// @Success 200 {object} models.Output[models.StoreCustomer]
// @Failure 400 {object} models.ErrorOutput "Invalid request"
// @Failure 404 {object} models.ErrorOutput "Customer not found"
// @Failure 500 {object} models.ErrorOutput "Internal Server Error"
// @Router /api/v1/store/auth/me [put]
func (ctrl *Controller) UpdateMe(c *fiber.Ctx) error {
	customerID := c.Locals("customer").(string)
	var in models.UpdateProfileInput
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, "invalid request body")
	}
	customer, err := ctrl.Customers.UpdateProfile(c.Context(), customerID, in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(customer))
}

// Logout godoc
// @Summary Logout the current storefront customer
// @Description PASETO tokens are stateless; logout is a client-side discard. This
// @Description endpoint exists for symmetry and future token-revocation support.
// @Tags store-auth
// @Security BearerAuth
// @Produce json
// @Success 200 {object} models.Output[models.MessageResponse]
// @Router /api/v1/store/auth/logout [post]
func (ctrl *Controller) Logout(c *fiber.Ctx) error {
	return c.JSON(models.NewOutput(models.MessageResponse{Message: "logged out"}))
}

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

// validateRegister returns a human-readable message for the first failing rule,
// or "" when the input is acceptable.
func validateRegister(in models.RegisterInput) string {
	switch {
	case strings.TrimSpace(in.Email) == "":
		return "email is required"
	case !strings.Contains(in.Email, "@"):
		return "email is invalid"
	case strings.TrimSpace(in.Name) == "":
		return "name is required"
	case len(in.Password) < 8:
		return "password must be at least 8 characters"
	}
	return ""
}
