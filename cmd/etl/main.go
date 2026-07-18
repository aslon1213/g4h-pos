// Command etl is a one-off data migrator that copies the legacy MongoDB data of
// the POS-ERP into the new PostgreSQL schema (platform/migrations). It is meant
// to be run ONCE against a freshly-migrated (empty) Postgres database.
//
// Usage:
//
//	ETL_MONGO_URI="mongodb://localhost:27017" \
//	ETL_MONGO_DB="magazin" \
//	ETL_PG_DSN="host=localhost port=5432 user=postgres password=postgres dbname=pos sslmode=disable" \
//	ETL_BATCH_SIZE=500 \   # optional; docs per transaction for large collections (default 500)
//	go run ./cmd/etl
//
// The program connects to both stores, migrates every collection into the PG
// tables in FK-safe order, and prints a reconciliation report (row counts +
// money checks). Small collections are written inside a single transaction;
// the large ones (activities, transactions, customers/bnpls, journals,
// proposals) are written in batches of ETL_BATCH_SIZE rows, one transaction per
// batch, so a big collection is not held in a single giant transaction. Because
// a batched collection commits per chunk, a mid-run failure can leave earlier
// batches persisted — re-run into a fresh/empty target.
//
// ---------------------------------------------------------------------------
// Documented data-loss caveat: transactions.transaction_type
// ---------------------------------------------------------------------------
// The top-level `type` field of a transaction document holds the INITIATOR
// (sale/supplier/bnpl/salary/...); the ETL maps it to `initiator_type`.
//
// Two document shapes exist in the source:
//   - Flat (older): amount/description/payment_method live at the top level.
//     The credit/debit DIRECTION collided on the bson key "type" with the
//     initiator and was not separately persisted for these rows.
//   - Nested (most "sale" rows): amount/description/payment_method live under a
//     `transactionbase` subdocument (the embedded TransactionBase was written as
//     a nested doc, not inlined), and `transactionbase.type` DOES hold the
//     credit/debit direction.
//
// Per the migration spec, `transaction_type` is set to NULL for every row for
// consistency (it is unavailable for the flat rows). Note: for nested-shape
// rows the direction could be recovered from `transactionbase.type` if desired.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/aslon1213/g4h_pos_erp/pkg/models"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func main() {
	mongoURI := envOr("ETL_MONGO_URI", "mongodb://localhost:27017")
	mongoDB := envOr("ETL_MONGO_DB", "magazin")
	pgDSN := os.Getenv("ETL_PG_DSN")
	if pgDSN == "" {
		fatalf("ETL_PG_DSN is required (target Postgres DSN)")
	}

	ctx := context.Background()

	logf("connecting to MongoDB %s (db=%s)", mongoURI, mongoDB)
	mclient, err := mongo.Connect(options.Client().ApplyURI(mongoURI))
	if err != nil {
		fatalf("mongo connect: %v", err)
	}
	defer func() { _ = mclient.Disconnect(ctx) }()
	if err := mclient.Ping(ctx, nil); err != nil {
		fatalf("mongo ping: %v", err)
	}

	logf("connecting to PostgreSQL")
	pg, err := gorm.Open(postgres.Open(pgDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		fatalf("postgres connect: %v", err)
	}
	sqlDB, err := pg.DB()
	if err != nil {
		fatalf("postgres handle: %v", err)
	}
	if err := sqlDB.Ping(); err != nil {
		fatalf("postgres ping: %v", err)
	}

	m := &Migrator{
		ctx:                  ctx,
		mdb:                  mclient.Database(mongoDB),
		pg:                   pg,
		supplierTxn:          map[string]string{},
		bnplTxn:              map[string]string{},
		embeddedSupplierTxns: map[string]bson.M{},
		categoryIDs:          map[string]bool{},
	}

	if err := m.Run(); err != nil {
		fatalf("migration failed: %v", err)
	}
}

// Migrator carries the source/target handles and the cross-collection lookup
// tables that let step 8 (transactions) resolve its foreign keys.
type Migrator struct {
	ctx context.Context
	mdb *mongo.Database
	pg  *gorm.DB

	// txn id -> supplier id (from suppliers.financial_data.transactions[])
	supplierTxn map[string]string
	// txn id -> bnpl id (from customers.bnpls[].transactions[])
	bnplTxn map[string]string
	// embedded supplier transaction docs, keyed by txn id, for the dedupe insert
	embeddedSupplierTxns map[string]bson.M
	// valid category ids (product_categories.category_id FK is enforced)
	categoryIDs map[string]bool
	// count of product->category references skipped for a missing category
	skippedProductCategories int64

	report []reconRow
}

// reconRow is one line of the final reconciliation table.
type reconRow struct {
	label    string // collection / relation
	expected int64  // count derived from the Mongo side
	pgTable  string
	pgCount  int64
}

