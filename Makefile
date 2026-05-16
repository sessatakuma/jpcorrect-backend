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

.PHONY: air swag db-up db-down db-logs api-tools deploy-up deploy-down

POSTGRES_PORT ?= 5432
API_TOOLS_PORT ?= 8000
API_TOOLS_ALLOW_ORIGINS ?= *
API_TOOLS_ALLOWED_HOSTS ?= *

air:
	go tool air --build.cmd "go build -o ./tmp/main$(BIN_EXT) ./cmd/jpcorrect/main.go" --build.entrypoint "./tmp/main$(BIN_EXT)"

swag:
	go tool swag init -g cmd/jpcorrect/main.go -o docs/swagger --parseDependency --parseInternal

db-up:
	docker compose -f compose.deploy.yml --env-file .env up -d postgres

db-down:
	docker compose -f compose.deploy.yml --env-file .env stop postgres

db-logs:
	docker compose -f compose.deploy.yml --env-file .env logs -f postgres

api-tools:
	cd API-tools && YAHOO_API_KEY="$(YAHOO_API_KEY)" ALLOW_ORIGINS="$(API_TOOLS_ALLOW_ORIGINS)" ALLOWED_HOSTS="$(API_TOOLS_ALLOWED_HOSTS)" X_API_KEY="$(API_TOOLS_KEY)" uv run uvicorn main:app --host 127.0.0.1 --port $(API_TOOLS_PORT)

deploy-up:
	BACKEND_ENV_FILE=.env.deploy docker compose -f compose.deploy.yml --env-file .env.deploy up -d

deploy-down:
	BACKEND_ENV_FILE=.env.deploy docker compose -f compose.deploy.yml --env-file .env.deploy down
