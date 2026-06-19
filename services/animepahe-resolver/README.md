# animepahe-resolver

A Node 20 + Fastify + puppeteer-extra stealth sidecar for the AnimeEnigma
scraper service. Maintains a single warmed Chromium page against
`https://animepahe.pw`, exposes a thin HTTP API on `:3000` that the Go scraper
calls (the parser rewrite ships in Plan 27-02), and handles DDoS-Guard challenge
solving via the stealth plugin.

> **Why this exists:** Phase 27 of the v3.1 Scraper Self-Healing milestone — see
> `.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/` for the
> full design rationale.

## Hardcoded upstream

This sidecar is **HARDCODED** to `https://animepahe.pw`. Adding a second
upstream requires sandbox re-enablement or explicit security review — see
`server.js` header comment + `STEALTH-PINS.md` § "Hardcoded-upstream
invariant".

## HTTP API

All routes listen on `0.0.0.0:3000` (internal docker network only).

| Route                                   | Purpose                                            |
|-----------------------------------------|----------------------------------------------------|
| `GET /healthz`                          | Two-layer probe (HTTP + `page.evaluate(()=>1)`). 200 with `{browser:"up", lastChallengeSolveAt, pageCount}` on success, 503 with `{browser:"down", reason}` otherwise. |
| `GET /search?q=<title>`                 | Passthrough of upstream `m=search` JSON. |
| `GET /release?session=<UUID>&page=<N>`  | Passthrough of upstream `m=release` JSON (page default 1). |
| `GET /play?animeSession=<sess>&episodeSession=<sess>` | Passthrough of upstream play-page HTML verbatim. |
| `GET /metrics`                          | Prometheus scrape endpoint. |

## Metrics

Counters exposed at `/metrics` (label values NEVER include cookies — see Threat T-27-01-04):

| Counter                                  | When it ticks |
|------------------------------------------|---------------|
| `stealth_challenge_solves_total`         | First-attempt 403 followed by a successful retry. |
| `stealth_challenge_failures_total`       | Second-attempt 403 — stealth plugin defeated; needs refresh per `STEALTH-PINS.md`. |
| `page_recycle_total`                     | Every `PAGE_RECYCLE_AT`-th request (default 100) — warm page closed + reopened. |
| `upstream_403_total{stage="first"\|"second"}` | Per-stage 403 counter. |
| Default `prom-client` process metrics    | CPU, memory, GC, event loop lag. |

## Local development

```bash
# Install deps without downloading Chromium (the puppeteer:24 base image
# ships one; for local node tests we don't need it at all):
cd services/animepahe-resolver
PUPPETEER_SKIP_DOWNLOAD=true npm ci

# Run the offline unit test suite:
npm test
```

## Building + running locally (matches Plan 27-01 Task 4)

```bash
# From the project root:
docker build -t animepahe-resolver:dev \
    -f services/animepahe-resolver/Dockerfile .

# Run with the same resource limits Plan 27-03 enforces in compose:
docker run --rm --name animepahe-resolver-soak \
    -p 127.0.0.1:3000:3000 \
    --shm-size=256m \
    --memory=500m \
    animepahe-resolver:dev

# Wait for /healthz to flip green:
until curl -fsS http://localhost:3000/healthz | grep -q '"browser":"up"'; do sleep 2; done

# Smoke a /search:
curl -sS http://localhost:3000/search?q=Frieren | jq '.data[0].title'
```

## Operator playbook

| Symptom                                                                                    | Fix path                                                                                       |
|---------------------------------------------------------------------------------------------|------------------------------------------------------------------------------------------------|
| `/healthz` returns 503 `browser:"down"` for > 90 s                                          | `make redeploy-animepahe-resolver` — compose `restart: unless-stopped` auto-recovers most cases. |
| `stealth_challenge_failures_total` rises sustained > 1 h                                   | Follow `STEALTH-PINS.md` § Refresh Procedure (npm install latest stealth pins + npm test + redeploy). |
| `/search` consistently returns 502 `stealth_challenge_failed` after pin refresh             | Set `animepahe` status to `degraded`/`disabled` in the catalog `scraper_providers` DB table (single source of truth; hot-reloaded ~60s) and escalate per `.claude/maintenance-prompt.md` Pattern 7 animepahe-resolver branch. |
| `page_recycle_total` not incrementing                                                       | Sidecar restarted recently; expected for the first `PAGE_RECYCLE_AT` requests. After ≥ N requests, ≥ 1 recycle should be visible. |
| `docker stats animepahe-resolver` shows RSS > 450 MB sustained                              | Lower `PAGE_RECYCLE_AT` env (e.g. to 50) and redeploy. If still > 450 MB, switch to close-first recycle order (`browser.js::recyclePage({closeFirst: true})`). Update STEALTH-PINS.md D5 section. |

## Environment variables

| Variable                        | Default                            | Purpose |
|---------------------------------|------------------------------------|---------|
| `PORT`                          | `3000`                             | HTTP listen port. |
| `HOST`                          | `0.0.0.0`                          | HTTP listen host. |
| `LOG_LEVEL`                     | `info`                             | Pino logger level. |
| `PUPPETEER_EXECUTABLE_PATH`     | unset (auto-detect via `PUPPETEER_CACHE_DIR`) | Optional override for the Chrome binary path. The puppeteer:24 base image ships Chrome under `/home/pptruser/.cache/puppeteer/chrome/linux-*/chrome-linux64/chrome`; puppeteer's launcher finds it via `PUPPETEER_CACHE_DIR`. Set this only when running outside the official image. |
| `PUPPETEER_SKIP_DOWNLOAD`       | `true` (in Dockerfile)             | Skip puppeteer's bundled Chromium download. |
| `PAGE_RECYCLE_AT`               | `100`                              | Pattern 3 recycle cadence (lower → more frequent). |

## See also

- `.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/27-CONTEXT.md` — locked operator decisions (D1 – D7).
- `.planning/phases/27-animepahe-revival-via-stealth-chromium-sidecar/27-RESEARCH.md` — architecture patterns, code examples, common pitfalls, security domain.
- `STEALTH-PINS.md` — pin manifest + refresh procedure.
- `.claude/maintenance-prompt.md` — Pattern 7 escalation (animepahe-resolver branch).
