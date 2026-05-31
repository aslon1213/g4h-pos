package client

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
)

func DecodeProductOutput(response *http.Response) (models.ProductOutput, error) {
	var productOutput models.ProductOutput
	err := json.NewDecoder(response.Body).Decode(&productOutput)
	if err != nil {
		return models.ProductOutput{}, err
	}
	return productOutput, nil
}

func (c *Client) CreateProduct(base *models.ProductBase) (*http.Response, models.ProductOutput, error) {

	body, err := json.Marshal(base)

	if err != nil {
		log.Fatal(err)
	}

	resp, err := c.MakeRequest(
		"POST",
		"/api/products",
		body,
		map[string]string{},
		true,
	)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	// decode response
	productOutput, err := DecodeProductOutput(resp)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	return resp, productOutput, nil

}

// EDIT
func (c *Client) EditProduct(id string, base *models.ProductBase) (*http.Response, models.ProductOutput, error) {
	body, err := json.Marshal(base)
	if err != nil {
		log.Fatal(err)
	}
	resp, err := c.MakeRequest(
		"PUT",
		"/api/products/"+id,
		body,
		map[string]string{},
		true,
	)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	// decode response
	productOutput, err := DecodeProductOutput(resp)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	return resp, productOutput, nil
}

// DELETE
func (c *Client) DeleteProduct(id string) (*http.Response, models.ProductOutput, error) {
	resp, err := c.MakeRequest(
		"DELETE",
		"/api/products/"+id,
		nil,
		map[string]string{},
		true,
	)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	// decode response
	productOutput, err := DecodeProductOutput(resp)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	return resp, productOutput, nil
}

// Query

func (c *Client) QueryProducts(params *models.ProductQueryParams) (*http.Response, models.ProductOutput, error) {
	// construct query string
	query := ""
	if params.BranchID != "" {
		query += "branch_id=" + params.BranchID + "&"
	}
	if params.Category != "" {
		query += "category=" + params.Category + "&"
	}
	if params.SKU != "" {
		query += "sku=" + params.SKU + "&"
	}
	if params.PriceMin != 0 {
		query += "price_min=" + strconv.FormatFloat(params.PriceMin, 'f', -1, 64) + "&"
	}
	if params.PriceMax != 0 {
		query += "price_max=" + strconv.FormatFloat(params.PriceMax, 'f', -1, 64) + "&"
	}
	resp, err := c.MakeRequest(
		"GET",
		"/api/products?"+query,
		nil,
		map[string]string{},
		true,
	)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	// decode response
	productOutput, err := DecodeProductOutput(resp)
	if err != nil {
		return nil, models.ProductOutput{}, err
	}
	return resp, productOutput, nil
}
