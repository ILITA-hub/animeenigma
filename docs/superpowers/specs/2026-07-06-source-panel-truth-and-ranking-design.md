# Source panel: tell the truth about every provider, rank by real playability

**Date:** 2026-07-06
**Status:** Approved (design)
**Scope:** catalog service (capability feed), library service (encode pipeline), analytics service (ClickHouse), frontend player (aePlayer Source panel)

---

## Problem

The in-player **Source** panel (`frontend/web/src/components/player/aePlayer/`) is fed by the capability feed (`GET /api/anime/{id}/capabilities`), which is the single source of truth for what a title can play. Three things make that feed lie or under-serve, plus the taxonomy that describes it has drifted:

1. **Absent providers vanish silently.** The per-title family builders (`animejoyLegFamily`, `kodikFamily`, `hanimeFamily`, `animelibFamily`) return `ok=false` — the whole family is dropped — when a title has no content on them. So when a title is not on AnimeJoy (e.g. NANA — AnimeJoy genuinely doesn't carry it), the `animejoy-*` chips don't appear **at all**, even in hacker mode. There is no way to see "this provider exists but has nothing for this title." The `ae` family already does the right thing (emits `no_content`, tinted) — the others don't.

2. **`ae` reports a fabricated audio kind.** `aeFamily` builds its variants from `variantsFromTraits(row)` — the **static** `ae` DB row (configured `sub`). So every self-hosted title shows "SUB · 1080p" regardless of the actual uploaded file. Concretely: Black Lagoon (`50310bfd-…`) is an **English dub** in MinIO but the feed labels it `sub`. The encoder **already ffprobes the audio language** (`ffmpeg/transcoder.go` → `ffprobeOutput.AudioLang`, `probedAudioStream`) and then **discards it** — `encoder_worker.go:376` hardcodes `Track: EpisodeTrackRaw` and no audio-language / quality is ever persisted on the `library_episodes` row.

3. **Degraded providers are ranked by name, not reliability.** In hacker mode the panel shows the full list; within the `degraded` bucket, `SourcePanel.vue`'s `sortedRows` tiebreaks on `order` (= `preference_weight`) and then stable input order (the backend query is `ORDER BY name asc`). There is **no** signal from "did this actually play recently" — not globally, not for this specific title, not from the health probe.

4. **The "family" taxonomy is over-segmented and half-dead.** The feed emits eight `family` values (`ae`, `ourenglish`, `adult`, `kodik`, `animelib`, `hanime`, `animejoy-sibnet`, `animejoy-allvideo`) **and** a parallel `group` field (`en`/`ru`/`adult`/`firstparty`) that is the actually load-bearing facet. `family` is functionally near-dead: its only real use is one line — `comboMapping.ts` `family === 'ourenglish' → 'english'`.

## Goal

Make the Source panel an honest, useful, reliability-ranked diagnostic surface, and clean up the taxonomy that describes it:

- **D. Taxonomy** — collapse the wire `family` field to a two-family + standalone model, migrate the one functional use onto `group`.
- **C. `ae` truth** — persist and surface the real audio / language / quality of self-hosted content.
- **A. Presence honesty** — every per-title provider that *could* have content shows up **tinted `no_content`** in hacker mode when it doesn't, with a title-specific tooltip.
- **B. Playability index** — rank degraded providers by a real playability score, and let strong per-title watch evidence **promote** a globally-degraded provider to selectable for that one title.

## Non-goals

- No language badge on chips, and no provider "details popover" (the owner scoped "show provider info" down to *fix `ae` audio accuracy only*).
- No visual family-header grouping in the panel — the panel stays a single flat ranked list. Phase D changes the wire model, not the panel layout.
- No change to the standalone **Classic Kodik iframe** (`KodikPlayer.vue`) surface — it stays outside aePlayer.
- No new admin UI for `ae` metadata — capture is fully automatic (auto-detect at encode).
- No CDN/allowlist changes; no scraper failover-chain changes.

## Decisions (locked)

| # | Decision |
|---|---|
| D1 | Wire `family` collapses to **`"18+"`** (hanime, 18anime), **`"others"`** (EN chain, kodik-HLS, animelib, animejoy-sibnet/allvideo), **`"aeProvider"`** (ae, standalone). |
| D2 | **Keep both Kodik surfaces as separate providers.** In-aePlayer Kodik HLS = a provider inside `"others"` (group `ru`); Classic Kodik iframe = standalone, **not in the feed**. |
| D3 | Migrate the combo-language shortcut from `family === 'ourenglish'` to **`group === 'en'`**; `family` retains no functional role after this. |
| A1 | **All** per-title providers (both `"18+"` and `"others"`) surface as tinted `no_content` in hacker mode when a title has no content on them (today only `ae` does). |
| A2 | The `no_content` tooltip carries a **title-specific reason**, not the generic row reason. |
| C1 | `ae` audio/lang/quality is **auto-detected at encode** (persist ffprobe audio language + output quality; infer dub vs original), with a **backfill** that re-probes existing MinIO titles. No manual admin tagging. |
| C2 | Audio-language → capability mapping: **original/JP audio → `sub` (original)**, **EN audio → `dub` + lang `en`**, **RU audio → `dub` + lang `ru`**. Undetected → fall back to trait (never worse than today). |
| B1 | The playability index re-sorts the **`degraded` bucket only**. Active / no_content buckets keep today's ordering. |
| B2 | Index = **balanced blend** of four decayed inputs: this-anime watch success, global watch success, recent probe-UP, provider health term. |
| B3 | The **this-anime watch** term is dominant and can **promote** a globally-degraded provider to `active` + selectable in **normal** mode *for that title* ("real watch beats coarse canary"). |
| B4 | Watch/UP signals use **exponential time decay (τ ≈ 14 days), no hard count threshold**. A single tunable `PromoteFloor` constant gates the binary promotion decision. |
| B5 | The index is **best-effort**: an analytics outage degrades to the health term only, never blocks or errors the capability feed. |

---

## Design

Phases are independently shippable but sequenced **D → C → A → B** so the foundational model settles before the accuracy/ranking work re-touches the feed shape.

### Phase D — Family taxonomy refactor  *(catalog + FE; behavior-preserving)*

**Backend (`services/catalog/internal/service/capability/`):**
- `buildFamilies` (service.go) currently emits family strings `ae`, `ourenglish`, `adult`, `kodik`, `animelib`, `hanime`, `animejoy-sibnet`, `animejoy-allvideo`. Remap the emitted `SourceFamily.Family` to the D1 values:
  - `hanime`, `18anime` families → `"18+"`.
  - `ourenglish`, `kodik`, `animelib`, `animejoy-sibnet`, `animejoy-allvideo` → `"others"`.
  - `ae` → `"aeProvider"`.
- Multiple providers now share a `family` string (e.g. all EN providers + kodik + animelib + animejoy under `"others"`). The feed's `families` array may therefore either (a) merge into one `SourceFamily` per new label, or (b) keep per-source `SourceFamily` structs that happen to share a label. **Chosen: (a) merge** — one `SourceFamily{Family:"others", Providers:[…all…]}`, one `{Family:"18+", …}`, one `{Family:"aeProvider", …}` — so the wire matches the model. `buildFamilies` gains a small post-assembly regroup step that buckets the built provider caps by target label, preserving the existing stable provider order within each bucket.
- `group` (`en`/`ru`/`adult`/`firstparty`) is **unchanged** — it remains the functional language/relevance facet.

**Frontend:**
- `types/capabilities.ts`: update the `SourceFamily.family` doc/type to `'18+' | 'others' | 'aeProvider'`.
- `composables/aePlayer/comboMapping.ts`: `providerToLegacyPlayer(providerId, family)` → `providerToLegacyPlayer(providerId, group)`, replacing `if (family === 'ourenglish') return 'english'` with `if (group === 'en') return 'english'`. Update `comboToWatchCombo` accordingly.
- `composables/aePlayer/useProviderFeed.ts`: `familyOfProvider` → `groupOfProvider` (returns the provider cap's `group`); callers (`AePlayer.vue:976`) pass `group` instead of `family`.
- Update the affected specs (`useCapabilities.spec.ts`, `useProviderFeed.spec.ts`, `AePlayer.*.spec.ts`) to the new labels.

**kodik-iframe:** no code change. Documented here (and in the player reference) as a standalone surface: the Classic Kodik toggle (`KodikPlayer.vue`) lives on the anime page, is not part of aePlayer, and never appears in the capability feed.

> **Risk note:** `family` is near-dead, so this is a rename + one shortcut migration. The single verification that matters: the audio/lang/provider **combo model is unchanged** (EN chain still resolves to legacy player `english`; kodik/ae/hanime/animelib still key off provider id).

### Phase C — `ae` reports real audio / language / quality  *(library + catalog + FE)*

> **Reality update 2026-07-07** (see `docs/superpowers/plans/2026-07-07-source-panel-phase-c-ae-audio-truth.md`): the 07-06 `library-batchingest -audio-lang` work landed the ffmpeg audio-track *selection* and ingested Black Lagoon S1 (shikimori 889) as English dub via `ae`, but persists nothing (no `audio_lang`/`quality` columns; `Track` stays `raw`; ae still surfaces as "sub"). So the **capture** half is now simpler than "ffprobe auto-detect" below — persist the language the ingest already knows (the `-audio-lang` flag) + set `Track=dub`; the autocache/JP path stays `raw`→original. The **surfacing** half (aeFamily real variants + `ProviderCap.lang`) is unchanged from the design below. Confirmed scope: also ingest Black Lagoon S2 (1519) + OVA (4901) English packs.

**Capture (library):**
- Migration: add two columns to `library_episodes` — `audio_lang TEXT NOT NULL DEFAULT ''` (ISO-639, e.g. `eng`/`jpn`/`rus`) and `quality TEXT NOT NULL DEFAULT ''` (output height label, e.g. `1080p`). Nullable-friendly empty-string defaults so GORM `AutoMigrate` adds them without touching existing rows.
- `domain/episode.go`: add `AudioLang string` + `Quality string` fields (gorm column tags). Leave `Track` semantics alone (still `raw` for storage-layout purposes; audio truth lives in the new columns).
- `service/encoder_worker.go`: when writing the `Episode` row, set `AudioLang` from the ffprobe result the transcoder already computes (`ffprobeOutput.AudioLang` / the mapped `probedAudioStream.Tags.Language`) and `Quality` from the encode's output resolution.
- **Backfill job:** a one-shot (idempotent) library task that iterates `library_episodes` rows with empty `audio_lang`, re-ffprobes the MinIO playlist/first segment, and fills the columns. Best-effort per row; failures leave the row empty (trait fallback). Runs on boot behind a guard flag, or as an admin-triggered internal endpoint.

**Surface (catalog):**
- New library internal endpoint (Docker-network-only, e.g. `GET /internal/library/{shikimoriID}/ae-info`) returning the per-title aggregate: `{present bool, audio_lang string, quality string}` (aggregated across the title's episodes — the common case is uniform; pick the modal/first non-empty).
- Extend `capability.LibrarySource`: replace/augment `HasLibraryTitle(ctx, animeID) (bool, error)` with `AeTitleInfo(ctx, animeID) (AeInfo, error)` where `AeInfo{Present bool; AudioLang, Quality string}`. The catalog `aeLibraryAdapter` (main.go) calls the new endpoint. `HasLibraryTitle` may be kept as a thin `Present`-only wrapper for callers that don't need detail.
- `families_firstparty.go` `aeFamily`: when `AeInfo.Present`, build the variant from real content instead of `variantsFromTraits`:
  - `audio_lang == jpn` (or matches the title's original language) → `Variant{Category:"sub", SubDelivery: <soft if ext subs else hard>, Qualities:[quality], QualitySource:"probed", Source:"discovered"}`, cap `Audios:["sub"]`.
  - `audio_lang == eng` → `Category:"dub"`, cap `Audios:["dub"]`, cap `Lang:"en"`.
  - `audio_lang == rus` → `Category:"dub"`, cap `Audios:["dub"]`, cap `Lang:"ru"`.
  - empty/undetected → today's `variantsFromTraits(row)` fallback.
- **Wire addition for the standalone:** `domain.ProviderCap` (and TS `ProviderCap`) gains an optional `lang string` (`'en'|'ru'|'ja'`). It is the **content language** and is only set for the `aeProvider` standalone (whose `group=firstparty` does not encode a language). For `en`/`ru`/`adult` groups it stays empty and language is derived from `group` as today.

**Frontend:**
- The chip already renders sub/dub + quality via `deriveCapLabels` → shows the truth immediately once the cap is accurate.
- **Combo routing (the integration point that actually fixes Black Lagoon):** wherever the combo model derives a provider's language, use `cap.lang ?? groupToLang(cap.group)`. So `ae` with EN-dub content routes under **DUB → EN**; JP-original `ae` routes under the RAW/sub slider. Verify end-to-end (select the anime, open Source, confirm `ae` sits under DUB/EN and plays), not just the label.

### Phase A — Tint absent per-title providers in hacker mode  *(catalog)*

**Backend:** the per-title family builders in `families_ru.go` (`kodikFamily`, `animelibFamily`, `hanimeFamily`, `animejoyLegFamily`) currently `return domain.SourceFamily{}, false` when they find no content. Change each so that, when its DB row exists and is registered, it emits a **`no_content`** provider cap (built from the row via `applyFeedFields(&cap, row, /*hasContent=*/false)` → `deriveProviderView` returns `no_content, selectable=false, hacker_only=false`) instead of dropping the family. No network content-resolution happens for the absent case (we already know there's nothing) — we only need the DB row, which `providerRow` loads cheaply.
- For `animejoy`, both legs share the resolved (empty) teams; each emits its own `no_content` cap.
- If the DB row is **absent or disabled**, still drop it (nothing to show).

**Tooltip (A2):** in `applyFeedFields`, when `hasContent == false`, set `cap.Reason` to a title-specific message via a new i18n-backed string (e.g. `"Не найдено на {provider} для этого тайтла"` / `"No content for this title on {provider}"`) rather than the generic `row.Reason`. The FE already renders `:title="row.reason"` on the tinted chip.

**Frontend:** no change required — `SourcePanel.visibleRows` already includes `no_content` rows in hacker mode, and `ProviderChip` already renders them tinted (`opacity-40`, "NO CONTENT" badge, `:title` tooltip). Add spec assertions.

### Phase B — Playability index + per-title promotion  *(analytics + catalog + FE)*

**Analytics — CH-derived terms (owns the ClickHouse data):**
- New internal endpoint `GET /internal/playability?anime_id=<id>` (Docker-network-only, **not** gateway-proxied — same posture as `/internal/effects`). Returns per-provider decayed weights:
  ```json
  { "providers": { "gogoanime": { "this_anime_watch": 2.7, "global_watch": 41.3, "recent_up": 6.1 }, … } }
  ```
- New `ClickHouseStore` query method behind it:
  - **watch success** = `events` where `effect_kind='player_resolve'` AND `JSONExtractBool(properties,'reached_playback')=1`, weighted `exp(-dateDiff('day', timestamp, now()) / 14.0)`, `GROUP BY provider` (`target`). `this_anime_watch` adds `WHERE anime_id = {id}`; `global_watch` omits it.
  - **recent_up** = `probe_runs` where `playable=1`, weighted `exp(-dateDiff('day', run_ts, now()) / 14.0)`, `GROUP BY provider`.
  - Provider names filtered to the known roster (reuse the existing whitelist posture).
- Unit tests for the decay expression and the empty-anime / unknown-provider cases.

**Catalog — blend + promotion + attach (the assembler):**
- Small best-effort analytics **read** client (reuses `ANALYTICS_INTERNAL_URL`, short timeout, drop-on-error), mirroring the existing effects **producer** posture. On any failure it returns an empty scores map.
- During report assembly, fetch the scores once per anime (cached alongside the report's 10-min TTL). Compute per provider:
  ```
  healthTerm  = up:+H_UP · recovering:+H_REC · down:−H_DOWN   (from catalog row Health/Policy)
  index = w_a·norm(this_anime_watch) + w_g·norm(global_watch) + w_up·norm(recent_up) + w_h·healthTerm
  ```
  Defaults roughly balanced after normalization; all weights + `H_*` are tunable constants in one file. Attach `index` to each provider cap as **`playability_index float64`** (TS `playability_index: number`).
- **Per-title promotion (B3/B4):** extend `deriveProviderView` to accept the this-anime signal. When `this_anime_watch >= PromoteFloor` for a provider that **has content** for this title, override to `state="active", selectable=true, hacker_only=false` regardless of `Policy==manual`. `PromoteFloor` default ≈ `0.5` (≈ one successful watch within the last week, decayed) — the single boundary that turns the otherwise threshold-free score into a binary state flip. Promotion only applies to has-content providers (a `no_content` provider can't have real watches, so no conflict with Phase A).
- Metric: count promotions (`capability_playability_promotions_total`) and index-fetch failures for observability.

**Frontend (`SourcePanel.vue`):**
- Add `playability_index` to the `ProviderCap` / `ProviderRow` mapping.
- In `sortedRows`, change the within-**`degraded`**-bucket tiebreak from `b.order - a.order` to `b.playability_index - a.playability_index` (then `order` as final tiebreak). `active`/`recovering`/`no_content` buckets unchanged. Promoted providers simply arrive as `active` and sort with the active bucket.
- Spec: degraded rows sort by index; a promoted provider appears active/selectable without hacker mode.

---

## Cross-cutting / integration

- **`aeProvider` is touched by C, D, and B and they compose:** C fills its real audio/lang (+ new `lang` field), D labels its family `"aeProvider"`, B can promote it. The `lang` field added in C is the same one the combo router reads.
- **Graceful degradation is a hard requirement (B5):** analytics down → index = health term only, no promotions, feed still serves. Library down / un-backfilled → `ae` falls back to trait variants. Absent DB row → provider dropped (not tinted). None of these error the feed.
- **Never-worse-than-today invariant:** every fallback path reproduces current behavior, so partial rollout of any phase is safe.

## Testing / verification

- **D:** `capability` service tests assert the three family labels + merged buckets + stable provider order. FE: `comboMapping`/`useProviderFeed` specs assert EN-chain → `english` via `group`, others via id. Manual: combo model unchanged on a known EN title.
- **C:** library encoder test asserts `AudioLang`/`Quality` persisted from a fake ffprobe; backfill test on a seeded empty row; catalog `aeFamily` tests for jpn→sub / eng→dub-en / rus→dub-ru / empty→trait; **e2e on Black Lagoon** (`50310bfd-…`) — `ae` shows EN DUB · real quality and routes under DUB/EN.
- **A:** `providerview`/family-builder tests: absent title → `no_content` cap emitted (not dropped) with title-specific reason; disabled/absent row → still dropped. FE: `SourcePanel`/`ProviderChip` specs for tinted + tooltip in hacker mode. **e2e on NANA** (`e893fa01-…`) — `animejoy-*` appear tinted in hacker mode.
- **B:** analytics query unit tests (decay math, anime filter, empty/unknown); catalog blend + promotion tests with a fake analytics client (success / down / timeout → degrade to health-only); `deriveProviderView` promotion cases; FE `SourcePanel.spec` degraded-sort-by-index + promoted-appears-active.
- Standard gates: `go test ./...` for touched services; `/frontend-verify` (DS-lint + i18n en/ru/ja parity for the new reason string + real `bun run build`) for all FE changes.

## File-touch summary

**Phase D**
- `services/catalog/internal/service/capability/service.go` (regroup to `18+`/`others`/`aeProvider`)
- `frontend/web/src/types/capabilities.ts`, `composables/aePlayer/comboMapping.ts`, `composables/aePlayer/useProviderFeed.ts`, `components/player/aePlayer/AePlayer.vue` + affected specs
- `docs/aeplayer-reference.md` (taxonomy + kodik-iframe standalone note)

**Phase C**
- `services/library/migrations/*` (add `audio_lang`, `quality`), `internal/domain/episode.go`, `internal/service/encoder_worker.go`, new backfill task, new `GET /internal/library/{id}/ae-info` handler + route
- `services/catalog/internal/service/capability/families_firstparty.go` (`aeFamily` real variants), `LibrarySource` iface, `cmd/catalog-api/main.go` (`aeLibraryAdapter.AeTitleInfo`), `domain/capability.go` (+`Lang`)
- `frontend/web/src/types/capabilities.ts` (+`lang`), combo language derivation site(s)

**Phase A**
- `services/catalog/internal/service/capability/families_ru.go` (emit `no_content` for kodik/animelib/hanime/animejoy), `providerview.go`/`applyFeedFields` (title-specific reason)
- `frontend/web/src/locales/{en,ru,ja}.json` (reason key) + `SourcePanel.spec.ts` / `ProviderChip.spec.ts` assertions

**Phase B**
- `services/analytics/internal/repo/clickhouse_store.go` (scores query), `internal/handler/*` + `internal/transport/router.go` (`/internal/playability`)
- `services/catalog/internal/service/capability/{service.go,providerview.go,rank.go}` (blend + promotion), new best-effort analytics read client, `domain/capability.go` (+`playability_index`), metrics
- `frontend/web/src/types/capabilities.ts`, `types/aePlayer.ts`, `components/player/aePlayer/SourcePanel.vue` + spec

## Effort metrics

- **UXΔ = +3 (Better)** — honest labels (`ae` truth), a real diagnostic selector (tinted-when-absent), reliability-ranked fallbacks, and a cleaner model.
- **CDI = 0.10 × 34** — four services touched, but every change is localized and additive; `family` is near-dead, fallbacks preserve current behavior, no shared-state rewrites. (Spread × Shift = 0.10; Effort_Fib = 34; not pre-multiplied.)
- **MVQ = Griffin 88%/85%** — composes existing signals and pipelines; the one novel, slop-prone bit is the per-title promotion logic, which the test plan guards explicitly.
