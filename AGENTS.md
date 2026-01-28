# jpcorrect-backend Agent Guide

This guide provides essential information for AI coding agents working in this repository.

## Project Overview

Japanese language correction platform backend built with Go, Gin, PostgreSQL, and sqlc.

**Architecture**: Clean architecture with layered structure
- `cmd/jpcorrect/`: Application entry point
- `internal/api/`: HTTP handlers (Gin)
- `internal/domain/`: Domain models and repository interfaces
- `internal/repository/`: PostgreSQL repository implementations
- `internal/cmd/`: Command execution logic
- `db/`: Database migrations, schemas, and sqlc queries

## Build, Run, and Test Commands

### Development
```bash
# Run with live reload (air)
make air

# Run directly
go run cmd/jpcorrect/main.go

# Install dependencies
go mod download
```

### Database Operations
```bash
# Generate repository code from SQL (sqlc)
make sqlc

# Create new migration
make migrate-create name=<migration_name>

# Run migrations
make migrate-up

# Rollback migrations
make migrate-down
```

### Testing
```bash
# Run all tests
go test ./...

# Run tests in specific package
go test ./internal/api/...

# Run single test
go test ./internal/api -run TestName

# Run with verbose output
go test -v ./...

# Run with coverage
go test -cover ./...
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out
```

**Note**: This codebase currently has no test files. When writing tests, follow standard Go testing conventions.

### Build
```bash
# Build binary
go build -o bin/jpcorrect cmd/jpcorrect/main.go

# Build for production
go build -ldflags="-s -w" -o bin/jpcorrect cmd/jpcorrect/main.go
```

### Linting and Formatting
```bash
# Format code
go fmt ./...

# Run go vet
go vet ./...

# Run golangci-lint (if installed)
golangci-lint run
```

## Code Style Guidelines

### Go Version
- **Go 1.25+** required

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
    "github.com/jackc/pgx/v5"
    
    "jpcorrect-backend/internal/domain"
    "jpcorrect-backend/internal/repository"
)
```

### Naming Conventions

- **Packages**: Lowercase, single word, no underscores (e.g., `api`, `domain`, `repository`)
- **Types/Structs**: PascalCase (e.g., `User`, `AICorrectionRepository`)
- **Functions/Methods**: PascalCase for exported, camelCase for unexported (e.g., `GetByID`, `fetch`)
- **Variables**: camelCase (e.g., `userID`, `aiCorrections`)
- **Constants**: PascalCase or ALL_CAPS depending on context
- **Interfaces**: PascalCase, often noun + "Repository" or "Service" (e.g., `UserRepository`)
- **Database fields**: snake_case in struct tags (e.g., `` `db:"user_id" json:"user_id"` ``)

### Type Definitions

**Domain Models**: Define in `internal/domain/`
```go
// Comment describes the struct and maps to DB table
type User struct {
    UserID int    `db:"user_id" json:"user_id"`
    Name   string `db:"name" json:"name"`
}
```

**Repository Interfaces**: Define in `internal/domain/` alongside models
```go
type UserRepository interface {
    GetByID(ctx context.Context, userID int) (*User, error)
    Create(ctx context.Context, user *User) error
    Update(ctx context.Context, user *User) error
    Delete(ctx context.Context, userID int) error
}
```

**Repository Implementations**: Define in `internal/repository/`
- Use unexported struct with lowercase name + "Repository" suffix
- Constructor returns interface type from domain
- Include private `fetch()` helper for query execution

```go
type postgresUserRepository struct {
    conn Connection
}

