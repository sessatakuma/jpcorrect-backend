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
Local development runs backend and `api-tools` directly on the host; only Postgres runs in Docker via the dedicated local compose file.

```bash
cp .env.example .env                  # one-time setup — then fill in JWKS_URL

make db-up                            # start local Postgres in Docker (compose.local.yml, bound to 127.0.0.1:5432)
make db-logs                          # tail Postgres logs
make db-down                          # stop Postgres

make api-tools                        # run Python API-tools on 127.0.0.1:8000 via uv
make air                              # run backend with live reload (go tool air)
make swag                             # regenerate Swagger docs

go run cmd/jpcorrect/main.go          # run backend directly (no live reload)
```

In this mode, `.env` has `DATABASE_URL=...@127.0.0.1:5432/...` and `API_TOOLS_URL=http://127.0.0.1:8000`. The local Postgres uses the `postgres_local_data` volume — completely separate from the deploy stack's `postgres_prod_data` / `postgres_dev_data`.

**Verified smoke-test endpoints** (all three services up):
- `curl http://127.0.0.1:8080/healthz` → `ok`
- `curl http://127.0.0.1:8080/swagger/index.html` → 200 (debug only)
- `curl http://127.0.0.1:8000/docs` → 200 (api-tools FastAPI)

### Deployment stack (two environments on one host)
`compose.deploy.yml` hosts **two backend instances** on this machine, both exposed via a single Cloudflare Tunnel:

- **Prod** (`backend-prod` → `api.sessatakuma.dev`) — `GIN_MODE=release`. All middlewares active (`CLIENT_API_KEY` on api-tools routes, JWT on `/v1/*`). Image: `:stable` (only updated when a `v*.*.*` git tag is pushed).
- **Dev** (`backend-dev` → `api-dev.sessatakuma.dev`) — `GIN_MODE=debug`. APIKey + Auth middlewares are **skipped**; access is gated solely at the edge by a Cloudflare Access Zero Trust policy. Swagger UI is visible. Image: `:latest` (every main merge).

A single `watchtower` container polls GHCR every 5 min and auto-restarts whichever backend image changed (label-enable mode — only backend-prod and backend-dev are watched).

Each env has its own Postgres on an isolated bridge network:

| Container | prod-net | dev-net | jpcorrect-shared |
|---|---|---|---|
| postgres-prod | ✓ | | |
| postgres-dev | | ✓ | |
| backend-prod | ✓ | | ✓ |
| backend-dev | | ✓ | ✓ |
| cloudflared | | | ✓ |

So `backend-dev` cannot reach `postgres-prod` (different network, no DNS). Both backends reach the sibling `jpcorrect-api-tools` container via `jpcorrect-shared`.

#### One-time host setup
```bash
docker network create jpcorrect-shared 2>/dev/null || true

# Bring up sibling api-tools so its container exists for DNS resolution
docker compose -f ../API-tools/docker-compose.yml up -d

# Cloudflare tunnel `jb` already exists in ./.cloudflared/. Register DNS routes:
docker run --rm -v $PWD/.cloudflared:/home/nonroot/.cloudflared \
  cloudflare/cloudflared:latest tunnel route dns jb api.sessatakuma.dev
docker run --rm -v $PWD/.cloudflared:/home/nonroot/.cloudflared \
  cloudflare/cloudflared:latest tunnel route dns jb api-dev.sessatakuma.dev

# Cloudflare Access (Zero Trust dashboard — no IaC):
#   Access → Applications → Add → Self-hosted
#     Application domain: api-dev.sessatakuma.dev
#     Policy: e.g. include emails ending in @sessatakuma.dev
# Do NOT add an Access app for api.sessatakuma.dev (prod stays publicly reachable).

# GHCR auth (only if the image is private — Watchtower mounts ~/.docker/config.json):
docker login ghcr.io -u <github-user>   # paste a PAT with read:packages
```

#### Day-to-day
```bash
cp .env.deploy.prod.example .env.deploy.prod   # fill in prod CLIENT_API_KEY, JWKS_URL, etc.
cp .env.deploy.dev.example  .env.deploy.dev    # dev: leave CLIENT_API_KEY / JWKS_URL empty

make deploy-up                # both envs + cloudflared + watchtower
make deploy-up-prod           # just prod (postgres-prod + backend-prod)
make deploy-up-dev            # just dev  (postgres-dev  + backend-dev)
make deploy-up-infra          # just cloudflared + watchtower
make deploy-pull-prod         # manual pull of :stable + restart backend-prod
make deploy-pull-dev          # manual pull of :latest + restart backend-dev
make deploy-down              # everything

docker exec -it jpcorrect-postgres-prod psql -U jpcorrect   # psql into prod DB
```

