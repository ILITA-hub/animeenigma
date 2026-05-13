# infra/grafana/alerts/

Canonical source-of-truth for **v3.1 Self-Healing** Grafana unified-alerting
rules (Phase 23 onward). New alert rules related to the scraper
self-healing loop, canary observability, and maintenance-bot dispatch land
here.

## Why this directory exists

Prior to v3.1, all alerts lived inline in
`docker/grafana/provisioning/alerting/rules.yml` and were tightly coupled
to the dev docker-compose stack. Production Kubernetes provisioning had
to duplicate the YAML into
`deploy/kustomize/base/monitoring/grafana/configmap-alerts.yaml`, which
drifted easily.

`infra/grafana/alerts/` is the single source-of-truth that **both** dev
compose and prod Kubernetes pull from. The dev stack mounts this
directory directly into the running Grafana container (see
[`docker/docker-compose.yml`](../../../docker/docker-compose.yml)
`grafana.volumes` entry); production K8s rendering (out of scope for
Phase 23) reads the same source.

## Files

| File | Rules | Routes to | Owner phase |
|------|-------|-----------|-------------|
| `scraper.yaml` | `ScraperPlayabilityRegression` (warning), `ScraperAdDecoySurge` (warning), `ScraperUnplayableSpike` (critical) | `maintenance-webhook` contact point → services/maintenance `/api/grafana-webhook` | Phase 23 / v3.1 |

## Label contract

Every rule in this directory MUST emit the `provider`, `server`, and
`reason` labels in its `annotations` block (using `{{ $labels.X }}`
templates). The maintenance bot's reason-enum dispatch table in
`.claude/maintenance-prompt.md` matches on these labels to pick Pattern 6
(ad-decoy) or Pattern 7 (schema drift / packed-JS rotation) fix paths.
Omitting any of the three labels degrades the bot to "escalate" (manual
admin attention) instead of "button_fix" (autonomous proposal).

## Provisioning wiring

The dev stack uses **Option A** from the Phase 23 plan: the three rules
are also appended inline into
`docker/grafana/provisioning/alerting/rules.yml` so Grafana's existing
provisioning provider picks them up without a second provider entry.

If those two copies drift, the source-of-truth is **this directory**.
A "keep in sync" pointer comment lives in
`docker/grafana/provisioning/alerting/rules.yml` immediately above the
appended block.

## Production K8s

Out of scope for Phase 23. When v3.2+ ships Kubernetes alert
provisioning, the kustomize base should source-of-truth from this
directory via `configMapGenerator` (mirroring the dashboards path).
