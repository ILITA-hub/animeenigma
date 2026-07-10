# Kodik solodcdn edge — patient immediate-failover + hacker-mode telemetry

**Date:** 2026-07-10
**Status:** Approved (design), ready for implementation
**Origin:** feedback report `2026-07-10T06-13-08_NANDIorg_9_feedback` — "Kodik dubs don't work" on
*Welcome to Demon School, Iruma-kun! 4*. Diagnosis: p13.solodcdn.com served the manifest but the first
`.ts` segment never completed (browser `status:0`); hls.js retried `seg-1` 11× (6 with the `aescrub`
fallback) and gave up. Playback never started.

## Problem

The AUTO-562 self-heal (`libs/videoutils/proxy.go` `maybeRotateSolodcdnEdge`) rotates a Kodik
`p<N>.solodcdn.com` edge to a sibling **only when the first upstream response is HTTP `≥500`**. When the
primary fetch instead returns a **transport error / response-header timeout** — a hung or reset
connection, which is exactly the `status:0` the report shows — `ProxyWithRefererCounted` bails at the
`resp, err := doUpstream(...)` guard (proxy.go ~706) and returns a 502 **without ever attempting
rotation**. So a *timing-out or connection-refusing* edge slips past the self-heal, while a cleanly-500
edge is covered.

Additionally, a cold-starting edge (accepts TCP, then needs time to prepare HLS) can exceed the current
`ResponseHeaderTimeout = 20s` and be abandoned as a "timeout" even though it was about to serve.

## Goals

1. When a specific Kodik `p<N>` edge **fails**, try another sibling **straight ahead** — extend the
   trigger from "≥500 only" to also cover **hard transport errors** (dial refused/reset/unreachable) and
   **response-header timeout**.
2. Be **patient**, not eager: a cold edge preparing HLS should be *waited on*, not prematurely rotated.
   Widen the response-header window so a slow-but-alive edge can answer.
3. Surface the **metrics and the logic** behind edge selection (not just the final decision) in the
   aePlayer **hacker mode**, plus Prometheus + structured logs.

## Non-goals (explicitly chosen: reactive-immediate)

- No background edge pinger / active canary.
- No persisted per-edge health score or cooldown table.
- No Redis-shared edge health.
- No change to non-solodcdn provider paths (EN / Raw / Hanime / ae) beyond the shared-transport
  `ResponseHeaderTimeout` widening (which only makes every provider *more* patient).

## Design

### Part 1 — Patient failover (backend: `libs/videoutils/proxy.go`)

- **Widen `ResponseHeaderTimeout` 20s → 45s** (transport-wide; conservative — waits longer before
  declaring a timeout so a cold edge preparing HLS can respond). Keep the **10s dial timeout** so a
  genuinely refused/unreachable edge fails the dial fast and rotates immediately.
- **Refactor** the self-heal into a single `fetchWithEdgeFailover(nominalURL, do)` that owns the whole
  ordered attempt sequence for a `solodcdn` p-edge host: **nominal edge first**, then siblings, capped at
  `maxSolodcdnRotations` (2), fenced to `solodcdnEdgeRe` (`^p\d+\.solodcdn\.com$`). For any non-solodcdn
  host it is a pass-through of a single attempt (current behavior preserved exactly).
- **Rotation trigger table:**

  | Primary attempt outcome | Behaviour |
  |---|---|
  | `<500` (incl. 4xx) | serve it (a 4xx is authoritative — stop) |
  | `≥500` response | rotate to sibling (unchanged) |
  | **hard dial error** (refused/reset/DNS/unreachable) | **rotate immediately** (new) |
  | **response-header timeout** (after the full 45s window) | **rotate** (new) |
  | all attempts exhausted | return the last response/error (terminal 502, as today) |

- Each attempt records `(edge, outcome, elapsedMs)` into an in-request **attempt trail**; `outcome ∈
  {ok, http5xx, http4xx, dial_error, timeout}`.

### Part 2 — Server telemetry (backend: `libs/videoutils` + `services/streaming`)

- **Response headers** on the client-facing response (manifest and segments):
  - `X-AE-Edge-Served: p12` — the edge that actually answered (the *decision*).
  - `X-AE-Edge-Trail: p13:timeout:45003,p12:ok:210` — compact `edge:outcome:ms` CSV of every attempt
    (the *logic + metrics*). Emitted only for solodcdn p-edge sources; absent otherwise.
