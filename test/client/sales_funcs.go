package client

import (
	"encoding/json"
	"net/http"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"

	"github.com/rs/zerolog/log"
)

func (c *Client) CreateSalesTransaction(branch_id string, transaction models.TransactionBase) (resp *http.Response, output models.TransactionOutputSingle, err error) {
	log.Info().Msgf("Transaction: %+v", transaction)
	json_transaction, err := json.Marshal(transaction)
	if err != nil {
		return nil, models.TransactionOutputSingle{}, err
	}
	resp, err = c.MakeRequest("POST", "/api/sales/transactions/"+branch_id, json_transaction, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.TransactionOutputSingle{}, err
	}
	output, err = DecodeTransactionOutputSingle(resp)
	return resp, output, err
}

func (c *Client) DeleteSalesTransaction(branch_id string, transaction_id string) (resp *http.Response, output models.TransactionOutputSingle, err error) {
	resp, err = c.MakeRequest("DELETE", "/api/sales/"+branch_id+"/"+transaction_id, nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.TransactionOutputSingle{}, err
	}
	output, err = DecodeTransactionOutputSingle(resp)
	return resp, output, err
}
