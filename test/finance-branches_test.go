package test

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/app"
	"github.com/aslon1213/g4h_pos_erp/pkg/configs"
	"github.com/aslon1213/g4h_pos_erp/pkg/utils"
	"github.com/aslon1213/g4h_pos_erp/test/client"
	"go.mongodb.org/mongo-driver/bson"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"github.com/stretchr/testify/assert"
)

// --- Helper Functions ---

func ChangeLogging() {
	if os.Getenv("DISABLE_LOGGING") == "true" {
		zerolog.SetGlobalLevel(zerolog.Disabled)
		zerolog.New(io.Discard)
	}
}

func getConfig(t *testing.T) *configs.Config {
	configs, err := configs.LoadConfig("../../")
	if err != nil {
		t.Fatal(err)
	}
	return configs
}

func getClient(t *testing.T) *client.Client {
	configs := getConfig(t)
	return client.NewClient("localhost", configs.Server.Port, "admin", "admin")
}

func getAllBranches(t *testing.T, c *client.Client) []models.BranchFinance {
	resp, output, err := c.GetAllBranches()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output.Error, "Expected no error, but got one")
	return output.Data
}

func getAllSuppliers(t *testing.T, c *client.Client) []models.Supplier {
	resp, output, err := c.GetAllSuppliers()
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output.Error, "Expected no error, but got one")
	return output.Data
}

func getFirstOpenJournal(t *testing.T, journals []models.Journal) models.Journal {
	for _, journal := range journals {
		if !journal.Shift_is_closed {
			return journal
		}
	}
	t.Fatal("No open journal found")
	return models.Journal{}
}

func getFirstClosedJournal(t *testing.T, journals []models.Journal) models.Journal {
	for _, journal := range journals {
		if journal.Shift_is_closed {
			return journal
		}
	}
	t.Fatal("No closed journal found")
	return models.Journal{}
}

func getJournals(t *testing.T, c *client.Client, branchID string, page, pageSize uint8) []models.Journal {
	resp, journals_output, err := c.QueryJournalEntries(branchID, models.JournalQueryParams{
		Page:     int(page),
		PageSize: int(pageSize),
	})
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, journals_output.Error, "Expected no error, but got one")
	return journals_output.Data
}

func getBranchByID(t *testing.T, c *client.Client, branchID string) models.BranchFinance {
	resp, output, err := c.GetBranchByID(branchID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output.Error, "Expected no error, but got one")
	return output.Data
}

func getSupplierByID(t *testing.T, c *client.Client, supplierID string) models.Supplier {
	resp, output, err := c.GetSupplierByID(supplierID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output.Error, "Expected no error, but got one")
	return output.Data
}

// --- TestMain ---

func TestMain(m *testing.M) {
	ChangeLogging()
	app := app.New()

	app.DB.Database("magazin").Collection("finance").DeleteMany(context.Background(), bson.M{})
	app.DB.Database("magazin").Collection("suppliers").DeleteMany(context.Background(), bson.M{})
	app.DB.Database("magazin").Collection("transactions").DeleteMany(context.Background(), bson.M{})
	app.DB.Database("magazin").Collection("journals").DeleteMany(context.Background(), bson.M{})

	go app.Run()
	m.Run()
}

// --- Tests ---

func TestCreateBranches(t *testing.T) {
	ChangeLogging()
	configs := getConfig(t)

	time.Sleep(1 * time.Second)

	branches := []models.NewBranchFinanceInput{
		{BranchName: "Xonobod", Details: map[string]interface{}{"location": "Location A"}},
		{BranchName: "Polevoy", Details: map[string]interface{}{"location": "Location B"}},
		{BranchName: "Branch C", Details: map[string]interface{}{"location": "Location C"}},
	}

	client := client.NewClient("localhost", configs.Server.Port, "admin", "admin")

	for _, branch := range branches {
		resp, output, err := client.CreateBranch(branch)
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status code 201, but got %d", resp.StatusCode)
		if err != nil {
			t.Fatal(err)
		}
		assert.Nil(t, output.Error, "Expected no error, but got one")
	}
}

