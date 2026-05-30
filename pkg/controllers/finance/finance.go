package finance

import (
	"context"
	"net/url"

	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

type FinanceController struct {
	FinanceCollection    *mongo.Collection
	ActivitiesCollection *mongo.Collection
}

func New(db *mongo.Database) *FinanceController {
	log.Info().Msg("Initializing FinanceController")
	financeCollection := db.Collection("finance")

	log.Info().Msg("Creating indexes for finance collection")
	financeCollection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.M{"branch_id": 1},
	})
	_, _ = financeCollection.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.M{"branch_name": 1},
		Options: options.Index().SetUnique(true),
	})

	log.Info().Msg("FinanceController initialized successfully")
	return &FinanceController{
		FinanceCollection:    financeCollection,
		ActivitiesCollection: db.Collection("activities"),
	}
}

// GetBranches godoc
// @Security BearerAuth
// @Summary Fetch all branches
// @Description Retrieve all branches from the finance collection
// @Tags finance
// @Produce json
// @Success 200 {object} models.Output[[]models.BranchFinance]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/finance/branches [get]
func (f *FinanceController) GetBranches(c *fiber.Ctx) error {
	log.Debug().Msg("Fetching all branches")
	cursor, err := f.FinanceCollection.Find(context.Background(), bson.M{})
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch branches")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.NewError(err.Error(), fiber.StatusInternalServerError)))
	}

	var branches []models.BranchFinance
	// database_url := f.collection.Database().Client()
	// log.Debug().Str("Databse URL", f.collection.Database().Client()).Msg("Fetching branches")
	if err := cursor.All(context.Background(), &branches); err != nil {
		log.Error().Err(err).Msg("Failed to decode branches")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.NewError(err.Error(), fiber.StatusInternalServerError)))
	}
	if branches == nil {
		log.Warn().Msg("No branches found")
		return c.JSON(models.NewOutput([]string{}, models.Error{
			Message: "No branches found",
			Code:    fiber.StatusOK,
		}))
	}

	log.Debug().Int("count", len(branches)).Msg("Successfully fetched branches")
	return c.JSON(models.NewOutput(branches))
}

// GetBranchByBranchID godoc
// @Security BearerAuth
// @Summary Fetch branch by ID
// @Description Retrieve a branch using its ID
// @Tags finance
// @Param id path string true "Branch ID"
// @Produce json
// @Success 200 {object} models.Output[models.BranchFinance]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/finance/branch/id/{id} [get]
func (f *FinanceController) GetFinanceBranchByBranchID(c *fiber.Ctx) error {
	branchID := c.Params("id")
	log.Debug().Str("branch_id", branchID).Msg("Fetching branch by ID")
	filter := bson.M{"branch_id": branchID}

	var branch models.BranchFinance
	err := f.FinanceCollection.FindOne(context.Background(), filter).Decode(&branch)
	if err != nil {
		log.Error().Err(err).Str("branch_id", branchID).Msg("Branch not found")
		return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.NewError("Branch not found", fiber.StatusNotFound)))
	}

	log.Debug().Str("branch_id", branchID).Msg("Successfully fetched branch")
	return c.JSON(models.NewOutput(branch))
}

// GetFinanceByBranchName godoc
// @Security BearerAuth
// @Summary Fetch finance by branch name
// @Description Retrieve finance details using the branch name
// @Tags finance
// @Param branch_name path string true "Branch Name"
// @Produce json
// @Success 200 {object} models.Output[models.BranchFinance]
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/finance/branch/name/{branch_name} [get]
func (f *FinanceController) GetFinanceByBranchName(c *fiber.Ctx) error {
	branchName := c.Params("branch_name")
	log.Debug().Str("branch_name", branchName).Msg("Fetching finance by branch name")
	// normalize the branch name as it may contain spaces or special characters
	normalizedBranchName, _ := url.QueryUnescape(branchName)
	branchName = normalizedBranchName
	log.Info().Str("branch_name", branchName).Msg("Normalized branch name")
	filter := bson.M{"branch_name": bson.M{"$regex": "^" + normalizedBranchName + "$", "$options": "i"}}

	var branch models.BranchFinance
	err := f.FinanceCollection.FindOne(context.Background(), filter).Decode(&branch)
	if err != nil {
		log.Error().Err(err).Str("branch_name", branchName).Msg("Branch not found")
		return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.NewError("Branch not found", fiber.StatusNotFound)))
	}

	log.Debug().Str("branch_name", branchName).Msg("Successfully fetched branch finance")
	return c.JSON(models.NewOutput(branch))
}

// NewFinanceOfBranch godoc
// @Security BearerAuth
// @Summary Create new finance for a branch
// @Description Add new financial records for a branch
// @Tags finance
// @Accept json
// @Produce json
// @Param branch body models.NewBranchFinanceInput true "Branch finance input"
// @Success 201 {object} models.Output[models.BranchFinance]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/finance [post]
func (f *FinanceController) NewFinanceOfBranch(c *fiber.Ctx) error {
	log.Debug().Msg("Creating new finance for branch")
	var Input models.NewBranchFinanceInput

	if err := c.BodyParser(&Input); err != nil {
		log.Error().Err(err).Msg("Failed to parse input for new branch finance")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.NewError(err.Error(), fiber.StatusBadRequest)))
	}

	branchFinance := models.BranchFinance{
		BranchID:   uuid.New().String(),
		BranchName: Input.BranchName,
		Details:    Input.Details,
		Finance: models.Finance{
			Balance: models.Balance{
				Cash:       0,
				Bank:       0,
				MobileApps: 0,
			},
			TotalIncome:   0,
			TotalExpenses: 0,
			Debt:          0,
		},
		Suppliers: []string{},
	}

	log.Info().Str("branch_id", branchFinance.BranchID).Msg("Inserting new branch finance")
	_, err := f.FinanceCollection.InsertOne(context.Background(), branchFinance)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert new branch finance")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.NewError(err.Error(), fiber.StatusInternalServerError)))
	}

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateFinance, branchFinance, f.ActivitiesCollection)

	log.Debug().Str("branch_id", branchFinance.BranchID).Msg("Successfully created new branch finance")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(branchFinance))
}

// GetFinanceByID godoc
// @Security BearerAuth
// @Summary Fetch finance by ID
// @Description Retrieve finance details using its ID --- ObjectID not branch_id
// @Tags finance
// @Param id path string true "Finance ID"
// @Produce json
// @Success 200 {object} models.Output[models.BranchFinance]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Router /api/v1/admin/finance/id/{id} [get]
func (f *FinanceController) GetFinanceByID(c *fiber.Ctx) error {
	financeID := c.Params("id")
	log.Debug().Str("id", financeID).Msg("Fetching finance by ID")

	// convert to objectsid
	financeIDBson, err := bson.ObjectIDFromHex(financeID)
	if err != nil {
		log.Error().Err(err).Str("id", financeID).Msg("Invalid finance ID")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.NewError("Invalid finance ID", fiber.StatusBadRequest)))
	}

	filter := bson.M{"_id": financeIDBson}

	var finance models.BranchFinance
	err = f.FinanceCollection.FindOne(context.Background(), filter).Decode(&finance)
	if err != nil {
		log.Error().Err(err).Str("id", financeID).Msg("Finance not found")
		return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.NewError("Finance not found", fiber.StatusNotFound)))
	}

	log.Debug().Str("id", financeID).Msg("Successfully fetched finance")
	return c.JSON(models.NewOutput(finance))
}
