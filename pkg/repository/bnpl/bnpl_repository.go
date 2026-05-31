// Package bnpl is the repository for the admin BNPL (buy-now-pay-later) domain.
// BNPLs are embedded on the `customers` documents; a credit payment additionally
// records a transaction and reflects it on the owning branch's finance balances.
// It owns every mongo/bson interaction for the BNPL controller, mirroring the
// suppliers and finance repositories: the controller holds a *BNPLRepository and
// its handlers call these methods instead of touching collections directly.
package bnpl

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/aslon1213/g4h_pos_erp/platform/database"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// BNPLRepository owns the customers, transactions and finance collections.
// BNPLs live as an array on each customer document; transactions and finance are
// needed when a credit payment is recorded against a BNPL.
type BNPLRepository struct {
	customers    *mongo.Collection
	transactions *mongo.Collection
	finance      *mongo.Collection
}

// New builds the repository.
func New(db *mongo.Database) *BNPLRepository {
	return &BNPLRepository{
		customers:    db.Collection("customers"),
		transactions: db.Collection("transactions"),
		finance:      db.Collection("finance"),
	}
}

// CustomerExists reports whether a customer with the given id exists, returning
// repoerr.ErrNotFound when it does not.
func (r *BNPLRepository) CustomerExists(ctx context.Context, customerID string) error {
	customer := &models.Customer{}
	err := r.customers.FindOne(ctx, bson.M{"_id": customerID}).Decode(customer)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return repoerr.ErrNotFound
	}
	if err != nil {
		return err
	}
	return nil
}

// Create builds a new BNPL (with the provided total amount) and pushes it onto
// the customer's `bnpls` array. The caller is responsible for resolving the
// total amount; this method only persists. Returns the created BNPL.
func (r *BNPLRepository) Create(ctx context.Context, customerID, branchID string, totalAmount int32, products map[string]models.SalesSessionItem) (*models.BNPL, error) {
	bnpl := &models.BNPL{
		ID:           uuid.New().String(),
		CustomerID:   customerID,
		TotalAmount:  totalAmount,
		PaidAmount:   0,
		Products:     products,
		Status:       models.BNPLStatusActive,
		Transactions: []string{},
		BranchID:     branchID,
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}

	_, err := r.customers.UpdateOne(ctx, bson.M{"_id": customerID}, bson.M{"$push": bson.M{"bnpls": bnpl}})
	if err != nil {
		return nil, err
	}
	return bnpl, nil
}

// GetByID returns a single BNPL by its id, extracted from whichever customer
// holds it. Mirrors the controller's aggregation pipeline exactly.
func (r *BNPLRepository) GetByID(ctx context.Context, bnplID string) (*models.BNPL, error) {
	log.Debug().Str("bnpl_id", bnplID).Msg("Getting BNPL from database")

	pipeline := mongo.Pipeline{
		bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "bnpls.id", Value: bnplID},
			}},
		},
		bson.D{
			{Key: "$project", Value: bson.D{
				{Key: "bnpl", Value: bson.D{
					{Key: "$first", Value: bson.D{
						{Key: `$filter`, Value: bson.D{
							{Key: "input", Value: `$bnpls`},
							{Key: "as", Value: "b"},
							{Key: "cond", Value: bson.D{
								{Key: "$eq", Value: bson.A{"$$b.id", bnplID}},
							}},
						}},
					}},
				}},
				{Key: "_id", Value: 0},
			}},
		},
	}

	cursor, err := r.customers.Aggregate(ctx, pipeline)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnplID).Msg("Failed to find BNPL in database")
		return nil, err
	}

	type PipelineResult struct {
		BNPL models.BNPL `bson:"bnpl"`
	}
	var output []PipelineResult

	err = cursor.All(ctx, &output)
	if err != nil {
		log.Error().Err(err).Str("bnpl_id", bnplID).Msg("Failed to decode BNPL from database")
		return nil, err
	}

	if len(output) == 0 {
		return nil, repoerr.ErrNotFound
	}

	log.Debug().Interface("output", output).Str("bnpl_id", bnplID).Msg("Successfully retrieved BNPL from database")
	return &output[0].BNPL, nil
}

// Delete pulls the BNPL with the given id off whichever customer holds it.
func (r *BNPLRepository) Delete(ctx context.Context, bnplID string) error {
	_, err := r.customers.UpdateOne(
		ctx,
		bson.M{
			"bnpls.id": bnplID,
		},
		bson.M{
			"$pull": bson.M{
				"bnpls": bson.M{"id": bnplID},
			},
		},
	)
	if err != nil {
		return err
	}
	return nil
}

// GetCustomerBNPLs returns the BNPLs embedded on a customer. When branchID is
// non-empty the customer is additionally required to hold a BNPL on that branch
// (mirroring the controller filter). Returns repoerr.ErrNotFound when no
// matching customer exists.
func (r *BNPLRepository) GetCustomerBNPLs(ctx context.Context, customerID, branchID string) ([]models.BNPL, error) {
	customer := &models.Customer{}
	query := bson.M{"_id": customerID}
	if branchID != "" {
		query["bnpls.branch_id"] = branchID
	}
	err := r.customers.FindOne(ctx, query).Decode(customer)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return customer.BNPLs, nil
}

