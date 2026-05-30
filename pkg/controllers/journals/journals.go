package journal_handlers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/controllers/sales"
	"github.com/aslon1213/g4h_pos_erp/pkg/middleware"
	models "github.com/aslon1213/g4h_pos_erp/pkg/repository"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"
	"github.com/aslon1213/g4h_pos_erp/platform/database"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

type JournalHandlers struct {
	ctx                    context.Context
	JournalCollection      *mongo.Collection
	FinanceCollection      *mongo.Collection
	TransactionsCollection *mongo.Collection
	ActivitiesCollection   *mongo.Collection
	Tracer                 trace.Tracer
}

func New(db *mongo.Database) *JournalHandlers {
	ctx := context.Background()
	journalCollection := db.Collection("journals")
	index_model := mongo.IndexModel{
		Keys: bson.D{
			{Key: "date", Value: -1},
			{Key: "branch", Value: -1},
		},
		Options: options.Index().SetUnique(true),
	}
	_, err := journalCollection.Indexes().CreateOne(
		ctx,
		index_model,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create index for journals")
	}
	financeCollection := db.Collection("finance")
	transactionsCollection := db.Collection("transactions")
	activitiesCollection := db.Collection("activities")
	tracer := otel.Tracer("journals")
	return &JournalHandlers{
		ctx:                    ctx,
		JournalCollection:      journalCollection,
		FinanceCollection:      financeCollection,
		TransactionsCollection: transactionsCollection,
		ActivitiesCollection:   activitiesCollection,
		Tracer:                 tracer,
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
	// log.Info().Msg("Fetching journal entry by ID")
	journal, err := j.FetchJournalByID(j.ctx, c, true)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch journal entry by ID")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	log.Info().Msg("Successfully fetched journal entry by ID")
	// log.Info().Interface("journal", journal).Msg("Journal")
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
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
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
	results, err := QueryJournals(span, ctx, c, queryParams, j.JournalCollection)
	if err != nil {
		log.Error().Err(err).Msg("Failed to query journal entries")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	// log.Info().Interface("results", results).Msg("Results")
	return c.Status(fiber.StatusOK).JSON(models.NewOutput(results))
}

func QueryJournals(span trace.Span, ctx context.Context, c *fiber.Ctx, queryParams models.JournalQueryParams, journalsCollection *mongo.Collection) ([]models.Journal, error) {

	match := bson.M{}
	// if !queryParams.FromDate.IsZero() {
	// 	match["date"] = bson.M{"$gte": queryParams.FromDate}
	// }
	// if !queryParams.ToDate.IsZero() {
	// 	match["date"] = bson.M{"$lte": queryParams.ToDate}
	// }
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

	// log activity

	// get the branch from the database
	financeBranch := models.BranchFinance{}
	err := j.FinanceCollection.FindOne(j.ctx, bson.M{"$or": []bson.M{{"branch_name": bson.M{"$regex": input.BranchNameOrID, "$options": "i"}}, {"branch_id": input.BranchNameOrID}}}).Decode(&financeBranch)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find branch in finance collection")
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// parse the date to the timezone first and set to midnight
	loc := utils.GetTimeZone()
	input.Date = input.Date.In(loc)
	input.Date = time.Date(input.Date.Year(), input.Date.Month(), input.Date.Day(), 0, 0, 0, 0, loc)

	branch := models.Branch_names[input.BranchNameOrID]

	journal := models.JournalWithTransactionID{
		JournalBase: models.JournalBase{
			Branch: models.Branch{
				Name:     branch.Name,
				Location: branch.Location,
				Phone:    branch.Phone,
				ID:       financeBranch.BranchID,
			},
			Date:            input.Date,
			Shift_is_closed: false,
			Terminal_income: 0,
			Cash_left:       0,
			Total:           0,
			ID:              bson.NewObjectID(),
		},
		Operations: []string{},
	}

	middleware.LogActivityWithCtx(c, middleware.ActivityTypeCreateJournal, input, j.ActivitiesCollection)

	_, err = j.JournalCollection.InsertOne(j.ctx, journal)
	if err != nil {
		log.Error().Err(err).Msg("Failed to insert new journal entry")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.NewError(err.Error(), fiber.StatusInternalServerError)))
	}
	log.Info().Msg("Successfully created new journal entry")

	return c.Status(fiber.StatusCreated).JSON(models.NewOutput(
		journal,
	))

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

	// make transactions - [cash, terminal]

	// start a db transaction
	ses, ctx, err := database.StartTransaction(j.JournalCollection.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	defer ses.EndSession(ctx)
	// get journal info
	journal, err := FetchJournalByID(ctx, c, true, j.JournalCollection)
	if err != nil {
		log.Error().Err(err).Msg("Failed to find journal entry")

		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	branchID := journal.Branch.ID

	cash_transaction_base := models.TransactionBase{
		Amount:        input.CashLeft,
		Description:   "Cash left at the end of the day",
		PaymentMethod: models.PaymentMethodCash,
		Type:          models.TransactionTypeCredit,
	}
	terminal_transaction_base := models.TransactionBase{
		Amount:        input.TerminalIncome,
		Description:   "Terminal income at the end of the day",
		PaymentMethod: models.PaymentMethodTerminal,
		Type:          models.TransactionTypeCredit,
	}
	terminal_transaction, err := sales.NewTransaction(
		ctx,
		terminal_transaction_base,
		branchID,
		j.TransactionsCollection,
		j.FinanceCollection,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create terminal transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	cash_transaction, err := sales.NewTransaction(
		ctx,
		cash_transaction_base,
		branchID,
		j.TransactionsCollection,
		j.FinanceCollection,
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to create cash left transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}

	// update the journal
	_, err = j.JournalCollection.UpdateOne(ctx,
		bson.M{"_id": journalID},
		bson.M{
			"$set": bson.M{
				"cash_left": input.CashLeft, "terminal_income": input.TerminalIncome, "shift_is_closed": true, "total": journal.Total + input.CashLeft + input.TerminalIncome,
			},
			"$push": bson.M{"operations": bson.M{"$each": []string{terminal_transaction.ID, cash_transaction.ID}}},
		},
	)
	if err != nil {
		log.Error().Err(err).Msg("Failed to update journal entry")

		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": err.Error(),
		})
	}
	// commit the transaction
	if err := ses.CommitTransaction(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Msg("Successfully closed journal entry")
	journal.Total = journal.Total + input.CashLeft + input.TerminalIncome
	journal.Cash_left = input.CashLeft
	journal.Terminal_income = input.TerminalIncome
	return c.Status(fiber.StatusOK).JSON(
		models.NewOutput(
			journal,
		),
	)
}

func (j *JournalHandlers) FetchJournalByID(ctx context.Context, c *fiber.Ctx, fetchTransactions bool) (*models.Journal, error) {
	log.Info().Str("journal_id", c.Params("id")).Msg("Fetching journal by ID")
	return FetchJournalByID(ctx, c, fetchTransactions, j.JournalCollection)
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
	// start a db transaction
	ses, ctx, err := database.StartTransaction(j.JournalCollection.Database().Client())
	if err != nil {
		log.Error().Err(err).Msg("Failed to start transaction")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	// log activity

	defer ses.EndSession(ctx)
	// fetch journal Info
	journal := models.JournalWithTransactionID{}
	journalID, err := ParseJournalID(c)

	if err != nil {
		log.Error().Err(err).Msg("Failed to parse journal ID")

		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	err = j.JournalCollection.FindOne(ctx, bson.M{"_id": journalID}).Decode(&journal)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch journal entry")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	// delete two transactions - [cash, terminal by ID]
	_, err = sales.DeleteSalesTransaction(ctx, journal.Operations[len(journal.Operations)-1], j.TransactionsCollection, j.FinanceCollection)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete cash left transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	_, err = sales.DeleteSalesTransaction(ctx, journal.Operations[len(journal.Operations)-2], j.TransactionsCollection, j.FinanceCollection)
	if err != nil {
		log.Error().Err(err).Msg("Failed to delete terminal transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	_, err = j.JournalCollection.UpdateByID(ctx, journal.ID, bson.M{
		"$set": bson.M{"shift_is_closed": false, "cash_left": 0, "terminal_income": 0, "total": journal.Total - (journal.Cash_left + journal.Terminal_income), "operations": journal.Operations[:len(journal.Operations)-2]},
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to update journal entry")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	// commit the transaction
	if err := ses.CommitTransaction(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to commit transaction")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Info().Msg("Successfully reopened journal entry")
	journal_new, err := FetchJournalByID(ctx, c, true, j.JournalCollection)
	if err != nil {
		log.Error().Err(err).Msg("Failed to fetch journal entry")

		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}
	middleware.LogActivityWithCtx(c, middleware.ActivityTypeReopenJournal, map[string]string{
		"journal_id": journalID.Hex(),
	}, j.ActivitiesCollection)

	return c.Status(fiber.StatusOK).JSON(models.NewOutput(
		journal_new,
	))
}

func ParseJournalID(c *fiber.Ctx) (bson.ObjectID, error) {
	// log.Info().Msg("Parsing journal ID")
	journalID, err := bson.ObjectIDFromHex(c.Params("id"))
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse journal ID")
		return bson.NilObjectID, err
	}
	// log.Info().Msg("Successfully parsed journal ID")
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

func FetchJournalByID(ctx context.Context, c *fiber.Ctx, fetchTransactions bool, journalsCollection *mongo.Collection) (*models.Journal, error) {
	// log.Info().Msg("Fetching journal by ID")
	journalID, err := ParseJournalID(c)
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse journal ID")
		return nil, err
	}

	// Fetch transactions related to this journal using a MongoDB pipeline
	pipeline := mongo.Pipeline{}
	if fetchTransactions {
		log.Info().Str("journal_id", journalID.Hex()).Msg("Fetching transactions with lookup")
		pipeline = mongo.Pipeline{
			{{Key: "$match", Value: bson.D{
				{Key: "$or", Value: bson.A{
					bson.D{{Key: "_id", Value: journalID}},
					bson.D{{Key: "_id", Value: c.Params("id")}},
				}},
			}}},
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
	} else {
		log.Info().Str("journal_id", journalID.Hex()).Msg("Fetching journal by ID without transactions")
		pipeline = mongo.Pipeline{
			{{Key: "$match", Value: bson.D{{Key: "_id", Value: journalID}}}},
		}
	}

	// log.Info().Interface("pipeline", pipeline).Msg("Pipeline")

	cursor, err := journalsCollection.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Msg("Failed to aggregate journal entries")
		return nil, err
	}
	defer cursor.Close(ctx)

	var results []models.Journal
	if err := cursor.All(ctx, &results); err != nil {
		log.Error().Err(err).Msg("Failed to decode journal entries")
		return nil, err
	}

	log.Debug().Msg("Successfully fetched journal by ID")
	if len(results) == 0 {
		log.Error().Msg("No journal found")
		return nil, errors.New("no journal found")
	}
	return &results[0], nil
}
