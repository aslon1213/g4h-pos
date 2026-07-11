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
	ID         string `json:"id" bson:"id" gorm:"column:id;primaryKey"`
	CustomerID string `json:"customer_id" bson:"customer_id" gorm:"column:customer_id"`
	Label      string `json:"label" bson:"label" gorm:"column:label"` // e.g. "Home", "Work"
	FullName   string `json:"full_name" bson:"full_name" gorm:"column:full_name"`
	Phone      string `json:"phone" bson:"phone" gorm:"column:phone"`
	Line1      string `json:"line1" bson:"line1" gorm:"column:line1"`
	Line2      string `json:"line2" bson:"line2" gorm:"column:line2"`
	City       string `json:"city" bson:"city" gorm:"column:city"`
	Region     string `json:"region" bson:"region" gorm:"column:region"`
	PostalCode string `json:"postal_code" bson:"postal_code" gorm:"column:postal_code"`
	Country    string `json:"country" bson:"country" gorm:"column:country"`
	IsDefault  bool   `json:"is_default" bson:"is_default" gorm:"column:is_default"`
}

func (Address) TableName() string { return "addresses" }

// StoreCustomer is a storefront account (separate from the staff-managed POS
// `customers` collection). Stored in the `store_customers` collection.
//
// PasswordHash is never serialised to JSON (json:"-"); it is written/read only
// by the customer repository for authentication.
type StoreCustomer struct {
	ID            string    `json:"id" bson:"_id" gorm:"column:id;primaryKey"`
	Email         string    `json:"email" bson:"email" gorm:"column:email"`
	Phone         string    `json:"phone" bson:"phone" gorm:"column:phone"`
	Name          string    `json:"name" bson:"name" gorm:"column:name"`
	PasswordHash  string    `json:"-" bson:"password_hash" gorm:"column:password_hash"`
	EmailVerified bool      `json:"email_verified" bson:"email_verified" gorm:"column:email_verified"`
	Addresses     []Address `json:"addresses" bson:"addresses" gorm:"-"`
	CreatedAt     time.Time `json:"created_at" bson:"created_at" gorm:"column:created_at"`
	UpdatedAt     time.Time `json:"updated_at" bson:"updated_at" gorm:"column:updated_at"`
}

func (StoreCustomer) TableName() string { return "store_customers" }

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
