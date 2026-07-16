# Deploy operations

Operational notes for this host. For architecture, networks, and the deploy stack
design see [`../AGENTS.md`](../AGENTS.md) → *Deployment stack (two environments
on one host)*. This file only documents day-to-day ops gotchas that you can't
read off the `compose.yml`.

## Day-to-day commands

All deploy targets live in [`./Makefile`](./Makefile). Run from this directory
or with `make -C deploy <target>`. `docker compose` auto-detects `./compose.yml`
so no `-f` is needed.

```bash
make -C deploy up             # both envs + cloudflared + watchtower-prod + watchtower-dev
make -C deploy up-prod        # just prod (postgres-prod + backend-prod + api-tools-prod)
make -C deploy up-dev         # just dev  (postgres-dev  + backend-dev  + api-tools-dev)
make -C deploy up-infra       # just cloudflared + watchtower-prod + watchtower-dev
make -C deploy pull-prod      # manual pull of :stable for backend-prod + api-tools-prod
make -C deploy pull-dev       # manual pull of :dev    for backend-dev  + api-tools-dev
make -C deploy down           # everything
```

## Changing env vars (env/prod, env/dev) — recreate, don't restart

> **TL;DR**: edit `env/prod` or `env/dev` → **recreate** the container, not
> restart. `docker restart` will **not** pick up the new values.

Docker loads `env_file` exactly once, at container **create** time. After that
the values live inside the container's metadata; `docker restart` only kills and
re-runs the same PID 1 with the same baked-in env. So this sequence silently
keeps the old value:

```bash
vim deploy/env/prod          # edit CLIENT_API_KEY
docker restart jpcorrect-backend-prod   # ❌ container still has the old key
```

The symptom is usually a hard-to-debug auth/secret mismatch: clients send the
new value, the server compares against the stale one in memory, every request
401s. To actually reload `env_file`, recreate the container:

```bash
# After editing deploy/env/prod
cd deploy
docker compose up -d --force-recreate backend-prod

# After editing deploy/env/dev
cd deploy
docker compose up -d --force-recreate backend-dev
```

`--force-recreate` keeps the same image/network/volume — it just rebuilds the
container metadata, which means `env_file` is re-read. Postgres data persists
in the named volume, so DBs are untouched.

### Image-tag override when :dev / :stable doesn't exist yet

`backend-dev` and `api-tools-dev` default to `:dev`; `backend-prod` and
`api-tools-prod` default to `:stable`. Until those tags exist on GHCR (first
main merge / first `v*.*.*` tag), a plain `docker compose up --force-recreate`
will fail with `image not found`. Override the tag inline for the recreate:

```bash
BACKEND_DEV_IMAGE=ghcr.io/sessatakuma/jpcorrect-backend:test \
  docker compose up -d --force-recreate backend-dev
```

Same shape for `BACKEND_PROD_IMAGE`, `API_TOOLS_PROD_IMAGE`, and `API_TOOLS_DEV_IMAGE`.

## Cloudflare tunnel DNS routes

DNS routes (CNAME → tunnel) are registered once per hostname and persist on
Cloudflare's side — they survive container restarts and re-deployments.
You only re-register if a route was deleted or never created:

```bash
# From this directory:
docker exec jpcorrect-cloudflared cloudflared tunnel route dns jb api.sessatakuma.dev
docker exec jpcorrect-cloudflared cloudflared tunnel route dns jb api-dev.sessatakuma.dev
```

Add `--overwrite-dns` if the hostname currently has a non-tunnel A/AAAA/CNAME
record you want to replace (e.g. cutting over from a legacy Heroku CNAME). This
is destructive — traffic for that hostname **immediately** starts hitting the
tunnel.

## Watchtower: only updates the **same** image:tag, never switches tags

Watchtower pulls a fresher digest **for the same image reference** the
container is currently running. It does **not** switch tags. So:

- `backend-prod` / `api-tools-prod` running `:stable` → new `:stable` digest auto-updates. ✓
- `backend-dev`  / `api-tools-dev`  running `:dev`    → new `:dev` digest auto-updates. ✓
- `backend-dev` running `:test` (one-off manual dispatch) → only updates if
  `:test` itself is re-published. To go back to tracking `:dev`, you must
  manually recreate with the default tag (see above).

### Per-env cadence (scope split)

There are two Watchtower instances, separated by `WATCHTOWER_SCOPE`:

- `watchtower-prod` (`scope=prod`) — `WATCHTOWER_SCHEDULE="0 0 3 * * *"` with
  `TZ=Asia/Taipei`, so it wakes up once a day at 03:00 Taipei and pulls fresh
  `:stable` digests for `backend-prod` + `api-tools-prod`. Off-hours window
  means a bad `:stable` doesn't take prod down during the day.
- `watchtower-dev` (`scope=dev`) — `WATCHTOWER_POLL_INTERVAL=300`, polls every
  5 minutes and pulls fresh `:dev` digests for `backend-dev` + `api-tools-dev`.

Each watchtower only touches containers carrying the matching
`com.centurylinklabs.watchtower.scope=<env>` label, so the two instances are
truly independent — dev rolling forward never triggers a prod restart and
vice versa.
