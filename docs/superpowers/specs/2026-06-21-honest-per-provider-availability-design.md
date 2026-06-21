# Honest Per-Provider Availability (Probe + Player) — Design

**Date:** 2026-06-21
**Status:** Approved (design) → pending spec review → plan
**Owner:** @0neymik0
**Constraint:** Do NOT change scraper provider/extraction logic
(`services/scraper/internal/providers/`, `embeds/`). The only scraper-service
change is a new **no-failover routing mode** in the orchestrator + handler
query parsing. Provider matching/extraction is untouched.

---

## 1. Problem

The unified playback probe and the AePlayer both ask the scraper "can provider
X play anime Y?" — but neither can currently tell **"provider X doesn't have
this anime"** apart from **"provider X has it but the CDN is broken."**

Root cause: `prefer=<provider>` is a **soft** preference. `orderedProviders`
(`services/scraper/internal/service/orchestrator.go`) moves the preferred
provider to position 0 but **still appends the whole failover chain**, and
`failoverDecision` retries on `ErrNotFound`, `ErrProviderDown`, and
`ErrExtractFailed` alike. So when gogoanime genuinely lacks an anime:

1. gogoanime returns `ErrNotFound` →
2. the orchestrator silently fails over to the next provider →
3. that provider serves episodes/servers →
4. its stream then breaks →
5. the caller sees a `stream`-stage failure and **blames gogoanime**.

Observed live (2026-06-21 03:41 UTC probe run): gogoanime was scored **UP**
while 4 of 5 tuples actually failed. "Hibike! Euphonium 2" (which gogoanime
never uploaded) was logged as `stage=stream / cdn_unreachable` instead of
"not found"; "One Piece Fan Letter" (which gogoanime *does* have at
`gogoanime.by/series/one-piece-fan-letter/`, but which is genuinely broken)
was correctly a fail. The response even carries `meta.provider` (who actually
served) — but the probe resolver ignores it.

Two downstream consequences:

- **Probe:** a not-found anime drags a provider's verdict down even though the
  provider is fine; and the any-pass→UP scorer flips to UP on a single lucky
  anchor while most content is dead.
- **Player (hacker mode):** power users pinning a specific provider get a
  generic "source unavailable" with no way to know whether the anime simply
  isn't on that site or the site's CDN is down.

## 2. Goals / Non-Goals

**Goals**
- A single backend primitive that returns an **honest, no-failover,
  single-provider** verdict: `200` (has it) / `404 not_found` (lacks it) /
  `502 provider_down|extract_failed` (has it, broken).
- Probe: classify not-found vs broken; **skip+re-roll** not-found anime from a
  top-100 pool; score providers by **pass-percentage** thresholds.
- Player: in **hacker mode only**, a reactive, cached, per-provider tooltip
  distinguishing "doesn't have this anime" from "CDN unreachable for this
  anime." Zero added cost for casual users.
- A maintained `docs/scraper-framework.md` describing how scraping works, with
  code + DB links.

**Non-Goals (YAGNI)**
- No eager/background availability probing for casual users.
- No change to normal auto-failover playback (it still fails over).
- No new standalone `/scraper/search` endpoint — `exclusive=true` reuses the
  existing route family.
- No provider matching/extraction changes.
- No `probe_runs` schema change beyond what already exists (anime_uuid is
  sufficient; name resolution stays a dashboard concern).

## 3. Decisions (locked)

| # | Decision |
|---|----------|
| D1 | New `exclusive` query param on `/api/anime/{uuid}/scraper/episodes\|servers\|stream`. When true, the orchestrator routes to **only** the preferred provider (no failover). |
| D2 | `exclusive` is implemented by making `orderedProviders(prefer)` return `[preferred]` only. The existing `summarizeFailover` + handler error mapping already yield honest `404 not_found` / `502 provider_down`. Provider impls untouched. |
| D3 | The probe resolver calls the scraper with `exclusive=true` so it tests the named provider, not the failover landing spot. |
| D4 | New probe reason/stage: `not_found` at a `search` stage, distinct from `cdn_unreachable`. NOT_FOUND is **not** a provider failure by itself. |
| D5 | **Re-roll:** on NOT_FOUND for a (provider, slot), pick **one** random anime from `GET /api/anime/popular?page_size=100` and probe it once with `exclusive=true`. Re-roll PASS → slot PASS; re-roll fails for **any** reason (not-found OR broken) → slot FAIL. One re-roll max. |
| D6 | **Verdict thresholds** over the 4 post-re-roll slot verdicts: `PASS% > 50%` → UP; `0% < PASS% ≤ 50%` → DEGRADED; `PASS% = 0%` → DOWN. Replaces the any-pass→UP Rollup. |
| D7 | Player availability check is **hacker-mode-gated**. Casual/auto users: unchanged, no extra requests. |
| D8 | In hacker mode a **manual** provider pick resolves with `exclusive=true` (truthful, no silent failover); the classified result is cached per `(animeId, providerId)` and drives the ProviderChip tooltip. Lazy: fills in per provider as tried; cache invalidates on anime change. |
| D9 | Ship a `docs/scraper-framework.md` + a memory pointer to maintain it. |

