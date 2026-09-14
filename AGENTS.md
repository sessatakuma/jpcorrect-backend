# jpcorrect-backend Agent Guide

Japanese language correction platform backend: Go 1.25+, Gin, PostgreSQL, GORM.

> `CLAUDE.md` is a symlink to this file. Edit `AGENTS.md` — both stay in sync.

## Layout

Clean architecture. `cmd/jpcorrect/` (main server) and `cmd/webrtc-demo/`
(separate static-server binary); `internal/{api,domain,repository,cmd,database}/`
for handlers, models + repo interfaces, GORM implementations, server wiring, and
the DB connection.

`internal/api/api_tools.go` holds 7 handlers that reverse-proxy to **API-tools**,
a Python FastAPI service in a [separate repo](https://github.com/sessatakuma/API-tools)
— not a submodule, coupling is HTTP-only via `API_TOOLS_URL`. Local dev expects
it cloned as a sibling at `../API-tools/` and run with `uv`; the deploy stack
pulls it from GHCR instead.

### API-tools compatibility

The proxy handlers and their typed models match api-tools branch
`feat/commercializable-openjtalk` @ `8bc3781` (local fugashi + UniDic: no
`/MarkFurigana/`, added `/MarkAccent/stream/`, `script` option on both accent
endpoints). When api-tools changes its schema or endpoint set, update the proxy
and this line in the same PR. Pre-#38 the backend targeted the Yahoo-MA-era
`feat/docker-compose` @ `d8acc55`.

## Local development

Backend and api-tools run on the host; only Postgres runs in Docker (repo-root
`compose.yml`, bound to `127.0.0.1:5432`, volume `postgres_local_data` — fully
separate from the deploy stack's volumes).

```bash
cp .env.example .env                  # then fill in JWKS_URL
docker compose up -d                  # local Postgres

cd ../API-tools && uv run uvicorn main:app --host 127.0.0.1 --port 8000

make air                              # backend with live reload (go tool air)
go run cmd/jpcorrect/main.go          # or without live reload
make swag                             # regenerate Swagger docs
```

Smoke tests with all three up: `/healthz` → `ok`, `/swagger/index.html` → 200
(debug only), `http://127.0.0.1:8000/docs` → 200.

## Testing

```bash
go test ./...
go test ./internal/api/...                            # single package
go test -v ./internal/api -run TestAPIKeyMiddleware    # single test
go test -coverprofile=coverage.out ./...; go tool cover -html=coverage.out
```

Tests use `testify` + `httptest`; the sqlmock-based repository tests were
removed in #49, so the repository layer currently has no unit tests.

## Swagger

`make swag` runs `swag init` with `--parseDependency --parseInternal` — without
both flags handler annotations are not found. The blank import
`_ "jpcorrect-backend/docs/swagger"` in `internal/api/api.go` registers the
generated spec; drop it and `/swagger/doc.json` returns 500.

Two schemes are declared in `cmd/jpcorrect/main.go`: `BearerAuth`
(`Authorization`, tolerates a bare `<jwt>` as well as `Bearer <jwt>`) for
JWT-protected `/v1/*` handlers, and `ApiKeyAuth` (`X-API-Key`) for the 7
api-tools proxy routes, which do **not** accept JWT. Every authenticated handler
needs a `// @Security <Scheme>` line above `// @Router`, or Swagger UI won't
attach the header.

## Code conventions

- **Schema**: GORM `AutoMigrate` in `internal/cmd/api.go` is the only schema
  tool — add every new domain model to that call. Models: `User`, `Guild`,
  `GuildAttendee`, `Event`, `EventAttendee`, `Transcript`, `Mistake`.
- **UUIDs**: always `uuid.UUID`, never strings/ints; generate with `uuid.New()`
  in `Create` when missing.
- **Context**: first argument everywhere, `.WithContext(ctx)` on every GORM call.
- **Errors**: repositories must map GORM/PG errors through `MapGormError()` to
  the sentinels in `internal/domain/errors.go` (`ErrNotFound`,
  `ErrDuplicateEntry`, `ErrHasRelatedRecords`) — never return raw GORM errors.
  Auth is the exception: `domain.AuthError` struct with `StatusCode`, `Message`,
  `Details`.
- **Soft delete**: only `User`, `Guild`, `Event` carry `gorm.DeletedAt` (use
  `Unscoped()` for a hard delete). The other four models hard-delete.
- **Imports**: stdlib → third-party → local.
- `/v1/practices` routes use the `Event` model (backward-compatibility naming).
- `Transcript.Accent` is `datatypes.JSON` / `jsonb` — the only JSONB column.
- `GuildAttendeeRepository` lives in `gorm_guild.go`, not its own file.
- TLS is opt-in: HTTPS only when both `API_CERT_PATH` and `API_KEY_PATH` exist,
  otherwise HTTP with a warning.

## Environment variables

`.env` is auto-loaded via `github.com/joho/godotenv/autoload`.

| Variable | Notes |
| --- | --- |
| `DATABASE_URL` | **Required.** `127.0.0.1:5432` locally; `postgres-prod:5432` / `postgres-dev:5432` in the deploy stack |
| `JWKS_URL` | **Required in release** — the app fatals on an empty value unless `GIN_MODE=debug` |
| `GIN_MODE` | `debug` **skips APIKeyMiddleware + AuthMiddleware on all `/v1` routes** and exposes Swagger UI. Only safe behind an edge gateway |
| `API_TOOLS_URL` | `http://127.0.0.1:8000` locally; `http://jpcorrect-api-tools-{prod,dev}:8000` in the stack. The backend → api-tools call needs no key |
| `CLIENT_API_KEY` | Inbound `X-API-Key` for the 7 proxy routes, release mode only. Empty ⇒ those routes always 401 |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins. Empty = reject all in release, allow all in debug |
| `PORT` | Default `8080` |
| `API_CERT_PATH` / `API_KEY_PATH` | Default `./certs/{cert,key}.pem` |
| `WEBRTC_CONN_SEC` / `WEBRTC_CONN_MAX` | WebRTC rate-limit window / max connections (`10` / `15`) |
| `WEBRTC_DEMO_PORT` / `WEBRTC_DEMO_BASE_DIR` / `WEBRTC_DEMO_CERT_PATH` / `WEBRTC_DEMO_KEY_PATH` | `cmd/webrtc-demo` only |

Compose-level (never seen by the Go process): `POSTGRES_PORT` for the local
stack; `POSTGRES_{PROD,DEV}_PASSWORD`, `{BACKEND,API_TOOLS}_{PROD,DEV}_IMAGE`
and `HOME` for the deploy stack. The api-tools containers need no env vars at
all post local-unidic — `YAHOO_API_KEY` is gone.

Also note: `NewRateLimiter(10*time.Second, 15)` in `internal/api/api.go:56`.

## Deployment

`deploy/compose.yml` (top-level `name: jpcorrect-backend`, `./` mounts relative
to `deploy/`) runs two independent environments on one host behind a single
Cloudflare Tunnel:

| | prod | dev |
| --- | --- | --- |
| Hostname | `api.sessatakuma.dev` | `api-dev.sessatakuma.dev` |
| `GIN_MODE` | `release`, all middlewares on | `debug`, middlewares skipped — gated only by Cloudflare Access |
| Image tag | `:stable` (pushed on a `v*.*.*` git tag) | `:dev` (every main merge) |
| Watchtower | `watchtower-prod`, daily 03:00 Asia/Taipei | `watchtower-dev`, 5-min poll |

Each env owns a Postgres and an api-tools instance on its own bridge network;
only the two backends and `cloudflared` sit on `jpcorrect-shared`. So
`backend-dev` cannot resolve `postgres-prod` or `api-tools-prod` at all — that
isolation is deliberate. The two watchtowers are separated by
`com.centurylinklabs.watchtower.scope=<env>` labels and all four app containers
now auto-update.

The deploy host is arm64, so images must carry a `linux/arm64` manifest. Backend
CD has built `linux/amd64,linux/arm64` since #38, but `:stable` is only
republished on a `v*.*.*` tag — the first multi-arch backend `:stable` is
**`v1.0.0`**. api-tools went multi-arch in
[API-tools#64](https://github.com/sessatakuma/API-tools/pull/64) (released as
`v1.1.0`); a `:stable` cut before that is still amd64-only, so verify with
`docker buildx imagetools inspect <ref> | grep Platform` before letting
`watchtower-prod` pull it.

```bash
make -C deploy up          # both envs + cloudflared + both watchtowers
make -C deploy up-prod     # or up-dev / up-infra
make -C deploy pull-prod   # manual pull of :stable (pull-dev for :dev)
make -C deploy down
```

Everything deploy-only lives under `deploy/`: `compose.yml` + `Makefile` +
`cloudflared/config.yml` are tracked; `.env` (compose-interpolated Postgres
passwords), `env/prod`, `env/dev` and `cloudflared/creds/` are host-only and
gitignored, each with a tracked `.example` alongside.

`deploy/README.md` is the operational reference: one-time host setup, GHCR
login (both packages are private), why the Postgres passwords must live in
`deploy/.env` and be mirrored URL-encoded into each `deploy/env/<env>`'s
`DATABASE_URL`, why an `env_file` edit needs `--force-recreate` rather than
`docker restart`, tunnel DNS routes, and the watchtower dry-run recipe. Read it
before touching the host.

## CI

PR checks run through `sessatakuma/org-workflows`: `go mod tidy`,
`golangci-lint` (`.golangci.yml`: errcheck, govet, ineffassign, staticcheck,
unused), tests with `-race`, build, plus `yamllint` (default rules — 80-col
lines fail; `docs/swagger/` is excluded via `.yamllint`), `jq`, and `taplo`.
`docker-build.yml` separately builds the image and smoke-tests it offline.

## Git

Conventional Commits, header ≤ 72 chars, lowercase imperative summary:
`feat(api): add JWT authentication middleware`. Types: `feat`, `fix`, `docs`,
`style`, `refactor`, `perf`, `test`, `build`, `ci`, `chore`, `revert`,
`hotfix`; `!` plus a `BREAKING CHANGE:` footer for breaking changes; issue refs
in the footer. PR titles follow the same format (≤ 75 chars) and the body uses
`.github/PULL_REQUEST_TEMPLATE.md`.

## Gotchas

1. **`make air` needs `go` on the `/bin/sh` PATH.** The Makefile shell does not
   source your zshrc, so `which go` can work while `make air` reports
   `go: not found`. Use `PATH="/usr/local/go/bin:$PATH" make air`.
2. **Debug mode is wide open.** It skips both `/v1` middlewares, so any
   non-localhost debug build needs an edge gateway in front of it.
3. **`CLIENT_API_KEY` is inbound-only.** It guards the proxy routes in release
   mode; the internal backend → api-tools call is keyless either way.
4. **Old api-tools images assert on `YAHOO_API_KEY` at import.** Pin a
   local-unidic image to avoid that.
5. The repo-root `compose.yml` (local Postgres) and `deploy/compose.yml` (the
   full stack) are two unrelated stacks that share a filename.
