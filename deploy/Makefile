# Deploy stack — two backends (prod + dev), two postgreses (one per env), shared
# cloudflared + watchtower. Run from this directory (`cd deploy && make up`, or
# `make -C deploy up`); docker compose auto-detects ./compose.yml so no -f is
# needed. See compose.yml for the network split.

.PHONY: up down \
        up-prod down-prod pull-prod \
        up-dev down-dev pull-dev \
        up-infra down-infra

# Bring up everything (both envs + shared infra).
up: up-prod up-dev up-infra

down:
	docker compose down

# Prod env: api.sessatakuma.dev — full auth (CLIENT_API_KEY + JWT), Swagger hidden.
up-prod:
	docker compose up -d postgres-prod backend-prod

down-prod:
	docker compose stop postgres-prod backend-prod

# Pull a newer :stable tag immediately (Watchtower normally handles this every 5 min).
pull-prod:
	docker compose pull backend-prod
	docker compose up -d backend-prod

# Dev env: api-dev.sessatakuma.dev — Cloudflare Access at edge, app middlewares
# skipped (GIN_MODE=debug), Swagger visible.
up-dev:
	docker compose up -d postgres-dev backend-dev

down-dev:
	docker compose stop postgres-dev backend-dev

pull-dev:
	docker compose pull backend-dev
	docker compose up -d backend-dev

# Shared infra: cloudflared (tunnel for both hostnames) + watchtower (auto-pull).
up-infra:
	docker compose up -d cloudflared watchtower

down-infra:
	docker compose stop cloudflared watchtower
