// Package inventory holds the stock-movement primitives — the writes that move
// product quantity in and out of product_stock when a sale is charged or voided.
//
// It is a sibling of pkg/repository/ledger rather than part of it: ledger is
// scoped to money (transactions, supplier balances, branch finance) and stock is
// not money. Like the ledger primitives, every function here takes a *gorm.DB
// that is expected to be a transaction handle, so the stock movement commits
// atomically with the sale and journal writes that accompany it.
//
// Quantity is fractional (numeric(12,3)) because weighted goods sell by the
// kilogram; see models.SaleCartItem.
package inventory

import (
	"context"
	"database/sql"
	"errors"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"
	"github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
	"github.com/rs/zerolog/log"
	"gorm.io/gorm"
)

// ApplyStockDecrement subtracts each item's quantity from product_stock at the
// branch, as one atomic UPDATE per line (`quantity = quantity - ?`) so two tills
// selling the last unit concurrently cannot both read the same starting value.
//
// Overselling does NOT fail the sale. By the time checkout runs, the goods are
// physically in the customer's basket or on the counter, and refusing payment
// there strands them at the till; availability is checked earlier, when an item
// is scanned and can still be put back. A line that drives stock below zero is
// logged as a stocktaking signal and the sale proceeds.
//
// A product that is not stocked at the branch at all is repoerr.ErrNotFound —
// that is a data error, not an oversell.
func ApplyStockDecrement(ctx context.Context, tx *gorm.DB, branchID string, items []models.SaleCartItem) error {
	return applyStockDelta(ctx, tx, branchID, items, -1)
}

// ApplyStockRestore is the reverse: it adds each item's quantity back, for when
// a recorded sale is deleted or voided. Without it a voided sale would silently
// lose the stock it consumed, since the money side is already reversed by
// ledger.DeleteSalesTransaction.
func ApplyStockRestore(ctx context.Context, tx *gorm.DB, branchID string, items []models.SaleCartItem) error {
	return applyStockDelta(ctx, tx, branchID, items, +1)
}

// RestoreForSale puts back the stock a sale consumed, given the transaction being
// voided. It resolves the cart through transaction.CartID and adds every line's
// quantity back to the branch.
//
// It is a no-op when the transaction has no cart — a keyed/manual sale never
// decremented anything, and a sale whose cart was later hard-deleted has had its
// cart_id nulled, so there is no item list left to reverse. Both are expected
// states, not errors: the money reversal is what matters there.
//
// Call this from every path that removes a sale, inside the same transaction as
// the delete, so stock and money reverse together or not at all.
func RestoreForSale(ctx context.Context, tx *gorm.DB, transaction *models.Transaction) error {
	if transaction == nil || transaction.Type != models.InitiatorTypeSales {
		return nil
	}
	if transaction.CartID == nil || *transaction.CartID == "" {
		return nil
	}
	items, err := gorm.G[models.SaleCartItem](tx).Where("cart_id = ?", *transaction.CartID).Find(ctx)
	if err != nil {
		return err
	}
	if len(items) == 0 {
		return nil
	}
	log.Info().Str("transaction_id", transaction.ID).Str("cart_id", *transaction.CartID).
		Int("items", len(items)).Msg("stock movement: restoring stock for a voided sale")
	return ApplyStockRestore(ctx, tx, transaction.BranchID, items)
}

// applyStockDelta applies sign * quantity to each item's stock row. It uses
// RETURNING so the resulting quantity is observed in the same statement that
// changed it — no read-modify-write race, and no extra query to detect oversell.
func applyStockDelta(ctx context.Context, tx *gorm.DB, branchID string, items []models.SaleCartItem, sign float64) error {
	for _, item := range items {
		if item.Quantity <= 0 {
			continue
		}
		delta := sign * item.Quantity

		var remaining float64
		err := tx.WithContext(ctx).Raw(
			`UPDATE product_stock SET quantity = quantity + ?
			 WHERE product_id = ? AND place_id = ?
			 RETURNING quantity`,
			delta, item.ProductID, branchID,
		).Row().Scan(&remaining)

		if errors.Is(err, sql.ErrNoRows) {
			log.Error().Str("product_id", item.ProductID).Str("branch_id", branchID).
				Msg("stock movement: product not stocked at branch")
			return repoerr.ErrNotFound
		}
		if err != nil {
			return err
		}

		if remaining < 0 {
			log.Warn().
				Str("product_id", item.ProductID).
				Str("branch_id", branchID).
				Float64("quantity", item.Quantity).
				Float64("remaining", remaining).
				Msg("stock movement: branch stock is now negative (oversold) — sale allowed, needs stocktaking")
		}
	}
	return nil
}
