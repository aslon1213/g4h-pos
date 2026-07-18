-- Unified POS-side carts: Scan & Go handoff carts and till (POS) carts now share
-- one header table (`sale_carts`) and one items table (`sale_cart_items`).
--
-- Why: a sale-type transaction needs exactly one place to point at so staff can
-- open a journal operation and see what was actually sold. Before this, handoff
-- carts lived in `handoff_carts`/`handoff_cart_lines` and POS "carts" were an
-- ephemeral Redis map (models.SalesSession) with no rows at all — so a sale
-- transaction had nothing to reference.
--
-- NOT in scope: the storefront cart (`carts`/`cart_items`). That is a separate
-- customer-facing basket with its own money semantics (double precision) and is
-- deliberately left untouched.
--
-- Conventions:
--   * text uuid primary keys supplied by the app (as in 00001/00003).
--   * MONEY IS INTEGER so'm. We never carry fractional money: a line's total is
--     rounded to the nearest so'm at write time (round-half-away-from-zero), and
--     a cart total is the exact integer SUM of already-rounded line totals. This
--     keeps the cart arithmetic identical to the integer ledger downstream
--     (transactions.amount, journals.total), so no rounding happens at checkout.
--   * QUANTITY IS FRACTIONAL — numeric(12,3) — because weighted goods sell by
--     the kilogram (1.500 kg). This is the only fractional dimension.

-- +goose Up

-- ---------------------------------------------------------------------------
-- Fractional quantities: stock is decremented by cart quantity at checkout, so
-- the stock counters must carry the same precision as a cart line or a weighted
-- sale could not be subtracted correctly. Widening integer -> numeric preserves
-- every existing value exactly.
-- ---------------------------------------------------------------------------

ALTER TABLE product_stock
    ALTER COLUMN quantity TYPE numeric(12,3) USING quantity::numeric(12,3);

ALTER TABLE product_income_history
    ALTER COLUMN quantity TYPE numeric(12,3) USING quantity::numeric(12,3);

-- ---------------------------------------------------------------------------
-- Unified cart header
-- ---------------------------------------------------------------------------

