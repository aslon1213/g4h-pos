package customers

import (
	"errors"

	models "github.com/aslon1213/g4h_pos_erp/pkg/models"
	customers_repository "github.com/aslon1213/g4h_pos_erp/pkg/repository/customers"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"

	"github.com/gofiber/fiber/v2"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// CustomersController exposes the admin customers endpoints. All database access
// goes through Repo; the controller only parses requests, validates input, and
// renders the response envelope.
type CustomersController struct {
	Repo *customers_repository.CustomersRepository
}

func New(db *gorm.DB) *CustomersController {
	return &CustomersController{
		Repo: customers_repository.New(db),
	}
}

type SortByBNPLTotal string

const (
	SortByBNPLTotalDesc SortByBNPLTotal = "max"
	SortByBNPLTotalAsc  SortByBNPLTotal = "min"
	SortByBNPLTotalNone SortByBNPLTotal = "none"
)

type CustomerQuery struct {
	Name            string          `query:"name"`
	Phone           string          `query:"phone"`
	Address         string          `query:"address"`
	SortByBNPLTotal SortByBNPLTotal `query:"sort_by_bnpl_total"`

	Page  int `query:"page" default:"1"`
	Count int `query:"count" default:"10"`
}

func (query *CustomerQuery) SetDefaults() {
	if query.SortByBNPLTotal == "" {
		query.SortByBNPLTotal = SortByBNPLTotalDesc
	}
	if query.Page <= 0 {
		query.Page = 1
	}
	if query.Count <= 0 {
		query.Count = 10
	}
}

// GetCustomers godoc
// @Security BearerAuth
// @Summary Get all customers
// @Description Get all customers from the database
// @Tags customers
// @Accept json
// @Produce json
// @Param name query string false "Customer name"
// @Param phone query string false "Customer phone"
// @Param address query string false "Customer address"
// @Param page query int false "Page number"
// @Param count query int false "Number of customers per page"
// @Param sort_by_bnpl_total query string false "Sort by BNPL total (max, min, none)"
// @Success 200 {object} models.CustomerQueryOutput
// @Failure 400 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/customers [get]
func (ctrl *CustomersController) GetCustomers(c *fiber.Ctx) error {
	var query CustomerQuery
	if err := c.QueryParser(&query); err != nil {
		log.Error().Err(err).Msg("Failed to parse customer query")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	query.SetDefaults()

	customers, total, err := ctrl.Repo.Find(c.Context(), customers_repository.ListParams{
		Name:    query.Name,
		Phone:   query.Phone,
		Address: query.Address,
		Sort:    string(query.SortByBNPLTotal),
		Page:    query.Page,
		Count:   query.Count,
	})
	if err != nil {
		log.Error().Err(err).Msg("Failed to find customers")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Debug().Int("count", len(customers)).Msg("Successfully retrieved customers")

	total_pages := int(total) / query.Count
	if int(total)%query.Count != 0 {
		total_pages++
	}
	output := models.NewCustomerQueryOutput(customers, total_pages, query.Page, query.Count)
	return c.JSON(output)
}

// GetCustomerByID godoc
// @Security BearerAuth
// @Summary Get a customer by ID
// @Description Get a customer by its ID
// @Tags customers
// @Produce json
// @Param id path string true "Customer ID"
// @Success 200 {object} models.Output[[]models.Customer]
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/customers/{id} [get]
func (ctrl *CustomersController) GetCustomerByID(c *fiber.Ctx) error {
	id := c.Params("id")
	log.Debug().Str("id", id).Msg("Getting customer by ID")

	customer, err := ctrl.Repo.GetByID(c.Context(), id)
	if err != nil {
		if errors.Is(err, repoerr.ErrNotFound) {
			log.Debug().Str("id", id).Msg("Customer not found")
			return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Customer not found",
				Code:    fiber.StatusNotFound,
			}))
		}
		log.Error().Err(err).Str("id", id).Msg("Failed to find customer")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Debug().Str("id", id).Msg("Successfully retrieved customer")
	return c.JSON(models.NewOutput([]models.Customer{*customer}))
}

// CreateCustomer godoc
// @Security BearerAuth
// @Summary Create a new customer
// @Description Create a new customer in the database
// @Tags customers
// @Accept json
// @Produce json
// @Param customer body models.CustomerBase true "Customer data"
// @Success 201 {object} models.Output[[]models.Customer]
// @Failure 400 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/customers [post]
func (ctrl *CustomersController) CreateCustomer(c *fiber.Ctx) error {
	log.Debug().Msg("Creating new customer")

	var customerBase models.CustomerBase
	if err := c.BodyParser(&customerBase); err != nil {
		log.Error().Err(err).Msg("Failed to parse customer data")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	// Validate required fields
	if customerBase.Name == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Customer name is required",
			Code:    fiber.StatusBadRequest,
		}))
	}
	if customerBase.Phone == "" {
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: "Customer phone is required",
			Code:    fiber.StatusBadRequest,
		}))
	}

	customer, err := ctrl.Repo.Create(c.Context(), customerBase)
	if err != nil {
		if errors.Is(err, repoerr.ErrConflict) {
			return c.Status(fiber.StatusConflict).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Customer with this phone number already exists",
				Code:    fiber.StatusConflict,
			}))
		}
		log.Error().Err(err).Msg("Failed to create customer")
		return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		}))
	}

	log.Debug().Str("id", customer.ID).Msg("Successfully created customer")
	return c.Status(fiber.StatusCreated).JSON(models.NewOutput([]models.Customer{*customer}))
}

