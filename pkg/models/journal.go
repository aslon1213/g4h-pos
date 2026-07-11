package models

import (
	"time"
)

type Branch struct {
	Name     string `bson:"name" json:"name" gorm:"column:name"`
	Location string `bson:"location" json:"location" gorm:"column:location"`
	Phone    string `bson:"phone" json:"phone" gorm:"column:phone"`
	ID       string `bson:"_id" json:"id" gorm:"column:id"`
}

func DoesBranchExist(branch string) bool {
	_, ok := Branch_names[branch]
	return ok
}

var Branch_names map[string]Branch = map[string]Branch{
	"Xonobod": {
		Name:     "Xonobod",
		Location: "Xonobod",
		Phone:    "+998 97 034 38 58",
	},
	"Yangi Hayot": {
		Name:     "Yangi Hayot",
		Location: "Yangi Hayot Qo'rg'ontepa mahallasi",
		Phone:    "+998 33 119 12 13",
	},
	"Polevoy": {
		Name:     "Polevoy",
		Location: "Polevoy Savatchi mahallasi",
		Phone:    "+998 33 119 12 13",
	},
}

type JournalBase struct {
	Branch          Branch    `bson:"branch" json:"branch" gorm:"embedded;embeddedPrefix:branch_"`
	Date            time.Time `bson:"date" json:"date" gorm:"column:date"`
	ID              string    `bson:"_id" json:"id" gorm:"column:id;primaryKey"`
	Shift_is_closed bool      `bson:"shift_is_closed" json:"shift_is_closed" gorm:"column:shift_is_closed"`
	Terminal_income uint32    `bson:"terminal_income" json:"terminal_income" gorm:"column:terminal_income"`
	Cash_left       uint32    `bson:"cash_left" json:"cash_left" gorm:"column:cash_left"`
	Total           uint32    `bson:"total" json:"total" gorm:"column:total"`
}

type Journal struct {
	JournalBase `bson:",inline" gorm:"embedded"`
	Operations  []Transaction `bson:"operations" json:"operations" gorm:"-"`
}

func (Journal) TableName() string { return "journals" }

type JournalWithTransactionID struct {
	JournalBase `bson:",inline"`
	Operations  []string `bson:"operations" json:"operations" gorm:"-"`
}

type NewJournalEntryInput struct {
	BranchNameOrID string    `json:"branch_name_or_id"`
	Date           time.Time `json:"date"`
}

type TotalValueQueryParams struct {
	Min int32 `query:"min" default:"-1"`
	Max int32 `query:"max" default:"30000000"`
	Use bool  `query:"used" default:"false"`
}

type JournalQueryParams struct {
	BranchID string    `query:"branch_id" default:""`
	FromDate time.Time `query:"from_date" default:""`
	ToDate   time.Time `query:"to_date" default:""`
	Page     int       `query:"page" default:"1"`
	PageSize int       `query:"page_size" default:"10"`
	Total    TotalValueQueryParams
}

type JournalOperationInput struct {
	TransactionBase
	SupplierTransaction bool   `json:"supplier_transaction" bson:"supplier_transaction"`
	SupplierID          string `json:"supplier_id" bson:"supplier_id"`
}

type CloseJournalEntryInput struct {
	CashLeft       uint32 `json:"cash_left"`
	TerminalIncome uint32 `json:"terminal_income"`
}

type JournalOutput struct {
	Data  Journal `json:"data"`
	Error []Error `json:"error"`
}
type JournalOutputList struct {
	Data  []Journal `json:"data"`
	Error []Error   `json:"error"`
}

func (j *Journal) GetSummOfTotals(journals []*Journal) uint32 {
	var total uint32
	for _, journal := range journals {
		total += journal.Total
	}
	return total
}

func (j *Journal) GetNumberOfOperations() int {
	return len(j.Operations)
}
