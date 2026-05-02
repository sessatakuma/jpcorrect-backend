# jpcorrect-backend
This repository contains the backend for the jpcorrect system, a Japanese language correction platform.

## Getting Started

### Prerequisites
- Go 1.25+
- PostgreSQL

### Installation
```bash
git clone https://github.com/sessatakuma/jpcorrect-backend.git
cd jpcorrect-backend
go mod download
```

### Environment Variables
A `.env` file is required in the project root. Copy and configure it:
```bash
cp .env.example .env
```

Variables:
| Variable | Description |
| --- | --- |
| `PORT` | Server port (default `8080`) |
| `DATABASE_URL` | PostgreSQL connection string |
| `API_TOOLS_URL` | External API tools service URL |
| `JWKS_URL` | JWKS endpoint for JWT verification |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins (empty = allow all in debug mode) |
| `GIN_MODE` | `debug` or `release` |
| `API_CERT_PATH` | TLS certificate path (optional; enables HTTPS if both cert and key exist) |
| `API_KEY_PATH` | TLS key path (optional) |

> **Docker note:** `DATABASE_URL` must use the Docker hostname `postgres` instead of `localhost` when running via `docker compose`. See `.env` for the default value.

### Run
```bash
go run cmd/jpcorrect/main.go
```

### Development
Run with [air](https://github.com/air-verse/air) for live reloading:
```bash
make air
```

### Swagger / API Documentation

Generate Swagger docs with:
```bash
make swag
```

This runs `swag init` and outputs to `docs/swagger/`. The Swagger UI is served at `/swagger/index.html`.

When adding or modifying API handlers, update the Swagger annotations on each handler function. Annotations are written as Go comments above the handler. Reference: [swaggo/swag declarative comments format](https://github.com/swaggo/swag#declarative-comments-format).

**General API info** is declared in `cmd/jpcorrect/main.go`:
```go
// @title jpcorrect API
// @version 1.0
// @description Japanese language correction platform backend API
// @host localhost:8080
// @BasePath /
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
```

**Per-handler annotations** example:
```go
// @Summary Create a user
// @Description Create a new user
// @Tags users
// @Accept json
// @Produce json
// @Param user body domain.User true "User data"
// @Success 201 {object} domain.User
// @Failure 400 {object} map[string]string
// @Failure 409 {object} map[string]string
// @Failure 500 {object} map[string]string
// @Router /v1/users [post]
func (a *API) UserCreateHandler(c *gin.Context) { ... }
```

Common annotations:

| Annotation | Description |
| --- | --- |
| `@Summary` | Short summary of the operation |
| `@Description` | Detailed description |
| `@Tags` | Group operations under a tag |
| `@Accept` | Request content type (`json`, `xml`, etc.) |
| `@Produce` | Response content type |
| `@Param` | Parameter: `{name} {in} {type} {required} "{desc}"` |
| `@Success` / `@Failure` | Response: `{status} {type} {model} "{desc}"` |
| `@Router` | Route: `{path} [{method}]` |
| `@Security` | Security definition to apply (e.g., `BearerAuth`) |

## Docker Support

### Quick Start
```bash
# Start all services (postgres + backend)
docker compose up -d

# Build and start (use after code changes)
docker compose up --build

# Stop all services
docker compose down
```

> **Note:** The Docker setup uses a multi-stage build. You must run `docker compose up --build` to pick up code changes — there is no hot reload inside the container. For live reloading during development, use `make air` locally instead.

### Access Services
- **Backend API**: http://localhost:8080
- **Swagger UI**: http://localhost:8080/swagger/index.html
- **PostgreSQL**: localhost:5432
  - User: `jpcorrect`
  - Password: `jpcorrect_password`
  - Database: `jpcorrect`

### Docker Environment Variables
The backend service reads `.env` via `env_file` in `docker-compose.yml`, so all variables are automatically injected into the container. Make sure `DATABASE_URL` in `.env` uses the Docker hostname `postgres` (not `localhost`).
