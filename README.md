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

See `AGENTS.md` → "API-tools compatibility" for the api-tools version this backend's proxy is aligned to.

### Environment Variables
A `.env` file is required in the project root for local development. Copy and configure it:
```bash
cp .env.example .env
```

Variables:
| Variable | Description |
| --- | --- |
| `PORT` | Server port (default `8080`) |
| `DATABASE_URL` | PostgreSQL connection string |
| `API_TOOLS_URL` | API tools service URL |
| `CLIENT_API_KEY` | Inbound `X-API-Key` required by the 7 api-tools proxy endpoints (JWT not accepted; empty value returns 401) |
| `CLOUDFLARE_TUNNEL_TOKEN` | Cloudflare Tunnel token for the optional `cloudflared` compose profile |
| `JWKS_URL` | JWKS endpoint for JWT verification |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins (empty = allow all in debug mode) |
| `GIN_MODE` | `debug` or `release` |
| `API_CERT_PATH` | TLS certificate path (optional; enables HTTPS if both cert and key exist) |
| `API_KEY_PATH` | TLS key path (optional) |
| `YAHOO_API_KEY` | Yahoo API key for the API-tools service |

> **Local development note:** `.env.example` is for host-run development. When the backend runs on your machine, `DATABASE_URL` should use `127.0.0.1` and `API_TOOLS_URL` should point at your local `api-tools` process.

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
cd API-tools && YAHOO_API_KEY="$YAHOO_API_KEY" uv run uvicorn main:app --host 127.0.0.1 --port 8000

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
  middlewares skipped (gated at the edge by Cloudflare Access), image `:latest`
  (every main merge).

```bash
cp deploy/env/prod.example deploy/env/prod   # fill in prod CLIENT_API_KEY, JWKS_URL, etc.
cp deploy/env/dev.example  deploy/env/dev    # dev: leave CLIENT_API_KEY / JWKS_URL empty

make -C deploy up     # both envs + cloudflared + watchtower
make -C deploy down   # everything
```

Each env has its own Postgres on an isolated bridge network, and both backends
reach the sibling `jpcorrect-api-tools` container over the external
`jpcorrect-shared` network. A `watchtower` container polls GHCR every 5 min and
auto-restarts whichever backend image changed.

See **AGENTS.md → "Deployment stack (two environments on one host)"** for the
full network table, one-time host setup, and the `deploy/` file layout.

### Cloudflare Tunnel

`cloudflared` is part of `deploy/compose.yml`; ingress is defined in
`deploy/cloudflared/config.yml` and credentials live in
`deploy/cloudflared/creds/` (gitignored).

Configure the Cloudflare public hostname to forward to:

```text
http://backend:8080
```

> `cloudflared` runs inside Docker, so do **not** use `localhost:8080` as the Cloudflare service URL. Inside the tunnel container, `localhost` points to itself, not the `backend` service.

### Environment files

- `.env.example`: local development defaults for `docker compose up -d`, `uv run uvicorn ...`, and `make air`
- `deploy/env/prod.example` / `deploy/env/dev.example`: deploy defaults for `backend-prod` / `backend-dev`
- `.env`: your local development environment file (gitignored)
- `deploy/env/prod` / `deploy/env/dev`: your per-env deploy files (gitignored)
- `API-tools/.env.example`: standalone `api-tools` local runtime example

### Why this layout

This setup keeps Docker focused on deployment concerns while local development stays fast and simple. It also keeps environment values explicit per runtime, which avoids accidentally reusing host-only settings inside containers.