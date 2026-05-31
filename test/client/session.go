package client

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
)

type SessionResponse struct {
	Data  []*models.SalesSession `json:"data"`
	Error models.Error           `json:"error"`
}

func DecodeSessionResponse(response *http.Response) ([]*models.SalesSession, error) {
	session_response := SessionResponse{}
	err := json.NewDecoder(response.Body).Decode(&session_response)
	if err != nil {
		return nil, err
	}
	return session_response.Data, nil
}

func (c *Client) NewSession(branch_id string) (*models.SalesSession, error) {
	endpoint := fmt.Sprintf("/api/sales/session/branch/%s", branch_id)
	response, err := c.MakeRequest("POST", endpoint, nil, nil, false)
	if err != nil {
		return nil, err
	}
	sessions, err := DecodeSessionResponse(response)
	if err != nil {
		return nil, err
	}
	session := sessions[0]

	return session, nil
}

func (c *Client) GerSalesSession(session_id string) (*models.SalesSession, error) {
	endpoint := fmt.Sprintf("/api/sales/session/%s", session_id)
	response, err := c.MakeRequest("GET", endpoint, nil, nil, false)
	if err != nil {
		return nil, err
	}
	sessions, err := DecodeSessionResponse(response)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, errors.New("no session found")
	}
	return sessions[0], nil
}

type AddProductItemToSessionInput struct {
	ID       string `json:"id"`
	Quantity int    `json:"quantity"`
}

func (c *Client) AddProductToSession(session_id string, product_id string, quantity int) (*models.SalesSession, error) {

	endpoint := fmt.Sprintf("/api/sales/session/%s/product", session_id)

	input := AddProductItemToSessionInput{
		ID:       product_id,
		Quantity: quantity,
	}

	body, err := json.Marshal(input)
	if err != nil {
		return nil, err
	}

	response, err := c.MakeRequest("POST", endpoint, body, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, err
	}
	sessions, err := DecodeSessionResponse(response)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, errors.New("no session found")
	}
	return sessions[0], nil
}

func (c *Client) GetSalesSessionByBranchID(branch_id string) ([]*models.SalesSession, error) {
	endpoint := fmt.Sprintf("/sales/session/branch/%s", branch_id)
	response, err := c.MakeRequest("GET", endpoint, nil, nil, false)
	if err != nil {
		return nil, err
	}
	return DecodeSessionResponse(response)
}

func (c *Client) DeleteSalesSession(session_id string) (*models.SalesSession, error) {
	endpoint := fmt.Sprintf("/api/sales/session/%s", session_id)
	response, err := c.MakeRequest("DELETE", endpoint, nil, nil, false)
	if err != nil {
		return nil, err
	}
	sessions, err := DecodeSessionResponse(response)
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, errors.New("no session found")
	}
	return sessions[0], nil
}
