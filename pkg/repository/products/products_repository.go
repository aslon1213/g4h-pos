// Package products is the repository for the admin products domain. It owns
// every mongo/bson interaction for products, mirroring the suppliers/finance
// repositories: the controller holds a *ProductsRepository and its handlers
// call these methods instead of touching collections directly. Non-Mongo I/O
// (S3 image storage) stays in the controller; this repository only mutates the
// product document's image array in Mongo.
package products

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/suppliers"
	"github.com/aslon1213/g4h_pos_erp/platform/database"

	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ProductsRepository owns the products, transactions, finance and suppliers
// collections. The latter three are needed for the income/distribution flow,
// which reflects a supplier transaction on the branch finance and the supplier
// via suppliers.NewSupplierTransaction.
type ProductsRepository struct {
	products     *mongo.Collection
	transactions *mongo.Collection
	finance      *mongo.Collection
	suppliers    *mongo.Collection
}

// New builds the repository.
func New(db *mongo.Database) *ProductsRepository {
	return &ProductsRepository{
		products:     db.Collection("products"),
		transactions: db.Collection("transactions"),
		finance:      db.Collection("finance"),
		suppliers:    db.Collection("suppliers"),
	}
}

// Query returns products matching the params. Filters preserve the controller's
// original aggregation semantics byte-for-byte (regex name, branch-scoped
// distribution id, sku, and price min/max on the distribution price).
func (r *ProductsRepository) Query(ctx context.Context, params models.ProductQueryParams) ([]models.Product, error) {
	pipeline := bson.D{}
	if params.Name != "" {
		pipeline = append(pipeline, bson.E{Key: "name", Value: bson.M{"$regex": params.Name, "$options": "i"}})
	}
	if params.BranchID != "" {
		pipeline = append(pipeline, bson.E{Key: "quantity_distribution.place.id", Value: params.BranchID})
	}
	if params.SKU != "" {
		pipeline = append(pipeline, bson.E{Key: "sku", Value: params.SKU})
	}
	if params.PriceMin != 0 {
		pipeline = append(pipeline, bson.E{Key: "quantity_distribution.price", Value: bson.E{Key: "$gte", Value: params.PriceMin}})
	}
	if params.PriceMax != 0 {
		pipeline = append(pipeline, bson.E{Key: "quantity_distribution.price", Value: bson.E{Key: "$lte", Value: params.PriceMax}})
	}

	cursor, err := r.products.Find(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	products := []models.Product{}
	if err := cursor.All(ctx, &products); err != nil {
		return nil, err
	}
	return products, nil
}

// GetByID returns a single product, or repoerr.ErrNotFound.
func (r *ProductsRepository) GetByID(ctx context.Context, id string) (*models.Product, error) {
	product := &models.Product{}
	err := r.products.FindOne(ctx, bson.M{"_id": id}).Decode(product)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return product, nil
}

// Create inserts a new product (built from base with zeroed distribution,
// images and income history) and returns it.
func (r *ProductsRepository) Create(ctx context.Context, base *models.ProductBase) (*models.Product, error) {
	product := models.NewProduct(base)
	if _, err := r.products.InsertOne(ctx, product); err != nil {
		return nil, err
	}
	return product, nil
}

// Update patches the editable fields of a product and returns the updated
// document. Only non-zero fields of base are set, matching the controller.
// Returns repoerr.ErrNotFound when no product matches the id.
func (r *ProductsRepository) Update(ctx context.Context, id string, base *models.ProductBase) (*models.Product, error) {
	set := bson.M{}
	if base.Name != "" {
		set["name"] = base.Name
	}
	if base.Description != "" {
		set["description"] = base.Description
	}
	if base.Manufacturer.Name != "" {
		set["manufacturer.name"] = base.Manufacturer.Name
	}
	if base.Manufacturer.Country != "" {
		set["manufacturer.country"] = base.Manufacturer.Country
	}
	if base.Manufacturer.Address != "" {
		set["manufacturer.address"] = base.Manufacturer.Address
	}
	if base.Manufacturer.Phone != "" {
		set["manufacturer.phone"] = base.Manufacturer.Phone
	}
	if base.Manufacturer.Email != "" {
		set["manufacturer.email"] = base.Manufacturer.Email
	}
	if base.Category != nil {
		set["category"] = base.Category
	}
	if base.SKU != "" {
		set["sku"] = base.SKU
	}
	if base.MinimumStockAlert != 0 {
		set["minimum_stock_alert"] = base.MinimumStockAlert
	}

	if _, err := r.products.UpdateOne(ctx, bson.M{"_id": id}, bson.M{"$set": set}); err != nil {
		return nil, err
	}

	product := &models.Product{}
	err := r.products.FindOne(ctx, bson.M{"_id": id}).Decode(product)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return product, nil
}

// Delete removes a product. The controller historically ignores a missing
// document (returns 200 with the id), so Delete reports repoerr.ErrNotFound and
// lets the caller decide; callers that want the legacy behaviour can ignore it.
func (r *ProductsRepository) Delete(ctx context.Context, id string) error {
	res, err := r.products.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// AddImage pushes an S3 image key onto the product's images array. S3 upload
// itself stays in the controller; this only mutates Mongo. Returns
// repoerr.ErrNotFound when no product matches the id.
func (r *ProductsRepository) AddImage(ctx context.Context, productID, key string) error {
	res, err := r.products.UpdateOne(ctx,
		bson.M{"_id": productID},
		bson.M{"$push": bson.M{"images": key}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// RemoveImage pulls an S3 image key from the product's images array. S3 deletion
// stays in the controller. Returns repoerr.ErrNotFound when no product matches.
func (r *ProductsRepository) RemoveImage(ctx context.Context, productID, key string) error {
	res, err := r.products.UpdateOne(ctx,
		bson.M{"_id": productID},
		bson.M{"$pull": bson.M{"images": key}},
	)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// GetIncomeHistory returns the recorded income history of a product, or
// repoerr.ErrNotFound when the product does not exist.
func (r *ProductsRepository) GetIncomeHistory(ctx context.Context, productID string) ([]models.IncomeHistory, error) {
	product := &models.Product{}
	err := r.products.FindOne(ctx, bson.M{"_id": productID}).Decode(product)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return product.IncomeHistory, nil
}

// AddIncome records new income for a product inside a MongoDB multi-document
// transaction (requires a replica set): it updates the per-branch quantity
// distribution (incrementing quantity and setting the selling price on a
// matching place, or pushing a new distribution otherwise) and applies a
// supplier debit transaction via suppliers.NewSupplierTransaction, reflecting it
// on the branch finance and the supplier. The price/quantity money math and all
// filters/updates mirror the controller byte-for-byte. Returns the product as it
// was read before the distribution writes (matching the controller's response)
// and repoerr.ErrNotFound when the product does not exist.
func (r *ProductsRepository) AddIncome(ctx context.Context, productID string, input *models.IncomeHistory, sellingPrice int32) (*models.Product, error) {
	session, sessCtx, err := database.StartTransaction(r.products.Database().Client())
	if err != nil {
		return nil, err
	}
	defer session.EndSession(sessCtx)

	// get the product
	product := &models.Product{}
	res := r.products.FindOne(sessCtx, bson.M{"_id": productID})
	if errors.Is(res.Err(), mongo.ErrNoDocuments) {
		_ = session.AbortTransaction(sessCtx)
		return nil, repoerr.ErrNotFound
	}
	if res.Err() != nil {
		_ = session.AbortTransaction(sessCtx)
		return nil, res.Err()
	}
	if err := res.Decode(product); err != nil {
		_ = session.AbortTransaction(sessCtx)
		return nil, err
	}

	// update the quantity distribution
	if len(product.QuantityDistribution) > 0 {
		for _, distribution := range product.QuantityDistribution {
			if distribution.Place.ID == input.UploadedTo.ID {
				filter := bson.M{"_id": productID, "quantity_distribution.place._id": input.UploadedTo.ID}
				update := bson.M{"$set": bson.M{"quantity_distribution.$.price": sellingPrice}, "$inc": bson.M{"quantity_distribution.$.quantity": input.Quantity}}
				if _, err := r.products.UpdateOne(sessCtx, filter, update); err != nil {
					_ = session.AbortTransaction(sessCtx)
					return nil, err
				}
			} else {
				if err := r.appendQuantityDistribution(sessCtx, productID, input, sellingPrice); err != nil {
					_ = session.AbortTransaction(sessCtx)
					return nil, err
				}
			}
		}
	} else {
		if err := r.appendQuantityDistribution(sessCtx, productID, input, sellingPrice); err != nil {
			_ = session.AbortTransaction(sessCtx)
			return nil, err
		}
	}

	// create supplier transaction
	transactionBase := models.TransactionBase{
		Amount:        uint32(input.Price * input.Quantity),
		Description:   "Income from " + input.SupplierID,
		Type:          models.TransactionTypeDebit,
		PaymentMethod: models.PaymentMethodUndefined,
	}

	supplierTransaction, err := suppliers.NewSupplierTransaction(
		sessCtx,
		transactionBase,
		input.SupplierID,
		input.UploadedTo.ID,
		r.transactions,
		r.finance,
		r.suppliers,
	)
	if err != nil {
		_ = session.AbortTransaction(sessCtx)
		return nil, err
	}
	log.Debug().Interface("supplier_transaction", supplierTransaction).Msg("Supplier transaction created")

	if err := session.CommitTransaction(sessCtx); err != nil {
		_ = session.AbortTransaction(sessCtx)
		return nil, err
	}

	log.Info().Str("product_id", productID).Msg("Successfully processed new income")
	return product, nil
}

// appendQuantityDistribution pushes a new per-place distribution onto the
// product within the active session context.
func (r *ProductsRepository) appendQuantityDistribution(ctx context.Context, productID string, input *models.IncomeHistory, sellingPrice int32) error {
	filter := bson.M{"_id": productID}
	update := bson.M{"$push": bson.M{"quantity_distribution": models.ProductDistribution{
		ProductQuantityInfo: models.ProductQuantityInfo{
			Quantity: input.Quantity,
		},
		Place: input.UploadedTo,
		Price: sellingPrice,
	}}}
	if _, err := r.products.UpdateOne(ctx, filter, update); err != nil {
		log.Error().Err(err).Msg("Failed to create new distribution")
		return err
	}
	return nil
}
