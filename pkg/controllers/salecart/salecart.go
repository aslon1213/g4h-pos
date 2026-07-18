// Package salecart holds the Fiber handlers for the POS-side carts: the Scan &
// Go handoff surface (customer, authenticated by a session token) and the till
// surface (staff, authenticated by PASETO + capability). It parses requests,
// enforces authorization, logs audit activity and renders the standard response
// envelope; all cart logic and the state machine live in the salecart repository.
package salecart

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	activities_repo "github.com/aslon1213/g4h_pos_erp/pkg/repository/activities"
	salecart_repo "github.com/aslon1213/g4h_pos_erp/pkg/repository/salecart"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// sessionTokenHeader carries the entry-QR session token on the customer surface.
const sessionTokenHeader = "X-Handoff-Session"

// idempotencyHeader carries the required checkout idempotency key.
const idempotencyHeader = "Idempotency-Key"

// Controllers exposes the customer (handoff) and seller (handoff + POS) endpoints.
type Controllers struct {
	Repo           *salecart_repo.SaleCartRepository
	ActivitiesRepo *activities_repo.ActivitiesRepo
}

func New(db *gorm.DB) *Controllers {
	return &Controllers{
		Repo:           salecart_repo.New(db),
		ActivitiesRepo: activities_repo.New(db),
	}
}

