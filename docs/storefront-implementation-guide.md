# Storefront Implementation Guide — Models, Repositories & Controllers

This guide defines the layering for implementing the storefront (`/api/v1/store`)
Phase-2 logic, and is the pattern all new store domains must follow.

## Goals (from the requirements)

1. **Models** live in `pkg/models` (new package).
2. **Repositories** live in `pkg/repository/...` and implement *all* database operations.
3. Each controller holds its repository as a **struct field** and reaches it from handlers.
4. **Handlers never touch the database** — no `mongo`/`bson`, no `db.Collection(...)`.
   They parse the request, call a repository method, and render the response envelope.

## Layering

```
HTTP request
  └─ Controller handler  (pkg/controllers/store/<domain>)   ← parse req, map result→HTTP
       └─ Repository      (pkg/repository/store/<domain>)    ← all mongo/bson lives here
            └─ Model       (pkg/models)                       ← plain structs + DTOs
```

A handler depends on its repository and on the response helpers — **not** on mongo.

## Package layout & the transition

`pkg/repository` is **currently the `models` package** (the legacy structs `Customer`,
`Product`, `Output`, … sit in `pkg/repository/*.go` as `package models`). Those will move
to `pkg/models` *later*. Until then, to avoid two `package models` directories colliding,
new code is placed as follows:

| Concern | Location | Package |
|---|---|---|
| New store models | `pkg/models/<domain>.go` | `models` |
| Store repositories | `pkg/repository/store/<domain>/` | `<domain>` (e.g. `cart`) |
| Shared repo errors | `pkg/repository/repoerr/` | `repoerr` |
| Response envelope (legacy, stays for now) | `pkg/repository/output.go` | `models` |
| Store controllers | `pkg/controllers/store/<domain>/` | `store<domain>` |

**Import aliases** keep the two `models` packages unambiguous in a controller:

```go
import (
    "github.com/aslon1213/g4h_pos_erp/pkg/models"                       // new data models  → models.Cart
    resp "github.com/aslon1213/g4h_pos_erp/pkg/repository"              // legacy envelope  → resp.NewOutput
    cartrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/cart"
    "github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
)
```

> When the legacy structs migrate to `pkg/models`, the `Output`/`NewOutput`/`NotImplemented`
> helpers move with them and the `resp` alias collapses to plain `models`. Nothing else changes.

## Conventions

- **IDs are `string` (uuid)**, matching existing `Customer`/`Product`/`User`. This keeps
  `pkg/models` free of any mongo import (`bson:"_id"` is just a struct tag).
- **Repository owns mongo**: `bson`, `mongo`, `options`, indexes, aggregation pipelines.
- **Repository method signature**: `(ctx context.Context, …) (*models.X, error)` — always
  takes `ctx` (handler passes `c.Context()`), returns a model + error.
- **Repository translates mongo errors → sentinels** (`repoerr.ErrNotFound`, `repoerr.ErrConflict`)
  so controllers map them to HTTP status without importing mongo.
- **Indexes** are created in the repository `New(db)` constructor (mirrors `finance.New`).
- **Constructor**: controller `New(db *mongo.Database)` builds the repo — the *only* place a
  store controller references `mongo` (just the `*mongo.Database` parameter type).

## What changes vs. today

Today handlers call the DB directly (anti-pattern we are replacing):

```go
// pkg/controllers/finance/finance.go  (current — DB in the handler)
cursor, err := f.FinanceCollection.Find(context.Background(), bson.M{})
```

After this pattern, the handler calls a repository method and never sees mongo/bson.

---

## Worked example — Cart

### 1. Model — `pkg/models/cart.go`

```go
package models

import "time"

type CartItem struct {
    ID        string  `json:"id" bson:"id"`
    ProductID string  `json:"product_id" bson:"product_id"`
    Quantity  int     `json:"quantity" bson:"quantity"`
    UnitPrice float64 `json:"unit_price" bson:"unit_price"`
}

type Cart struct {
    ID         string     `json:"id" bson:"_id"`
    CustomerID string     `json:"customer_id" bson:"customer_id"`
    Items      []CartItem `json:"items" bson:"items"`
    CouponCode string     `json:"coupon_code" bson:"coupon_code"`
    CreatedAt  time.Time  `json:"created_at" bson:"created_at"`
    UpdatedAt  time.Time  `json:"updated_at" bson:"updated_at"`
}

// AddCartItemInput is the request DTO for POST /cart/items.
type AddCartItemInput struct {
    ProductID string `json:"product_id"`
    Quantity  int    `json:"quantity"`
}
```

### 2. Shared errors — `pkg/repository/repoerr/errors.go`

```go
package repoerr

import "errors"

var (
    ErrNotFound = errors.New("resource not found")
    ErrConflict = errors.New("resource already exists")
)
```

### 3. Repository — `pkg/repository/store/cart/cart_repository.go`

