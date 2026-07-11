package storewishlist

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	cartrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/cart"
	wishlistrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/wishlist"
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

// Controller handles the customer's wishlist (likelist). Wishlist operations go
// through Wishlist; moving an item to the cart reuses the cart repository.
type Controller struct {
	Wishlist *wishlistrepo.WishlistRepository
	Cart     *cartrepo.CartRepository
}

func New(db *gorm.DB) *Controller {
	return &Controller{
		Wishlist: wishlistrepo.New(db),
		Cart:     cartrepo.New(db),
	}
}

// GetWishlist godoc
// @Summary Get the current customer's wishlist
// @Tags store-wishlist
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/wishlist [get]
func (ctrl *Controller) GetWishlist(c *fiber.Ctx) error { return models.NotImplemented(c) }

// AddItem godoc
// @Summary Add a product to the wishlist
// @Tags store-wishlist
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/wishlist/items [post]
func (ctrl *Controller) AddItem(c *fiber.Ctx) error { return models.NotImplemented(c) }

// RemoveItem godoc
// @Summary Remove a product from the wishlist
// @Tags store-wishlist
// @Security BearerAuth
// @Param product_id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/wishlist/items/{product_id} [delete]
func (ctrl *Controller) RemoveItem(c *fiber.Ctx) error { return models.NotImplemented(c) }

// MoveItemToCart godoc
// @Summary Move a wishlist product into the cart
// @Tags store-wishlist
// @Security BearerAuth
// @Param product_id path string true "Product ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/wishlist/items/{product_id}/move-to-cart [post]
func (ctrl *Controller) MoveItemToCart(c *fiber.Ctx) error { return models.NotImplemented(c) }
