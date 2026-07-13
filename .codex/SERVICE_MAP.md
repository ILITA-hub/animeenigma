# Service Impact Map

This map is based on the current `docker/docker-compose.yml` and Makefile, not the incomplete legacy after-update list.

## Deployable Compose application services

`auth`, `catalog`, `streaming`, `player`, `rooms`, `scheduler`, `gateway`, `themes`, `scraper`, `stealth-scraper`, `library`, `notifications`, `watch-together`, `gacha`, `recs`, `policy`, `governor`, `storage`, `anidle`, `fanfic`, `analytics`, `upscaler`, and `web` (`frontend/web`).

`maintenance` is a host-native service, not a Compose application; build, restart, and verify it through its dedicated maintenance workflow rather than `bin/ae-deploy.sh`.

Infrastructure services—Postgres, Redis, MinIO, NATS, Meilisearch, Prometheus, Grafana, ClickHouse, exporters, backup, the Megacloud helper, and Jackett—are not routine application redeploys. Their configuration changes are operations work with dedicated preflight and rollback.

## Path-to-impact rules

| Changed path | Minimum assessment |
|---|---|
| `services/<name>/**` | Verify/deploy `<name>` and direct consumers if its contract/config changes. |
| `services/maintenance/**` | Verify the host-native maintenance binary and its system service; do not treat it as a Compose container. |
| `libs/**` | Identify runtime importers; do not use a fixed legacy all-service list. |
| `api/**` | Identify generated server/client consumers and review generated diffs. |
| `frontend/web/**` | Verify/deploy `web`; run locale checks if locales changed. |
| `docker/docker-compose.yml` | Production infrastructure assessment for every changed service. |
| `deploy/kustomize/**` | Kustomize guards/validation; apply requires explicit target authority. |
| `infra/**` | Host/monitoring/edge change; runbook plus rollback required. |
| `scripts/**`, `bin/**`, `Makefile` | Establish whether live workflow behavior changes before relying on it. |

## Safeguards

- `bin/ae-deploy.sh` is a Compose helper, not a complete impact engine.
- The Claude after-update service map is historical and incomplete. Derive the deployment set from changed paths and this map.
- Serialized deployment is mandatory.
- If the shared tree is dirty or cannot fast-forward, stop. Do not force-sync, stash, reset, or deploy an ambiguous revision.