## 4. Architecture

```
                    ┌─────────────────────────────────────────────┐
                    │  A. scraper orchestrator: exclusive=true     │
                    │  orderedProviders(prefer) → [preferred] only │
                    │  → honest 200 / 404 not_found / 502 down     │
                    └───────────────▲─────────────────▲────────────┘
                                    │ exclusive=true   │ exclusive=true
          ┌─────────────────────────┘                 └────────────────────────┐
          │ B. analytics probe                          C. AePlayer (hacker mode)│
          │  resolver classify → NOT_FOUND/PASS/FAIL     manual pick → classify  │
          │  NOT_FOUND → re-roll top-100 (1×)            cache (animeId,provider) │
          │  scorer: PASS% >50 UP / >0 DEGRADED / 0 DOWN tooltip: not-have / cdn  │
          └──────────────────────────────────────────────────────────────────────┘

   D. docs/scraper-framework.md  (orchestrator, chain, errors, prefer/exclusive,
      providers+embeds, stealth sidecar, stream_providers DB roster, capabilities)
```

### Component A — `exclusive` no-failover param (backend)

**Interface.** Add `&exclusive=true` to the existing route family. Default
`false` preserves today's soft-prefer behavior exactly.

| `prefer` | `exclusive` | gogoanime state | Result |
|----------|-------------|-----------------|--------|
| gogoanime | false (default) | lacks it | fails over; may 200 from another provider (today's behavior) |
| gogoanime | **true** | has it | `200` + episodes/servers/stream |
| gogoanime | **true** | lacks it | `404 {code: not_found}` — **no failover** |
| gogoanime | **true** | broken CDN/anti-bot | `502 {code: provider_down}` / `extract_failed` |

**Implementation anchors.**
- `services/scraper/internal/handler/scraper.go` — `queryParams` struct
  (`:106`) gains `exclusive bool`; `parseQuery` (`:143`) parses
  `q.Get("exclusive") == "true"`. `GetEpisodes`/`GetServers`/`GetStream`
  (`:218/:256/:292`) thread it into the service calls and into
  `OrderedProviderNames` (so `meta.tried` reflects the single-provider list).
- `services/scraper/internal/service/orchestrator.go` —
  `orderedProviders(prefer)` and `OrderedProviderNames(prefer)` gain an
  `exclusive bool` (or a sibling `orderedProvidersExclusive`). When exclusive,
  return only the matched preferred provider (empty if `prefer` unset/unknown —
  exclusive requires a valid `prefer`). The failover loop, `failoverDecision`,
  and `summarizeFailover` are **unchanged**: a one-element chain runs once and
  propagates the single typed error, which the handler already maps to
  404/502.
- `services/catalog/internal/handler/scraper.go` — the catalog passthrough
  forwards `exclusive` alongside `prefer` to the scraper service (mirror the
  existing `prefer` plumbing in `GetScraperEpisodes`/`Servers`/`Stream` and the
  catalog scraper service client).
- Edge: `exclusive=true` with empty/unknown `prefer` → `400` (an exclusive
  request without a valid target provider is a caller bug; see §6).

### Component B — probe not-found re-roll + percentage scorer (analytics)

Files (in the deployed probe branch — see §8):
`services/analytics/internal/probe/{resolver,engine,scorer,animeset,types}.go`.

**B1. Resolver honesty.** `HTTPResolver.Resolve`
(`resolver.go:76`) adds `exclusive=true` to its `episodes`/`servers`/`stream`
calls. It classifies the episodes response:
- HTTP `404` (body `code: not_found`) → return a sentinel
  `(nil, StageSearch, ErrProviderNotFound)` so the engine can branch to re-roll.
- HTTP `502` / empty episodes / empty stream / transport error → existing
  `cdn_unreachable`-style failure (FAIL).
- otherwise resolve servers → streams as today.
A new `StageSearch` stage and a `not_found` reason are added to
`types.go`/`streamprobe.Reason` usage. (The probe maps the scraper's
`not_found` code, not its own guess.)

**B2. Engine re-roll.** `Engine.probeProvider` (`engine.go:26`): for each
`(provider, ref)`, if `Resolve` returns the not-found sentinel, request **one**
random anime from the top-100 pool and re-probe it (also `exclusive=true`):
- re-roll PASS (≥1 server ffprobe-playable) → emit a PASS verdict for the slot.
- re-roll NOT_FOUND or FAIL → emit a FAIL verdict for the slot.
Both the original NOT_FOUND verdict (for transparency) and the re-roll verdict
(the scored one) are appended. The top-100 pool is a new
`AnimeSetResolver`-adjacent helper (`PopularPool`) calling
`GET /api/anime/popular?page_size=100` once per run, cached for the run; random
pick excludes the anchor and the not-found anime. Recovery: pool fetch failure
→ skip re-roll, treat the slot as FAIL (logged).

**B3. Slot verdict.** A slot PASSES iff any of its servers is ffprobe-playable;
else FAIL; NOT_FOUND only when episodes 404'd (→ triggers re-roll, never a
direct slot verdict).

