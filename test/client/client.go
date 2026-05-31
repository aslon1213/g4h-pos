package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"

	"github.com/rs/zerolog/log"
)

type Client struct {
	Host     string
	Port     string
	Username string
	Passwrod string
	Token    string
}

func NewClient(host, port, username, password string) *Client {
	c := &Client{
		Host:     host,
		Port:     port,
		Username: username,
		Passwrod: password,
	}
	_, token, err := c.Login()
	if err != nil {
		log.Error().Err(err).Msg("Failed to login")
	}
	c.Token = token
	return c
}

func (c *Client) MakeRequest(method, path string, body []byte, headers map[string]string, auth_required bool) (*http.Response, error) {

	// if auth_required {
	// 	if c.Token == "" {
	// 		_, token, err := c.Login()
	// 		if err != nil {
	// 			return nil, err
	// 		}
	// 		c.Token = token
	// 	}
	// 	// headers["Authorization"] = "Bearer " + c.Token
	// }

	client := &http.Client{}
	req, err := http.NewRequest(method, "http://"+c.Host+c.Port+path, bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}

	for key, value := range headers {
		req.Header.Set(key, value)
	}
	// set token to header
	req.Header.Set("Authorization", c.Token)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	// if resp.StatusCode !=

	return resp, nil
}

func (c *Client) DecodeResponseMultiple(resp *http.Response) (models.BranchFinanceOutput, error) {
	output := models.BranchFinanceOutput{}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return output, err
	}
	err = json.Unmarshal(body, &output)
	if err != nil {
		log.Error().Str("body", string(body)).Err(err).Msg("Failed to decode response multiple")
		return output, err
	}
	return output, nil
}

func (c *Client) DecodeResponseSingle(resp *http.Response) (models.BranchFinanceOutputSingle, error) {
	output := models.BranchFinanceOutputSingle{}
	if resp.StatusCode != http.StatusOK {
		var output_ struct {
			Data  interface{}
			Error []models.Error
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return output, err
		}
		err = json.Unmarshal(body, &output_)
		if err != nil {
			log.Error().Err(err).Msg("Failed to decode response single")
			return output, err
		}
		output.Error = output_.Error
		return output, nil
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return output, err
	}
	err = json.Unmarshal(body, &output)

	if err != nil {
		log.Error().Str("body", string(body)).Err(err).Msg("Failed to decode response single")
		return output, err
	}
	return output, nil
}

func (c *Client) EncodeBody(body interface{}) ([]byte, error) {
	return json.Marshal(body)
}

// Get all branches
func (c *Client) GetAllBranches() (resp *http.Response, output models.BranchFinanceOutput, err error) {
	resp, err = c.MakeRequest("GET", "/api/finance/branches", nil, map[string]string{"Content-Type": "application/json"}, true)
	if err != nil {
		return nil, models.BranchFinanceOutput{}, err
	}
	// body, err := io.ReadAll(resp.Body)
	// if err != nil {
	// 	return nil, models.BranchFinanceOutput{}, err
	// }
	// log.Info().Str("resp", string(body)).Msg("Getting all branches")
	output, err = c.DecodeResponseMultiple(resp)
	log.Info().Str("output", fmt.Sprintf("%+v", output)).Err(err).Msg("Getting all branches")
	return resp, output, err
}

func (c *Client) GetBranchByID(branchID string) (resp *http.Response, output models.BranchFinanceOutputSingle, err error) {
	resp, err = c.MakeRequest("GET", fmt.Sprintf("/api/finance/branch/id/%s", branchID), nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.BranchFinanceOutputSingle{}, err
	}

	output, err = c.DecodeResponseSingle(resp)
	return resp, output, err
}

func (c *Client) GetBranchByName(branchName string) (resp *http.Response, output models.BranchFinanceOutputSingle, err error) {
	resp, err = c.MakeRequest("GET", fmt.Sprintf("/api/finance/branch/name/%s", branchName), nil, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.BranchFinanceOutputSingle{}, err
	}
	output, err = c.DecodeResponseSingle(resp)
	return resp, output, err
}

func (c *Client) CreateBranch(branch models.NewBranchFinanceInput) (resp *http.Response, output models.BranchFinanceOutputSingle, err error) {
	body, err := c.EncodeBody(branch)
	if err != nil {
		return nil, models.BranchFinanceOutputSingle{}, err
	}

	log.Info().Str("body", string(body)).Msg("Creating branch")

	resp, err = c.MakeRequest("POST", "/api/finance", body, map[string]string{"Content-Type": "application/json"}, false)
	if err != nil {
		return nil, models.BranchFinanceOutputSingle{}, err
	}
	output, err = c.DecodeResponseSingle(resp)
	return resp, output, err
}
