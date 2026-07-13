// Package handoffcart holds the Fiber handlers for the Scan & Go handoff cart.
// It parses requests, enforces the session-token / capability authorization, logs
// audit activity and renders the standard response envelope; all cart logic and
// the state machine live in the handoff repository.
package handoffcart

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	handoffcart_repo "github.com/aslon1213/g4h_pos_erp/pkg/repository/HandOffCart"
	activities_repo "github.com/aslon1213/g4h_pos_erp/pkg/repository/activities"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// sessionTokenHeader carries the entry-QR session token on the customer surface.
const sessionTokenHeader = "X-Handoff-Session"

// idempotencyHeader carries the required checkout idempotency key.
const idempotencyHeader = "Idempotency-Key"

// HandOffCartControllers exposes the customer and seller handoff endpoints.
type HandOffCartControllers struct {
	Repo           *handoffcart_repo.HandOffCartRepository
	ActivitiesRepo *activities_repo.ActivitiesRepo
}

func New(db *gorm.DB) *HandOffCartControllers {
	return &HandOffCartControllers{
		Repo:           handoffcart_repo.New(db),
		ActivitiesRepo: activities_repo.New(db),
	}
}

// SessionAuthMiddleware guards the customer surface: it requires the entry-QR
// session token (X-Handoff-Session header) and stashes it in the request locals.
// Token validity (unknown/expired) is resolved by the repository on lookup.
func (h *HandOffCartControllers) SessionAuthMiddleware(c *fiber.Ctx) error {
	token := c.Get(sessionTokenHeader)
	if token == "" {
		return models.RespondError(c, fiber.StatusUnauthorized, "missing handoff session token")
	}
	c.Locals("handoff_session_token", token)
	return c.Next()
}

func sessionToken(c *fiber.Ctx) string {
	token, _ := c.Locals("handoff_session_token").(string)
	return token
}

// ---------------------------------------------------------------------------
// Customer surface (session token)
// ---------------------------------------------------------------------------

// StartSession godoc
// @Summary Start a Scan & Go session (scoped to the entry-QR branch)
// @Description Creates a server-side cart and returns its one-time session token.
// @Tags handoff
// @Accept json
// @Produce json
// @Param body body models.StartHandoffSessionInput true "Branch from the entry QR"
// @Success 201 {object} models.Output[models.StartHandoffSessionResponse]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/handoff/sessions [post]
func (h *HandOffCartControllers) StartSession(c *fiber.Ctx) error {
	in := models.StartHandoffSessionInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, token, err := h.Repo.StartSession(c.Context(), in.BranchID)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(models.StartHandoffSessionResponse{
		SessionToken: token,
		Cart:         *cart,
	}))
}

