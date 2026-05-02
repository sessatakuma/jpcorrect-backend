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
A `.env` file is required in the project root. Copy and configure it:
```bash
cp .env.example .env
```

Variables:
| Variable | Description |
| --- | --- |
| `PORT` | Server port (default `8080`) |
| `DATABASE_URL` | PostgreSQL connection string |
| `API_TOOLS_URL` | External API tools service URL |
| `JWKS_URL` | JWKS endpoint for JWT verification |
| `ALLOWED_ORIGINS` | Comma-separated CORS origins (empty = allow all in debug mode) |
| `GIN_MODE` | `debug` or `release` |
| `API_CERT_PATH` | TLS certificate path (optional; enables HTTPS if both cert and key exist) |
| `API_KEY_PATH` | TLS key path (optional) |

> **Docker note:** `DATABASE_URL` must use the Docker hostname `postgres` instead of `localhost` when running via `docker compose`. See `.env` for the default value.

### Run
```bash
go run cmd/jpcorrect/main.go
```

### Development
Run with [air](https://github.com/air-verse/air) for live reloading:
```bash
make air
```

## Docker Support

### Quick Start
```bash
# Start all services (postgres + backend)
docker compose up -d

# Build and start (use after code changes)
docker compose up --build

# Stop all services
docker compose down
```

> **Note:** The Docker setup uses a multi-stage build. You must run `docker compose up --build` to pick up code changes — there is no hot reload inside the container. For live reloading during development, use `make air` locally instead.

### Access Services
- **Backend API**: http://localhost:8080
- **Swagger UI**: http://localhost:8080/swagger/index.html
- **PostgreSQL**: localhost:5432
  - User: `jpcorrect`
  - Password: `jpcorrect_password`
  - Database: `jpcorrect`

### Docker Environment Variables
The backend service reads `.env` via `env_file` in `docker-compose.yml`, so all variables are automatically injected into the container. Make sure `DATABASE_URL` in `.env` uses the Docker hostname `postgres` (not `localhost`).
