package models

import (
	"errors"
	"time"
)

type BNPLStatus string

const (
	BNPLStatusActive    BNPLStatus = "active"
	BNPLStatusCompleted BNPLStatus = "completed"
	BNPLStatusCancelled BNPLStatus = "cancelled"
)

type NewBNPLInput struct {
	CustomerID           string                      `json:"customer_id"`
	TotalAmount          int32                       `json:"total_amount"`
	CalculateTotalAmount bool                        `json:"calculate_total_amount"`
	BranchID             string                      `json:"branch_id"`
	Products             map[string]SalesSessionItem `json:"products"`
}

func (n *NewBNPLInput) Validate() error {
	if n.CustomerID == "" {
		return errors.New("customer_id is required")
	}
	if n.BranchID == "" {
		return errors.New("branch_id is required")
	}
	return nil
}

type BNPL struct {
	ID           string                      `json:"id" bson:"id" gorm:"column:id;primaryKey"`
	CustomerID   string                      `json:"customer_id" bson:"customer_id" gorm:"column:customer_id"`
	TotalAmount  int32                       `json:"total_amount" bson:"total_amount" gorm:"column:total_amount"`
	BranchID     string                      `json:"branch_id" bson:"branch_id" gorm:"column:branch_id"`
	Products     map[string]SalesSessionItem `json:"products" bson:"products" gorm:"-"` // products in the BNPL
	PaidAmount   int32                       `json:"paid_amount" bson:"paid_amount" gorm:"column:paid_amount"`
	Status       BNPLStatus                  `json:"status" bson:"status" gorm:"column:status"` // active, completed, cancelled
	Transactions []string                    `json:"transactions" bson:"transactions" gorm:"-"` // id of transactions
	CreatedAt    time.Time                   `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt    time.Time                   `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (BNPL) TableName() string { return "bnpls" }

type BnplProduct struct {
	ID        uint   `json:"-" bson:"-" gorm:"column:id;primaryKey"`
	BnplID    string `json:"-" bson:"-" gorm:"column:bnpl_id"`
	ProductID string `json:"product_id" bson:"product_id" gorm:"column:product_id"`
	Quantity  int    `json:"quantity" bson:"quantity" gorm:"column:quantity"`
	Price     int32  `json:"price" bson:"price" gorm:"column:price"`
}

func (BnplProduct) TableName() string { return "bnpl_products" }

func (b *BNPL) UpdateCreatedAt() {
	b.CreatedAt = time.Now()
}
