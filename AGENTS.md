# jpcorrect-backend Agent Guide

Essential information for AI coding agents working in this repository.

## Project Overview

Japanese language correction platform backend: Go (1.25+), Gin, PostgreSQL, GORM.

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
make air                              # Live reload
go run cmd/jpcorrect/main.go         # Run directly
go mod download                       # Install deps
```

### Database
GORM `AutoMigrate` is used for schema management in `internal/cmd/api.go`. Modify domain models in `internal/domain/` and add them to the AutoMigrate call.

### Testing
```bash
go test ./...                         # All tests
go test ./internal/repository/...      # Specific package
go test -v ./internal/repository -run TestUserCreate  # Single test
go test -coverprofile=coverage.out ./...; go tool cover -html=coverage.out  # Coverage
```

**Testing Patterns**: `sqlmock` for DB mocking, `testify/assert`, `setupMockDB(t)`, `t.Run()` sub-cases, `mock.ExpectationsWereMet()`.

### Build & Lint
```bash
go build -o bin/jpcorrect cmd/jpcorrect/main.go
go fmt ./...; go vet ./...; golangci-lint run
```

## Code Style

### Go Version
**Go 1.25+** required (tool directive in go.mod).

### Imports
Three groups separated by blank lines: stdlib → third-party → local.

```go
import (
    "context"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"

    "jpcorrect-backend/internal/domain"
)
```

### Naming
Packages: lowercase single word. Types: PascalCase. Functions: PascalCase (exported), camelCase (internal). Variables: camelCase. UUIDs: `uuid.UUID` type.

### Domain Models
Structs with `gorm` and `json` tags. `uuid.UUID` IDs with `gorm:"type:uuid;primaryKey"`. Include `CreatedAt`, `UpdatedAt`, `DeletedAt`.

```go
type User struct {
    ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"user_id"`
    Email     string         `gorm:"uniqueIndex" json:"email"`
    CreatedAt time.Time      `json:"created_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}
```

### Repository Implementation
Implement domain interfaces. Use GORM directly. Pass `context.Context` first, use `.WithContext(ctx)`. Wrap ALL GORM errors with `MapGormError(err)`. Generate UUIDs in `Create` if missing.

```go
func (r *gormUserRepository) Create(ctx context.Context, user *User) error {
    if user.ID == uuid.Nil { user.ID = uuid.New() }
    return MapGormError(r.db.WithContext(ctx).Create(user).Error)
}
```

### Error Handling
Sentinel errors in `internal/domain/errors.go` (e.g., `ErrNotFound`). Map GORM/PG errors to domain errors in repository. API handlers check domain errors and return appropriate HTTP status codes.

### API Handlers
Handlers on `*API` struct. Parse UUIDs: `uuid.Parse(c.Param("id"))`. Return `c.JSON(status, gin.H{"error": ...})` on failure.

## Project Conventions

### Database
PostgreSQL via GORM. Connection in `internal/database/`. Auto-migrate: add models to `AutoMigrate` in `internal/cmd/api.go`.

### Environment Variables
`DATABASE_URL` (Postgres), `API_TOOLS_URL`, `PORT` (default 8080), `JWKS_URL`, `ALLOWED_ORIGINS` (comma-separated), `API_CERT_PATH`, `API_KEY_PATH`, `GIN_MODE` (debug/release).

### Tools
`go tool air`: Live reload. `go tool migrate`: Migrations (golang-migrate). `go tool sqlc`: SQL gen (configured, secondary to GORM).

### Git
Use conventional commit format for PR titles.

## Common Gotchas
1. **AutoMigrate**: Primary schema tool.
2. **UUIDs**: Always `uuid.UUID`, never strings/ints.
3. **Context**: Pass down everywhere.
4. **GORM Errors**: Always map to domain errors.
5. **Soft Delete**: Use `Unscoped()` for hard delete.
