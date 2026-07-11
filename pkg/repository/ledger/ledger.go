// Package ledger holds the cross-domain money primitives — the atomic writes
// that move money between the transactions ledger, supplier balances and branch
// finance balances. They are centralized here (rather than in the suppliers /
// sales packages, as under Mongo) so every domain (suppliers, sales, journals,
// products, bnpl) can apply them inside its own gorm transaction without a
// cross-package dependency.
//
// Every function takes a *gorm.DB that is expected to be a transaction handle
// (from db.Transaction(func(tx *gorm.DB) error { ... })), so the ledger write
// and the caller's other writes commit atomically. The money math mirrors the
// original MongoDB implementation exactly; only the persistence changes.
package ledger

import (
	"context"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ApplySupplierTransaction records a supplier transaction and reflects it on both
// the supplier's balances and the owning branch's finance balances. Money
// semantics:
//   - credit (we paid the supplier): supplier balance +amount, branch debt -amount,
//     and the matching branch balance bucket is reduced by amount.
//   - debit (supplier delivered value): supplier balance -amount, branch debt +amount.
//
// Returns repoerr.ErrNotFound if the supplier does not exist.
func ApplySupplierTransaction(ctx context.Context, tx *gorm.DB, base models.TransactionBase, supplierID, branchID string) (*models.Transaction, error) {
	transaction := models.NewTransaction(&base, models.InitiatorTypeSupplier, branchID)
	sid := supplierID
	transaction.SupplierID = &sid

	if err := gorm.G[models.Transaction](tx).Create(ctx, transaction); err != nil {
		return nil, err
	}

	isCredit := transaction.TransactionBase.Type == models.TransactionTypeCredit
	amount := int32(transaction.Amount)

	// supplier balances
	supplierBalanceDelta := amount
	if !isCredit {
		supplierBalanceDelta = -amount
	}
	incIncome, incExpense := int32(0), int32(0)
	if isCredit {
		incIncome = amount
	} else {
		incExpense = amount
	}
	res := tx.Table("suppliers").Where("id = ?", supplierID).Updates(map[string]interface{}{
		"balance":        gorm.Expr("balance + ?", supplierBalanceDelta),
		"total_income":   gorm.Expr("total_income + ?", incIncome),
		"total_expenses": gorm.Expr("total_expenses + ?", incExpense),
	})
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, repoerr.ErrNotFound
	}

	// branch finance: debt and (on credit only) the matching balance bucket
	debtDelta := amount
	if isCredit {
		debtDelta = -amount
	}
	inc := map[string]interface{}{"debt": gorm.Expr("debt + ?", debtDelta)}
	if isCredit {
		outflow := -amount
		switch transaction.TransactionBase.PaymentMethod {
		case models.PaymentMethodCash:
			inc["balance_cash"] = gorm.Expr("balance_cash + ?", outflow)
		case models.PaymentMethodBank, models.OnlineMobileAppPayment:
			inc["balance_bank"] = gorm.Expr("balance_bank + ?", outflow)
		case models.OnlineTransfer:
			inc["balance_mobile_apps"] = gorm.Expr("balance_mobile_apps + ?", outflow)
		}
	}
	// mirrors the Mongo original: branch finance update is best-effort (no
	// not-found error) so a supplier without a finance row still records.
	if err := tx.Table("branch_finance").Where("branch_id = ?", branchID).Updates(inc).Error; err != nil {
		return nil, err
	}

	log.Info().Str("supplier_id", supplierID).Str("branch_id", branchID).Msg("Supplier transaction applied")
	return transaction, nil
}

// ApplySalesTransaction records a sales (POS income) transaction for a branch and
// increments the branch's matching balance bucket. The type is forced to credit
// and the payment method validated. Returns repoerr.ErrNotFound when the branch
// finance row does not exist.
func ApplySalesTransaction(ctx context.Context, tx *gorm.DB, base models.TransactionBase, branchID string) (*models.Transaction, error) {
	base.Type = models.TransactionTypeCredit
	if err := models.ValidatePaymentMethod(base.PaymentMethod); err != nil {
		return nil, err
	}

	transaction := models.NewTransaction(&base, models.InitiatorTypeSales, branchID)
	if err := gorm.G[models.Transaction](tx).Create(ctx, transaction); err != nil {
		return nil, err
	}

	if err := adjustBranchBalance(ctx, tx, branchID, base.PaymentMethod, int32(base.Amount)); err != nil {
		return nil, err
	}

	log.Info().Str("transaction_id", transaction.ID).Uint32("amount", transaction.Amount).
		Str("payment_method", string(transaction.PaymentMethod)).Msg("Sales transaction created")
	return transaction, nil
}

// DeleteSalesTransaction looks up a sales transaction, deletes it and reverses its
// effect on the branch balance. Returns repoerr.ErrNotFound when it does not exist.
func DeleteSalesTransaction(ctx context.Context, tx *gorm.DB, transactionID string) (*models.Transaction, error) {
	transaction, err := gorm.G[models.Transaction](tx).Where("id = ?", transactionID).First(ctx)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, repoerr.ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	affected, err := gorm.G[models.Transaction](tx).Where("id = ?", transactionID).Delete(ctx)
	if err != nil {
		return nil, err
	}
	if affected == 0 {
		return nil, repoerr.ErrNotFound
	}

	if err := adjustBranchBalance(ctx, tx, transaction.BranchID, transaction.PaymentMethod, -int32(transaction.Amount)); err != nil {
		return nil, err
	}
	return &transaction, nil
}

// adjustBranchBalance adds delta (which may be negative) to the branch's balance
// bucket that matches the payment method. Returns repoerr.ErrNotFound when no
// branch finance row matches.
func adjustBranchBalance(ctx context.Context, tx *gorm.DB, branchID string, pm models.PaymentMethod, delta int32) error {
	var col string
	switch pm {
	case models.PaymentMethodCash:
		col = "balance_cash"
	case models.PaymentMethodBank:
		col = "balance_bank"
	case models.PaymentMethodTerminal:
		col = "balance_terminal"
	case models.OnlineMobileAppPayment, models.OnlineTransfer:
		col = "balance_mobile_apps"
	default:
		return nil // undefined payment method: no bucket to adjust (mirrors Mongo)
	}
	res := tx.WithContext(ctx).Table("branch_finance").Where("branch_id = ?", branchID).
		Update(col, gorm.Expr(col+" + ?", delta))
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return repoerr.ErrNotFound
	}
	return nil
}
