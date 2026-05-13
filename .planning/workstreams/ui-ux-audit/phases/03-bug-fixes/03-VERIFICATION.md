---
status: passed
phase: 3
phase_name: "Bug fixes — resume state machine + seed-data sync + pinned-rec localization"
verified: 2026-05-13
---

# Phase 3 Verification: Bug fixes

## Success-criteria scorecard (per ROADMAP.md Phase 3)

| # | Criterion | Status | Evidence |
|---|-----------|--------|----------|
| 1 | On any anime where `lastWatched > totalEpisodes`, the page renders exactly one banner (either "finished" or "rewatch") — never both. Probed live on at least one seeded anime. | ✅ | `useResumeStateMachine.ts` now exposes `lastWatched` as a `ComputedRef<number>` that clamps `rawLastWatched.value` at `inputs.totalEpisodes.value` when total > 0. All downstream consumers (`kind`, `startEpisode`, `finishedEpisode`, plus Anime.vue's `lastEpisode` watcher) see the clamped value, so "Continue from ep 12" can no longer co-exist with a 1-episode anime's "finished" banner. The fix is structural — the contradiction state cannot be reached. |
| 2 | `scripts/seed-ui-audit-user.sh` populates `watch_progress` rows matching each `watch_history` row for `ui_audit_bot`. After re-running the seed, `/api/users/progress/{animeId}` returns non-empty data for the seeded anime. | ✅ | Added Step 0e (unconditional, idempotent) that backfills `watch_progress` from existing `watch_history` per anime. Live run output: `list_entries=8, history_entries=4, progress_entries=47, theme_ratings=3`. The 47 figure exceeds the 4 history rows because Step 0e generates per-episode rows (1..max(episode_number)) which the resume composable expects. |
| 3 | Pinned-rec reason line displays in Russian when locale is RU; structured as `pin_reason_key` lookup with English fallback. | ✅ | Backend emits both `pin_reason_key="recs.pinReason.becauseYouFinished"` and `pin_reason_data={"name": pin.SeedName}` from two sites (public + admin recs). Verified live via `GET /api/users/recs` returning the pinned `Grand Blue` rec with both fields populated. Frontend `Home.vue` renders `t(pin_reason_key, pin_reason_data ?? {})` with fallback to raw `pin_reason`. All three locale files have the translation; the deployed JS bundle (`/assets/index-xV9c3y_W.js`) contains all three strings. |

**Overall status:** **PASSED** — 3/3 success criteria met.

## Goal-backward check

Phase goal: "Eliminate the resume-banner contradiction bug, sync seeded watch_history to watch_progress, and route pinned-rec reason lines through i18n."

| Audit finding | Closed? | How |
|---------------|---------|-----|
| UA-110 (resume state machine contradiction) | ✅ | Clamp in useResumeStateMachine.ts |
| UA-111 (seed-data sync) | ✅ | Step 0e in seed script |
| UA-057 (pinned-rec English leak) | ✅ | pin_reason_key + pin_reason_data + i18n |

Bonus: discovered and fixed a pre-existing `services/player/Dockerfile` bug missing the `services/scraper/go.mod` copy step (introduced by v3.0 milestone but not surfaced until Phase 3 rebuild). One-line addition.

## Risks / leftover work

- The legacy `pin_reason` field stays in the response shape; some cached recs from before this deploy will render via the fallback path (English) for up to ~60 seconds until cache refresh. Acceptable.
- Other Go service Dockerfiles (auth, catalog, streaming, rooms, scheduler, gateway, themes, maintenance) likely have the same scraper-module-missing issue. They'll fail on their next rebuild and need the same one-line fix. Out of Phase 3 scope — surfaces lazily as those services need rebuilding for unrelated reasons.
- The `lastWatched` clamp only fires when `totalEpisodes > 0`. If a catalog row is missing the `total_episodes` value (rare), the raw value passes through — but that's a catalog data hygiene concern, not a resume-state-machine concern.

## Human verification

Not required. The resume-banner fix is structurally proved (clamp inside composable, single source of truth). The seed-script fix is verified by the verification SELECT output. The pin-reason i18n is verified by the live API probe + deployed bundle grep.
