package routes

import (
	"bytes"
	"encoding/base64"
	"io"
	"net/http"

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
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/suppliers"
	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/transactions"
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"

	pasetoware "github.com/gofiber/contrib/paseto"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/rs/zerolog/log"
)

func DashboardRoutes(router *fiber.App, dashboardController *analytics.DashboardHandler, middleware *middleware.Middlewares) {
	dashboard := router.Group("/dashboard")
	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}
	auth := basicauth.New(basicauth.Config{
		Users: map[string]string{
			config.Server.AdminDocsUsers[0].Username: config.Server.AdminDocsUsers[0].Password,
			config.Server.AdminDocsUsers[1].Username: config.Server.AdminDocsUsers[1].Password,
		},
		Realm: "Restricted",
	})

	// log.Debug().Interface("auth", auth).Msg("Auth middleware initialized")
	dashboard.Get("/journals", auth, dashboardController.ServeDashBoardDays)
	dashboard.Get("/general", auth, dashboardController.ServeDashBoardGeneral)
	dashboard.Get("/comparison", auth, dashboardController.ServeDashBoardComparison)
	dashboard.Get("/", auth, dashboardController.MainPage)
	// dashboard.Get("/branches")
}

// PublicAuthRoutes registers auth endpoints that must stay OUTSIDE the PASETO
// token guard. It MUST be called before the /api group middleware is registered
// in SetupRoutes, otherwise these routes would require a token to reach.
func PublicAuthRoutes(router *fiber.App, authController *auth.AuthControllers, middleware *middleware.Middlewares) {
	router.Post("/api/v1/admin/auth/login", authController.Login) // login -- public, activity logged here if succesfull
	// router.Post("/api/v1/admin/auth/register", authController.Register) // register -- disabled
}

func AuthRoutes(router *fiber.App, authController *auth.AuthControllers, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Get("/auth/me", authController.InfoMe)                         // get user info
	api.Get("/activities/recent", authController.GetRecentActivities) // get recent activities
	api.Get("/activities/me", authController.GetActivitesOfUser)      // get activities of user
}

func SuppliersRoutes(router *fiber.App, suppliersController *suppliers.SuppliersController, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Get("/suppliers", suppliersController.GetSuppliers)                                         // get all suppliers
	api.Get("/suppliers/:id", suppliersController.GetSupplierByID)                                  // get supplier by id
	api.Post("/suppliers", suppliersController.CreateSupplier)                                      // create supplier -- activity logged here if succesfull                                   // create supplier
	api.Put("/suppliers/:id", suppliersController.UpdateSupplier)                                   // update supplier
	api.Delete("/suppliers/:id", suppliersController.DeleteSupplier)                                // delete supplier
	api.Post("/suppliers/:branch_id/:supplier_id/transactions", suppliersController.NewTransaction) // create transaction
}

func SalesRoutes(router *fiber.App, salesController *sales.SalesTransactionsController, middleware *middleware.Middlewares) {
	api := router.Group("/api")
	api.Post("/sales/transactions/:branch_id", salesController.CreateSalesTransaction)        // create sales transaction -- activity logged here if succesfull
	api.Delete("/sales/transactions/:transaction_id", salesController.DeleteSalesTransaction) // delete sales transaction -- activity logged here if succesfull
	// sales session routes
	// api.Post("/sales/session/branch/:branch_id", salesController.OpenSalesSession)          // open sales session
	// api.Post("/sales/session/:session_id/product", salesController.AddProductItemToSession) // add product to session
	// api.Post("/sales/session/:session_id/close", salesController.CloseSalesSession)         // close sales session
	// api.Get("/sales/session/:session_id", salesController.GetSalesSession)                  // get sales session
	// api.Delete("/sales/session/:session_id", salesController.DeleteSalesSession)            // delete sales session
	// api.Get("/sales/session/branch/:branch_id", salesController.GetSalesSessionsOfBranch)   // get sales of session
	// router.Get("/sales/branch/:branch_id/sessions", salesController.GetSalesOfSession)

}

