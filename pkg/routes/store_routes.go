package routes

import (
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/store"
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	"github.com/gofiber/fiber/v2"
)

const storePrefix = "/api/v1/store"

// StorePublicRoutes registers the storefront endpoints that require NO token
// (browse surface + register/login). It MUST be called before the customer
// token guard is mounted on the /api/v1/store group in SetupRoutes, otherwise
// these routes would inherit the guard.
func StorePublicRoutes(router *fiber.App, sc *store.Controllers, mw *middleware.Middlewares) {
	g := router.Group(storePrefix)

	// customer auth (public)
	g.Post("/auth/register", sc.Auth.Register)              // register a storefront account
	g.Post("/auth/login", sc.Auth.Login)                    // login -> token
	g.Post("/auth/refresh", sc.Auth.RefreshToken)           // refresh access token
	g.Post("/auth/password/forgot", sc.Auth.ForgotPassword) // request password reset
	g.Post("/auth/password/reset", sc.Auth.ResetPassword)   // reset password with token

	// catalog (browse)
	g.Get("/catalog/categories", sc.Catalog.GetCategories)                    // list categories
	g.Get("/catalog/categories/:id", sc.Catalog.GetCategoryByID)              // category detail
	g.Get("/catalog/categories/:id/products", sc.Catalog.GetCategoryProducts) // products in category
	g.Get("/catalog/brands", sc.Catalog.GetBrands)                            // list brands
	g.Get("/catalog/search", sc.Catalog.Search)                               // search catalog

	// products (browse)
	g.Get("/products", sc.Products.ListProducts)                            // list products
	g.Get("/products/:id", sc.Products.GetProductByID)                      // product detail
	g.Get("/products/:id/images", sc.Products.GetProductImages)             // product images
	g.Get("/products/:id/related", sc.Products.GetRelatedProducts)          // related products
	g.Get("/products/:id/availability", sc.Products.GetProductAvailability) // stock per branch
	g.Get("/products/:id/reviews", sc.Products.GetProductReviews)           // reviews listing (public)

	// promotions (public)
	g.Get("/promotions", sc.Promotions.ListPromotions)          // active promotions
	g.Get("/promotions/:id", sc.Promotions.GetPromotionByID)    // promotion detail
	g.Get("/promotions/coupons/:code", sc.Promotions.GetCoupon) // coupon terms lookup
}

// StoreProtectedRoutes registers the storefront endpoints that require a valid
// customer token. It MUST be called after the customer token guard is mounted on
// the /api/v1/store group in SetupRoutes.
func StoreProtectedRoutes(router *fiber.App, sc *store.Controllers, mw *middleware.Middlewares) {
	g := router.Group(storePrefix)

	// customer auth (protected)
	g.Get("/auth/me", sc.Auth.Me)       // current profile
	g.Put("/auth/me", sc.Auth.UpdateMe) // update profile
	g.Post("/auth/logout", sc.Auth.Logout)

	// account / address book
	g.Get("/account/addresses", sc.Account.GetAddresses)
	g.Post("/account/addresses", sc.Account.CreateAddress)
	g.Put("/account/addresses/:address_id", sc.Account.UpdateAddress)
	g.Delete("/account/addresses/:address_id", sc.Account.DeleteAddress)
	g.Put("/account/addresses/:address_id/default", sc.Account.SetDefaultAddress)

	// cart
	g.Get("/cart", sc.Cart.GetCart)
	g.Post("/cart/items", sc.Cart.AddItem)
	g.Put("/cart/items/:item_id", sc.Cart.UpdateItem)
	g.Delete("/cart/items/:item_id", sc.Cart.RemoveItem)
	g.Delete("/cart", sc.Cart.ClearCart)
	g.Post("/cart/promo", sc.Cart.ApplyPromo)
	g.Delete("/cart/promo", sc.Cart.RemovePromo)

	// wishlist (likelist)
	g.Get("/wishlist", sc.Wishlist.GetWishlist)
	g.Post("/wishlist/items", sc.Wishlist.AddItem)
	g.Delete("/wishlist/items/:product_id", sc.Wishlist.RemoveItem)
	g.Post("/wishlist/items/:product_id/move-to-cart", sc.Wishlist.MoveItemToCart)

	// checkout / orders
	g.Post("/checkout/preview", sc.Orders.CheckoutPreview)
	g.Post("/orders", sc.Orders.PlaceOrder)
	g.Get("/orders", sc.Orders.ListOrders)
	g.Get("/orders/:id", sc.Orders.GetOrderByID)
	g.Get("/orders/:id/status", sc.Orders.GetOrderStatus)
	g.Post("/orders/:id/cancel", sc.Orders.CancelOrder)
	g.Post("/orders/:id/reorder", sc.Orders.ReorderOrder)

	// reviews (write/manage)
	g.Post("/products/:id/reviews", sc.Reviews.CreateReview)
	g.Put("/reviews/:id", sc.Reviews.UpdateReview)
	g.Delete("/reviews/:id", sc.Reviews.DeleteReview)
	g.Post("/reviews/:id/vote", sc.Reviews.VoteReview)

	// promotions (protected)
	g.Post("/promotions/validate", sc.Promotions.ValidateCoupon)
}