// GetBranchBNPLs returns the customers holding BNPLs on the given branch,
// optionally filtered by customer name/phone/address (case-insensitive regex).
// Mirrors the controller's aggregation pipeline exactly.
func (r *BNPLRepository) GetBranchBNPLs(ctx context.Context, branchID, customerName, customerPhone, customerAddress string) ([]models.Customer, error) {
	query := mongo.Pipeline{
		bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "bnpls.branch_id", Value: branchID},
			}},
		},
	}
	if customerName != "" {
		query = append(query, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "name", Value: bson.D{
					{Key: "$regex", Value: customerName},
					{Key: "$options", Value: "i"},
				}},
			}},
		})
	}
	if customerPhone != "" {
		query = append(query, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "phone", Value: bson.D{
					{Key: "$regex", Value: customerPhone},
					{Key: "$options", Value: "i"},
				}},
			}},
		})
	}
	if customerAddress != "" {
		query = append(query, bson.D{
			{Key: "$match", Value: bson.D{
				{Key: "address", Value: bson.D{
					{Key: "$regex", Value: customerAddress},
					{Key: "$options", Value: "i"},
				}},
			}},
		})
	}

	cursor, err := r.customers.Aggregate(ctx, query)
	if err != nil {
		return nil, err
	}
	var output []models.Customer
	if err := cursor.All(ctx, &output); err != nil {
		return nil, err
	}
	return output, nil
}

// Credit applies a credit payment of `amount` (in the given payment method)
// against the BNPL identified by bnplID, atomically in a MongoDB multi-document
// transaction (requires a replica set). It records a transaction, increments the
// owning branch's cash balance, and updates the embedded BNPL's paid amount and
// status (completed when fully paid, otherwise active). Returns the updated BNPL.
//
// Money/accounting math and filters are preserved byte-for-byte from the
// controller; a missing branch surfaces as repoerr.ErrNotFound.
func (r *BNPLRepository) Credit(ctx context.Context, bnplID string, amount int, paymentMethod string) (*models.BNPL, error) {
	session, sessCtx, err := database.StartTransaction(r.customers.Database().Client())
	if err != nil {
		return nil, err
	}
	defer session.EndSession(sessCtx)

	transaction := models.TransactionBase{
		Amount:        uint32(amount),
		Description:   "Credit BNPL",
		Type:          models.TransactionTypeCredit,
		PaymentMethod: models.PaymentMethod(paymentMethod),
	}

	// get the BNPL from the customers collection
	bnpl, err := r.GetByID(sessCtx, bnplID)
	if err != nil {
		session.AbortTransaction(sessCtx)
		return nil, err
	}

	trx := models.NewTransaction(&transaction, models.InitiatorTypeBNPL, bnpl.BranchID)
	if _, err := r.transactions.InsertOne(sessCtx, trx); err != nil {
		session.AbortTransaction(sessCtx)
		return nil, err
	}
	trx_id := trx.ID
	bnpl.Transactions = append(bnpl.Transactions, trx_id)

	// update finance of branch
	log.Info().Str("branch_id", bnpl.BranchID).Msg("Updating branch finance")
	update_res, err := r.finance.UpdateOne(
		sessCtx,
		bson.M{"branch_id": bnpl.BranchID},
		bson.M{"$inc": bson.M{"finance.balance.cash": amount}},
	)
	if err != nil {
		session.AbortTransaction(sessCtx)
		return nil, err
	}
	if update_res.MatchedCount == 0 {
		log.Error().Str("branch_id", bnpl.BranchID).Msg("Branch not found")
		session.AbortTransaction(sessCtx)
		return nil, repoerr.ErrNotFound
	}

	total_paid_amount := bnpl.PaidAmount + int32(amount)
	bnpl.UpdatedAt = time.Now()
	bnpl.PaidAmount = total_paid_amount
	bnpl.Transactions = append(bnpl.Transactions, trx_id)

	if total_paid_amount >= bnpl.TotalAmount {
		log.Info().Msg("BNPL payment completed")
		bnpl.Status = models.BNPLStatusCompleted

		update_res, err = r.customers.UpdateOne(sessCtx, bson.M{"bnpls.id": bnplID}, bson.M{
			"$set":  bson.M{"bnpls.$.paid_amount": total_paid_amount, "bnpls.$.updated_at": time.Now(), "bnpls.$.status": models.BNPLStatusCompleted},
			"$push": bson.M{"bnpls.$.transactions": trx_id},
		})
		if err != nil || update_res.MatchedCount == 0 {
			session.AbortTransaction(sessCtx)
			if err != nil {
				return nil, err
			}
			return nil, repoerr.ErrNotFound
		}
	} else {
		log.Info().Msg("Updating BNPL payment amount")
		update_res, err = r.customers.UpdateOne(sessCtx, bson.M{"bnpls.id": bnplID}, bson.M{
			"$set":  bson.M{"bnpls.$.paid_amount": total_paid_amount, "bnpls.$.updated_at": time.Now(), "bnpls.$.status": models.BNPLStatusActive},
			"$push": bson.M{"bnpls.$.transactions": trx_id},
		})
		if err != nil || update_res.MatchedCount == 0 {
			session.AbortTransaction(sessCtx)
			if err != nil {
				return nil, err
			}
			return nil, repoerr.ErrNotFound
		}
	}

	session.CommitTransaction(sessCtx)
	log.Info().Str("bnpl_id", bnplID).Msg("Successfully processed BNPL payment")
	return bnpl, nil
}
