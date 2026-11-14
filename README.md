# jpcorrect-backend
This repository contains the backend for the jpcorrect system, a Japanese language correction platform.

## Getting Started

### Prerequisites
- Go 1.25+
- PostgreSQL

### Installation
Backend and api-tools live in separate repos — clone both as siblings:
```bash
git clone https://github.com/sessatakuma/jpcorrect-backend.git
git clone https://github.com/sessatakuma/API-tools.git    # sibling, not a submodule
cd jpcorrect-backend
go mod download
```

The backend talks to `api-tools` over HTTP only (`API_TOOLS_URL`) — in dev it runs on the host via `uv`, in deployment it is pulled as a container.

### Environment Variables

A `.env` file is required in the project root for local development. It is
auto-loaded via `github.com/joho/godotenv/autoload`. Copy and configure it:
```bash
cp .env.example .env
```

| Variable | Description |
| --- | --- |
| `DATABASE_URL` | **Required.** PostgreSQL connection string. `127.0.0.1:5432` locally; `postgres-prod:5432` / `postgres-dev:5432` in the deploy stack |
| `JWKS_URL` | **Required in every mode** — the app fatals on an empty or unreachable value. Authentication runs on user routes in debug as well as release |
| `GIN_MODE` | `debug` exposes Swagger UI and **skips APIKeyMiddleware on the api-tools `/v1` routes only**. JWT authentication runs on every user route in every mode |
| `API_TOOLS_URL` | API-tools service URL. `http://127.0.0.1:8000` locally; `http://jpcorrect-api-tools-{prod,dev}:8000` in the stack. The backend → api-tools call needs no key |
| `CLIENT_API_KEY` | Inbound `X-API-Key` for the 7 api-tools proxy routes, release mode only (JWT not accepted). Empty ⇒ those routes always 401 |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins. Empty = reject all in release, allow all in debug |
| `PORT` | Server port (default `8080`) |
| `API_CERT_PATH` / `API_KEY_PATH` | TLS certificate/key paths (default `./certs/{cert,key}.pem`); HTTPS only when both exist, otherwise HTTP with a warning |
| `WEBRTC_CONN_SEC` / `WEBRTC_CONN_MAX` | WebRTC rate-limit window / max connections for `cmd/webrtc-demo` (`10` / `15`) |
| `WEBRTC_DEMO_PORT` / `WEBRTC_DEMO_BASE_DIR` / `WEBRTC_DEMO_CERT_PATH` / `WEBRTC_DEMO_KEY_PATH` | `cmd/webrtc-demo` only |

> **Local development note:** `.env.example` is for host-run development. When the backend runs on your machine, `DATABASE_URL` should use `127.0.0.1` and `API_TOOLS_URL` should point at your local `api-tools` process.

Compose-level variables (never seen by the Go process): `POSTGRES_PORT` for the local stack; `POSTGRES_{PROD,DEV}_PASSWORD`, `{BACKEND,API_TOOLS}_{PROD,DEV}_IMAGE` and `HOME` for the deploy stack. The api-tools containers need no env vars at all.

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

## Testing

Without a database, the full suite runs with both connection variables
cleared:

```bash
env -u DATABASE_URL -u TEST_DATABASE_URL go test ./...
```

Everything that needs PostgreSQL then skips itself; see below.

### Integration tests

The repository integration tests (`internal/repository`,
`internal/testutil`) run against a real PostgreSQL server. They connect with
`TEST_DATABASE_URL` and nothing else.

| Variable | Used by | Purpose |
| --- | --- | --- |
| `DATABASE_URL` | the `cmd/jpcorrect` server | database the API reads and writes |
| `DATABASE_URL` (in tests) | `internal/database` connection smoke test | existing test, not migrated; skips when unset |
| `TEST_DATABASE_URL` | repository integration tests | throwaway database the tests may create and drop schemas in |

The no-fallback rule applies to the repository integration tests only: the
older `internal/database` connection smoke test (`TestNewGormDB`) still reads
`DATABASE_URL` and skips when it is unset. Point `TEST_DATABASE_URL` at a
dedicated throwaway database whose role may `CREATE SCHEMA` and `DROP SCHEMA`,
never at the production or deploy database. When it is unset, `go test` still
passes: every repository integration test skips itself instead of guessing a
connection string.