```go
package cart

import (
    "context"
    "errors"
    "time"

    "github.com/aslon1213/g4h_pos_erp/pkg/models"
    "github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr"
    "go.mongodb.org/mongo-driver/v2/bson"
    "go.mongodb.org/mongo-driver/v2/mongo"
    "go.mongodb.org/mongo-driver/v2/mongo/options"
)

type Repository struct {
    coll *mongo.Collection
}

func New(db *mongo.Database) *Repository {
    coll := db.Collection("carts")
    _, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
        Keys:    bson.D{{Key: "customer_id", Value: 1}},
        Options: options.Index().SetUnique(true),
    })
    return &Repository{coll: coll}
}

// GetByCustomer returns the customer's cart, or repoerr.ErrNotFound.
func (r *Repository) GetByCustomer(ctx context.Context, customerID string) (*models.Cart, error) {
    cart := &models.Cart{}
    err := r.coll.FindOne(ctx, bson.M{"customer_id": customerID}).Decode(cart)
    if errors.Is(err, mongo.ErrNoDocuments) {
        return nil, repoerr.ErrNotFound
    }
    if err != nil {
        return nil, err
    }
    return cart, nil
}

// AddItem upserts the customer's cart and appends an item.
func (r *Repository) AddItem(ctx context.Context, customerID string, item models.CartItem) (*models.Cart, error) {
    now := time.Now()
    update := bson.M{
        "$push":        bson.M{"items": item},
        "$set":         bson.M{"updated_at": now},
        "$setOnInsert": bson.M{"customer_id": customerID, "created_at": now},
    }
    opts := options.UpdateOne().SetUpsert(true)
    if _, err := r.coll.UpdateOne(ctx, bson.M{"customer_id": customerID}, update, opts); err != nil {
        return nil, err
    }
    return r.GetByCustomer(ctx, customerID)
}
```

### 4. Response helpers — add to `pkg/repository/output.go` (package `models`)

```go
import "github.com/aslon1213/g4h_pos_erp/pkg/repository/repoerr" // + "errors"

func RespondError(c *fiber.Ctx, code int, msg string) error {
    return c.Status(code).JSON(NewOutput(nil, NewError(msg, code)))
}

// RespondRepoError maps repository sentinel errors to HTTP responses so every
// handler can do `return resp.RespondRepoError(c, err)`.
func RespondRepoError(c *fiber.Ctx, err error) error {
    switch {
    case errors.Is(err, repoerr.ErrNotFound):
        return RespondError(c, fiber.StatusNotFound, err.Error())
    case errors.Is(err, repoerr.ErrConflict):
        return RespondError(c, fiber.StatusConflict, err.Error())
    default:
        return RespondError(c, fiber.StatusInternalServerError, err.Error())
    }
}
```

### 5. Controller — `pkg/controllers/store/cart/cart.go`

```go
package storecart

import (
    "errors"

    "github.com/aslon1213/g4h_pos_erp/pkg/models"
    resp "github.com/aslon1213/g4h_pos_erp/pkg/repository"
    cartrepo "github.com/aslon1213/g4h_pos_erp/pkg/repository/store/cart"
    "github.com/gofiber/fiber/v2"
    "github.com/google/uuid"
    "go.mongodb.org/mongo-driver/v2/mongo"
)

type Controller struct {
    Repo *cartrepo.Repository
}

func New(db *mongo.Database) *Controller {
    return &Controller{Repo: cartrepo.New(db)}
}

// GetCart godoc … (swaggo unchanged)
func (ctrl *Controller) GetCart(c *fiber.Ctx) error {
    customerID := c.Locals("customer").(string) // set by CustomerAuthMiddleware
    cart, err := ctrl.Repo.GetByCustomer(c.Context(), customerID)
    if err != nil {
        return resp.RespondRepoError(c, err)
    }
    return c.JSON(resp.NewOutput(cart))
}

func (ctrl *Controller) AddItem(c *fiber.Ctx) error {
    customerID := c.Locals("customer").(string)
    var in models.AddCartItemInput
    if err := c.BodyParser(&in); err != nil {
        return resp.RespondError(c, fiber.StatusBadRequest, "invalid request body")
    }
    item := models.CartItem{ID: uuid.New().String(), ProductID: in.ProductID, Quantity: in.Quantity}
    cart, err := ctrl.Repo.AddItem(c.Context(), customerID, item)
    if err != nil {
        return resp.RespondRepoError(c, err)
    }
    return c.Status(fiber.StatusCreated).JSON(resp.NewOutput(cart))
}
```

Notice: the handler imports `errors`/`uuid`/`models`/`resp`/`cartrepo` — **no `bson`, no
`db.Collection`, no query logic.** `mongo` appears only in the `New` constructor signature.

---

## Adding a new store domain — checklist

1. **Model** → `pkg/models/<domain>.go`: `<Domain>`, nested types, `<Action>Input` DTOs. String IDs.
2. **Repository** → `pkg/repository/store/<domain>/<domain>_repository.go`: `Repository` struct,
   `New(db)` (with indexes), one method per data operation, mongo→`repoerr` translation.
3. **Controller** → add `Repo *<domain>repo.Repository` field; `New(db)` builds it; replace each
   stubbed `models.NotImplemented(c)` body with parse → `ctrl.Repo.X(...)` → `resp.NewOutput`.
4. Routes & wiring already exist (Phase 1) — no changes to `routes/store.go` or `SetupRoutes`.
5. **Build & verify**: `go build ./...`, then boot and exercise the endpoint (a valid
   32-byte `SERVER_SECRET_SYMMETRIC_KEY` is required — see note below).

## Suggested domain order

`auth` + `account` first (everything customer-scoped needs `store_customers` + the
`StoreCustomer` model fleshed out: password hash via `bcrypt`, token issuance via
`pasetoware.CreateToken` as in admin `auth.Login`), then `catalog`/`products` (read-only over
`products`), then `cart` → `wishlist` → `orders` → `reviews` → `promotions`.

## Notes

- **Reuse, don't duplicate**: catalog/products repositories read the existing `products`
  collection; reviews repository can be shared between the public listing handler
  (`storeproducts.GetProductReviews`) and the write handlers (`storereviews.*`).
- **Local boot** currently needs a valid key: `config.local.yaml`'s `secret_symmetric_key`
  decodes to 24 bytes but PASETO needs 32 — override with
  `SERVER_SECRET_SYMMETRIC_KEY=$(openssl rand -base64 32)` or fix the config.
- This guide intentionally does **not** migrate the legacy `pkg/repository` structs yet; that is
  a separate, mechanical follow-up once the store domains are on the new pattern.
