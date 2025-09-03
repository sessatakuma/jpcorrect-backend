# jpcorrect-backend
This repository contains the backend for the jpcorrect system, a Japanese language correction platform.

## Getting Started

### Prerequisites
- Go 1.18+
- PostgreSQL

### Installation
```bash
git clone https://github.com/sessatakuma/jpcorrect-backend.git
cd jpcorrect-backend
go mod tidy
```

### Environment Variables
Create `.env` for configuration (in project root).
```ini
DATABASE_URL=postgresql://[user[:password]@][netloc][:port][/dbname][?param1=value1&...]
```

### Run
```bash
go run cmd/jpcorrect/main.go
```