func TestCreateBranchDuplicate(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	branch := models.NewBranchFinanceInput{
		BranchName: "Xonobod",
		Details:    map[string]interface{}{"location": "Location A"},
	}
	resp, output, err := client.CreateBranch(branch)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, resp.StatusCode, http.StatusInternalServerError, "Expected status code 400, but got %d", resp.StatusCode)
	assert.NotNil(t, output.Error, "Expected error, but got none")
}

func TestGetBranches(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	_, output, err := client.GetAllBranches()
	if err != nil {
		t.Fatal(err)
	}

	log.Info().Interface("output", output).Msg("Output")
	assert.Nil(t, output.Error, "Expected no error, but got one")
}

func TestGetBranchByID(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	branches := getAllBranches(t, client)
	if len(branches) == 0 {
		t.Fatal("No branches found")
	}

	firstBranch := branches[0]
	branchID := firstBranch.BranchID

	resp, output_, err := client.GetBranchByID(branchID)
	if err != nil {
		log.Error().Err(err).Interface("Response", output_.Data).Msg("Failed to decode response")
		t.Fatal(err)
	}
	log.Info().Interface("resp", resp).Msg("Response")
	assert.Nil(t, output_.Error, "Expected no error, but got one")
}

func TestGetBranchByName(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	branches := getAllBranches(t, client)
	branchName := branches[0].BranchName

	resp, output_, err := client.GetBranchByName(branchName)
	assert.Nil(t, err)
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_.Error, "Expected no error, but got one")
	assert.Equal(t, output_.Data.BranchName, branchName, "Expected branch name to be %s, but got %s", branchName, output_.Data.BranchName)
}

func TestCreateSupplier(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	suppliers := []models.SupplierBase{
		{Name: "Supplier One", Email: "supplier1@example.com", Phone: "1234567890", Address: "Address One", INN: "111111111", Notes: "Notes for Supplier One", Branch: "Xonobod"},
		{Name: "Supplier Two", Email: "supplier2@example.com", Phone: "2345678901", Address: "Address Two", INN: "222222222", Notes: "Notes for Supplier Two", Branch: "Xonobod"},
		{Name: "Supplier Three", Email: "supplier3@example.com", Phone: "3456789012", Address: "Address Three", INN: "333333333", Notes: "Notes for Supplier Three", Branch: "Xonobod"},
		{Name: "Supplier Four", Email: "supplier4@example.com", Phone: "4567890123", Address: "Address Four", INN: "444444444", Notes: "Notes for Supplier Four", Branch: "Polevoy"},
		{Name: "Supplier Five", Email: "supplier5@example.com", Phone: "5678901234", Address: "Address Five", INN: "555555555", Notes: "Notes for Supplier Five", Branch: "Polevoy"},
	}
	for _, newSupplier := range suppliers {
		resp, output, err := client.CreateSupplier(newSupplier)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status code 201, but got %d", resp.StatusCode)
		assert.Nil(t, output.Error, "Expected no error, but got one")
		assert.Equal(t, output.Data.Name, newSupplier.Name, "Expected supplier name to be %s, but got %s", newSupplier.Name, output.Data.Name)
	}
}

func TestGetSuppliers(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	suppliers := getAllSuppliers(t, client)
	assert.NotEmpty(t, suppliers, "Expected to find suppliers, but got none")
}

func TestGetSupplierByID(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	suppliers := getAllSuppliers(t, client)
	if len(suppliers) == 0 {
		t.Fatal("No suppliers found")
	}

	firstSupplier := suppliers[0]
	supplierID := firstSupplier.ID

	supplier := getSupplierByID(t, client, supplierID)
	assert.Equal(t, supplier.ID, supplierID, "Expected supplier ID to be %s, but got %s", supplierID, supplier.ID)
}