**B4. Scorer.** `Rollup` (`scorer.go:26`) is rewritten from any-pass→UP to a
pass-ratio over slot verdicts:
```
passSlots / totalSlots > 0.50      → StatusUp
0 < passSlots / totalSlots ≤ 0.50  → StatusDegraded
passSlots / totalSlots == 0        → StatusDown
```
`totalSlots` = distinct slots that produced a definitive PASS/FAIL (every slot
does, after at most one re-roll). The dominant-reason label logic is retained
for the DEGRADED/DOWN `probe_provider_status` reason string. (Determinism /
tie-break behavior from commit `22d3c095` is preserved.)

**B5. Reporting.** `reporter.go` unchanged in shape; the new `not_found` reason
and `search` stage flow through `probe_runs_total{...,reason}` and the
`probe_runs` rows. `probe_provider_up` now reflects the new thresholds.

### Component C — player hacker-mode availability tooltip (frontend)

Files (mid-rename `components/player/unified/` → `aePlayer/`; plan resolves
exact paths against the deployed branch — see §8):
`SourcePanel.vue`, `ProviderChip.vue`,
`composables/unifiedPlayer/useProviderResolver.ts` (`makeScraperAdapter`),
`UnifiedPlayer.vue`/`AePlayer.vue`, `locales/{en,ru,ja}.json`.

**C1. Gating.** All availability logic is behind the existing `hackerMode`
flag. Casual path: untouched — `makeScraperAdapter` keeps soft `prefer`
(failover) and no availability tracking. No new requests for casual users.

**C2. Exclusive manual pick.** When `hackerMode` is on and the user **manually**
selects a provider, the scraper adapter resolves with `exclusive=true`
(`api.getEpisodes/getServers/getStream` gain an optional `exclusive` arg). The
result is classified from the HTTP status / envelope `code`:
- `404 not_found` → `{available:false, reason:'not_found'}`
- `502 provider_down` / `extract_failed` / `NotAvailableError` post-stream →
  `{available:false, reason:'cdn_unreachable'}`
- success → `{available:true}`

**C3. Cache + tooltip.** A module/composable `Map<animeId, Map<providerId,
Availability>>` (invalidated on anime change). `ProviderChip` reads it to render
the tooltip via the existing `:title`/`reason` + pill pattern (`ProviderChip.vue`
state-pill block). Two new i18n keys (added to **all** of en/ru/ja for the
locale-parity test):
- `not_found` → RU "У этого источника нет этого аниме" / EN "This provider
  doesn't have this anime"
- `cdn_unreachable` → RU "CDN источника недоступен для этого аниме" / EN
  "Provider CDN is unreachable for this anime"

**C4. Lazy fill.** Tooltips appear per provider as the hacker-mode user tries
each; non-tried rows show no availability tooltip. No fan-out.

### Component D — scraper framework doc + memory

