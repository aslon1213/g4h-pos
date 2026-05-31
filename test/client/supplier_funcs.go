package client

import (
	"encoding/json"
	"fmt"
	"net/http"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
)

func DecodeSupplierOutputSingle(resp *http.Response) (models.SupplierOutputSingle, error) {
	var output models.SupplierOutputSingle
	err := json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return models.SupplierOutputSingle{}, err
	}
	return output, nil
}

func DecodeSupplierOutput(resp *http.Response) (models.SupplierOutput, error) {
	var output models.SupplierOutput
	err := json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return models.SupplierOutput{}, err
	}
	return output, nil
}

func (c *Client) GetAllSuppliers() (resp *http.Response, output models.SupplierOutput, err error) {
	resp, err = c.MakeRequest("GET", "/api/suppliers", nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.SupplierOutput{}, err
	}
	output, err = DecodeSupplierOutput(resp)
	return resp, output, err
}

func (c *Client) GetSupplierByID(id string) (resp *http.Response, output models.SupplierOutputSingle, err error) {
	resp, err = c.MakeRequest("GET", fmt.Sprintf("/api/suppliers/%s", id), nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.SupplierOutputSingle{}, err
	}
	output, err = DecodeSupplierOutputSingle(resp)
	return resp, output, err
}

func (c *Client) CreateSupplier(supplier models.SupplierBase) (resp *http.Response, output models.SupplierOutputSingle, err error) {
	body, err := json.Marshal(supplier)
	if err != nil {
		return nil, models.SupplierOutputSingle{}, err
	}
	resp, err = c.MakeRequest("POST", "/api/suppliers", body, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.SupplierOutputSingle{}, err
	}
	output, err = DecodeSupplierOutputSingle(resp)
	return resp, output, err
}

func (c *Client) UpdateSupplier(id string, supplier models.SupplierBase) (resp *http.Response, output map[string]string, err error) {
	body, err := json.Marshal(supplier)
	if err != nil {
		return nil, map[string]string{}, err
	}
	resp, err = c.MakeRequest("PUT", fmt.Sprintf("/api/suppliers/%s", id), body, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, map[string]string{}, err
	}
	// unmarshal json to map[string]string
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return nil, map[string]string{}, err
	}
	return resp, output, err
}

func (c *Client) DeleteSupplier(id string) (resp *http.Response, output map[string]string, err error) {
	resp, err = c.MakeRequest("DELETE", fmt.Sprintf("/api/suppliers/%s", id), nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, map[string]string{}, err
	}
	// unmarshal json to map[string]string
	err = json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return nil, map[string]string{}, err
	}
	return resp, output, err
}

func DecodeTransactionOutputSingle(resp *http.Response) (models.TransactionOutputSingle, error) {
	var output models.TransactionOutputSingle
	err := json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return models.TransactionOutputSingle{}, err
	}
	return output, nil
}

func DecodeTransactionOutput(resp *http.Response) (models.TransactionOutput, error) {
	var output models.TransactionOutput
	err := json.NewDecoder(resp.Body).Decode(&output)
	if err != nil {
		return models.TransactionOutput{}, err
	}
	return output, nil
}
func (c *Client) NewSupplierTransaction(branch_id, supplier_id string, transaction models.TransactionBase) (resp *http.Response, output models.TransactionOutputSingle, err error) {
	body, err := json.Marshal(transaction)
	if err != nil {
		return nil, models.TransactionOutputSingle{}, err
	}
	resp, err = c.MakeRequest("POST", fmt.Sprintf("/api/suppliers/%s/%s/transactions", branch_id, supplier_id), body, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.TransactionOutputSingle{}, err
	}
	output, err = DecodeTransactionOutputSingle(resp)
	return resp, output, err
}
