package client

import (
	"fmt"
	"net/http"
	"time"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
)

func (c *Client) GetTransactions(branch_id string, description string, amount_min uint32, amount_max uint32, payment_method models.PaymentMethod, type_of_transaction models.TransactionType, initiator_type models.InitiatorType, date_min time.Time, date_max time.Time, page int, count int) (resp *http.Response, output models.TransactionOutput, err error) {
	url := fmt.Sprintf("/api/transactions/branch/%s?", branch_id)
	if description != "" {
		url += fmt.Sprintf("description=%s&", description)
	}
	if amount_min != 0 {
		url += fmt.Sprintf("amount_min=%d&", amount_min)
	}
	if amount_max != 0 {
		url += fmt.Sprintf("amount_max=%d&", amount_max)
	}
	if payment_method != "" {
		url += fmt.Sprintf("payment_method=%s&", payment_method)
	}
	if type_of_transaction != "" {
		url += fmt.Sprintf("type_of_transaction=%s&", type_of_transaction)
	}
	if initiator_type != "" {
		url += fmt.Sprintf("initiator_type=%s&", initiator_type)
	}
	if !date_min.IsZero() {
		url += fmt.Sprintf("date_min=%s&", date_min.Format(time.RFC3339))
	}
	if !date_max.IsZero() {
		url += fmt.Sprintf("date_max=%s&", date_max.Format(time.RFC3339))
	}
	if page != 0 && count != 0 {
		url += fmt.Sprintf("page=%d&count=%d", page, count)
	}
	resp, err = c.MakeRequest("GET", url, nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.TransactionOutput{}, err
	}
	output, err = DecodeTransactionOutput(resp)
	return resp, output, err
}

func (c *Client) GetTransactionByID(branch_id string, transaction_id string) (resp *http.Response, output models.TransactionOutputSingle, err error) {
	resp, err = c.MakeRequest("GET", fmt.Sprintf("/api/transactions/%s", transaction_id), nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.TransactionOutputSingle{}, err
	}
	output, err = DecodeTransactionOutputSingle(resp)
	return resp, output, err
}
