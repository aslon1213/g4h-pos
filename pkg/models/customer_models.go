package models

import "time"

type CustomerBase struct {
	Name  string `json:"name" bson:"name" gorm:"column:name"`
	Phone string `json:"phone" bson:"phone" gorm:"column:phone"`
	// Email   string `json:"email" bson:"email"`
	Address        string            `json:"address" bson:"address" gorm:"column:address"`
	AdditionalInfo map[string]string `json:"additional_info" bson:"additional_info" gorm:"column:additional_info;serializer:json"`
}

type Customer struct {
	CustomerBase    `bson:",inline" gorm:"embedded"`
	ID              string         `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	BNPLs           []BNPL         `json:"bnpls" bson:"bnpls" gorm:"-"`
	PurchaseHistory []SalesSession `json:"purchase_history" bson:"purchase_history" gorm:"column:purchase_history;serializer:json"` // purchase history is updated when a customer a sales session or completes a BNPL session
	CreatedAt       time.Time      `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt       time.Time      `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (Customer) TableName() string { return "customers" }

type CustomerQueryOutputData struct {
	Customers []Customer `json:"customers" bson:"customers"`
	Total     int        `json:"total" bson:"total"`
	Page      int        `json:"page" bson:"page"`
	Count     int        `json:"count" bson:"count"`
}

type CustomerQueryOutput struct {
	Data  []CustomerQueryOutputData `json:"data" bson:"data"`
	Error []Error                   `json:"error" bson:"error"`
}

func NewCustomerQueryOutput(customers []Customer, total int, page int, count int) *CustomerQueryOutput {
	return &CustomerQueryOutput{
		Data: []CustomerQueryOutputData{
			{Customers: customers, Total: total, Page: page, Count: count},
		},
	}
}

func (c *Customer) UpdateCreatedAt() {
	c.CreatedAt = time.Now()
}
