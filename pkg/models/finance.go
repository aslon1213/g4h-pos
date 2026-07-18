package models

import (
	"fmt"
	"slices"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/utils"

	"github.com/google/uuid"
)

type TransactionType string
type InitiatorType string

const (
	TransactionTypeCredit TransactionType = "credit" // credit means - income - when money is gained or received into an account
	TransactionTypeDebit  TransactionType = "debit"  // debit means - outcome - when money is lost, spent, or withdrawn from an account
)

type TransactionOutputSingle struct {
	Data  Transaction `json:"data" bson:"data"`
	Error []Error     `json:"error" bson:"error"`
}

type TransactionOutput struct {
	Data  []Transaction `json:"data" bson:"data"`
	Error []Error       `json:"error" bson:"error"`
}

func NewTransactionBase(amount uint32, description string, typeOfTransaction TransactionType) *TransactionBase {
	return &TransactionBase{
		Amount:      amount,
		Description: description,
		Type:        typeOfTransaction,
	}
}

const (
	InitiatorTypeSalary    InitiatorType = "salary"
	InitiatorTypeRent      InitiatorType = "rent"
	InitiatorTypeUtilities InitiatorType = "utilities"
	InitiatorTypeOther     InitiatorType = "other"
	InitiatorTypeSales     InitiatorType = "sale"
	InitiatorTypeSupplier  InitiatorType = "supplier"
	InitiatorTypeBNPL      InitiatorType = "bnpl" // buy now pay later BNPL transactions
)

type PaymentMethod string

// | Mode              | Description                          | Common Term(s)               |
// |-------------------|--------------------------------------|------------------------------|
// | Cash              | Physical currency (bills/coins)      | Cash payment, Cash transaction|
// | Bank Transfer     | Funds moved between bank accounts    | Bank transfer, Wire transfer |
// | Credit/Debit Card | Via POS or online gateways           | Card payment, POS transaction|
// | Mobile Payment    | e.g., Apple Pay, Google Pay, QR scan | Mobile payment, Digital wallet|
// | Cheque            | Written order to transfer money      | Cheque payment               |
// | Online Transfer   | e.g., PayPal, Stripe, Revolut        | Online payment, e-payment    |

const (
	PaymentMethodCash      PaymentMethod = "cash"
	PaymentMethodBank      PaymentMethod = "bank"
	PaymentMethodTerminal  PaymentMethod = "terminal"
	OnlineMobileAppPayment PaymentMethod = "online_payment"
	Cheque                 PaymentMethod = "cheque"
	OnlineTransfer         PaymentMethod = "online_transfer"
	PaymentMethodUndefined PaymentMethod = "undefined"
)

type TransactionBase struct {
	Amount      uint32 `json:"amount" bson:"amount" gorm:"column:amount"`
	Description string `json:"description" bson:"description" gorm:"column:description"`
	// Type here is the credit/debit direction. In Mongo it collided on the bson
	// key "type" with Transaction.Type (initiator) and was effectively dropped;
	// in Postgres it has its own column `transaction_type`.
	Type          TransactionType `json:"type" bson:"type" gorm:"column:transaction_type"`
	PaymentMethod PaymentMethod   `json:"payment_method" bson:"payment_method" gorm:"column:payment_method"`
}

