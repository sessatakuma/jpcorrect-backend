# jpcorrect-backend Agent Guide

Essential information for AI coding agents working in this repository.

## Project Overview

Japanese language correction platform backend: Go 1.25+, Gin, PostgreSQL, GORM.

**Architecture**: Clean architecture
- `cmd/jpcorrect/`: Entry point
- `internal/api/`: HTTP handlers (Gin)
- `internal/domain/`: Domain models and repository interfaces
- `internal/repository/`: GORM implementations
- `internal/cmd/`: Command execution and server setup
- `internal/database/`: Database connection and GORM config

## Build, Run, Test

### Development
```bash
make air                              # Live reload (uses go tool air)
make swag                             # Regenerate Swagger docs
go run cmd/jpcorrect/main.go          # Run directly
```

### Docker
```bash
docker compose up --build             # Build and start (must --build for code changes)
docker compose up -d                  # Start detached
docker compose down                   # Stop all services
```

Docker Compose reads `.env` via `env_file`. `DATABASE_URL` must use Docker hostname `postgres` (not `localhost`) in `.env`.

### Database
GORM `AutoMigrate` in `internal/cmd/api.go`. Modify domain models in `internal/domain/` and add them to the AutoMigrate call.

Models: `User`, `Guild`, `GuildAttendee`, `Event`, `EventAttendee`, `Transcript`, `Mistake`.

### Testing
```bash
go test ./...                                                      # All tests
go test ./internal/repository/...                                   # Specific package
go test -v ./internal/repository -run TestUserCreate                # Single test
go test -coverprofile=coverage.out ./...; go tool cover -html=coverage.out  # Coverage
```

**Testing Patterns**: `sqlmock` for DB mocking, `testify/assert`, `setupMockDB(t)` helper, `t.Run()` sub-cases, always call `mock.ExpectationsWereMet()` at end of success cases.

## Code Style

### Imports
Three groups: stdlib → third-party → local. Blank import `_ "jpcorrect-backend/docs/swagger"` is **required** in `internal/api/api.go` — without it Swagger UI returns 500 on `/swagger/doc.json`.

### UUIDs
Always `uuid.UUID` type, never strings or ints. Generate in `Create` methods if missing: `uuid.New()`.

### Context
Pass `context.Context` as first argument. Use `.WithContext(ctx)` on all GORM calls.

### Error Handling
Sentinel errors in `internal/domain/errors.go`: `ErrNotFound`, `ErrDuplicateEntry`, `ErrHasRelatedRecords`. Map all GORM/PG errors via `MapGormError()` in repository layer. API handlers check domain errors and return appropriate HTTP status codes.

Auth errors use `domain.AuthError` struct (not sentinel) with `StatusCode`, `Message`, `Details`.

### Soft Delete
Only `User`, `Guild`, `Event` have `DeletedAt gorm.DeletedAt` (soft delete). `Transcript`, `Mistake`, `GuildAttendee`, `EventAttendee` do **not** — use hard delete or status-based lifecycle.

### Route Naming
`/v1/practices` routes use the `Event` domain model (backward-compatibility naming).

### Transcript.Accent
`datatypes.JSON` with `gorm:"type:jsonb"` — the only JSONB column.

### GuildAttendeeRepository
Implemented in `gorm_guild.go`, not a separate file.

## Swagger / API Documentation

After adding or modifying API handlers with `@Summary`, `@Router`, etc. annotations:
```bash
make swag   # runs: swag init -g cmd/jpcorrect/main.go -o docs/swagger --parseDependency --parseInternal
```

The `_ "jpcorrect-backend/docs/swagger"` import in `api.go` registers generated specs. CI runs `yamllint` on all YAML — `docs/swagger/` is excluded via `.yamllint`.

## Project Conventions

### Environment Variables
`.env` is auto-loaded by `github.com/joho/godotenv/autoload` (blank import in main.go).

| Variable | Required | Default | Notes |
| --- | --- | --- | --- |
| `DATABASE_URL` | Yes | — | Postgres connection string. Use `@postgres:5432` in Docker |
| `JWKS_URL` | Yes | — | App fatals if empty |
| `PORT` | No | `8080` | |
| `API_TOOLS_URL` | No | — | External API tools service |
| `ALLOWED_ORIGINS` | No | — | Comma-separated. Empty = reject all in release, allow all in debug |
| `GIN_MODE` | No | — | `debug` or `release` |
| `API_CERT_PATH` | No | `./certs/cert.pem` | Enables HTTPS if both cert and key exist |
| `API_KEY_PATH` | No | `./certs/key.pem` | |

