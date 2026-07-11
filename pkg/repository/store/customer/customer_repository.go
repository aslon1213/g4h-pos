// Package customer is the repository for storefront accounts (the
// `store_customers` table and its `addresses` child table). It owns all GORM
// access for the auth and account controllers; handlers call these methods and
// never touch the database directly.
package customer

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
)

// CustomerRepository owns the store_customers table (and the addresses child).
type CustomerRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *CustomerRepository {
	return &CustomerRepository{db: db}
}

// isUniqueViolation reports whether err is a Postgres unique-constraint (23505)
// violation.
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// Create inserts a new storefront customer. The id/timestamps are assigned here.
// Returns repoerr.ErrConflict if the email is already registered.
func (r *CustomerRepository) Create(ctx context.Context, customer *models.StoreCustomer) (*models.StoreCustomer, error) {
	now := time.Now()
	customer.ID = uuid.New().String()
	customer.CreatedAt = now
	customer.UpdatedAt = now
	if customer.Addresses == nil {
		customer.Addresses = []models.Address{}
	}
	if err := gorm.G[models.StoreCustomer](r.db).Create(ctx, customer); err != nil {
		if isUniqueViolation(err) {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	return customer, nil
}

// GetByID returns the customer (with addresses), or repoerr.ErrNotFound.
func (r *CustomerRepository) GetByID(ctx context.Context, id string) (*models.StoreCustomer, error) {
	return r.findOne(ctx, "id = ?", id)
}

// GetByEmail returns the customer matching the email, or repoerr.ErrNotFound.
func (r *CustomerRepository) GetByEmail(ctx context.Context, email string) (*models.StoreCustomer, error) {
	return r.findOne(ctx, "email = ?", email)
}

func (r *CustomerRepository) findOne(ctx context.Context, query string, args ...interface{}) (*models.StoreCustomer, error) {
	customer, err := gorm.G[models.StoreCustomer](r.db).Where(query, args...).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.attachAddresses(ctx, &customer)
	return &customer, nil
}

// attachAddresses loads the customer's address book onto the customer (best-effort).
func (r *CustomerRepository) attachAddresses(ctx context.Context, customer *models.StoreCustomer) {
	addrs, err := gorm.G[models.Address](r.db).Where("customer_id = ?", customer.ID).Order("id").Find(ctx)
	if err == nil {
		customer.Addresses = addrs
	}
	if customer.Addresses == nil {
		customer.Addresses = []models.Address{}
	}
}

// UpdateProfile patches the customer's name/phone and returns the fresh doc.
func (r *CustomerRepository) UpdateProfile(ctx context.Context, id string, in models.UpdateProfileInput) (*models.StoreCustomer, error) {
	set := map[string]interface{}{"updated_at": time.Now()}
	if in.Name != "" {
		set["name"] = in.Name
	}
	if in.Phone != "" {
		set["phone"] = in.Phone
	}
	res := r.db.WithContext(ctx).Model(&models.StoreCustomer{}).Where("id = ?", id).Updates(set)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, repoerr.ErrNotFound
	}
	return r.GetByID(ctx, id)
}

// UpdatePassword sets a new password hash for the customer.
func (r *CustomerRepository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	res := r.db.WithContext(ctx).Model(&models.StoreCustomer{}).Where("id = ?", id).Updates(map[string]interface{}{
		"password_hash": passwordHash,
		"updated_at":    time.Now(),
	})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// ---- address book ----

// GetAddresses returns the customer's saved addresses.
func (r *CustomerRepository) GetAddresses(ctx context.Context, customerID string) ([]models.Address, error) {
	customer, err := r.GetByID(ctx, customerID)
	if err != nil {
		return nil, err
	}
	return customer.Addresses, nil
}

// AddAddress appends an address (assigning it an id) and returns it. If the new
// address is flagged default, all others are cleared first.
func (r *CustomerRepository) AddAddress(ctx context.Context, customerID string, in models.AddressInput) (*models.Address, error) {
	if _, err := r.GetByID(ctx, customerID); err != nil {
		return nil, err
	}
	addr := models.Address{
		ID:         uuid.New().String(),
		CustomerID: customerID,
		Label:      in.Label,
		FullName:   in.FullName,
		Phone:      in.Phone,
		Line1:      in.Line1,
		Line2:      in.Line2,
		City:       in.City,
		Region:     in.Region,
		PostalCode: in.PostalCode,
		Country:    in.Country,
		IsDefault:  in.IsDefault,
	}
	if in.IsDefault {
		if err := r.clearDefault(ctx, customerID); err != nil {
			return nil, err
		}
	}
	if err := gorm.G[models.Address](r.db).Create(ctx, &addr); err != nil {
		return nil, err
	}
	r.touch(ctx, customerID)
	return &addr, nil
}

// UpdateAddress replaces the editable fields of one address in place.
func (r *CustomerRepository) UpdateAddress(ctx context.Context, customerID, addressID string, in models.AddressInput) error {
	if in.IsDefault {
		if err := r.clearDefault(ctx, customerID); err != nil {
			return err
		}
	}
	res := r.db.WithContext(ctx).Model(&models.Address{}).
		Where("id = ? AND customer_id = ?", addressID, customerID).
		Updates(map[string]interface{}{
			"label":       in.Label,
			"full_name":   in.FullName,
			"phone":       in.Phone,
			"line1":       in.Line1,
			"line2":       in.Line2,
			"city":        in.City,
			"region":      in.Region,
			"postal_code": in.PostalCode,
			"country":     in.Country,
			"is_default":  in.IsDefault,
		})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return repoerr.ErrNotFound
	}
	r.touch(ctx, customerID)
	return nil
}

// DeleteAddress removes one address from the address book.
func (r *CustomerRepository) DeleteAddress(ctx context.Context, customerID, addressID string) error {
	affected, err := gorm.G[models.Address](r.db).Where("id = ? AND customer_id = ?", addressID, customerID).Delete(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	r.touch(ctx, customerID)
	return nil
}

// SetDefaultAddress marks one address as default and clears the flag on the rest.
func (r *CustomerRepository) SetDefaultAddress(ctx context.Context, customerID, addressID string) error {
	if err := r.clearDefault(ctx, customerID); err != nil {
		return err
	}
	affected, err := gorm.G[models.Address](r.db).
		Where("id = ? AND customer_id = ?", addressID, customerID).
		Update(ctx, "is_default", true)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	r.touch(ctx, customerID)
	return nil
}

// clearDefault unsets is_default on every address of the customer.
func (r *CustomerRepository) clearDefault(ctx context.Context, customerID string) error {
	_, err := gorm.G[models.Address](r.db).Where("customer_id = ?", customerID).Update(ctx, "is_default", false)
	return err
}

// touch bumps the customer's updated_at after an address-book change (best-effort).
func (r *CustomerRepository) touch(ctx context.Context, customerID string) {
	_, _ = gorm.G[models.StoreCustomer](r.db).Where("id = ?", customerID).Update(ctx, "updated_at", time.Now())
}
