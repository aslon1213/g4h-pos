-- Cart handoff sessions ("Scan & Go").
--
-- A customer scans a per-branch entry QR, which starts a SERVER-SIDE cart
-- (handoff_carts) they build by scanning products (handoff_cart_lines). At
-- checkout the customer mints a short-lived handoff token; a seller claims the
-- cart on their POS and charges it, delegating to the existing journal
-- operation (sale) flow.
--
-- Conventions (match 00001):
--   * text uuid primary keys supplied by the app.
--   * POS money is `integer` (display prices here are only for the customer's
--     benefit; the authoritative price is applied by the sale flow at checkout).
--   * timestamps are `timestamptz`.
--
-- Two token types, stored ONLY as hashes (never the plaintext):
--   * session_token — high-entropy (>=128 bits), bearer of the entry-QR link;
--     lives for the shopping trip (expires at end of its created day).
--   * handoff token — minted at checkout: a low-entropy typed code (safe only
--     because it is single-use, short-lived, state-gated, rate-limited and
--     branch-scoped) plus a higher-entropy QR reference.

-- +goose Up

CREATE TABLE handoff_carts (
    id                       text PRIMARY KEY,
    branch_id                text NOT NULL REFERENCES branches (id) ON DELETE CASCADE,
    state                    text NOT NULL DEFAULT 'active'
        CHECK (state IN ('active', 'ready_for_handoff', 'claimed', 'completed', 'expired', 'cancelled')),

    -- session token (entry-QR bearer), stored as a sha-256 hex digest
    session_token_hash       text NOT NULL,
    session_expires_at       timestamptz NOT NULL,

    -- handoff token (minted at checkout, in ready_for_handoff only)
    handoff_code_hash        text,         -- sha-256 of the typed N-digit code
    handoff_ref_hash         text,         -- sha-256 of the higher-entropy QR reference
    handoff_expires_at       timestamptz,  -- short TTL (minutes)
    handoff_attempts         integer NOT NULL DEFAULT 0,  -- failed claim attempts since mint
    handoff_locked_until     timestamptz,  -- brute-force lockout window

    -- claim (edit rights transfer to this seller; customer writes rejected)
    claimed_by               text,
    claimed_at               timestamptz,

    -- checkout (idempotent via the stored key; sale recorded on a shift journal)
    checkout_idempotency_key text,
    sale_journal_id          text,

    finalized_at             timestamptz,  -- set when reaching a terminal state
    created_at               timestamptz NOT NULL DEFAULT now(),
    updated_at               timestamptz NOT NULL DEFAULT now()
);

-- The session token is the credential for the customer endpoints: unique + the
-- primary lookup key.
CREATE UNIQUE INDEX handoff_carts_session_token_idx ON handoff_carts (session_token_hash);

-- The typed handoff code must be unambiguous among carts currently awaiting
-- handoff, so claim-by-code resolves to exactly one cart.
CREATE UNIQUE INDEX handoff_carts_handoff_code_idx
    ON handoff_carts (handoff_code_hash)
    WHERE handoff_code_hash IS NOT NULL AND state = 'ready_for_handoff';

CREATE INDEX handoff_carts_branch_state_idx ON handoff_carts (branch_id, state);
-- Supports the janitor sweeping stale non-terminal carts / expired tokens.
CREATE INDEX handoff_carts_state_expiry_idx ON handoff_carts (state, session_expires_at);

CREATE TABLE handoff_cart_lines (
    id            text PRIMARY KEY,
    cart_id       text NOT NULL REFERENCES handoff_carts (id) ON DELETE CASCADE,
    product_id    text NOT NULL,
    sku           text,
    name          text,
    uom           text,
    quantity      integer NOT NULL DEFAULT 0,
    display_price integer NOT NULL DEFAULT 0,  -- customer-facing snapshot; NOT authoritative
    line_total    integer NOT NULL DEFAULT 0
);
CREATE INDEX handoff_cart_lines_cart_idx ON handoff_cart_lines (cart_id);

-- +goose Down
DROP TABLE IF EXISTS handoff_cart_lines;
DROP TABLE IF EXISTS handoff_carts;
