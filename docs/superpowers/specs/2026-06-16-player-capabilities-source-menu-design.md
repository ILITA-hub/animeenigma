# Player: wire the Source menu to /capabilities

**Date:** 2026-06-16
**Status:** Design approved, pre-plan
**Scope:** Frontend only (`frontend/web/`). Backend `GET /api/anime/{uuid}/capabilities` already shipped (scraper-capability effort P1–P4b, origin/main `80b4533a`).
**Effort metrics:** UXΔ = +3 (Better) · CDI = 0.04 * 21 · MVQ = Griffin 85%/80%

---

## 1. Goal

Turn the assembled, ranked `GET /api/anime/{uuid}/capabilities` report into the player's
Source-menu behaviour, delivering the three things the original feedback
(`2026-06-13T02-19-47_tNeymik_telegram`) asked for and the owner confirmed:

1. **Server-driven ranking** — order providers by the endpoint's live `rank`
   (health + playability + quality + sub-delivery aware) instead of the static
   hand-maintained `CURATED_TIER` array.
2. **Declutter via collapse** — the default (non-hacker) view shows only the
   single best **playable** source for the active audio/lang; the rest are hidden
   behind hacker mode, with a lightweight "Try another source" escape hatch on
   playback failure.
3. **Sub/dub/quality/team labels** — each provider chip is decorated with its
   category tags (`SUB`/`DUB`/`RAW`), quality ceiling, and a subtitle-delivery
   hint (burned-in vs selectable); team chips gain a per-team `SUB`/`DUB` tag.

**Non-goals.** No backend changes. No change to the failover resolver
(`useProviderResolver`), the scraper microservice, or the streaming proxy. No
removal of the deprecated per-provider player SFCs (`OurEnglishPlayer.vue` etc.) —
out of scope; the live surface is `UnifiedPlayer.vue`. No EN dub-studio team-name
crowd-sourcing (parked backlog: `.planning/backlog/SCRAPER-EN-dub-studio-names.md`).

---

## 2. Current state (verified by exploration)

The live player is `components/player/unified/UnifiedPlayer.vue`. Its source UI is
the chip-based `unified/SourcePanel.vue` (NOT the deprecated `OurEnglishPlayer.vue`
native `<select>`). Relevant anchors:

- **Provider rows** come from `composables/unifiedPlayer/useProviderHealth.ts`
  (`computeProviderRows(scraperHealth, filter)` — registry + live `/scraper/health`
  poll every 30s + audio/lang/content relevance → `ProviderRow{ def, state, reason }`,
  `state ∈ active|disabled|down|irrelevant|wip`).
- **Ordering / smart default** = `composables/unifiedPlayer/smartDefault.ts`
  `pickSmartDefault(rows, CURATED_TIER, { needsCheck, isAvailable })`, walking the
  static `CURATED_TIER` array in `unified/providerRegistry.ts`.
- **State** = `composables/unifiedPlayer/usePlayerState.ts` — `combo` ref
  `{ audio, lang, provider, server, team }`; **`hackerMode` ref already exists**
  (persisted `localStorage 'pl_hacker_mode'`, currently gates only `overlays/DebugHud.vue`,
  toggled in `unified/PlaybackSettingsMenu.vue`).
- **Types** = `types/unifiedPlayer.ts` (`ProviderDef`, `ProviderRow`, `Combo`,
  `StreamResult`).
- **API** = `api/client.ts` (`scraperApi`, `kodikApi`, `animeLibApi`, …). No
  capabilities client yet.
- **Chips** = `unified/ProviderChip.vue` (name + identity-hue dot + state badge),
  `unified/SourcePanel.vue` team-chip section (lines ~88–110).
- **DS constraint:** player components are EXEMPT from the `Select`-primitive lint
  rule (fullscreen native pickers); brand/provider hues (cyan/orange/pink/rose) are
  allowlisted. Bind colors to tokens otherwise.
- **i18n:** player keys under the `player.*` namespace in `locales/{en,ru,ja}.json`.
  All three locales are redeploy-gated (`i18n-lint.sh` hard-fails `make redeploy-web`).

**Coverage gap (important):** `/capabilities` families are `ourenglish`, `kodik`,
`animelib`, `hanime`. The registry also has `ae` (first-party), `raw` (JP AllAnime),
and `18anime` (adult EN) — these have **no** capability entry and must keep working
off the existing registry/health path.

---

## 3. Approach (chosen: enrichment layer)

Capabilities is an **enrichment + ranking + collapse layer over the existing
pipeline**, not a replacement. `useProviderHealth` stays the source of provider rows
and their state machine. A new `useCapabilities` composable yields a
`Map<providerId, ProviderCap>`. `SourcePanel` merges the two: sorts by server `rank`,
decorates chips with capability labels, applies the collapse. If `/capabilities`
fails or is empty, the map is empty and everything degrades to exactly today's
behaviour (`CURATED_TIER` order, no extra labels). Rejected alternatives: capabilities
as the row source (doesn't cover `ae`/`raw`/`18anime`, loses the health state
machine — big rewrite); a new backend bootstrap endpoint (out of scope).

