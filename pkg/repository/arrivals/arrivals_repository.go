// Package arrivals is the repository for the admin arrivals/proposals domain
// (the `proposals` collection). It owns every mongo/bson interaction for the
// proposals controller, mirroring the suppliers/finance repositories: the
// controller holds an *ArrivalsRepository and its handlers call these methods
// instead of touching the collection directly. Non-Mongo concerns (S3/local
// image-file storage, the invoice forward-proxy, PDF rendering) intentionally
// remain in the controller — only the document reads/writes (including the
// image_file field update) live here.
package arrivals

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
)

// ArrivalsRepository owns the proposals collection.
type ArrivalsRepository struct {
	proposals *mongo.Collection
}

// New builds the repository.
func New(db *mongo.Database) *ArrivalsRepository {
	return &ArrivalsRepository{
		proposals: db.Collection("proposals"),
	}
}

// ProposalQueryParams holds the optional filters the GetProposals handler
// applies. Empty string fields are skipped. Fulfilled is a pointer so the
// caller distinguishes "no filter" from an explicit true/false. DateFrom and
// DateTo are inclusive bounds already parsed by the caller; a nil bound lets
// the repository fall back to the controller's documented defaults (last 30
// days for the lower bound, now for the upper bound).
type ProposalQueryParams struct {
	Name      string
	Branch    string
	Fulfilled *bool
	DateFrom  *time.Time
	DateTo    *time.Time
}

// Find returns proposals matching the query, replicating the controller's
// filter construction exactly (case-insensitive regex on name/branch, a
// fulfilled flag, and a date range with the same default bounds).
func (r *ArrivalsRepository) Find(ctx context.Context, q ProposalQueryParams) ([]models.ProductProposal, error) {
	filter := bson.M{}

	if q.Name != "" {
		filter["name"] = bson.M{"$regex": regexp.QuoteMeta(q.Name), "$options": "i"}
	}
	if q.Branch != "" {
		filter["branch"] = bson.M{"$regex": regexp.QuoteMeta(q.Branch), "$options": "i"}
	}
	if q.Fulfilled != nil {
		filter["fulfilled"] = *q.Fulfilled
	}

	// Lower date bound: provided date_from, else 30 days ago.
	if q.DateFrom != nil {
		filter["date"] = bson.M{"$gte": *q.DateFrom}
	} else {
		filter["date"] = bson.M{"$gte": time.Now().Add(-30 * 24 * time.Hour)}
	}

	// Upper date bound: provided date_to (caller adds the +1 day), else now.
	if q.DateTo != nil {
		if _, exists := filter["date"]; exists {
			filter["date"].(bson.M)["$lte"] = *q.DateTo
		} else {
			filter["date"] = bson.M{"$lte": *q.DateTo}
		}
	} else {
		if _, exists := filter["date"]; exists {
			filter["date"].(bson.M)["$lte"] = time.Now()
		} else {
			filter["date"] = bson.M{"$lte": time.Now()}
		}
	}

	cursor, err := r.proposals.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	proposals := []models.ProductProposal{}
	if err := cursor.All(ctx, &proposals); err != nil {
		return nil, err
	}
	return proposals, nil
}

// FindUnfulfilled returns every proposal whose fulfilled flag is false, used by
// the PDF generation handler.
func (r *ArrivalsRepository) FindUnfulfilled(ctx context.Context) ([]models.ProductProposal, error) {
	cursor, err := r.proposals.Find(ctx, bson.M{"fulfilled": false})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	proposals := []models.ProductProposal{}
	if err := cursor.All(ctx, &proposals); err != nil {
		return nil, err
	}
	return proposals, nil
}

// GetByID returns a single proposal by its hex ObjectID. A malformed hex string
// yields repoerr.ErrInvalidInput; a missing document repoerr.ErrNotFound.
func (r *ArrivalsRepository) GetByID(ctx context.Context, hexID string) (*models.ProductProposal, error) {
	oid, err := bson.ObjectIDFromHex(hexID)
	if err != nil {
		return nil, repoerr.ErrInvalidInput
	}
	proposal := &models.ProductProposal{}
	err = r.proposals.FindOne(ctx, bson.M{"_id": oid}).Decode(proposal)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return proposal, nil
}

