# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Overview

A Point-of-Sale (POS) / ERP backend for a multi-branch retail business, written in Go with the Fiber v2 web framework and MongoDB. A separate Python (FastAPI) service in `invoce-generator/` parses supplier proposal/invoice documents and is reached through the Go server's forward proxy.

## Commands

```bash
# Run the server (loads config.local.yaml by default — see Configuration)
go run cmd/main.go            # cmd/main.go is the real entrypoint; root main.go is a stale stub

# Build
go build -o bin/pos-erp cmd/main.go
goreleaser build --snapshot --clean   # cross-platform snapshot

# Tests — see "Testing" below; they need a running MongoDB replica set
go test ./test                                   # all tests
DISABLE_LOGGING=true go test ./test              # quiet
go test ./test -run TestCreateBranches           # a single test

# Regenerate Swagger docs after changing handler annotations (run from repo root)
swag init -g cmd/main.go      # NOTE: the older `--dir pkg,routes,app` flags are stale — `routes` is pkg/routes
```

There is no Makefile or lint config; use standard `go vet ./...` / `gofmt`.

## Architecture

Request flow: `cmd/main.go` → `pkg/app.New()` (builds `App` with logger, Mongo client, Fiber router, config) → `App.Run()` → `NewControllers(db)` → `SetupRoutes(router, controllers)` → `router.Listen(server.port)`.

- **`pkg/app/`** — wiring only. `app.go` builds the Fiber app, OTel tracer, CORS, BasicAuth-protected `/docs` Swagger. `controllers.go` holds the `Controllers` struct, `NewControllers` (constructs every controller from one `*mongo.Database`), and `SetupRoutes`.
- **`pkg/controllers/<domain>/`** — one package per staff domain (auth, customers/bnpl, finance, journals, products, sales, suppliers, transactions, arrivals, analytics, proxy). Each has a `New(db *mongo.Database)` constructor that grabs the Mongo collections it needs (and creates indexes inline), plus Fiber handlers with `swag` doc-comments. **Storefront** controllers live under `pkg/controllers/store/<domain>/` (auth, account, catalog, products, cart, wishlist, orders, reviews, promotions), aggregated by `pkg/controllers/store/store.go` into a `store.Controllers`.
- **`pkg/routes/routes.go`** + **`pkg/routes/store.go`** — pure route tables: one `XxxRoutes(router, controller, middleware)` function per domain, called from `SetupRoutes`. Routes are where middleware (auth, `ShiftIsOpenMiddleware`) is attached.
- **`pkg/repository/`** — all Mongo models / DTOs (package `models`), shared across controllers. No ORM; collections are accessed directly. The standard response envelope is the **generic** `Output[T]` (`output.go`): `NewOutput(data)` infers `T` and yields `{ "data": <T>, "error": [] }`. Error/no-data responses use `NewErrorOutput(...)` and the concrete `ErrorOutput` type; `responses.go` also defines `MessageResponse` and `TokenResponse`. **All handlers render via these helpers.** Swaggo docs them as `models.Output[Type]` (`@Success`) and `models.ErrorOutput` (`@Failure`) — no bare `Output[any]`.
- **`pkg/middleware/`** — `auth.go`: `AuthMiddleware` (staff — loads `users` by PASETO subject into `c.Locals("user")`) and `CustomerAuthMiddleware` (storefront — loads `store_customers` into `c.Locals("customer")`). `activitiy.go` audit-logs `Activity` records with typed `ActivityType` constants after successful mutations.
- **`platform/`** — infrastructure: `database/` (Mongo connect + transaction helpers), `logger/` (zerolog setup + Fiber middleware), `s3/` (product/proposal image storage), `cache/` (Redis, mostly unused).

### API surface & auth

Two route trees, both under `/api` (so both share the staff-guard prefix unless registered before it):

- **Admin/staff API — `/api/v1/admin/*`** (auth/activities, suppliers, products, journals, finance, transactions, customers, bnpl, proposals). Protected by **PASETO** validating against the `users` collection (`AuthMiddleware`).
- **Storefront API — `/api/v1/store/*`** (auth, account, catalog, products, cart, wishlist, orders, reviews, promotions). A **public** browse surface (catalog/products/promotions read + customer register/login) plus a **protected** customer surface (cart/wishlist/orders/profile/write-reviews) guarded by **`CustomerAuthMiddleware`** validating against the `store_customers` collection.
- **Sales** remains at `/api/sales/transactions/*` (not yet moved under `/api/v1/admin`).

**Two PASETO guards on overlapping `/api` prefixes, resolved by Fiber registration order** (`SetupRoutes` in `pkg/app/controllers.go`): a route registered *before* a `Group(prefix, mw)` call does **not** inherit `mw`. The registration order is therefore deliberate:
1. `routes.PublicAuthRoutes` → `POST /api/v1/admin/auth/login` (public; **register is intentionally disabled**).
2. `routes.StorePublicRoutes` → public store routes (no token).
3. `app.Group("/api/v1/store", customerPaseto)` → `routes.StoreProtectedRoutes` (customer-guarded).
4. `app.Group("/api", staffPaseto)` → all admin route registrations (store + the two logins are already registered, so the staff guard never sees them).

Both guards share the decoded symmetric key. **The key must be Base64-encoded 32 bytes** (`openssl rand -base64 32`), decoded at startup; a bad/short key is fatal. ⚠️ The committed `config.local.yaml` key decodes to only **24 bytes** — to boot locally, override `SERVER_SECRET_SYMMETRIC_KEY` with a valid key.

