// Package arrivals is the repository for the admin arrivals/proposals domain
// (the `proposals` table). It owns every gorm/Postgres interaction for the
// proposals controller, mirroring the other ported repositories: the controller
// holds an *ArrivalsRepository and its handlers call these methods instead of
// touching the database directly. Non-DB concerns (S3/local image-file storage,
// the invoice forward-proxy, PDF rendering) intentionally remain in the
// controller — only the row reads/writes (including the image_file field
// update) live here.
package arrivals

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

// ArrivalsRepository owns the proposals table.
type ArrivalsRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *ArrivalsRepository {
	return &ArrivalsRepository{db: db}
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
// filter construction (case-insensitive contains on name/branch, a fulfilled
// flag, and a date range with the same default bounds).
func (r *ArrivalsRepository) Find(ctx context.Context, q ProposalQueryParams) ([]models.ProductProposal, error) {
	// Date bounds are always applied: provided date_from/date_to (the caller
	// adds the +1 day to date_to), else the last 30 days / now defaults.
	lower := time.Now().Add(-30 * 24 * time.Hour)
	if q.DateFrom != nil {
		lower = *q.DateFrom
	}
	upper := time.Now()
	if q.DateTo != nil {
		upper = *q.DateTo
	}

	query := gorm.G[models.ProductProposal](r.db).
		Where("date >= ?", lower).
		Where("date <= ?", upper)

	if q.Name != "" {
		query = query.Where("name ILIKE ?", "%"+q.Name+"%")
	}
	if q.Branch != "" {
		query = query.Where("branch ILIKE ?", "%"+q.Branch+"%")
	}
	if q.Fulfilled != nil {
		query = query.Where("fulfilled = ?", *q.Fulfilled)
	}

	proposals, err := query.Find(ctx)
	if err != nil {
		return nil, err
	}
	return proposals, nil
}

// FindUnfulfilled returns every proposal whose fulfilled flag is false, used by
// the PDF generation handler.
func (r *ArrivalsRepository) FindUnfulfilled(ctx context.Context) ([]models.ProductProposal, error) {
	proposals, err := gorm.G[models.ProductProposal](r.db).Where("fulfilled = ?", false).Find(ctx)
	if err != nil {
		return nil, err
	}
	return proposals, nil
}

// GetByID returns a single proposal by its id. A missing row yields
// repoerr.ErrNotFound.
func (r *ArrivalsRepository) GetByID(ctx context.Context, id string) (*models.ProductProposal, error) {
	proposal, err := gorm.G[models.ProductProposal](r.db).Where("id = ?", id).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &proposal, nil
}

// CreateMany inserts one proposal per name under the given branch (all freshly
// dated, unfulfilled) and returns the inserted rows' ids in insertion order.
func (r *ArrivalsRepository) CreateMany(ctx context.Context, branch string, names []string) ([]string, error) {
	now := time.Now()

	proposals := make([]models.ProductProposal, 0, len(names))
	ids := make([]string, 0, len(names))
	for _, name := range names {
		id := uuid.New().String()
		proposals = append(proposals, models.ProductProposal{
			ID:        id,
			Name:      name,
			Date:      now,
			Branch:    branch,
			Fulfilled: false,
		})
		ids = append(ids, id)
	}
	if len(proposals) == 0 {
		return []string{}, nil
	}

	if err := gorm.G[models.ProductProposal](r.db).CreateInBatches(ctx, &proposals, 100); err != nil {
		return nil, err
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

// Update patches the editable fields of a proposal identified by id. The
// controller's original UpdateOne did not assert a matched count, so this method
// preserves that behaviour and does not return ErrNotFound on a no-op match.
func (r *ArrivalsRepository) Update(ctx context.Context, id string, in ProposalUpdate) error {
	set := map[string]interface{}{}
	if in.Name != "" {
		set["name"] = in.Name
	}
	if in.Branch != "" {
		set["branch"] = in.Branch
	}
	if in.Fulfilled != nil {
		set["fulfilled"] = *in.Fulfilled
	}
	if len(set) == 0 {
		return nil
	}

	return r.db.WithContext(ctx).Table("proposals").Where("id = ?", id).Updates(set).Error
}

// SetImageFile records the stored image path on the proposal row. The path
// itself is produced by the controller's file-storage logic; only the row write
// lives here.
func (r *ArrivalsRepository) SetImageFile(ctx context.Context, id, imageFile string) error {
	return r.db.WithContext(ctx).Table("proposals").Where("id = ?", id).Update("image_file", imageFile).Error
}

// Delete removes a proposal by its id. A delete that matched nothing yields
// repoerr.ErrNotFound.
func (r *ArrivalsRepository) Delete(ctx context.Context, id string) error {
	affected, err := gorm.G[models.ProductProposal](r.db).Where("id = ?", id).Delete(ctx)
	if err != nil {
		return err
	}
	if affected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// Fulfill marks the given proposals as fulfilled, scoped to a branch. Empty ids
// are skipped; when no usable id remains it yields repoerr.ErrInvalidInput. The
// filter requires both id IN ids and branch == branch, exactly as before.
// Postgres reports matched == modified for the UPDATE, so both counts are equal.
func (r *ArrivalsRepository) Fulfill(ctx context.Context, branch string, ids []string) (matched int64, modified int64, err error) {
	valid := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		valid = append(valid, id)
	}
	if len(valid) == 0 {
		return 0, 0, repoerr.ErrInvalidInput
	}

	affected, err := gorm.G[models.ProductProposal](r.db).
		Where("id IN ?", valid).
		Where("branch = ?", branch).
		Update(ctx, "fulfilled", true)
	if err != nil {
		return 0, 0, err
	}
	return int64(affected), int64(affected), nil
}
