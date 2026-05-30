package suppliers

import (
	"context"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type SuppliersController struct {
	suppliersCollection    *mongo.Collection
	transactionsCollection *mongo.Collection
	financeCollection      *mongo.Collection
	activitiesCollection   *mongo.Collection
	DB                     *mongo.Database
}

func New(db *mongo.Database) *SuppliersController {
	// suppliers collection
	// suppliersCollection := db.Collection("suppliers")

	return &SuppliersController{
		suppliersCollection:    db.Collection("suppliers"),
		transactionsCollection: db.Collection("transactions"),
		financeCollection:      db.Collection("finance"),
		activitiesCollection:   db.Collection("activities"),
		DB:                     db,
	}
}

type SupplierQuery struct {
	Name    string `query:"name"`
	INN     string `query:"inn"`
	Branch  string `query:"branch"`
	Email   string `query:"email"`
	Phone   string `query:"phone"`
	Address string `query:"address"`
	Notes   string `query:"notes"`
}

// GetSuppliers godoc
// @Security BearerAuth
// @Summary Get all suppliers
// @Description Get all suppliers from the database
// @Tags suppliers
// @Accept json
// @Produce json
// @Param name query string false "Supplier name"
// @Param inn query string false "Supplier INN"
// @Param branch query string false "Supplier branch"
// @Param email query string false "Supplier email"
// @Param phone query string false "Supplier phone"
// @Param address query string false "Supplier address"
// @Param notes query string false "Supplier notes"
// @Success 200 {object} models.Output[[]models.Supplier]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers [get]
func (s *SuppliersController) GetSuppliers(c *fiber.Ctx) error {

	var query SupplierQuery
	if err := c.QueryParser(&query); err != nil {
		log.Error().Err(err).Msg("Failed to parse supplier query")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}
	name := query.Name
	inn := query.INN
	branch := query.Branch
	email := query.Email
	phone := query.Phone
	address := query.Address
	notes := query.Notes
	filter := bson.M{}

	log.Debug().Str("name", name).Str("inn", inn).Str("branch", branch).Str("email", email).Str("phone", phone).Str("address", address).Str("notes", notes).Msg("Getting all suppliers")

	if name != "" {
		filter["name"] = name
	}
	if inn != "" {
		filter["inn"] = inn
	}
	if branch != "" {
		// get branch by name or id
		branch_data := models.BranchFinance{}

		err := s.financeCollection.FindOne(context.Background(), bson.M{"$or": []bson.M{{"branch_id": branch}, {"branch_name": branch}}}).Decode(&branch_data)
		if err != nil {
			log.Error().Err(err).Str("id or name", branch).Msg("Branch not found")
			return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Branch not found",
				Code:    fiber.StatusNotFound,
			}))
		}
		filter["branch"] = branch_data.BranchID
	}
	if email != "" {
		filter["email"] = email
	}
	if phone != "" {
		filter["phone"] = phone
	}
	if address != "" {
		filter["address"] = address
	}
	if notes != "" {
		filter["notes"] = notes
	}
	log.Debug().Msg("Getting all suppliers")

	var suppliers []models.Supplier
	cursor, err := s.suppliersCollection.Find(context.Background(), filter)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find suppliers")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	if err := cursor.All(context.Background(), &suppliers); err != nil {
		log.Error().Err(err).Msg("Failed to decode suppliers")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Debug().Int("count", len(suppliers)).Msg("Successfully retrieved suppliers")
	if len(suppliers) == 0 {
		return c.JSON(models.NewOutput([]string{}))
	}
	return c.JSON(models.NewOutput(suppliers))
}

// GetSupplierByID godoc
// @Security BearerAuth
// @Summary Get a supplier by ID
// @Description Get a supplier by its ID
// @Tags suppliers
// @Accept json
// @Produce json
// @Param id path string true "Supplier ID"
// @Success 200 {object} models.Output[models.Supplier]
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{id} [get]
func (s *SuppliersController) GetSupplierByID(c *fiber.Ctx) error {
	id := c.Params("id")
	log.Debug().Str("id", id).Msg("Getting supplier by ID")

	var supplier models.Supplier
	err := s.suppliersCollection.FindOne(context.Background(), bson.M{"_id": id}).Decode(&supplier)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			log.Debug().Str("id", id).Msg("Supplier not found")
			return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Supplier not found",
				Code:    fiber.StatusNotFound,
			}))
		}
		log.Error().Err(err).Str("id", id).Msg("Failed to find supplier")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Debug().Str("id", id).Msg("Successfully retrieved supplier")
	return c.JSON(models.NewOutput(supplier))
}

