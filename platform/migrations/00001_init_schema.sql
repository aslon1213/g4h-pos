-- Initial PostgreSQL schema for the POS-ERP, migrated from MongoDB.
--
-- Design = "hybrid": the money/transactional core is normalized into real
-- tables with foreign keys; genuinely free-form fields (additional_info,
-- activity data, finance details, purchase history, product images) are jsonb.
--
-- Conventions:
--   * Primary keys are `text` (the app supplies uuid strings; journals/proposals
--     ObjectIDs are converted to text by the ETL). Child tables with no natural
--     id use bigserial.
--   * POS money (amounts, balances, journal totals) is `integer` (matches the
--     int32/uint32 Go fields). Storefront money (cart/order/promotion) is
--     `double precision` (matches the float64 Go fields).
--   * Timestamps are `timestamptz`.
--
-- Apply with goose:  goose -dir platform/migrations postgres "$DATABASE_DSN" up
--
-- Branch rows are NOT seeded here: the ETL creates them from the existing
-- finance documents (authoritative branch_ids). For a greenfield/dev DB, seed
-- branches + branch_finance separately.

-- +goose Up

-- ---------------------------------------------------------------------------
-- Core reference tables
-- ---------------------------------------------------------------------------

CREATE TABLE branches (
    id         text PRIMARY KEY,
    name       text NOT NULL UNIQUE,
    location   text,
    phone      text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE branch_finance (
    branch_id            text PRIMARY KEY REFERENCES branches (id) ON DELETE CASCADE,
    branch_name          text NOT NULL,
    balance_cash         integer NOT NULL DEFAULT 0,
    balance_bank         integer NOT NULL DEFAULT 0,
    balance_terminal     integer NOT NULL DEFAULT 0,
    balance_mobile_apps  integer NOT NULL DEFAULT 0,
    total_income         integer NOT NULL DEFAULT 0,
    total_expenses       integer NOT NULL DEFAULT 0,
    debt                 integer NOT NULL DEFAULT 0,
    details              jsonb,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX branch_finance_branch_name_idx ON branch_finance (branch_name);

CREATE TABLE users (
    id       text PRIMARY KEY,
    email    text,
    username text NOT NULL,
    password text NOT NULL,
    role     text,
    phone    text,
    branch   text
);
CREATE UNIQUE INDEX users_username_idx ON users (username);

CREATE TABLE activities (
    id      bigserial PRIMARY KEY,
    user_id text,
    action  text,
    data    jsonb,
    ip      text,
    date    timestamptz NOT NULL DEFAULT now(),
    status  integer
);
CREATE INDEX activities_date_idx ON activities (date DESC);
CREATE INDEX activities_user_date_idx ON activities (user_id, date DESC);

-- ---------------------------------------------------------------------------
-- Catalog
-- ---------------------------------------------------------------------------

CREATE TABLE brands (
    id      text PRIMARY KEY,
    name    text NOT NULL,
    slug    text,
    logo    text,
    country text
);
CREATE INDEX brands_slug_idx ON brands (slug);

CREATE TABLE categories (
    id          text PRIMARY KEY,
    name        text NOT NULL,
    slug        text,
    parent_id   text REFERENCES categories (id) ON DELETE SET NULL,
    description text,
    image       text,
    sort_order  integer NOT NULL DEFAULT 0,
    is_active   boolean NOT NULL DEFAULT true,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX categories_slug_idx ON categories (slug);
CREATE INDEX categories_parent_idx ON categories (parent_id);

-- ---------------------------------------------------------------------------
-- Suppliers (financial_data flattened onto the row; embedded transactions
-- normalized into the transactions table via supplier_id)
-- ---------------------------------------------------------------------------

CREATE TABLE suppliers (
    id             text PRIMARY KEY,
    name           text NOT NULL,
    address        text,
    phone          text,
    email          text,
    inn            text,
    notes          text,
    branch_id      text REFERENCES branches (id) ON DELETE SET NULL,
    balance        integer NOT NULL DEFAULT 0,
    total_income   integer NOT NULL DEFAULT 0,
    total_expenses integer NOT NULL DEFAULT 0,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX suppliers_branch_idx ON suppliers (branch_id);

-- ---------------------------------------------------------------------------
-- Products + child tables (was quantity_distribution[]/income_history[]/
-- category[]); images kept as jsonb (leaf list of S3 urls)
-- ---------------------------------------------------------------------------

CREATE TABLE products (
    id                   text PRIMARY KEY,
    name                 text NOT NULL,
    description          text,
    manufacturer_name    text,
    manufacturer_country text,
    manufacturer_address text,
    manufacturer_phone   text,
    manufacturer_email   text,
    brand_id             text REFERENCES brands (id) ON DELETE SET NULL,
    sku                  text,
    minimum_stock_alert  integer NOT NULL DEFAULT 0,
    general_income_price double precision NOT NULL DEFAULT 0,
    images               jsonb NOT NULL DEFAULT '[]'::jsonb,
    rating               double precision NOT NULL DEFAULT 0,
    review_count         integer NOT NULL DEFAULT 0,
    created_at           timestamptz NOT NULL DEFAULT now(),
    updated_at           timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX products_brand_idx ON products (brand_id);
CREATE INDEX products_sku_idx ON products (sku);

CREATE TABLE product_categories (
    product_id  text NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    category_id text NOT NULL REFERENCES categories (id) ON DELETE CASCADE,
    PRIMARY KEY (product_id, category_id)
);
CREATE INDEX product_categories_category_idx ON product_categories (category_id);

CREATE TABLE product_stock (
    id         bigserial PRIMARY KEY,
    product_id text NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    place_id   text NOT NULL,
    place_type text,
    quantity   integer NOT NULL DEFAULT 0,
    unit       text,
    price      integer NOT NULL DEFAULT 0,
    UNIQUE (product_id, place_id)
);

CREATE TABLE product_income_history (
    id          bigserial PRIMARY KEY,
    product_id  text NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    date        text,
    price       integer NOT NULL DEFAULT 0,
    quantity    integer NOT NULL DEFAULT 0,
    place_id    text,
    place_type  text,
    supplier_id text REFERENCES suppliers (id) ON DELETE SET NULL,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX product_income_history_product_idx ON product_income_history (product_id);

-- ---------------------------------------------------------------------------
-- POS customers + BNPL (BNPLs were embedded in customers.bnpls[])
-- ---------------------------------------------------------------------------

CREATE TABLE customers (
    id               text PRIMARY KEY,
    name             text,
    phone            text,
    address          text,
    additional_info  jsonb,
    purchase_history jsonb,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE bnpls (
    id           text PRIMARY KEY,
    customer_id  text NOT NULL REFERENCES customers (id) ON DELETE CASCADE,
    branch_id    text REFERENCES branches (id) ON DELETE SET NULL,
    total_amount integer NOT NULL DEFAULT 0,
    paid_amount  integer NOT NULL DEFAULT 0,
    status       text NOT NULL,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX bnpls_customer_idx ON bnpls (customer_id);
CREATE INDEX bnpls_branch_status_idx ON bnpls (branch_id, status);

CREATE TABLE bnpl_products (
    id         bigserial PRIMARY KEY,
    bnpl_id    text NOT NULL REFERENCES bnpls (id) ON DELETE CASCADE,
    product_id text NOT NULL,
    quantity   integer NOT NULL DEFAULT 0,
    price      integer NOT NULL DEFAULT 0,
    UNIQUE (bnpl_id, product_id)
);

-- ---------------------------------------------------------------------------
-- Journals (one shift per branch/day) + transactions
-- transactions replaces journals.operations[] (journal_id), the embedded
-- supplier transactions (supplier_id) and bnpl.transactions[] (bnpl_id).
-- ---------------------------------------------------------------------------

CREATE TABLE journals (
    id              text PRIMARY KEY,
    branch_id       text REFERENCES branches (id) ON DELETE SET NULL,
    branch_name     text,
    branch_location text,
    branch_phone    text,
    date            timestamptz NOT NULL,
    shift_is_closed boolean NOT NULL DEFAULT false,
    terminal_income integer NOT NULL DEFAULT 0,
    cash_left       integer NOT NULL DEFAULT 0,
    total           integer NOT NULL DEFAULT 0,
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX journals_date_branch_idx ON journals (date, branch_id);

CREATE TABLE transactions (
    id               text PRIMARY KEY,
    amount           integer NOT NULL DEFAULT 0,
    description      text,
    transaction_type text,   -- credit | debit  (was TransactionBase.Type)
    initiator_type   text,   -- sale | supplier | bnpl | salary | ...  (was Transaction.Type)
    payment_method   text,
    branch_id        text REFERENCES branches (id) ON DELETE SET NULL,
    journal_id       text REFERENCES journals (id) ON DELETE SET NULL,
    supplier_id      text REFERENCES suppliers (id) ON DELETE SET NULL,
    bnpl_id          text REFERENCES bnpls (id) ON DELETE SET NULL,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX transactions_branch_idx ON transactions (branch_id);
CREATE INDEX transactions_journal_idx ON transactions (journal_id);
CREATE INDEX transactions_supplier_idx ON transactions (supplier_id);
CREATE INDEX transactions_bnpl_idx ON transactions (bnpl_id);
CREATE INDEX transactions_created_at_idx ON transactions (created_at DESC);

-- ---------------------------------------------------------------------------
-- Storefront: accounts + address book
-- ---------------------------------------------------------------------------

CREATE TABLE store_customers (
    id             text PRIMARY KEY,
    email          text NOT NULL,
    phone          text,
    name           text,
    password_hash  text NOT NULL,
    email_verified boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX store_customers_email_idx ON store_customers (email);

CREATE TABLE addresses (
    id          text PRIMARY KEY,
    customer_id text NOT NULL REFERENCES store_customers (id) ON DELETE CASCADE,
    label       text,
    full_name   text,
    phone       text,
    line1       text,
    line2       text,
    city        text,
    region      text,
    postal_code text,
    country     text,
    is_default  boolean NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX addresses_customer_idx ON addresses (customer_id);

-- ---------------------------------------------------------------------------
-- Storefront: cart + wishlist (one per customer)
-- ---------------------------------------------------------------------------

CREATE TABLE carts (
    id          text PRIMARY KEY,
    customer_id text NOT NULL REFERENCES store_customers (id) ON DELETE CASCADE,
    coupon_code text,
    subtotal    double precision NOT NULL DEFAULT 0,
    discount    double precision NOT NULL DEFAULT 0,
    total       double precision NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX carts_customer_idx ON carts (customer_id);

CREATE TABLE cart_items (
    id         text PRIMARY KEY,
    cart_id    text NOT NULL REFERENCES carts (id) ON DELETE CASCADE,
    product_id text NOT NULL,
    name       text,
    image      text,
    quantity   integer NOT NULL DEFAULT 0,
    unit_price double precision NOT NULL DEFAULT 0,
    line_total double precision NOT NULL DEFAULT 0
);
CREATE INDEX cart_items_cart_idx ON cart_items (cart_id);

CREATE TABLE wishlists (
    id          text PRIMARY KEY,
    customer_id text NOT NULL REFERENCES store_customers (id) ON DELETE CASCADE,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX wishlists_customer_idx ON wishlists (customer_id);

CREATE TABLE wishlist_items (
    id          bigserial PRIMARY KEY,
    wishlist_id text NOT NULL REFERENCES wishlists (id) ON DELETE CASCADE,
    product_id  text NOT NULL,
    name        text,
    image       text,
    price       double precision NOT NULL DEFAULT 0,
    added_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (wishlist_id, product_id)
);

-- ---------------------------------------------------------------------------
-- Storefront: orders (totals + address snapshot frozen onto the row)
-- ---------------------------------------------------------------------------

CREATE TABLE orders (
    id                  text PRIMARY KEY,
    number              text NOT NULL,
    customer_id         text REFERENCES store_customers (id) ON DELETE SET NULL,
    status              text NOT NULL,
    coupon_code         text,
    note                text,
    -- totals (embedded OrderTotals)
    subtotal            double precision NOT NULL DEFAULT 0,
    discount            double precision NOT NULL DEFAULT 0,
    shipping            double precision NOT NULL DEFAULT 0,
    tax                 double precision NOT NULL DEFAULT 0,
    total               double precision NOT NULL DEFAULT 0,
    -- address snapshot (frozen copy of the Address at order time)
    address_id          text,
    address_customer_id text,
    address_label       text,
    address_full_name   text,
    address_phone       text,
    address_line1       text,
    address_line2       text,
    address_city        text,
    address_region      text,
    address_postal_code text,
    address_country     text,
    address_is_default  boolean NOT NULL DEFAULT false,
    created_at          timestamptz NOT NULL DEFAULT now(),
    updated_at          timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX orders_number_idx ON orders (number);
CREATE INDEX orders_customer_created_idx ON orders (customer_id, created_at DESC);

CREATE TABLE order_items (
    id         bigserial PRIMARY KEY,
    order_id   text NOT NULL REFERENCES orders (id) ON DELETE CASCADE,
    product_id text NOT NULL,
    name       text,
    image      text,
    quantity   integer NOT NULL DEFAULT 0,
    unit_price double precision NOT NULL DEFAULT 0,
    line_total double precision NOT NULL DEFAULT 0
);
CREATE INDEX order_items_order_idx ON order_items (order_id);

-- ---------------------------------------------------------------------------
-- Storefront: reviews (+ helpful-vote dedupe) and promotions
-- ---------------------------------------------------------------------------

CREATE TABLE reviews (
    id            text PRIMARY KEY,
    product_id    text NOT NULL REFERENCES products (id) ON DELETE CASCADE,
    customer_id   text NOT NULL,
    customer_name text,
    rating        integer NOT NULL,
    title         text,
    body          text,
    helpful_votes integer NOT NULL DEFAULT 0,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (product_id, customer_id)
);
CREATE INDEX reviews_product_created_idx ON reviews (product_id, created_at DESC);

CREATE TABLE review_votes (
    review_id   text NOT NULL REFERENCES reviews (id) ON DELETE CASCADE,
    customer_id text NOT NULL,
    PRIMARY KEY (review_id, customer_id)
);

CREATE TABLE promotions (
    id            text PRIMARY KEY,
    title         text,
    description   text,
    banner        text,
    code          text,
    discount_type text,
    value         double precision NOT NULL DEFAULT 0,
    min_subtotal  double precision NOT NULL DEFAULT 0,
    usage_limit   integer NOT NULL DEFAULT 0,
    used_count    integer NOT NULL DEFAULT 0,
    is_active     boolean NOT NULL DEFAULT true,
    starts_at     timestamptz,
    ends_at       timestamptz,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX promotions_code_idx ON promotions (code);

-- ---------------------------------------------------------------------------
-- Arrivals / proposals
-- ---------------------------------------------------------------------------

CREATE TABLE proposals (
    id         text PRIMARY KEY,
    name       text,
    date       timestamptz,
    branch     text,
    fulfilled  boolean NOT NULL DEFAULT false,
    image_file text
);

-- internal_expenses is a placeholder: the controller is currently unimplemented
-- (handlers panic). Kept so the collection reference has a relational home.
CREATE TABLE internal_expenses (
    id          text PRIMARY KEY,
    branch_id   text REFERENCES branches (id) ON DELETE SET NULL,
    amount      integer NOT NULL DEFAULT 0,
    description text,
    data        jsonb,
    created_at  timestamptz NOT NULL DEFAULT now()
);

-- +goose Down

DROP TABLE IF EXISTS internal_expenses;
DROP TABLE IF EXISTS proposals;
DROP TABLE IF EXISTS promotions;
DROP TABLE IF EXISTS review_votes;
DROP TABLE IF EXISTS reviews;
DROP TABLE IF EXISTS order_items;
DROP TABLE IF EXISTS orders;
DROP TABLE IF EXISTS wishlist_items;
DROP TABLE IF EXISTS wishlists;
DROP TABLE IF EXISTS cart_items;
DROP TABLE IF EXISTS carts;
DROP TABLE IF EXISTS addresses;
DROP TABLE IF EXISTS store_customers;
DROP TABLE IF EXISTS transactions;
DROP TABLE IF EXISTS journals;
DROP TABLE IF EXISTS bnpl_products;
DROP TABLE IF EXISTS bnpls;
DROP TABLE IF EXISTS customers;
DROP TABLE IF EXISTS product_income_history;
DROP TABLE IF EXISTS product_stock;
DROP TABLE IF EXISTS product_categories;
DROP TABLE IF EXISTS products;
DROP TABLE IF EXISTS suppliers;
DROP TABLE IF EXISTS categories;
DROP TABLE IF EXISTS brands;
DROP TABLE IF EXISTS activities;
DROP TABLE IF EXISTS users;
DROP TABLE IF EXISTS branch_finance;
DROP TABLE IF EXISTS branches;
