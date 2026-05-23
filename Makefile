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

.PHONY: air swag db-up db-down db-logs api-tools \
        deploy-up deploy-down \
        deploy-up-prod deploy-down-prod deploy-pull-prod \
        deploy-up-dev deploy-down-dev deploy-pull-dev \
        deploy-up-infra deploy-down-infra

POSTGRES_PORT ?= 5432
API_TOOLS_PORT ?= 8000

air:
	go tool air --build.cmd "go build -o ./tmp/main$(BIN_EXT) ./cmd/jpcorrect/main.go" --build.entrypoint "./tmp/main$(BIN_EXT)"

swag:
	go tool swag init -g cmd/jpcorrect/main.go -o docs/swagger --parseDependency --parseInternal

# Local-dev Postgres — single container bound to 127.0.0.1 so `make air` can connect.
# Independent volume from deploy postgres (postgres_local_data).
db-up:
	docker compose -f compose.local.yml up -d

db-down:
	docker compose -f compose.local.yml stop

db-logs:
	docker compose -f compose.local.yml logs -f

api-tools:
	cd API-tools && uv run uvicorn main:app --host 127.0.0.1 --port $(API_TOOLS_PORT)

# Deploy stack — two backends (prod + dev), two postgreses (one per env), shared
# cloudflared + watchtower. See compose.deploy.yml for the network split.

# Bring up everything (both envs + shared infra).
deploy-up: deploy-up-prod deploy-up-dev deploy-up-infra

deploy-down:
	docker compose -f compose.deploy.yml down

# Prod env: api.sessatakuma.dev — full auth (CLIENT_API_KEY + JWT), Swagger hidden.
deploy-up-prod:
	docker compose -f compose.deploy.yml up -d postgres-prod backend-prod

deploy-down-prod:
	docker compose -f compose.deploy.yml stop postgres-prod backend-prod

# Pull a newer :stable tag immediately (Watchtower normally handles this every 5 min).
deploy-pull-prod:
	docker compose -f compose.deploy.yml pull backend-prod
	docker compose -f compose.deploy.yml up -d backend-prod

# Dev env: api-dev.sessatakuma.dev — Cloudflare Access at edge, app middlewares
# skipped (GIN_MODE=debug), Swagger visible.
deploy-up-dev:
	docker compose -f compose.deploy.yml up -d postgres-dev backend-dev

deploy-down-dev:
	docker compose -f compose.deploy.yml stop postgres-dev backend-dev

deploy-pull-dev:
	docker compose -f compose.deploy.yml pull backend-dev
	docker compose -f compose.deploy.yml up -d backend-dev

# Shared infra: cloudflared (tunnel for both hostnames) + watchtower (auto-pull).
deploy-up-infra:
	docker compose -f compose.deploy.yml up -d cloudflared watchtower

deploy-down-infra:
	docker compose -f compose.deploy.yml stop cloudflared watchtower
