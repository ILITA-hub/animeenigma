# aePlayer Subtitles + Subtitle Observability — Design

**Date:** 2026-06-22
**Status:** Approved (brainstorming) — pending implementation plan
**Author:** claude-code (with project owner)

## Problem

The aePlayer's subtitle picker is a **disconnected stub**. Concretely, on a watch
page (e.g. `/anime/<uuid>?...&episode=8`):

- The episode shows **empty JP subs** with **no provider options** to choose from,
  even though the user expects "at least two options from different sub providers".

Root causes (verified in code, `origin/main`):

1. **`AePlayer.vue` passes `:tracks="[]"`** to `BrowseSubsModal` — the track list is a
   hardcoded empty array. The modal is fully built (search + provider/lang filters +
   grouping + badges) but is fed nothing.
2. **`StreamResult` has no subtitle field** (`types/aePlayer.ts`), and
   `useProviderResolver.ts` declares the scraper envelope's `stream.tracks?: unknown[]`
   but **drops it** — provider-supplied soft-subs (e.g. gogoanime/megaplay EN VTT,
   allanime/nineanime/sidecar tracks) are discarded, despite the backend already
   returning them **signed** by catalog.
3. **No aggregation is wired.** The backend already exposes
   `GET /api/anime/{id}/subtitles/all?episode=N` (the `SubsAggregator`, merging
   **Jimaku** JP + **OpenSubtitles**), and `subtitlesApi.all()` already exists in
   `api/client.ts`. The old `OtherSubsPanel.vue` consumed it fully — but it was
   never ported to the aePlayer.

Separately, there is **no observability** on subtitle resolution. Jimaku is known to
go down intermittently, but nothing surfaces per-provider health: the aggregator
already computes `ProvidersDown` (and emits `X-Subtitle-Providers-Down`) but emits no
Prometheus metrics, so an outage is invisible on Grafana.

## Goals

- aePlayer surfaces a real, browseable list of subtitle tracks merged from **the
  aggregation endpoint (Jimaku + OpenSubtitles)** *and* **the current provider's own
  signed `stream.tracks`**.
- When a stream is a SUB/raw cut with **no burned-in subs**, the player
  **auto-selects a best-match track** so subtitles appear without manual browsing
  (the user complaint). User picks still override.
- `BrowseSubsModal` gains **loading + error/retry states** (it currently assumes a
  static array) and an explicit **"Subtitles off"** affordance.
- A new **`subtitle-health` Grafana dashboard** shows subtitle resolve rate/latency
  and **per-provider uptime** (Jimaku / OpenSubtitles), driven by real resolve traffic.

## Non-Goals (YAGNI)

- No new subtitle providers; no rewrite of the working backend `SubsAggregator`.
- **No active polling of Jimaku in this work** — the uptime gauge is driven by real
  `/subtitles/all` traffic (cheaper, reflects real user impact). Active probe-style
  polling is filed as a follow-up TODO in `/admin/feedback` (owner request,
  2026-06-22) and may feed the same dashboard later.
- No changes to `SubtitleOverlay.vue` rendering (ASS/SRT/VTT) — it already works; we
  only feed it a chosen track URL.

## Packaging

**One spec, ship together.** Part A (player) and Part B (observability) land as a
single update (web + catalog + grafana provisioning).

---

## Part A — Wire subtitles into the aePlayer

### A1. `StreamResult` carries provider subtitle tracks

Add an optional field to `StreamResult` (`frontend/web/src/types/aePlayer.ts`):

```ts
export interface SubtitleTrack {
  url: string
  provider: string   // e.g. 'gogoanime' (provider's own track) — see SubTrack in modal
  lang: string       // 'en' | 'ja' | ...
  label: string
  format: string     // 'vtt' | 'srt' | 'ass'
}
export interface StreamResult {
  // ...existing...
  subtitles?: SubtitleTrack[]
}
```

In `useProviderResolver.ts`, the scraper adapter maps the envelope's
`stream.tracks` (already signed by catalog — carry `exp`/`sig` query params on the
URL exactly like stream sources) into `StreamResult.subtitles`. Tracks with
`kind === 'thumbnails'` (or non-subtitle kinds) are excluded; only caption tracks
are kept. Provider label = the active provider id.

This shape is intentionally identical to the modal's existing
`SubTrack {url, provider, lang, label, format}` so no translation layer is needed.

### A2. `useSubtitleTracks` composable

New `frontend/web/src/composables/aePlayer/useSubtitleTracks.ts`:

