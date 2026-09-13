# Deploy operations

Operational notes for this host. For architecture, networks, and the deploy stack
design see [`../AGENTS.md`](../AGENTS.md) → *Deployment stack (two environments
on one host)*. This file only documents day-to-day ops gotchas that you can't
read off the `compose.yml`.

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
- `api-tools-prod` / `api-tools-dev` → **not** auto-updated right now; see
  "api-tools is opted out" below.
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

### api-tools is opted out of auto-update (for now)

Both `api-tools-*` services carry
`com.centurylinklabs.watchtower.enable=false`. The backend's GHCR images are
multi-arch (amd64 + arm64) as of #38, but API-tools still publishes
**amd64-only** tags — so on the arm64 deploy host a watchtower pull would swap a
working locally-built arm64 image for one the host cannot execute.

Re-enable (flip both labels to `enable=true` and recreate the two containers)
once **[sessatakuma/API-tools#64](https://github.com/sessatakuma/API-tools/pull/64)**
has merged *and* its CD has republished `:stable` / `:dev` as multi-arch. Check
before flipping:

```bash
docker buildx imagetools inspect ghcr.io/sessatakuma/api-tools:dev | grep Platform
# needs both linux/amd64 and linux/arm64
```

Until then, keep the tags pinned by hand via `API_TOOLS_PROD_IMAGE` /
`API_TOOLS_DEV_IMAGE`.

### Before enabling watchtower-prod on an arm64 host

`:stable` is only republished when a `v*.*.*` tag is pushed. If the newest
`:stable` on GHCR predates the multi-arch switch it is amd64-only, and letting
`watchtower-prod` pull it would break prod. Push a `v*.*.*` tag first, confirm
the manifest has a `linux/arm64` entry (same `imagetools inspect` command as
above), then bring `watchtower-prod` up.

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
