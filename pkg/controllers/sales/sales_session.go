package sales

// import (
// 	"errors"

// 	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
// 	"github.com/aslon1213/g4h_pos_erp/platform/database"

// 	"github.com/gofiber/fiber/v2"
// 	"github.com/rs/zerolog/log"
// 	"go.mongodb.org/mongo-driver/v2/bson"
// )

// // OpenSalesSession godoc
// // @Security BearerAuth
// // @Summary Open a new sales session
// // @Description Creates a new sales session for a branch
// // @Tags sales/session
// // @Accept json
// // @Produce json
// // @Param branch_id path string true "Branch ID"
// // @Success 200 {object} models.Output[any]
// // @Router /api/sales/session/branch/{branch_id} [post]
// func (s *SalesTransactionsController) OpenSalesSession(c *fiber.Ctx) error {
// 	branch_id := c.Params("branch_id")
// 	log.Info().Str("branch_id", branch_id).Msg("Opening new sales session")

// 	// check for branch_id existing
// 	count, err := s.finances.CountDocuments(c.Context(), bson.M{"_id": branch_id})
// 	if err != nil {
// 		log.Error().Err(err).Str("branch_id", branch_id).Msg("Failed to find branch")
// 		return models.ReturnError(c, err)
// 	}

// 	if count == 0 {
// 		log.Error().Str("branch_id", branch_id).Msg("Branch not found")
// 		return models.ReturnError(c, errors.New("branch not found"))
// 	}

// 	session, err := models.NewSalesSession(branch_id, s.cache)
// 	if err != nil {
// 		log.Error().Err(err).Msg("Failed to create new sales session")
// 		return models.ReturnError(c, err)
// 	}
// 	return c.JSON(models.NewOutput([]*models.SalesSession{session}))
// }

// type AddProductItemToSessionInput struct {
// 	ID       string `json:"id"`
// 	Quantity int    `json:"quantity"`
// }

// // AddProductItemToSession godoc
// // @Security BearerAuth
// // @Summary Add product to sales session
// // @Description Adds a product item to an existing sales session
// // @Tags sales/session
// // @Accept json
// // @Produce json
// // @Param session_id path string true "Session ID"
// // @Param product body AddProductItemToSessionInput true "Product details"
// // @Success 200 {object} models.Output[any]
// // @Failure 400 {object} models.ErrorOutput
// // @Failure 500 {object} models.ErrorOutput
// // @Router /api/sales/session/{session_id}/product [post]
// func (s *SalesTransactionsController) AddProductItemToSession(c *fiber.Ctx) error {
// 	session_id := c.Params("session_id")

// 	product_item := AddProductItemToSessionInput{}
// 	if err := c.BodyParser(&product_item); err != nil {
// 		log.Error().Err(err).Msg("Failed to parse product item")
// 		return models.ReturnError(c, err)
// 	}

// 	log.Info().Str("session_id", session_id).Interface("product_item", product_item).Msg("Adding product to session")
// 	session, err := models.GetSalesSession(session_id, s.cache)
// 	if err != nil {
// 		log.Error().Err(err).Str("session_id", session_id).Msg("Failed to get sales session")
// 		return models.ReturnError(c, err)
// 	}

// 	product := models.Product{}
// 	err = s.products.FindOne(c.Context(), bson.M{"_id": product_item.ID}).Decode(&product)
// 	if err != nil {
// 		log.Error().Err(err).Str("product_id", product_item.ID).Msg("Failed to find product")
// 		return models.ReturnError(c, err)
// 	}
// 	log.Info().Interface("product_item", product_item).Msg("Product item")

// 	for _, distribution := range product.QuantityDistribution {
// 		if distribution.Place.PlaceType == models.ProductPlaceTypeBranch {
// 			log.Info().Str("branch_id", distribution.Place.ID).Msg("Adding product to session")

// 			// TODO: here may be the logic for synchronizing the product quantity of this product ---
// 			// handle situations when the product quantity is not enough, product is not available, product is not in the branch, etc.

// 			err = session.AddProductItem(product_item.ID, product_item.Quantity, distribution.Price, s.cache)
// 			if err != nil {
// 				log.Error().Err(err).Str("product_id", product_item.ID).Msg("Failed to add product item to session")
// 				return models.ReturnError(c, err)
// 			}
// 			log.Info().
// 				Str("product_id", product_item.ID).
// 				Int("quantity", product_item.Quantity).
// 				Int32("price", distribution.Price).
// 				Msg("Added product item to session")
// 		}
// 	}

// 	return c.JSON(models.NewOutput([]*models.SalesSession{session}))
// }

// // CloseSalesSession godoc
// // @Security BearerAuth
// // @Summary Close a sales session
// // @Description Closes an existing sales session and processes the transaction
// // @Tags sales/session
// // @Accept json
// // @Produce json
// // @Param session_id path string true "Session ID"
// // @Success 200 {object} models.Output[any]
// // @Failure 500 {object} models.ErrorOutput
// // @Router /api/sales/session/{session_id}/close [post]
// func (s *SalesTransactionsController) CloseSalesSession(c *fiber.Ctx) error {
// 	session_id := c.Params("session_id")
// 	log.Info().Str("session_id", session_id).Msg("Closing sales session")
// 	session, err := models.GetSalesSession(session_id, s.cache)
// 	if err != nil {
// 		log.Error().Err(err).Str("session_id", session_id).Msg("Failed to get sales session")
// 		return models.ReturnError(c, err)
// 	}