```bash
export TEST_DATABASE_URL='postgres://user:pass@127.0.0.1:5432/jpcorrect_test?sslmode=disable'
go test -race -count=1 -shuffle=on -v ./internal/repository/...
```

Each test gets a randomly named schema migrated with the same eight models as
the production `AutoMigrate` list in `internal/cmd/api.go`, and the search path
is pinned in the DSN so every pooled connection resolves unqualified names in
that schema. Cleanup drops only the schema this test created. Keep
`testutil.BaselineModels` in sync with the production model list.

CI runs the same command in the `guild-concurrency-test` job, which starts a
PostgreSQL service and always sets `TEST_DATABASE_URL`; with the database
reachable no repository test is skipped.

An optional local PostgreSQL, if you do not already have one (the root
`compose.yml` already provides one on `127.0.0.1:5432`; do not modify it for
testing):

```bash
docker run --rm -d --name jpcorrect-test-pg \
  -e POSTGRES_USER=test -e POSTGRES_PASSWORD=test -e POSTGRES_DB=test \
  -p 127.0.0.1:5433:5432 postgres:15-alpine
```

Use the port you published (`5433` above) in `TEST_DATABASE_URL`; the
loopback binding keeps the throwaway database off non-loopback interfaces.

Scope of the current schema change (#69): the two `guild_attendee` foreign keys
to `guild` and `user` are the only constraints added. This is not the complete
#72 work, and the other six foreign keys are out of scope here. Applying it to
a database that already holds orphan rows fails when the foreign key is
created; nothing deletes or repairs data automatically, so those rows must be
fixed first.

## Local Development and Deployment

This repository uses two Docker Compose files, each named `compose.yml` (docker's
default) within its own context:

- `compose.yml` (repo root) — local-dev Postgres only (`docker compose up -d`); the backend
  and `api-tools` run on the host
- `deploy/compose.yml` — the two-env deployment stack (backend-prod + backend-dev
  + cloudflared + watchtower)

Local development is driven through the `Makefile` rather than an all-in-one stack.

### Local development

Copy the local development environment file first:

```bash
cp .env.example .env
```

Useful local commands:

```bash
# Start only PostgreSQL in Docker (root compose.yml, bound to 127.0.0.1:5432)
docker compose up -d

# Follow PostgreSQL logs / stop it
docker compose logs -f
docker compose stop

# Run API-tools locally with uv on 127.0.0.1:8000 (clone the sibling repo first)
cd ../API-tools && uv run uvicorn main:app --host 127.0.0.1 --port 8000

# Run backend locally with air
make air
```

In this workflow:

- backend runs on your host machine
- `api-tools` runs on your host machine through `uv run`
- PostgreSQL runs in Docker via `compose.yml`

`GIN_MODE` no longer changes authentication on user routes: every `/v1` user
route (including `POST /v1/users/init`, which only skips the database lookup)
requires a real Supabase JWT in debug and in release. Local development
therefore needs real Supabase accounts and tokens, and `JWKS_URL` must point at
the project's JWKS document. There is no `X-Debug-User-ID` header and no fake
caller: use the Swagger UI "Authorize" button with a real token, or send it
yourself.

```bash
# 1. Sign in through Supabase Auth and obtain an access token
# 2. Create the internal account on first login (idempotent)
curl -X POST http://127.0.0.1:8080/v1/users/init \
  -H "Authorization: Bearer $ACCESS_TOKEN"

# 3. Use the resolved profile for subsequent calls
curl http://127.0.0.1:8080/v1/users/me \
  -H "Authorization: Bearer $ACCESS_TOKEN"

curl -X POST http://127.0.0.1:8080/v1/guilds \
  -H 'Content-Type: application/json' \
  -H "Authorization: Bearer $ACCESS_TOKEN" \
  -d '{"name":"Local guild"}'
```

A valid token for a user who never called `users/init` answers 401
`USER_NOT_INITIALIZED`; a missing or invalid token answers 401
`UNAUTHORIZED`. Debug mode only skips the `X-API-Key` check of the api-tools
proxy routes.

The local `.env.example` is configured for this workflow with host-reachable values such as:

```text
DATABASE_URL=postgres://...@127.0.0.1:5432/...
API_TOOLS_URL=http://127.0.0.1:8000
```

### Deployment / CD stack

All deploy files live under `deploy/`. The stack (`deploy/compose.yml`)
runs **two backend instances** on one host behind a single Cloudflare Tunnel:

- **prod** (`backend-prod` → `api.sessatakuma.dev`) — `GIN_MODE=release`, full
  auth, image `:stable` (pushed on `v*.*.*` git tags).
- **dev** (`backend-dev` → `api-dev.sessatakuma.dev`) — `GIN_MODE=debug`, same
  JWT authentication as prod (only the api-tools `X-API-Key` check and the
  Swagger UI differ) and gated at the edge by Cloudflare Access, image `:dev`
  (every main merge).

```bash
cp deploy/.env.example     deploy/.env      # POSTGRES_PROD_PASSWORD + POSTGRES_DEV_PASSWORD
cp deploy/env/prod.example deploy/env/prod   # fill in prod CLIENT_API_KEY, JWKS_URL, etc.
cp deploy/env/dev.example  deploy/env/dev    # dev: fill in JWKS_URL, leave CLIENT_API_KEY empty
# deploy/.env is the compose interpolation source (env/prod and env/dev are
# service env_files, which compose never reads during interpolation). Give each
# env a unique random password and mirror it, URL-encoded, into that env's
# DATABASE_URL before continuing.

make -C deploy up     # both envs + cloudflared + both watchtowers
make -C deploy down   # everything
```

Each env has its own Postgres **and its own api-tools** on an isolated bridge
network, so `backend-dev` cannot reach `postgres-prod` or `api-tools-prod` at
all. Each backend talks to its own instance over that env-net
(`http://jpcorrect-api-tools-prod:8000` / `...-dev:8000`); only the backends and
`cloudflared` sit on the shared `jpcorrect-shared` network.

Two `watchtower` containers keep the envs on separate update cadences, matched
by a `com.centurylinklabs.watchtower.scope=<env>` label:

- `watchtower-dev` polls GHCR every 5 min for the `:dev` images
- `watchtower-prod` runs once a day at 03:00 Asia/Taipei for the `:stable`
  images, so a bad release cannot take prod down mid-day

Each env's Postgres is reachable only from its own bridge network (never
published to the host), and `cloudflared` is the single ingress — no container
ports are exposed.