---

## 4. Components & data flow

```
GET /api/anime/{id}/capabilities
        │  capabilitiesApi.get(id)            (api/client.ts — new)
        ▼
useCapabilities(animeId)                       (composable — new)
   → report: CapabilityReport | null
   → capMap: Map<providerId, ProviderCap>      (flatten families→providers)
   → rankedIds: string[]                        (providers by rank desc)
   → loaded: boolean / error: boolean
        │
        ├──────────────► rankedProviderIds(rows, rankedIds, CURATED_TIER)   (new pure fn)
        │                   capability-ranked first, registry-only appended in CURATED order
        │                       │
        │                       ├─► pickSmartDefault(rows, rankedProviderIds, …)   (existing, new arg)
        │                       └─► SourcePanel row sort
        │
        └──────────────► ProviderChip label props (variants → tags/quality/sub-delivery)
                          SourcePanel team-chip tags (kodik/animelib variant.category)
```

`UnifiedPlayer.vue` owns the wiring: calls `useCapabilities(animeId)` alongside
`useProviderHealth`, computes `rankedProviderIds`, passes `capMap` + `hackerMode` +
ranked order into `SourcePanel`, and feeds `rankedProviderIds` to the existing
`pickSmartDefault` watcher.

### 4.1 New TypeScript types (`types/capabilities.ts`)

Mirror the Go shapes verbatim (snake_case JSON):

```ts
export interface CapabilityReport { anime_id: string; families: SourceFamily[] }
export interface SourceFamily { family: string; providers: ProviderCap[] }
export interface ProviderCap {
  provider: string; display_name: string; enabled: boolean;
  health: 'up' | 'down' | 'unknown'; playable?: boolean;
  rank: number; variants: Variant[];
}
export interface Variant {
  category: 'sub' | 'dub' | 'raw';
  team?: { id?: string; name: string };
  sub_delivery: 'soft' | 'hard' | 'none';
  qualities?: string[];
  quality_source: 'hls_master' | 'discrete' | 'unknown' | 'trait';
  source: 'trait' | 'discovered';
}
```

### 4.2 `useCapabilities(animeId: Ref<string>)`

- Fetches once per anime id (watch + in-memory cache keyed by id); shares no state
  with the health poller.
- `capMap`: flatten every `family.providers[]` into `Map<provider, ProviderCap>`.
  Provider ids in the report already match registry ids (`gogoanime`, `allanime`,
  `miruro`, `animepahe`, `animefever`, `nineanime`, `kodik`, `animelib`, `hanime`).
- `rankedIds`: all report providers sorted by `rank` desc, name tiebreak (mirrors
  backend stable sort).
- On error/empty → `capMap` empty, `rankedIds: []`, `error/loaded` flags set. Never
  throws to the caller.

### 4.3 `rankedProviderIds(rows, rankedIds, curated)` (pure, unit-tested)

Order for both the smart default and the panel sort:
1. `rankedIds` that exist as rows (capability-ranked, best first).
2. registry rows absent from `rankedIds` (e.g. `ae`, `raw`, `18anime`) appended in
   `curated` (`CURATED_TIER`) order, then any remainder alphabetically.

`pickSmartDefault` receives this in place of `CURATED_TIER`; its existing
`needsCheck`/`isAvailable` gate for `ae` is unchanged. Dead-source safety: the
"top-1" the panel shows is the first row in this order whose `state === 'active'`
(skips `down`/`wip`/`irrelevant`), so a non-playable top-ranked provider never
becomes the lone visible source.

---

## 5. SourcePanel UI

### 5.1 Collapse (tie reveal to hacker mode + error escape hatch)

Within the active audio/lang filter, after sorting rows by `rankedProviderIds`:

- **Default (hacker off, no error):** render only the **first `active` row** (the
  top playable source), marked "best".
- **Hacker mode on:** render the full sorted list (incl. `down`/`wip`/`irrelevant`
  with their existing badges) — same as today's list, just reordered + labelled.
- **Playback-error escape hatch:** a local `expanded` ref (independent of hacker
  mode). When `UnifiedPlayer` reports a stream/playback error (existing error state),
  `SourcePanel` shows a `Try another source ▾` disclosure under the top chip that, when
  clicked, reveals the remaining `active` rows inline. Selecting one clears the error
  and re-resolves. Resets on provider/anime change.

State precedence for what's visible: `hackerMode ? fullList : (expanded ? activeRows : [topActive])`.

### 5.2 Chip labels (`ProviderChip.vue`)

New optional `cap?: ProviderCap` prop. When present, render a compact label row under
the provider name:

