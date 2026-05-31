package finance

import (
	"net/url"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	financerepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/finance"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// FinanceController exposes the admin finance endpoints. All database access
// goes through Repo; the controller parses requests, logs audit activity, and
// renders the response envelope.
type FinanceController struct {
	Repo                 *financerepo.FinanceRepository
	ActivitiesCollection *mongo.Collection
}

func New(db *mongo.Database) *FinanceController {
	log.Info().Msg("Initializing FinanceController")
	return &FinanceController{
		Repo:                 financerepo.New(db),
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
	branches, err := f.Repo.GetAllBranches(c.Context())
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	if len(branches) == 0 {
		log.Warn().Msg("No branches found")
		return c.JSON(models.NewOutput([]string{}, models.NewError("No branches found", fiber.StatusOK)))
	}
	return c.JSON(models.NewOutput(branches))
}

// GetFinanceBranchByBranchID godoc
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
	branch, err := f.Repo.GetByBranchID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
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
	// Normalize the branch name as it may contain spaces or special characters.
	branchName, _ := url.QueryUnescape(c.Params("branch_name"))
	log.Info().Str("branch_name", branchName).Msg("Fetching finance by (normalized) branch name")

	branch, err := f.Repo.GetByBranchName(c.Context(), branchName)
	if err != nil {
		return models.RespondRepoError(c, err)
	}
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
	var input models.NewBranchFinanceInput
	if err := c.BodyParser(&input); err != nil {
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	branch, err := f.Repo.CreateBranch(c.Context(), input)
	if err != nil {
		// A duplicate branch name surfaces here as a 500 (documented behaviour).
		return models.RespondRepoError(c, err)
	}

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateFinance, branch, f.ActivitiesCollection)
	log.Debug().Str("branch_id", branch.BranchID).Msg("Successfully created new branch finance")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(branch))
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
	branch, err := f.Repo.GetByObjectID(c.Context(), c.Params("id"))
	if err != nil {
		return models.RespondRepoError(c, err)
	}
	return c.JSON(models.NewOutput(branch))
}
