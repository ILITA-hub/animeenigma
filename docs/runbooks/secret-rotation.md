# Runbook — Rotating datastore secrets

**Why this exists.** Historically `docker/docker-compose.yml` hardcoded weak
secrets (`POSTGRES_PASSWORD: postgres`, `MINIO_ROOT_*: minioadmin`,
`MEILI_MASTER_KEY: masterkey`, ClickHouse `changeme`, Grafana `admin`,
`ANALYTICS_IP_SALT: change-me-in-production`) and ignored the matching
`docker/.env` keys. The compose file now reads every one of these from
`docker/.env` (with the old weak values kept only as defaults), so this runbook
is the second half: replacing the live values.

**Threat context.** Postgres / Redis / MinIO / Meili / ClickHouse are all bound
to `127.0.0.1` only (see the `ports:` blocks), so they are not reachable from
the internet — an attacker needs host/localhost access. The two items that ARE
reachable beyond the host are **Grafana** (`/admin/grafana` via the gateway) and
the **analytics IP salt** (a public salt makes hashed IPs reversible). Prioritise
those two; the loopback datastores are defence-in-depth.

> ⚠️ Changing an env var does NOT re-key a datastore whose data volume is
> already initialized. Postgres and Grafana set their admin password on FIRST
> init only. You must run the explicit rotation command for each. Do this in a
> short maintenance window — several services restart.

All commands run from the repo root unless noted. `docker/.env` is git-ignored;
keep a backup of the new values in your password manager.

Generate strong values: `openssl rand -base64 32`

---

## 1. Postgres password (shared by all services)

`DB_USER` / `DB_PASSWORD` / `DB_NAME` are one source of truth — the postgres
server and every service read the same vars.

```bash
# a) choose a new password
NEWPW="$(openssl rand -base64 32)"

# b) re-key the live role IN postgres (volume already initialized)
docker compose -f docker/docker-compose.yml exec postgres \
  psql -U postgres -d animeenigma -c "ALTER USER postgres WITH PASSWORD '$NEWPW';"

# c) put it in docker/.env  (DB_PASSWORD=...)  — edit the file, then verify:
grep '^DB_PASSWORD=' docker/.env

# d) recreate every service so they pick up the new env (postgres itself is
#    already re-keyed in step b; POSTGRES_PASSWORD only matters on first init):
docker compose -f docker/docker-compose.yml up -d
```

Verify: `make health` (all green) and `docker compose logs catalog | grep -i "password authentication failed"` returns nothing.

Rollback: re-run the `ALTER USER` with the old password and revert `docker/.env`.

---

## 2. MinIO root credentials

MinIO re-reads `MINIO_ROOT_USER` / `MINIO_ROOT_PASSWORD` from env on every
start, so no SQL-style rotation is needed — but all consumers must restart
together.

```bash
# set MINIO_ROOT_USER / MINIO_ROOT_PASSWORD in docker/.env, then:
docker compose -f docker/docker-compose.yml up -d minio
docker compose -f docker/docker-compose.yml up -d streaming player library
```

> ⚠️ The `streaming` service authenticates with `S3_ACCESS_KEY` / `S3_SECRET_KEY`
> (separate vars, already strong in the live `.env`). If those were created as a
> MinIO **user** (not root), keep them in sync via `mc admin user`. Confirm
> playback (`ae` source) after rotating — that is the canary for MinIO auth.

Verify: MinIO console at `http://127.0.0.1:9001` accepts the new creds; an `ae`
title plays end-to-end.

---

## 3. Grafana admin password (internet-reachable — do this one)

Grafana stores the admin password in its own DB on first init; the
`GF_SECURITY_ADMIN_PASSWORD` env is ignored afterward. Reset it via the CLI:

```bash
# set GRAFANA_ADMIN_PASSWORD in docker/.env (used for fresh installs), then
# reset the EXISTING admin password in the running container:
docker compose -f docker/docker-compose.yml exec grafana \
  grafana-cli admin reset-admin-password "$(grep '^GRAFANA_ADMIN_PASSWORD=' docker/.env | cut -d= -f2-)"
```

Verify: log in at `/admin/grafana` with the new password.

---

## 4. Analytics IP salt (internet-relevant — do this one)

`ANALYTICS_IP_SALT` is consumed at runtime to hash client IPs. The default
`change-me-in-production` is public, so hashed IPs are currently reversible.
Rotating is **forward-only** (no data migration; historical hashes simply stop
joining to new ones — acceptable):

```bash
# set ANALYTICS_IP_SALT=<openssl rand -base64 32> in docker/.env, then:
docker compose -f docker/docker-compose.yml up -d analytics
```

---

## 5. ClickHouse password

```bash
# set CLICKHOUSE_PASSWORD in docker/.env (the clickhouse image applies it from
# env on container (re)create), then recreate clickhouse + its readers:
docker compose -f docker/docker-compose.yml up -d clickhouse
docker compose -f docker/docker-compose.yml up -d analytics otel-collector grafana
```

Verify: `docker compose logs analytics | grep -i clickhouse` shows no auth
errors; the product-analytics Grafana dashboard still loads.

---

## 6. Redis password (optional — loopback-bound)

Enabling Redis auth needs a SERVER-side change too. In `docker-compose.yml`,
change the redis `command:` to require a password, then set `REDIS_PASSWORD`:

```yaml
  redis:
    command: redis-server --appendonly yes --requirepass ${REDIS_PASSWORD:-}
```

```bash
# set REDIS_PASSWORD in docker/.env, then recreate redis + all clients:
docker compose -f docker/docker-compose.yml up -d
```

Verify: `docker compose exec redis redis-cli -a "$REDIS_PASSWORD" ping` → `PONG`;
gateway/watch-together logs show no `NOAUTH` errors. (Lowest priority — Redis is
loopback-only and holds no long-term secrets.)

---

## 7. Meilisearch master key (optional — loopback-bound)

```bash
# set MEILI_MASTER_KEY in docker/.env, then recreate meili + its clients:
docker compose -f docker/docker-compose.yml up -d meilisearch
docker compose -f docker/docker-compose.yml up -d catalog   # whoever indexes
```

---

## Order & blast radius

1. **Analytics IP salt** and **Grafana** first — these are the only items
   reachable beyond the host, and both are low-risk to rotate.
2. **Postgres** next — highest blast radius (touches every service); pick a
   quiet window and run `make health` after.
3. **MinIO** — verify playback after.
4. **ClickHouse / Redis / Meili** — loopback-only, do at leisure.

After all rotations: `make health`, smoke-test login + one playback, and confirm
the Grafana dashboards still render.
