# Deploy operations

Operational notes for this host. This file documents
the one-time host setup plus the day-to-day ops gotchas you can't read off the
`compose.yml`.

## One-time host setup

```bash
# The two backends and cloudflared meet on an external shared network.
docker network create jpcorrect-shared 2>/dev/null || true

# Register a DNS route per hostname for the `jb` tunnel (creds must already be
# in ./cloudflared/creds/). Idempotent, and persists on Cloudflare's side.
docker run --rm -v $PWD/cloudflared/creds:/home/nonroot/.cloudflared \
  cloudflare/cloudflared:latest tunnel route dns jb api.sessatakuma.dev
docker run --rm -v $PWD/cloudflared/creds:/home/nonroot/.cloudflared \
  cloudflare/cloudflared:latest tunnel route dns jb api-dev.sessatakuma.dev
```

Then, in the Cloudflare Zero Trust dashboard (no IaC): **Access → Applications →
Add → Self-hosted**, domain `api-dev.sessatakuma.dev`, with a policy such as
*include emails ending in `@sessatakuma.dev`*. This is the **only** thing
protecting the dev backend, which runs with its app middlewares skipped. Do
**not** add an Access app for `api.sessatakuma.dev` — prod stays publicly
reachable and enforces auth in the app.

If a standalone `jpcorrect-api-tools` container from the old
`../API-tools/docker-compose.yml`, or from the pre-split version of this stack,
is still running, remove it first so it can't hoard the name or port:
`docker rm -f jpcorrect-api-tools`.

Finally, copy the env templates (`deploy/.env.example` → `deploy/.env`,
`deploy/env/{prod,dev}.example` → `deploy/env/{prod,dev}`) and log in to GHCR —
both covered below.

## Registry auth (do this first on a new host)

Both GHCR packages (`jpcorrect-backend`, `api-tools`) are **private**, so a host
with no credentials cannot pull anything:

```console
$ docker pull ghcr.io/sessatakuma/jpcorrect-backend:dev
Error response from daemon: error from registry: unauthorized
```

Log in once with a PAT that has `read:packages`:

```bash
echo "$GHCR_PAT" | docker login ghcr.io -u <github-username> --password-stdin
```

This also creates `~/.docker/config.json`, which both watchtower services
bind-mount as `/config.json` to reuse the same credentials. **The file has to
exist before `make -C deploy up-infra`** — Docker would otherwise create a
*directory* at that path and watchtower would silently find no credentials.

## Postgres passwords (`deploy/.env`)

Each env's Postgres password is a Compose **interpolation** variable, so it has
to come from `deploy/.env` — the file Compose auto-loads from the directory
holding `compose.yml`:

```bash
cp deploy/.env.example deploy/.env    # then fill in both values
```

`env/prod` and `env/dev` cannot hold these. They are service `env_file`s, which
Docker hands to the container at runtime and never consults during
interpolation, so a password placed there leaves `${POSTGRES_PROD_PASSWORD}`
unset and every `docker compose` command aborts before creating a service:

```console
$ make -C deploy up-prod
error while interpolating services.postgres-prod.environment.POSTGRES_PASSWORD:
required variable POSTGRES_PROD_PASSWORD is missing a value: set in deploy/.env
```

The password appears twice per env and both copies must agree:

| Where | Variable | Consumed by |
| --- | --- | --- |
| `deploy/.env` | `POSTGRES_PROD_PASSWORD` | `postgres-prod` at `initdb` |
| `deploy/env/prod` | inside `DATABASE_URL` | the Go backend at connect time |
| `deploy/.env` | `POSTGRES_DEV_PASSWORD` | `postgres-dev` at `initdb` |
| `deploy/env/dev` | inside `DATABASE_URL` | the Go backend at connect time |

Prod and dev get **different** values: `backend-dev` runs with the app
middlewares skipped, so anything that reaches it must not hold credentials that
also work against the prod database.

> **Rotation is not automatic.** `POSTGRES_*_PASSWORD` is read by the official
> image only when it initialises an empty `PGDATA`. Editing it later changes
> nothing inside an existing volume — the container comes back up with the old
> password and the backend starts failing auth. To actually rotate, change it
> in-place first, then update both files:
>
> ```bash
> docker exec -it jpcorrect-postgres-prod \
>   psql -U jpcorrect -c "ALTER USER jpcorrect PASSWORD 'new-secret';"
> # then edit deploy/.env + deploy/env/prod, and recreate the backend:
> docker compose up -d --force-recreate backend-prod
> ```

## Upgrading from the pre-split stack (volume rename)

