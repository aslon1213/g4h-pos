package models

import (
	"github.com/google/uuid"
)

type User struct {
	ID       string   `bson:"_id" json:"id" gorm:"column:id;primaryKey"`
	Email    string   `bson:"email" unique:"true" json:"email" gorm:"column:email"`
	Username string   `bson:"username" unique:"true" json:"username" gorm:"column:username"`
	Password string   `bson:"password" json:"password" gorm:"column:password"`
	Role     UserRole `bson:"role" json:"role" gorm:"column:role"`
	Phone    string   `bson:"phone" json:"phone" gorm:"column:phone"`
	Branch   string   `bson:"branch" json:"branch" gorm:"column:branch"`
}

func (User) TableName() string { return "users" }

type UserRole string

const (
	UserRoleAdmin   UserRole = "admin"
	UserRoleStaff   UserRole = "staff"
	UserRoleManager UserRole = "manager"
	UserRoleUser    UserRole = "user"
)

type UserRegisterInput struct {
	Username string   `bson:"username" unique:"true" json:"username"`
	Password string   `bson:"password" json:"password"`
	Email    string   `bson:"email" unique:"true" json:"email"`
	Role     UserRole `bson:"role" json:"role"`
	Phone    string   `bson:"phone" json:"phone"`
	Branch   string   `bson:"branch" json:"branch"`
}

func NewUser(username string, password string, email string, role UserRole, phone string, branch string) *User {
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