#### Image tags and multi-arch

The deploy machine — the machine we currently deploy on, which may change —
is **arm64**, so images must carry a `linux/arm64` manifest. Both repos' CD
builds `linux/amd64,linux/arm64`; `:stable` is only republished on a
`v*.*.*` tag and `:dev` on every main merge.

Before letting `watchtower-prod` pull a manually pinned tag, verify the
manifest carries arm64:

```bash
docker buildx imagetools inspect <ref> | grep Platform
```

See **`deploy/README.md`** for one-time host setup and day-to-day operations,
and `AGENTS.md` for agent-oriented conventions and gotchas.

### Cloudflare Tunnel

`cloudflared` is part of `deploy/compose.yml`; ingress is defined in
`deploy/cloudflared/config.yml` and credentials live in
`deploy/cloudflared/creds/` (gitignored).

Ingress is file-driven, so there is nothing to configure in the Cloudflare
dashboard beyond the DNS routes. `deploy/cloudflared/config.yml` maps the two
hostnames onto the two backends:

| Hostname | Upstream | Edge auth |
| --- | --- | --- |
| `api.sessatakuma.dev` | `http://backend-prod:8080` | none; the app enforces JWT / `X-API-Key` |
| `api-dev.sessatakuma.dev` | `http://backend-dev:8080` | Cloudflare Access Zero Trust policy |

Register the routes once with `cloudflared tunnel route dns` — see
**`deploy/README.md` → "One-time host setup"**.

> The upstream host is the Compose **service name**, and `cloudflared` runs
> inside Docker: do **not** use `localhost:8080`, which inside the tunnel
> container points at the tunnel itself rather than at a backend.

### Environment files

- `.env.example`: local development defaults for `docker compose up -d`, `uv run uvicorn ...`, and `make air`
- `deploy/env/prod.example` / `deploy/env/dev.example`: deploy defaults for `backend-prod` / `backend-dev`
- `.env`: your local development environment file (gitignored)
- `deploy/env/prod` / `deploy/env/dev`: your per-env deploy files (gitignored)
- `API-tools/.env.example`: standalone `api-tools` local runtime example

### Why this layout

This setup keeps Docker focused on deployment concerns while local development stays fast and simple. It also keeps environment values explicit per runtime, which avoids accidentally reusing host-only settings inside containers.
