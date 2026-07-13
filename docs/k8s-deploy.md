# Kubernetes Deploy Target (`deploy/kustomize/`)

> Status: **real deploy target since 2026-07-13** (was a 9-service POC before; audit 2026-06-21 findings #10, #11, #28, #29 closed). Production still runs docker-compose — this tree exists so a k8s migration is a decision, not a project.

## Layout

```
deploy/kustomize/
├── base/                      # everything EXCEPT secrets — not deployable alone
│   ├── namespace.yaml         # animeenigma namespace
│   ├── configmap.yaml         # shared env: service URLs, datastore hosts
│   ├── services/              # 1 file per first-party service (Deployment+Service)
│   ├── datastores/            # postgres/redis/minio/nats/clickhouse/meilisearch
│   │                          #   StatefulSets + otel-collector/jackett Deployments
│   ├── monitoring/            # prometheus + grafana (namespace: monitoring)
│   └── admin/                 # pgadmin, kubernetes-dashboard, admin ingress
└── overlays/
    └── prod/                  # THE deployable entrypoint
        ├── app-secrets/       # animeenigma-secrets  ← animeenigma.env (git-ignored)
        └── admin-secrets/     # grafana/pgadmin/basic-auth ← *.env + auth.htpasswd
```

## Secrets model (audit #10)

No Secret objects are committed to git. The prod overlay **generates** them from env
files that live only on the deploy host, exactly like `docker/.env` does for compose:

```bash
cd deploy/kustomize/overlays/prod
cp app-secrets/animeenigma.env.example    app-secrets/animeenigma.env
cp admin-secrets/grafana.env.example      admin-secrets/grafana.env
cp admin-secrets/pgadmin.env.example      admin-secrets/pgadmin.env
htpasswd -nbB admin "$(openssl rand -base64 24)" > admin-secrets/auth.htpasswd
$EDITOR app-secrets/animeenigma.env       # fill in real values (openssl rand -hex 32)
```

Guards, in order of firing:

| Guard | When | What it refuses |
|---|---|---|
| `deploy/scripts/k8s-secret-guard.sh` | CI + `make k8s-validate` | placeholder strings in git-tracked kustomize files |
| `deploy/scripts/k8s-preflight.sh` | `make k8s-apply-prod` | missing env files, `CHANGE_ME`, weak jwt/stream secrets |
| `deploy/scripts/k8s-compose-parity.sh` | CI + `make k8s-validate` | compose↔kustomize service-set drift (audit #28 regression guard) |

Secret names are stable (`disableNameSuffixHash: true`), so changing a value needs
`make k8s-restart` afterwards — there is no hash-triggered rollout.

## Deploying (single-node k3s is the assumed shape)

```bash
# on the target node
curl -sfL https://get.k3s.io | sh -   # ships with traefik; we assume ingress-nginx annotations —
                                      # install ingress-nginx or translate admin/ingress.yaml
make k8s-apply-prod                   # preflight → kubectl apply -k overlays/prod → wait
make k8s-status                       # deployments / pods / services
make k8s-logs-catalog                 # per-service logs
```

Requirements the manifests assume:

1. **Images**: `ghcr.io/ilita-hub/animeenigma/<service>:latest` — published by
   `.github/workflows/docker.yml`. The cluster needs an imagePullSecret if the ghcr
   packages are private.
2. **Storage**: a default StorageClass (k3s ships `local-path`). PVC sizes:
   minio 100Gi, clickhouse 50Gi, postgres 20Gi, library staging 2×50Gi (see manifests).
3. **Ingress controller** with basic-auth annotation support (manifests use
   `nginx.ingress.kubernetes.io/*` — ingress-nginx, not the k3s default traefik).
4. **TLS**: not modeled yet — terminate at an external proxy or add cert-manager.

## What is deliberately NOT in k8s

| Compose service | Why absent |
|---|---|
| `backup`, `clickhouse-backup` | host-level backup design; k8s-native equivalent = future CronJobs |
| `node-exporter`, `vnstat-exporter` | host/NIC metrics — would become a DaemonSet, meaningless until multi-node |
| `web` dev-mounts etc. | compose dev conveniences have no k8s meaning |

`k8s-compose-parity.sh` only compares compose services with a `build:` key, so
image-only infra (`clickhouse-backup`, `node-exporter`) falls outside the check
automatically; `backup` and `vnstat-exporter` are its explicit `EXCLUSIONS`. If you
add a compose service, CI fails until you either add a manifest or justify an
exclusion there.

> `infra/helm/` (a gateway-only chart depending on a nonexistent `common` chart) was
> **deleted 2026-07-13** — it was an abandoned POC; kustomize is the single k8s source
> of truth. Recover from git history if helm ever becomes the direction.

## Validation without a cluster

```bash
make k8s-validate     # secret guard + parity + kustomize build (needs kustomize in PATH)
# full schema validation (what CI runs):
kustomize build deploy/kustomize/base | kubeconform -strict -ignore-missing-schemas -summary
```

CI: `.github/workflows/k8s-validate.yml` runs all of the above on every PR touching
`deploy/**` or `docker/docker-compose.yml`.

## Migration runbook sketch (compose → k8s, when the day comes)

1. Stand up k3s on a second node (or a VM) — never experiment on the prod host.
2. `make k8s-apply-prod`, seed secrets from `docker/.env` values.
3. Data: `pg_dump | psql` into the postgres PVC; `mc mirror` MinIO → MinIO;
   redis is cache-only (let it warm); clickhouse `BACKUP/RESTORE` or clickhouse-backup.
4. Point a test domain at the new ingress, run the probe suite + ui_audit_bot flows.
5. Cut DNS over; keep compose warm for instant rollback.

## Known caveats

- `stealth-scraper` (Camoufox) uses an in-memory `/dev/shm` emptyDir; browser pods are
  memory-hungry — watch the limit before adding replicas.
- `otel-collector` filelog receiver tails `/var/lib/docker/containers` via hostPath —
  on containerd runtimes (k3s default!) that path is empty and the log pipeline is
  silently inert; the metrics/traces pipelines are unaffected.
- Grafana/Prometheus upstream URLs cross namespaces (`*.monitoring.svc`) — already
  encoded in `configmap.yaml`.
- ClickHouse cannot set its preferred nofile ulimit from a pod spec; configure the
  container runtime default if CH complains.
