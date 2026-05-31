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
	ID           string                      `json:"id" bson:"id"`
	CustomerID   string                      `json:"customer_id" bson:"customer_id"`
	TotalAmount  int32                       `json:"total_amount" bson:"total_amount"`
	BranchID     string                      `json:"branch_id" bson:"branch_id"`
	Products     map[string]SalesSessionItem `json:"products" bson:"products"` // products in the BNPL
	PaidAmount   int32                       `json:"paid_amount" bson:"paid_amount"`
	Status       BNPLStatus                  `json:"status" bson:"status"`             // active, completed, cancelled
	Transactions []string                    `json:"transactions" bson:"transactions"` // id of transactions
	CreatedAt    time.Time                   `json:"created_at" bson:"created_at"`
	UpdatedAt    time.Time                   `json:"updated_at" bson:"updated_at"`
}

func (b *BNPL) UpdateCreatedAt() {
	b.CreatedAt = time.Now()
}
