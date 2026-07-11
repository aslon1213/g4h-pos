package models

import "time"

type SupplierBase struct {
	Name    string `json:"name" bson:"name" gorm:"column:name"`
	Address string `json:"address" bson:"address" gorm:"column:address"`
	Phone   string `json:"phone" bson:"phone" gorm:"column:phone"`
	Email   string `json:"email,omitempty" bson:"email,omitempty" gorm:"column:email"`
	INN     string `json:"inn,omitempty" bson:"inn,omitempty" gorm:"column:inn"`
	Notes   string `json:"notes,omitempty" bson:"notes,omitempty" gorm:"column:notes"`
	Branch  string `json:"branch" bson:"branch" gorm:"column:branch_id"` // holds a branch id (FK)
}

// FinancialData is flattened onto the suppliers row (balance/total_income/
// total_expenses). The embedded transactions are NOT a column: they live in the
// transactions table (supplier_id) and are attached on read by the repository.
type FinancialData struct {
	Balance       int32         `json:"balance" bson:"balance" gorm:"column:balance"`
	Transactions  []Transaction `json:"transactions" bson:"transactions" gorm:"-"`
	TotalIncome   int32         `json:"total_income" bson:"total_income" gorm:"column:total_income"`
	TotalExpenses int32         `json:"total_expenses" bson:"total_expenses" gorm:"column:total_expenses"`
}

type Supplier struct {
	SupplierBase  `bson:",inline" gorm:"embedded"`
	ID            string        `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	FinancialData FinancialData `json:"financial_data" bson:"financial_data" gorm:"embedded"`
	CreatedAt     time.Time     `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt     time.Time     `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (Supplier) TableName() string { return "suppliers" }

type SupplierOutput struct {
	Data  []Supplier `json:"data" bson:"data"`
	Error []Error    `json:"error" bson:"error"`
}

type SupplierOutputSingle struct {
	Data  Supplier `json:"data" bson:"data"`
	Error []Error  `json:"error" bson:"error"`
}

// SupplierQueryParams are the filters accepted by the suppliers list endpoint
// (bound from the query string). Branch may be a branch id or name; the
// repository resolves it to a branch id.
type SupplierQueryParams struct {
	Name    string `query:"name"`
	INN     string `query:"inn"`
	Branch  string `query:"branch"`
	Email   string `query:"email"`
	Phone   string `query:"phone"`
	Address string `query:"address"`
	Notes   string `query:"notes"`
}