### TLS
Server checks if both `API_CERT_PATH` and `API_KEY_PATH` files exist. If yes → HTTPS; if no → HTTP with warning log.

### Rate Limiter
`NewRateLimiter(10*time.Second, 15)` — 10-second window, max 15 connections.

## CI

PR checks (`sessatakuma/org-workflows`):
- `go mod tidy` check
- `golangci-lint` with config in `.golangci.yml` (errcheck, govet, ineffassign, staticcheck, unused)
- Tests with race detector
- Build verification
- YAML linting via `yamllint` (default rules, `.yamllint` excludes `docs/swagger/`)
- JSON syntax check (jq)
- TOML syntax check (taplo)

## Git

### Commits
Use Conventional Commits template:
```
# ----------------------------------------------------------------------
# Conventional Commit Message Template
# Based on https://www.conventionalcommits.org/en/v1.0.0/
# ----------------------------------------------------------------------

# HEADER (required)
# Format:
#   <type>(<scope>): <short summary>
# or (with breaking change):
#   <type>(<scope>)!: <short summary>
#
# <type> must be one of:
#   feat     - A new feature
#   fix      - A bug fix
#   docs     - Documentation only changes
#   style    - Code style changes (formatting, semicolons, etc.)
#   refactor - Code change that neither fixes a bug nor adds a feature
#   perf     - Improves performance
#   test     - Adds or corrects tests
#   build    - Build system or dependencies
#   ci       - CI configuration/scripts
#   chore    - Routine maintenance
#   revert   - Reverts a previous commit
#   hotfix   - Quick fix on bugs
#
# <scope> is optional (e.g., ui, api, core, auth, deps).
# <short summary> ≤ 72 chars (dont longer than the dash line, imperative
# (e.g., "add", "fix", "update") in lowercase.
#
# Examples:
#   feat(api): add JWT authentication middleware
#   fix(ui)!: remove deprecated button variants
#
# ----------------------------------------------------------------------

<type>(<scope>): <short summary>

# If this commit includes a BREAKING CHANGE, add "!" after <scope>) in  
# the header
# AND include a BREAKING CHANGE block in the footer below.

# ----------------------------------------------------------------------
# BODY (optional)
# Describe WHAT and WHY (not how). Include context, trade-offs, and 
# alternatives.
# Wrap lines at ~72 chars.
#
# Example:
#   Introduce a shared middleware for JWT verification across protected
#   routes.
#   Reduces duplicated logic and standardizes error responses.
# ----------------------------------------------------------------------

<body>

# ----------------------------------------------------------------------
# FOOTER (optional)
# Use for metadata:
# - BREAKING CHANGES: Start a block with "BREAKING CHANGE:" and explain 
#   impact. Include migration steps and rationale.
# - Issue refs: Closes #123, Fixes #456, Refs #789
#
# Examples:
#   BREAKING CHANGE: remove deprecated /v1 endpoints in favor of /v2.
#   Migration: update client base URL to /v2 and switch to OAuth2
#   tokens.
#   Closes #351, #422
# ----------------------------------------------------------------------

<footer>

```

### Pull Requests
- **Title**: Follow Conventional Commits template from the previous section, ≤ 75 characters.
- **Body**: Use the template in `.github/PULL_REQUEST_TEMPLATE.md`

## Common Gotchas

1. **Swagger blank import**: `_ "jpcorrect-backend/docs/swagger"` must exist in `api.go` or `/swagger/doc.json` returns 500
2. **AutoMigrate**: Primary schema tool. Add new models to the call in `internal/cmd/api.go`
3. **JWKS_URL**: App exits immediately if not set
4. **UUIDs**: Always `uuid.UUID`, never strings/ints
5. **Context**: Pass down everywhere, `.WithContext(ctx)` on all GORM calls
6. **GORM Errors**: Always map via `MapGormError()`, never return raw GORM errors from repository
7. **Soft Delete**: Only User/Guild/Event. Use `Unscoped()` for hard delete on those
8. **Docker rebuild**: Must `docker compose up --build` for code changes — no volume mount for hot reload
9. **Docker DATABASE_URL**: Use hostname `postgres`, not `localhost`
10. **`make swag` flags**: Must include `--parseDependency --parseInternal` or handler annotations won't be found
