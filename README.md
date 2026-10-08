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
| `JWKS_URL` | **Required in release** — the app fatals on an empty value unless `GIN_MODE=debug` |
| `GIN_MODE` | `debug` **skips APIKeyMiddleware + AuthMiddleware on all `/v1` routes** and exposes Swagger UI. Only safe behind an edge gateway |
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
- **dev** (`backend-dev` → `api-dev.sessatakuma.dev`) — `GIN_MODE=debug`, app
  middlewares skipped (gated at the edge by Cloudflare Access), image `:dev`
  (every main merge).

```bash
cp deploy/.env.example     deploy/.env      # POSTGRES_PROD_PASSWORD + POSTGRES_DEV_PASSWORD
cp deploy/env/prod.example deploy/env/prod   # fill in prod CLIENT_API_KEY, JWKS_URL, etc.
cp deploy/env/dev.example  deploy/env/dev    # dev: leave CLIENT_API_KEY / JWKS_URL empty
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
