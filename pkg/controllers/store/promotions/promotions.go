package storepromotions

import (
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	cartrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/cart"
	promotionrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/promotion"
	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// Controller handles storefront promotions and coupons. Promotions/coupons are
// read through Promotions; coupon validation reads the cart through Cart.
type Controller struct {
	Promotions *promotionrepo.PromotionRepository
	Cart       *cartrepo.CartRepository
}

func New(db *mongo.Database) *Controller {
	return &Controller{
		Promotions: promotionrepo.New(db),
		Cart:       cartrepo.New(db),
	}
}

// ListPromotions godoc
// @Summary List active promotions / banners
// @Tags store-promotions
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/promotions [get]
func (ctrl *Controller) ListPromotions(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetPromotionByID godoc
// @Summary Get a promotion by id
// @Tags store-promotions
// @Param id path string true "Promotion ID"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/promotions/{id} [get]
func (ctrl *Controller) GetPromotionByID(c *fiber.Ctx) error { return models.NotImplemented(c) }

// GetCoupon godoc
// @Summary Look up a coupon's terms by code
// @Tags store-promotions
// @Param code path string true "Coupon code"
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/promotions/coupons/{code} [get]
func (ctrl *Controller) GetCoupon(c *fiber.Ctx) error { return models.NotImplemented(c) }

// ValidateCoupon godoc
// @Summary Validate a coupon against the current cart / customer
// @Tags store-promotions
// @Security BearerAuth
// @Produce json
// @Success 501 {object} models.ErrorOutput
// @Router /api/v1/store/promotions/validate [post]
func (ctrl *Controller) ValidateCoupon(c *fiber.Ctx) error { return models.NotImplemented(c) }
