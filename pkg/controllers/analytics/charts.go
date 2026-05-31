package analytics

import (
	"fmt"
	"io"
	"math"
	"sort"
	"time"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/go-echarts/go-echarts/v2/charts"
	"github.com/go-echarts/go-echarts/v2/opts"
)

// Analytics structures
type BranchAnalytics struct {
	BranchID         string
	BranchName       string
	TotalRevenue     float64
	AverageDaily     float64
	TransactionCount int
	CashRatio        float64
	TerminalRatio    float64
	GrowthRate       float64
	BestDay          time.Time
	BestDayRevenue   float64
	WorstDay         time.Time
	WorstDayRevenue  float64
}

type DailyMetrics struct {
	Date           time.Time
	Revenue        float64
	CashLeft       float64
	TerminalIncome float64
	Operations     int
	CashRatio      float64
	TerminalRatio  float64
	AvgTransaction float64
}

// ComparisonPeriod represents a labeled set of journals in a period
type ComparisonPeriod struct {
	Label    string
	Journals []models.Journal
}

// RenderComparisonDashboard renders charts comparing multiple periods
func RenderComparisonDashboard(periods []ComparisonPeriod, writer io.Writer) {
	fmt.Fprint(writer, `<!DOCTYPE html>
<html>
<head>
    <title>Period Comparison Dashboard</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 20px; background: #f3f4f6; }
        .container { max-width: 1400px; margin: 0 auto; }
        .header { text-align: center; color: #111827; margin-bottom: 16px; }
        .nav { text-align: center; margin-bottom: 20px; }
        .nav a { margin: 0 8px; color: #4f46e5; text-decoration: none; font-weight: 600; }
        .dashboard-section { margin-bottom: 20px; background: white; border-radius: 12px; padding: 20px; box-shadow: 0 8px 25px rgba(0,0,0,0.05); }
        .section-title { color: #111827; font-size: 20px; margin-bottom: 12px; text-align: center; border-bottom: 2px solid #e5e7eb; padding-bottom: 10px; }
        .quick-select { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; margin: 12px 0; }
        .quick-select a, .quick-select button { background: #eef2ff; border: 1px solid #c7d2fe; color: #3730a3; padding: 8px 10px; border-radius: 8px; text-decoration: none; font-weight: 600; cursor: pointer; }
        form { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; margin-top: 12px; }
        label { font-size: 12px; color: #6b7280; }
        input { width: 100%; padding: 8px; border: 1px solid #e5e7eb; border-radius: 8px; }
        .submit { grid-column: 1 / -1; text-align: center; }
        .submit button { background: #4f46e5; color: #fff; border: none; padding: 10px 14px; border-radius: 8px; font-weight: 700; cursor: pointer; }
        .chart { border: 1px solid #e5e7eb; border-radius: 8px; padding: 20px; background: #fafafa; min-height: 400px; margin-bottom: 16px; width: 100%; }
        .chart > div { width: 100% !important; height: 400px !important; }
    </style>
    <meta name="viewport" content="width=device-width, initial-scale=1" />
</head>
<body>
  <div class="container">
    <div class="header">
      <h1>Period Comparison Dashboard</h1>
      <p>Compare trends across multiple time periods</p>
    </div>
    <div class="nav">
      <a href="/dashboard/general">General</a>
      <a href="/dashboard/journals">Daily</a>
      <a href="/dashboard/comparison">Comparison</a>
    </div>

    <div class="dashboard-section">
      <h2 class="section-title">Select Periods</h2>
      <div class="quick-select">
        <a href="?from1=2025-05-01&to1=2025-08-31&from2=2024-05-01&to2=2024-08-31">2025 May–Aug vs 2024 May–Aug</a>
        <a href="?from1=2024-05-01&to1=2024-08-31&from2=2023-05-01&to2=2023-08-31">2024 May–Aug vs 2023 May–Aug</a>
      </div>
      <form method="get">
        <div>
          <label>From 1</label>
          <input type="date" name="from1" />
        </div>
        <div>
          <label>To 1</label>
          <input type="date" name="to1" />
        </div>
        <div>
          <label>From 2</label>
          <input type="date" name="from2" />
        </div>
        <div>
          <label>To 2</label>
          <input type="date" name="to2" />
        </div>
        <div>
          <label>From 3</label>
          <input type="date" name="from3" />
        </div>
        <div>
          <label>To 3</label>
          <input type="date" name="to3" />
        </div>
        <div>
          <label>Branch ID (optional)</label>
          <input type="text" name="branch_id" placeholder="branch-uuid" />
        </div>
        <div class="submit">
          <button type="submit">Compare</button>
        </div>
      </form>
    </div>
`)

	// Build daily metrics per period
	type periodMetrics struct {
		label   string
		metrics []DailyMetrics
	}
	var all []periodMetrics
	maxLen := 0
	for _, p := range periods {
		m := calculateDailyMetrics(p.Journals)
		if len(m) > maxLen {
			maxLen = len(m)
		}
		all = append(all, periodMetrics{label: p.Label, metrics: m})
	}

	// X axis as day indices 1..maxLen
	var xIndex []string
	for i := 1; i <= maxLen; i++ {
		xIndex = append(xIndex, fmt.Sprintf("D%d", i))
	}

	// Line chart: daily revenue per period
	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: "Daily Revenue Comparison", Subtitle: "Overlay by period"}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Day Index"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Revenue ($)"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)
	line.SetXAxis(xIndex)

	for _, pm := range all {
		var series []opts.LineData
		for _, d := range pm.metrics {
			series = append(series, opts.LineData{Value: d.Revenue})
		}
		line.AddSeries(pm.label, series)
	}

	fmt.Fprint(writer, `<div class="dashboard-section">`)
	fmt.Fprint(writer, `<div class="chart">`)
	line.Render(writer)
	fmt.Fprint(writer, `</div></div>`)

	// Bar chart: total revenue per period
	barTotals := charts.NewBar()
	barTotals.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: "Total Revenue by Period"}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Period"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Revenue ($)"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)
	var periodLabels []string
	var totals []opts.BarData
	for _, pm := range all {
		sum := 0.0
		for _, d := range pm.metrics {
			sum += d.Revenue
		}
		periodLabels = append(periodLabels, pm.label)
		totals = append(totals, opts.BarData{Value: sum})
	}
	barTotals.SetXAxis(periodLabels)
	barTotals.AddSeries("Total Revenue", totals)

	fmt.Fprint(writer, `<div class="dashboard-section">`)
	fmt.Fprint(writer, `<div class="chart">`)
	barTotals.Render(writer)
	fmt.Fprint(writer, `</div></div>`)

	// Bar chart: average cash vs terminal ratio by period
	barRatios := charts.NewBar()
	barRatios.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: "Average Payment Method Ratios"}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Period"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Ratio (%)"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "10%"}),
	)
	var cashSeries []opts.BarData
	var terminalSeries []opts.BarData
	for _, pm := range all {
		cashSum := 0.0
		termSum := 0.0
		n := float64(len(pm.metrics))
		if n == 0 {
			n = 1
		}
		for _, d := range pm.metrics {
			cashSum += d.CashRatio
			termSum += d.TerminalRatio
		}
		cashSeries = append(cashSeries, opts.BarData{Value: (cashSum / n) * 100})
		terminalSeries = append(terminalSeries, opts.BarData{Value: (termSum / n) * 100})
	}
	barRatios.SetXAxis(periodLabels)
	barRatios.AddSeries("Cash Ratio", cashSeries)
	barRatios.AddSeries("Terminal Ratio", terminalSeries)

	fmt.Fprint(writer, `<div class="dashboard-section">`)
	fmt.Fprint(writer, `<div class="chart">`)
	barRatios.Render(writer)
	fmt.Fprint(writer, `</div></div>`)

	fmt.Fprint(writer, `</div></body></html>`)
}