// CreateMany inserts one proposal per name under the given branch (all freshly
// dated, unfulfilled) and returns the inserted documents' hex ObjectIDs in
// insertion order.
func (r *ArrivalsRepository) CreateMany(ctx context.Context, branch string, names []string) ([]string, error) {
	now := time.Now()

	var proposalsToInsert []interface{}
	for _, name := range names {
		proposal := models.ProductProposal{
			ID:        bson.NewObjectID(),
			Name:      name,
			Date:      now,
			Branch:    branch,
			Fulfilled: false,
		}
		proposalsToInsert = append(proposalsToInsert, proposal)
	}

	result, err := r.proposals.InsertMany(ctx, proposalsToInsert)
	if err != nil {
		return nil, err
	}

	var ids []string
	for _, id := range result.InsertedIDs {
		ids = append(ids, id.(bson.ObjectID).Hex())
	}
	return ids, nil
}

// ProposalUpdate carries the editable fields of a proposal. Empty strings and a
// nil Fulfilled pointer are skipped, matching the controller's partial-update
// semantics.
type ProposalUpdate struct {
	Name      string
	Branch    string
	Fulfilled *bool
}

// Update patches the editable fields of a proposal identified by hex ObjectID.
// A malformed hex string yields repoerr.ErrInvalidInput. The controller's
// UpdateOne did not assert a matched count, so this method preserves that
// behaviour and does not return ErrNotFound on a no-op match.
func (r *ArrivalsRepository) Update(ctx context.Context, hexID string, in ProposalUpdate) error {
	oid, err := bson.ObjectIDFromHex(hexID)
	if err != nil {
		return repoerr.ErrInvalidInput
	}

	set := bson.M{}
	if in.Name != "" {
		set["name"] = in.Name
	}
	if in.Branch != "" {
		set["branch"] = in.Branch
	}
	if in.Fulfilled != nil {
		set["fulfilled"] = *in.Fulfilled
	}

	_, err = r.proposals.UpdateOne(ctx, bson.M{"_id": oid}, bson.M{"$set": set})
	return err
}

// SetImageFile records the stored image path on the proposal document. The path
// itself is produced by the controller's file-storage logic; only the document
// write lives here. A malformed hex string yields repoerr.ErrInvalidInput.
func (r *ArrivalsRepository) SetImageFile(ctx context.Context, hexID, imageFile string) error {
	oid, err := bson.ObjectIDFromHex(hexID)
	if err != nil {
		return repoerr.ErrInvalidInput
	}
	_, err = r.proposals.UpdateOne(
		ctx,
		bson.M{"_id": oid},
		bson.M{"$set": bson.M{"image_file": imageFile}},
	)
	return err
}

// Delete removes a proposal by its hex ObjectID. A malformed hex string yields
// repoerr.ErrInvalidInput; a delete that matched nothing repoerr.ErrNotFound.
func (r *ArrivalsRepository) Delete(ctx context.Context, hexID string) error {
	oid, err := bson.ObjectIDFromHex(hexID)
	if err != nil {
		return repoerr.ErrInvalidInput
	}
	res, err := r.proposals.DeleteOne(ctx, bson.M{"_id": oid})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// Fulfill marks the given proposals as fulfilled, scoped to a branch. Hex IDs
// that fail to parse are skipped (mirroring the controller, which continues on
// a bad id). Returns the matched and modified counts; an empty objectIDs set —
// i.e. no parseable ids — yields repoerr.ErrInvalidInput. Note the filter
// requires both _id in ids and branch == branch, exactly as the controller did.
func (r *ArrivalsRepository) Fulfill(ctx context.Context, branch string, hexIDs []string) (matched int64, modified int64, err error) {
	var objectIDs []bson.ObjectID
	for _, id := range hexIDs {
		oid, perr := bson.ObjectIDFromHex(id)
		if perr != nil {
			continue
		}
		objectIDs = append(objectIDs, oid)
	}
	if len(objectIDs) == 0 {
		return 0, 0, repoerr.ErrInvalidInput
	}

	result, err := r.proposals.UpdateMany(
		ctx,
		bson.M{
			"_id":    bson.M{"$in": objectIDs},
			"branch": branch,
		},
		bson.M{"$set": bson.M{"fulfilled": true}},
	)
	if err != nil {
		return 0, 0, err
	}
	return result.MatchedCount, result.ModifiedCount, nil
}
