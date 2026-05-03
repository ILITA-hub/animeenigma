---
id: REC-S9-implicit-op-ed-affinity
title: Implicit OP/ED affinity from mid-episode skip behavior
captured_at: 2026-04-28
captured_during: rec-engine brainstorm
target_milestone: v2 (recommendations engine)
deferred_from: rec-engine v1 (was: S9 explicit themes-rating signal)
status: backlog
---

# Implicit OP/ED affinity (replaces explicit S9)

## Original idea

S9 in the v1 rec-engine brainstorm proposed using `themes_ratings` (explicit user
1-10 scores on OPs/EDs) as a signal: if the user rates "Unravel (Tokyo Ghoul)" 10/10,
recommend other anime with high-rated OPs/EDs in similar genres.

## Refined idea (this is what to build instead)

Skip the explicit-rating dependency. Use **implicit watch behavior** during the
intro/outro time-windows of every episode:

- HiAnime, Consumet, AnimeLib expose video events — we know `currentTime` per second.
- We can detect: did the user *watch through* the OP (~0:00 - ~1:30 mark) vs. press
  the skip-intro button or seek past it? Same for ED (~21:30 - end-of-episode).
- Aggregate per-anime, per-user: **OP-watch-through rate**, **ED-watch-through rate**.
- The signal: anime where the user *consistently does not skip* the OP/ED is an anime
  whose music they engage with. Compare to that user's baseline skip rate across all
  anime they watch — high relative watch-through = strong implicit OP/ED affinity for
  THAT specific anime.

## Why this is better than explicit themes-ratings

1. **No participation tax.** Explicit ratings require user effort; only a small fraction
   of users will rate OPs/EDs. Watch behavior is captured for free for every user, every
   episode.
2. **Continuous, not categorical.** "I usually skip OPs but I always watch THIS one" is
   a stronger signal than a 1-10 rating, because it's relative to the user's own baseline.
3. **Genre-agnostic.** A user who watches OPs only when they're musically interesting tells
   us about the music itself, not about the show — useful for recommending shows that share
   composer / artist / OP style.
4. **Cross-anime comparison is the point.** "Compared to other anime I watched, did I sit
   through the music more?" — exactly the framing that turns this into a usable rec signal.

## What we'd need to build (gap-fill)

This is a **Phase 5 / Phase 8 candidate** for the current Smart Watch Picker milestone,
or a milestone-1 gap for the rec engine v2:

- New event: `intro_skip` (timestamp, anime_id, episode, user_id, source: button-click vs seek)
- New event: `outro_skip` (same shape)
- Optional: per-episode `op_watch_seconds` / `ed_watch_seconds` aggregates from existing
  `watch_progress` data, computed by knowing the OP/ED time windows
- Knowledge base: OP/ED time windows per anime. Some come from Crunchyroll/HiAnime APIs;
  others may need crowd-sourcing or fallback to "first 90 seconds / last 90 seconds"
  heuristic.

## Cost estimate

| Component | Effort | Risk |
|---|---|---|
| New columns / events | Low (rides Phase 5 G-04 session_id work) | Low |
| OP/ED time-window data per anime | Medium (provider-dependent) | Medium — Kodik players don't expose this |
| Aggregation query | Low | Low |
| Frontend wiring | None for v1 (no UI surface) | — |

## Why deferred from v1

- The math is clean but the **data does not exist yet** — needs Phase 5 instrumentation
  + a per-anime OP/ED time-window catalog.
- v1 ships with 5 signals (S1–S5 + S6 + S11 filter). Adding S9 in implicit form would
  push v1 into "build the capture infrastructure first," which is exactly what the
  current Smart Watch Picker milestone's Phase 5 is for.
- Roadmap fit: this becomes a `Phase 8` (Recommendations Readiness Documentation) callout
  recommending the gap-fill, then a v2 milestone signal.

## Cross-references

- Brainstorm context: `docs/superpowers/specs/2026-04-28-rec-engine-design.md` (forthcoming)
- Phase 2 audit: `.planning/phases/02-analytics-audit/02-DRAFT-AUDIT.md` (G-01 drop-off
  gap is structurally adjacent — same instrumentation infrastructure)
- Phase 5 candidates: G-01 (drop-off), G-02 (rewatch), G-04-lite (session_id) — adding
  intro/outro skip events fits naturally alongside these
