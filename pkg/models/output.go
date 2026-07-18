package models

import (
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/gofiber/fiber/v2"
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

func ReturnError(c *fiber.Ctx, err error) error {
	return c.Status(fiber.StatusInternalServerError).JSON(NewErrorOutput(Error{
		Message: err.Error(),
		Code:    fiber.StatusInternalServerError,
	}))
}

// RespondError writes the standard error envelope with an explicit status code.
// Use it from handlers for validation/parse failures that are not repository
// sentinels (e.g. a bad request body).
func RespondError(c *fiber.Ctx, code int, msg string) error {
	return c.Status(code).JSON(NewErrorOutput(NewError(msg, code)))
}

// RespondRepoError maps a repository sentinel error (see pkg/repository/repoerr)
// to the matching HTTP status, so every handler can simply
// `return models.RespondRepoError(c, err)` without importing mongo.
func RespondRepoError(c *fiber.Ctx, err error) error {
	switch {
	case errors.Is(err, repoerr.ErrNotFound):
		return RespondError(c, fiber.StatusNotFound, err.Error())
	case errors.Is(err, repoerr.ErrConflict):
		return RespondError(c, fiber.StatusConflict, err.Error())
	case errors.Is(err, repoerr.ErrInvalidInput):
		return RespondError(c, fiber.StatusBadRequest, err.Error())
	default:
		return RespondError(c, fiber.StatusInternalServerError, err.Error())
	}
}

// NotImplemented is a placeholder response for stubbed-out handlers. It returns
// HTTP 501 in the standard Output envelope so routes are reachable and testable
// before their business logic is written.
func NotImplemented(c *fiber.Ctx) error {
	return c.Status(fiber.StatusNotImplemented).JSON(NewErrorOutput(NewError("not implemented", fiber.StatusNotImplemented)))
}