New `docs/scraper-framework.md` covering: the failover orchestrator + default
chain order (gogoanime→animepahe→allanime→animefever→miruro→nineanime, degraded
excluded), the `/api/anime/{uuid}/scraper/*` route family + `prefer` /
`exclusive` semantics, typed errors (`ErrNotFound`/`ErrProviderDown`/
`ErrExtractFailed`) → HTTP mapping, provider impls (`internal/providers/`) +
embed extractors (`internal/embeds/`), the Camoufox stealth-scraper sidecar,
the `stream_providers` Postgres roster (+`status` enum, capabilities), and the
analytics playback probe. Each section links to code paths + DB tables. Add a
`reference`-type memory entry pointing to it with a "keep updated" note.

## 5. Data Flow

**Probe (per provider, per slot):**
```
episodes?prefer=P&exclusive=true
  ├ 404 not_found → re-roll random∈top100 (exclusive) once
  │     ├ PASS → slot PASS
  │     └ NOT_FOUND/FAIL → slot FAIL
  ├ 200 → servers → stream(s) → ffprobe
  │     ├ any playable → slot PASS
  │     └ none playable → slot FAIL
  └ 502 → slot FAIL
→ Rollup(slots) → UP / DEGRADED / DOWN → metrics + probe_runs
```

**Player (hacker mode, manual pick):**
```
select(P) → adapter.resolve(exclusive=true)
  ├ 200 → play; cache available:true
  ├ 404 → cache {false, not_found};      tooltip "doesn't have this anime"
  └ 502 → cache {false, cdn_unreachable}; tooltip "CDN unreachable for this anime"
```

## 6. Error Handling / Edge Cases

- `exclusive=true` + missing/unknown `prefer` → `400` (caller bug).
- Top-100 pool fetch fails → skip re-roll, slot = FAIL (logged WARN).
- Re-rolled anime coincides with anchor/another slot → re-pick; bounded retries
  then accept.
- All 4 slots NOT_FOUND→re-roll→still not-found → 0% pass → DOWN (a provider
  that can't serve any of 8 popular anime is effectively down).
- Provider degraded in DB: still reachable via explicit `prefer` (today's
  behavior); exclusive honors it.
- Casual player path must show **no** behavior change (regression-test it).

## 7. Testing Strategy

- **A:** orchestrator unit test — `exclusive=true` yields a one-element chain
  and propagates `ErrNotFound`/`ErrProviderDown` verbatim (no failover);
  `exclusive=false` unchanged. Handler test — `404`/`502` mapping with
  exclusive. Catalog passthrough forwards the param.
- **B:** resolver test — 404→not-found sentinel, 502→FAIL, 200→resolve;
  engine test — not-found triggers exactly one re-roll, re-roll PASS/FAIL maps
  to slot verdict; scorer table test — boundary cases (3/4 UP, 2/4 DEGRADED,
  0/4 DOWN, 1/4 DEGRADED). Fakes only (no live upstream).
- **C:** adapter test — exclusive arg added, 404/502 classification; chip test
  — tooltip text per reason; **casual-mode regression** — no exclusive calls,
  no availability fetches when hackerMode off. Locale-parity test passes
  (en/ru/ja keys).
- **D:** doc-only.

## 8. Logistics — branch base (resolve in plan, not a design choice)

The deployed unified probe + gogoanime→Camoufox wiring live in the
**`/data/ae-probe-impl` worktree (HEAD `69822378`)**, *ahead* of `main`
(`7cf93aed`, which lacks `engine.go`/the probe handler/the scheduler trigger).
The shared working tree at `/data/animeenigma` is in a half-reverted dirty
state (probe + aePlayer files deleted), and the FE is mid-rename
(`unified/`→`aePlayer/`). Therefore:
- Do the work in a **clean worktree off the actually-deployed branch**
  (`69822378`) — not the dirty shared tree.
- The plan confirms the exact base and whether `69822378` should merge to
  `main` first, then resolves the FE paths (`unified/` vs `aePlayer/`) against
  that base.

## 9. Effort & Impact (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — honest provider verdicts; hacker-mode users learn
  *why* a source fails. Casual UX unchanged by construction.
- **CDI = 0.04 * 21** — Spread across scraper(orchestrator+handler), catalog
  passthrough, analytics probe(resolver/engine/scorer), and FE
  (adapter/chip/i18n); moderate shift (new param + scorer semantics), bounded
  by reusing existing typed errors and tooltip primitives. Effort_Fib 21.
- **MVQ = Griffin 85%/80%** — disciplined reuse of the existing failover error
  taxonomy and chip tooltip pattern; slop-resistant via the casual-mode
  regression gate and scorer boundary tests.
