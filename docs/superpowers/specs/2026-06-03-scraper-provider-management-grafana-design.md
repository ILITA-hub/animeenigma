# Scraper Provider Management via Config File + Grafana — Design Spec

**Date:** 2026-06-03
**Status:** Approved design — ready for implementation plan
**Related:** ISS-022 (per-provider failover budget), ISS-023 (animepahe → Cloudflare)

## 1. Problem

The EN scraper's failover chain (gogoanime → animepahe → allanime → animefever →
miruro → nineanime → animekai) is governed by one env var,
`SCRAPER_DEGRADED_PROVIDERS` — a bare comma-separated kill-list. It has three
problems:

1. **Invisible.** A provider listed there is never `Register()`-ed, so it
   *vanishes* from `/scraper/health` entirely. There is no surface that shows
   "animepahe is disabled **because** X."
2. **No reason or history.** The *why* and *when* of a disconnect live only in a
   docker-compose comment and the operator's memory. There's no at-a-glance view
   of which providers are off, why, and since when.
3. **Awkward to manage.** Editing a comma list in `docker/.env` and restarting,
   with the rationale scattered across comments and `docs/issues`.

The operator wants a **management surface**: a table of every provider, its
status, and — for disconnected ones — **why** (reason + description) and **when**
(history).

## 2. Goal & Non-Goals

**Goal:** Replace `SCRAPER_DEGRADED_PROVIDERS` with a structured, git-versioned
config file as the single source of truth for provider enable/disable + reason +
description; reflect that state (plus live health) into Prometheus metrics; and
render a **Grafana dashboard** showing a provider table + a disconnect-history
timeline.

**Decided shape (from brainstorming):**
- Disconnect is **manual only** (operator edits the file). No circuit breaker /
  auto-disconnect.
- Storage is a **config file** (Approach C), not Redis state or a DB.
- The table renders in **Grafana** (admin-only, behind `/admin/grafana`), not a
  custom Vue page.

**Non-Goals (YAGNI):**
- No runtime toggle / in-app buttons. Management = edit YAML + restart scraper.
- No circuit breaker or automatic disable/re-enable.
- No Loki "why-history" panel (the timeline + `description` + git cover it; can be
  added later if a richer per-event log is wanted).
- No management of the 5 video *players* (Kodik/AniLib/Hanime/Raw/OurEnglish) —
  only the EN **scraper providers** that `SCRAPER_DEGRADED_PROVIDERS` governs.
- No explicit `history:` array in the YAML — the Grafana state-timeline gives
  "when" automatically and git gives who/when-edited; redundant to hand-maintain.

## 3. Architecture Overview

```
docker/scraper-providers.yaml   ← operator edits (source of truth, git-versioned)
        │  (mounted read-only into the scraper container)
        ▼
scraper boot: config loader ──► register enabled providers (skip disabled, as today)
        │                   └─► emit metrics for ALL providers (incl. disabled):
        │                         provider_enabled{provider} = 0|1
        │                         provider_info{provider,reason,description} = 1
        ▼
Prometheus (already scrapes scraper:8088/metrics)
        │   + existing provider_health_up{provider,stage} (live health, probe-set)
        ▼
Grafana dashboard "Scraper / Provider Management" (auto-provisioned)
        ├─ Table panel:        provider | enabled | live up/down | reason | description
        └─ State-timeline:     provider_enabled (+ provider_health_up) over time = history
```

No frontend or gateway changes. Grafana reads Prometheus directly.

## 4. Components

### 4.1 Config file — `docker/scraper-providers.yaml`

Source of truth, mounted read-only into the scraper at a path given by
`SCRAPER_PROVIDERS_FILE` (default `/config/providers.yaml`). Edit + `docker
compose restart scraper` (no rebuild). Git history = the who/when-edited audit.

Schema:
```yaml
providers:
  - name: allanime          # required; must match a known provider name
    enabled: true           # required
  - { name: animefever, enabled: true }
  - { name: miruro,     enabled: true }
  - { name: nineanime,  enabled: true }
  - name: animepahe
    enabled: false
    reason: "Cloudflare challenge"        # short category, shown in table
    description: >                        # full why, shown in table
      animepahe.pw moved DDoS-Guard → Cloudflare managed challenge; the stealth
      sidecar can't solve it (0% solve). ISS-023. Disabled 2026-06-03.
  - name: gogoanime
    enabled: false
    reason: "Platform rebrand"
    description: "anitaku.to migrated to a platform the parser can't handle. Disabled 2026-05-13."
```

