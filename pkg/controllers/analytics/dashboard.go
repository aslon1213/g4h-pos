package analytics

import (
	"context"
	"time"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
	"gorm.io/gorm"
)

var branches []models.BranchFinance

type DashboardHandler struct {
	db     *gorm.DB
	tracer trace.Tracer
}

func New(db *gorm.DB) *DashboardHandler {
	tracer := otel.Tracer("DashboardHandler")
	ctx := context.Background()

	loaded, err := gorm.G[models.BranchFinance](db).Find(ctx)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get branches")
	}
	branches = loaded

	log.Info().Msgf("Fetched %d branches", len(branches))
	return &DashboardHandler{
		db:     db,
		tracer: tracer,
	}
}

// queryJournals replaces the former journal_handlers.QueryJournals mongo helper.
// It queries the journals table directly (branch + total filters, newest first,
// paged) and then attaches each journal's transactions as its Operations — the
// mongo pipeline did the same with a $lookup from the transactions collection.
// The match/sort/skip/limit shape is preserved byte-for-byte so the charts.go
// renderers receive the same data as before.
func (d *DashboardHandler) queryJournals(ctx context.Context, params models.JournalQueryParams) ([]models.Journal, error) {
	query := gorm.G[models.Journal](d.db).Order("date DESC")
	if params.BranchID != "" {
		query = query.Where("branch_id = ?", params.BranchID)
	}
	if params.Total.Use {
		query = query.Where("total >= ? AND total <= ?", params.Total.Min, params.Total.Max)
	}
	if params.Page > 1 && params.PageSize > 0 {
		query = query.Offset((params.Page - 1) * params.PageSize)
	}
	if params.PageSize > 0 {
		query = query.Limit(params.PageSize)
	}

	journals, err := query.Find(ctx)
	if err != nil {
		return nil, err
	}

	for i := range journals {
		ops, err := gorm.G[models.Transaction](d.db).Where("journal_id = ?", journals[i].ID).Find(ctx)
		if err != nil {
			return nil, err
		}
		journals[i].Operations = ops
	}
	return journals, nil
}

func NewQueryParams(from_date time.Time, to_date time.Time, page_size int, page int, branch_id string) models.JournalQueryParams {
	return models.JournalQueryParams{
		FromDate: from_date,
		ToDate:   to_date,
		BranchID: branch_id,
		Page:     page,
		PageSize: page_size,
		Total: models.TotalValueQueryParams{
			Use: true,
			Min: 0,
			Max: 30000000,
		},
	}
}

func (d *DashboardHandler) MainPage(c *fiber.Ctx) error {
	body := `
    <!DOCTYPE html>
    <html>
    <head>
        <title>Analytics Dashboards</title>
        <meta name="viewport" content="width=device-width, initial-scale=1" />
        <style>
            body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Oxygen, Ubuntu, Cantarell, 'Open Sans', 'Helvetica Neue', sans-serif; margin: 0; padding: 24px; background: #f3f4f6; }
            .container { max-width: 900px; margin: 0 auto; }
            .header { text-align: center; margin-bottom: 24px; }
            .cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(220px, 1fr)); gap: 16px; }
            .card { background: white; padding: 20px; border-radius: 12px; box-shadow: 0 8px 25px rgba(0,0,0,0.05); }
            .card h2 { margin: 0 0 8px 0; font-size: 18px; color: #111827; }
            .card p { margin: 0 0 12px 0; color: #6b7280; }
            .card a { display: inline-block; color: #fff; background: #4f46e5; padding: 10px 14px; border-radius: 8px; text-decoration: none; font-weight: 600; }
        </style>
    </head>
    <body>
        <div class="container">
            <div class="header">
                <h1>Analytics Dashboards</h1>
                <p>Select a dashboard to view analytics</p>
            </div>
            <div class="cards">
                <div class="card">
                    <h2>General Analytics</h2>
                    <p>Branch comparison and detailed operational insights.</p>
                    <a href="/dashboard/general">Open General</a>
                </div>
                <div class="card">
                    <h2>Daily Analytics</h2>
                    <p>Global performance, payment method trends, and KPIs.</p>
                    <a href="/dashboard/journals">Open Daily</a>
                </div>
            </div>
        </div>
    </body>
    </html>`
	c.Set(
		"Content-Type",
		"text/html",
	)
	return c.SendString(body)
}

