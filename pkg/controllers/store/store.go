package store

import (
	storeaccount "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/account"
	storeauth "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/auth"
	storecart "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/cart"
	storecatalog "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/catalog"
	storeorders "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/orders"
	storeproducts "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/products"
	storepromotions "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/promotions"
	storereviews "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/reviews"
	storewishlist "github.com/aslon1213/g4h_pos_erp/pkg/controllers/store/wishlist"
	"gorm.io/gorm"
)

// Controllers aggregates every storefront controller so the wiring (app) and
// the route tables (routes) can pass a single value instead of nine.
type Controllers struct {
	Auth       *storeauth.Controller
	Account    *storeaccount.Controller
	Catalog    *storecatalog.Controller
	Products   *storeproducts.Controller
	Cart       *storecart.Controller
	Wishlist   *storewishlist.Controller
	Orders     *storeorders.Controller
	Reviews    *storereviews.Controller
	Promotions *storepromotions.Controller
}

func New(db *gorm.DB) *Controllers {
	return &Controllers{
		Auth:       storeauth.New(db),
		Account:    storeaccount.New(db),
		Catalog:    storecatalog.New(db),
		Products:   storeproducts.New(db),
		Cart:       storecart.New(db),
		Wishlist:   storewishlist.New(db),
		Orders:     storeorders.New(db),
		Reviews:    storereviews.New(db),
		Promotions: storepromotions.New(db),
	}
}