func ProductsRoutes(router *fiber.App, productsController *products.ProductsController, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")

	api.Post("/products", productsController.CreateProduct)                        // create product -- activity logged here if succesfull
	api.Put("/products/:id", productsController.EditProduct)                       // edit product -- activity logged here if succesfull
	api.Delete("/products/:id", productsController.DeleteProduct)                  // delete product -- activity logged here if succesfull
	api.Get("/products/:id", productsController.GetProductByID)                    // get product by id
	api.Get("/products", productsController.QueryProducts)                         // query products
	api.Post("/products/:id/income", productsController.NewIncome)                 // create income -- activity logged here if succesfull
	api.Post("/products/transfer", productsController.NewTransfer)                 // create transfer -- activity logged here if succesfull
	api.Post("/products/:id/images", productsController.UploadProductImage)        // upload product image
	api.Delete("/products/:id/images/:key", productsController.DeleteProductImage) // delete product image
	api.Get("/products/:id/images", productsController.GetImagesOfProduct)         // get images of product
	api.Get("/products/images/:key", productsController.GetImage)                  // get image
}

func JournalsRoutes(router *fiber.App, journalsController *journal_handlers.JournalHandlers, operationsController *journal_handlers.OperationHandlers, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Get("/journals/:id", journalsController.GetJournalEntryByID)                                                  // get journal entry by id
	api.Get("/journals/branch/:branch_id", journalsController.QueryJournalEntries)                                    // query journal entries
	api.Post("/journals", journalsController.NewJournalEntry)                                                         // create journal entry -- activity logged here if succesfull
	api.Post("/journals/:id/close", operationsController.ShiftIsOpenMiddleware, journalsController.CloseJournalEntry) // close journal entry -- activity logged here if succesfull
	api.Post("/journals/:id/reopen", journalsController.ReOpenJournalEntry)                                           // reopen journal entry -- activity logged here if succesfull

	// operations
	api.Post("/journals/:id/operations", operationsController.ShiftIsOpenMiddleware, operationsController.NewOperationTransaction)                        // create operation transaction -- activity logged here if succesfull
	api.Put("/journals/:id/operations/:operation_id", operationsController.ShiftIsOpenMiddleware, operationsController.UpdateOperationTransactionByID)    // update operation transaction by id -- activity logged here if succesfull
	api.Delete("/journals/:id/operations/:operation_id", operationsController.ShiftIsOpenMiddleware, operationsController.DeleteOperationTransactionByID) // delete operation transaction by id -- activity logged here if succesfull
	api.Get("/journals/:id/operations/:operation_id", operationsController.GetOperationTransactionByID)                                                   // get operation transaction by id

}

func InternalExpensesRoutes(router *fiber.App, middleware *middleware.Middlewares) {

}

func FinanceRoutes(router *fiber.App, financeController *finance.FinanceController, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Get("/finance/branches", financeController.GetBranches)                            // get all branches
	api.Get("/finance/branch/id/:id", financeController.GetFinanceBranchByBranchID)        // get branch by id
	api.Get("/finance/branch/name/:branch_name", financeController.GetFinanceByBranchName) // get branch by name
	api.Get("/finance/id/:id", financeController.GetFinanceByID)                           // get finance by id
	api.Post("/finance", financeController.NewFinanceOfBranch)                             // create new finance of branch -- activity logged here if succesfull

}

func TransactionsRoutes(router *fiber.App, transactionsController *transactions.TransactionsController, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Get("/transactions/branch/:branch_id", transactionsController.GetTransactionsByQueryParams) // get transactions by query params
	api.Get("/transactions/:id", transactionsController.GetTransactionByID)                         // get transaction by id
	// router.Post("/transactions/:branch_id", transactionsController.Tra)
	api.Put("/transactions/:id", transactionsController.UpdateTransactionByID)            // update transaction by id
	api.Delete("/transactions/:id", transactionsController.DeleteTransactionByID)         // delete transaction by id
	api.Get("/transactions/docs/initiator_type", transactionsController.GetInitiatorType) // get initiator type
	api.Get("/transactions/docs/type", transactionsController.GetTransactionType)         // get transaction type
	api.Get("/transactions/docs/payment_method", transactionsController.GetPaymentMethod) // get payment method
}

func CustomerRoutes(router *fiber.App, customerController *customers.CustomersController, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Get("/customers", customerController.GetCustomers)          // get all customers
	api.Get("/customers/:id", customerController.GetCustomerByID)   // get customer by id
	api.Post("/customers", customerController.CreateCustomer)       // create customer -- activity logged here if succesfull
	api.Put("/customers/:id", customerController.UpdateCustomer)    // update customer
	api.Delete("/customers/:id", customerController.DeleteCustomer) // delete customer
}

