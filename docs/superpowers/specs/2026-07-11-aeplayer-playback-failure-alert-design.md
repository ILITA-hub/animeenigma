# aePlayer playback-failure telemetry + alert — Design

**Date:** 2026-07-11
**Status:** Approved (owner green-light 2026-07-11)
**Topic:** Automatically collect a full diagnostic bundle when a viewer cannot watch on aePlayer, and page the maintenance channel when it happens more than once an hour.

---

## 1. Problem

When a user opens an anime and aePlayer tries but **cannot play it**, they are shown the red source-error overlay and simply give up. Today there is **no trustworthy operational signal** for that outcome:

- `playerTelemetry.ts` already emits a `resolve` / `outcome:'fail'` event, but it fires on **every** resolve failure — including transient ones that immediately auto-recover via `advanceToNextSource`. A successful resolve also does not prove the stream actually played. So "resolve failed" ≠ "the user couldn't watch."
- The rich per-attempt diagnostics (`debugStats`: bandwidth / level / frag / served edge / edge-trail / rotations) are computed **only in hacker mode** (`if (!state.hackerMode.value) return null`), so they never reach the backend for a normal user's failed session.

We want: (a) automatic capture of the full diagnostic context at the moment a watch genuinely fails, and (b) an alert to the maintenance channel when failures cluster.

## 2. Goals / Non-goals

**Goals**
- Emit a single **terminal** telemetry event, `playback_failed`, exactly when a watch attempt is unrecoverable **or** the first-party `ae` source fails.
- Ship a full diagnostic bundle ("all logs") with each event to ClickHouse `analytics.events`.
- Fire a Grafana alert to the maintenance channel when **> 1** such event occurs in a rolling **1 hour**.

**Non-goals**
- No user-facing UI change — the error overlay already exists; this is invisible telemetry + ops alerting.
- No new backend endpoint or service — reuse the existing `/api/analytics/player-events` write path.
- Do **not** count content-gap states — "no source for this filter" (`noSourceForFacet`), "no episodes" (`noEpisodes`), or the case where **every** source is merely missing the requested episode (e.g. a not-yet-aired episode deep-linked). Those are not "aePlayer tried to play and failed"; they are excluded (see §3).

## 3. Trigger definition

The terminal signal fires from inside **`advanceToNextSource(reason)`** in `AePlayer.vue` — the single choke point that every playback/resolve/stall failure already funnels through (call sites: `resolve failed`, `silent stall`, `playback fatal`, `source missing the requested episode`). It is the right hook because it is invoked with the **failing** source still in `state.combo.value` and it is the function that decides whether any fallback remains.

Both branches fire **only for a genuine stream/playback failure** — the `advanceToNextSource` invocation whose `reason` is a stream failure (`resolve failed`, `silent stall`, `playback fatal`), **not** `source missing the requested episode` (that reason is a content/scheduling gap and is gated out of both branches, since a source lacking one episode — including a not-yet-aired episode, or `ae`'s known partial library — is not an aePlayer failure).

Given that gate, fire `playback_failed` when **either**:

- **A — all providers exhausted** (`reason: 'all_exhausted'`): `advanceToNextSource` finds **no** further candidate (it is about to return `false`, after which the caller sets `sourceError` and the viewer sees the error overlay). This is the true "couldn't watch it at all."
- **B — first-party `ae` failed** (`reason: 'ae_failed'`): the source being left is the first-party provider (`state.combo.value.provider === 'ae'`), **even when a fallback candidate exists and failover recovers**. A self-hosted MinIO/library/storage failure is worth catching on its own, not hidden behind a successful third-party fallback.

If both hold in one invocation (ae was the last candidate and it failed), emit **one** event with `reason: 'ae_failed'` and `all_exhausted: true` in the payload — never two rows for one `advanceToNextSource` call.

**Excluded:** `noSourceForFacet` sets `sourceError` directly and never calls `advanceToNextSource` (excluded by construction). The "every source missing the requested episode" exhaustion *does* run through `advanceToNextSource`, but its `reason` is gated out per above. Third-party providers flaking but recovering via failover never emit (only the exhausted tail or the ae branch does).

### Hacker mode — suppressed
No `playback_failed` is emitted while `state.hackerMode.value` is true. Rationale: (1) that is the owner debugging, not a real user; (2) hacker mode deliberately **suppresses auto-failover** (`advanceToNextSource` only records a "suppressed" message instead of switching), so its failures are not representative of the production failover path. This is the reading of the owner's "(don't do in hacker mode)".

### De-duplication
Collapse retry-spam: at most one emission per `(reason, anime_id, episode)` per unresolved failure streak. The guard re-arms on a genuinely new attempt (successful playback start, manual **Retry**, or an episode/provider change). Distinct reasons or a new episode are distinct occurrences. This keeps "> 1 occurrence in an hour" honest — one broken watch = at most one `all_exhausted` (plus at most one `ae_failed` if the first-party source was involved).

## 4. Data collected ("all logs")

Assembled at emit time regardless of hacker mode, then packed into the event's `Properties` JSON. All strings capped; all arrays length-bounded.