// GetCart godoc
// @Summary Get the current handoff cart
// @Tags handoff
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 401 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart [get]
func (h *HandOffCartControllers) GetCart(c *fiber.Ctx) error {
	cart, err := h.Repo.GetBySessionToken(c.Context(), sessionToken(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// AddLine godoc
// @Summary Add a scanned product line to the cart
// @Tags handoff
// @Accept json
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Param body body models.AddHandoffLineInput true "Product + quantity"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/lines [post]
func (h *HandOffCartControllers) AddLine(c *fiber.Ctx) error {
	in := models.AddHandoffLineInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, err := h.Repo.AddLine(c.Context(), sessionToken(c), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// UpdateLine godoc
// @Summary Update a cart line's quantity (0 removes it)
// @Tags handoff
// @Accept json
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Param line_id path string true "Line ID"
// @Param body body models.UpdateHandoffLineInput true "New quantity"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/lines/{line_id} [put]
func (h *HandOffCartControllers) UpdateLine(c *fiber.Ctx) error {
	in := models.UpdateHandoffLineInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, err := h.Repo.UpdateLine(c.Context(), sessionToken(c), c.Params("line_id"), in.Quantity)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// RemoveLine godoc
// @Summary Remove a line from the cart
// @Tags handoff
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Param line_id path string true "Line ID"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/lines/{line_id} [delete]
func (h *HandOffCartControllers) RemoveLine(c *fiber.Ctx) error {
	cart, err := h.Repo.RemoveLine(c.Context(), sessionToken(c), c.Params("line_id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// RequestHandoff godoc
// @Summary Request checkout — mint a handoff token (QR + 8-digit code)
// @Tags handoff
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Success 200 {object} models.Output[models.RequestHandoffResponse]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/request-handoff [post]
func (h *HandOffCartControllers) RequestHandoff(c *fiber.Ctx) error {
	cart, code, ref, err := h.Repo.RequestHandoff(c.Context(), sessionToken(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	resp := models.RequestHandoffResponse{Code: code, QRRef: ref, Cart: *cart}
	if cart.HandoffExpiresAt != nil {
		resp.ExpiresAt = *cart.HandoffExpiresAt
	}
	return c.JSON(models.NewOutput(resp))
}

// CancelSession godoc
// @Summary Cancel the current handoff cart
// @Tags handoff
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/cancel [post]
func (h *HandOffCartControllers) CancelSession(c *fiber.Ctx) error {
	cart, err := h.Repo.CancelBySession(c.Context(), sessionToken(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// ---------------------------------------------------------------------------
// Seller surface (staff PASETO + capability + branch scope)
// ---------------------------------------------------------------------------

func sellerID(c *fiber.Ctx) string { s, _ := c.Locals("seller_id").(string); return s }

// Claim godoc
// @Security BearerAuth
// @Summary Claim a cart by its handoff token (freezes customer edits)
// @Description Idempotent. The handoff token alone identifies the cart. Requires the handoff.claim capability.
// @Tags handoff
// @Accept json
// @Produce json
// @Param body body models.ClaimHandoffInput true "Handoff QR ref or 8-digit code"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/claim [post]
func (h *HandOffCartControllers) Claim(c *fiber.Ctx) error {
	in := models.ClaimHandoffInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	log.Info().Interface("input", in).Str("seller_id", sellerID(c)).Msg("handoff claim request")
	cart, err := h.Repo.ClaimByToken(c.Context(), in, sellerID(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	h.ActivitiesRepo.LogActivityWithCtx(c, activities_repo.ActivityTypeHandoffClaim, cart.ID)
	return c.JSON(models.NewOutput(*cart))
}

// SellerGetCart godoc
// @Security BearerAuth
// @Summary Get a handoff cart at the seller's branch
// @Tags handoff
// @Produce json
// @Param id path string true "Cart ID"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id} [get]
func (h *HandOffCartControllers) SellerGetCart(c *fiber.Ctx) error {
	cart, err := h.Repo.GetByIDForSeller(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// Checkout godoc
// @Security BearerAuth
// @Summary Charge a claimed cart (delegates to the sale flow)
// @Description Requires the handoff.checkout capability and an Idempotency-Key header. Records the sale on the seller's OPEN shift journal at the cart's branch.
// @Tags handoff
// @Accept json
// @Produce json
// @Param id path string true "Cart ID"
// @Param Idempotency-Key header string true "Idempotency key"
// @Param body body models.CheckoutHandoffInput true "Open journal + payment method"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id}/checkout [post]
func (h *HandOffCartControllers) Checkout(c *fiber.Ctx) error {
	key := c.Get(idempotencyHeader)
	if key == "" {
		return models.RespondError(c, fiber.StatusBadRequest, "missing "+idempotencyHeader+" header")
	}
	in := models.CheckoutHandoffInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, err := h.Repo.Checkout(c.Context(), c.Params("id"), sellerID(c), in, key)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	h.ActivitiesRepo.LogActivityWithCtx(c, activities_repo.ActivityTypeHandoffCheckout, cart.ID)
	return c.JSON(models.NewOutput(*cart))
}

// Release godoc
// @Security BearerAuth
// @Summary Release a claimed cart back to ready_for_handoff
// @Tags handoff
// @Produce json
// @Param id path string true "Cart ID"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id}/release [post]
func (h *HandOffCartControllers) Release(c *fiber.Ctx) error {
	cart, err := h.Repo.ReleaseClaim(c.Context(), c.Params("id"), sellerID(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// Cancel godoc
// @Security BearerAuth
// @Summary Cancel a claimed cart
// @Tags handoff
// @Produce json
// @Param id path string true "Cart ID"
// @Success 200 {object} models.Output[models.HandoffCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id}/cancel [post]
func (h *HandOffCartControllers) Cancel(c *fiber.Ctx) error {
	cart, err := h.Repo.CancelClaim(c.Context(), c.Params("id"), sellerID(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}