// CreateSupplier godoc
// @Security BearerAuth
// @Summary Create a new supplier
// @Description Create a new supplier in the database
// @Tags suppliers
// @Accept json
// @Produce json
// @Param supplier body models.SupplierBase true "Supplier data"
// @Success 201 {object} models.Output[models.Supplier]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers [post]
func (s *SuppliersController) CreateSupplier(c *fiber.Ctx) error {
	log.Debug().Msg("Creating new supplier")

	var supplierBase models.SupplierBase
	if err := c.BodyParser(&supplierBase); err != nil {
		log.Error().Err(err).Msg("Failed to parse supplier data")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}
	loc := utils.GetTimeZone()
	now := time.Now().In(loc)
	supplier := models.Supplier{
		SupplierBase: supplierBase,
		ID:           uuid.New().String(),
		FinancialData: models.FinancialData{
			Balance:       0,
			Transactions:  []models.Transaction{},
			TotalIncome:   0,
			TotalExpenses: 0,
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateSupplier, supplier, s.activitiesCollection)

	// check the branch exists
	branch := models.BranchFinance{}
	err := s.financeCollection.FindOne(context.Background(), bson.M{"$or": []bson.M{{"branch_id": supplierBase.Branch}, {"branch_name": supplierBase.Branch}}}).Decode(&branch)
	if err != nil {
		log.Error().Err(err).Str("id or name", supplierBase.Branch).Msg("Branch not found")

		return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Branch not found",
			Code:    fiber.StatusNotFound,
		}))
	}

	supplier.Branch = branch.BranchID // set the branch id to the supplier ensuring that the supplier is associated with the branch

	_, err = s.suppliersCollection.InsertOne(context.Background(), supplier)
	if err != nil {
		log.Error().Err(err).Str("id", supplier.ID).Msg("Failed to insert supplier")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	// insert to supplier to finance collection
	_, err = s.financeCollection.UpdateOne(context.Background(), bson.M{"$or": []bson.M{{"branch_id": supplierBase.Branch}, {"branch_name": supplierBase.Branch}}}, bson.M{"$push": bson.M{"suppliers": bson.M{
		"$each": []string{supplier.ID},
	}}})
	if err != nil {
		log.Error().Err(err).Str("id", supplier.ID).Msg("Failed to insert supplier to finance")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	log.Debug().Str("id", supplier.ID).Msg("Successfully created supplier")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(supplier))
}

// UpdateSupplier godoc
// @Security BearerAuth
// @Summary Update a supplier
// @Description Update a supplier's information
// @Tags suppliers
// @Accept json
// @Produce json
// @Param id path string true "Supplier ID"
// @Param supplier body models.SupplierBase true "Supplier data"
// @Success 200 {object} models.MessageResponse
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{id} [put]
func (s *SuppliersController) UpdateSupplier(c *fiber.Ctx) error {
	id := c.Params("id")
	log.Debug().Str("id", id).Msg("Updating supplier")

	var supplierBase models.SupplierBase
	if err := c.BodyParser(&supplierBase); err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to parse supplier update data")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	loc := utils.GetTimeZone()
	now := time.Now().In(loc)
	update := bson.M{
		"$set": bson.M{
			"updated_at": now,
		},
	}
	if supplierBase.Email != "" {
		update["$set"].(bson.M)["email"] = supplierBase.Email
	}
	if supplierBase.INN != "" {
		update["$set"].(bson.M)["inn"] = supplierBase.INN
	}
	if supplierBase.Name != "" {
		update["$set"].(bson.M)["name"] = supplierBase.Name
	}
	if supplierBase.Address != "" {
		update["$set"].(bson.M)["address"] = supplierBase.Address
	}
	if supplierBase.Phone != "" {
		update["$set"].(bson.M)["phone"] = supplierBase.Phone
	}
	if supplierBase.Notes != "" {
		update["$set"].(bson.M)["notes"] = supplierBase.Notes
	}

	result, err := s.suppliersCollection.UpdateOne(context.Background(), bson.M{"_id": id}, update)
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to update supplier")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	if result.MatchedCount == 0 {
		log.Debug().Str("id", id).Msg("Supplier not found for update")
		return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Supplier not found",
			Code:    fiber.StatusNotFound,
		}))
	}

	log.Debug().Str("id", id).Msg("Successfully updated supplier")
	return c.JSON(models.NewOutput(fiber.Map{"message": "Supplier updated successfully"}))
}

// DeleteSupplier godoc
// @Security BearerAuth
// @Summary Delete a supplier
// @Description Delete a supplier from the database
// @Tags suppliers
// @Accept json
// @Produce json
// @Param id path string true "Supplier ID"
// @Success 200 {object} models.MessageResponse
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/suppliers/{id} [delete]
func (s *SuppliersController) DeleteSupplier(c *fiber.Ctx) error {
	id := c.Params("id")
	log.Debug().Str("id", id).Msg("Deleting supplier")

	result, err := s.suppliersCollection.DeleteOne(context.Background(), bson.M{"_id": id})
	if err != nil {
		log.Error().Err(err).Str("id", id).Msg("Failed to delete supplier")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	if result.DeletedCount == 0 {
		log.Debug().Str("id", id).Msg("Supplier not found for deletion")
		return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Supplier not found",
			Code:    fiber.StatusNotFound,
		}))
	}

	log.Debug().Str("id", id).Msg("Successfully deleted supplier")
	return c.JSON(models.NewOutput(fiber.Map{"message": "Supplier deleted successfully"}))
}
