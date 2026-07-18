package internalexpenses

import (
	"github.com/gofiber/fiber/v2"
	"gorm.io/gorm"
)

type InternalExpensesController struct {
	db *gorm.DB
}

func New(db *gorm.DB) *InternalExpensesController {
	return &InternalExpensesController{
		db: db,
	}
}

func (i *InternalExpensesController) GetInternalExpenses(c *fiber.Ctx) error {
	panic("not implemented")
}

func (i *InternalExpensesController) GetInternalExpense(c *fiber.Ctx) error {
	panic("not implemented")
}

func (i *InternalExpensesController) CreateInternalExpense(c *fiber.Ctx) error {
	panic("not implemented")
}
