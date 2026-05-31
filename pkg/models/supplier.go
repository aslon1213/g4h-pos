package models

import "time"

type SupplierBase struct {
	Name    string `json:"name" bson:"name"`
	Address string `json:"address" bson:"address"`
	Phone   string `json:"phone" bson:"phone"`
	Email   string `json:"email,omitempty" bson:"email,omitempty"`
	INN     string `json:"inn,omitempty" bson:"inn,omitempty"`
	Notes   string `json:"notes,omitempty" bson:"notes,omitempty"`
	Branch  string `json:"branch" bson:"branch"`
}

type FinancialData struct {
	Balance       int32         `json:"balance" bson:"balance"`
	Transactions  []Transaction `json:"transactions" bson:"transactions"`
	TotalIncome   int32         `json:"total_income" bson:"total_income"`
	TotalExpenses int32         `json:"total_expenses" bson:"total_expenses"`
}

type Supplier struct {
	SupplierBase  `bson:",inline"`
	ID            string        `json:"id" bson:"_id"`
	FinancialData FinancialData `json:"financial_data" bson:"financial_data"`
	CreatedAt     time.Time     `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time     `json:"updated_at" bson:"updated_at"`
}

type SupplierOutput struct {
	Data  []Supplier `json:"data" bson:"data"`
	Error []Error    `json:"error" bson:"error"`
}

type SupplierOutputSingle struct {
	Data  Supplier `json:"data" bson:"data"`
	Error []Error  `json:"error" bson:"error"`
}