// RenderGeneralDashboard renders the branch-focused/general analytics page
func RenderGeneralDashboard(data []models.Journal, writer io.Writer) {
	// Map branch IDs to names from pre-fetched branches
	branchIDToName := make(map[string]string)
	for _, branch := range branches {
		branchIDToName[branch.BranchID] = branch.BranchName
	}

	// Group journals by branch ID
	branchData := make(map[string][]models.Journal)
	for _, journal := range data {
		branchID := journal.Branch.ID
		if branchID == "" {
			branchID = "unknown"
		}
		branchData[branchID] = append(branchData[branchID], journal)
	}

	// Calculate analytics for all branches
	branchAnalytics := calculateBranchAnalytics(branchData, branchIDToName)

	// HTML shell
	fmt.Fprint(writer, `<!DOCTYPE html>
<html>
<head>
    <title>General Analytics Dashboard</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 20px; background: #f3f4f6; }
        .container { max-width: 1400px; margin: 0 auto; }
        .header { text-align: center; color: #111827; margin-bottom: 24px; }
        .nav { text-align: center; margin-bottom: 24px; }
        .nav a { margin: 0 8px; color: #4f46e5; text-decoration: none; font-weight: 600; }
        .overview-cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(250px, 1fr)); gap: 20px; margin-bottom: 24px; }
        .card { background: white; padding: 20px; border-radius: 12px; box-shadow: 0 8px 25px rgba(0,0,0,0.05); text-align: center; }
        .card h3 { color: #374151; margin-bottom: 8px; font-size: 14px; text-transform: uppercase; letter-spacing: 1px; }
        .card .value { font-size: 28px; font-weight: bold; color: #4f46e5; }
        .dashboard-section { margin-bottom: 24px; background: white; border-radius: 12px; padding: 24px; box-shadow: 0 8px 25px rgba(0,0,0,0.05); }
        .section-title { color: #111827; font-size: 20px; margin-bottom: 16px; text-align: center; border-bottom: 2px solid #e5e7eb; padding-bottom: 12px; }
        .chart { border: 1px solid #e5e7eb; border-radius: 8px; padding: 20px; background: #fafafa; min-height: 400px; margin-bottom: 16px; width: 100%; }
        .chart > div { width: 100% !important; height: 400px !important; }
        .branch-section { margin-bottom: 24px; border: 1px solid #e5e7eb; border-radius: 12px; padding: 20px; background: white; }
        .branch-title { color: #111827; font-size: 18px; margin-bottom: 12px; text-align: center; }
        .kpi-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; margin-bottom: 12px; }
        .kpi-card { background: #f9fafb; padding: 12px; border-radius: 8px; text-align: center; border-left: 4px solid #4f46e5; }
        .kpi-card .label { font-size: 12px; color: #6b7280; text-transform: uppercase; margin-bottom: 4px; }
        .kpi-card .value { font-size: 18px; font-weight: bold; color: #111827; }
    </style>
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta http-equiv="Cache-Control" content="no-store" />
    <meta http-equiv="Pragma" content="no-cache" />
    <meta http-equiv="Expires" content="0" />
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>General Analytics Dashboard</h1>
            <p>Branch comparison and detailed operational insights</p>
        </div>
        <div class="nav">
            <a href="/dashboard/general">General</a>
            <a href="/dashboard/journals">Daily</a>
        </div>
`)

	// Overview
	createOverviewCards(data, branchAnalytics, writer)

	// Branch comparison section
	fmt.Fprint(writer, `<div class="dashboard-section">
        <h2 class="section-title">Branch Performance Comparison</h2>`)
	createBranchComparison(branchAnalytics, writer)
	fmt.Fprint(writer, `</div>`)

	// Individual branch analytics
	for branchID, journals := range branchData {
		branchName := branchIDToName[branchID]
		if branchName == "" {
			if len(journals) > 0 && journals[0].Branch.Name != "" {
				branchName = journals[0].Branch.Name
			} else {
				branchName = fmt.Sprintf("Branch %s", branchID)
			}
		}

		fmt.Fprintf(writer, `<div class="branch-section">
            <h3 class="branch-title">%s - Detailed Analytics</h3>`, branchName)

		createBranchDetailedAnalytics(journals, branchName, writer)

		fmt.Fprint(writer, `</div>`)
	}

	fmt.Fprint(writer, `</div></body></html>`)
}