func TestNewSupplierTransaction(t *testing.T) {
	ChangeLogging()
	client := getClient(t)

	suppliers := getAllSuppliers(t, client)
	supplierID := ""
	Branch := ""

	for _, supplier := range suppliers {
		if supplier.Branch != "" {
			supplierID = supplier.ID
			Branch = supplier.Branch
			break
		}
	}

	transactions := []models.TransactionBase{
		{Amount: 10000000, Description: "Test Transaction 1", Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 20000000, Description: "Test Transaction 2", Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 15000000, Description: "Test Transaction 3", Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 5000000, Description: "Test Transaction 3", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash},
		{Amount: 2500000, Description: "Test Transaction 3", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 3000000, Description: "Test Transaction 3", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 4000000, Description: "Test Transaction 3", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash},
		{Amount: 1000000, Description: "Test Transaction 3", Type: models.TransactionTypeCredit, PaymentMethod: models.OnlineTransfer},
	}
	total_income := 15500000
	total_expenses := 45000000
	balance := total_income - total_expenses

	for _, transaction := range transactions {
		resp, output, err := client.NewSupplierTransaction(Branch, supplierID, transaction)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status code 201, but got %d", resp.StatusCode)
		assert.Nil(t, output.Error, "Expected no error, but got one")
	}

	branch_response := getBranchByID(t, client, Branch)
	assert.Equal(t, int32(-balance), branch_response.Finance.Debt, "Expected balance to be %f, but got %f", -balance, branch_response.Finance.Debt)
	assert.Equal(t, int32(-5500000), branch_response.Finance.Balance.Bank, "Expected balance to be %f, but got %f", 5500000, branch_response.Finance.Balance.Bank)
	assert.Equal(t, int32(-9000000), branch_response.Finance.Balance.Cash, "Expected balance to be %f, but got %f", 10000000, branch_response.Finance.Balance.Cash)
	assert.Equal(t, int32(-1000000), branch_response.Finance.Balance.MobileApps, "Expected balance to be %f, but got %f", 1000000, branch_response.Finance.Balance.MobileApps)

	supplier_response := getSupplierByID(t, client, supplierID)
	assert.Equal(t, supplier_response.ID, supplierID, "Expected supplier ID to be %s, but got %s", supplierID, supplier_response.ID)
	assert.NotEqual(t, supplier_response.FinancialData.Balance, 0, "Expected balance to be %f, but got %f", balance, supplier_response.FinancialData.Balance)
}