// UpdateCustomer godoc
// @Security BearerAuth
// @Summary Update a customer
// @Description Update an existing customer in the database
// @Tags customers
// @Accept json
// @Produce json
// @Param id path string true "Customer ID"
// @Param customer body models.CustomerBase true "Customer data"
// @Success 200 {object} models.Output[[]models.Customer]
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 409 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/customers/{id} [put]
func (ctrl *CustomersController) UpdateCustomer(c *fiber.Ctx) error {
	id := c.Params("id")
	log.Debug().Str("id", id).Msg("Updating customer")

	var customerBase models.CustomerBase
	if err := c.BodyParser(&customerBase); err != nil {
		log.Error().Err(err).Msg("Failed to parse customer data")
		return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
			Message: err.Error(),
			Code:    fiber.StatusBadRequest,
		}))
	}

	updatedCustomer, err := ctrl.Repo.Update(c.Context(), id, customerBase)
	if err != nil {
		switch {
		case errors.Is(err, repoerr.ErrNotFound):
			return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Customer not found",
				Code:    fiber.StatusNotFound,
			}))
		case errors.Is(err, repoerr.ErrConflict):
			return c.Status(fiber.StatusConflict).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Another customer with this phone number already exists",
				Code:    fiber.StatusConflict,
			}))
		default:
			log.Error().Err(err).Str("id", id).Msg("Failed to update customer")
			return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: err.Error(),
				Code:    fiber.StatusInternalServerError,
			}))
		}
	}

	log.Debug().Str("id", id).Msg("Successfully updated customer")
	return c.JSON(models.NewOutput([]models.Customer{*updatedCustomer}))
}

// DeleteCustomer godoc
// @Security BearerAuth
// @Summary Delete a customer
// @Description Delete a customer from the database
// @Tags customers
// @Produce json
// @Param id path string true "Customer ID"
// @Success 200 {object} models.MessageResponse
// @Failure 400 {object} models.ErrorOutput
// @Failure 404 {object} models.ErrorOutput
// @Failure 500 {object} models.ErrorOutput
// @Router /api/v1/admin/customers/{id} [delete]
func (ctrl *CustomersController) DeleteCustomer(c *fiber.Ctx) error {
	id := c.Params("id")
	log.Debug().Str("id", id).Msg("Deleting customer")

	if err := ctrl.Repo.Delete(c.Context(), id); err != nil {
		switch {
		case errors.Is(err, customers_repository.ErrActiveBNPL):
			log.Debug().Str("id", id).Msg("Customer has active BNPL transactions")
			return c.Status(fiber.StatusBadRequest).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: err.Error(),
				Code:    fiber.StatusBadRequest,
			}))
		case errors.Is(err, repoerr.ErrNotFound):
			log.Debug().Str("id", id).Msg("Customer not found")
			return c.Status(fiber.StatusNotFound).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: "Customer not found",
				Code:    fiber.StatusNotFound,
			}))
		default:
			log.Error().Err(err).Str("id", id).Msg("Failed to delete customer")
			return c.Status(fiber.StatusInternalServerError).JSON(models.NewOutput([]interface{}{}, models.Error{
				Message: err.Error(),
				Code:    fiber.StatusInternalServerError,
			}))
		}
	}

	log.Debug().Str("id", id).Msg("Successfully deleted customer")
	return c.JSON(models.NewOutput(map[string]interface{}{
		"message": "Customer deleted successfully",
		"id":      id,
	}))
}