// RenderDaysDashboard renders the daily/time-series focused analytics page
func RenderDaysDashboard(data []models.Journal, writer io.Writer) {
	fmt.Fprint(writer, `<!DOCTYPE html>
<html>
<head>
    <title>Daily Analytics Dashboard</title>
    <style>
        body { font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; margin: 0; padding: 20px; background: #f3f4f6; }
        .container { max-width: 1400px; margin: 0 auto; }
        .header { text-align: center; color: #111827; margin-bottom: 16px; }
        .nav { text-align: center; margin-bottom: 20px; }
        .nav a { margin: 0 8px; color: #4f46e5; text-decoration: none; font-weight: 600; }
        .dashboard-section { margin-bottom: 20px; background: white; border-radius: 12px; padding: 20px; box-shadow: 0 8px 25px rgba(0,0,0,0.05); }
        .section-title { color: #111827; font-size: 20px; margin-bottom: 12px; text-align: center; border-bottom: 2px solid #e5e7eb; padding-bottom: 10px; }
        .quick-select { display: flex; flex-wrap: wrap; gap: 8px; justify-content: center; margin: 12px 0; }
        .quick-select a, .quick-select button { background: #eef2ff; border: 1px solid #c7d2fe; color: #3730a3; padding: 8px 10px; border-radius: 8px; text-decoration: none; font-weight: 600; cursor: pointer; }
        form { display: grid; grid-template-columns: repeat(auto-fit, minmax(200px, 1fr)); gap: 12px; margin-top: 12px; }
        label { font-size: 12px; color: #6b7280; }
        input { width: 100%; padding: 8px; border: 1px solid #e5e7eb; border-radius: 8px; }
        .submit { grid-column: 1 / -1; text-align: center; }
        .submit button { background: #4f46e5; color: #fff; border: none; padding: 10px 14px; border-radius: 8px; font-weight: 700; cursor: pointer; }
        .chart { border: 1px solid #e5e7eb; border-radius: 8px; padding: 20px; background: #fafafa; min-height: 400px; margin-bottom: 16px; width: 100%; }
        .chart > div { width: 100% !important; height: 400px !important; }
    </style>
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <meta http-equiv="Cache-Control" content="no-store" />
    <meta http-equiv="Pragma" content="no-cache" />
    <meta http-equiv="Expires" content="0" />
</head>
<body>


<div class="header">
            <h1>Daily Analytics Dashboard</h1>
            <p>Global performance, payment trends and KPIs</p>
        </div>
        <div class="nav">
            <a href="/dashboard/general">General</a>
            <a href="/dashboard/journals">Daily</a>
            <a href="/dashboard/comparison">Comparison</a>
        </div>

        <div class="dashboard-section">
            <h2 class="section-title">Select Date Range</h2>
            <form method="get">
                <div>
                    <label>From Date</label>
                    <input type="date" name="from_date" />
                </div>
                <div>
                    <label>To Date</label>
                    <input type="date" name="to_date" />
                </div>
                <div class="submit">
                    <button type="submit">Update Dashboard</button>
                </div>
            </form>
        </div>
    <div class="container">
        <div class="dashboard-section">
            <h2 class="section-title">Global Performance Analytics</h2>`)

	createGlobalAnalytics(data, writer)
	// Weekday averages section
	fmt.Fprint(writer, `<div class="dashboard-section">`)
	fmt.Fprint(writer, `<h2 class="section-title">Weekday Averages</h2>`)
	createWeekdayAveragesCharts(data, writer)
	fmt.Fprint(writer, `</div></div></body></html>`)

	fmt.Fprint(writer, `</div>`)

}

