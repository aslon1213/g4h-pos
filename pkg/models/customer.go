// Package models holds the plain data structs and request/response DTOs for the
// storefront (/api/v1/store) domains. It contains no mongo/bson logic — bson
// struct tags are just metadata consumed by the repositories in
// pkg/repository/store/<domain>, which own every database operation.
//
// IDs are strings (uuid), matching the legacy Customer/Product/User models, so
// this package never needs to import the mongo driver.
package models

import "time"

// Address is a single entry in a customer's address book.
type Address struct {
	ID         string `json:"id" bson:"id"`
	Label      string `json:"label" bson:"label"` // e.g. "Home", "Work"
	FullName   string `json:"full_name" bson:"full_name"`
	Phone      string `json:"phone" bson:"phone"`
	Line1      string `json:"line1" bson:"line1"`
	Line2      string `json:"line2" bson:"line2"`
	City       string `json:"city" bson:"city"`
	Region     string `json:"region" bson:"region"`
	PostalCode string `json:"postal_code" bson:"postal_code"`
	Country    string `json:"country" bson:"country"`
	IsDefault  bool   `json:"is_default" bson:"is_default"`
}

// StoreCustomer is a storefront account (separate from the staff-managed POS
// `customers` collection). Stored in the `store_customers` collection.
//
// PasswordHash is never serialised to JSON (json:"-"); it is written/read only
// by the customer repository for authentication.
type StoreCustomer struct {
	ID            string    `json:"id" bson:"_id"`
	Email         string    `json:"email" bson:"email"`
	Phone         string    `json:"phone" bson:"phone"`
	Name          string    `json:"name" bson:"name"`
	PasswordHash  string    `json:"-" bson:"password_hash"`
	EmailVerified bool      `json:"email_verified" bson:"email_verified"`
	Addresses     []Address `json:"addresses" bson:"addresses"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at"`
}

// ---- request DTOs ----

// RegisterInput is the body for POST /api/v1/store/auth/register.
type RegisterInput struct {
	Email    string `json:"email"`
	Phone    string `json:"phone"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

// LoginInput is the body for POST /api/v1/store/auth/login.
type LoginInput struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// UpdateProfileInput is the body for PUT /api/v1/store/auth/me.
type UpdateProfileInput struct {
	Name  string `json:"name"`
	Phone string `json:"phone"`
}

// ForgotPasswordInput is the body for POST /api/v1/store/auth/password/forgot.
type ForgotPasswordInput struct {
	Email string `json:"email"`
}

// ResetPasswordInput is the body for POST /api/v1/store/auth/password/reset.
type ResetPasswordInput struct {
	Token    string `json:"token"`
	Password string `json:"password"`
}

// AddressInput is the body for creating/updating an address. ID is ignored on
// create (the repository assigns one) and taken from the path on update.
type AddressInput struct {
	Label      string `json:"label"`
	FullName   string `json:"full_name"`
	Phone      string `json:"phone"`
	Line1      string `json:"line1"`
	Line2      string `json:"line2"`
	City       string `json:"city"`
	Region     string `json:"region"`
	PostalCode string `json:"postal_code"`
	Country    string `json:"country"`
	IsDefault  bool   `json:"is_default"`
}
