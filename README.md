# Magazin POS/ERP (Go)

[![Release](https://img.shields.io/github/v/release/aslon1213/g4h_pos_erp?style=flat-square)](https://github.com/aslon1213/g4h_pos_erp/releases)
[![Build Status](https://img.shields.io/github/actions/workflow/status/aslon1213/g4h_pos_erp/release.yaml?style=flat-square)](https://github.com/aslon1213/g4h_pos_erp/actions/workflows/release.yaml)
[![Go Version](https://img.shields.io/github/go-mod/go-version/aslon1213/g4h_pos_erp?style=flat-square)](https://golang.org/)
[![License](https://img.shields.io/github/license/aslon1213/g4h_pos_erp?style=flat-square)](LICENSE)

A Point of Sale (POS) and ERP backend built with Go for retail operations.

## Latest Changes (May 2026)

- All staff endpoints moved under a versioned **`/api/v1/admin/*`** namespace (auth, suppliers, products, journals, finance, transactions, customers, bnpl, proposals). Admin `login` stays public; `register` is disabled.
- Added a public **storefront API** under **`/api/v1/store/*`** (customer auth, catalog, products, cart, wishlist, orders, reviews, promotions) with a dedicated `CustomerAuthMiddleware` (validates the `store_customers` collection). Phase-1 stubs — see `docs/storefront-implementation-guide.md`.
- Response envelope is now the **generic `Output[T]`**; error responses use `ErrorOutput`. Swaggo annotations are typed per endpoint.

Earlier:

- Auth token handling decodes `server.secret_symmetric_key` from Base64 before creating/verifying PASETO tokens.
- Config loading supports explicit environment variable bindings (via Viper) with optional `.env` loading using `LOAD_DOT_ENV`.

## 🚀 Features

- **Sales Management** - Complete sales transaction processing
- **Product Management** - Inventory tracking and product catalog
- **Customer Management** - Customer data and relationship management
- **Supplier Management** - Vendor and supplier relationship tracking
- **Financial Management** - Transaction processing and financial reporting
- **Journal Entries** - Accounting and bookkeeping functionality
- **Internal Expenses** - Business expense tracking and management
- **REST API** - Full RESTful API with Swagger documentation
- **Real-time Data** - Redis caching for optimal performance
- **Observability** - OpenTelemetry integration for monitoring

## 🛠️ Tech Stack

- **Backend**: Go 1.24.3
- **Web Framework**: [Fiber v2](https://github.com/gofiber/fiber)
- **Database**: MongoDB
- **Cache**: Redis
- **Documentation**: Swagger/OpenAPI
- **Authentication**: PASETO (Bearer token) + BasicAuth (docs/dashboard)
- **Observability**: OpenTelemetry
- **Logging**: Zerolog
- **Configuration**: Viper (YAML + environment overrides)

## 📋 Prerequisites

- Go 1.24.3 or higher
- MongoDB 4.4+ cluster with replica set
- Redis 6.0+
- Docker & Docker Compose (for containerized setup)

## 🚀 Quick Start

### Using Docker Compose (Recommended)

1. **Clone the repository**

   ```bash
   git clone https://github.com/aslon1213/g4h_pos_erp.git
   cd g4h_pos_erp
   ```

2. **Prepare configuration**

   ```bash
   cp example.config.yaml config.local.yaml
   # Edit config.local.yaml to match your MongoDB/Redis/S3/settings
   ```

3. **Create required docker networks (first run only)**

   ```bash
   docker network create mongoCluster || true
   docker network create caddy || true
   ```

4. **Start optional infrastructure (reverse proxy, redis UI)**

   ```bash
   # Caddy (for domain/HTTPS via labels). Optional for local use
   docker compose -f deploy/docker-compose-caddy.yaml up -d

   # Redis (redis-stack with UI). Optional
   docker compose -f deploy/docker-compose-db.yml up -d
   ```

5. **Start the backend**

   ```bash
   docker compose -f deploy/docker-compose.yml up -d --build
   ```

### Manual Setup

1. **Install dependencies**

   ```bash
   go mod download
   ```

2. **Setup MongoDB**

   - Use an existing MongoDB replica set, or update your active config file (`config.local.yaml` by default, `config.yaml` in production) to point to your MongoDB URL. Ensure `replica_set` aligns with your cluster if replication is enabled.

3. **Configure the application**

   ```bash
   cp example.config.yaml config.local.yaml
   # Edit config.local.yaml with your database and Redis settings
   ```

4. **Run the application**
   ```bash
   go run cmd/main.go
   ```

## ⚙️ Configuration

The app loads config in this order:

- If `ENVIRONMENT=production`: uses `config.yaml`
- Otherwise: uses `config.local.yaml` (or `CONFIG_FILE` if set, e.g. `CONFIG_FILE=config.staging`)
- If `LOAD_DOT_ENV` is set (non-empty), `.env` is loaded
- Bound environment variables override matching YAML keys

Start from:

```bash
cp example.config.yaml config.local.yaml
```

YAML shape:

```yaml
database:
  host: "localhost"
  port: "27017"
  username: "admin"
  password: "admin"
  database: "store"
  max_connections: 20
  min_pool_size: 10
  auth: false
  replica_set: "rs0"
  url: "mongodb://localhost:27017/?replicaSet=rs0"

redis:
  host: "localhost"
  port: "6379"
  password: ""
  database: 0

server:
  port: ":12000"
```

### Environment Variables (Supported Bindings)

The config loader binds these env vars:

- Database: `DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_USERNAME`, `DATABASE_PASSWORD`, `DATABASE_NAME`, `DATABASE_MAX_CONNECTIONS`, `DATABASE_MIN_POOL_SIZE`, `DATABASE_AUTH`, `DATABASE_REPLICA_SET`, `DATABASE_URL`
- Redis: `REDIS_HOST`, `REDIS_PORT`, `REDIS_PASSWORD`, `REDIS_DATABASE`
- Server: `SERVER_HOST`, `SERVER_PORT`, `SERVER_SECRET_SYMMETRIC_KEY`, `SERVER_TOKEN_EXPIRY_HOURS`
- S3: `S3_REGION`, `S3_ENDPOINT`, `S3_ACCESS_KEY_ID`, `S3_SECRET_ACCESS_KEY`, `S3_IMAGE_BUCKET`

### Auth Key Requirement

`server.secret_symmetric_key` is Base64-decoded at runtime for PASETO middleware and token creation. Use a Base64-encoded 32-byte key, for example:

```bash
openssl rand -base64 32
```

## 📖 API Documentation

Once the application is running, access Swagger at:

```
http://localhost:12000/docs/index.html
```

Note: Access to `/docs` is protected with BasicAuth. Credentials are configured in `server.admin_docs_users` in your active config file.

The API is split into an **admin/staff** tree and a public **storefront** tree.

**Admin / staff — `/api/v1/admin/*`** (PASETO bearer token against the `users` collection):

- `/api/v1/admin/auth/login` (public), `/api/v1/admin/auth/me`, `/api/v1/admin/activities/*`
- `/api/v1/admin/products/*` - product catalog, stock ops, images
- `/api/v1/admin/customers/*` - customers
- `/api/v1/admin/suppliers/*` - suppliers and supplier transactions
- `/api/v1/admin/transactions/*` - transaction queries/updates/docs enums
- `/api/v1/admin/finance/*` - branch finance
- `/api/v1/admin/journals/*` - journal lifecycle and operations
- `/api/v1/admin/bnpl/*`, `/api/v1/admin/branches/:branch_id/bnpls`, `/api/v1/admin/customers/:customer_id/bnpls`
- `/api/v1/admin/proposals/*` and `/proposals` (proxy route)

**Storefront — `/api/v1/store/*`** (public browse + customer PASETO token against `store_customers`):

- `/api/v1/store/auth/*` (register/login public; me/logout protected), `/api/v1/store/account/addresses/*`
- `/api/v1/store/catalog/*`, `/api/v1/store/products/*` - public catalog & product browse + reviews
- `/api/v1/store/cart/*`, `/api/v1/store/wishlist/*`, `/api/v1/store/orders/*`, `/api/v1/store/checkout/*` - customer-only
- `/api/v1/store/reviews/*`, `/api/v1/store/promotions/*`

**Still under `/api`:** `/api/sales/transactions/*`.

Notes:

- Storefront endpoints are **Phase-1 stubs** (return HTTP 501 `not implemented`); routing and customer auth are wired, business logic is pending. See `docs/storefront-implementation-guide.md`.
- The admin `register` endpoint and `/api/sales/session/*` are disabled/commented out.
- Responses use a generic envelope `Output[T]` → `{ "data": <T>, "error": [] }`; errors use `ErrorOutput`.

## 🔧 Development

### Project Structure

```
.
├── cmd/                            # Application entry points - main.go
├── pkg/                            # Application packages
│   ├── app/                        # Main application logic
│   ├── controllers/                # Business logic controllers
│   │   ├── customers/              # Customer management (+ bnpl/)
│   │   ├── finance/                # Financial operations
│   │   ├── internalExpenses/       # Internal expenses
│   │   ├── journals/               # Journal entries for daily financial operations
│   │   ├── products/               # Product management
│   │   ├── sales/                  # Sales transactions
│   │   ├── suppliers/              # Supplier management
│   │   ├── transactions/           # Financial transactions
│   │   └── store/                  # Storefront API (auth, cart, catalog, orders, ...)
│   ├── routes/                     # API route definitions (routes.go, store.go)
│   ├── middleware/                 # HTTP middleware (staff + customer auth)
│   ├── repository/                 # Data access layer, models
│   ├── utils/                      # Utility functions
│   └── configs/                    # Configuration management
├── docs/                           # API documentation
├── deploy/                         # Deployment configurations - docker-compose*.yml
├── test/                           # Test files
├── web/                            # Frontend assets (if any)
└── scripts/                        # Build and deployment scripts
```

### Building the Application

```bash
# Build for current platform
go build -o bin/pos-erp cmd/main.go

# Build for multiple platforms using GoReleaser
goreleaser build --snapshot --clean
```

### Running Tests

```bash
go test ./test
```

### Generate Swagger Documentation

```bash
# Run from the repo root (the older --dir pkg,routes,app flags are stale)
swag init -g cmd/main.go
```

## 🐳 Docker Deployment

The project includes Docker Compose configuration for easy deployment:

```bash
# Start backend
docker compose -f deploy/docker-compose.yml up

# View logs
docker compose -f deploy/docker-compose.yml logs -f

# Stop services
docker compose -f deploy/docker-compose.yml down
```

## 📦 Releases & CI/CD

### Automated Releases

This project uses GitHub Actions with [GoReleaser](https://goreleaser.com/) for automated releases:

- **Trigger**: Releases are automatically triggered when you push a git tag starting with `v` (e.g., `v1.0.0`, `v2.1.3`)
- **Build Matrix**: Cross-platform builds for Linux, macOS, and Windows (amd64, arm64)
- **Artifacts**: Pre-built binaries, archives, and checksums
- **Distribution**: Automatically published to GitHub Releases

### Release Workflow

1. **Create and push a tag**:

   ```bash
   git tag v1.0.0
   git push origin v1.0.0
   ```

2. **GitHub Actions automatically**:
   - Builds binaries for all supported platforms
   - Runs tests and quality checks
   - Creates GitHub release with artifacts
   - Generates changelog

### Build Platforms

| OS      | Architecture | Status       |
| ------- | ------------ | ------------ |
| Linux   | amd64, arm64 | ✅ Supported |
| macOS   | amd64, arm64 | ✅ Supported |
| Windows | amd64        | ✅ Supported |

### Local Development Build

```bash
# Build snapshot for testing (without releasing)
goreleaser build --snapshot --clean

# Build for current platform only
go build -o bin/pos-erp cmd/main.go
```

### Download Latest Release

Visit the [Releases page](https://github.com/aslon1213/g4h_pos_erp/releases) to download pre-built binaries for your platform.

## 🤝 Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feature/<some-feature>`)
3. Commit your changes (`git commit -m 'Add some amazing -- <some-feature>'`)
4. Push to the branch (`git push origin feature/<some-feature>`)
5. Open a Pull Request

## 🆘 Support

For support and questions:

- Create an issue in the GitHub repository
- Contact: hamidovaslon13@gmail.com

## 🚧 Development Status

This project is actively maintained and under development. Please check the [issues](https://github.com/aslon1213/g4h_pos_erp/issues) for current development status and upcoming features.
