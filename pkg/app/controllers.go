package app

import (
	"encoding/base64"

	"github.com/aslon1213/g4h_pos_erp/pkg/configs"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/analytics"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/arrivals"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/auth"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/customers"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/customers/bnpl"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/finance"
	journal_handlers "github.com/aslon1213/g4h_pos_erp/pkg/controllers/journals"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/products"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/sales"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/store"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/suppliers"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/transactions"
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	"github.com/aslon1213/g4h_pos_erp/pkg/routes"
	pasetoware "github.com/gofiber/contrib/paseto"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Controllers struct {
	Finance      *finance.FinanceController
	Suppliers    *suppliers.SuppliersController
	Transactions *transactions.TransactionsController
	Sales        *sales.SalesTransactionsController
	Journals     *journal_handlers.JournalHandlers
	Operations   *journal_handlers.OperationHandlers
	Products     *products.ProductsController
	Auth         *auth.AuthControllers
	BNPL         *bnpl.BNPLController
	Customers    *customers.CustomersController
	Middlewares  *middleware.Middlewares
	Dashboard    *analytics.DashboardHandler
	Proposals    *arrivals.ProposalsHandlers
	Store        *store.Controllers
}

func NewControllers(db *mongo.Database) *Controllers {
	log.Debug().Msg("Initializing new controllers")
	middleware := middleware.New(db)
	controllers := &Controllers{
		Finance:      finance.New(db),
		Suppliers:    suppliers.New(db),
		Transactions: transactions.New(db),
		Sales:        sales.New(db),
		Journals:     journal_handlers.New(db),
		Operations:   journal_handlers.NewOperationsHandler(db),
		Products:     products.New(db),
		Auth:         auth.New(db),
		Customers:    customers.New(db),
		BNPL:         bnpl.New(db),
		Middlewares:  middleware,
		Dashboard:    analytics.New(db),
		Proposals:    arrivals.New(db),
		Store:        store.New(db),
	}
	log.Debug().Msg("Controllers initialized successfully")
	return controllers
}

func SetupRoutes(app *fiber.App, controllers *Controllers) {
	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	keyBytes, err := base64.StdEncoding.DecodeString(config.Server.SecretSymmetricKey)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid symmetric key")
	}

	// Public routes must be registered BEFORE the /api token guard below, so the
	// PASETO middleware does not apply to them (e.g. login can't require a token).
	routes.PublicAuthRoutes(app, controllers.Auth, controllers.Middlewares)
	log.Debug().Msg("Public auth routes set up successfully")

	// Storefront API (/api/v1/store). These are registered before the staff /api
	// guard so they do NOT inherit the staff (users-collection) auth. Public store
	// routes get no guard; protected ones get their own customer-token guard below.
	routes.StorePublicRoutes(app, controllers.Store, controllers.Middlewares)
	log.Debug().Msg("Store public routes set up successfully")

	app.Group("/api/v1/store", pasetoware.New(
		pasetoware.Config{
			SymmetricKey:   keyBytes,
			SuccessHandler: controllers.Middlewares.CustomerAuthMiddleware,
		},
	))
	routes.StoreProtectedRoutes(app, controllers.Store, controllers.Middlewares)
	log.Debug().Msg("Store protected routes set up successfully")

	app.Group("/api", pasetoware.New(
		pasetoware.Config{
			SymmetricKey: keyBytes,
			// TokenPrefix:    "Bearer",
			SuccessHandler: controllers.Middlewares.AuthMiddleware,
		},
	))

	log.Debug().Msg("Setting up routes")
	routes.AuthRoutes(app, controllers.Auth, controllers.Middlewares)
	log.Debug().Msg("Auth routes set up successfully")
	routes.SuppliersRoutes(app, controllers.Suppliers, controllers.Middlewares)
	log.Debug().Msg("Suppliers routes set up successfully")
	routes.TransactionsRoutes(app, controllers.Transactions, controllers.Middlewares)
	log.Debug().Msg("Transactions routes set up successfully")
	routes.FinanceRoutes(app, controllers.Finance, controllers.Middlewares)
	log.Debug().Msg("Finance routes set up successfully")
	routes.SalesRoutes(app, controllers.Sales, controllers.Middlewares)
	log.Debug().Msg("Sales routes set up successfully")
	routes.JournalsRoutes(app, controllers.Journals, controllers.Operations, controllers.Middlewares)
	log.Debug().Msg("Journals routes set up successfully")
	routes.ProductsRoutes(app, controllers.Products, controllers.Middlewares)
	log.Debug().Msg("Products routes set up successfully")
	routes.CustomerRoutes(app, controllers.Customers, controllers.Middlewares)
	log.Debug().Msg("Customer routes set up successfully")
	routes.BNPLRoutes(app, controllers.BNPL, controllers.Middlewares)
	log.Debug().Msg("BNPL routes set up successfully")
	routes.DashboardRoutes(app, controllers.Dashboard, controllers.Middlewares)
	log.Debug().Msg("Dashboard routes set up successfully")
	routes.ProxyRoutes(app, controllers.Middlewares)
	log.Debug().Msg("Proxy routes set up successfully")
	routes.ProposalsRoutes(app, controllers.Proposals, controllers.Middlewares)
	log.Debug().Msg("Proposals routes set up successfully")
	log.Debug().Msg("All routes set up successfully")
}