// SessionAuthMiddleware guards the customer surface: it requires the entry-QR
// session token (X-Handoff-Session header) and stashes it in the request locals.
// Token validity (unknown/expired) is resolved by the repository on lookup.
func (h *Controllers) SessionAuthMiddleware(c *fiber.Ctx) error {
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

func sellerID(c *fiber.Ctx) string { s, _ := c.Locals("seller_id").(string); return s }

func sellerBranch(c *fiber.Ctx) string { s, _ := c.Locals("seller_branch").(string); return s }

// ---------------------------------------------------------------------------
// Handoff customer surface (session token)
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
func (h *Controllers) StartSession(c *fiber.Ctx) error {
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
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 401 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart [get]
func (h *Controllers) GetCart(c *fiber.Ctx) error {
	cart, err := h.Repo.GetBySessionToken(c.Context(), sessionToken(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// AddItem godoc
// @Summary Add a scanned product line to the cart
// @Description Quantity is fractional, so weighed goods can be sold by the kilogram.
// @Tags handoff
// @Accept json
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Param body body models.AddSaleCartItemInput true "Product + quantity"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/items [post]
func (h *Controllers) AddItem(c *fiber.Ctx) error {
	in := models.AddSaleCartItemInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, err := h.Repo.AddItem(c.Context(), sessionToken(c), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// UpdateItem godoc
// @Summary Update a cart line's quantity (0 removes it)
// @Tags handoff
// @Accept json
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Param item_id path string true "Item ID"
// @Param body body models.UpdateSaleCartItemInput true "New quantity"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/items/{item_id} [put]
func (h *Controllers) UpdateItem(c *fiber.Ctx) error {
	in := models.UpdateSaleCartItemInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, err := h.Repo.UpdateItem(c.Context(), sessionToken(c), c.Params("item_id"), in.Quantity)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// RemoveItem godoc
// @Summary Remove a line from the cart
// @Tags handoff
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Param item_id path string true "Item ID"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/items/{item_id} [delete]
func (h *Controllers) RemoveItem(c *fiber.Ctx) error {
	cart, err := h.Repo.RemoveItem(c.Context(), sessionToken(c), c.Params("item_id"))
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
func (h *Controllers) RequestHandoff(c *fiber.Ctx) error {
	cart, code, ref, err := h.Repo.RequestHandoff(c.Context(), sessionToken(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	resp := models.RequestHandoffResponse{Code: code, QRRef: ref, Cart: *cart}
	if cart.Session != nil && cart.Session.HandoffExpiresAt != nil {
		resp.ExpiresAt = *cart.Session.HandoffExpiresAt
	}
	return c.JSON(models.NewOutput(resp))
}

// CancelSession godoc
// @Summary Cancel the current handoff cart
// @Tags handoff
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/cancel [post]
func (h *Controllers) CancelSession(c *fiber.Ctx) error {
	cart, err := h.Repo.CancelBySession(c.Context(), sessionToken(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// DeleteSession godoc
// @Summary Delete the current handoff session
// @Description Hard-deletes the cart identified by the session token, cascading its items and its handoff session row. The session token dies with the row, so the X-Handoff-Session header the client cached stops resolving immediately — clients MUST drop it from their local store on a 204. Unlike /cart/cancel (which keeps the cart in a cancelled state), nothing is retained. A token that is unknown or already deleted is 404.
// @Tags handoff
// @Produce json
// @Param X-Handoff-Session header string true "Session token"
// @Success 204 "Session deleted; discard the cached session header"
// @Failure 401 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/handoff/cart/session [delete]
func (h *Controllers) DeleteSession(c *fiber.Ctx) error {
	if err := h.Repo.DeleteBySessionToken(c.Context(), sessionToken(c)); err != nil {
		return models.RespondRepoError(c, err)
	}
	// Drop the token from the request locals too, so nothing later in the chain
	// can act on a credential that no longer exists.
	c.Locals("handoff_session_token", "")
	return c.SendStatus(fiber.StatusNoContent)
}

// ---------------------------------------------------------------------------
// Handoff seller surface (staff PASETO + capability)
// ---------------------------------------------------------------------------

// Claim godoc
// @Security BearerAuth
// @Summary Claim a cart by its handoff token (freezes customer edits)
// @Description Idempotent. The handoff token alone identifies the cart. Requires the handoff.claim capability.
// @Tags handoff
// @Accept json
// @Produce json
// @Param body body models.ClaimHandoffInput true "Handoff QR ref or 8-digit code"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/claim [post]
func (h *Controllers) Claim(c *fiber.Ctx) error {
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

// Release godoc
// @Security BearerAuth
// @Summary Release a claimed cart back to ready_for_handoff
// @Tags handoff
// @Produce json
// @Param id path string true "Cart ID"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id}/release [post]
func (h *Controllers) Release(c *fiber.Ctx) error {
	cart, err := h.Repo.ReleaseClaim(c.Context(), c.Params("id"), sellerID(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// ---------------------------------------------------------------------------
// Seller cart surface — serves both kinds, addressed by cart id
// ---------------------------------------------------------------------------

// SellerGetCart godoc
// @Security BearerAuth
// @Summary Get a cart (handoff or POS) by id
// @Tags handoff
// @Produce json
// @Param id path string true "Cart ID"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id} [get]
func (h *Controllers) SellerGetCart(c *fiber.Ctx) error {
	cart, err := h.Repo.GetByID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// Checkout godoc
// @Security BearerAuth
// @Summary Charge a cart (delegates to the sale flow and decrements stock)
// @Description Requires the handoff.checkout capability and an Idempotency-Key header. Records the sale on the seller's OPEN shift journal at the cart's branch, then decrements branch stock by each line's quantity. A handoff cart must be claimed by the caller; a POS cart must be theirs and still active. Replaying the same idempotency key returns the completed cart without charging or decrementing again.
// @Tags handoff
// @Accept json
// @Produce json
// @Param id path string true "Cart ID"
// @Param Idempotency-Key header string true "Idempotency key"
// @Param body body models.CheckoutCartInput true "Open journal + payment method"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id}/checkout [post]
func (h *Controllers) Checkout(c *fiber.Ctx) error {
	key := c.Get(idempotencyHeader)
	if key == "" {
		return models.RespondError(c, fiber.StatusBadRequest, "missing "+idempotencyHeader+" header")
	}
	in := models.CheckoutCartInput{}
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

// Cancel godoc
// @Security BearerAuth
// @Summary Cancel a cart (claimed handoff cart, or the seller's own POS cart)
// @Tags handoff
// @Produce json
// @Param id path string true "Cart ID"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/handoff/carts/{id}/cancel [post]
func (h *Controllers) Cancel(c *fiber.Ctx) error {
	cart, err := h.Repo.CancelClaim(c.Context(), c.Params("id"), sellerID(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// ---------------------------------------------------------------------------
// POS (till) surface
// ---------------------------------------------------------------------------

// OpenPOSCart godoc
// @Security BearerAuth
// @Summary Open a till cart at the seller's branch
// @Description Replaces the old Redis-backed sales session: the cart is a durable row, so a till recovers its basket after a reload or restart. Branch defaults to the authenticated seller's branch.
// @Tags pos
// @Accept json
// @Produce json
// @Param body body models.OpenPOSCartInput false "Optional branch override"
// @Success 201 {object} models.Output[models.SaleCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/pos/carts [post]
func (h *Controllers) OpenPOSCart(c *fiber.Ctx) error {
	in := models.OpenPOSCartInput{}
	// Body is optional: with none supplied the seller's own branch is used.
	_ = c.BodyParser(&in)
	branch := in.BranchID
	if branch == "" {
		branch = sellerBranch(c)
	}
	cart, err := h.Repo.OpenPOSCart(c.Context(), branch, sellerID(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(*cart))
}

// ListOpenPOSCarts godoc
// @Security BearerAuth
// @Summary List the seller's open till carts
// @Tags pos
// @Produce json
// @Success 200 {object} models.Output[[]models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/pos/carts [get]
func (h *Controllers) ListOpenPOSCarts(c *fiber.Ctx) error {
	carts, err := h.Repo.ListOpenPOSCarts(c.Context(), sellerBranch(c), sellerID(c))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(carts))
}

// POSAddItem godoc
// @Security BearerAuth
// @Summary Add a scanned line to a till cart
// @Description Quantity is fractional, so weighed goods can be sold by the kilogram.
// @Tags pos
// @Accept json
// @Produce json
// @Param id path string true "Cart ID"
// @Param body body models.AddSaleCartItemInput true "Product + quantity"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/pos/carts/{id}/items [post]
func (h *Controllers) POSAddItem(c *fiber.Ctx) error {
	in := models.AddSaleCartItemInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, err := h.Repo.AddItemByID(c.Context(), c.Params("id"), in)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// POSUpdateItem godoc
// @Security BearerAuth
// @Summary Update a till cart line's quantity (0 removes it)
// @Tags pos
// @Accept json
// @Produce json
// @Param id path string true "Cart ID"
// @Param item_id path string true "Item ID"
// @Param body body models.UpdateSaleCartItemInput true "New quantity"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/pos/carts/{id}/items/{item_id} [put]
func (h *Controllers) POSUpdateItem(c *fiber.Ctx) error {
	in := models.UpdateSaleCartItemInput{}
	if err := c.BodyParser(&in); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	cart, err := h.Repo.UpdateItemByID(c.Context(), c.Params("id"), c.Params("item_id"), in.Quantity)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// POSRemoveItem godoc
// @Security BearerAuth
// @Summary Remove a line from a till cart
// @Tags pos
// @Produce json
// @Param id path string true "Cart ID"
// @Param item_id path string true "Item ID"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/pos/carts/{id}/items/{item_id} [delete]
func (h *Controllers) POSRemoveItem(c *fiber.Ctx) error {
	cart, err := h.Repo.RemoveItemByID(c.Context(), c.Params("id"), c.Params("item_id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}

// POSCancelCart godoc
// @Security BearerAuth
// @Summary Abandon a till cart
// @Tags pos
// @Produce json
// @Param id path string true "Cart ID"
// @Success 200 {object} models.Output[models.SaleCart]
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Router /api/v1/admin/pos/carts/{id}/cancel [post]
func (h *Controllers) POSCancelCart(c *fiber.Ctx) error {
	cart, err := h.Repo.CancelByID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(*cart))
}
