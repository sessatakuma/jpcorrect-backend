# jpcorrect-backend Agent Guide

This guide provides essential information for AI coding agents working in this repository.

## Project Overview

Japanese language correction platform backend built with Go (1.25+), Gin, PostgreSQL, and GORM.

**Architecture**: Clean architecture with layered structure
- `cmd/jpcorrect/`: Application entry point
- `internal/api/`: HTTP handlers (Gin)
- `internal/domain/`: Domain models and repository interfaces
- `internal/repository/`: GORM-based repository implementations
- `internal/cmd/`: Command execution logic and server setup
- `internal/database/`: Database connection and GORM configuration
- `db/`: Database schema reference (migrations are minimal, uses AutoMigrate)

## Build, Run, and Test Commands

### Development
```bash
# Run with live reload (uses go tool air)
make air

# Run directly
go run cmd/jpcorrect/main.go

# Install dependencies
go mod download
```

### Database Operations
**Primary Method**: GORM `AutoMigrate` is used in `internal/cmd/api.go`.
Modify domain models in `internal/domain/` to change schema.

**Secondary/Manual Tools** (Available via `go tool` but not primary):
```bash
# Create migration file (if needed for manual fixes)
make migrate-create name=<migration_name>

# Run migrations manual
make migrate-up

# Rollback migrations manual
make migrate-down
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./internal/repository/...

# Run single test
go test -v ./internal/repository -run TestUserCreate

# Run with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

### Build
```bash
# Build binary
go build -o bin/jpcorrect cmd/jpcorrect/main.go
```

### Linting
```bash
# Format code
go fmt ./...

# Run go vet
go vet ./...
```

## Code Style Guidelines

### Go Version
- **Go 1.25+** required (uses `tool` directive in `go.mod`)

### Import Organization
Organize imports in THREE groups separated by blank lines:
1. Standard library
2. Third-party packages
3. Local project packages

```go
import (
    "context"
    "net/http"

    "github.com/gin-gonic/gin"
    "github.com/google/uuid"
    "gorm.io/gorm"

    "jpcorrect-backend/internal/domain"
    "jpcorrect-backend/internal/repository"
)
```

### Naming Conventions
- **Packages**: Lowercase, single word (e.g., `api`, `domain`)
- **Types/Structs**: PascalCase (e.g., `User`, `EventRepository`)
- **Functions**: PascalCase (Exported), camelCase (internal)
- **Variables**: camelCase
- **UUIDs**: Use `uuid.UUID` type, not string

### Domain Models (`internal/domain/`)
- Define structs with `gorm` and `json` tags.
- Use `uuid.UUID` for IDs with `gorm:"type:uuid;primaryKey"`.
- Include `CreatedAt`, `UpdatedAt`, and `DeletedAt` (for soft deletes).

```go
type User struct {
    ID        uuid.UUID      `gorm:"type:uuid;primaryKey" json:"user_id"`
    Email     string         `gorm:"uniqueIndex" json:"email"`
    CreatedAt time.Time      `json:"created_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at"`
}
```

### Repository Implementation (`internal/repository/`)
- Implement interfaces defined in `domain`.
- Use **GORM** directly.
- **Context**: Always pass `context.Context` as first argument and use `.WithContext(ctx)`.
- **Error Handling**: Wrap ALL GORM errors with `MapGormError(err)` before returning.
- **UUID Generation**: Generate new UUIDs in `Create` methods if not provided.

```go
func (r *gormUserRepository) Create(ctx context.Context, user *User) error {
    if user.ID == uuid.Nil {
        user.ID = uuid.New()
    }
    return MapGormError(r.db.WithContext(ctx).Create(user).Error)
}
```

### Error Handling
- Sentinel errors in `internal/domain/errors.go` (e.g., `ErrNotFound`).
- Map implementation-specific errors (GORM) to domain errors in repository layer.
- API Handlers check for domain errors and return appropriate HTTP status codes.

### API Handler Conventions
- Handlers on `*API` struct in `internal/api/`.
- Parse UUIDs safely: `uuid.Parse(c.Param("id"))`.
- Return `c.JSON(status, gin.H{"error": ...})` on failure.

## Project-Specific Conventions

### Database
- **PostgreSQL** via GORM.
- **Connection**: Managed in `internal/database/`.
- **Schema**: Auto-migrated. Add new models to `AutoMigrate` call in `internal/cmd/api.go`.

### Environment Variables
- `DATABASE_URL`: Postgres connection string.
- `API_TOOLS_URL`: External tools service.
- `PORT`: Service port.

### Tools
- `go tool air`: Live reload.
- `go tool migrate`: Database migrations (golang-migrate).
- `go tool sqlc`: SQL generation (configured in `go.mod` but currently unused/secondary to GORM).

## Git Guidelines

**Conventional Commits**: Use conventional commit format for PR titles.

## Common Gotchas

1. **AutoMigrate**: Primary schema management tool.
2. **UUIDs**: Always use `uuid.UUID`, not strings or ints.
3. **Context**: Pass it down everywhere.
4. **GORM Errors**: Always map them to domain errors.
5. **Soft Delete**: `Unscoped()` if you really need to delete.