The single-env `compose.deploy.yml` this stack replaces kept its data in
`jpcorrect-backend_postgres_data`. The split stack uses
`jpcorrect-backend_postgres_prod_data` and `jpcorrect-backend_postgres_dev_data`,
so a host that still has the old volume would get a freshly initialised (empty)
prod database and leave the old one orphaned.

The current deploy host never carried that volume — prod was created fresh under
the new name — so nothing had to be migrated here. On any host that *does* still
have it, copy the data across **before** the first `make -C deploy up-prod`:

```bash
docker volume ls | grep jpcorrect-backend_postgres_data   # confirm it exists
docker volume create jpcorrect-backend_postgres_prod_data
docker run --rm \
  -v jpcorrect-backend_postgres_data:/from \
  -v jpcorrect-backend_postgres_prod_data:/to \
  alpine sh -c 'cd /from && cp -a . /to'
```

Keep the old volume until the new prod stack has been verified; it is the only
rollback.

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

- `backend-prod` running `:stable` → new `:stable` digest auto-updates. ✓
- `backend-dev`  running `:dev`    → new `:dev` digest auto-updates. ✓
- `api-tools-prod` running `:stable` / `api-tools-dev` running `:dev` → both
  auto-update too; see "api-tools auto-updates too" below.
- `backend-dev` running `:test` (one-off manual dispatch) → only updates if
  `:test` itself is re-published. To go back to tracking `:dev`, you must
  manually recreate with the default tag (see above).

### Per-env cadence (scope split)

There are two Watchtower instances, separated by `WATCHTOWER_SCOPE`:

- `watchtower-prod` (`scope=prod`) — `WATCHTOWER_SCHEDULE="0 0 3 * * *"` with
  `TZ=Asia/Taipei`, so it wakes up once a day at 03:00 Taipei and pulls fresh
  `:stable` digests for `backend-prod`. Off-hours window means a bad `:stable`
  doesn't take prod down during the day.
- `watchtower-dev` (`scope=dev`) — `WATCHTOWER_POLL_INTERVAL=300`, polls every
  5 minutes and pulls fresh `:dev` digests for `backend-dev`.

Each watchtower only touches containers carrying the matching
`com.centurylinklabs.watchtower.scope=<env>` label, so the two instances are
truly independent — dev rolling forward never triggers a prod restart and
vice versa.

### api-tools auto-updates too

Both `api-tools-*` services carry
`com.centurylinklabs.watchtower.enable=true`; API-tools publishes multi-arch
(`linux/amd64,linux/arm64`) images, so a watchtower pull is safe on this arm64
machine.

The two run on different cadences, because that is how API-tools tags its
images:

| Service | Tag | Republished when | Watchtower |
| --- | --- | --- | --- |
| `api-tools-dev` | `:dev` | every push to API-tools `main` | `watchtower-dev`, polls every 5 min |
| `api-tools-prod` | `:stable` | a `v*.*.*` tag is pushed in API-tools | `watchtower-prod`, daily at 03:00 Taipei |

So dev tracks API-tools `main` within minutes, while prod only moves when that
repo cuts a release.

### Check a tag is multi-arch before prod pulls it

Watchtower pulls whatever digest currently sits behind the tag, so before
letting `watchtower-prod` pull a manually pinned or freshly cut tag, verify
the manifest carries arm64:

```bash
docker buildx imagetools inspect ghcr.io/sessatakuma/api-tools:stable \
  | grep Platform
# needs both linux/amd64 and linux/arm64
```

If a tag is missing `linux/arm64`, don't let `watchtower-prod` pull it. To
hold a known-good image, pin it by digest via `API_TOOLS_PROD_IMAGE` /
`API_TOOLS_DEV_IMAGE` — an image reference watchtower will not move off.

### Verifying watchtower without touching a running env

The scope label is the safety mechanism, so a dry run is just a watchtower with
a scope nothing real carries. This scans exactly one throwaway container and
cannot see the live ones:

```bash
docker run -d --name wt-probe \
  --label com.centurylinklabs.watchtower.enable=true \
  --label com.centurylinklabs.watchtower.scope=probe \
  alpine:latest sleep infinity

docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -e DOCKER_API_VERSION=1.41 -e WATCHTOWER_LABEL_ENABLE=true \
  -e WATCHTOWER_SCOPE=probe -e WATCHTOWER_CLEANUP=false \
  containrrr/watchtower:1.7.1 --run-once

docker rm -f wt-probe
```

A healthy run ends in `Session done  Failed=0 Scanned=1 Updated=0` — `Scanned=1`
is the proof that the scope filter excluded everything else on the host.
