# jpcorrect-backend
This repository contains the backend for the jpcorrect system, a Japanese language correction platform.

## Getting Started

### Prerequisites
- Go 1.25+
- PostgreSQL

### Installation
```bash
git clone https://github.com/sessatakuma/jpcorrect-backend.git
cd jpcorrect-backend
go mod download
```

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
| `API_TOOLS_KEY` | API key forwarded as `X-API-KEY` header to API tools service |
| `CLOUDFLARE_TUNNEL_TOKEN` | Cloudflare Tunnel token for the optional `cloudflared` compose profile |
| `JWKS_URL` | JWKS endpoint for JWT verification |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins (empty = allow all in debug mode) |
| `GIN_MODE` | `debug` or `release` |
| `API_CERT_PATH` | TLS certificate path (optional; enables HTTPS if both cert and key exist) |
| `API_KEY_PATH` | TLS key path (optional) |
| `YAHOO_API_KEY` | Yahoo API key for the API-tools service |
| `API_TOOLS_ALLOW_ORIGINS` | CORS origins for the API-tools service (default `*`) |
| `API_TOOLS_ALLOWED_HOSTS` | Trusted hosts for the API-tools service (default `*`) |

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

This repository now uses a **single Docker Compose file**:

- `compose.deploy.yml` for deployment infrastructure and containerized services

Local development is handled through the `Makefile` instead of a separate all-in-one compose stack.

### Local development

Copy the local development environment file first:

```bash
cp .env.example .env
```

Useful local targets:

```bash
# Start only PostgreSQL in Docker (bound to 127.0.0.1:5432)
make db-up

# Follow PostgreSQL logs
make db-logs

# Stop PostgreSQL
make db-down

# Run API-tools locally with uv on 127.0.0.1:8000
make api-tools

# Run backend locally with air
make air
```

In this workflow:

- backend runs on your host machine
- `api-tools` runs on your host machine through `uv run`
- PostgreSQL runs in Docker via `compose.deploy.yml`

The local `.env.example` is configured for this workflow with host-reachable values such as:

```text
DATABASE_URL=postgres://...@127.0.0.1:5432/...
API_TOOLS_URL=http://127.0.0.1:8000
```

### Deployment / CD stack

For deployment, use a dedicated deploy env file:

```bash
cp .env.deploy.example .env.deploy
BACKEND_ENV_FILE=.env.deploy docker compose -f compose.deploy.yml --env-file .env.deploy up -d
```

For this mode, `.env.deploy` should use deployment-ready values such as:

```text
DATABASE_URL=postgres://...@postgres:5432/...
API_TOOLS_URL=http://host.docker.internal:8000
```

This stack is intended for CD / production-style deploys:

- `backend` runs from `BACKEND_IMAGE` (for example a GHCR image)
- `postgres` runs in Docker and is bound only to `127.0.0.1`
- `cloudflared` forwards traffic to `http://backend:8080`
- `backend` reaches host-run `api-tools` through `http://host.docker.internal:8000`

This keeps a single deployment compose file while allowing `api-tools` to have an independent lifecycle.

### Cloudflare Tunnel

`cloudflared` is part of `compose.deploy.yml`.

Configure the Cloudflare public hostname to forward to:

```text
http://backend:8080
```

> `cloudflared` runs inside Docker, so do **not** use `localhost:8080` as the Cloudflare service URL. Inside the tunnel container, `localhost` points to itself, not the `backend` service.

### Environment files

- `.env.example`: local development defaults for `make db-up`, `make api-tools`, and `make air`
- `.env.deploy.example`: deployment defaults for `compose.deploy.yml`
- `.env`: your local development environment file (gitignored)
- `.env.deploy`: your deployment environment file (gitignored)
- `API-tools/.env.example`: standalone `api-tools` local runtime example

### Why this layout

This setup keeps Docker focused on deployment concerns while local development stays fast and simple. It also keeps environment values explicit per runtime, which avoids accidentally reusing host-only settings inside containers.