Rules:
- `name` + `enabled` required. `reason`/`description` optional (sensible for
  enabled providers to omit).
- Names are validated at boot against the known provider set (the same names used
  in `main.go` registration: `gogoanime, animepahe, allanime, animefever, miruro,
  nineanime, animekai`). An unknown name → **fail-fast at boot** (matches the
  existing "fail-fast on typos" theme around `ValidatePriorityList`).
- A known provider **absent** from the file defaults to `enabled: true` (so
  forgetting to list one never silently disables it).

### 4.2 Scraper config loader — `services/scraper/internal/config/providers.go` (new)

- `LoadProviders(path string) (ProvidersConfig, error)` — parse + validate the
  YAML (`gopkg.in/yaml.v3`, already in the scraper module graph as indirect →
  promote to direct).
- `ProvidersConfig` exposes: `IsEnabled(name) bool`, and `All() []ProviderMeta`
  where `ProviderMeta{Name, Enabled, Reason, Description}`.
- Wiring in `Load()` (`config.go`): read `SCRAPER_PROVIDERS_FILE`.
  - **File present & valid** → it is the source of truth. `IsEnabled==false`
    replaces `DegradedProviders.IsDegraded==true` at every registration site in
    `main.go`.
  - **File absent** → fall back to the existing `SCRAPER_DEGRADED_PROVIDERS` env
    (back-compat; nothing breaks during migration). Log a WARN that the file is
    missing and env fallback is in use.
  - **File present but invalid** → fail-fast (boot error), same as other config
    validation.

### 4.3 Registration changes — `services/scraper/cmd/scraper-api/main.go`

Each provider block currently gates on
`cfg.DegradedProviders.IsDegraded(p.Name())`. Replace with a single resolved
predicate `providerEnabled(name) bool` that consults the file (or the env
fallback). Behaviour is identical to today: disabled → not registered → skipped
with zero per-request cost. (The ISS-022 per-provider budget still protects the
chain for any *enabled* provider that hangs.)

After resolving config, emit metrics for **every** provider in the known set
(enabled and disabled):
- `provider_enabled{provider}` = 1 if enabled else 0
- `provider_info{provider, reason, description}` = 1 (labels carry the text;
  empty strings for enabled providers with no reason)

This is what makes disabled providers **visible** instead of vanishing.

### 4.4 Metrics — `libs/metrics/provider.go` (extend)

Add alongside the existing `ProviderHealthUp` / `ProviderProbeLastTick`, matching
the existing `provider_*` namespace (NOT `scraper_*` — the Prometheus `job=scraper`
label already scopes them, and `provider_health_up` set the precedent):

```go
// ProviderEnabled is the config-driven management gauge: 1 = enabled (registered
// in the failover chain), 0 = disabled. Emitted for ALL known providers so
// disabled ones remain visible in Grafana. Source: scraper-providers.yaml.
ProviderEnabled = promauto.NewGaugeVec(prometheus.GaugeOpts{
    Name: "provider_enabled",
    Help: "Whether a scraper provider is enabled in the failover chain (1=enabled, 0=disabled), per scraper-providers.yaml",
}, []string{"provider"})

// ProviderInfo is an info-style gauge (always 1) carrying the human-readable
// management metadata for the Grafana table. reason/description come from
// scraper-providers.yaml; empty for enabled providers with none. Cardinality is
// bounded (~7 providers, values change only on restart after a file edit).
ProviderInfo = promauto.NewGaugeVec(prometheus.GaugeOpts{
    Name: "provider_info",
    Help: "Info metric (always 1) exposing per-provider management metadata (reason, description) for Grafana",
}, []string{"provider", "reason", "description"})
```

Cardinality note: ~7 providers; `reason`/`description` are operator-authored and
change only on a config edit + restart — no cardinality risk. On a reason change,
stale `provider_info` series age out normally (the old label set stops being
emitted); acceptable for this low volume.

Live up/down reuses the existing `provider_health_up{provider,stage}` (probe-set,
enabled providers only — disabled providers correctly show no live series).

### 4.5 Grafana dashboard — `infra/grafana/dashboards/scraper-providers.json` (new)

New dashboard (keep separate from the canary-focused
`scraper-provider-health.json`), auto-provisioned via the existing
`infra/grafana/dashboards` → `/var/lib/grafana/dashboards-infra` mount.
- `uid: scraper-provider-management`, `schemaVersion: 38`, datasource
  `{"type":"prometheus","uid":"${DS_PROMETHEUS}"}` (mirror existing dashboards).