func Dashboard(data []models.Journal, writer io.Writer) {
	// Create a map from branch ID to branch name
	branchIDToName := make(map[string]string)
	for _, branch := range branches {
		branchIDToName[branch.BranchID] = branch.BranchName
	}

	// Group journals by branch ID
	branchData := make(map[string][]models.Journal)
	for _, journal := range data {
		branchID := journal.Branch.ID
		if branchID == "" {
			branchID = "unknown"
		}
		branchData[branchID] = append(branchData[branchID], journal)
	}

	// Calculate analytics for all branches
	branchAnalytics := calculateBranchAnalytics(branchData, branchIDToName)

	// Write HTML header
	fmt.Fprint(writer, `<!DOCTYPE html>
<html>
<head>
    <title>Advanced Financial Analytics Dashboard</title>
    <style>
        body { 
            font-family: 'Segoe UI', Tahoma, Geneva, Verdana, sans-serif; 
            margin: 0; 
            padding: 20px;
            background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
            min-height: 100vh;
        }
        .container {
            max-width: 1400px;
            margin: 0 auto;
        }
        .header {
            text-align: center;
            color: white;
            margin-bottom: 40px;
        }
        .header h1 {
            font-size: 36px;
            margin-bottom: 10px;
            text-shadow: 2px 2px 4px rgba(0,0,0,0.3);
        }
        .overview-cards {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(250px, 1fr));
            gap: 20px;
            margin-bottom: 40px;
        }
        .card {
            background: white;
            padding: 20px;
            border-radius: 12px;
            box-shadow: 0 8px 25px rgba(0,0,0,0.1);
            text-align: center;
        }
        .card h3 {
            color: #333;
            margin-bottom: 10px;
            font-size: 14px;
            text-transform: uppercase;
            letter-spacing: 1px;
        }
        .card .value {
            font-size: 28px;
            font-weight: bold;
            color: #667eea;
            margin-bottom: 5px;
        }
        .dashboard-section {
            margin-bottom: 50px;
            background: white;
            border-radius: 12px;
            padding: 30px;
            box-shadow: 0 8px 25px rgba(0,0,0,0.1);
        }
        .section-title {
            color: #333;
            font-size: 24px;
            margin-bottom: 30px;
            text-align: center;
            border-bottom: 3px solid #667eea;
            padding-bottom: 15px;
        }
        .charts-vertical {
            display: block;
            margin-bottom: 30px;
        }
        .chart {
            border: 1px solid #e0e0e0;
            border-radius: 8px;
            padding: 20px;
            background: #fafafa;
            min-height: 400px;
            margin-bottom: 30px;
            width: 100%;
        }
        .chart > div {
            width: 100% !important;
            height: 400px !important;
        }
        .branch-section {
            margin-bottom: 40px;
            border: 1px solid #ddd;
            border-radius: 12px;
            padding: 25px;
            background: white;
        }
        .branch-title {
            color: #333;
            font-size: 20px;
            margin-bottom: 20px;
            text-align: center;
        }
        .kpi-grid {
            display: grid;
            grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
            gap: 15px;
            margin-bottom: 25px;
        }
        .kpi-card {
            background: #f8f9fa;
            padding: 15px;
            border-radius: 8px;
            text-align: center;
            border-left: 4px solid #667eea;
        }
        .kpi-card .label {
            font-size: 12px;
            color: #666;
            text-transform: uppercase;
            margin-bottom: 5px;
        }
        .kpi-card .value {
            font-size: 18px;
            font-weight: bold;
            color: #333;
        }
    </style>
</head>
<body>
    <div class="container">
        <div class="header">
            <h1>Advanced Financial Analytics Dashboard</h1>
            <p>Comprehensive business intelligence and performance metrics</p>
        </div>
`)

	// Overview cards
	createOverviewCards(data, branchAnalytics, writer)

	// Global analytics section
	fmt.Fprintf(writer, `<div class="dashboard-section">
        <h2 class="section-title">Global Performance Analytics</h2>`)

	createGlobalAnalytics(data, writer)

	fmt.Fprint(writer, `</div>`)

	// Branch comparison section
	fmt.Fprintf(writer, `<div class="dashboard-section">
        <h2 class="section-title">Branch Performance Comparison</h2>`)

	createBranchComparison(branchAnalytics, writer)

	fmt.Fprintf(writer, `</div>`)

	// Individual branch analytics
	for branchID, journals := range branchData {
		branchName := branchIDToName[branchID]
		if branchName == "" {
			if len(journals) > 0 && journals[0].Branch.Name != "" {
				branchName = journals[0].Branch.Name
			} else {
				branchName = fmt.Sprintf("Branch %s", branchID)
			}
		}

		fmt.Fprintf(writer, `<div class="branch-section">
            <h3 class="branch-title">%s - Detailed Analytics</h3>`, branchName)

		createBranchDetailedAnalytics(journals, branchName, writer)

		fmt.Fprint(writer, `</div>`)
	}

	fmt.Fprint(writer, `</div></body></html>`)
}

