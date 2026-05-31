// Package finance is the repository for the admin finance domain (per-branch
// balances in the `finance` collection). It owns all mongo/bson for the finance
// controller, mirroring the store/suppliers repositories.
package finance

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// FinanceRepository owns the finance collection.
type FinanceRepository struct {
	coll *mongo.Collection
}

// New builds the repository and ensures the branch indexes exist.
func New(db *mongo.Database) *FinanceRepository {
	coll := db.Collection("finance")
	_, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.M{"branch_id": 1},
	})
	_, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys:    bson.M{"branch_name": 1},
		Options: options.Index().SetUnique(true),
	})
	return &FinanceRepository{coll: coll}
}

// GetAllBranches returns every branch finance document (possibly empty).
func (r *FinanceRepository) GetAllBranches(ctx context.Context) ([]models.BranchFinance, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	branches := []models.BranchFinance{}
	if err := cursor.All(ctx, &branches); err != nil {
		return nil, err
	}
	return branches, nil
}

// GetByBranchID returns one branch by its branch id, or repoerr.ErrNotFound.
func (r *FinanceRepository) GetByBranchID(ctx context.Context, branchID string) (*models.BranchFinance, error) {
	return r.findOne(ctx, bson.M{"branch_id": branchID})
}

// GetByBranchName returns one branch by name (case-insensitive exact match), or
// repoerr.ErrNotFound. The caller is expected to have already URL-decoded name.
func (r *FinanceRepository) GetByBranchName(ctx context.Context, name string) (*models.BranchFinance, error) {
	filter := bson.M{"branch_name": bson.M{"$regex": "^" + name + "$", "$options": "i"}}
	return r.findOne(ctx, filter)
}

// GetByObjectID returns one branch by its Mongo _id (hex ObjectID). A malformed
// hex string yields repoerr.ErrInvalidInput; a missing document repoerr.ErrNotFound.
func (r *FinanceRepository) GetByObjectID(ctx context.Context, hexID string) (*models.BranchFinance, error) {
	oid, err := bson.ObjectIDFromHex(hexID)
	if err != nil {
		return nil, repoerr.ErrInvalidInput
	}
	return r.findOne(ctx, bson.M{"_id": oid})
}

func (r *FinanceRepository) findOne(ctx context.Context, filter bson.M) (*models.BranchFinance, error) {
	branch := &models.BranchFinance{}
	err := r.coll.FindOne(ctx, filter).Decode(branch)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return branch, nil
}

// CreateBranch creates a new branch finance document with zeroed balances and a
// fresh branch id. A duplicate branch name surfaces as the raw write error
// (mapped to HTTP 500 by the controller), matching the documented behaviour.
func (r *FinanceRepository) CreateBranch(ctx context.Context, in models.NewBranchFinanceInput) (*models.BranchFinance, error) {
	branch := models.BranchFinance{
		BranchID:   uuid.New().String(),
		BranchName: in.BranchName,
		Details:    in.Details,
		Finance: models.Finance{
			Balance:       models.Balance{Cash: 0, Bank: 0, MobileApps: 0},
			TotalIncome:   0,
			TotalExpenses: 0,
			Debt:          0,
		},
		Suppliers: []string{},
	}
	if _, err := r.coll.InsertOne(ctx, branch); err != nil {
		return nil, err
	}
	log.Debug().Str("branch_id", branch.BranchID).Msg("Created new branch finance")
	return &branch, nil
}
