# Playback-Health Provider Roster Unification — Design

**Date:** 2026-06-17
**Status:** Approved (design); pending implementation plan
**Owner:** project owner (0neymik0)
**Related:** `docker/grafana/dashboards/playback-health.json`, [[project_scraper_provider_config_db]], [[project_retire_all_players_except_aeplayer]], [[project_aeplayer_rename_and_deterministic_best]]

## Scoring (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — operator-facing observability; users unaffected directly, but faster failure diagnosis shortens playback outages.
- **CDI = 0.06 * 21** — Spread across catalog + scraper + scheduler + analytics + Grafana provisioning (wide); Shift moderate (rename + new column + status-gated emit path touches boot/migration/failover-consume code). Effort_Fib = 21.
- **MVQ = Griffin 88%/85%** — structural consolidation onto a single roster; high coherence payoff, low slop once the status-gated framework lands.

## Problem

The `playback-health` Grafana dashboard shows a **different, inconsistent subset of providers in every panel** ("Parser p95 shows 4–5, Probe last tick 6–7, ae has its own bespoke table, telemetry ~5"). Root cause is architectural, not per-panel bugs:

1. **No roster spanning all stream sources.** `scraper_providers` (catalog Postgres) is a real single source of truth, but only covers the **8 EN + adult scraper providers**. The legacy players (kodik, animelib, hanime, raw) and the first-party **ae** are registered ad-hoc in `services/catalog/internal/service/health_checker.go`, not in the table.
2. **Every metric is emitted from a different code path covering a different subset, none reconciled against the roster:**
   - `parser_request_duration_seconds` (Parser p95) — only catalog-*internal* parsers call `metrics.ObserveParser()` (kodik, animelib, hanime, 18anime, allanime). The EN chain runs in the scraper service and is never instrumented here.
   - `provider_probe_last_tick_timestamp` / `provider_health_up` — only providers in the scraper orchestrator's probe loop (`services/scraper/cmd/scraper-api/main.go` ~556). Adult orchestrator + catalog-side providers excluded.
   - ClickHouse `events.provider` telemetry — only services that wire the egress producer; catalog-internal parsers don't tag a provider.
   - ae — a bespoke `http_requests_total{path="/api/anime/:id/ae"}` panel, registered nowhere as a provider.
3. **No shared framework** forces emitters or dashboard panels to enumerate the roster. The `$provider` template var is computed from `label_values(provider_health_up, provider)` — i.e. from *one* of the inconsistent metrics, so the dropdown and panels disagree.

## Strategic context

Owner decision (2026-06-17): **retire all video players except aePlayer.** Kodik/AniLib/Hanime/Raw are on the way out; ae is the survivor and warrants first-class instrumentation. This refactor builds the roster + dashboard that tracks that transition. Retirement is modeled by `status=disabled`, never row deletion.

## Goals

1. **One roster for every stream source** — rename `scraper_providers` → `stream_providers`; it holds ae, the EN scraper chain, adult, and the legacy players.
2. **A shared, status-driven instrumentation framework** — every owning service emits per-provider metrics by iterating the roster, gated by `status`. No service keeps a hardcoded provider list.
3. **A unified dashboard** — every panel keyed off the `provider` label with no hardcoded subsets; one status table; alerts only for `enabled` providers; the bespoke ae panel retired.

## Non-goals