func createOverviewCards(data []models.Journal, analytics []BranchAnalytics, writer io.Writer) {
	totalRevenue := 0.0
	totalTransactions := 0
	avgCashRatio := 0.0
	totalBranches := len(analytics)

	for _, branch := range analytics {
		totalRevenue += branch.TotalRevenue
		totalTransactions += branch.TransactionCount
		avgCashRatio += branch.CashRatio
	}

	if totalBranches > 0 {
		avgCashRatio /= float64(totalBranches)
	}

	fmt.Fprintf(writer, `<div class="overview-cards">
        <div class="card">
            <h3>Total Revenue</h3>
            <div class="value">$%.2f</div>
        </div>
        <div class="card">
            <h3>Total Transactions</h3>
            <div class="value">%d</div>
        </div>
        <div class="card">
            <h3>Active Branches</h3>
            <div class="value">%d</div>
        </div>
        <div class="card">
            <h3>Avg Cash Ratio</h3>
            <div class="value">%.1f%%</div>
        </div>
    </div>`, totalRevenue, totalTransactions, totalBranches, avgCashRatio*100)
}

func createGlobalAnalytics(data []models.Journal, writer io.Writer) {
	// 1. Daily revenue trend
	createDailyRevenueChart(data, writer)

	// 2. Payment method trends
	createPaymentMethodTrendsChart(data, writer)

	// 3. 7-day moving average
	createMovingAverageChart(data, writer)

	// 4. Financial KPIs over time
	createFinancialKPIsChart(data, writer)
}

// Weekday averages computation and charts
type WeekdayAverages struct {
	Name          string
	AvgRevenue    float64
	AvgCash       float64
	AvgTerminal   float64
	AvgOperations float64
}

func calculateWeekdayAverages(data []models.Journal) []WeekdayAverages {
	daily := calculateDailyMetrics(data)
	// Index by time.Weekday (0=Sunday ... 6=Saturday)
	var revenueSum [7]float64
	var cashSum [7]float64
	var terminalSum [7]float64
	var opsSum [7]float64
	var count [7]int

	for _, d := range daily {
		w := int(d.Date.Weekday())
		revenueSum[w] += d.Revenue
		cashSum[w] += d.CashLeft
		terminalSum[w] += d.TerminalIncome
		opsSum[w] += float64(d.Operations)
		count[w]++
	}

	order := []time.Weekday{time.Monday, time.Tuesday, time.Wednesday, time.Thursday, time.Friday, time.Saturday, time.Sunday}
	names := []string{"Mon", "Tue", "Wed", "Thu", "Fri", "Sat", "Sun"}

	var result []WeekdayAverages
	for i, wd := range order {
		idx := int(wd)
		c := count[idx]
		if c == 0 {
			c = 1
		}
		result = append(result, WeekdayAverages{
			Name:          names[i],
			AvgRevenue:    revenueSum[idx] / float64(c),
			AvgCash:       cashSum[idx] / float64(c),
			AvgTerminal:   terminalSum[idx] / float64(c),
			AvgOperations: opsSum[idx] / float64(c),
		})
	}
	// round the result
	for i, r := range result {
		result[i].AvgRevenue = math.Round(r.AvgRevenue)
		result[i].AvgCash = math.Round(r.AvgCash)
		result[i].AvgTerminal = math.Round(r.AvgTerminal)
		result[i].AvgOperations = math.Round(r.AvgOperations)
	}
	return result
}
func createWeekdayAveragesCharts(data []models.Journal, writer io.Writer) {
	avgs := calculateWeekdayAverages(data)

	var days []string
	var avgRevenue []opts.BarData
	var avgCash []opts.BarData
	var avgTerminal []opts.BarData
	var avgOps []opts.BarData

	for _, a := range avgs {
		days = append(days, a.Name)
		avgRevenue = append(avgRevenue, opts.BarData{Value: a.AvgRevenue, Label: &opts.Label{Show: opts.Bool(true), Position: "inside"}})
		avgCash = append(avgCash, opts.BarData{Value: a.AvgCash, Label: &opts.Label{Show: opts.Bool(true), Position: "inside"}})
		avgTerminal = append(avgTerminal, opts.BarData{Value: a.AvgTerminal, Label: &opts.Label{Show: opts.Bool(true), Position: "inside"}})
		avgOps = append(avgOps, opts.BarData{Value: a.AvgOperations, Label: &opts.Label{Show: opts.Bool(true), Position: "inside"}})
	}

	// Amounts chart (Total, Cash, Terminal)
	amounts := charts.NewBar()
	amounts.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: "Average by Weekday (Amounts)"}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Weekday"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Amount ($)", Min: 9000000}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Bottom: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)
	amounts.SetXAxis(days)
	amounts.AddSeries("Avg Total", avgRevenue)
	amounts.AddSeries("Avg Cash", avgCash)
	amounts.AddSeries("Avg Terminal", avgTerminal)

	fmt.Fprint(writer, `<div class="chart">`)
	amounts.Render(writer)
	fmt.Fprint(writer, `</div>`)

	// Operations chart
	ops := charts.NewBar()
	ops.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{Title: "Average Operations by Weekday"}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Weekday"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Operations"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Bottom: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)
	ops.SetXAxis(days)
	ops.AddSeries("Avg Operations", avgOps)

	fmt.Fprint(writer, `<div class="chart">`)
	ops.Render(writer)
	fmt.Fprint(writer, `</div>`)
}

