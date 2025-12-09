# Load environment variables from .env file if it exists
ifneq (,$(wildcard .env))
    include .env
    export
endif

# Determine binary extension based on OS
BIN_EXT =
ifeq ($(OS),Windows_NT)
    BIN_EXT = .exe
endif

# Convert DATABASE_URL to PGX format
PGX_URL=$(subst postgres://,pgx5://,$(DATABASE_URL))

.PHONY: air sqlc migrate-create migrate-up migrate-down

air:
	go tool air --build.cmd "go build -o ./tmp/main$(BIN_EXT) ./cmd/jpcorrect/main.go" --build.entrypoint "./tmp/main$(BIN_EXT)"

sqlc:
	go tool sqlc generate

migrate-create:
	go tool migrate create -ext sql -dir db/migrations -seq "$(name)"

migrate-up:
	go tool migrate -path db/migrations -database "$(PGX_URL)" up

migrate-down:
	go tool migrate -path db/migrations -database "$(PGX_URL)" down