- **Panel 1 — Table "Provider Management"** (current state). Build from:
  - `provider_info` (provides the `provider`, `reason`, `description` columns via
    label-to-field),
  - `provider_enabled` (Enabled column),
  - `max by (provider) (provider_health_up)` (live Up/Down column).
  Use Grafana table transformations (outer join on `provider`) + value mappings
  (1→🟢 Up / Enabled, 0→🔴 Down / ⚪ Disabled). Columns: Provider | Enabled |
  Live | Reason | Description.
- **Panel 2 — State timeline "Connect / Disconnect History"**:
  `provider_enabled{provider}` (and optionally `max by (provider)
  (provider_health_up)`) over time → shows exactly when each provider was
  disabled/re-enabled. This is the **when/history** view, accruing from launch.

### 4.6 docker-compose — `docker/docker-compose.yml`

- Add a read-only volume mount to the `scraper` service:
  `./scraper-providers.yaml:/config/providers.yaml:ro`.
- Add `SCRAPER_PROVIDERS_FILE: /config/providers.yaml`.
- Update the `SCRAPER_DEGRADED_PROVIDERS` comment to mark it **deprecated /
  fallback-only** (used only when the file is absent). Leave the var in place for
  back-compat; do not remove this milestone.

### 4.7 Migration / seed

Create `docker/scraper-providers.yaml` reflecting current reality: `allanime`,
`animefever`, `miruro`, `nineanime` enabled; `animepahe` (Cloudflare/ISS-023) and
`gogoanime` (rebrand) disabled with reasons/descriptions. Once committed +
mounted, the host `docker/.env` `SCRAPER_DEGRADED_PROVIDERS` override can be
removed (the file supersedes it).

## 5. Data Flow (request-time, unchanged)

The failover request path is **unchanged** — disabled providers are simply not
registered, exactly as `SCRAPER_DEGRADED_PROVIDERS` does today. This design only
changes (a) *where* the enable/disable decision is read from and (b) adds the
metrics + dashboard for visibility. No new hot-path latency.

## 6. Error Handling

- Missing file → WARN + env fallback (no outage).
- Malformed YAML or unknown provider name → fail-fast at boot (operator sees it
  immediately; same posture as other scraper config validation).
- Metrics emission is best-effort and never blocks boot.
- Grafana dashboard is provisioned read-only; a JSON error surfaces in Grafana's
  provisioning log, never affecting the scraper.

## 7. Testing

**Go (`services/scraper`):**
- `providers_test.go`: valid file parses; `enabled:false` → `IsEnabled` false;
  unknown name → error; missing optional fields OK; absent provider defaults
  enabled; malformed YAML → error.
- `config_test.go`: file-present path wins; file-absent → env fallback path.
- main.go wiring: a disabled provider is not registered but its
  `provider_enabled`/`provider_info` series are emitted (assert via
  `prometheus/testutil`).

**Dashboard:** JSON validity + schema sanity (the repo's existing dashboard-lint
/ `python -m json.tool` gate, mirroring how other dashboards are checked).

**Manual smoke:** restart scraper with the seed file; confirm `/metrics` shows
`provider_enabled` for all 7 providers (animepahe/gogoanime = 0) and
`provider_info` carries the reasons; open the Grafana dashboard and confirm the
table + timeline render.

## 8. Project Metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +1 (Better)** — operator-facing: a clear table of why/when each provider
  is off replaces an invisible env list. Indirect end-user benefit (faster outage
  recovery); no direct end-user surface change.
- **CDI = 0.03 * 13** — low coherence disruption (additive metrics + a new
  dashboard + a config loader that preserves the env fallback) × medium effort
  (Fibonacci 13: new loader + 2 metrics + dashboard JSON + migration + tests).
- **MVQ = Griffin 85% / 85%** — a tidy, well-bounded observability/operability
  win; reliable and hard to slop (small surface, clear contracts).

## 9. Open questions / future work

- If a richer per-event "why log" is wanted, emit a structured line to Loki on
  each boot (provider state + reason) and add a logs panel — deferred (Loki
  datasource wiring unverified; out of scope here).
- Extending the same pattern to the 5 video players is a possible follow-up.
- A future runtime-toggle (Redis-backed) could layer on top without changing the
  metrics/dashboard contract — the file stays the default source of truth.