func createDailyRevenueChart(data []models.Journal, writer io.Writer) {
	dailyMetrics := calculateDailyMetrics(data)

	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Daily Revenue Trend",
			Subtitle: "Revenue performance over time",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Date"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Revenue ($)"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var dates []string
	var revenues []opts.LineData
	for _, metric := range dailyMetrics {
		dates = append(dates, metric.Date.Format("01-02"))
		revenues = append(revenues, opts.LineData{Value: metric.Revenue})
	}

	line.SetXAxis(dates)
	line.AddSeries("Daily Revenue", revenues)

	fmt.Fprint(writer, `<div class="chart">`)
	line.Render(writer)
	fmt.Fprint(writer, `</div>`)
}

func createPaymentMethodTrendsChart(data []models.Journal, writer io.Writer) {
	dailyMetrics := calculateDailyMetrics(data)

	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Payment Method Trends",
			Subtitle: "Terminal vs Cash ratio over time",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Date"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Ratio (%)"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var dates []string
	var cashRatios []opts.LineData
	var terminalRatios []opts.LineData

	for _, metric := range dailyMetrics {
		dates = append(dates, metric.Date.Format("01-02"))
		cashRatios = append(cashRatios, opts.LineData{Value: metric.CashRatio * 100})
		terminalRatios = append(terminalRatios, opts.LineData{Value: metric.TerminalRatio * 100})
	}

	line.SetXAxis(dates)
	line.AddSeries("Cash Ratio", cashRatios)
	line.AddSeries("Terminal Ratio", terminalRatios)

	fmt.Fprint(writer, `<div class="chart">`)
	line.Render(writer)
	fmt.Fprint(writer, `</div>`)
}
func createMovingAverageChart(data []models.Journal, writer io.Writer) {
	dailyMetrics := calculateDailyMetrics(data)

	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title: "N-Day Moving Average",

			Subtitle: "Smoothed revenue trends",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Date"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Revenue ($)"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var dates []string
	var actualRevenues []opts.LineData
	var movingAverages7Day []opts.LineData
	var movingAverages3Day []opts.LineData

	for i, metric := range dailyMetrics {
		dates = append(dates, metric.Date.Format("01-02"))
		actualRevenues = append(actualRevenues, opts.LineData{Value: metric.Revenue})

		// Calculate 7-day moving average
		start7 := int(math.Max(0, float64(i-6)))
		sum7 := 0.0
		count7 := 0
		for j := start7; j <= i; j++ {
			sum7 += dailyMetrics[j].Revenue
			count7++
		}
		avg7 := sum7 / float64(count7)
		movingAverages7Day = append(movingAverages7Day, opts.LineData{Value: avg7})

		// Calculate 3-day moving average
		start3 := int(math.Max(0, float64(i-2)))
		sum3 := 0.0
		count3 := 0
		for j := start3; j <= i; j++ {
			sum3 += dailyMetrics[j].Revenue
			count3++
		}
		avg3 := sum3 / float64(count3)
		movingAverages3Day = append(movingAverages3Day, opts.LineData{Value: avg3})
	}

	line.SetXAxis(dates)
	line.AddSeries("Actual Revenue", actualRevenues)
	line.AddSeries("7-Day Moving Average", movingAverages7Day)
	line.AddSeries("3-Day Moving Average", movingAverages3Day)

	fmt.Fprint(writer, `<div class="chart">`)
	line.Render(writer)
	fmt.Fprint(writer, `</div>`)
}
func create3DayMovingAverageChart(data []models.Journal, writer io.Writer) {
	dailyMetrics := calculateDailyMetrics(data)

	line := charts.NewLine()
	line.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "3-Day Moving Average",
			Subtitle: "Smoothed revenue trends",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Date"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Revenue ($)"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var dates []string
	var actualRevenues []opts.LineData
	var movingAverages []opts.LineData

	for i, metric := range dailyMetrics {
		dates = append(dates, metric.Date.Format("01-02"))
		actualRevenues = append(actualRevenues, opts.LineData{Value: metric.Revenue})

		// Calculate 3-day moving average
		start := int(math.Max(0, float64(i-2)))
		sum := 0.0
		count := 0
		for j := start; j <= i; j++ {
			sum += dailyMetrics[j].Revenue
			count++
		}
		avg := sum / float64(count)
		movingAverages = append(movingAverages, opts.LineData{Value: avg})
	}

	line.SetXAxis(dates)
	line.AddSeries("Actual Revenue", actualRevenues)
	line.AddSeries("3-Day Moving Average", movingAverages)

	fmt.Fprint(writer, `<div class="chart">`)
	line.Render(writer)
	fmt.Fprint(writer, `</div>`)
}