func TestSalesTransaction(t *testing.T) {
	client := getClient(t)

	branches := getAllBranches(t, client)
	branchID := ""
	for _, branch := range branches {
		if branch.BranchName == "Xonobod" {
			branchID = branch.BranchID
			break
		}
	}

	transactions := []models.TransactionBase{
		{Amount: 10000000, Description: "Test Transaction 1", Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 20000000, Description: "Test Transaction 2", Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 30000000, Description: "Test Transaction 3", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodBank},
		{Amount: 40000000, Description: "Test Transaction 4", Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodCash},
		{Amount: 50000000, Description: "Test Transaction 5", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash},
		{Amount: 60000000, Description: "Test Transaction 6", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash},
		{Amount: 500000, Description: "Test Transaction 7", Type: models.TransactionTypeCredit, PaymentMethod: models.OnlineMobileAppPayment},
		{Amount: 205000, Description: "Test Transaction 8", Type: models.TransactionTypeCredit, PaymentMethod: models.OnlineMobileAppPayment},
		{Amount: 1000000, Description: "Test Transaction 9", Type: models.TransactionTypeCredit, PaymentMethod: models.OnlineTransfer},
		{Amount: 1000000, Description: "Test Transaction 10", Type: models.TransactionTypeCredit, PaymentMethod: models.OnlineTransfer},
		{Amount: 1000000, Description: "Test Transaction 11", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodTerminal},
		{Amount: 1000000, Description: "Test Transaction 12", Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodTerminal},
	}

	for _, transaction := range transactions {
		resp, output, err := client.CreateSalesTransaction(branchID, transaction)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status code 201, but got %d", resp.StatusCode)
		assert.Equal(t, output.Data.Amount, transaction.Amount, "Expected amount to be %f, but got %f", transaction.Amount, output.Data.Amount)
		assert.Nil(t, output.Error, "Expected no error, but got one")
	}

	cash := -9000000
	bank := -5500000
	terminal := 0
	mobile_apps := -1000000
	for _, transaction := range transactions {
		switch transaction.PaymentMethod {
		case models.PaymentMethodCash:
			cash += int(transaction.Amount)
		case models.PaymentMethodBank:
			bank += int(transaction.Amount)
		case models.PaymentMethodTerminal:
			terminal += int(transaction.Amount)
		case models.OnlineMobileAppPayment, models.OnlineTransfer:
			mobile_apps += int(transaction.Amount)
		}
	}
	balance := models.Balance{
		Cash:       int32(cash),
		Bank:       int32(bank),
		Terminal:   int32(terminal),
		MobileApps: int32(mobile_apps),
	}
	output_branch := getBranchByID(t, client, branchID)
	assert.Equal(t, output_branch.Finance.Balance, balance, "Expected balance to be %f, but got %f", balance, output_branch.Finance.Balance)

	resp, output_transactions, err := client.GetTransactions(branchID, "", 0, 0, "", "", models.InitiatorTypeSales, time.Time{}, time.Time{}, 1, 100)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_transactions.Error, "Expected no error, but got one")
	assert.Equal(t, len(output_transactions.Data), 12, "Expected 12 transactions, but got %d", len(output_transactions.Data))
}

func TestOpenJournals(t *testing.T) {
	loc := utils.GetTimeZone()
	client := getClient(t)

	days := 10
	journals := []models.NewJournalEntryInput{}
	for i := 0; i < days; i++ {
		journals = append(journals, models.NewJournalEntryInput{
			BranchNameOrID: "Xonobod",
			Date:           time.Now().In(loc).AddDate(0, 0, -i),
		})
	}

	for _, journal := range journals {
		resp, output, err := client.OpenJournal(journal)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status code 201, but got %d", resp.StatusCode)
		assert.Nil(t, output.Error, "Expected no error, but got one")
	}

	resp, output, err := client.OpenJournal(journals[0])
	if err != nil {
		log.Error().Err(err).Msg("Failed to open journal")
		t.Fatal(err)
	}
	assert.NotEqual(t, http.StatusOK, resp.StatusCode, "Expected status code 400, but got %d", resp.StatusCode)
	assert.NotNil(t, output.Error, "Expected error, but got none")
}

