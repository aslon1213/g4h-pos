// Package bnpl is the repository for the admin BNPL (buy-now-pay-later) domain.
// Under Postgres a BNPL is its own row in the `bnpls` table (no longer embedded
// on the customer): its products live in `bnpl_products` and its payments are
// `transactions` rows carrying the bnpl_id foreign key. A credit payment records
// a transaction and reflects it on the owning branch's finance balances. It owns
// every gorm/Postgres interaction for the BNPL controller, mirroring the other
// ported repositories: the controller holds a *BNPLRepository and its handlers
// call these methods instead of touching the database directly.
package bnpl

import (
	"context"
	"errors"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/google/uuid"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// BNPLRepository owns the bnpls, bnpl_products, transactions and branch_finance
// tables. branch_finance is touched when a credit payment is recorded.
type BNPLRepository struct {
	db *gorm.DB
}

// New builds the repository.
func New(db *gorm.DB) *BNPLRepository {
	return &BNPLRepository{db: db}
}

// CustomerExists reports whether a customer with the given id exists, returning
// repoerr.ErrNotFound when it does not.
func (r *BNPLRepository) CustomerExists(ctx context.Context, customerID string) error {
	count, err := gorm.G[models.Customer](r.db).Where("id = ?", customerID).Count(ctx, "id")
	if err != nil {
		return err
	}
	if count == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}

// Create inserts a new BNPL row (with the provided total amount) plus one
// bnpl_products row per product, atomically in a single gorm transaction. The
// caller is responsible for resolving the total amount; this method only
// persists. Returns the created BNPL.
func (r *BNPLRepository) Create(ctx context.Context, customerID, branchID string, totalAmount int32, products map[string]models.SalesSessionItem) (*models.BNPL, error) {
	now := time.Now()
	bnpl := &models.BNPL{
		ID:           uuid.New().String(),
		CustomerID:   customerID,
		BranchID:     branchID,
		TotalAmount:  totalAmount,
		PaidAmount:   0,
		Products:     products,
		Status:       models.BNPLStatusActive,
		Transactions: []string{},
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	err := r.db.Transaction(func(tx *gorm.DB) error {
		if err := gorm.G[models.BNPL](tx).Create(ctx, bnpl); err != nil {
			return err
		}
		if len(products) > 0 {
			items := make([]models.BnplProduct, 0, len(products))
			for productID, item := range products {
				items = append(items, models.BnplProduct{
					BnplID:    bnpl.ID,
					ProductID: productID,
					Quantity:  item.Quantity,
					Price:     item.Price,
				})
			}
			if err := gorm.G[models.BnplProduct](tx).CreateInBatches(ctx, &items, len(items)); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	log.Info().Str("bnpl_id", bnpl.ID).Msg("Created new BNPL")
	return bnpl, nil
}

// GetByID returns a single BNPL by its id, with its products and transaction ids
// attached. Returns repoerr.ErrNotFound when no such BNPL exists.
func (r *BNPLRepository) GetByID(ctx context.Context, bnplID string) (*models.BNPL, error) {
	bnpl, err := gorm.G[models.BNPL](r.db).Where("id = ?", bnplID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := r.attachChildren(ctx, r.db, &bnpl); err != nil {
		return nil, err
	}
	return &bnpl, nil
}

// Delete removes the BNPL with the given id. Its bnpl_products rows cascade via
// the foreign key. Mirrors the original behaviour of not erroring when nothing
// matched.
func (r *BNPLRepository) Delete(ctx context.Context, bnplID string) error {
	if _, err := gorm.G[models.BNPL](r.db).Where("id = ?", bnplID).Delete(ctx); err != nil {
		return err
	}
	return nil
}

// GetCustomerBNPLs returns all BNPLs of a customer (each with products and
// transaction ids attached). When branchID is non-empty the customer is
// additionally required to hold a BNPL on that branch (mirroring the original
// filter). Returns repoerr.ErrNotFound when no matching customer/branch exists.
func (r *BNPLRepository) GetCustomerBNPLs(ctx context.Context, customerID, branchID string) ([]models.BNPL, error) {
	count, err := gorm.G[models.Customer](r.db).Where("id = ?", customerID).Count(ctx, "id")
	if err != nil {
		return nil, err
	}
	if count == 0 {
		return nil, repoerr.ErrNotFound
	}
	if branchID != "" {
		bcount, err := gorm.G[models.BNPL](r.db).
			Where("customer_id = ? AND branch_id = ?", customerID, branchID).Count(ctx, "id")
		if err != nil {
			return nil, err
		}
		if bcount == 0 {
			return nil, repoerr.ErrNotFound
		}
	}
	return r.bnplsForCustomer(ctx, r.db, customerID)
}

// GetBranchBNPLs returns the customers holding BNPLs on the given branch (each
// customer with its BNPLs attached), optionally filtered by customer
// name/phone/address (case-insensitive ILIKE substring match).
func (r *BNPLRepository) GetBranchBNPLs(ctx context.Context, branchID, customerName, customerPhone, customerAddress string) ([]models.Customer, error) {
	query := gorm.G[models.Customer](r.db).
		Where("id IN (SELECT customer_id FROM bnpls WHERE branch_id = ?)", branchID)
	if customerName != "" {
		query = query.Where("name ILIKE ?", "%"+customerName+"%")
	}
	if customerPhone != "" {
		query = query.Where("phone ILIKE ?", "%"+customerPhone+"%")
	}
	if customerAddress != "" {
		query = query.Where("address ILIKE ?", "%"+customerAddress+"%")
	}

	customers, err := query.Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range customers {
		bnpls, err := r.bnplsForCustomer(ctx, r.db, customers[i].ID)
		if err != nil {
			return nil, err
		}
		customers[i].BNPLs = bnpls
	}
	return customers, nil
}

// Credit applies a credit payment of `amount` (in the given payment method)
// against the BNPL identified by bnplID, atomically in a single gorm
// transaction. It records a transaction carrying the bnpl_id, increments the
// owning branch's cash balance, and updates the BNPL's paid amount and status
// (completed when fully paid, otherwise active). Returns the updated BNPL.
//
// The money math is preserved from the original: the branch's cash bucket is
// always the one credited, regardless of the payment method. A missing branch
// surfaces as repoerr.ErrNotFound.
func (r *BNPLRepository) Credit(ctx context.Context, bnplID string, amount int, paymentMethod string) (*models.BNPL, error) {
	var updated *models.BNPL

	err := r.db.Transaction(func(tx *gorm.DB) error {
		bnpl, err := gorm.G[models.BNPL](tx).Where("id = ?", bnplID).First(ctx)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return repoerr.ErrNotFound
		}
		if err != nil {
			return err
		}

		// record a transaction against this BNPL
		base := models.TransactionBase{
			Amount:        uint32(amount),
			Description:   "Credit BNPL",
			Type:          models.TransactionTypeCredit,
			PaymentMethod: models.PaymentMethod(paymentMethod),
		}
		trx := models.NewTransaction(&base, models.InitiatorTypeBNPL, bnpl.BranchID)
		bid := bnpl.ID
		trx.BNPLID = &bid
		if err := gorm.G[models.Transaction](tx).Create(ctx, trx); err != nil {
			return err
		}

		// reflect the payment on the owning branch's cash balance
		log.Info().Str("branch_id", bnpl.BranchID).Msg("Updating branch finance")
		res := tx.WithContext(ctx).Table("branch_finance").Where("branch_id = ?", bnpl.BranchID).
			Update("balance_cash", gorm.Expr("balance_cash + ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			log.Error().Str("branch_id", bnpl.BranchID).Msg("Branch not found")
			return repoerr.ErrNotFound
		}

		// update the BNPL paid amount + status
		totalPaid := bnpl.PaidAmount + int32(amount)
		status := models.BNPLStatusActive
		if totalPaid >= bnpl.TotalAmount {
			status = models.BNPLStatusCompleted
		}
		now := time.Now()
		res2 := tx.WithContext(ctx).Table("bnpls").Where("id = ?", bnplID).Updates(map[string]interface{}{
			"paid_amount": totalPaid,
			"status":      string(status),
			"updated_at":  now,
		})
		if res2.Error != nil {
			return res2.Error
		}
		if res2.RowsAffected == 0 {
			return repoerr.ErrNotFound
		}

		bnpl.PaidAmount = totalPaid
		bnpl.Status = status
		bnpl.UpdatedAt = now
		updated = &bnpl
		return nil
	})
	if err != nil {
		return nil, err
	}

	if err := r.attachChildren(ctx, r.db, updated); err != nil {
		return nil, err
	}
	log.Info().Str("bnpl_id", bnplID).Msg("Successfully processed BNPL payment")
	return updated, nil
}

// bnplsForCustomer loads all BNPLs of a customer, each with its products and
// transaction ids attached.
func (r *BNPLRepository) bnplsForCustomer(ctx context.Context, db *gorm.DB, customerID string) ([]models.BNPL, error) {
	bnpls, err := gorm.G[models.BNPL](db).Where("customer_id = ?", customerID).Order("created_at").Find(ctx)
	if err != nil {
		return nil, err
	}
	for i := range bnpls {
		if err := r.attachChildren(ctx, db, &bnpls[i]); err != nil {
			return nil, err
		}
	}
	return bnpls, nil
}

// attachChildren populates a BNPL's Products map (from bnpl_products) and its
// Transactions slice (the ids of transactions carrying its bnpl_id).
func (r *BNPLRepository) attachChildren(ctx context.Context, db *gorm.DB, bnpl *models.BNPL) error {
	products, err := gorm.G[models.BnplProduct](db).Where("bnpl_id = ?", bnpl.ID).Find(ctx)
	if err != nil {
		return err
	}
	bnpl.Products = make(map[string]models.SalesSessionItem, len(products))
	for _, p := range products {
		bnpl.Products[p.ProductID] = models.SalesSessionItem{Quantity: p.Quantity, Price: p.Price}
	}

	txns, err := gorm.G[models.Transaction](db).Select("id").Where("bnpl_id = ?", bnpl.ID).Order("created_at").Find(ctx)
	if err != nil {
		return err
	}
	bnpl.Transactions = make([]string, 0, len(txns))
	for _, t := range txns {
		bnpl.Transactions = append(bnpl.Transactions, t.ID)
	}
	return nil
}