type Transaction struct {
	TransactionBase `gorm:"embedded"`
	// Type here is the initiator (sale/supplier/bnpl/...). Stored in `initiator_type`.
	Type      InitiatorType `json:"type" bson:"type" gorm:"column:initiator_type"`
	ID        string        `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	CreatedAt time.Time     `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt time.Time     `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
	BranchID  string        `json:"branch_id" bson:"branch_id" gorm:"column:branch_id"`
	// Relational foreign keys (Postgres only; not persisted in Mongo). They
	// replace journals.operations[], the embedded supplier transactions, and
	// bnpl.transactions[].
	JournalID  *string `json:"journal_id,omitempty" bson:"-" gorm:"column:journal_id"`
	SupplierID *string `json:"supplier_id,omitempty" bson:"-" gorm:"column:supplier_id"`
	BNPLID     *string `json:"bnpl_id,omitempty" bson:"-" gorm:"column:bnpl_id"`
	// CartID is the type-specific reference for initiator_type='sale': the
	// sale_carts row whose items produced this transaction. It is NULL for a
	// keyed/manual sale (an amount typed straight into a journal or the sales
	// endpoint), which has no items to point at — so a null cart on a sale is
	// expected, not a broken link.
	CartID *string `json:"cart_id,omitempty" bson:"-" gorm:"column:cart_id"`

	// ItemCount is how many cart lines sit behind this transaction. It is NOT a
	// column — the journals repository fills it in one batched query when it
	// loads a journal's operations, so a staff list can show "12 items" without
	// expanding every cart. Zero for any transaction with no cart.
	//
	// Clients decide whether a drill-in is available from CartID being non-null,
	// not from this count (a cart could legitimately be empty).
	ItemCount int `json:"item_count" bson:"-" gorm:"-"`
}

func (Transaction) TableName() string { return "transactions" }

// TransactionDetails is the type-specific payload behind a transaction: the
// thing staff actually want to see when they open one. It is a discriminated
// union keyed by Kind — exactly one of the pointers is set, and all of them are
// nil for a type that has no detail record of its own (salary, rent, utilities,
// other), where the description is the whole story.
type TransactionDetails struct {
	Kind     InitiatorType `json:"kind"`
	Cart     *SaleCart     `json:"cart,omitempty"`
	Supplier *Supplier     `json:"supplier,omitempty"`
	BNPL     *BNPL         `json:"bnpl,omitempty"`
}

// TransactionWithDetails is a transaction plus its resolved type-specific
// detail, as returned by GET /api/v1/admin/transactions/{id}/details.
type TransactionWithDetails struct {
	Transaction
	Details TransactionDetails `json:"details"`
}

func NewTransaction(transactionBase *TransactionBase, typeOfTransaction InitiatorType, branchID string) *Transaction {
	loc := utils.GetTimeZone()
	return &Transaction{
		TransactionBase: *transactionBase,
		Type:            typeOfTransaction,
		ID:              uuid.New().String(),
		CreatedAt:       time.Now().In(loc),
		UpdatedAt:       time.Now().In(loc),
		BranchID:        branchID,
	}
}

type TransactionQueryParams struct {
	Description       string          `query:"description"`
	AmountMin         uint32          `query:"amount_min"`
	AmountMax         uint32          `query:"amount_max"`
	DateMin           time.Time       `query:"date_min"`
	DateMax           time.Time       `query:"date_max"`
	PaymentMethod     PaymentMethod   `query:"payment_method"`
	TypeOfTransaction TransactionType `query:"type_of_transaction"`
	InitiatorType     InitiatorType   `query:"initiator_type"`
	Count             int             `query:"count"`
	Page              int             `query:"page"`
}

func ValidatePaymentMethod(paymentMethod PaymentMethod) error {
	paymentMethods := []PaymentMethod{
		PaymentMethodCash,
		PaymentMethodBank,
		PaymentMethodTerminal,
		OnlineMobileAppPayment,
		Cheque,
		OnlineTransfer,
	}
	if !slices.Contains(paymentMethods, paymentMethod) {
		return fmt.Errorf("invalid payment method: %s", paymentMethod)
	}
	return nil
}

func ValidateTransactionType(transactionType TransactionType) error {
	transactionTypes := []TransactionType{
		TransactionTypeCredit,
		TransactionTypeDebit,
	}
	if !slices.Contains(transactionTypes, transactionType) {
		return fmt.Errorf("invalid transaction type")
	}
	return nil
}

func ValidateInitiatorType(initiatorType InitiatorType) error {
	initiatorTypes := []InitiatorType{
		InitiatorTypeSalary,
		InitiatorTypeRent,
		InitiatorTypeUtilities,
		InitiatorTypeOther,
		InitiatorTypeSales,
		InitiatorTypeSupplier,
	}
	if !slices.Contains(initiatorTypes, initiatorType) {
		return fmt.Errorf("invalid initiator type")
	}
	return nil
}
func (t *TransactionQueryParams) Validate() error {

	// check payment method, type of transaction, initiator type
	if t.PaymentMethod != "" {
		if err := ValidatePaymentMethod(t.PaymentMethod); err != nil {
			return err
		}
	}
	if t.TypeOfTransaction != "" {
		if err := ValidateTransactionType(t.TypeOfTransaction); err != nil {
			return err
		}
	}
	if t.InitiatorType != "" {
		if err := ValidateInitiatorType(t.InitiatorType); err != nil {
			return err
		}
	}
	return nil
}

// mobile apps like click, paynet, payme, smartbank if they are used to transfer money directly
// then we need to add it to the mobile apps balance
// if apps are used to pay using qr code or app service then it is considered as bank transfer
type Balance struct {
	Cash       int32 `json:"cash" bson:"cash" gorm:"column:cash"`
	Bank       int32 `json:"bank" bson:"bank" gorm:"column:bank"`
	Terminal   int32 `json:"terminal" bson:"terminal" gorm:"column:terminal"`
	MobileApps int32 `json:"mobile_apps" bson:"mobile_apps" gorm:"column:mobile_apps"`
}

type Finance struct {
	Balance       Balance `json:"balance" bson:"balance" gorm:"embedded;embeddedPrefix:balance_"`
	TotalIncome   int32   `json:"total_income" bson:"total_income" gorm:"column:total_income"`
	TotalExpenses int32   `json:"total_expenses" bson:"total_expenses" gorm:"column:total_expenses"`
	Debt          int32   `json:"debt" bson:"debt" gorm:"column:debt"`
}

type FinanceWithTransactions struct {
	Finance
	Transactions []Transaction `json:"transactions" bson:"transactions" gorm:"-"`
}

// BranchFinance is the per-branch finance row (table branch_finance). Balances
// live in embedded columns (balance_cash/…); Suppliers is derived (not stored);
// Details is free-form jsonb.
type BranchFinance struct {
	Finance    `gorm:"embedded"`
	Suppliers  []string    `json:"suppliers" bson:"suppliers" gorm:"-"`
	BranchID   string      `json:"branch_id" bson:"branch_id" gorm:"column:branch_id;primaryKey"`
	BranchName string      `json:"branch_name" bson:"branch_name" gorm:"column:branch_name"`
	Details    interface{} `json:"details" bson:"details" gorm:"column:details;serializer:json"`
}

func (BranchFinance) TableName() string { return "branch_finance" }

type NewBranchFinanceInput struct {
	BranchName string      `json:"branch_name"`
	Details    interface{} `json:"details"`
}

type BranchFinanceOutput struct {
	Data  []BranchFinance `json:"data" bson:"data"`
	Error []Error         `json:"error" bson:"error"`
}
type BranchFinanceOutputSingle struct {
	Data  BranchFinance `json:"data" bson:"data"`
	Error []Error       `json:"error" bson:"error"`
}
