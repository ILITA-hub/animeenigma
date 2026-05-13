# infra/grafana/dashboards/

Canonical source-of-truth for **v3.1 Self-Healing** Grafana dashboards
(Phase 23 onward). New dashboards related to the scraper self-healing
loop, canary observability, and maintenance-bot dispatch land here.

## Why this directory exists

Prior to v3.1, all dashboards lived under
`docker/grafana/dashboards/` and were tightly coupled to the dev
docker-compose stack. Production Kubernetes provisioning had to
duplicate the JSON into
`deploy/kustomize/base/monitoring/grafana/configmap-dashboards.yaml`,
which drifted easily.

`infra/grafana/dashboards/` is the single source-of-truth that **both**
dev compose and prod Kubernetes pull from. The dev stack mounts this
directory directly into the running Grafana container (see
[`docker/docker-compose.yml`](../../../docker/docker-compose.yml)
`grafana.volumes` entry); production K8s rendering (out of scope for
this phase) will sync from the same files via Kustomize.

## Naming convention

`<area>-<subject>.json` — kebab-case, lowercase, no version suffixes.
The dashboard `uid` field uses the same name plus a discriminator
(e.g., `scraper-provider-health-canary`) to avoid colliding with older
dashboards that target a similar area.

## Relationship to `docker/grafana/dashboards/`

| Directory | Role |
|-----------|------|
| `docker/grafana/dashboards/` | Legacy dev-only mount. Existing 7 dashboards predate this convention and stay where they are. |
| `infra/grafana/dashboards/` | v3.1+ source-of-truth, mounted into dev Grafana at `/var/lib/grafana/dashboards-infra` (sibling path — a subdirectory of the existing `/var/lib/grafana/dashboards` read-only mount cannot host a second mountpoint) via a second file-provisioner provider entry (`infra-self-healing`, folder `Self-Healing`). |
| `deploy/kustomize/base/monitoring/grafana/configmap-dashboards.yaml` | Production K8s dashboards configmap — will sync from this directory in a future deploy plan. **Out of scope for Phase 23.** |

## Current dashboards

- **`scraper-provider-health.json`** — Phase 23-02. Four panels driven
  by `playability_canary_runs_total` (Plan 23-01) and
  `scheduler_job_last_success_timestamp{job="scraper_playability_canary"}`:
  pass/fail stacked bar per (provider, server) over 24h, failure
  reason breakdown, last canary run timestamp, and top failing
  (provider, server, reason) tuples table.

## Adding a new dashboard

1. Drop the JSON into this directory using the naming convention above.
2. Validate with `jq -e . infra/grafana/dashboards/<name>.json`.
3. Restart Grafana (`docker compose -f docker/docker-compose.yml restart grafana`)
   to trigger the file provisioner to pick it up.
4. (Future) Update the kustomize configmap when production deploy is in scope.
