# jpcorrect-backend Agent Guide

Go 1.25+ / Gin / GORM / PostgreSQL backend for the jpcorrect Japanese
correction platform. Human-facing docs and deploy background: `README.md`;
deploy runbook: `deploy/README.md`.

## Layout

- `cmd/jpcorrect/` main server; `cmd/webrtc-demo/` separate static-server
  binary. `internal/{api,domain,repository,cmd,database}/` — handlers; models
  + repo interfaces; GORM implementations; server wiring; DB connection.
- `internal/api/api_tools.go` holds 7 handlers that reverse-proxy
  **API-tools**, a Python FastAPI service in a
  [separate repo](https://github.com/sessatakuma/API-tools) — not a submodule,
  coupling is HTTP-only via `API_TOOLS_URL` (local dev: sibling clone at
  `../API-tools/`, run with `uv`). When api-tools' endpoints or schemas
  change, update the proxy handlers to match.

## Local development

```bash
cp .env.example .env                  # then fill in JWKS_URL
docker compose up -d                  # local Postgres only (127.0.0.1:5432)
cd ../API-tools && uv run uvicorn main:app --host 127.0.0.1 --port 8000
make air                              # backend with live reload
make swag                             # regenerate Swagger docs
```

Smoke test with all three up: `/healthz` → `ok`, `/swagger/index.html` → 200
(debug only), `http://127.0.0.1:8000/docs` → 200.

## Testing

```bash
go test ./...
go test ./internal/api/...                            # single package
go test -v ./internal/api -run TestAPIKeyMiddleware    # single test
```

`testify` + `httptest`; repository layer has no unit tests (sqlmock tests
removed in #49).

## Swagger

- `make swag` runs `swag init --parseDependency --parseInternal` — without
  both flags handler annotations are not found.
- Keep the blank import `_ "jpcorrect-backend/docs/swagger"` in
  `internal/api/api.go`; without it `/swagger/doc.json` returns 500.
- Schemes in `cmd/jpcorrect/main.go`: `BearerAuth` (`Authorization`, tolerates
  a bare `<jwt>`) for JWT-protected `/v1/*` handlers; `ApiKeyAuth`
  (`X-API-Key`) for the 7 api-tools proxy routes, which do **not** accept JWT.
  Every authenticated handler needs a `// @Security <Scheme>` line above
  `// @Router`, or Swagger UI won't attach the header.

## Code conventions

- **Schema**: the GORM `AutoMigrate` call in `internal/cmd/api.go` is the only
  schema tool — add every new domain model there (User, Guild,
  GuildAttendee, Event, EventAttendee, Transcript, Mistake).
- **UUIDs**: always `uuid.UUID`, never strings/ints; generate with
  `uuid.New()` in `Create` when missing.
- **Context**: first argument everywhere; `.WithContext(ctx)` on every GORM
  call.
- **Errors**: repositories map GORM/PG errors through `MapGormError()` to the
  sentinels in `internal/domain/errors.go` — never return raw GORM errors.
  Auth exception: `domain.AuthError` struct with `StatusCode`, `Message`,
  `Details`.
- **Soft delete**: only `User`, `Guild`, `Event` carry `gorm.DeletedAt` (use
  `Unscoped()` for a hard delete); the other four models hard-delete.
- `/v1/practices` routes use the `Event` model (backward-compat naming).
- `Transcript.Accent` is `datatypes.JSON` / `jsonb` — the only JSONB column.
- `GuildAttendeeRepository` lives in `gorm_guild.go`, not its own file.
- Imports: stdlib → third-party → local.

## Deployment

Two envs on one host via `deploy/compose.yml` — prod (`api.sessatakuma.dev`,
`:stable` on `v*.*.*` tags) and dev (`api-dev.sessatakuma.dev`, `:dev` on every
main merge). Env split, watchtower cadences, and multi-arch image rules are
documented in `README.md` → "Deployment / CD stack"; the deploy machine is
arm64 (the one we currently deploy on, may change), so images must carry a
`linux/arm64` manifest. Host-only files (gitignored,
each with a tracked `.example`): `deploy/.env`, `deploy/env/{prod,dev}`,
`deploy/cloudflared/creds/`. Read `deploy/README.md` before touching the host.

```bash
make -C deploy up          # both envs + cloudflared + both watchtowers
make -C deploy up-prod     # or up-dev / up-infra; also down, pull-prod / pull-dev
```

## CI

PR checks run through `sessatakuma/org-workflows`: `go mod tidy`, golangci-lint
(`.golangci.yml`), tests with `-race`, build, plus yamllint (default rules —
80-col lines fail; `docs/swagger/` is excluded via `.yamllint`), jq, and
taplo. `docker-build.yml` separately builds the image and smoke-tests it
offline.

## Git

Conventional Commits: header ≤ 72 chars, lowercase imperative summary —
`feat(api): add JWT authentication middleware`. Types: feat, fix, docs, style,
refactor, perf, test, build, ci, chore, revert, hotfix; `!` plus a
`BREAKING CHANGE:` footer for breaking changes; issue refs in the footer. PR
titles follow the same format (≤ 75 chars) and the body uses
`.github/PULL_REQUEST_TEMPLATE.md`.

## Gotchas

1. **`make air` needs `go` on the `/bin/sh` PATH.** The Makefile shell does
   not source your zshrc, so `which go` can work while `make air` reports
   `go: not found`. Use `PATH="/usr/local/go/bin:$PATH" make air`.
2. **Debug mode is wide open.** It skips both `/v1` middlewares, so any
   non-localhost debug build needs an edge gateway in front of it.
3. **`CLIENT_API_KEY` is inbound-only.** It guards the proxy routes in release
   mode; the internal backend → api-tools call is keyless either way.
4. **Old api-tools images assert on `YAHOO_API_KEY` at import.** Pin a
   local-unidic image to avoid that.
5. The repo-root `compose.yml` (local Postgres) and `deploy/compose.yml` (the
   full stack) are two unrelated stacks that share a filename.
6. `WEBRTC_CONN_SEC` / `WEBRTC_CONN_MAX` only apply to `cmd/webrtc-demo`; the
   main server's WebRTC rate limit is hardcoded:
   `NewRateLimiter(10*time.Second, 15)` in `internal/api/api.go:56`.