func BNPLRoutes(router *fiber.App, bnplController *bnpl.BNPLController, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Post("/bnpl", bnplController.NewBNPL)                                   // create bnpl -- activity logged here if succesfull
	api.Post("/bnpl/:id/credit", bnplController.CreditBNPL)                     // credit bnpl
	api.Delete("/bnpl/:id", bnplController.DeleteBNPL)                          // delete bnpl
	api.Get("/bnpl/:id", bnplController.GetBNPLByID)                            // get bnpl by id
	api.Get("/customers/:customer_id/bnpls", bnplController.GetBNPLSofCustomer) // get bnpls of customer
	api.Get("/branches/:branch_id/bnpls", bnplController.GetBNPLsOfBranch)      // get bnpls of branch
}

func ProposalsRoutes(router *fiber.App, proposalsController *arrivals.ProposalsHandlers, middleware *middleware.Middlewares) {
	api := router.Group("/api/v1/admin")
	api.Get("/proposals", proposalsController.GetProposals)               // get proposals
	api.Get("/proposals/detail", proposalsController.GetProposalDetail)   // get proposal by id
	api.Post("/proposals/new", proposalsController.NewProposals)          // create proposal
	api.Put("/proposals/edit", proposalsController.EditProposal)          // update proposal
	api.Delete("/proposals/delete", proposalsController.DeleteProposal)   // delete proposal
	api.Get("/proposals/image", proposalsController.GetImageByProposalID) // get image by proposal id
	api.Post("/proposals/image", proposalsController.UploadImage)         // upload image
	api.Get("/proposals/pdf/pdf", proposalsController.GeneratePDF)        // generate pdf
	api.Get("/proposals/fulfill", proposalsController.FulfillProposals)   // fulfill proposals
}

func ForwardProxy(c *fiber.Ctx) error {
	log.Info().
		Str("method", c.Method()).
		Str("original_url", c.OriginalURL()).
		Msg("Starting proxy request")

	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Error().Err(err).Msg("Failed to load config")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	proxyConfig := config.Server.Proxy[0]

	// Create target URL
	targetURL := proxyConfig.Addr + c.OriginalURL()
	log.Debug().Str("target_url", targetURL).Msg("Created target URL")

	// Create HTTP client
	client := &http.Client{}

	// Create request with same method and body
	req, err := http.NewRequest(c.Method(), targetURL, bytes.NewReader(c.Body()))
	if err != nil {
		log.Error().Err(err).Str("target_url", targetURL).Msg("Failed to create HTTP request")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// Copy headers from original request
	for key, values := range c.GetReqHeaders() {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	// set x-api-key to header
	req.Header.Set("x-api-key", config.Server.Proxy[0].APIKey)
	log.Debug().Interface("headers", req.Header).Msg("Copied headers to proxy request")

	// Make the request
	resp, err := client.Do(req)
	if err != nil {
		log.Error().Err(err).Str("target_url", targetURL).Msg("Failed to make proxy request")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	defer resp.Body.Close()

	log.Info().
		Int("status_code", resp.StatusCode).
		Str("target_url", targetURL).
		Msg("Received response from proxy target")

	// Copy response headers
	for key, values := range resp.Header {
		for _, value := range values {
			c.Set(key, value)
		}
	}

	// Copy response status and body
	c.Status(resp.StatusCode)
	log.Debug().Int("status_code", resp.StatusCode).Msg("Forwarding response to client")
	// read the body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error().Err(err).Msg("Failed to read response body")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	log.Debug().Str("body", string(body)).Msg("Response body")
	return c.Send(body)
}

// ProxyRoutes sets up proxy routes for forwarding requests to external services
// @Summary Setup proxy routes
// @Description Configures proxy routes based on server configuration to forward requests to external services
// @Tags proxy
// @Security BearerAuth
// @Router /proposals [get]
// @Router /proposals [post]
// @Router /proposals [put]
// @Router /proposals [delete]
func ProxyRoutes(router *fiber.App, middleware *middleware.Middlewares) {

	config, err := configs.LoadConfig(".")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to load config")
	}

	proxyConfig := config.Server.Proxy[0]

	zayavka := router.Group(proxyConfig.Path)
	keyBytes, err := base64.StdEncoding.DecodeString(config.Server.SecretSymmetricKey)
	if err != nil {
		log.Fatal().Err(err).Msg("invalid symmetric key")
	}
	zayavka.Use(pasetoware.New(
		pasetoware.Config{
			SymmetricKey: keyBytes,
			// TokenPrefix:    "Bearer",
			SuccessHandler: middleware.AuthMiddleware,
		},
	))
	zayavka.Use(ForwardProxy)
}
