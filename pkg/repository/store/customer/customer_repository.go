// Package customer is the repository for storefront accounts (the
// `store_customers` collection). It owns all mongo/bson for the auth and
// account controllers; handlers call these methods and never touch the driver.
package customer

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// CustomerRepository owns the store_customers collection.
type CustomerRepository struct {
	coll *mongo.Collection
}

// New builds the repository and ensures the unique email index exists.
func New(db *mongo.Database) *CustomerRepository {
	coll := db.Collection("store_customers")
	_, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.D{{Key: "email", Value: 1}},
		Options: options.Index().SetUnique(true),
	})
	return &CustomerRepository{coll: coll}
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
	if _, err := r.coll.InsertOne(ctx, customer); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return nil, repoerr.ErrConflict
		}
		return nil, err
	}
	return customer, nil
}

// GetByID returns the customer, or repoerr.ErrNotFound.
func (r *CustomerRepository) GetByID(ctx context.Context, id string) (*models.StoreCustomer, error) {
	return r.findOne(ctx, bson.M{"_id": id})
}

// GetByEmail returns the customer matching the email, or repoerr.ErrNotFound.
func (r *CustomerRepository) GetByEmail(ctx context.Context, email string) (*models.StoreCustomer, error) {
	return r.findOne(ctx, bson.M{"email": email})
}

func (r *CustomerRepository) findOne(ctx context.Context, filter bson.M) (*models.StoreCustomer, error) {
	customer := &models.StoreCustomer{}
	err := r.coll.FindOne(ctx, filter).Decode(customer)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return customer, nil
}

// UpdateProfile patches the customer's name/phone and returns the fresh doc.
func (r *CustomerRepository) UpdateProfile(ctx context.Context, id string, in models.UpdateProfileInput) (*models.StoreCustomer, error) {
	set := bson.M{"updated_at": time.Now()}
	if in.Name != "" {
		set["name"] = in.Name
	}
	if in.Phone != "" {
		set["phone"] = in.Phone
	}
	return r.findOneAndUpdate(ctx, bson.M{"_id": id}, bson.M{"$set": set})
}

// UpdatePassword sets a new password hash for the customer.
func (r *CustomerRepository) UpdatePassword(ctx context.Context, id, passwordHash string) error {
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": id}, bson.M{
		"$set": bson.M{"password_hash": passwordHash, "updated_at": time.Now()},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
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
	addr := models.Address{
		ID:         uuid.New().String(),
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
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": customerID}, bson.M{
		"$push": bson.M{"addresses": addr},
		"$set":  bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		return nil, err
	}
	if res.MatchedCount == 0 {
		return nil, repoerr.ErrNotFound
	}
	return &addr, nil
}

// UpdateAddress replaces the editable fields of one address in place.
func (r *CustomerRepository) UpdateAddress(ctx context.Context, customerID, addressID string, in models.AddressInput) error {
	if in.IsDefault {
		if err := r.clearDefault(ctx, customerID); err != nil {
			return err
		}
	}
	res, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": customerID, "addresses.id": addressID},
		bson.M{"$set": bson.M{
			"addresses.$.label":       in.Label,
			"addresses.$.full_name":   in.FullName,
			"addresses.$.phone":       in.Phone,
			"addresses.$.line1":       in.Line1,
			"addresses.$.line2":       in.Line2,
			"addresses.$.city":        in.City,
			"addresses.$.region":      in.Region,
			"addresses.$.postal_code": in.PostalCode,
			"addresses.$.country":     in.Country,
			"addresses.$.is_default":  in.IsDefault,
			"updated_at":              time.Now(),
		}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// DeleteAddress removes one address from the address book.
func (r *CustomerRepository) DeleteAddress(ctx context.Context, customerID, addressID string) error {
	res, err := r.coll.UpdateOne(ctx, bson.M{"_id": customerID}, bson.M{
		"$pull": bson.M{"addresses": bson.M{"id": addressID}},
		"$set":  bson.M{"updated_at": time.Now()},
	})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// SetDefaultAddress marks one address as default and clears the flag on the rest.
func (r *CustomerRepository) SetDefaultAddress(ctx context.Context, customerID, addressID string) error {
	if err := r.clearDefault(ctx, customerID); err != nil {
		return err
	}
	res, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": customerID, "addresses.id": addressID},
		bson.M{"$set": bson.M{"addresses.$.is_default": true, "updated_at": time.Now()}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// clearDefault unsets is_default on every address of the customer.
func (r *CustomerRepository) clearDefault(ctx context.Context, customerID string) error {
	_, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": customerID},
		bson.M{"$set": bson.M{"addresses.$[].is_default": false}},
	)
	return err
}

func (r *CustomerRepository) findOneAndUpdate(ctx context.Context, filter, update bson.M) (*models.StoreCustomer, error) {
	customer := &models.StoreCustomer{}
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := r.coll.FindOneAndUpdate(ctx, filter, update, opts).Decode(customer)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return customer, nil
}
