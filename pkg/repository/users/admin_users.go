package users_repository

import (
	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"gorm.io/gorm"
)

// AdminUserRepository provides CRUD operations for admin users
type AdminUserRepository struct {
	db *gorm.DB
}

func NewAdminUserRepository(db *gorm.DB) *AdminUserRepository {
	return &AdminUserRepository{
		db: db,
	}
}

// Create inserts a new admin user
func (r *AdminUserRepository) Create(user *models.User) error {
	return r.db.Create(user).Error
}

// GetByID retrieves an admin user by ID
func (r *AdminUserRepository) GetByID(id string) (*models.User, error) {
	var user models.User
	err := r.db.Where("id = ?", id).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// GetByID retrieves an admin user by ID
func (r *AdminUserRepository) GetByName(name string) (*models.User, error) {
	var user models.User
	err := r.db.Where("username = ?", name).First(&user).Error
	if err != nil {
		return nil, err
	}
	return &user, nil
}

// Update modifies an existing admin user
func (r *AdminUserRepository) Update(user *models.User) error {
	return r.db.Save(user).Error
}

// Delete removes an admin user by ID
func (r *AdminUserRepository) Delete(id string) error {
	return r.db.Delete(&models.User{}, "id = ?", id).Error
}

// List returns all admin users
func (r *AdminUserRepository) List() ([]models.User, error) {
	var users []models.User
	err := r.db.Find(&users).Error
	return users, err
}