- Actually decommissioning the legacy players (separate effort; here they just become `disabled` rows when the time comes).
- Changing failover ranking logic or the watch-combo resolver.
- Rebuilding the ClickHouse telemetry schema (it's already open-ended; we only ensure uniform `provider` tagging).

## Design

### 1. Data model — `stream_providers`

Rename the table and extend the model (`services/catalog/internal/domain/scraper_provider.go` → `stream_provider.go`).

- **Rename** via a **guarded one-time SQL migration** in `services/catalog/cmd/catalog-api/main.go`, mirroring the existing 2026-06-17 status-enum migration block:
  - `ALTER TABLE IF EXISTS scraper_providers RENAME TO stream_providers;` — idempotent (guard on existence of old table / absence of new).
  - GORM `AutoMigrate(&domain.StreamProvider{})` then adds the new column.
- **New column** `ScraperOperated bool` (`gorm:"default:false"`, json `scraper_operated`):
  - `true` — operated by the scraper microservice: gogoanime, animepahe, allanime, animefever, miruro, nineanime, animekai, 18anime.
  - `false` — catalog-internal / first-party: ae, kodik, animelib, hanime, raw.
- **Seed** (`services/catalog/internal/service/scraperprovider/seed.go`) extended with the legacy + first-party rows (insert-if-absent, so operator edits survive): `ae` (firstparty, enabled), `kodik` (ru, enabled), `animelib` (ru), `hanime` (adult), `raw`/`allanime-raw` (jp). Existing 8 rows get `scraper_operated=true`.
- `group` stays a display/ordering hint; extend the value set as needed (en/adult/ru/jp/firstparty).

**Critical correctness point — scraper consumer filter.** The scraper fetches the roster from `/internal/scraper/providers` (`services/scraper/internal/config/providers_remote.go`) and validates names against `KnownProviders` (`services/scraper/internal/config/providers.go`), failing fast on unknowns. The consumer **must filter to `scraper_operated=true` before validation**, otherwise the new ae/kodik/etc. rows break scraper boot. This filter is also what guarantees EN failover never tries to fail over into ae/kodik/a first-party source.

`/internal/scraper/providers` (`internal_scraper_providers.go`) keeps returning all rows (the scraper filters client-side); `BuildENFamily` (`service/capability/service.go`) keeps its existing `status <> 'disabled' AND group='en'` filter and is unaffected by the rename beyond the model name.

### 2. Shared instrumentation framework (status-driven)

Extend `libs/metrics/provider.go` into a single roster-driven emit helper that each owning service calls on boot + on roster refresh:

- **catalog** owns first-party + RU/JP/18+ internal parsers (ae, kodik, animelib, hanime, raw).
- **scraper** owns the EN chain + adult.

Each service iterates **only the roster rows it owns** and, **gated by `status`**, decides registration:

| `status` | roster reflection | health/probe + parser + telemetry | alerts |
|---|---|---|---|
| `enabled` | yes | emit | yes |
| `degraded` | yes | emit | **no** |
| `disabled` | yes (roster only, see §3a) | **none emitted** | no |

Concrete parity work ("full parity for everyone" while they live):
- **Probes/health** — add health probing for the catalog-side providers that today have none (ae, kodik, animelib, hanime, raw), extending the existing catalog `PlayerHealthChecker`/liveness pattern so they emit `provider_health_up` + `provider_probe_last_tick_timestamp`. Scraper-side probe loop already covers the EN chain; extend to cover adult (separate orchestrator) so 18anime gets ticks.
- **Parser latency/success** — ensure the scraper emits `parser_request_duration_seconds` / `parser_requests_total` per EN-chain provider (today only catalog-internal parsers do). Legacy players keep their existing catalog emit. ae gets new emit.
- **Telemetry** — every resolve path tags `provider` with the canonical roster slug; ae resolves emit `player_resolve` with `target=ae`, retiring the bespoke `/api/anime/:id/ae` http-path measurement.
- **Disabled rows emit nothing** — the emit helper skips metric/probe/alert registration entirely for `status=disabled`.

### 3. Dashboard refactor (`docker/grafana/dashboards/playback-health.json`)

- **Template vars:** `$provider` sourced from the full roster (see §3a). Add `$status`, `$group`, `$scraper_operated` multi-select filters. Remove the dependence on `label_values(provider_health_up, …)`.
- **Per-provider panels:** every panel keyed off the `provider` label, no hardcoded regex/path subsets. Parser p95/success, probe last tick, connect/disconnect history, telemetry resolve/stall/fail panels all enumerate the same roster.
- **Unified status table** replaces the scattered subset panels **and** the ae-specific panel: one row per provider with status, group, scraper_operated, last-probe age, parser success %, p95, telemetry resolve %.
- **Alerts** fire only for `enabled` rows (degraded/disabled excluded) — encode the `status` gate in alert queries/labels.

#### 3a. How `disabled`/retired rows appear — **(a) Postgres datasource**

Add a **Grafana Postgres datasource** (provisioned alongside the existing Prometheus + ClickHouse datasources under `docker/grafana/`) pointed at the catalog DB read-only. The **roster/status table panel queries `stream_providers` directly** — so disabled rows list from the DB with their status, while the live-metric panels query Prometheus/ClickHouse and naturally show data only for enabled+degraded providers (which are the only ones emitting). This cleanly honors "no metrics for disabled": disabled providers appear in the roster table but have no live-metric series anywhere.

`$provider` / `$group` / `$status` template vars are sourced from the Postgres roster query too, so the dropdowns reflect the authoritative roster rather than a metric's label cardinality.

> Fallback if the Postgres datasource proves undesirable: **(b)** emit a roster-reflection-only `provider_info` gauge for all rows (incl. disabled) from Prometheus and keep everything else status-gated. Recorded here as the contingency; primary plan is (a).

### 4. Migration & rollout safety

- Rename migration is idempotent and guarded (old-table-exists / new-table-absent), runs once on catalog boot.
- Scraper consumer filters `scraper_operated=true` **before** `KnownProviders` validation (§1) — verify scraper boots green against the enlarged roster.
- Seed is insert-if-absent; operator SQL edits to status/scraper_operated survive redeploys.
- Cardinality: new providers add a bounded handful of label values; canary metric (`playability_canary_runs_total`) stays scraper-only and is unaffected.
- Roll out backend (catalog + scraper + scheduler + analytics) before the dashboard JSON + datasource provisioning, so the new metrics exist when the panels query them.

## Affected components

| Area | Files |
|---|---|
| Domain model + rename | `services/catalog/internal/domain/scraper_provider.go`, `services/catalog/cmd/catalog-api/main.go` (migration + AutoMigrate) |
| Seed | `services/catalog/internal/service/scraperprovider/seed.go` |
| Roster endpoint / capability | `services/catalog/internal/handler/internal_scraper_providers.go`, `services/catalog/internal/service/capability/service.go` |
| Scraper consume + validate | `services/scraper/internal/config/providers_remote.go`, `services/scraper/internal/config/providers.go`, `services/scraper/cmd/scraper-api/main.go` |
| Shared metrics framework | `libs/metrics/provider.go`, `libs/metrics/parser.go` |
| Catalog-side health/probe | `services/catalog/internal/service/health_checker.go` |
| Scraper-side probes | `services/scraper/internal/health/probe.go`, scraper main probe loop |
| Telemetry tagging | resolve paths in catalog/scraper; `services/analytics/internal/repo/clickhouse_schema.go` (no schema change, verify only) |
| Dashboard + datasource | `docker/grafana/dashboards/playback-health.json`, `docker/grafana/` datasource provisioning |

## Testing

- Catalog: unit test the rename migration is idempotent (run twice → one `stream_providers`, data preserved); seed insert-if-absent test incl. new rows + `scraper_operated`.
- Scraper: test the remote-roster consumer filters `scraper_operated=true` and boots green; unknown-name validation still fires for genuinely-unknown scraper rows.
- Metrics framework: table-driven test of the status gate (enabled→emit+alert, degraded→emit/no-alert, disabled→nothing).
- Dashboard: lint the JSON; verify every per-provider panel uses the `provider` label and template vars; verify the roster table panel uses the Postgres datasource.
- Manual: after deploy, every per-provider panel shows the same row set; disabled rows appear in the roster table with no live series.

## Open questions

None blocking. Contingency (3b) documented if the Grafana Postgres datasource is rejected during implementation.