CREATE TABLE sale_carts (
    id        text PRIMARY KEY,

    -- Which surface owns this cart. Gates the legal state transitions (see the
    -- kind-aware transition map in pkg/models) and whether a handoff satellite
    -- row exists.
    kind      text NOT NULL CHECK (kind IN ('handoff', 'pos')),

    branch_id text NOT NULL REFERENCES branches (id) ON DELETE CASCADE,

    -- Shared lifecycle. 'ready_for_handoff' and 'claimed' are reachable only by
    -- kind='handoff'; a POS cart goes active -> completed|cancelled|expired.
    state     text NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'ready_for_handoff', 'claimed', 'completed', 'expired', 'cancelled')),

    -- POS: the staff user who opened the cart at the till.
    -- Handoff: the seller who claimed it (set on claim, cleared on release).
    seller_id text,

    -- Exact integer sum of sale_cart_items.line_total (already-rounded so'm).
    subtotal  integer NOT NULL DEFAULT 0,

    -- Checkout bookkeeping (idempotent via the stored key; the sale is recorded
    -- on a shift journal through the existing journal operation flow).
    checkout_idempotency_key text,
    sale_journal_id          text,

    finalized_at timestamptz,  -- set when reaching a terminal state
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX sale_carts_branch_state_idx ON sale_carts (branch_id, state);
CREATE INDEX sale_carts_kind_state_idx   ON sale_carts (kind, state);
-- Supports the janitor sweeping stale non-terminal carts (it replaces the Redis
-- TTL that used to expire POS sessions).
CREATE INDEX sale_carts_state_updated_idx ON sale_carts (state, updated_at);

-- ---------------------------------------------------------------------------
-- Unified cart items — the single shared items table
-- ---------------------------------------------------------------------------

CREATE TABLE sale_cart_items (
    id         text PRIMARY KEY,
    cart_id    text NOT NULL REFERENCES sale_carts (id) ON DELETE CASCADE,
    product_id text NOT NULL,

    -- Snapshot of the product at scan time, so a later catalog edit does not
    -- rewrite the history of a completed sale.
    name       text,
    sku        text,
    uom        text,

    -- Fractional: 1.500 kg. The only non-integer column here.
    quantity   numeric(12,3) NOT NULL DEFAULT 0,

    -- Integer so'm. unit_price is the authoritative price copied from
    -- product_stock at the cart's branch; line_total is
    -- round(quantity * unit_price) computed at write time.
    unit_price integer NOT NULL DEFAULT 0,
    line_total integer NOT NULL DEFAULT 0
);

CREATE INDEX sale_cart_items_cart_idx    ON sale_cart_items (cart_id);
CREATE INDEX sale_cart_items_product_idx ON sale_cart_items (product_id);

-- ---------------------------------------------------------------------------
-- Handoff satellite: the Scan & Go credentials and claim bookkeeping.
--
-- These columns are meaningless for a POS cart (which is opened by an
-- authenticated staff user at a till, not by scanning an entry QR), so they live
-- in a satellite rather than as always-null columns on sale_carts.
--
-- Two token types, stored ONLY as hashes (never the plaintext):
--   * session_token — high-entropy (>=128 bits), bearer of the entry-QR link;
--     lives for the shopping trip (expires at end of its created day).
--   * handoff token — minted at checkout: a low-entropy typed code (safe only
--     because it is single-use, short-lived, state-gated and rate-limited) plus
--     a higher-entropy QR reference.
-- ---------------------------------------------------------------------------

CREATE TABLE handoff_cart_sessions (
    cart_id              text PRIMARY KEY REFERENCES sale_carts (id) ON DELETE CASCADE,

    session_token_hash   text NOT NULL,
    session_expires_at   timestamptz NOT NULL,

    handoff_code_hash    text,
    handoff_ref_hash     text,
    handoff_expires_at   timestamptz,
    handoff_attempts     integer NOT NULL DEFAULT 0,
    handoff_locked_until  timestamptz,

    claimed_at           timestamptz
);

-- The session token is the credential for the customer endpoints: unique + the
-- primary lookup key.
CREATE UNIQUE INDEX handoff_sessions_token_idx ON handoff_cart_sessions (session_token_hash);

-- The typed handoff code must be unambiguous among carts currently awaiting
-- handoff, so claim-by-code resolves to exactly one cart.
--
-- The 00003 version of this index scoped uniqueness to state = 'ready_for_handoff'.
-- State now lives on sale_carts, and a partial index cannot reference another
-- table, so the equivalent local predicate is claimed_at IS NULL: a token stops
-- needing to be unique the moment it is spent. Claimed carts keep their hash so
-- ClaimByToken can still resolve an idempotent double-tap by the same seller.
--
-- Uniqueness keys on PRESENCE, so an un-minted token MUST be NULL. The pre-00004
-- app wrote '' instead (the Go field was a plain string), which is present as far
-- as Postgres is concerned — hence the NULLIF normalisation in the data copy
-- below, and the pointer fields in models.HandoffSession.
CREATE UNIQUE INDEX handoff_sessions_code_idx
    ON handoff_cart_sessions (handoff_code_hash)
    WHERE handoff_code_hash IS NOT NULL AND claimed_at IS NULL;

CREATE INDEX handoff_sessions_expiry_idx ON handoff_cart_sessions (session_expires_at);

-- ---------------------------------------------------------------------------
-- Migrate existing handoff carts across, then retire the 00003 tables.
-- Money widens int -> int (unchanged) and quantity int -> numeric, so every
-- existing row carries over exactly.
-- ---------------------------------------------------------------------------

INSERT INTO sale_carts (
    id, kind, branch_id, state, seller_id, subtotal,
    checkout_idempotency_key, sale_journal_id, finalized_at, created_at, updated_at
)
SELECT
    c.id,
    'handoff',
    c.branch_id,
    c.state,
    c.claimed_by,
    COALESCE((SELECT SUM(l.line_total) FROM handoff_cart_lines l WHERE l.cart_id = c.id), 0),
    c.checkout_idempotency_key,
    c.sale_journal_id,
    c.finalized_at,
    c.created_at,
    c.updated_at
FROM handoff_carts c;

INSERT INTO sale_cart_items (id, cart_id, product_id, name, sku, uom, quantity, unit_price, line_total)
SELECT
    l.id, l.cart_id, l.product_id, l.name, l.sku, l.uom,
    l.quantity::numeric(12,3),
    l.display_price,
    l.line_total
FROM handoff_cart_lines l;

INSERT INTO handoff_cart_sessions (
    cart_id, session_token_hash, session_expires_at,
    handoff_code_hash, handoff_ref_hash, handoff_expires_at,
    handoff_attempts, handoff_locked_until, claimed_at
)
SELECT
    c.id, c.session_token_hash, c.session_expires_at,
    -- '' was the pre-00004 sentinel for "no token minted". It must become NULL:
    -- the partial unique index above keys on presence, and every cart that never
    -- reached checkout carries '', so copying them verbatim collides on the first
    -- duplicate.
    NULLIF(c.handoff_code_hash, ''),
    NULLIF(c.handoff_ref_hash, ''),
    c.handoff_expires_at,
    c.handoff_attempts, c.handoff_locked_until, c.claimed_at
FROM handoff_carts c;

DROP TABLE IF EXISTS handoff_cart_lines;
DROP TABLE IF EXISTS handoff_carts;

-- ---------------------------------------------------------------------------
-- Sale-type transactions point at the cart that produced them.
--
-- NULLABLE even for initiator_type='sale': a keyed/manual sale (the amount-only
-- POST /api/sales/transactions/:branch_id, and a journal operation added with
-- supplier_transaction=false) has no line items and therefore no cart. The CHECK
-- below only forbids a transaction carrying a foreign key belonging to a
-- DIFFERENT initiator type; it never requires one to be present.
-- ---------------------------------------------------------------------------

ALTER TABLE transactions ADD COLUMN cart_id text REFERENCES sale_carts (id) ON DELETE SET NULL;
CREATE INDEX transactions_cart_idx ON transactions (cart_id);

-- Recover the cart link for handoff sales recorded before this migration: the
-- checkout flow embedded the cart id in the operation description.
UPDATE transactions
SET cart_id = substring(description from '^Scan & Go handoff (.+)$')
WHERE initiator_type = 'sale'
  AND description LIKE 'Scan & Go handoff %'
  AND substring(description from '^Scan & Go handoff (.+)$') IN (SELECT id FROM sale_carts);

ALTER TABLE transactions ADD CONSTRAINT transactions_initiator_refs_chk CHECK (
       (initiator_type = 'sale'     AND supplier_id IS NULL AND bnpl_id IS NULL)
    OR (initiator_type = 'supplier' AND cart_id IS NULL AND bnpl_id IS NULL)
    OR (initiator_type = 'bnpl'     AND cart_id IS NULL AND supplier_id IS NULL)
    OR (initiator_type NOT IN ('sale', 'supplier', 'bnpl')
        AND cart_id IS NULL AND supplier_id IS NULL AND bnpl_id IS NULL)
    OR initiator_type IS NULL
);

-- +goose Down

ALTER TABLE transactions DROP CONSTRAINT IF EXISTS transactions_initiator_refs_chk;
DROP INDEX IF EXISTS transactions_cart_idx;
ALTER TABLE transactions DROP COLUMN IF EXISTS cart_id;

-- Recreate the 00003 tables and move the handoff carts back. POS carts have no
-- pre-00004 representation (they were an ephemeral Redis map) and are dropped.

CREATE TABLE handoff_carts (
    id                       text PRIMARY KEY,
    branch_id                text NOT NULL REFERENCES branches (id) ON DELETE CASCADE,
    state                    text NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'ready_for_handoff', 'claimed', 'completed', 'expired', 'cancelled')),
    session_token_hash       text NOT NULL,
    session_expires_at       timestamptz NOT NULL,
    handoff_code_hash        text,
    handoff_ref_hash         text,
    handoff_expires_at       timestamptz,
    handoff_attempts         integer NOT NULL DEFAULT 0,
    handoff_locked_until     timestamptz,
    claimed_by               text,
    claimed_at               timestamptz,
    checkout_idempotency_key text,
    sale_journal_id          text,
    finalized_at             timestamptz,
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX handoff_carts_session_token_idx ON handoff_carts (session_token_hash);
CREATE UNIQUE INDEX handoff_carts_handoff_code_idx
    ON handoff_carts (handoff_code_hash)
    WHERE handoff_code_hash IS NOT NULL AND state = 'ready_for_handoff';
