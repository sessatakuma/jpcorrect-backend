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
Create `.env` for configuration (in project root).
```ini
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