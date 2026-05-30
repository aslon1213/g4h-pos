package models

import (
	"context"

	"github.com/gofiber/fiber/v2"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

type Error struct {
	Message string `json:"message"`
	Code    int    `json:"code"`
}

// Output is the standard response envelope. T is the type of the Data payload,
// so callers get a typed body instead of an untyped interface{}.
type Output[T any] struct {
	Data  T       `json:"data"`
	Error []Error `json:"error"`
}

// NewOutput builds a success envelope around data of any type T. T is inferred
// from the argument, so call sites need no type annotation (e.g.
// NewOutput([]Product{...}) yields Output[[]Product]).
func NewOutput[T any](data T, errors ...Error) Output[T] {
	return Output[T]{
		Data:  data,
		Error: errors,
	}
}

// NewErrorOutput builds an envelope with no data payload, only errors. Use this
// instead of NewOutput(nil, ...) — an untyped nil cannot drive type inference.
func NewErrorOutput(errors ...Error) Output[any] {
	return Output[any]{
		Data:  nil,
		Error: errors,
	}
}

func NewError(message string, code int) Error {
	return Error{
		Message: message,
		Code:    code,
	}
}
func NewErrors(errors ...error) []Error {
	errs := []Error{}
	for _, err := range errors {
		errs = append(errs, Error{
			Message: err.Error(),
			Code:    fiber.StatusInternalServerError,
		})
	}
	return errs
}

func AbortTransactionAndReturnError(ctx context.Context, session *mongo.Session, c *fiber.Ctx, err error) error {

	return c.Status(fiber.StatusInternalServerError).JSON(NewErrorOutput(Error{
		Message: err.Error(),
		Code:    fiber.StatusInternalServerError,
	}))
}

func ReturnError(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusInternalServerError).JSON(NewErrorOutput(Error{
		Message: err.Error(),
		Code:    fiber.StatusInternalServerError,
	}))
}

// NotImplemented is a placeholder response for stubbed-out handlers. It returns
// HTTP 501 in the standard Output envelope so routes are reachable and testable
// before their business logic is written.
func NotImplemented(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(NewErrorOutput(NewError("not implemented", fiber.StatusNotImplemented)))
}