func createFinancialKPIsChart(data []models.Journal, writer io.Writer) {
	dailyMetrics := calculateDailyMetrics(data)

	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Financial KPIs",
			Subtitle: "Average transaction size and operational metrics",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Date"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Value"}),
		charts.WithLegendOpts(opts.Legend{Show: opts.Bool(true), Top: "10%"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var dates []string
	var avgTransactions []opts.BarData
	var operationCounts []opts.BarData

	for _, metric := range dailyMetrics {
		dates = append(dates, metric.Date.Format("01-02"))
		avgTransactions = append(avgTransactions, opts.BarData{Value: metric.AvgTransaction})
		operationCounts = append(operationCounts, opts.BarData{Value: metric.Operations})
	}

	bar.SetXAxis(dates)
	bar.AddSeries("Avg Transaction Size", avgTransactions)
	bar.AddSeries("Operations Count", operationCounts)

	fmt.Fprint(writer, `<div class="chart">`)
	bar.Render(writer)
	fmt.Fprint(writer, `</div>`)
}

func createBranchComparison(analytics []BranchAnalytics, writer io.Writer) {
	// Sort branches by total revenue
	sort.Slice(analytics, func(i, j int) bool {
		return analytics[i].TotalRevenue > analytics[j].TotalRevenue
	})

	// Branch ranking by revenue
	createBranchRevenueRankingChart(analytics, writer)

	// Growth rate comparison
	createBranchGrowthRateChart(analytics, writer)
}

func createBranchRevenueRankingChart(analytics []BranchAnalytics, writer io.Writer) {
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Branch Revenue Ranking",
			Subtitle: "Total revenue comparison across branches",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Branch"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Revenue ($)"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var branchNames []string
	var revenues []opts.BarData
	for _, branch := range analytics {
		branchNames = append(branchNames, branch.BranchName)
		revenues = append(revenues, opts.BarData{Value: branch.TotalRevenue})
	}

	bar.SetXAxis(branchNames)
	bar.AddSeries("Total Revenue", revenues)

	fmt.Fprint(writer, `<div class="chart">`)
	bar.Render(writer)
	fmt.Fprint(writer, `</div>`)
}

func createBranchGrowthRateChart(analytics []BranchAnalytics, writer io.Writer) {
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Branch Growth Rates",
			Subtitle: "Performance growth comparison",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Branch"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Growth Rate (%)"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var branchNames []string
	var growthRates []opts.BarData
	for _, branch := range analytics {
		branchNames = append(branchNames, branch.BranchName)
		growthRates = append(growthRates, opts.BarData{Value: branch.GrowthRate * 100})
	}

	bar.SetXAxis(branchNames)
	bar.AddSeries("Growth Rate", growthRates)

	fmt.Fprint(writer, `<div class="chart">`)
	bar.Render(writer)
	fmt.Fprint(writer, `</div>`)
}

func createBranchDetailedAnalytics(data []models.Journal, branchName string, writer io.Writer) {
	if len(data) == 0 {
		fmt.Fprintf(writer, `<p>No data available for %s</p>`, branchName)
		return
	}

	// Calculate KPIs for this branch
	analytics := calculateSingleBranchAnalytics(data, branchName)

	// KPI cards
	fmt.Fprintf(writer, `<div class="kpi-grid">
        <div class="kpi-card">
            <div class="label">Total Revenue</div>
            <div class="value">$%.2f</div>
        </div>
        <div class="kpi-card">
            <div class="label">Daily Average</div>
            <div class="value">$%.2f</div>
        </div>
        <div class="kpi-card">
            <div class="label">Cash Ratio</div>
            <div class="value">%.1f%%</div>
        </div>
        <div class="kpi-card">
            <div class="label">Terminal Ratio</div>
            <div class="value">%.1f%%</div>
        </div>
        <div class="kpi-card">
            <div class="label">Best Day</div>
            <div class="value">%s</div>
        </div>
        <div class="kpi-card">
            <div class="label">Best Revenue</div>
            <div class="value">$%.2f</div>
        </div>
    </div>`,
		analytics.TotalRevenue,
		analytics.AverageDaily,
		analytics.CashRatio*100,
		analytics.TerminalRatio*100,
		analytics.BestDay.Format("01-02"),
		analytics.BestDayRevenue)

	// Operational health chart
	// createOperationalHealthChart(data, writer)
}

func createOperationalHealthChart(data []models.Journal, writer io.Writer) {
	bar := charts.NewBar()
	bar.SetGlobalOptions(
		charts.WithTitleOpts(opts.Title{
			Title:    "Operational Health",
			Subtitle: "Daily operations and shift closure status",
		}),
		charts.WithXAxisOpts(opts.XAxis{Name: "Date"}),
		charts.WithYAxisOpts(opts.YAxis{Name: "Count"}),
		charts.WithInitializationOpts(opts.Initialization{Width: "100%", Height: "400px"}),
	)

	var dates []string
	var operations []opts.BarData
	var closedShifts []opts.BarData

	for _, journal := range data {
		dates = append(dates, journal.Date.Format("01-02"))
		operations = append(operations, opts.BarData{Value: len(journal.Operations)})
		if journal.Shift_is_closed {
			closedShifts = append(closedShifts, opts.BarData{Value: 1})
		} else {
			closedShifts = append(closedShifts, opts.BarData{Value: 0})
		}
	}

	bar.SetXAxis(dates)
	bar.AddSeries("Operations Count", operations)
	bar.AddSeries("Shift Closed", closedShifts)

	fmt.Fprintf(writer, `<div class="chart">`)
	bar.Render(writer)
	fmt.Fprintf(writer, `</div>`)
}

func calculateBranchAnalytics(branchData map[string][]models.Journal, branchIDToName map[string]string) []BranchAnalytics {
	var analytics []BranchAnalytics

	for branchID, journals := range branchData {
		branchName := branchIDToName[branchID]
		if branchName == "" && len(journals) > 0 {
			branchName = journals[0].Branch.Name
		}
		if branchName == "" {
			branchName = fmt.Sprintf("Branch %s", branchID)
		}

		branchAnalytic := calculateSingleBranchAnalytics(journals, branchName)
		branchAnalytic.BranchID = branchID
		analytics = append(analytics, branchAnalytic)
	}

	return analytics
}

func calculateSingleBranchAnalytics(journals []models.Journal, branchName string) BranchAnalytics {
	if len(journals) == 0 {
		return BranchAnalytics{BranchName: branchName}
	}

	totalRevenue := 0.0
	totalCash := 0.0
	totalTerminal := 0.0
	totalOperations := 0
	bestRevenue := 0.0
	worstRevenue := math.MaxFloat64
	var bestDay, worstDay time.Time

	for _, journal := range journals {
		revenue := float64(journal.Total)
		totalRevenue += revenue
		totalCash += float64(journal.Cash_left)
		totalTerminal += float64(journal.Terminal_income)
		totalOperations += len(journal.Operations)

		if revenue > bestRevenue {
			bestRevenue = revenue
			bestDay = journal.Date
		}
		if revenue < worstRevenue {
			worstRevenue = revenue
			worstDay = journal.Date
		}
	}

	averageDaily := totalRevenue / float64(len(journals))
	cashRatio := 0.0
	terminalRatio := 0.0

	if totalRevenue > 0 {
		cashRatio = totalCash / totalRevenue
		terminalRatio = totalTerminal / totalRevenue
	}

	// Simple growth rate calculation (last half vs first half)
	growthRate := 0.0
	if len(journals) > 1 {
		mid := len(journals) / 2
		firstHalfAvg := 0.0
		secondHalfAvg := 0.0

		for i := 0; i < mid; i++ {
			firstHalfAvg += float64(journals[i].Total)
		}
		firstHalfAvg /= float64(mid)

		for i := mid; i < len(journals); i++ {
			secondHalfAvg += float64(journals[i].Total)
		}
		secondHalfAvg /= float64(len(journals) - mid)

		if firstHalfAvg > 0 {
			growthRate = (secondHalfAvg - firstHalfAvg) / firstHalfAvg
		}
	}

	return BranchAnalytics{
		BranchName:       branchName,
		TotalRevenue:     totalRevenue,
		AverageDaily:     averageDaily,
		TransactionCount: totalOperations,
		CashRatio:        cashRatio,
		TerminalRatio:    terminalRatio,
		GrowthRate:       growthRate,
		BestDay:          bestDay,
		BestDayRevenue:   bestRevenue,
		WorstDay:         worstDay,
		WorstDayRevenue:  worstRevenue,
	}
}

func calculateDailyMetrics(data []models.Journal) []DailyMetrics {
	// Group by date
	dailyData := make(map[string][]models.Journal)
	for _, journal := range data {
		dateKey := journal.Date.Format("2006-01-02")
		dailyData[dateKey] = append(dailyData[dateKey], journal)
	}

	var metrics []DailyMetrics
	for dateStr, journals := range dailyData {
		date, _ := time.Parse("2006-01-02", dateStr)

		totalRevenue := 0.0
		totalCash := 0.0
		totalTerminal := 0.0
		totalOperations := 0

		for _, journal := range journals {
			totalRevenue += float64(journal.Total)
			totalCash += float64(journal.Cash_left)
			totalTerminal += float64(journal.Terminal_income)
			totalOperations += len(journal.Operations)
		}

		cashRatio := 0.0
		terminalRatio := 0.0
		avgTransaction := 0.0

		if totalRevenue > 0 {
			cashRatio = totalCash / totalRevenue
			terminalRatio = totalTerminal / totalRevenue
		}

		if totalOperations > 0 {
			avgTransaction = totalRevenue / float64(totalOperations)
		}

		metrics = append(metrics, DailyMetrics{
			Date:           date,
			Revenue:        totalRevenue,
			CashLeft:       totalCash,
			TerminalIncome: totalTerminal,
			Operations:     totalOperations,
			CashRatio:      cashRatio,
			TerminalRatio:  terminalRatio,
			AvgTransaction: avgTransaction,
		})
	}

	// Sort by date
	sort.Slice(metrics, func(i, j int) bool {
		return metrics[i].Date.Before(metrics[j].Date)
	})

	return metrics
}
