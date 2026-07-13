package middleware

import (
	"slices"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// Capabilities are fine-grained, branch-scoped permissions checked on top of the
// staff PASETO guard. They are the authorization layer for the Scan & Go seller
// endpoints (claim/checkout).
const (
	CapHandoffClaim    = "handoff.claim"
	CapHandoffCheckout = "handoff.checkout"
)

// roleCapabilities maps a staff role (models.UserRole) to the capabilities it
// grants. Admin, manager and staff (the POS operators) may run the Scan & Go
// claim/checkout flow; UserRoleUser has no handoff capabilities. Adjust as your
// role policy evolves.
var roleCapabilities = map[models.UserRole][]string{
	models.UserRoleAdmin:   {CapHandoffClaim, CapHandoffCheckout},
	models.UserRoleManager: {CapHandoffClaim, CapHandoffCheckout},
	models.UserRoleStaff:   {CapHandoffClaim, CapHandoffCheckout},
}

// roleHasCapability reports whether role grants capability.
func roleHasCapability(role models.UserRole, capability string) bool {
	return slices.Contains(roleCapabilities[role], capability)
}

// RequireCapability builds a Fiber middleware that authorizes the staff user
// (already resolved by AuthMiddleware, which stored the username in "user_id")
// for the given capability. On success it stashes the seller identity and branch
// in the request locals ("seller_id", "seller_branch") for branch-scoped
// handlers. A missing capability is 403; an unresolvable user is 401.
func (m *Middlewares) RequireCapability(capability string) fiber.Handler {
	return func(c *fiber.Ctx) error {
		username, _ := c.Locals("user_id").(string)
		if username == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		user, err := gorm.G[models.User](m.db).Where("username = ?", username).First(c.Context())
		if err != nil {
			log.Error().Err(err).Str("username", username).Msg("capability check: user not found")
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		if !roleHasCapability(user.Role, capability) {
			return models.RespondError(c, fiber.StatusForbidden, "missing capability: "+capability+" you have a role: "+string(user.Role))
		}
		c.Locals("seller_id", user.Username)
		c.Locals("seller_branch", user.Branch)
		return c.Next()
	}
}