Cloudflared ingress lives in `deploy/cloudflared/config.yml` (bind-mounted into the container on top of `.cloudflared/`); credentials (`cert.pem`, `<uuid>.json`) stay in `.cloudflared/` and remain gitignored. See `../talkuma-outline/README.md` for the one-time `tunnel login` / `tunnel create` walkthrough.

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
| `DATABASE_URL` | Yes | — | Postgres connection. `127.0.0.1:5432` for local dev, `postgres-prod:5432` or `postgres-dev:5432` for the deploy stack |
| `JWKS_URL` | Yes in release | — | App fatals if empty AND `GIN_MODE != debug`. In debug mode AuthMiddleware is skipped so JWKS isn't loaded. |
| `PORT` | No | `8080` | |
| `API_TOOLS_URL` | No | — | URL of the `API-tools` service. `http://127.0.0.1:8000` for local dev, `http://jpcorrect-api-tools:8000` for the deploy stack (via shared bridge). The Python service no longer requires an `X-API-KEY` header on local server-to-server calls |
| `CLIENT_API_KEY` | No | — | Inbound `X-API-Key` for the 7 api-tools endpoints (JWT not accepted on those routes). Empty value locks those routes (always 401) **in release mode**. In debug mode the middleware is skipped entirely so the value is unused. |
| `ALLOWED_ORIGINS` | No | — | Comma-separated CORS origins. Empty = reject all in release, allow all in debug |
| `GIN_MODE` | No | — | `debug` or `release`. **Debug mode skips APIKeyMiddleware + AuthMiddleware on all /v1 routes** (only safe behind an edge gateway like Cloudflare Access). Also makes Swagger UI visible at `/swagger/index.html`. |
| `API_CERT_PATH` | No | `./certs/cert.pem` | Enables HTTPS if both cert and key exist |
| `API_KEY_PATH` | No | `./certs/key.pem` | |
| `WEBRTC_CONN_SEC` / `WEBRTC_CONN_MAX` | No | `10` / `15` | WebRTC rate limit window (seconds) and max connections |
| `WEBRTC_DEMO_PORT` | No | `3000` | Port for the `cmd/webrtc-demo` server |
| `WEBRTC_DEMO_BASE_DIR` / `WEBRTC_DEMO_CERT_PATH` / `WEBRTC_DEMO_KEY_PATH` | No | — | Paths for the WebRTC demo static server |

Deploy-stack only (read by `compose.deploy.yml` for compose-level substitution, not the Go process):

| Variable | Notes |
| --- | --- |
| `BACKEND_PROD_IMAGE` | backend-prod image to run (default `ghcr.io/sessatakuma/jpcorrect-backend:stable`) |
| `BACKEND_DEV_IMAGE` | backend-dev image to run (default `ghcr.io/sessatakuma/jpcorrect-backend:latest`) |
| `HOME` | Used to mount `~/.docker/config.json` into Watchtower for GHCR auth |

Local-only (`compose.local.yml`):

| Variable | Notes |
| --- | --- |
| `POSTGRES_PORT` | Host-side bind port for the local-dev Postgres (default `5432`) |

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
3. **JWKS_URL**: App exits immediately if not set AND `GIN_MODE != debug`. Debug mode skips AuthMiddleware so it tolerates an empty JWKS_URL.
4. **UUIDs**: Always `uuid.UUID`, never strings/ints
5. **Context**: Pass down everywhere, `.WithContext(ctx)` on all GORM calls
6. **GORM Errors**: Always map via `MapGormError()`, never return raw GORM errors from repository
7. **Soft Delete**: Only User/Guild/Event. Use `Unscoped()` for hard delete on those
8. **`DATABASE_URL` hostname**: `127.0.0.1` for local dev (`make air`), `postgres-prod` or `postgres-dev` inside the deploy compose stack (depending on which backend is talking)
9. **`API_TOOLS_URL` from containers**: `http://jpcorrect-api-tools:8000` in the deploy stack — both backends reach the sibling container via the `jpcorrect-shared` external bridge
10. **`make swag` flags**: Must include `--parseDependency --parseInternal` or handler annotations won't be found
11. **`CLIENT_API_KEY` is inbound only AND release-mode only**: It guards the 7 api-tools proxy routes when `GIN_MODE=release` (X-API-Key only — JWT is rejected there; empty value returns 401). In debug mode `APIKeyMiddleware` is skipped entirely so the value is unused; **an edge gateway (e.g. Cloudflare Access) must protect any non-localhost exposure of a debug-mode build**. The internal jp backend → API-tools call is keyless either way.
12. **`make air` needs `go` on `/bin/sh` PATH**: The Makefile invokes `go tool air` via the default shell, which does not source your zshrc. If `which go` works in your terminal but `make air` reports `go: not found`, prepend the path explicitly: `PATH="/usr/local/go/bin:$PATH" make air` (or export `PATH` in `~/.profile`).
13. **api-tools needs no env vars** (post local-unidic): accent/furigana come from a bundled local UniDic dict, so the old `YAHOO_API_KEY` requirement is gone. `make api-tools` runs `uv run uvicorn ...` with no env. (Older Yahoo-MA-era images still assert on `YAHOO_API_KEY` at import — pin a local-unidic image to avoid that.)