// 	ses, ctx, err := database.StartTransaction(s.transactions.Database().Client())
// 	if err != nil {
// 		log.Error().Err(err).Msg("Failed to start transaction")
// 		return models.ReturnError(c, err)
// 	}
// 	defer ses.EndSession(ctx)

// 	total_price := 0
// 	for id, item := range session.Products {
// 		total_price += int(item.Price) * item.Quantity
// 		log.Debug().
// 			Str("product_id", id).
// 			Int("quantity", item.Quantity).
// 			Int32("price", item.Price).
// 			Int("subtotal", int(item.Price)*item.Quantity).
// 			Msg("Calculating product price")
// 	}

// 	session.DeleteSession(s.cache)
// 	panic("Not implemented")

// 	if err := ses.CommitTransaction(ctx); err != nil {
// 		log.Error().Err(err).Msg("Failed to commit transaction")
// 		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
// 			Message: err.Error(),
// 			Code:    fiber.StatusInternalServerError,
// 		}))
// 	}

// 	log.Info().
// 		Str("session_id", session_id).
// 		Int("total_price", total_price).
// 		Msg("Sales session closed successfully")

// 	return c.JSON(models.NewOutput(fiber.Map{
// 		"total_price": total_price,
// 		"session":     session,
// 	}))
// }

// // GetSalesSession godoc
// // @Security BearerAuth
// // @Summary Get a sales session by ID
// // @Description Retrieves details of a specific sales session
// // @Tags sales/session
// // @Accept json
// // @Produce json
// // @Param session_id path string true "Session ID"
// // @Success 200 {object} models.Output[any]
// // @Failure 400 {object} models.ErrorOutput
// // @Failure 404 {object} models.ErrorOutput
// // @Failure 500 {object} models.ErrorOutput
// // @Router /api/sales/session/{session_id} [get]
// func (s *SalesTransactionsController) GetSalesSession(c *fiber.Ctx) error {
// 	session_id := c.Params("session_id")
// 	log.Info().Str("session_id", session_id).Msg("Getting sales session")

// 	session, err := models.GetSalesSession(session_id, s.cache)
// 	if err != nil {
// 		log.Error().Err(err).Str("session_id", session_id).Msg("Failed to get sales session")
// 		return models.ReturnError(c, err)
// 	}

// 	log.Debug().
// 		Str("session_id", session_id).
// 		Interface("session", session).
// 		Msg("Successfully retrieved sales session")

// 	return c.JSON(models.NewOutput([]*models.SalesSession{session}))
// }

// // GetSalesOfSession godoc
// // @Security BearerAuth
// // @Summary Get all sales sessions for a branch
// // @Description Retrieves all sales sessions associated with a branch
// // @Tags sales/session
// // @Accept json
// // @Produce json
// // @Param branch_id path string true "Branch ID"
// // @Success 200 {object} models.Output[any]
// // @Failure 400 {object} models.ErrorOutput
// // @Failure 404 {object} models.ErrorOutput
// // @Failure 500 {object} models.ErrorOutput
// // @Router /api/sales/session/branch/{branch_id} [get]
// func (s *SalesTransactionsController) GetSalesSessionsOfBranch(c *fiber.Ctx) error {
// 	branch_id := c.Params("branch_id")
// 	log.Info().Str("branch_id", branch_id).Msg("Getting sales sessions for branch")

// 	sessions, err := models.GetSalesSessionByBranchID(branch_id, s.cache)
// 	if err != nil {
// 		log.Error().Err(err).Str("branch_id", branch_id).Msg("Failed to get sales sessions")
// 		return models.ReturnError(c, err)
// 	}

// 	log.Debug().
// 		Str("branch_id", branch_id).
// 		Int("session_count", len(sessions)).
// 		Msg("Successfully retrieved sales sessions")

// 	return c.JSON(models.NewOutput(sessions))
// }

// // DeleteSalesSession godoc
// // @Security BearerAuth
// // @Summary Delete a sales session
// // @Description Deletes a sales session by ID
// // @Tags sales/session
// // @Accept json
// // @Produce json
// // @Param session_id path string true "Session ID"
// // @Success 200 {object} models.Output[any]
// // @Failure 400 {object} models.ErrorOutput
// // @Failure 404 {object} models.ErrorOutput
// // @Failure 500 {object} models.ErrorOutput
// // @Router /api/sales/session/{session_id} [delete]
// func (s *SalesTransactionsController) DeleteSalesSession(c *fiber.Ctx) error {
// 	session_id := c.Params("session_id")
// 	log.Info().Str("session_id", session_id).Msg("Deleting sales session")

// 	session, err := models.GetSalesSession(session_id, s.cache)
// 	if err != nil {
// 		log.Error().Err(err).Str("session_id", session_id).Msg("Failed to get sales session")
// 		return models.ReturnError(c, err)
// 	}

// 	err = session.DeleteSession(s.cache)
// 	if err != nil {
// 		log.Error().Err(err).Str("session_id", session_id).Msg("Failed to delete sales session")
// 		return models.ReturnError(c, err)
// 	}

// 	return c.JSON(models.NewOutput([]*models.SalesSession{session}))
// }