| Field | Source | Notes |
|---|---|---|
| `schema_version` | const | bump on shape change |
| `reason` | trigger | `all_exhausted` \| `ae_failed` |
| `all_exhausted` / `is_first_party` | trigger | booleans, both may be true |
| `anime_id`, `episode` | `props.animeId`, `selectedEpisode` | |
| `combo` | `state.combo.value` | `audio`, `lang`, `provider`, `server`, `team` |
| `error_kind` | mapped from `reason` arg / caught error | `stream_error` \| `not_available` \| `stall_timeout` \| `playback_fatal` (never `missing_episode` — that reason is gated out, §3) |
| `error_message` | caught error / i18n key | capped ≤ 500 |
| `attempt_trail` | new in-component ring buffer | `[{provider, server, outcome, error_kind, latency_ms}]`, each `advanceToNextSource` appends the source it just left; cap ≤ 30 |
| `engine` | `engine.*` + `playbackStats` | `bw_bps`, `level`, `frag_size_kb`, `frag_load_ms`, `served_edge`, `edge_trail`, `edge_rotations`, `buffer_ahead_s`, `buffer_behind_s`, `video_ready_state` |
| `capability_snapshot` | capability feed | `[{provider, group, state}]` (active / no_content / degraded) so we can see what was on offer; cap ≤ 40 |
| `client` | `navigator` | `ua`, `viewport`, `connection.effectiveType` (when present), app build id |
| `ts` | now | ISO-8601 |

The `attempt_trail` is a small ring buffer added to `AePlayer.vue`: `advanceToNextSource` pushes the source it is abandoning (provider/server + why) each time it runs, giving a cross-source failover history the engine's per-CDN `edgeTrail` does not capture. Reset it when a fresh watch attempt begins.

## 5. Frontend changes (`frontend/web/src/`)

1. **`utils/playerTelemetry.ts`**
   - Add `'playback_failed'` to the `PlayerEvent['kind']` union and to the `recordPlayerEvent` kind whitelist.
   - Add an optional `detail?: Record<string, unknown>` field carried on the event and serialized in the batch body (the diagnostic bundle). Keep the existing never-throw / rate-cap / batch-flush contract untouched.
2. **`components/player/aePlayer/AePlayer.vue`**
   - Add the `attempt_trail` ring buffer + a `reportPlaybackFailed(...)` helper that builds the bundle (§4) and calls `recordPlayerEvent({ kind: 'playback_failed', ... })`.
   - Extend `advanceToNextSource(reason)` to: append to `attempt_trail`; compute "candidate exists?"; and, unless `hackerMode`, call `reportPlaybackFailed` for the `all_exhausted` and/or `ae_failed` cases with de-dup.
   - Lift the diagnostic bundle assembly out of the hacker-mode-gated `debugStats` computed into a plain helper both can share, so collection no longer depends on hacker mode being on.

## 6. Backend changes (`services/analytics/`)

`internal/handler/playertelemetry.go`:
- Add `case "playback_failed": effectKind = "player_failed"` to the kind switch.
- Extend `wirePlayerEvent` with `Detail json.RawMessage` (size-capped); when present, merge its keys into `propMap` before marshaling `Properties`. Body already `LimitReader`-capped at 256 KB — one bundle fits comfortably.
- Row written: `EventType=player`, `EffectKind='player_failed'`, `Target=provider`, `TargetKind='provider'`, `AnimeID`, `Source='fe'`, `Properties=<bundle>`.

**Whitelist check (must-verify):** the handler drops events whose provider is not in `playertelemetry_whitelist.go`. Confirm `'ae'` passes; if absent, add it — otherwise every `ae_failed` row is silently dropped.

## 7. Alert (Grafana)

New ClickHouse-datasource unified-alerting rule **`AePlayerPlaybackFailures`**:

- Query `analytics.events` for `count()` where `effect_kind = 'player_failed'` over the trailing **1 h**.
- **Fires at ≥ 2** (the owner's "more than 1 occurrence in one hour").
- Routes to the existing `maintenance-webhook` contact point → Telegram maintenance channel. Severity **notifying** (not `diagnostic`/muted — this is real user impact).
- Follows the `ScraperStreamCascadeLatency` precedent: numeric `format: 1` in the CH query (string `"table"` is rejected for alert rules), appended to `docker/grafana/provisioning/alerting/rules.yml`, and mirrored into the human source-of-truth `infra/grafana/alerts/` copy. Deploy = `make restart-grafana`.
- Validate non-destructively before shipping via the provisioning API (`POST /api/v1/provisioning/alert-rules` with `X-Disable-Provenance` → poll `/api/prometheus/grafana/api/v1/rules` for `health=ok` → `DELETE`).

## 8. Testing

- **FE unit (`playerTelemetry.spec.ts`)**: `playback_failed` accepted; `detail` serialized in the batch body; rate-cap/never-throw still hold.
- **FE unit (AePlayer)**: `advanceToNextSource` emits once for `all_exhausted` when a stream-failure reason exhausts the chain; emits `ae_failed` when leaving `provider:'ae'` even with a candidate; emits **nothing** in hacker mode; emits **nothing** when the exhausting `reason` is `source missing the requested episode`; de-dup collapses a retry streak; content-gap paths (`noSourceForFacet`) emit nothing.
- **Backend (`playertelemetry_test.go`)**: `kind:'playback_failed'` → `effect_kind='player_failed'`; `detail` merged into `Properties`; oversized detail rejected/capped; `'ae'` provider survives the whitelist.
- **Alert**: provisioning-API validation returns `health=ok`; a synthetic seed of 2 rows in an hour trips the rule.

## 9. Rollout

Ship as one change: FE (`web`) + analytics service + Grafana provisioning. Deploy order: `make redeploy-analytics && make redeploy-web && make restart-grafana`. No migration (ClickHouse `events` columns are reused; the bundle lives in the existing `Properties` string). No user-facing changelog needed for the alert/telemetry, but the after-update flow still runs.

## 10. Resolved decisions

- Trigger = **all providers exhausted OR ae failed** (not every error overlay). ✔ owner
- Hacker mode = **suppressed**. ✔ (owner "don't do in hacker mode")
- Alert threshold = **≥ 2 in 1 h**, to `maintenance-webhook`. ✔ owner
- Reuse `/api/analytics/player-events` + `analytics.events`; no new endpoint. ✔