func NewPostgresUser(conn Connection) domain.UserRepository {
    return &postgresUserRepository{conn: conn}
}
```

### Error Handling

**Sentinel errors**: Define in `internal/domain/errors.go`
```go
var (
    ErrNotFound       = errors.New("record not found")
    ErrDuplicateEntry = errors.New("duplicate entry")
)
```

**Error checking patterns**:
- Always check errors immediately
- Return errors up the stack
- Check for specific domain errors (e.g., `domain.ErrNotFound`)
- Map domain errors to appropriate HTTP status codes in API handlers

```go
user, err := a.userRepo.GetByID(c.Request.Context(), id)
if err != nil {
    if err == domain.ErrNotFound {
        c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
        return
    }
    c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
    return
}
```

**Never**: Use `panic()` for business logic errors

### API Handler Conventions

- Handler methods on `*API` struct
- Handler names: `{Resource}{Action}Handler` (e.g., `UserGetHandler`, `UserCreateHandler`)
- Always accept `*gin.Context` parameter
- Parse parameters early, return 400 for invalid input
- Use `c.JSON()` for all responses
- Return appropriate HTTP status codes

### Repository Patterns

- All repository methods accept `context.Context` as first parameter
- Use PostgreSQL parameterized queries (`$1`, `$2`, etc.)
- Private `fetch()` helper for SELECT queries that return multiple rows
- Return `domain.ErrNotFound` when no rows found
- Use `RETURNING` clause for INSERT operations to get generated IDs

### Comments and Documentation

- Document all exported types, functions, and methods
- Use godoc-style comments (complete sentences starting with the name)
- Comments for structs should describe their purpose and DB mapping

```go
// User represents a user in the jpcorrect system.
// Maps to jpcorrect.user table.
type User struct {
    // ...
}
```

### Context Usage

- Always pass `context.Context` as first parameter
- Use `c.Request.Context()` in Gin handlers
- Never store context in structs

## Project-Specific Conventions

### Database Schema
- Schema: `jpcorrect`
- Use qualified table names in queries: `jpcorrect.user`, `jpcorrect.practice`
- Primary keys: `{table}_id` pattern (e.g., `user_id`, `practice_id`)
- Generated identity columns for all primary keys
- Foreign keys with explicit constraint names

### Environment Variables
- `DATABASE_URL`: PostgreSQL connection string (required)
- Load via `.env` file (using godotenv with autoload in main.go)
- Format: `postgresql://[user[:password]@][netloc][:port][/dbname][?param1=value1&...]`

### API Structure
- Base path: `/`
- Resource groups: `/{resources}` (e.g., `/users`, `/practices`)
- RESTful routes:
  - POST `/resource` - Create
  - GET `/resource/:id` - Get by ID
  - PUT `/resource/:id` - Update
  - DELETE `/resource/:id` - Delete
  - GET `/resource/{relation}/:id` - Get by relationship

### Graceful Shutdown
Server implements graceful shutdown with signal handling (SIGINT, SIGTERM).

## Pull Request Guidelines

**Conventional Commits**: Use conventional commit format for PR titles

**PR Template** (in Traditional Chinese):
- 目的 (Purpose): Describe what problem is solved
- 方法／實作說明 (Implementation): Technical approach and key changes
- 關聯 Issue (Related Issues): Link related issues
- 附註 (Notes): Additional context, TODOs

## Common Gotchas

1. **Migration numbering**: Use sequential numbers with `make migrate-create`
2. **SQLC generation**: Run `make sqlc` after changing queries in `db/queries/`
3. **Context propagation**: Always pass context from Gin handler to repository
4. **Error handling**: Check for `domain.ErrNotFound` explicitly before generic error handling
5. **Database connections**: Use connection pooling via `pgxpool`, not individual connections
6. **JSON tags**: Always include both `db` and `json` tags on struct fields

## Dependencies Management

- Use `go mod` for dependency management
- Run `go mod tidy` after adding/removing dependencies
- Commit `go.mod` and `go.sum`

## Related Documentation

- [Gin Framework](https://gin-gonic.com/docs/)
- [pgx (PostgreSQL driver)](https://github.com/jackc/pgx)
- [sqlc (SQL code generator)](https://docs.sqlc.dev/en/latest/)
- [golang-migrate](https://github.com/golang-migrate/migrate)