func TestQueryJournal(t *testing.T) {
	client := getClient(t)
	branches := getAllBranches(t, client)
	branchID := branches[0].BranchID

	journals := getJournals(t, client, branchID, 1, 10)
	assert.Equal(t, 10, len(journals), "Expected 1 journal, but got %d", len(journals))

	resp, output_, err := client.GetJournalByID(journals[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_.Error, "Expected no error, but got one")
	assert.Equal(t, journals[0].ID, output_.Data.ID, "Expected journal ID to be %s, but got %s", journals[0].ID, output_.Data.ID)
}

func TestNewOperation(t *testing.T) {
	client := getClient(t)
	branches := getAllBranches(t, client)
	branch := branches[0]
	branchID := branch.BranchID

	journals := getJournals(t, client, branchID, 1, 10)
	assert.Equal(t, 10, len(journals), "Expected 1 journal, but got %d", len(journals))

	journalID := journals[0].ID
	operations := []models.JournalOperationInput{
		{TransactionBase: models.TransactionBase{Amount: 1000000, Description: "Test Operation 1 in journal " + journalID, Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodBank}, SupplierTransaction: false},
		{TransactionBase: models.TransactionBase{Amount: 1000000, Description: "Test Operation 2 in journal " + journalID, Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodBank}, SupplierTransaction: false},
		{TransactionBase: models.TransactionBase{Amount: 1000000, Description: "Test Operation 3 in journal " + journalID, Type: models.TransactionTypeDebit, PaymentMethod: models.PaymentMethodCash}, SupplierTransaction: false},
		{TransactionBase: models.TransactionBase{Amount: 1000000, Description: "Test Operation 4 in journal " + journalID, Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash}, SupplierTransaction: false},
		{TransactionBase: models.TransactionBase{Amount: 1000000, Description: "Test Operation 5 in journal " + journalID, Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash}, SupplierTransaction: true, SupplierID: branch.Suppliers[0]},
		{TransactionBase: models.TransactionBase{Amount: 1000000, Description: "Test Operation 6 in journal " + journalID, Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash}, SupplierTransaction: true, SupplierID: branch.Suppliers[0]},
		{TransactionBase: models.TransactionBase{Amount: 1000000, Description: "Test Operation 7 in journal " + journalID, Type: models.TransactionTypeCredit, PaymentMethod: models.PaymentMethodCash}, SupplierTransaction: true, SupplierID: branch.Suppliers[0]},
	}

	for _, operation := range operations {
		resp, output, err := client.NewJournalOperation(journalID, operation)
		if err != nil {
			t.Fatal(err)
		}
		assert.Equal(t, http.StatusCreated, resp.StatusCode, "Expected status code 201, but got %d", resp.StatusCode)
		assert.Nil(t, output.Error, "Expected no error, but got one")
	}
}

func TestOperationsGotInserted(t *testing.T) {
	client := getClient(t)
	branches := getAllBranches(t, client)
	branchID := branches[0].BranchID

	journals := getJournals(t, client, branchID, 1, 10)
	journal := journals[0]
	assert.Equal(t, journal.Branch.ID, branchID, "Expected branch ID to be %s, but got %s", branchID, journal.Branch.ID)
	assert.Equal(t, 7, len(journal.Operations), "Expected 7 operations, but got %d", len(journal.Operations))
}

func TestUpdateOperation(t *testing.T) {
	client := getClient(t)
	branches := getAllBranches(t, client)
	branchID := branches[0].BranchID

	journals := getJournals(t, client, branchID, 1, 10)
	journal_ := getFirstOpenJournal(t, journals)

	operation_1 := journal_.Operations[0]
	operation_2 := journal_.Operations[1]

	resp, output_, err := client.UpdateOperation(journal_.ID, operation_1.ID, 1000000, "")
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_.Error, "Expected no error, but got one")
	assert.Equal(t, output_.Data.Total, journal_.Total+1000000-operation_1.Amount, "Expected total to be %d, but got %d", journal_.Total+1000000-operation_1.Amount, output_.Data.Total)
	assert.Equal(t, output_.Data.Operations[0].Amount, uint32(1000000), "Expected amount to be %d, but got %d", 1000000, output_.Data.Operations[0].Amount)
	assert.Equal(t, output_.Data.Operations[0].Description, operation_1.Description, "Expected description to be %s, but got %s", operation_1.Description, output_.Data.Operations[0].Description)

	resp, output_, err = client.UpdateOperation(journal_.ID, operation_2.ID, 1000000, "Test Operation 2 - bla bla bla")
	if err != nil {
		t.Fatal(err)
	}
	log.Info().Interface("output", output_).Msg("Output")
	log.Info().Interface("journal", journal_).Msg("Journal")
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_.Error, "Expected no error, but got one")
	assert.Equal(t, output_.Data.Total, journal_.Total+2000000-operation_1.Amount-operation_2.Amount, "Expected total to be %d, but got %d", journal_.Total+1000000-operation_1.Amount-operation_2.Amount, output_.Data.Total)
	assert.Equal(t, output_.Data.Operations[1].Amount, uint32(1000000), "Expected amount to be %d, but got %d", 1000000, output_.Data.Operations[1].Amount)
	assert.Equal(t, output_.Data.Operations[1].Description, "Test Operation 2 - bla bla bla", "Expected description to be %s, but got %s", "Test Operation 2 + bla bla bla", output_.Data.Operations[1].Description)
}