CREATE INDEX handoff_carts_branch_state_idx ON handoff_carts (branch_id, state);
CREATE INDEX handoff_carts_state_expiry_idx ON handoff_carts (state, session_expires_at);

CREATE TABLE handoff_cart_lines (
    id            text PRIMARY KEY,
    cart_id       text NOT NULL REFERENCES handoff_carts (id) ON DELETE CASCADE,
    product_id    text NOT NULL,
    sku           text,
    name          text,
    uom           text,
    quantity      integer NOT NULL DEFAULT 0,
    display_price integer NOT NULL DEFAULT 0,
    line_total    integer NOT NULL DEFAULT 0
);
CREATE INDEX handoff_cart_lines_cart_idx ON handoff_cart_lines (cart_id);

INSERT INTO handoff_carts (
    id, branch_id, state, session_token_hash, session_expires_at,
    handoff_code_hash, handoff_ref_hash, handoff_expires_at, handoff_attempts,
    handoff_locked_until, claimed_by, claimed_at, checkout_idempotency_key,
    sale_journal_id, finalized_at, created_at, updated_at
)
SELECT
    c.id, c.branch_id, c.state, s.session_token_hash, s.session_expires_at,
    s.handoff_code_hash, s.handoff_ref_hash, s.handoff_expires_at, s.handoff_attempts,
    s.handoff_locked_until, c.seller_id, s.claimed_at, c.checkout_idempotency_key,
    c.sale_journal_id, c.finalized_at, c.created_at, c.updated_at
FROM sale_carts c
JOIN handoff_cart_sessions s ON s.cart_id = c.id
WHERE c.kind = 'handoff';

-- Fractional quantities cannot survive the round trip: rounding to integer is
-- the only way back into the old column type.
INSERT INTO handoff_cart_lines (id, cart_id, product_id, sku, name, uom, quantity, display_price, line_total)
SELECT i.id, i.cart_id, i.product_id, i.sku, i.name, i.uom,
       ROUND(i.quantity)::integer, i.unit_price, i.line_total
FROM sale_cart_items i
JOIN sale_carts c ON c.id = i.cart_id
WHERE c.kind = 'handoff';

DROP TABLE IF EXISTS handoff_cart_sessions;
DROP TABLE IF EXISTS sale_cart_items;
DROP TABLE IF EXISTS sale_carts;

ALTER TABLE product_income_history
    ALTER COLUMN quantity TYPE integer USING ROUND(quantity)::integer;

ALTER TABLE product_stock
    ALTER COLUMN quantity TYPE integer USING ROUND(quantity)::integer;