**BasicAuth** protects `/docs/*` and `/dashboard/*`, using the two `server.admin_docs_users`.

### Storefront (Phase 1 — stubs)

The `/api/v1/store` controllers are currently compiling **stubs** that return `models.NotImplemented` (HTTP 501): routes, the customer-auth guard, and the `store_customers` collection are wired, but business logic and most data models are not yet built. `docs/storefront-implementation-guide.md` defines the Phase-2 pattern — models in `pkg/models`, repositories under `pkg/repository/store/<domain>/` (handlers call repository methods, never the DB directly), held as a field on each controller.

### Journals, shifts, and finance (core domain)

The accounting heart of the system. A **journal** (`pkg/repository/journal.go`) is a per-branch daily shift containing **operations** (transactions). Key invariant: operations can only be added/edited/deleted while the shift is open — enforced by `operations.ShiftIsOpenMiddleware` (checks `Shift_is_closed`), wired in `JournalsRoutes`. Mutations that touch multiple collections (journal close/reopen, operation create/update/delete) run inside MongoDB multi-document transactions via `database.StartTransaction(client)` → `ses.CommitTransaction(ctx)` (see `pkg/controllers/journals/operations.go`). **MongoDB must be a replica set** for these transactions to work. Journal/operation activity also rolls up into branch `finance` balances and the `transactions` collection.

### Proxy to the invoice service

`config.server.proxy[0]` defines a forward proxy (default path `/proposals` → `http://localhost:11000`). `ProxyRoutes` PASETO-protects the proxy path and forwards requests (adding the configured `x-api-key`) to the Python `invoce-generator` FastAPI service, which parses uploaded proposal documents and generates PDFs.

## Configuration

Config loading (`pkg/configs/configs.go`, via Viper):
- `ENVIRONMENT=production` → reads `config.yaml`; otherwise `config.local.yaml` (or `$CONFIG_FILE`, e.g. `CONFIG_FILE=config.staging`).
- If `LOAD_DOT_ENV` is non-empty, `.env` is loaded.
- Explicit env bindings override YAML — see the `bindings` map in `configs.go` (`DATABASE_*`, `REDIS_*`, `SERVER_*`, `S3_*`). Viper's automatic-env replaces `.` with `_`.
- `LoadConfig(".")` is called repeatedly from many places (app, routes, middleware) rather than passed around — it re-reads each time, so config files must be resolvable from the working directory (it also searches `../` and `../../`, which is how tests find it).

Copy `example.config.yaml` → `config.local.yaml` to start.

## Testing

Tests live in `test/` (package `test`) and are **integration tests against a live server + real MongoDB**, not unit tests:
- `TestMain` (in `test/finance-branches_test.go`) calls `app.New()`, wipes the `magazin` collections (finance, suppliers, transactions, journals), then `go app.Run()` to start the server in-process, then `m.Run()`.
- Tests drive the server over HTTP through the typed client in `test/client/` (`client.NewClient(host, port, user, pass)`), hitting `localhost` + `server.port` from `config.local.yaml`.
- Therefore a MongoDB replica set matching `config.local.yaml` must be reachable before running `go test ./test`. `test/mock/` holds seed JSON (finances, suppliers).
- Tests share global state (the wiped DB + running server) and are order-sensitive; `TestCreateBranches` seeds branches that later tests rely on.

## Deployment

`deploy/docker-compose.yml` builds `build/Dockerfile` and runs the backend behind Caddy (labels for `api.g4h.uz`), mounting `config.yaml` and `tmp/`, with `ENVIRONMENT=PRODUCTION`. External Docker networks `mongoCluster` and `caddy` must exist first. Releases are tag-driven (`v*`) via GitHub Actions + GoReleaser.

## Gotchas

- Root `main.go` is a leftover Petstore stub — the real entrypoint is `cmd/main.go`.
- Many handlers return HTTP 500 (`StatusInternalServerError`) for business/validation errors (e.g. duplicate branch), not 4xx; tests assert on this.
- `AdminDocsUsers` is indexed `[0]` and `[1]` directly — config must define at least two `admin_docs_users` or startup/route setup panics.
- Sales *session* routes (`/api/sales/session/*`) are commented out; only `/api/sales/transactions/*` is active.
- `config.local.yaml`'s `secret_symmetric_key` decodes to 24 bytes but PASETO needs 32 — local boot panics unless you override `SERVER_SECRET_SYMMETRIC_KEY` (and missing PASETO token → HTTP **400** `missing PASETO token`, not 401).
- `Output` is generic (`Output[T]`); never write `NewOutput(nil, ...)` (untyped nil breaks inference) — use `NewErrorOutput(...)`. Swaggo `@Success`/`@Failure` must use a concrete type (`models.Output[Type]` / `models.ErrorOutput`), never `models.Output[any]`.
- A few files don't import the `models` package (e.g. `product-images.go`), so their swaggo can't reference `models.*` types — they use native types (`{file} binary`, `map[string]string`).
- Pre-existing gofmt drift in `pkg/controllers/proxy/proxy.go` and `pkg/repository/product-models.go` (unformatted at HEAD) — leave unless intentionally tidying.



####### NEVER 
NEVER RUN GO TESTS --- `go test` --- they require human inputs and should be run by humans always