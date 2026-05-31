package journal_handlers

import (
	"context"
	"errors"
	"fmt"

	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	journalsrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/journals"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

// JournalHandlers exposes the admin journals endpoints. All database access goes
// through Repo; the controller only parses requests, logs audit activity, and
// renders the response envelope.
type JournalHandlers struct {
	ctx                  context.Context
	Repo                 *journalsrepo.JournalsRepository
	ActivitiesCollection *mongo.Collection
	Tracer               trace.Tracer
}

func New(db *mongo.Database) *JournalHandlers {
	ctx := context.Background()
	activitiesCollection := db.Collection("activities")
	tracer := otel.Tracer("journals")
	return &JournalHandlers{
		ctx:                  ctx,
		Repo:                 journalsrepo.New(db),
		ActivitiesCollection: activitiesCollection,
		Tracer:               tracer,
	}
}

// GetJournalEntryByID godoc
// @Security BearerAuth
// @Summary Get a journal entry by ID
// @Description Get a journal entry by its ID
// @Tags journals
// @Produce json
// @Param journal_id path string true "Journal ID"
// @Success 200 {object} models.Output[models.Journal]
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/{journal_id} [get]
func (j *JournalHandlers) GetJournalEntryByID(c *fiber.Ctx) error {
	journal, err := j.Repo.GetByID(c.Context(), c.Params("id"), true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch journal entry by ID")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	log.Info().Msg("Successfully fetched journal entry by ID")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(journal))
}

// QueryJournalEntries godoc
// @Security BearerAuth
// @Summary Query journal entries
// @Description Query journal entries by branch ID
// @Tags journals
// @Produce json
// @Param branch_id path string true "Branch ID"
// @Param page query int false "Page number"
// @Param page_size query int false "Page size"
// @Success 200 {object} models.Output[[]models.Journal]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/branch/{branch_id} [get]
func (j *JournalHandlers) QueryJournalEntries(c *fiber.Ctx) error {
	ctx, span := j.Tracer.Start(j.ctx, "query_journal_entries")
	defer span.End()
	log.Info().Msg("Querying journal entries --- using tracer")

	queryParams := models.JournalQueryParams{}
	if err := c.QueryParser(&queryParams); err != nil {
		log.Error().Err(err).Msg("Failed to parse query parameters")
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}
	span.AddEvent("query_params", trace.WithAttributes(attribute.String("query_params", fmt.Sprintf("%v", queryParams))))
	log.Info().Interface("queryParams", queryParams).Str("branch_id", c.Params("branch_id")).Msg("Querying journal entries")
	if queryParams.Page == 0 {
		queryParams.Page = 1
	}
	if queryParams.PageSize == 0 {
		queryParams.PageSize = 10
	}
	queryParams.BranchID = c.Params("branch_id")
	log.Info().Interface("queryParams", queryParams).Msg("Querying journal entries")
	results, err := j.Repo.Query(ctx, queryParams)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query journal entries")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(results))
}

// NewJournalEntry godoc
// @Security BearerAuth
// @Summary Create a new journal entry
// @Description Create a new journal entry for a branch
// @Tags journals
// @Accept json
// @Produce json
// @Param input body models.NewJournalEntryInput true "New Journal Entry Input"
// @Success 201 {object} models.Output[models.JournalWithTransactionID]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals [post]
func (j *JournalHandlers) NewJournalEntry(c *fiber.Ctx) error {
	log.Info().Msg("Creating new journal entry")
	input := models.NewJournalEntryInput{}
	if err := c.BodyParser(&input); err != nil {
		log.Error().Err(err).Msg("Failed to parse new journal entry input")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateJournal, input, j.ActivitiesCollection)

	journal, err := j.Repo.Create(c.Context(), input)
	if err != nil {
		// The branch lookup failure originally surfaced as a 400; any other
		// failure (e.g. the insert) surfaced as a 500. Preserve both codes.
		if errors.Is(err, repoerr.ErrNotFound) || errors.Is(err, repoerr.ErrInvalidInput) {
			log.Error().Err(err).Msg("Failed to find branch in finance collection")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		log.Error().Err(err).Msg("Failed to insert new journal entry")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
	log.Info().Msg("Successfully created new journal entry")

	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(journal))
}

// CloseJournalEntry godoc
// @Security BearerAuth
// @Summary Close a journal entry
// @Description Close a journal entry by updating its transactions
// @Tags journals
// @Accept json
// @Produce json
// @Param journal_id path string true "Journal ID"
// @Param input body models.CloseJournalEntryInput true "Close Journal Entry Input"
// @Success 200 {object} models.Output[models.Journal]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/{journal_id}/close [post]
func (j *JournalHandlers) CloseJournalEntry(c *fiber.Ctx) error {
	log.Info().Msg("Closing journal entry")
	journalID, err := ParseJournalID(c)

	if err != nil {
		log.Error().Err(err).Msg("Failed to parse journal ID")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	input := models.CloseJournalEntryInput{}
	if err := c.BodyParser(&input); err != nil {
		log.Error().Err(err).Msg("Failed to parse close journal entry input")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	// log activity
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCloseJournal, map[string]string{
		"journal_id":      journalID.Hex(),
		"cash_left":       fmt.Sprintf("%d", input.CashLeft),
		"terminal_income": fmt.Sprintf("%d", input.TerminalIncome),
	}, j.ActivitiesCollection)

	journal, err := j.Repo.Close(c.Context(), journalID.Hex(), input)
	if err != nil {
		// The original handler returned 400 when the journal could not be
		// fetched, and 500 for every other (transaction/update/commit) failure.
		if errors.Is(err, repoerr.ErrNotFound) || errors.Is(err, repoerr.ErrInvalidInput) {
			log.Error().Err(err).Msg("Failed to find journal entry")
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": err.Error(),
			})
		}
		log.Error().Err(err).Msg("Failed to close journal entry")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().Msg("Successfully closed journal entry")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(journal))
}

// ReOpenJournalEntry godoc
// @Security BearerAuth
// @Summary Reopen a closed journal entry
// @Description Reopen a journal entry by removing its closing transactions
// @Tags journals
// @Produce json
// @Param journal_id path string true "Journal ID"
// @Success 200 {object} models.Output[models.Journal]
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/{journal_id}/reopen [post]
func (j *JournalHandlers) ReOpenJournalEntry(c *fiber.Ctx) error {
	log.Info().Msg("Reopening journal entry")

	journalID, err := ParseJournalID(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse journal ID")
		return models.RespondError(c, fiber.StatusBadRequest, err.Error())
	}

	journal_new, err := j.Repo.ReOpen(c.Context(), journalID.Hex())
	if err != nil {
		log.Error().Err(err).Msg("Failed to reopen journal entry")
		return models.RespondError(c, fiber.StatusInternalServerError, err.Error())
	}

	log.Info().Msg("Successfully reopened journal entry")
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeReopenJournal, map[string]string{
		"journal_id": journalID.Hex(),
	}, j.ActivitiesCollection)

	return c.Status(fiber.StatusOK).JSON(models.NewOutput(journal_new))
}

func ParseJournalID(c *fiber.Ctx) (bson.ObjectID, error) {
	journalID, err := bson.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse journal ID")
		return bson.NilObjectID, err
	}
	return journalID, nil
}