func (m *Migrator) Run() error {
	start := time.Now()
	steps := []struct {
		name string
		fn   func() error
	}{
		{"finance -> branches + branch_finance", m.migrateFinance},
		{"users", m.migrateUsers},
		{"activities", m.migrateActivities},
		{"brands", m.migrateBrands},
		{"categories", m.migrateCategories},
		{"suppliers (+ embedded txns)", m.migrateSuppliers},
		{"products (+ stock/income/categories)", m.migrateProducts},
		{"customers (+ bnpls/bnpl_products)", m.migrateCustomers},
		{"transactions", m.migrateTransactions},
		{"journals (+ txn journal_id)", m.migrateJournals},
		{"store_customers (+ addresses)", m.migrateStoreCustomers},
		{"carts (+ cart_items)", m.migrateCarts},
		{"wishlists (+ wishlist_items)", m.migrateWishlists},
		{"orders (+ order_items)", m.migrateOrders},
		{"reviews (+ review_votes)", m.migrateReviews},
		{"promotions", m.migratePromotions},
		{"proposals", m.migrateProposals},
		{"internal_expenses (best-effort)", m.migrateInternalExpenses},
	}
	for _, s := range steps {
		logf("==> %s", s.name)
		if err := s.fn(); err != nil {
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}
	logf("all collections migrated in %s", time.Since(start).Round(time.Millisecond))
	return m.reconcile()
}

// ---------------------------------------------------------------------------
// 1. finance -> branches + branch_finance
// ---------------------------------------------------------------------------

func (m *Migrator) migrateFinance() error {
	docs, err := m.findAll("finance")
	if err != nil {
		return err
	}
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			branchID := getStr(d, "branch_id")
			branchName := getStr(d, "branch_name")
			if branchID == "" {
				logf("  WARN finance doc without branch_id (branch_name=%q) skipped", branchName)
				continue
			}
			var location, phone interface{}
			if b, ok := models.Branch_names[branchName]; ok {
				location = nullIfEmpty(b.Location)
				phone = nullIfEmpty(b.Phone)
			}
			if err := tx.Exec(
				`INSERT INTO branches (id, name, location, phone) VALUES (?, ?, ?, ?)`,
				branchID, branchName, location, phone,
			).Error; err != nil {
				return fmt.Errorf("insert branch %s: %w", branchID, err)
			}

			bal := getDoc(d, "balance")
			if err := tx.Exec(
				`INSERT INTO branch_finance
				 (branch_id, branch_name, balance_cash, balance_bank, balance_terminal,
				  balance_mobile_apps, total_income, total_expenses, debt, details)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb)`,
				branchID, branchName,
				getInt(bal, "cash"), getInt(bal, "bank"), getInt(bal, "terminal"), getInt(bal, "mobile_apps"),
				getInt(d, "total_income"), getInt(d, "total_expenses"), getInt(d, "debt"),
				toJSONB(d["details"]),
			).Error; err != nil {
				return fmt.Errorf("insert branch_finance %s: %w", branchID, err)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("finance", int64(len(docs)), "branches", "branch_finance")
	return nil
}

// ---------------------------------------------------------------------------
// 2. users
// ---------------------------------------------------------------------------

func (m *Migrator) migrateUsers() error {
	docs, err := m.findAll("users")
	if err != nil {
		return err
	}
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			if err := tx.Exec(
				`INSERT INTO users (id, email, username, password, role, phone, branch)
				 VALUES (?, ?, ?, ?, ?, ?, ?)`,
				docID(d), nullIfEmpty(getStr(d, "email")), getStr(d, "username"),
				getStr(d, "password"), nullIfEmpty(getStr(d, "role")),
				nullIfEmpty(getStr(d, "phone")), nullIfEmpty(getStr(d, "branch")),
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("users", int64(len(docs)), "users")
	return nil
}

// ---------------------------------------------------------------------------
// 3. activities (drop _id; bigserial assigns a new one)
// ---------------------------------------------------------------------------

func (m *Migrator) migrateActivities() error {
	docs, err := m.findAll("activities")
	if err != nil {
		return err
	}
	err = m.inBatches("activities", docs, func(tx *gorm.DB, batch []bson.M) error {
		for _, d := range batch {
			if err := tx.Exec(
				`INSERT INTO activities (user_id, action, data, ip, date, status)
				 VALUES (?, ?, ?::jsonb, ?, ?, ?)`,
				nullIfEmpty(getStr(d, "user_id")), nullIfEmpty(getStr(d, "action")),
				toJSONB(d["data"]), nullIfEmpty(getStr(d, "ip")),
				orNow(getTime(d, "date")), getInt(d, "status"),
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("activities", int64(len(docs)), "activities")
	return nil
}

// ---------------------------------------------------------------------------
// 4. brands + categories
// ---------------------------------------------------------------------------

func (m *Migrator) migrateBrands() error {
	docs, err := m.findAll("brands")
	if err != nil {
		return err
	}
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			if err := tx.Exec(
				`INSERT INTO brands (id, name, slug, logo, country) VALUES (?, ?, ?, ?, ?)`,
				docID(d), getStr(d, "name"), nullIfEmpty(getStr(d, "slug")),
				nullIfEmpty(getStr(d, "logo")), nullIfEmpty(getStr(d, "country")),
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("brands", int64(len(docs)), "brands")
	return nil
}

func (m *Migrator) migrateCategories() error {
	docs, err := m.findAll("categories")
	if err != nil {
		return err
	}
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			if err := tx.Exec(
				`INSERT INTO categories (id, name, slug, parent_id, description, image, sort_order, is_active, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, getStr(d, "name"), nullIfEmpty(getStr(d, "slug")),
				nullIfEmpty(getStr(d, "parent_id")), // "" -> NULL
				nullIfEmpty(getStr(d, "description")), nullIfEmpty(getStr(d, "image")),
				getInt(d, "sort_order"), getBoolDefault(d, "is_active", true),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
			m.categoryIDs[id] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("categories", int64(len(docs)), "categories")
	return nil
}

// ---------------------------------------------------------------------------
// 5. suppliers (flatten financial_data; stash embedded transactions)
// ---------------------------------------------------------------------------

func (m *Migrator) migrateSuppliers() error {
	docs, err := m.findAll("suppliers")
	if err != nil {
		return err
	}
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			fin := getDoc(d, "financial_data")
			if err := tx.Exec(
				`INSERT INTO suppliers
				 (id, name, address, phone, email, inn, notes, branch_id, balance, total_income, total_expenses, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, getStr(d, "name"), nullIfEmpty(getStr(d, "address")), nullIfEmpty(getStr(d, "phone")),
				nullIfEmpty(getStr(d, "email")), nullIfEmpty(getStr(d, "inn")), nullIfEmpty(getStr(d, "notes")),
				nullIfEmpty(getStr(d, "branch")), // branch -> branch_id
				getInt(fin, "balance"), getInt(fin, "total_income"), getInt(fin, "total_expenses"),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
			// stash embedded transactions for step 8 (dedupe against top-level)
			for _, t := range getArray(fin, "transactions") {
				td := asDoc(t)
				if td == nil {
					continue
				}
				tid := docID(td)
				if tid == "" {
					continue
				}
				m.supplierTxn[tid] = id
				m.embeddedSupplierTxns[tid] = td
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("suppliers", int64(len(docs)), "suppliers")
	return nil
}

// ---------------------------------------------------------------------------
// 6. products (+ product_stock / product_income_history / product_categories)
// ---------------------------------------------------------------------------

func (m *Migrator) migrateProducts() error {
	docs, err := m.findAll("products")
	if err != nil {
		return err
	}
	var stockN, incomeN, catN int64
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			man := getDoc(d, "manufacturer")
			images := getStrArray(d, "images")
			imagesJSON, _ := json.Marshal(images) // never nil (defaults to "[]")
			if err := tx.Exec(
				`INSERT INTO products
				 (id, name, description, manufacturer_name, manufacturer_country, manufacturer_address,
				  manufacturer_phone, manufacturer_email, brand_id, sku, minimum_stock_alert,
				  general_income_price, images, rating, review_count, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?::jsonb, ?, ?, ?, ?)`,
				id, getStr(d, "name"), nullIfEmpty(getStr(d, "description")),
				nullIfEmpty(getStr(man, "name")), nullIfEmpty(getStr(man, "country")), nullIfEmpty(getStr(man, "address")),
				nullIfEmpty(getStr(man, "phone")), nullIfEmpty(getStr(man, "email")),
				nullIfEmpty(getStr(d, "brand_id")), nullIfEmpty(getStr(d, "sku")),
				getInt(d, "minimum_stock_alert"), getFloat(d, "general_income_price"),
				string(imagesJSON), getFloat(d, "rating"), getInt(d, "review_count"),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}

			// category[] -> product_categories junction rows. The FK is enforced,
			// so references to categories that don't exist (the catalog was never
			// populated in the source) are skipped and counted, not migrated.
			for _, c := range getStrArray(d, "category") {
				if c == "" {
					continue
				}
				if !m.categoryIDs[c] {
					m.skippedProductCategories++
					continue
				}
				if err := tx.Exec(
					`INSERT INTO product_categories (product_id, category_id) VALUES (?, ?)
					 ON CONFLICT DO NOTHING`,
					id, c,
				).Error; err != nil {
					return err
				}
				catN++
			}

			// quantity_distribution[] -> product_stock
			for _, q := range getArray(d, "quantity_distribution") {
				qd := asDoc(q)
				if qd == nil {
					continue
				}
				place := getDoc(qd, "place")
				if err := tx.Exec(
					`INSERT INTO product_stock (product_id, place_id, place_type, quantity, unit, price)
					 VALUES (?, ?, ?, ?, ?, ?)`,
					// quantity is numeric(12,3) since 00004 (weighted goods sell by
					// the kilogram) — read it as a float so a fractional source value
					// is not silently truncated on the way in. price stays integer
					// so'm.
					id, getStr(place, "id"), nullIfEmpty(getStr(place, "place_type")),
					getFloat(qd, "quantity"), nullIfEmpty(getStr(qd, "unit")), getInt(qd, "price"),
				).Error; err != nil {
					return err
				}
				stockN++
			}

			// income_history[] -> product_income_history
			for _, h := range getArray(d, "income_history") {
				hd := asDoc(h)
				if hd == nil {
					continue
				}
				place := getDoc(hd, "uploaded_to")
				if err := tx.Exec(
					`INSERT INTO product_income_history
					 (product_id, date, price, quantity, place_id, place_type, supplier_id)
					 VALUES (?, ?, ?, ?, ?, ?, ?)`,
					// quantity is numeric(12,3) since 00004; see migrateProducts above.
					id, nullIfEmpty(getStr(hd, "date")), getInt(hd, "price"), getFloat(hd, "quantity"),
					nullIfEmpty(getStr(place, "id")), nullIfEmpty(getStr(place, "place_type")),
					nullIfEmpty(getStr(hd, "supplier_id")),
				).Error; err != nil {
					return err
				}
				incomeN++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	if m.skippedProductCategories > 0 {
		logf("  NOTE: skipped %d product->category reference(s) to non-existent categories (empty catalog)", m.skippedProductCategories)
	}
	m.record("products", int64(len(docs)), "products")
	m.record("products.category[]", catN, "product_categories")
	m.record("products.quantity_distribution[]", stockN, "product_stock")
	m.record("products.income_history[]", incomeN, "product_income_history")
	return nil
}

// ---------------------------------------------------------------------------
// 7. customers (+ bnpls, bnpl_products); build bnplTxn map
// ---------------------------------------------------------------------------

func (m *Migrator) migrateCustomers() error {
	docs, err := m.findAll("customers")
	if err != nil {
		return err
	}
	var bnplN, bnplProdN int64
	err = m.inBatches("customers", docs, func(tx *gorm.DB, batch []bson.M) error {
		for _, d := range batch {
			id := docID(d)
			if err := tx.Exec(
				`INSERT INTO customers (id, name, phone, address, additional_info, purchase_history, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?::jsonb, ?::jsonb, ?, ?)`,
				id, nullIfEmpty(getStr(d, "name")), nullIfEmpty(getStr(d, "phone")),
				nullIfEmpty(getStr(d, "address")),
				toJSONB(d["additional_info"]), toJSONB(d["purchase_history"]),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}

			for _, b := range getArray(d, "bnpls") {
				bd := asDoc(b)
				if bd == nil {
					continue
				}
				bid := getStr(bd, "id") // BNPL uses bson:"id", not _id
				if bid == "" {
					bid = docID(bd)
				}
				if err := tx.Exec(
					`INSERT INTO bnpls (id, customer_id, branch_id, total_amount, paid_amount, status, created_at, updated_at)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					bid, id, nullIfEmpty(getStr(bd, "branch_id")),
					getInt(bd, "total_amount"), getInt(bd, "paid_amount"),
					strOrDefault(getStr(bd, "status"), string(models.BNPLStatusActive)),
					orNow(getTime(bd, "created_at")), orNow(getTime(bd, "updated_at")),
				).Error; err != nil {
					return err
				}
				bnplN++

				// products map -> bnpl_products rows
				for productID, item := range getDoc(bd, "products") {
					im := asDoc(item)
					if err := tx.Exec(
						`INSERT INTO bnpl_products (bnpl_id, product_id, quantity, price)
						 VALUES (?, ?, ?, ?) ON CONFLICT (bnpl_id, product_id) DO NOTHING`,
						bid, productID, getInt(im, "quantity"), getInt(im, "price"),
					).Error; err != nil {
						return err
					}
					bnplProdN++
				}

				// transactions[] id list -> bnplTxn map (bnpl_id set in step 8)
				for _, tid := range getStrArray(bd, "transactions") {
					if tid != "" {
						m.bnplTxn[tid] = bid
					}
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("customers", int64(len(docs)), "customers")
	m.record("customers.bnpls[]", bnplN, "bnpls")
	m.record("customers.bnpls[].products", bnplProdN, "bnpl_products")
	return nil
}

// ---------------------------------------------------------------------------
// 8. transactions (top-level) + embedded-supplier-only txns (deduped)
// ---------------------------------------------------------------------------

func (m *Migrator) migrateTransactions() error {
	docs, err := m.findAll("transactions")
	if err != nil {
		return err
	}
	inserted := map[string]bool{}
	// top-level transactions (batched; `inserted` accumulates across batches)
	err = m.inBatches("transactions", docs, func(tx *gorm.DB, batch []bson.M) error {
		for _, d := range batch {
			id := docID(d)
			if id == "" || inserted[id] {
				continue
			}
			if err := m.insertTransaction(tx, id, d,
				ptrOrNil(m.supplierTxn[id]), ptrOrNil(m.bnplTxn[id])); err != nil {
				return err
			}
			inserted[id] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	// embedded supplier transactions not present in the top-level collection.
	// Collect the leftovers (keyed by docID == the map key) so they batch too.
	var embedded []bson.M
	for id, d := range m.embeddedSupplierTxns {
		if !inserted[id] {
			embedded = append(embedded, d)
		}
	}
	err = m.inBatches("transactions (embedded)", embedded, func(tx *gorm.DB, batch []bson.M) error {
		for _, d := range batch {
			id := docID(d)
			if id == "" || inserted[id] {
				continue
			}
			sid := m.supplierTxn[id]
			if err := m.insertTransaction(tx, id, d, &sid, ptrOrNil(m.bnplTxn[id])); err != nil {
				return err
			}
			inserted[id] = true
		}
		return nil
	})
	if err != nil {
		return err
	}
	// journal_id is deliberately left NULL here; step 9 fills it via UPDATE.
	m.record("transactions (+embedded)", int64(len(inserted)), "transactions")
	return nil
}

// insertTransaction writes one row. transaction_type (credit|debit) is always
// NULL: the direction was never reliably persisted in Mongo (see the file
// header).
//
// Two document shapes exist in the source. Older/flat transactions store
// amount/description/payment_method at the top level. Newer transactions (most
// "sale" rows) nest them under a `transactionbase` subdocument — the embedded
// TransactionBase was serialised as a nested doc rather than inlined — with the
// top-level `type` holding the initiator. The content fields are read from the
// top level when present and otherwise recovered from `transactionbase`.
func (m *Migrator) insertTransaction(tx *gorm.DB, id string, d bson.M, supplierID, bnplID *string) error {
	tb := getDoc(d, "transactionbase")

	amount := getInt(d, "amount")
	if _, ok := d["amount"]; !ok {
		amount = getInt(tb, "amount")
	}
	description := firstNonEmpty(getStr(d, "description"), getStr(tb, "description"))
	paymentMethod := firstNonEmpty(getStr(d, "payment_method"), getStr(tb, "payment_method"))

	created := getTime(d, "created_at")
	if created.IsZero() {
		created = getTime(tb, "created_at")
	}
	updated := getTime(d, "updated_at")
	if updated.IsZero() {
		updated = getTime(tb, "updated_at")
	}

	return tx.Exec(
		`INSERT INTO transactions
		 (id, amount, description, transaction_type, initiator_type, payment_method,
		  branch_id, journal_id, supplier_id, bnpl_id, created_at, updated_at)
		 VALUES (?, ?, ?, NULL, ?, ?, ?, NULL, ?, ?, ?, ?)`,
		id, amount, nullIfEmpty(description),
		nullIfEmpty(getStr(d, "type")), // top-level Mongo `type` == initiator
		nullIfEmpty(paymentMethod),
		nullIfEmpty(getStr(d, "branch_id")),
		supplierID, bnplID,
		orNow(created), orNow(updated),
	).Error
}

// ---------------------------------------------------------------------------
// 9. journals (ObjectID _id -> hex); UPDATE txn journal_id from operations[]
// ---------------------------------------------------------------------------

func (m *Migrator) migrateJournals() error {
	docs, err := m.findAll("journals")
	if err != nil {
		return err
	}
	err = m.inBatches("journals", docs, func(tx *gorm.DB, batch []bson.M) error {
		for _, d := range batch {
			id := docID(d) // ObjectID -> 24-char hex
			br := getDoc(d, "branch")
			if err := tx.Exec(
				`INSERT INTO journals
				 (id, branch_id, branch_name, branch_location, branch_phone, date,
				  shift_is_closed, terminal_income, cash_left, total, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, now(), now())`,
				id, nullIfEmpty(getStr(br, "_id")), nullIfEmpty(getStr(br, "name")),
				nullIfEmpty(getStr(br, "location")), nullIfEmpty(getStr(br, "phone")),
				orNow(getTime(d, "date")), getBool(d, "shift_is_closed"),
				getInt(d, "terminal_income"), getInt(d, "cash_left"), getInt(d, "total"),
			).Error; err != nil {
				return err
			}
			// operations[] (txn id strings) -> transactions.journal_id
			for _, tid := range getStrArray(d, "operations") {
				if tid == "" {
					continue
				}
				if err := tx.Exec(
					`UPDATE transactions SET journal_id = ? WHERE id = ?`, id, tid,
				).Error; err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("journals", int64(len(docs)), "journals")
	return nil
}

// ---------------------------------------------------------------------------
// 10. store_customers (+ addresses)
// ---------------------------------------------------------------------------

func (m *Migrator) migrateStoreCustomers() error {
	docs, err := m.findAll("store_customers")
	if err != nil {
		return err
	}
	var addrN int64
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			if err := tx.Exec(
				`INSERT INTO store_customers (id, email, phone, name, password_hash, email_verified, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				id, getStr(d, "email"), nullIfEmpty(getStr(d, "phone")), nullIfEmpty(getStr(d, "name")),
				getStr(d, "password_hash"), getBool(d, "email_verified"),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
			for _, a := range getArray(d, "addresses") {
				ad := asDoc(a)
				if ad == nil {
					continue
				}
				aid := getStr(ad, "id")
				if aid == "" {
					aid = docID(ad)
				}
				if err := tx.Exec(
					`INSERT INTO addresses
					 (id, customer_id, label, full_name, phone, line1, line2, city, region, postal_code, country, is_default)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
					aid, id, nullIfEmpty(getStr(ad, "label")), nullIfEmpty(getStr(ad, "full_name")),
					nullIfEmpty(getStr(ad, "phone")), nullIfEmpty(getStr(ad, "line1")), nullIfEmpty(getStr(ad, "line2")),
					nullIfEmpty(getStr(ad, "city")), nullIfEmpty(getStr(ad, "region")),
					nullIfEmpty(getStr(ad, "postal_code")), nullIfEmpty(getStr(ad, "country")),
					getBool(ad, "is_default"),
				).Error; err != nil {
					return err
				}
				addrN++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("store_customers", int64(len(docs)), "store_customers")
	m.record("store_customers.addresses[]", addrN, "addresses")
	return nil
}

// ---------------------------------------------------------------------------
// 11. carts / wishlists / orders
// ---------------------------------------------------------------------------

func (m *Migrator) migrateCarts() error {
	docs, err := m.findAll("carts")
	if err != nil {
		return err
	}
	var itemN int64
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			if err := tx.Exec(
				`INSERT INTO carts (id, customer_id, coupon_code, subtotal, discount, total, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
				id, getStr(d, "customer_id"), nullIfEmpty(getStr(d, "coupon_code")),
				getFloat(d, "subtotal"), getFloat(d, "discount"), getFloat(d, "total"),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
			for _, it := range getArray(d, "items") {
				id2 := asDoc(it)
				if id2 == nil {
					continue
				}
				ciid := getStr(id2, "id")
				if ciid == "" {
					ciid = docID(id2)
				}
				if err := tx.Exec(
					`INSERT INTO cart_items (id, cart_id, product_id, name, image, quantity, unit_price, line_total)
					 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
					ciid, id, getStr(id2, "product_id"), nullIfEmpty(getStr(id2, "name")),
					nullIfEmpty(getStr(id2, "image")), getInt(id2, "quantity"),
					getFloat(id2, "unit_price"), getFloat(id2, "line_total"),
				).Error; err != nil {
					return err
				}
				itemN++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("carts", int64(len(docs)), "carts")
	m.record("carts.items[]", itemN, "cart_items")
	return nil
}

func (m *Migrator) migrateWishlists() error {
	docs, err := m.findAll("wishlists")
	if err != nil {
		return err
	}
	var itemN int64
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			if err := tx.Exec(
				`INSERT INTO wishlists (id, customer_id, created_at, updated_at) VALUES (?, ?, ?, ?)`,
				id, getStr(d, "customer_id"),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
			for _, it := range getArray(d, "items") {
				wi := asDoc(it)
				if wi == nil {
					continue
				}
				if err := tx.Exec(
					`INSERT INTO wishlist_items (wishlist_id, product_id, name, image, price, added_at)
					 VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT (wishlist_id, product_id) DO NOTHING`,
					id, getStr(wi, "product_id"), nullIfEmpty(getStr(wi, "name")),
					nullIfEmpty(getStr(wi, "image")), getFloat(wi, "price"),
					orNow(getTime(wi, "added_at")),
				).Error; err != nil {
					return err
				}
				itemN++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("wishlists", int64(len(docs)), "wishlists")
	m.record("wishlists.items[]", itemN, "wishlist_items")
	return nil
}

func (m *Migrator) migrateOrders() error {
	docs, err := m.findAll("orders")
	if err != nil {
		return err
	}
	var itemN int64
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			tot := getDoc(d, "totals")
			addr := getDoc(d, "address")
			if err := tx.Exec(
				`INSERT INTO orders
				 (id, number, customer_id, status, coupon_code, note,
				  subtotal, discount, shipping, tax, total,
				  address_id, address_customer_id, address_label, address_full_name, address_phone,
				  address_line1, address_line2, address_city, address_region, address_postal_code,
				  address_country, address_is_default, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, getStr(d, "number"), nullIfEmpty(getStr(d, "customer_id")),
				getStr(d, "status"), nullIfEmpty(getStr(d, "coupon_code")), nullIfEmpty(getStr(d, "note")),
				getFloat(tot, "subtotal"), getFloat(tot, "discount"), getFloat(tot, "shipping"),
				getFloat(tot, "tax"), getFloat(tot, "total"),
				nullIfEmpty(getStr(addr, "id")), nullIfEmpty(getStr(addr, "customer_id")),
				nullIfEmpty(getStr(addr, "label")), nullIfEmpty(getStr(addr, "full_name")), nullIfEmpty(getStr(addr, "phone")),
				nullIfEmpty(getStr(addr, "line1")), nullIfEmpty(getStr(addr, "line2")), nullIfEmpty(getStr(addr, "city")),
				nullIfEmpty(getStr(addr, "region")), nullIfEmpty(getStr(addr, "postal_code")),
				nullIfEmpty(getStr(addr, "country")), getBool(addr, "is_default"),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
			for _, it := range getArray(d, "items") {
				oi := asDoc(it)
				if oi == nil {
					continue
				}
				if err := tx.Exec(
					`INSERT INTO order_items (order_id, product_id, name, image, quantity, unit_price, line_total)
					 VALUES (?, ?, ?, ?, ?, ?, ?)`,
					id, getStr(oi, "product_id"), nullIfEmpty(getStr(oi, "name")),
					nullIfEmpty(getStr(oi, "image")), getInt(oi, "quantity"),
					getFloat(oi, "unit_price"), getFloat(oi, "line_total"),
				).Error; err != nil {
					return err
				}
				itemN++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("orders", int64(len(docs)), "orders")
	m.record("orders.items[]", itemN, "order_items")
	return nil
}

// ---------------------------------------------------------------------------
// 12. reviews (+ review_votes from voters[])
// ---------------------------------------------------------------------------

func (m *Migrator) migrateReviews() error {
	docs, err := m.findAll("reviews")
	if err != nil {
		return err
	}
	var voteN int64
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			id := docID(d)
			if err := tx.Exec(
				`INSERT INTO reviews
				 (id, product_id, customer_id, customer_name, rating, title, body, helpful_votes, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, getStr(d, "product_id"), getStr(d, "customer_id"), nullIfEmpty(getStr(d, "customer_name")),
				getInt(d, "rating"), nullIfEmpty(getStr(d, "title")), nullIfEmpty(getStr(d, "body")),
				getInt(d, "helpful_votes"),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
			for _, v := range getStrArray(d, "voters") {
				if v == "" {
					continue
				}
				if err := tx.Exec(
					`INSERT INTO review_votes (review_id, customer_id) VALUES (?, ?)
					 ON CONFLICT DO NOTHING`,
					id, v,
				).Error; err != nil {
					return err
				}
				voteN++
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("reviews", int64(len(docs)), "reviews")
	m.record("reviews.voters[]", voteN, "review_votes")
	return nil
}

// ---------------------------------------------------------------------------
// 13. promotions
// ---------------------------------------------------------------------------

func (m *Migrator) migratePromotions() error {
	docs, err := m.findAll("promotions")
	if err != nil {
		return err
	}
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			if err := tx.Exec(
				`INSERT INTO promotions
				 (id, title, description, banner, code, discount_type, value, min_subtotal,
				  usage_limit, used_count, is_active, starts_at, ends_at, created_at, updated_at)
				 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				docID(d), nullIfEmpty(getStr(d, "title")), nullIfEmpty(getStr(d, "description")),
				nullIfEmpty(getStr(d, "banner")), nullIfEmpty(getStr(d, "code")),
				nullIfEmpty(getStr(d, "discount_type")), getFloat(d, "value"), getFloat(d, "min_subtotal"),
				getInt(d, "usage_limit"), getInt(d, "used_count"), getBoolDefault(d, "is_active", true),
				timePtr(getTime(d, "starts_at")), timePtr(getTime(d, "ends_at")),
				orNow(getTime(d, "created_at")), orNow(getTime(d, "updated_at")),
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("promotions", int64(len(docs)), "promotions")
	return nil
}

// ---------------------------------------------------------------------------
// 14. proposals (ObjectID _id -> hex)
// ---------------------------------------------------------------------------

func (m *Migrator) migrateProposals() error {
	docs, err := m.findAll("proposals")
	if err != nil {
		return err
	}
	err = m.inBatches("proposals", docs, func(tx *gorm.DB, batch []bson.M) error {
		for _, d := range batch {
			if err := tx.Exec(
				`INSERT INTO proposals (id, name, date, branch, fulfilled, image_file)
				 VALUES (?, ?, ?, ?, ?, ?)`,
				docID(d), nullIfEmpty(getStr(d, "name")), timePtr(getTime(d, "date")),
				nullIfEmpty(getStr(d, "branch")), getBool(d, "fulfilled"),
				nullIfEmpty(getStr(d, "image_file")),
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("proposals", int64(len(docs)), "proposals")
	return nil
}

// internal_expenses: best-effort. The collection is normally absent (the
// controller is unimplemented); migrate rows only if any exist.
func (m *Migrator) migrateInternalExpenses() error {
	docs, err := m.findAll("internal_expenses")
	if err != nil {
		return err
	}
	if len(docs) == 0 {
		logf("  no internal_expenses documents; skipping")
		return nil
	}
	err = m.pg.Transaction(func(tx *gorm.DB) error {
		for _, d := range docs {
			if err := tx.Exec(
				`INSERT INTO internal_expenses (id, branch_id, amount, description, data, created_at)
				 VALUES (?, ?, ?, ?, ?::jsonb, ?)`,
				docID(d), nullIfEmpty(getStr(d, "branch_id")), getInt(d, "amount"),
				nullIfEmpty(getStr(d, "description")), toJSONB(d["data"]),
				orNow(getTime(d, "created_at")),
			).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	m.record("internal_expenses", int64(len(docs)), "internal_expenses")
	return nil
}

// ---------------------------------------------------------------------------
// Reconciliation
// ---------------------------------------------------------------------------

func (m *Migrator) reconcile() error {
	fmt.Println()
	fmt.Println("==================== RECONCILIATION: ROW COUNTS ====================")
	fmt.Printf("%-38s %10s  %-24s %10s  %s\n", "MONGO (source)", "EXPECTED", "PG TABLE", "ACTUAL", "STATUS")
	fmt.Println("-------------------------------------------------------------------------------------------------------")
	mismatches := 0
	for i := range m.report {
		r := &m.report[i]
		n, err := m.pgCount(r.pgTable)
		if err != nil {
			return err
		}
		r.pgCount = n
		status := "OK"
		if r.pgCount != r.expected {
			status = "*** MISMATCH ***"
			mismatches++
		}
		fmt.Printf("%-38s %10d  %-24s %10d  %s\n", r.label, r.expected, r.pgTable, r.pgCount, status)
	}
	fmt.Println("-------------------------------------------------------------------------------------------------------")
	if mismatches == 0 {
		fmt.Println("row counts: ALL OK")
	} else {
		fmt.Printf("row counts: %d MISMATCH(es)\n", mismatches)
	}

	if err := m.reconcileMoney(); err != nil {
		return err
	}
	fmt.Println()
	if mismatches > 0 {
		fmt.Printf("RECONCILIATION FINISHED WITH %d COUNT MISMATCH(ES)\n", mismatches)
	} else {
		fmt.Println("RECONCILIATION FINISHED: counts match.")
	}
	return nil
}

func (m *Migrator) reconcileMoney() error {
	fmt.Println()
	fmt.Println("==================== RECONCILIATION: MONEY ====================")

	// Suppliers: stored balance vs sum(amount) of linked transactions.
	fmt.Println("-- suppliers: stored balance vs SUM(transactions.amount) where supplier_id --")
	type srow struct {
		ID      string
		Name    string
		Balance int64
		Summed  int64
	}
	var srows []srow
	if err := m.pg.Raw(`
		SELECT s.id, s.name, s.balance,
		       COALESCE((SELECT SUM(t.amount) FROM transactions t WHERE t.supplier_id = s.id), 0) AS summed
		FROM suppliers s ORDER BY s.id`).Scan(&srows).Error; err != nil {
		return err
	}
	moneyTable(len(srows) == 0)
	for _, r := range srows {
		fmt.Printf("  %-24s stored=%-12d txn_sum=%-12d %s\n", trunc(r.Name, 24), r.Balance, r.Summed, okDiff(r.Balance == r.Summed))
	}

	// Journals: stored total vs sum(amount) of linked transactions.
	fmt.Println("-- journals: stored total vs SUM(transactions.amount) where journal_id --")
	type jrow struct {
		ID     string
		Total  int64
		Summed int64
	}
	var jrows []jrow
	if err := m.pg.Raw(`
		SELECT j.id, j.total,
		       COALESCE((SELECT SUM(t.amount) FROM transactions t WHERE t.journal_id = j.id), 0) AS summed
		FROM journals j ORDER BY j.id`).Scan(&jrows).Error; err != nil {
		return err
	}
	moneyTable(len(jrows) == 0)
	for _, r := range jrows {
		fmt.Printf("  %-26s stored=%-12d txn_sum=%-12d %s\n", r.ID, r.Total, r.Summed, okDiff(r.Total == r.Summed))
	}

	// Branches: informational (direction was lost, so a signed net balance
	// cannot be recomputed from transactions). We print stored balances next to
	// the raw sum of the branch's transaction amounts.
	fmt.Println("-- branches: stored balances vs SUM(transactions.amount) [informational: direction lost] --")
	type brow struct {
		BranchID      string
		BranchName    string
		BalanceCash   int64
		BalanceBank   int64
		BalanceTerm   int64
		BalanceMobile int64
		TotalIncome   int64
		TotalExpenses int64
		Debt          int64
		TxnSum        int64
		TxnCount      int64
	}
	var brows []brow
	if err := m.pg.Raw(`
		SELECT bf.branch_id, bf.branch_name,
		       bf.balance_cash AS balance_cash, bf.balance_bank AS balance_bank,
		       bf.balance_terminal AS balance_term, bf.balance_mobile_apps AS balance_mobile,
		       bf.total_income, bf.total_expenses, bf.debt,
		       COALESCE((SELECT SUM(t.amount) FROM transactions t WHERE t.branch_id = bf.branch_id), 0) AS txn_sum,
		       COALESCE((SELECT COUNT(*)    FROM transactions t WHERE t.branch_id = bf.branch_id), 0) AS txn_count
		FROM branch_finance bf ORDER BY bf.branch_name`).Scan(&brows).Error; err != nil {
		return err
	}
	moneyTable(len(brows) == 0)
	for _, r := range brows {
		fmt.Printf("  %-16s cash=%d bank=%d term=%d mobile=%d | income=%d expenses=%d debt=%d | txns=%d sum=%d\n",
			trunc(r.BranchName, 16), r.BalanceCash, r.BalanceBank, r.BalanceTerm, r.BalanceMobile,
			r.TotalIncome, r.TotalExpenses, r.Debt, r.TxnCount, r.TxnSum)
	}

	// transaction_type caveat.
	var nullDir int64
	if err := m.pg.Raw(`SELECT COUNT(*) FROM transactions WHERE transaction_type IS NULL`).Scan(&nullDir).Error; err != nil {
		return err
	}
	fmt.Printf("-- caveat: transactions with transaction_type (credit/debit) NULL: %d (direction not persisted in Mongo)\n", nullDir)
	if m.skippedProductCategories > 0 {
		fmt.Printf("-- caveat: %d product->category reference(s) skipped (referenced categories absent from source)\n", m.skippedProductCategories)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func (m *Migrator) findAll(collection string) ([]bson.M, error) {
	cur, err := m.mdb.Collection(collection).Find(m.ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var docs []bson.M
	if err := cur.All(m.ctx, &docs); err != nil {
		return nil, err
	}
	logf("  read %d documents from %q", len(docs), collection)
	return docs, nil
}

// inBatches writes docs in chunks of batchSize(), each chunk inside its own
// transaction, so a large collection is not held in a single giant transaction.
// fn receives the tx handle and the slice of docs for that chunk. Counters that
// accumulate across chunks must be declared in the caller's scope (they are
// captured by the closure and survive between batches). Because each chunk
// commits independently, a mid-run failure leaves earlier chunks persisted; the
// ETL is meant to be re-run into a fresh/empty target. label is used only for
// progress logging.
func (m *Migrator) inBatches(label string, docs []bson.M, fn func(tx *gorm.DB, batch []bson.M) error) error {
	n := batchSize()
	total := len(docs)
	for start := 0; start < total; start += n {
		end := min(start+n, total)
		batch := docs[start:end]
		if err := m.pg.Transaction(func(tx *gorm.DB) error {
			return fn(tx, batch)
		}); err != nil {
			return fmt.Errorf("%s batch [%d:%d): %w", label, start, end, err)
		}
		if total > n {
			logf("  %s: wrote %d/%d", label, end, total)
		}
	}
	return nil
}

func (m *Migrator) pgCount(table string) (int64, error) {
	var n int64
	if err := m.pg.Raw("SELECT COUNT(*) FROM " + table).Scan(&n).Error; err != nil {
		return 0, err
	}
	return n, nil
}

func (m *Migrator) record(label string, expected int64, tables ...string) {
	for _, t := range tables {
		m.report = append(m.report, reconRow{label: label, expected: expected, pgTable: t})
	}
}

// docID resolves a document's identifier to the string PG expects: a string _id
// is returned verbatim; an ObjectID _id is converted to its 24-char hex form.
func docID(d bson.M) string {
	switch v := d["_id"].(type) {
	case string:
		return v
	case bson.ObjectID:
		return v.Hex()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func getStr(d bson.M, key string) string {
	if d == nil {
		return ""
	}
	switch v := d[key].(type) {
	case string:
		return v
	case bson.ObjectID:
		return v.Hex()
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}

func getInt(d bson.M, key string) int64 {
	if d == nil {
		return 0
	}
	switch v := d[key].(type) {
	case int32:
		return int64(v)
	case int64:
		return v
	case int:
		return int64(v)
	case float64:
		return int64(v)
	case float32:
		return int64(v)
	default:
		return 0
	}
}

func getFloat(d bson.M, key string) float64 {
	if d == nil {
		return 0
	}
	switch v := d[key].(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case int:
		return float64(v)
	default:
		return 0
	}
}

func getBool(d bson.M, key string) bool {
	if d == nil {
		return false
	}
	b, _ := d[key].(bool)
	return b
}

func getBoolDefault(d bson.M, key string, def bool) bool {
	if d == nil {
		return def
	}
	if v, ok := d[key].(bool); ok {
		return v
	}
	return def
}

func getTime(d bson.M, key string) time.Time {
	if d == nil {
		return time.Time{}
	}
	switch v := d[key].(type) {
	case bson.DateTime:
		return v.Time()
	case time.Time:
		return v
	case string:
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
		return time.Time{}
	default:
		return time.Time{}
	}
}

func getDoc(d bson.M, key string) bson.M {
	if d == nil {
		return nil
	}
	return asDoc(d[key])
}

// asDoc coerces any decoded BSON value to a bson.M. When decoding into a
// bson.M, the mongo-driver decodes nested subdocuments (both direct and inside
// arrays) as the ordered bson.D type, so every nested-document access must go
// through this helper.
func asDoc(v interface{}) bson.M {
	switch t := v.(type) {
	case bson.M:
		return t
	case map[string]interface{}:
		return bson.M(t)
	case bson.D:
		m := make(bson.M, len(t))
		for _, e := range t {
			m[e.Key] = e.Value
		}
		return m
	default:
		return nil
	}
}

// toPlain recursively rewrites a decoded BSON value into JSON-friendly Go types
// (map/slice/scalars) so that json.Marshal produces a natural jsonb value. In
// particular it flattens bson.D (which would otherwise marshal as a list of
// {Key,Value} objects) and renders ObjectID/DateTime as strings.
func toPlain(v interface{}) interface{} {
	switch t := v.(type) {
	case bson.D:
		m := make(map[string]interface{}, len(t))
		for _, e := range t {
			m[e.Key] = toPlain(e.Value)
		}
		return m
	case bson.M:
		m := make(map[string]interface{}, len(t))
		for k, val := range t {
			m[k] = toPlain(val)
		}
		return m
	case map[string]interface{}:
		m := make(map[string]interface{}, len(t))
		for k, val := range t {
			m[k] = toPlain(val)
		}
		return m
	case bson.A:
		a := make([]interface{}, len(t))
		for i, e := range t {
			a[i] = toPlain(e)
		}
		return a
	case []interface{}:
		a := make([]interface{}, len(t))
		for i, e := range t {
			a[i] = toPlain(e)
		}
		return a
	case bson.DateTime:
		return t.Time().UTC().Format(time.RFC3339)
	case bson.ObjectID:
		return t.Hex()
	default:
		return t
	}
}

func getArray(d bson.M, key string) bson.A {
	if d == nil {
		return nil
	}
	switch v := d[key].(type) {
	case bson.A:
		return v
	case []interface{}:
		return bson.A(v)
	default:
		return nil
	}
}

func getStrArray(d bson.M, key string) []string {
	arr := getArray(d, key)
	out := make([]string, 0, len(arr))
	for _, e := range arr {
		switch v := e.(type) {
		case string:
			out = append(out, v)
		case bson.ObjectID:
			out = append(out, v.Hex())
		}
	}
	return out
}

// toJSONB marshals a bson value for a jsonb column, returning nil (SQL NULL)
// for absent/nil values.
func toJSONB(v interface{}) interface{} {
	if v == nil {
		return nil
	}
	b, err := json.Marshal(toPlain(v))
	if err != nil || string(b) == "null" {
		return nil
	}
	return string(b)
}

func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

func strOrDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// firstNonEmpty returns the first non-empty string among its arguments.
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

func ptrOrNil(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func timePtr(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

func orNow(t time.Time) time.Time {
	if t.IsZero() {
		return time.Now()
	}
	return t
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func okDiff(ok bool) string {
	if ok {
		return "OK"
	}
	return "DIFF"
}

func moneyTable(empty bool) {
	if empty {
		fmt.Println("  (none)")
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// defaultBatchSize is the number of documents written per transaction for the
// large collections when ETL_BATCH_SIZE is unset.
const defaultBatchSize = 500

// batchSize resolves the per-batch document count from ETL_BATCH_SIZE, falling
// back to defaultBatchSize for an unset/invalid/non-positive value.
func batchSize() int {
	if v := os.Getenv("ETL_BATCH_SIZE"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return defaultBatchSize
}

func logf(format string, args ...interface{}) {
	fmt.Printf("[etl] "+format+"\n", args...)
}

func fatalf(format string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, "[etl] FATAL: "+format+"\n", args...)
	os.Exit(1)
}
