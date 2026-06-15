# Scraper Capability & Provider-Config API — Design

- **Date:** 2026-06-15
- **Source:** feedback report `2026-06-13T02-19-47_tNeymik_telegram` ("придумать как надёжно
  разобраться со всратыми провайдерами … не понятно у кого есть сабы у кого дабы, кто
  встраивает сабы в видеоряд … нет лесенок качества и меню настроек замусорен провайдерами").
- **Scope:** **backend only** — `services/scraper` + `services/catalog`. The player is
  explicitly OUT of scope ("dont touch player yet"); this spec produces the *assembled,
  ranked capability data* the player will later render.
- **Status:** approved design → spec.

## Problem

The OurEnglish player exposes a raw, cluttered Source dropdown listing every EN scraper
provider, and the backend gives the player no way to know, per title:

1. **What each provider actually offers** — sub / dub / raw, whether subtitles are *soft*
   (selectable track) or *burned into the picture* (hardsub), and which qualities.
2. **Which provider to prefer** — no server-side ranking, so the menu is a flat,
   confusing list of "всратые провайдеры".

Today: `Server.Type` carries `sub|dub|raw`, but only **gogoanime** and **animepahe** parse
category — every other provider defaults every server to `sub`. `Source.Quality` is
optional and inconsistent. There is no aggregated "who has what, best first" answer in a
single call. Provider config lives in `docker/scraper-providers.yaml` (edit + restart).

## Goals

- A single catalog endpoint returns an **assembled, ranked capability report** per anime
  (episode-aware) across source families, so future player work is *render-only*.
- Honest **sub/dub/raw + soft-vs-burned-in + quality** metadata, via a **hybrid** model:
  curated static traits (instant baseline + declutter) enriched by what live discovery
  already found for the title (no fresh fan-out).
- **Provider is the rankable unit** for EN (real translation-team names deferred —
  `.planning/backlog/SCRAPER-EN-dub-studio-names.md`). RU (Kodik/AniLib) keep their real
  teams; Hanime included. Raw/JP out.
- **Migrate `scraper-providers.yaml` config into a database** (catalog's Postgres), DB as
  runtime source of truth, YAML retained as seed + offline fallback.

## Non-Goals

- No player/frontend changes (separate later effort).
- No admin CRUD UI/API this phase (DB seed only; edited via SQL/migration — admin surface
  is a documented follow-up).
- No EN translation-team / dub-studio name extraction (backlog
  `SCRAPER-EN-dub-studio-names`).
- No new YAML file — extend the existing records, then migrate them.
- No eager cross-anime pre-resolution / background warmer (documented future optimization).

## Locked Decisions (from brainstorm)

| # | Decision |
|---|----------|
| D1 | Capability source = **hybrid** (static traits + opportunistic live enrich; no fresh fan-out on the hot path). |
| D2 | Coverage = **EN scraper providers + normalized labeling for Kodik / AniLib / Hanime**. Raw/JP excluded. |
| D3 | **Provider is the rankable unit** for EN; `Variant.Team` reserved (nil for EN), real for RU. |
| D4 | Burned-in (hardsub) status is a **curated trait**, not runtime-detected; flipped to `soft` per-title when discovery finds soft tracks. |
| D5 | Provider+capability config lives in **catalog's Postgres**; scraper fetches via internal API at boot + periodic refresh; YAML = offline fallback. |
| D6 | Maintenance this phase = **DB seed only** (from YAML); admin CRUD deferred. |
| D7 | **Improve live category detection for ALL providers** (not just the two that parse today). |
| D8 | Assembled endpoint granularity = **per-anime, episode-aware**. |
| D9 | Assembled + ranked endpoint lives in **catalog** (the aggregator). |

## Architecture

```
                       ┌─────────────────────────────────────────────┐
  Player (later) ──────│ GET /api/anime/{uuid}/capabilities?episode=  │
                       └───────────────┬─────────────────────────────┘
                                       │ gateway /api/anime/* → catalog (no gw change)
                       ┌───────────────▼───────────────┐
                       │ catalog: CapabilityService     │
                       │  - assemble + RANK across families
                       │  - adapters: kodik / animelib / hanime
                       │  - Redis cache  capabilities:<anime>:<ep?>
                       └───┬───────────────────────┬───┘
        internal (docker)  │                       │ GET /scraper/capabilities
   GET /internal/scraper/  │                       ▼
        providers  ◄───────┤            ┌──────────────────────────┐
                           │            │ scraper: CapabilityHandler│
   ┌───────────────────────▼──┐         │  traits (from cfg) + live │
   │ catalog Postgres          │         │  health/playable/discovered│
   │  table scraper_providers  │         └──────────┬───────────────┘
   │  (seeded from YAML)       │                    │ reads failover/probe cache
   └───────────────────────────┘                    │ + already-discovered servers
            ▲ boot + ~60s refresh                    ▼ (NO fresh fan-out — D1)
            └──────────── scraper config loader ──── providers/* (improved category parse)
                          (YAML fallback if catalog down)
```

## Data Model

### Scraper-side capability payload (`services/scraper/internal/domain`)

```go
type ProviderCapability struct {
    Provider         string  `json:"provider"`
    Enabled          bool    `json:"enabled"`
    Group            string  `json:"group"`            // "en" | "adult"
    // static traits (from DB-migrated config):
    SupportsSub      bool    `json:"supports_sub"`
    SupportsDub      bool    `json:"supports_dub"`
    SupportsRaw      bool    `json:"supports_raw"`
    SubDelivery      string  `json:"sub_delivery"`     // "soft" | "hard" | "none" (default claim)
    QualityCeiling   string  `json:"quality_ceiling"`  // e.g. "1080p"
    PreferenceWeight int     `json:"preference_weight"`
    // live signals:
    Health           string  `json:"health"`           // "up" | "down" | "unknown"
    Playable         *bool   `json:"playable,omitempty"`
    Discovered       []DiscoveredVariant `json:"discovered,omitempty"` // confirmed for THIS title
}

type DiscoveredVariant struct {
    Category    string   `json:"category"`     // "sub" | "dub" | "raw"
    SubDelivery string   `json:"sub_delivery"` // "soft" if soft tracks found, else trait default
    Qualities   []string `json:"qualities,omitempty"`
    QualitySource string `json:"quality_source"` // "hls_master" | "discrete" | "unknown"
}
```

New endpoint:
`GET /scraper/capabilities?mal_id=&title=&title_alt=&episode=` →
`{ "success": true, "data": { "providers": []ProviderCapability, "meta": {"granularity":"anime|episode"} } }`.
Enrichment reads the failover/probe health cache + servers already discovered for the
title; it does **not** call providers it hasn't already touched (D1).

### Catalog-side unified report (`services/catalog/internal/domain` or `service/capability`)

```go
type CapabilityReport struct {
    AnimeID  string         `json:"anime_id"`
    Episode  *int           `json:"episode,omitempty"`
    Families []SourceFamily `json:"families"`
}

type SourceFamily struct {
    Family    string        `json:"family"`    // "ourenglish" | "kodik" | "animelib" | "hanime"
    Providers []ProviderCap `json:"providers"` // ranked best-first within family
}

type ProviderCap struct {
    Provider    string    `json:"provider"`
    DisplayName string    `json:"display_name"`
    Enabled     bool      `json:"enabled"`
    Health      string    `json:"health"`            // up | down | unknown
    Playable    *bool     `json:"playable,omitempty"`
    Rank        float64   `json:"rank"`
    Variants    []Variant `json:"variants"`
}

type Variant struct {
    Category      string   `json:"category"`       // sub | dub | raw
    Team          *Team    `json:"team,omitempty"` // RU only; nil for EN (reserved — backlog)
    SubDelivery   string   `json:"sub_delivery"`   // soft | hard | none
    Qualities     []string `json:"qualities,omitempty"`
    QualitySource string   `json:"quality_source"` // hls_master | discrete | unknown
    Source        string   `json:"source"`         // "trait" | "discovered" (provenance)
}

type Team struct {
    ID   string `json:"id,omitempty"`
    Name string `json:"name"`
}
```

Public endpoint: `GET /api/anime/{uuid}/capabilities?episode=` (gateway `/api/anime/*` →
catalog, **no gateway change**). Internal: `GET /internal/scraper/providers`
(Docker-network-only, not gateway-proxied).

## Provider Config DB Migration

### Table `scraper_providers` (catalog Postgres, GORM)

| column | type | notes |
|--------|------|-------|
| `name` | text PK | gogoanime, animepahe, … |
| `enabled` | bool | failover participation |
| `group` | text | `en` \| `adult` (intrinsic, validated) |
| `reason` | text | short, for dashboard |
| `description` | text | full why |
| `supports_sub` | bool | trait |
| `supports_dub` | bool | trait |
| `supports_raw` | bool | trait |
| `sub_delivery` | text | `soft` \| `hard` \| `none` (default claim) |
| `quality_ceiling` | text | `1080p` etc |
| `preference_weight` | int | ranking bias |
| `updated_at` | timestamptz | audit |

### Seeder (one-time, idempotent upsert)

Reads `docker/scraper-providers.yaml` for `name/enabled/group/reason/description`; seeds the
trait columns from the initial mapping below. **Insert-if-absent only (idempotent): a row
that already exists is never overwritten** — so operator edits in the DB survive re-seeding.
DB is runtime source of truth; YAML stays as seed + offline fallback.

### Initial trait mapping (best-guess; refined per-title by P2 live discovery)

EN CDN "sub" streams are typically **burned-in** — hence `sub_delivery: hard` as the default
claim, flipped to `soft` per-title when discovery finds soft tracks (directly answers the
"кто встраивает сабы в видеоряд" complaint).

| provider | grp | sub | dub | raw | sub_delivery | quality_ceiling | weight | note |
|----------|-----|-----|-----|-----|--------------|-----------------|--------|------|
| allanime | en | ✓ | ✓ | – | hard (soft-capable) | 1080p | 90 | direct MP4, reliable |
| gogoanime | en | ✓ | ✓ | – | hard | 1080p | 85 | megaplay HLS; parses `-dub` |
| miruro | en | ✓ | ✓ | – | hard | 1080p | 70 | per-inner-provider sub/dub |
| animefever | en | ✓ | ? | – | hard | 1080p | 60 | tserver only |
| nineanime | en | ✓ | ? | – | hard | 720p | 40 | last-resort |
| animepahe | en | ✓ | ✓ | – | hard | 1080p | 30 | disabled (Cloudflare, ISS-023) |
| animekai | en | ✓ | ? | – | hard | 1080p | 0 | stub (ListServers unimplemented) |
| 18anime | adult | ✓ | – | ✓ | hard | 1080p | n/a | adult group, separate orchestrator |

`?` = unknown at seed time → seeded `false`, set `true` if P2 live discovery confirms.

### Scraper config loader change

Replace the boot-time YAML read with: fetch `GET catalog/internal/scraper/providers` at
boot; cache in memory; refresh every ~60s (so enable/disable no longer needs restart). If
catalog is unreachable at boot, fall back to the bundled `scraper-providers.yaml`. Unknown
provider name still fails boot (preserve current guard). Boot order is safe — catalog does
not depend on scraper at boot.

## Auto-Discovery & Parsing (per-provider category improvements — D7)

`ListServers` already auto-discovers servers; the work is making `Server.Type` (category)
honest across all providers, plus capturing qualities + soft-track presence:

| provider | category signal to parse | quality signal |
|----------|--------------------------|----------------|
| gogoanime | slug `-dub` suffix (exists) | HLS master → `hls_master` |
| animepahe | `data-audio` eng/jpn (exists) | button text "720p·…" → discrete |
| allanime | GraphQL `translationType` (sub/dub); soft `tracks` → `sub_delivery=soft` | discrete MP4 qualities |
| miruro | per-inner-provider sub/dub map | upstream quality labels |
| nineanime | dub embed presence (if any) | HLS master |
| animefever | audio indicator on embed | HLS master |
| 18anime | raw/sub from mirror metadata | mirror `quality` regex (exists) |

For HLS providers the real ladder lives in the master playlist (`quality_source=hls_master`,
read at play time); the capability layer reports the trait `quality_ceiling` + any discrete
qualities discovered. Hardsub is never auto-detected (D4).

### RU/Hanime adapters (catalog-side, map existing parser data into the unified shape)

- **Kodik** → family `kodik`; `Variant.Team` from `Translation.Title`+`ID`; `type==voice`→`dub`,
  `type==subtitles`→`sub`; iframe → `sub_delivery=hard`/`unknown`, `quality_source=unknown`.
- **AniLib** → family `animelib`; `Variant.Team` from `Team{ID,Name}`; `translation_type`
  voice→`dub`/subtitles→`sub`; parser already distinguishes external ASS/VTT (`soft`) vs
  burned-in (`hard`) per team — feed that into `sub_delivery`; discrete MP4 qualities.
- **Hanime** → family `hanime`; no teams; `sub` + `sub_delivery=hard`; HLS qualities.

## Ranking (provider = unit, per category, within family)

Computed server-side in catalog:

```
score = preference_weight
      + health(up:+0  | down: EXCLUDE)
      + playable(true:+25 | false:-25 | unknown:0)
      + quality_ceiling_scaled(1080p:+15, 720p:+8, …)
      + sub_delivery(soft:+10 | hard:-5 | none:0)   // soft is better UX
      + discovered_for_title:+20                     // we know it actually has content
tiebreak: provider name asc  (deterministic, stable order)
```

Weights are constants in code this phase (tunable later via the deferred admin surface).
Output: providers ranked best-first within each family; top entry is the natural default.

## Caching & Error Handling

- Redis `capabilities:<animeID>:<episode|_>` — short TTL (~10 min). **Degraded results
  (any provider down/unknown) cached short (~60s)** so a transient failure self-heals
  (ISS-019 lesson). Provider-config cache key `scraper:providers:config`.
- A provider/family that errors is reported with `health=down|unknown` and empty/trait-only
  variants — it never fails the whole report (graceful degradation).
- Cache-shape change ⇒ flush `capabilities:*` same-day after deploy
  (`feedback_spotlight_cache_shape_migration` lesson) and runtime-smoke the live endpoint.

## Testing

Go unit tests, **handwritten fakes (no testify/mock — project convention)**:

- trait + discovered **join** logic (discovered overrides trait per title).
- **ranking** ordering (health gate, playable boost/penalty, soft>hard, discovered boost,
  name tiebreak).
- per-provider **category parsers** — fixture HTML/JSON table tests for each improved provider.
- **seeder** YAML→DB upsert (idempotent; existing rows untouched).
- scraper **config fetch** + **YAML fallback** when catalog unreachable.
- catalog **cross-family assembly** with fakes (kodik/animelib/hanime adapters + scraper).

No player/e2e (out of scope). `go test ./...` in both services; `go vet`.

## Phasing & Effort

Per `.planning/CONVENTIONS.md` (no time units).

### P1 — Provider config → DB + scraper fetch-with-fallback
Table + GORM model + migration + YAML seeder; catalog `GET /internal/scraper/providers`;
scraper config loader fetch + ~60s refresh + YAML fallback; trait columns added (seeded).
- **UXΔ** = 0 (Neutral) — internal plumbing, no user-visible change yet.
- **CDI** = 0.03 * 13
- **MVQ** = Griffin 80%/85%

### P2 — Per-provider category + quality parsing (D7)
Honest `Server.Type` across all providers; capture qualities + soft-track presence;
`DiscoveredVariant` plumbing.
- **UXΔ** = +1 (Better) — backend accuracy; surfaces once P3/P4 land.
- **CDI** = 0.04 * 21
- **MVQ** = Kraken 80%/75%

### P3 — Scraper `/scraper/capabilities` join endpoint
Traits (from config) + live health/playable/discovered, hybrid (no fresh fan-out).
- **UXΔ** = +1 (Better)
- **CDI** = 0.03 * 13
- **MVQ** = Griffin 85%/80%

### P4 — Catalog assembled + ranked `/api/anime/{uuid}/capabilities`
Unified `CapabilityReport` across families incl. Kodik/AniLib/Hanime adapters + ranking +
caching.
- **UXΔ** = +3 (Better) — the assembled, ranked, declutter-ready data the feedback asks for.
- **CDI** = 0.05 * 21
- **MVQ** = Phoenix 85%/80%

## Open Risks

- **Boot dependency:** scraper now reads catalog at boot. Mitigated by bundled-YAML
  fallback; unknown-name guard preserved.
- **Trait drift:** seeded `?` dub flags may be wrong until P2 discovery confirms — acceptable
  (discovered overrides trait per title).
- **Hardsub default:** `sub_delivery=hard` default may mislabel a soft-sub provider until P2
  flips it per-title; conservative and matches the dominant EN reality.

## Cross-References

- Backlog (deferred EN team names): `.planning/backlog/SCRAPER-EN-dub-studio-names.md`
- Provider config + header docs: `docker/scraper-providers.yaml`,
  `services/scraper/internal/config/providers.go`
- Failover/health: `services/scraper/internal/service/orchestrator.go`,
  `services/scraper/internal/health/`
- Trust model (signed stream URLs, no new allowlist entries): CLAUDE.md "Video Player
  Architecture"; `libs/videoutils/proxy.go`
- RU team models to mirror: `services/catalog/internal/parser/kodik/client.go`
  (`Translation`), `services/catalog/internal/parser/animelib/client.go` (`Team`)
- Lessons applied: ISS-019 (degraded-result short TTL),
  `feedback_spotlight_cache_shape_migration` (flush on shape change)
- Source feedback: `2026-06-13T02-19-47_tNeymik_telegram`
