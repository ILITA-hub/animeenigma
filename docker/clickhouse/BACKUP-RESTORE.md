# ClickHouse Backup & Restore Runbook (AR-STORE-01)

Operational runbook for the ClickHouse unified event plane (v4.0 Activity
Register, Phase 1). Backups are driven through the `clickhouse-backup` sidecar
(`altinity/clickhouse-backup:2.7.0`) via `docker exec` — the sidecar runs
`sleep infinity` and does no work on its own.

## Where backups live

Backups are written **inside the shared `clickhouse_data` named volume** at
`/var/lib/clickhouse/backup/<name>/`. They are NOT on a separate volume.

> **Why same volume:** `clickhouse-backup` creates a backup by **hardlinking**
> table parts from ClickHouse's `shadow/` directory into `backup/`. Hardlinks
> cannot cross filesystems, so the backup directory MUST sit on the same device
> as the data directory. Mounting a separate volume over
> `/var/lib/clickhouse/backup` produces `invalid cross-device link` and the
> backup fails. (Discovered during the AR-STORE-01 dry-run; see Deviations in
> 01-01-SUMMARY.md.)

Because backups share the data volume, they survive container restarts but are
NOT off-host. Off-host replication (remote object storage) is out of scope for
Phase 1 and can be layered later via `clickhouse-backup`'s `upload`/`remote`
config.

## Credentials

The sidecar authenticates as the dedicated CH user, NOT `default`:

- `CLICKHOUSE_HOST=clickhouse`
- `CLICKHOUSE_USERNAME=${CLICKHOUSE_USER:-analytics}`
- `CLICKHOUSE_PASSWORD=${CLICKHOUSE_PASSWORD:-changeme}`

> **Why username matters:** the CH entrypoint creates `CLICKHOUSE_USER`
> **instead of** `default` (we run with `CLICKHOUSE_DEFAULT_ACCESS_MANAGEMENT`),
> so a username-less sidecar fails auth with `code: 516 ... Authentication
> failed`. Keep `CLICKHOUSE_USERNAME` in sync with `CLICKHOUSE_USER`.

The prod host MUST override `CLICKHOUSE_PASSWORD` in `docker/.env` — do not ship
the `changeme` placeholder to production.

## Create a backup

```bash
# Name the backup; date-stamped names are recommended.
docker exec animeenigma-clickhouse-backup \
  clickhouse-backup create "backup-$(date +%F)"
```

`clickhouse-backup create` errors with `no tables for backup` if every database
is empty — that is expected on a fresh instance before the analytics schema
(01-02 / 01-03) lands. Once tables exist, the backup captures schema + data.

## List backups

```bash
docker exec animeenigma-clickhouse-backup clickhouse-backup list
# local  backup-2026-06-04  ... data:...  meta:...  regular
```

## Restore

### Full restore (schema + data) — DESTRUCTIVE

```bash
docker exec animeenigma-clickhouse-backup \
  clickhouse-backup restore "backup-2026-06-04"
```

### Schema-only dry-run into a scratch database (SAFE — proves the procedure)

Restoring straight back over a live/just-dropped table hits a `code: 57 ...
Directory for table data store/<uuid>/ already exists` conflict, because
Atomic-engine databases keep the on-disk store dir (keyed by table UUID) for a
delay window after a `DROP`. To prove a restore non-destructively, restore the
**schema** into a throwaway database via a database mapping:

```bash
docker exec animeenigma-clickhouse-backup \
  clickhouse-backup restore --schema \
  --restore-database-mapping analytics:restore_scratch \
  "backup-2026-06-04"

# Verify, then tear down the scratch DB:
curl -s "http://127.0.0.1:8123/?user=analytics&password=$CLICKHOUSE_PASSWORD" \
  --data-binary "EXISTS restore_scratch.<table>"
curl -s "http://127.0.0.1:8123/?user=analytics&password=$CLICKHOUSE_PASSWORD" \
  --data-binary "DROP DATABASE IF EXISTS restore_scratch SYNC"
```

## Recorded dry-run result

> **Dry-run restore verified 2026-06-04: PASS.** Created backup `dryrun-2026-06-04`
> (image `altinity/clickhouse-backup:2.7.0` against `clickhouse/clickhouse-server:26.3.12.3`)
> capturing a sentinel `MergeTree` table in the `analytics` database; confirmed via
> `clickhouse-backup list` (`local ... regular`). Restored the schema into a scratch
> database with `restore --schema --restore-database-mapping analytics:restore_scratch`
> — `clickhouse-backup` logged `done, operation=restore_schema` and the table was
> recreated with the correct DDL (`id UInt64, note String, ts DateTime`). Scratch
> database dropped afterward; the `dryrun-2026-06-04` backup remains in the
> `clickhouse_data` volume. The sentinel table was a transient verification artifact
> and was not left behind — the real analytics schema lands in plans 01-02 / 01-03.
