# jpcorrect-backend Agent Guide

Essential information for AI coding agents working in this repository.

> **Note:** `CLAUDE.md` is a symlink to this file. Edit `AGENTS.md` — both stay in sync.

## Project Overview

Japanese language correction platform backend: Go 1.25+, Gin, PostgreSQL, GORM.

**Architecture**: Clean architecture
- `cmd/jpcorrect/`: Main API server entry point
- `cmd/webrtc-demo/`: Standalone WebRTC demo web server (separate binary)
- `internal/api/`: HTTP handlers (Gin), including `api_tools.go` (proxy to API-tools service) and `webrtc.go`
- `internal/domain/`: Domain models and repository interfaces
- `internal/repository/`: GORM implementations
- `internal/cmd/`: Command execution and server setup
- `internal/database/`: Database connection and GORM config
- `API-tools/`: Sibling Python FastAPI service in a **separate repo** ([sessatakuma/API-tools](https://github.com/sessatakuma/API-tools)). Cloned by devs into `./API-tools/` and run with `uv`; reached at runtime via `API_TOOLS_URL`. Not a submodule — coupling is HTTP-only, see "API-tools compatibility" below.

## API-tools compatibility

The 7 proxy handlers in `internal/api/api_tools.go` are aligned to the HTTP surface of api-tools branch **`spike/local-unidic`** (commit `46f6c43` or newer — the local fugashi + UniDic migration that removed `/MarkFurigana/` and added `/MarkAccent/stream/`). When api-tools changes its request/response schema or adds/removes endpoints, update both the backend proxy + this line in the same PR.

Compatibility log (update on every contract-affecting change):

| Backend version | Compatible api-tools | Notes |
| --- | --- | --- |
| `v0.2.x` (post-#38) | `spike/local-unidic @ 46f6c43+` | local UniDic engine, no `/MarkFurigana`, `/MarkAccent/stream` available |
| pre-#38 | `feat/docker-compose @ d8acc55` | Yahoo MA API era, `/MarkFurigana` still present |

## Build, Run, Test

### One-time setup
The backend and `api-tools` live in separate repos and are cloned as siblings:

```bash
git clone git@github.com:sessatakuma/jpcorrect-backend.git
git clone git@github.com:sessatakuma/API-tools.git   # sibling, NOT a submodule
cd jpcorrect-backend
```

### Local development (preferred)
Local development runs backend and `api-tools` directly on the host; only Postgres runs in Docker.

```bash
cp .env.example .env                  # one-time setup — then fill in JWKS_URL

make db-up                            # start Postgres in Docker (bound to 127.0.0.1:5432)
make db-logs                          # tail Postgres logs
make db-down                          # stop Postgres

make api-tools                        # run Python API-tools on 127.0.0.1:8000 via uv
make air                              # run backend with live reload (go tool air)
make swag                             # regenerate Swagger docs

go run cmd/jpcorrect/main.go          # run backend directly (no live reload)
```

In this mode, `.env` has `DATABASE_URL=...@127.0.0.1:5432/...` and `API_TOOLS_URL=http://127.0.0.1:8000`.

**Verified smoke-test endpoints** (all three services up):
- `curl http://127.0.0.1:8080/healthz` → `ok`
- `curl http://127.0.0.1:8080/swagger/index.html` → 200 (debug only)
- `curl http://127.0.0.1:8000/docs` → 200 (api-tools FastAPI)

### Deployment stack
For CD / production-style deploys, use `compose.deploy.yml` with a separate `.env.deploy`:

```bash
docker network create jpcorrect-shared     # one-time, shared bridge between this stack and API-tools
cp .env.deploy.example .env.deploy
make deploy-up                             # docker compose -f compose.deploy.yml up -d
make deploy-down
```

The `jpcorrect-shared` external network lets the `backend` container reach the sibling `api-tools` container by name (`API_TOOLS_URL=http://jpcorrect-api-tools:8000`) instead of going through the host. `postgres` stays on the project default network and is not exposed to api-tools. The API-tools repo's `docker-compose.yml` also joins this network — bring it up first (`docker compose -f ../API-tools/docker-compose.yml up -d`) so its container exists for DNS resolution.

In this mode, `.env.deploy` has `DATABASE_URL=...@postgres:5432/...` (Docker hostname) and `API_TOOLS_URL=http://jpcorrect-api-tools:8000` (api-tools container name on the shared bridge). The stack includes `backend` (pulled from `BACKEND_IMAGE`), `postgres`, and `cloudflared` (file-based credentials mounted from `./.cloudflared/`, forwarding to `http://backend:8080`). See `../talkuma-outline/README.md` for the one-time `tunnel login` / `tunnel create` / `tunnel route dns` setup pattern this repo follows.

### Database
GORM `AutoMigrate` in `internal/cmd/api.go` is the primary schema tool. When adding a new domain model, add it to the `AutoMigrate(...)` call.

Models: `User`, `Guild`, `GuildAttendee`, `Event`, `EventAttendee`, `Transcript`, `Mistake`.

### Testing
```bash
go test ./...                                                      # All tests
go test ./internal/repository/...                                  # Specific package
go test -v ./internal/repository -run TestUserCreate               # Single test
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
make swag   # runs: go tool swag init -g cmd/jpcorrect/main.go -o docs/swagger --parseDependency --parseInternal
```

The `_ "jpcorrect-backend/docs/swagger"` import in `api.go` registers generated specs. CI runs `yamllint` on all YAML — `docs/swagger/` is excluded via `.yamllint`.

### Security schemes
Two `@securityDefinitions.apikey` schemes are declared at the top of `cmd/jpcorrect/main.go`:

- `BearerAuth` — header `Authorization`. Used by every JWT-protected handler under `v1.Use(AuthMiddleware())`. The parser tolerates a bare `<jwt>` as well as `Bearer <jwt>` (case-insensitive), so users can paste the raw token into Swagger UI's Authorize dialog.
- `ApiKeyAuth` — header `X-API-Key`. Used by the 7 api-tools proxy handlers under `apiTools.Use(APIKeyMiddleware())`. JWT is **not** accepted on these routes.

Every authenticated handler must carry a `// @Security <Scheme>` line above `// @Router`, otherwise Swagger UI won't show a lock icon and "Try it out" won't attach the header. Pattern: `BearerAuth` for `/v1/{users,guilds,...}`, `ApiKeyAuth` for `/v1/{mark-accent,dict-query,...}`.

## Project Conventions

### Environment Variables
`.env` is auto-loaded by `github.com/joho/godotenv/autoload` (blank import in `main.go`).

| Variable | Required | Default | Notes |
| --- | --- | --- | --- |
| `DATABASE_URL` | Yes | — | Postgres connection. `127.0.0.1:5432` for local dev, `postgres:5432` for deploy stack |
| `JWKS_URL` | Yes | — | App fatals if empty |
| `PORT` | No | `8080` | |
| `API_TOOLS_URL` | No | — | URL of the `API-tools` service. `http://127.0.0.1:8000` for local dev, `http://jpcorrect-api-tools:8000` for the deploy stack (via shared bridge). The Python service no longer requires an `X-API-KEY` header on local server-to-server calls |
| `CLIENT_API_KEY` | No | — | Inbound `X-API-Key` for the 7 api-tools endpoints (JWT not accepted). Empty value locks those routes (always 401) |
| `ALLOWED_ORIGINS` | No | — | Comma-separated CORS origins. Empty = reject all in release, allow all in debug |
| `GIN_MODE` | No | — | `debug` or `release` |
| `API_CERT_PATH` | No | `./certs/cert.pem` | Enables HTTPS if both cert and key exist |
| `API_KEY_PATH` | No | `./certs/key.pem` | |
| `WEBRTC_CONN_SEC` / `WEBRTC_CONN_MAX` | No | `10` / `15` | WebRTC rate limit window (seconds) and max connections |
| `WEBRTC_DEMO_PORT` | No | `3000` | Port for the `cmd/webrtc-demo` server |
| `WEBRTC_DEMO_BASE_DIR` / `WEBRTC_DEMO_CERT_PATH` / `WEBRTC_DEMO_KEY_PATH` | No | — | Paths for the WebRTC demo static server |

Deploy-stack only (read by `compose.deploy.yml`, not the Go process):

| Variable | Notes |
| --- | --- |
| `BACKEND_IMAGE` | Backend image to pull (default `ghcr.io/sessatakuma/jpcorrect-backend:latest`) |
| `BACKEND_ENV_FILE` | Env file passed into the backend container (default `.env`; the `deploy-up` target sets `.env.deploy`) |
| `POSTGRES_PORT` | Host-side bind port for Postgres (default `5432`) |

The `api-tools` service needs no env vars: accent/furigana come from a bundled local UniDic dict (the old `YAHOO_API_KEY` requirement is gone post local-unidic).

### TLS
Server checks if both `API_CERT_PATH` and `API_KEY_PATH` files exist. If yes → HTTPS; if no → HTTP with warning log.

### Rate Limiter
`NewRateLimiter(10*time.Second, 15)` in `internal/api/api.go:56` — 10-second window, max 15 connections.

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
Use Conventional Commits (https://www.conventionalcommits.org/en/v1.0.0/):

```
<type>(<scope>): <short summary>
```

- `<type>` ∈ `feat`, `fix`, `docs`, `style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`, `hotfix`
- `<scope>` optional (e.g., `api`, `auth`, `deps`)
- Header ≤ 72 chars, lowercase imperative ("add", "fix", "update")
- Append `!` after `<scope>` and add a `BREAKING CHANGE:` footer for breaking changes
- Optional body wrapped at ~72 chars describing the *what* and *why*
- Footer takes issue refs (`Closes #123`, `Refs #456`) and breaking-change notes

Examples: `feat(api): add JWT authentication middleware`, `fix(ui)!: remove deprecated button variants`.

### Pull Requests
- **Title**: Conventional Commits format, ≤ 75 characters.
- **Body**: Use the template in `.github/PULL_REQUEST_TEMPLATE.md`

## Common Gotchas

1. **Swagger blank import**: `_ "jpcorrect-backend/docs/swagger"` must exist in `api.go` or `/swagger/doc.json` returns 500
2. **AutoMigrate**: Primary schema tool. Add new models to the call in `internal/cmd/api.go`
3. **JWKS_URL**: App exits immediately if not set
4. **UUIDs**: Always `uuid.UUID`, never strings/ints
5. **Context**: Pass down everywhere, `.WithContext(ctx)` on all GORM calls
6. **GORM Errors**: Always map via `MapGormError()`, never return raw GORM errors from repository
7. **Soft Delete**: Only User/Guild/Event. Use `Unscoped()` for hard delete on those
8. **`DATABASE_URL` hostname**: `127.0.0.1` for local dev (`make air`), `postgres` only inside the deploy compose stack
9. **`API_TOOLS_URL` from containers**: `host.docker.internal` in the deploy stack — `api-tools` runs on the host, not in Docker
10. **`make swag` flags**: Must include `--parseDependency --parseInternal` or handler annotations won't be found
11. **`CLIENT_API_KEY` is inbound only**: It guards the 7 api-tools proxy routes (X-API-Key only — JWT is rejected there; empty value returns 401). The internal jp backend → API-tools call is now keyless (local server-to-server, no auth required), so there is no second key to configure.
12. **`make air` needs `go` on `/bin/sh` PATH**: The Makefile invokes `go tool air` via the default shell, which does not source your zshrc. If `which go` works in your terminal but `make air` reports `go: not found`, prepend the path explicitly: `PATH="/usr/local/go/bin:$PATH" make air` (or export `PATH` in `~/.profile`).
13. **api-tools needs no env vars** (post local-unidic): accent/furigana come from a bundled local UniDic dict, so the old `YAHOO_API_KEY` requirement is gone. `make api-tools` runs `uv run uvicorn ...` with no env. (Older Yahoo-MA-era images still assert on `YAHOO_API_KEY` at import — pin a local-unidic image to avoid that.)
