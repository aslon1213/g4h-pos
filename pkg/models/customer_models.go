package models

import "time"

type CustomerBase struct {
	Name  string `json:"name" bson:"name"`
	Phone string `json:"phone" bson:"phone"`
	// Email   string `json:"email" bson:"email"`
	Address        string            `json:"address" bson:"address"`
	AdditionalInfo map[string]string `json:"additional_info" bson:"additional_info"`
}

type Customer struct {
	CustomerBase    `bson:",inline"`
	ID              string         `json:"id" bson:"_id"`
	BNPLs           []BNPL         `json:"bnpls" bson:"bnpls"`
	PurchaseHistory []SalesSession `json:"purchase_history" bson:"purchase_history"` // purchase history is updated when a customer a sales session or completes a BNPL session
	CreatedAt       time.Time      `json:"created_at" bson:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at" bson:"updated_at"`
}

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
