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
Create `.env` for configuration (in project root):
```ini
PORT=8080
DATABASE_URL=postgresql://[user[:password]@][netloc][:port][/dbname][?param1=value1&...]
API_TOOLS_URL=your_api_tools_url
GIN_MODE=debug
```

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
# Start all services (postgres + backend + optional pgadmin)
docker-compose up -d

# Build and start backend only
docker-compose up backend

# Stop all services
docker-compose down
```

### Access Services
- **Backend API**: http://localhost:8080
- **PostgreSQL**: localhost:5432
  - User: `jpcorrect`
  - Password: `jpcorrect_password`
  - Database: `jpcorrect`
- **pgAdmin**: http://localhost:5050 (if adminer enabled)
  - User: `admin`
  - Password: `admin` (see docker-compose.yml for admin credentials)

### Docker Environment Variables
You can override environment variables in docker-compose.yml:

```bash
# Override default DATABASE_URL for Supabase
docker-compose up backend -e DATABASE_URL="postgres://[PROJECT-REF]:[PASSWORD]@aws-0-[REGION].pooler.supabase.com:5432/jpcorrect?sslmode=require"

# Override API_TOOLS_URL
docker-compose up backend -e API_TOOLS_URL="your_custom_url"
```

### Hot Reload
The docker-compose.yml includes volume mounting for hot code reload in development.

### Environment Variables
Create `.env` for configuration (in project root).
```ini
DATABASE_URL=postgresql://[user[:password]@][netloc][:port][/dbname][?param1=value1&...]
API_TOOLS_URL=your_api_tools_url
GIN_MODE=debug
JWKS_URL=your_jwks_url
```

### Run
```bash
go run cmd/jpcorrect/main.go
```

### Development
Run with [air](https://github.com/air-verse/air) for live reloading:
```bash
make air
```
Generate repository codes using [sqlc](https://github.com/sqlc-dev/sqlc):
```bash
make sqlc
```
Create migration files using [migrate](https://github.com/golang-migrate/migrate):
```bash
make migrate-create name=<your_migration_name>
```
Migrate database up or down:
```bash
make migrate-up
make migrate-down
```