func (d *DashboardHandler) ServeDashBoardGeneral(c *fiber.Ctx) error {
	// General dashboard - branch comparison, growth, operations health
	from_date := c.Query(
		"from_date",
		time.Now().AddDate(0, 0, -30).Format("2006-01-02"),
	)
	to_date := c.Query(
		"to_date",
		time.Now().Local().Format("2006-01-02"),
	)
	branch_id := c.Query("branch_id", "")

	from_date_time, err := time.Parse("2006-01-02", from_date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid from date",
		})
	}
	to_date_time, err := time.Parse("2006-01-02", to_date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid to date",
		})
	}
	length := int(to_date_time.Sub(from_date_time).Hours() / 24)
	log.Info().Msgf("Length: %d", length)

	ctx, span := d.tracer.Start(c.Context(), "DashboardHandler.ServeDashBoardGeneral")
	defer span.End()

	var allJournals []models.Journal
	if branch_id != "" {
		journals, err := d.queryJournals(ctx, NewQueryParams(from_date_time, to_date_time, length, 1, branch_id))
		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to get journal data",
			})
		}
		allJournals = journals
	} else {
		for _, branch := range branches {
			journals, err := d.queryJournals(ctx, NewQueryParams(from_date_time, to_date_time, length, 1, branch.BranchID))
			if err != nil {
				log.Error().Err(err).Msgf("Failed to get journal data for branch %s", branch.BranchName)
				continue
			}
			allJournals = append(allJournals, journals...)
		}
	}

	c.Set("Content-Type", "text/html")
	RenderGeneralDashboard(allJournals, c.Response().BodyWriter())
	return nil
}

func (d *DashboardHandler) ServeDashBoardDays(c *fiber.Ctx) error {
	// this is where the days dashboard - for showing [Global Performance Analytics, Payment Method Trends, Moving Average, Financial KPIs]
	from_date := c.Query(
		"from_date",
		time.Now().AddDate(0, 0, -30).Format("2006-01-02"),
	)
	to_date := c.Query(
		"to_date",
		time.Now().Local().Format("2006-01-02"),
	)
	branch_id := c.Query("branch_id", "") // Optional branch filter

	from_date_time, err := time.Parse("2006-01-02", from_date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid from date",
		})
	}
	to_date_time, err := time.Parse("2006-01-02", to_date)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"message": "Invalid to date",
		})
	}

	log.Info().Msgf("From date: %s", from_date)
	log.Info().Msgf("To date: %s", to_date)
	log.Info().Msgf("Branch ID filter: %s", branch_id)

	ctx, span := d.tracer.Start(c.Context(), "DashboardHandler.ServeDashBoard")
	defer span.End()

	var allJournals []models.Journal
	length := int(to_date_time.Sub(from_date_time).Hours() / 24)
	// limit := length * len(branches)
	// log.Info().Msgf("Length: %d", length)
	// log.Info().Msgf("Limit: %d", limit)
	// log.Info().Msgf("From date: %s", from_date_time)
	// log.Info().Msgf("To date: %s", to_date_time)

	log.Info().Msgf("Length: %d", length)
	if branch_id != "" {
		// Query for specific branch
		journals, err := d.queryJournals(ctx, NewQueryParams(from_date_time, to_date_time, length, 1, branch_id))

		if err != nil {
			return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
				"message": "Failed to get journal data",
			})
		}
		allJournals = journals
	} else {
		// Query for all branches

		for _, branch := range branches {
			journals, err := d.queryJournals(ctx, NewQueryParams(from_date_time, to_date_time, length, 1, branch.BranchID))

			if err != nil {
				log.Error().Err(err).Msgf("Failed to get journal data for branch %s", branch.BranchName)
				continue
			}
			allJournals = append(allJournals, journals...)
		}
	}

	log.Info().Msgf("Fetched %d journals", len(allJournals))

	c.Set(
		"Content-Type",
		"text/html",
	)
	RenderDaysDashboard(allJournals, c.Response().BodyWriter())

	return nil
}

// ServeDashBoardComparison builds multiple periods from query params and renders comparison
func (d *DashboardHandler) ServeDashBoardComparison(c *fiber.Ctx) error {
	// Query params: from1,to1, from2,to2, from3,to3, branch_id (optional)
	branchID := c.Query("branch_id", "")
	ranges := [][2]string{
		{c.Query("from1", ""), c.Query("to1", "")},
		{c.Query("from2", ""), c.Query("to2", "")},
		{c.Query("from3", ""), c.Query("to3", "")},
	}

	ctx, span := d.tracer.Start(c.Context(), "DashboardHandler.ServeDashBoardComparison")
	defer span.End()

	var periods []ComparisonPeriod
	for idx, r := range ranges {
		if r[0] == "" || r[1] == "" {
			continue
		}
		fromTime, err := time.Parse("2006-01-02", r[0])
		if err != nil {
			continue
		}
		toTime, err := time.Parse("2006-01-02", r[1])
		if err != nil {
			continue
		}

		var journals []models.Journal
		if branchID != "" {
			j, err := d.queryJournals(ctx, NewQueryParams(fromTime, toTime, 200, 1, branchID))
			if err == nil {
				journals = j
			}
		} else {
			for _, br := range branches {
				j, err := d.queryJournals(ctx, NewQueryParams(fromTime, toTime, 200, 1, br.BranchID))
				if err != nil {
					log.Error().Err(err).Msgf("Failed to get journal data for branch %s", br.BranchName)
					continue
				}
				journals = append(journals, j...)
			}
		}

		label := c.Query("label"+string(rune('1'+idx)), "")
		if label == "" {
			label = fromTime.Format("2006-01-02") + " to " + toTime.Format("2006-01-02")
		}
		periods = append(periods, ComparisonPeriod{Label: label, Journals: journals})
	}

	c.Set("Content-Type", "text/html")
	RenderComparisonDashboard(periods, c.Response().BodyWriter())
	return nil
}