- **Inputs:** `animeId`, reactive `episodeNumber`, reactive `currentStream`.
- **Behavior:** lazily fetches `subtitlesApi.all(animeId, episode)` (on first subs-menu
  open and on episode change; cached per `(animeId, episode)` so reopening is free).
  Maps `AggregateResponse.languages{}` into `SubTrack[]` (provider `jimaku` /
  `opensubtitles`). Merges in `currentStream.subtitles` (provider's own).
  De-duplicates by `url`.
- **Exposes:** `tracks: SubTrack[]`, `loading: boolean`, `error: string | null`,
  `providersDown: string[]` (from the aggregation response), `refetch()`.
- **Failure model:** mirrors the backend's fail-soft — an aggregation error sets
  `error` but still surfaces any provider tracks already in `currentStream`. Jimaku
  being down just means fewer tracks + a `providersDown` entry, never an empty modal
  when provider tracks exist.

Unit-tested in isolation with a fake `subtitlesApi` (no network), following the
existing `aePlayer/*.spec.ts` handwritten-fake style.

### A3. `AePlayer.vue` wiring

- Instantiate `useSubtitleTracks(props.animeId, selectedEpisodeNumber, currentStream)`.
- `BrowseSubsModal`: `:tracks="subtitleTracks"` (was `[]`), plus `:loading` / `:error`
  / `@retry`.
- **Auto-select default** (the fix): after a stream resolves, if the cut is SUB/raw
  AND there is no soft track currently chosen AND no burned-in hardsub note applies,
  pick a best-match track:
  1. lang matches the combo (`ja` for raw, `en` for english-sub, `ru` for ru-sub),
  2. prefer `jimaku` provider, then provider-own, then `opensubtitles`,
  3. else first available.
  Set `chosenSub` + turn the overlay on. A manual pick or explicit "off" always wins
  and suppresses re-auto-select for that stream. Implemented as a pure
  `pickDefaultSubtitle(tracks, combo)` helper (unit-tested), called from the existing
  resolve flow.

### A4. `BrowseSubsModal` loading / error / off states

The modal already has: search, provider chips, lang chips, grouped list, badges,
selected state, empty state, ESC/scrim close. Add:

- **Loading state** — skeleton/spinner while `loading` (replaces the body list).
- **Error state** — message + **Retry** button (emits `retry`) when `error` is set;
  critical given Jimaku flakiness. If some provider tracks exist, show them *and* a
  non-blocking "couldn't reach: <providersDown>" notice.
- **"Subtitles off"** — an explicit row/button at the top that clears `chosenSub`
  (distinct from "no tracks match search").

New props: `loading?: boolean`, `error?: string | null`, `providersDown?: string[]`.
New emit: `retry`, `off`. Existing `.spec.ts` extended for the new states.

---

## Part B — Subtitle observability

### B1. Metrics (`libs/metrics/subtitles.go`)

Follow the existing `libs/metrics` promauto pattern:

- `catalog_subtitle_resolve_total{provider,status}` — counter. `status ∈ {ok,down,empty}`
  recorded per provider per `FetchAll`.
- `catalog_subtitle_resolve_duration_seconds` — histogram, one observation per
  `FetchAll` (overall aggregation latency).
- `catalog_subtitle_provider_up{provider}` — gauge set 1/0 each `FetchAll` from
  whether the provider was in `ProvidersDown` (captures Jimaku going down).
- `catalog_subtitle_tracks_returned{provider}` — counter, tracks merged per provider.

### B2. Instrument `SubsAggregator.FetchAll`

`services/catalog/internal/service/subs_aggregator.go` already runs both providers in
parallel and computes `ProvidersDown`. Add: time the call, and after merge set the
gauge / increment counters per provider (`ok` when it returned ≥0 tracks without
error, `down` when in `ProvidersDown`, `empty` when reachable but 0 tracks).
**Cache hits emit no metric** — the gauge and counters are driven only by live
provider calls, so the uptime signal reflects real upstream reachability rather than
Redis cache state. No behavior change to the response.

### B3. Grafana dashboard (`docker/grafana/dashboards/subtitle-health.json`)

New provisioned dashboard (sibling of `playback-health.json`):

- **Resolve rate** — `rate(catalog_subtitle_resolve_total[5m])` by status.
- **Resolve latency** — p50/p95 from the histogram.
- **Per-provider uptime timeline** — `catalog_subtitle_provider_up{provider}` over
  time (Jimaku / OpenSubtitles) — the panel that makes a Jimaku outage obvious.
- **Tracks returned** — `rate(catalog_subtitle_tracks_returned[5m])` by provider.
- **Stat: providers currently down** — `min_over_time(...up...)`/last value.

Provisioned the same way as the other `docker/grafana/dashboards/*.json` (no manual
import). No Prometheus scrape change needed — catalog already exposes `/metrics`.

---

## Components & Boundaries

| Unit | Responsibility | Depends on |
|------|----------------|------------|
| `StreamResult.subtitles` + resolver map | Carry provider's own signed tracks | scraper envelope |
| `useSubtitleTracks` | Fetch + merge + dedupe aggregation & provider tracks | `subtitlesApi`, `currentStream` |
| `pickDefaultSubtitle` | Pure best-match selection | combo, tracks |
| `BrowseSubsModal` (loading/error/off) | Present + filter tracks, surface fetch state | props only |
| `libs/metrics/subtitles.go` | Metric definitions | promauto |
| `SubsAggregator` instrumentation | Emit resolve/uptime metrics | metrics pkg |
| `subtitle-health.json` | Visualize subtitle health | Prometheus, catalog `/metrics` |

## Testing

- **FE unit (vitest):** `useSubtitleTracks` (merge/dedupe/fail-soft with fake api);
  `pickDefaultSubtitle` (lang/provider precedence, off-suppression);
  `BrowseSubsModal` new loading/error/off states.
- **FE type/lint:** `vue-tsc`, DS-lint, i18n parity for any new keys (en/ru/ja).
- **BE (go test):** `SubsAggregator` metric emission (ok/down/empty paths) with fake
  Jimaku/OpenSubtitles clients (existing handwritten-fake style, no testify/mock);
  assert gauge flips when a provider is in `ProvidersDown`.
- **Dashboard:** JSON validates + provisions (loads in Grafana without error).

## Risks / Notes

- **Subtitle URL signing:** provider tracks must carry `exp`/`sig` to the HLS proxy or
  it 502s (same rule as stream sources). Aggregation URLs (Jimaku/OpenSubtitles) are
  already signed by catalog. The resolver must forward both.
- **Cascade caveat:** the modal's loading/empty states touch jsdom-invisible styling;
  if a Chrome smoke is wanted, flag it (per DS-NF-06 it's opt-in).
- **i18n:** new modal strings (loading/error/retry/off) added to en/ru/ja.