- **Category tags:** the distinct `variant.category` set → `SUB` / `DUB` / `RAW`
  pills (reuse existing `player.sub`/`player.dub` styling; add `raw`).
- **Quality:** the max quality across variants (`qualities[]`) → e.g. `1080p`. Omit
  when `quality_source === 'unknown'`/absent (e.g. Kodik iframe).
- **Sub-delivery hint:** on the `SUB` tag, a hint from the sub variant's
  `sub_delivery`: `hard` → "burned-in" (muted), `soft` → "selectable". Tooltip via
  `title`. Directly answers the "which burns subs in" feedback. `none`/dub → no hint.

No capability prop (ae/raw/18anime) → chip renders exactly as today.

### 5.3 Team chips

The existing team-chip section gets a per-team `SUB`/`DUB` tag. Source: for the
selected provider (`kodik`/`animelib`), match the team to a `capMap` variant by
`variant.team.name` and read `variant.category`. Unmatched teams render untagged (no
regression). Team names still come from the existing resolver `listTeams` path — this
spec only adds the tag, it does not re-plumb team fetching.

---

## 6. Failure / degradation

`/capabilities` is decoration + ordering only — it must never block playback:

- Endpoint error / 5xx / timeout / empty families → `capMap` empty → ordering falls
  back to `CURATED_TIER`, chips show no extra labels, collapse still works off
  `state` alone (top-1 = first `active` row by `CURATED_TIER`).
- Health poll and capabilities are independent; either can be missing.
- A provider present in capabilities but absent from the registry is ignored (registry
  is authoritative for what's renderable).

---

## 7. i18n

New keys under `player.sources.*` in **all three** `locales/{en,ru,ja}.json`
(redeploy-gated — author all three together, run `frontend/web/scripts/i18n-lint.sh`):

- `player.sources.best` — "Best" badge
- `player.sources.tryAnother` — "Try another source"
- `player.sources.raw` — "RAW"
- `player.sources.subBurnedIn` — "burned-in"
- `player.sources.subSelectable` — "selectable"
- (reuse existing `player.sub` / `player.dub`)

---

## 8. Testing

Vitest, co-located `.spec.ts` (project convention; no testify/Playwright unless asked):

- `useCapabilities.spec.ts` — flatten families→`capMap`; `rankedIds` order; error/empty
  degrade to empty map without throwing.
- `rankedProviderIds.spec.ts` — capability-ranked first; `ae`/`raw`/`18anime` appended
  in `CURATED_TIER` order; dead-source promote (top-ranked `down` is not the lone
  visible top-1).
- `SourcePanel.spec.ts` — collapse states (default top-1, hacker full list, error→
  expanded), chip label rendering (category/quality/sub-delivery), team-tag mapping.
- `ProviderChip.spec.ts` — label row present with `cap`, absent without.

`bunx tsc --noEmit` + `bunx vitest run` green; DS lint + i18n lint pass.

---

## 9. File inventory

**Create:**
- `frontend/web/src/types/capabilities.ts`
- `frontend/web/src/composables/unifiedPlayer/useCapabilities.ts` (+ `.spec.ts`)
- `frontend/web/src/composables/unifiedPlayer/rankedProviderIds.ts` (+ `.spec.ts`)

**Modify:**
- `frontend/web/src/api/client.ts` — `capabilitiesApi.get(animeId)`
- `frontend/web/src/components/player/unified/UnifiedPlayer.vue` — wire `useCapabilities`,
  compute `rankedProviderIds`, pass to `SourcePanel` + `pickSmartDefault`
- `frontend/web/src/composables/unifiedPlayer/smartDefault.ts` — accept ranked ids arg
  (default `CURATED_TIER` for back-compat)
- `frontend/web/src/components/player/unified/SourcePanel.vue` — collapse + labels +
  team tags + try-another disclosure (+ `.spec.ts`)
- `frontend/web/src/components/player/unified/ProviderChip.vue` — `cap` label row
  (+ `.spec.ts`)
- `frontend/web/src/locales/{en,ru,ja}.json` — `player.sources.*`

---

## 10. Risks

- **Ranking divergence:** server `rank` is EN-only-meaningful (RU/Hanime providers
  rank 0). Within a single audio/lang filter the rows are mostly one family, so a flat
  RU rank is fine (Kodik/AniLib disambiguate by team, not provider rank). Documented;
  no per-family rank needed.
- **Top-1 hides a working alternative the user prefers:** mitigated by hacker mode +
  the error escape hatch; the saved-combo restore (existing Stage 1b) still pins a
  user's last explicit provider choice ahead of the smart default.
- **Capability/health disagreement** (capabilities says `up`, health poll says `down`
  or vice-versa): health poll is fresher (30s) and authoritative for `state`; rank only
  orders. The dead-source promote keys off `state`, so the live poll wins.
