# "May not work" mark — content-verify unreachable → aePlayer Source panel

**Date:** 2026-07-24
**Backlog:** `.planning/backlog/PLAYER-mark-probe-unreachable-sources.md`
**Source feedback:** `2026-07-23T15-59-04_tNeymik_telegram` (admin TODO, @tNeymik)

## Goal

Surface a proactive, **all-users** "may not work" mark in the aePlayer Source panel
for providers the content-verify probe has rated **unreachable**, without removing
their selectability.

## Owner decisions (2026-07-24)

- **Behavior:** informational badge, source **stays selectable**. Preserves the
  deliberate "live playback in the user's browser is the real test" grace
  (`services/catalog/internal/service/capability/providerview.go`).
- **Trigger signal:** **content-verify `unreachable`** verdicts — NOT the roster
  `health=down` state machine, NOT the unified playback probe.
- **Copy:** "may not work" / `может не работать` / `再生できない可能性`.
- **Visibility:** all users (not hacker-mode gated).

## The signal

Content-verify's `StatusUnreachable` (`services/content-verify/internal/prober/prober.go`)
means the stream **resolved but its media is dead** (first fragment dead / no
fragments extracted / HLS localize failed). It deliberately excludes:

- **Provider down (503)** → `StatusDeferred` (never persisted).
- **Probe budget exceeded / cancelled** → `StatusInconclusive` (not punished).

Provider-level rollup rule (added to `domain.Summarize`):

> `unreachable = (len(units) > 0) && (every unit.Status == StatusUnreachable)`

Any `verified` or `inconclusive` unit means the provider is at least partially
reachable → **not** flagged. Strictest rule = fewest false positives, which pairs
with informational-only + still-selectable.

## Changes

### Backend (additive)

1. `services/content-verify/internal/domain/verify.go` — add `Unreachable bool`
   (`json:"unreachable"`) to `ProviderSummary`; compute in `Summarize`. Flows out
   of `/internal/verify/verdicts` (handler already calls `domain.Summarize`).
2. `services/catalog/internal/domain/capability.go` — add `Unreachable bool` to
   `VerifySummary` (decoded off the wire by `VerifyClient.Summaries`).
3. `services/catalog/internal/service/capability/verify_synth.go` — add the field
   to `providerSummaryWire` + ported `summarizeSynth` (parity with `Summarize`).
   ae/kodik synth units are always `verified` → never flagged (correct: first-party
   / iframe, not probe targets). Propagate through `SynthSummaries`.

Result: `ProviderCap.verify.unreachable` rides both the 10-min capability feed and
the live `/content-verify` poll.

### Frontend

4. `frontend/web/src/types/contentVerify.ts` — `unreachable?: boolean` on
   `VerifySummary` / `ProviderVerify`; copy through `useContentVerify` normalization.
5. `frontend/web/src/components/player/aePlayer/ProviderChip.vue` —
   - Badge "may not work" (`text-destructive` + `bg-destructive-soft`), right-aligned,
     takes priority over recovering/degraded/no_content badge slot when
     `verify.unreachable`.
   - Dot turns red (`bg-destructive`) when unreachable.
   - Selectability unchanged; all users.
6. i18n `player.sources.mayNotWork` — en / ru / ja.

## Edge cases

- Mixed units (any verified/inconclusive) → not flagged.
- ae / kodik → synth-verified → never flagged.
- content-verify down / kill-switch off → summary absent → no flag (graceful).
- `no_content` provider → no probe units → not flagged.

## Deliberate simplification

No recency window — a provider stays flagged until its next reprobe (unreachable
units back off up to 7d). Informational-only + still-selectable makes a stale flag
low-harm; tighten later if needed.

## Tests

- content-verify `domain.Summarize`: all-unreachable → true; mixed → false; empty → false.
- catalog `TestSynthSummaries_Parity` stays green; assert synth never sets unreachable.
- FE `ProviderChip.spec`: `verify.unreachable` → badge + red dot, still clickable.
- `/frontend-verify` + `go test ./...` + `/animeenigma-after-update`.
