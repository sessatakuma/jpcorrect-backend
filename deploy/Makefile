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
	docker compose up -d postgres-prod backend-prod api-tools-prod

down-prod:
	docker compose stop postgres-prod backend-prod api-tools-prod

# Pull newer :stable tags immediately (Watchtower normally handles this on the
# nightly 03:00 Asia/Taipei cron).
pull-prod:
	docker compose pull backend-prod api-tools-prod
	docker compose up -d backend-prod api-tools-prod

# Dev env: api-dev.sessatakuma.dev — Cloudflare Access at edge, app middlewares
# skipped (GIN_MODE=debug), Swagger visible.
up-dev:
	docker compose up -d postgres-dev backend-dev api-tools-dev

down-dev:
	docker compose stop postgres-dev backend-dev api-tools-dev

# Pull newer :dev tags immediately (Watchtower normally handles this every 5 min).
pull-dev:
	docker compose pull backend-dev api-tools-dev
	docker compose up -d backend-dev api-tools-dev

# Shared infra: cloudflared (tunnel for both hostnames) + the two scoped
# watchtowers (watchtower-prod on a nightly cron, watchtower-dev on a 5-min poll).
up-infra:
	docker compose up -d cloudflared watchtower-prod watchtower-dev

down-infra:
	docker compose stop cloudflared watchtower-prod watchtower-dev
