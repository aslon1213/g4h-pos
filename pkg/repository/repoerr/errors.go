// Package repoerr defines the sentinel errors that repositories return so that
// controllers can map a failure to an HTTP status without importing mongo/bson.
//
// Repositories translate driver-specific errors (e.g. mongo.ErrNoDocuments, a
// duplicate-key write error) into these sentinels; handlers compare with
// errors.Is and let models.RespondRepoError pick the status code.
package repoerr

import "errors"

var (
	// ErrNotFound is returned when a requested document does not exist.
	ErrNotFound = errors.New("resource not found")
	// ErrConflict is returned on a uniqueness violation (duplicate key).
	ErrConflict = errors.New("resource already exists")
	// ErrInvalidInput is returned when the repository rejects the arguments
	// (e.g. empty id, non-positive quantity) before touching the database.
	ErrInvalidInput = errors.New("invalid input")
)