// GetReport godoc
// @Security BearerAuth
// @Summary Get a report
// @Description Get a report of journal entries
// @Tags journals
// @Produce json
// @Success 200 {object} models.MessageResponse
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/journals/report [get]
func (j *JournalHandlers) GetReport(c *fiber.Ctx) error {
	log.Info().Msg("Generating report")
	panic("Not implemented")
}

// QueryJournals runs the journal query pipeline against the supplied collection.
// It is retained as a package-level helper because the analytics dashboard
// controller calls it directly with its own journals collection. The repository
// owns the same logic for the journals controllers' own use; this helper mirrors
// it byte-for-byte so analytics behaviour is unchanged.
func QueryJournals(span trace.Span, ctx context.Context, c *fiber.Ctx, queryParams models.JournalQueryParams, journalsCollection *mongo.Collection) ([]models.Journal, error) {

	match := bson.M{}
	if queryParams.BranchID != "" {
		match["branch._id"] = queryParams.BranchID
	}
	if queryParams.Total.Use {
		match["total"] = bson.M{"$gte": queryParams.Total.Min, "$lte": queryParams.Total.Max}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$sort", Value: bson.D{{Key: "date", Value: -1}}}},              // Sort by date in ascending order
		{{Key: "$skip", Value: (queryParams.Page - 1) * queryParams.PageSize}}, // Skip the first N documents = page_size * page_number
		{{Key: "$limit", Value: queryParams.PageSize}},                         // Limit the number of documents returned = page_size
		{
			{Key: "$lookup", Value: bson.M{
				"from":         "transactions",
				"localField":   "operations",
				"foreignField": "_id",
				"as":           "transactions",
			}},
		},
		{
			{Key: "$project", Value: bson.M{
				"date":            1,
				"total":           1,
				"shift_is_closed": 1,
				"terminal_income": 1,
				"cash_left":       1,
				"branch":          1,
				"operations":      "$transactions",
			}},
		},
	}
	span.AddEvent("Created pipeline")
	cursor, err := journalsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate journal entries")
		return nil, err
	}
	defer cursor.Close(ctx)
	span.AddEvent("Got cursor")
	var results []models.Journal
	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("Failed to decode journal entries")
		return nil, err
	}
	span.AddEvent("Got results", trace.WithAttributes(attribute.Int("results_count", len(results))))
	log.Info().Int("results_count", len(results)).Msgf("Successfully queried journal entries")
	return results, nil
}