- **Prometheus** (streaming service, via the existing `OnEdgeRotation` seam + new hooks):
  - `proxy_edge_rotations_total{from,to,outcome}` — **keep** (existing).
  - `proxy_edge_attempt_seconds{edge,outcome}` — **new** latency histogram per attempt.
  - `proxy_edge_selected_total{edge}` — **new** counter of which edge ultimately served.
- **Structured log** on every rotation: `log.Infow("solodcdn edge rotation", "from", …, "to", …,
  "reason", …, "ms", …)` — the logic trail in logs.

  > Metric wiring note: per the libs/metrics auto-registration trap, single-emitter counters/histograms
  > live **in the streaming service**, fed from `libs/videoutils` via callback hooks — do **not** put a
  > plain promauto metric in `libs/videoutils`.

### Part 3 — Hacker-mode `EDGE` telemetry (frontend)

- `useVideoEngine.ts` (`FRAG_LOADED` already handled): read
  `data.networkDetails?.getResponseHeader('X-AE-Edge-Served')` and `X-AE-Edge-Trail` → expose reactive
  `servedEdge` + `edgeTrail` on the engine.
- `AePlayer.vue` `debugStats` computed (~2240): add `edge` (served) + `edgeTrail` fields; populate only
  when the header is present (Kodik/solodcdn sources).
- `PlaybackSettingsMenu.vue` debug panel (`v-if="hackerMode && debugStats"`, next to BW/BUF/LVL/FRAG):
  add mono lines showing **metrics + logic, not only the decision**:
  ```
  EDGE p12                       ← decision (served)
  TRY  p13 45.0s✗ → p12 0.21s✓   ← logic (attempt trail) + metrics (latency)
  ROT  ×1                        ← metric (rotations this load)
  ```
  Omitted entirely for non-Kodik sources. No i18n (debug labels are untranslated, like BW/BUF/LVL/FRAG).

### Part 4 — Standing memory guideline

Write a memory codifying: *"When surfacing live-playback data in aePlayer hacker mode
(`PlaybackSettingsMenu.vue` `debugStats`), expose the **metrics** (counts, latencies) and the **logic**
(why / attempt trail) behind a decision — not only the final decision. Any new meaningful live-playback
datum is surfaced there by default."* Index in `MEMORY.md`, link to `reference_aeplayer_canonical_doc`
and `project_solodcdn_edge_flap_rotation_selfheal`.

## Testing

- **Go** (`libs/videoutils/proxy_edge_rotation_test.go`, extend): table-driven —
  - primary `dial_error` on a p-edge → rotates to sibling and serves it (new);
  - primary `timeout` on a p-edge → rotates after the window (new);
  - primary `≥500` → rotates (existing, keep green);
  - non-solodcdn primary error → **no** rotation, error surfaced unchanged;
  - attempt trail + `X-AE-Edge-Served`/`X-AE-Edge-Trail` header content assertions.
- **Frontend** (vitest): `debugStats` includes `edge`/`edgeTrail` when the FRAG_LOADED header is present;
  `PlaybackSettingsMenu` renders the EDGE/TRY/ROT lines under `hackerMode`, and nothing for non-Kodik.

## Rollout / verify

- Backend touches a shared lib (`libs/videoutils`) → redeploy **streaming** (the only consumer that
  emits the new metrics). Verify with a live reproduce: resolve a Kodik stream → fetch manifest + first
  segment via `/api/v1/hls-proxy` → confirm `X-AE-Edge-Served`/`X-AE-Edge-Trail` headers present and a
  segment `200 video/mp2t`. Check `curl localhost:8082/metrics | grep proxy_edge_`.
- Frontend → `/frontend-verify` (DS-lint + i18n + real build).
- `/animeenigma-after-update` → changelog + redeploy + push.

## Metrics (project convention)

- **UXΔ** = +2 (Better) — fewer "Kodik dub won't play" failures during edge flaps; power users get full
  edge reasoning in hacker mode.
- **CDI** = 0.04 * 13 — moderate spread (videoutils + streaming metrics + 3 FE files + memory), low
  behavioral shift (extends a fenced, existing self-heal).
- **MVQ** = Griffin 88%/83% — precise hybrid backend+frontend reliability fix.