func TestDeleteOperation(t *testing.T) {
	ChangeLogging()
	client := getClient(t)
	branches := getAllBranches(t, client)
	branchID := branches[0].BranchID

	journals := getJournals(t, client, branchID, 1, 10)
	journal := getFirstOpenJournal(t, journals)

	resp, output_, err := client.DeleteOperation(journal.ID, journal.Operations[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_.Error, "Expected no error, but got one")
	assert.Equal(t, output_.Data.Total, journal.Total-journal.Operations[0].Amount, "Expected total to be %d, but got %d", journal.Total-journal.Operations[0].Amount, output_.Data.Total)
	assert.Equal(t, len(output_.Data.Operations), len(journal.Operations)-1, "Expected operations to be %d, but got %d", len(journal.Operations)-1, len(output_.Data.Operations))
}

func TestCloseJournal(t *testing.T) {
	ChangeLogging()
	client := getClient(t)
	branches := getAllBranches(t, client)
	branchID := branches[0].BranchID

	journals := getJournals(t, client, branchID, 1, 10)
	journal := getFirstOpenJournal(t, journals)
	log.Info().Interface("journals", journal.ID).Msg("Journals")
	log.Info().Interface("journal chosen", journal).Msg("Journal")

	input := models.CloseJournalEntryInput{
		CashLeft:       1000000,
		TerminalIncome: 1000000,
	}

	resp, output_, err := client.CloseJournal(journal.ID, input)
	if err != nil {
		log.Error().Err(err).Msg("Failed to close journal")
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_.Error, "Expected no error, but got one")

	resp, output_2, err := client.GetJournalByID(journal.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get journal")
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_2.Error, "Expected no error, but got one")
	assert.Equal(t, output_2.Data.Shift_is_closed, true, "Expected shift to be closed, but got %t", output_2.Data.Shift_is_closed)
	assert.Equal(t, output_2.Data.Total, journal.Total+input.CashLeft+input.TerminalIncome)
	assert.Equal(t, output_2.Data.Cash_left, input.CashLeft)
	assert.Equal(t, output_2.Data.Terminal_income, input.TerminalIncome)
}

func TestReOpenJournal(t *testing.T) {
	ChangeLogging()
	client := getClient(t)
	branches := getAllBranches(t, client)
	branchID := branches[0].BranchID

	journals := getJournals(t, client, branchID, 1, 10)
	journal := getFirstClosedJournal(t, journals)
	log.Info().Interface("journal chosen", journal.ID).Msg("Journal")

	resp, output_, err := client.ReOpenJournal(journal.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to re-open journal")
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_.Error, "Expected no error, but got one")

	resp, output_2, err := client.GetJournalByID(journal.ID)
	if err != nil {
		log.Error().Err(err).Msg("Failed to get journal")
		t.Fatal(err)
	}
	assert.Equal(t, http.StatusOK, resp.StatusCode, "Expected status code 200, but got %d", resp.StatusCode)
	assert.Nil(t, output_2.Error, "Expected no error, but got one")
	assert.Equal(t, false, output_2.Data.Shift_is_closed, "Expected shift to be open, but got %t", output_2.Data.Shift_is_closed)
	assert.Equal(t, uint32(0), output_2.Data.Cash_left)
	assert.Equal(t, uint32(0), output_2.Data.Terminal_income)
}
