package models

import (
	"github.com/google/uuid"
)

type User struct {
	ID       string `bson:"_id" json:"id"`
	Email    string `bson:"email" unique:"true" json:"email"`
	Username string `bson:"username" unique:"true" json:"username"`
	Password string `bson:"password" json:"password"`
	Role     string `bson:"role" json:"role"`
	Phone    string `bson:"phone" json:"phone"`
	Branch   string `bson:"branch" json:"branch"`
}

type UserRegisterInput struct {
	Username string `bson:"username" unique:"true" json:"username"`
	Password string `bson:"password" json:"password"`
	Email    string `bson:"email" unique:"true" json:"email"`
	Role     string `bson:"role" json:"role"`
	Phone    string `bson:"phone" json:"phone"`
	Branch   string `bson:"branch" json:"branch"`
}

func NewUser(username string, password string, email string, role string, phone string, branch string) *User {
	return &User{
		ID:       uuid.New().String(),
		Username: username,
		Password: password,
		Email:    email,
		Role:     role,
		Phone:    phone,
		Branch:   branch,
	}
}
