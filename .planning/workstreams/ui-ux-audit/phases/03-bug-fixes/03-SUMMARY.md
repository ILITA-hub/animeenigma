# Phase 3 Summary: Bug fixes

**Completed:** 2026-05-13
**Plan:** 03-PLAN.md
**Outcome:** All three bug fixes shipped. One pre-existing Dockerfile bug fixed inline.

## Changes shipped

### UX-07 — Resume state machine cap (UA-110)

`useResumeStateMachine.ts` — rename internal `lastWatched` ref to `rawLastWatched`. Expose `lastWatched` as a `ComputedRef<number>` that clamps the raw value at `inputs.totalEpisodes.value` whenever total > 0. All downstream computeds (`kind`, `startEpisode`, `finishedEpisode`) plus consumer bindings (Anime.vue's `lastEpisode` watcher) now see a coherent value. The "Continue from ep 12" + "You finished, rewatch" double-banner is eliminated structurally — the raw-vs-clamped distinction lives entirely inside the composable.

### UX-08 — Seed-data sync (UA-111)

`scripts/seed-ui-audit-user.sh` — added Step 0e outside the watch_history-empty guard. Backfills `watch_progress` from existing `watch_history` rows with `generate_series(1, max(episode_number))` per anime, marking each `completed=TRUE`. Idempotent via `ON CONFLICT (user_id, anime_id, episode_number) DO NOTHING`. Re-running the seed script on existing seeded users now populates the previously-missing watch_progress rows.

Verification SELECT updated to include `progress_entries`. After running the updated script for `ui_audit_bot`: `list_entries=8, history_entries=4, progress_entries=47, theme_ratings=3` (progress_entries combined newly-seeded + pre-existing rows).

### UX-09 — Pinned-rec reason i18n (UA-057)

Backend (`services/player/internal/handler/`):
- `recs.go` — `RecItem` extended with `PinReasonKey string` + `PinReasonData map[string]any`. The s6 pin emission at line 498 now sets both: `PinReasonKey: "recs.pinReason.becauseYouFinished"`, `PinReasonData: {"name": pin.SeedName}`. Legacy `PinReason` ("Because you finished ${seed}") preserved for any cached/stale consumers.
- `admin_recs.go` — same fields on `AdminRecRow`, same emission at line ~396 (admin debug surface mirrors the public recs response).

Frontend:
- `useRecs.ts` — `RecItem` type extended with `pin_reason_key?: string` and `pin_reason_data?: Record<string, unknown>`.
- `Home.vue` — pin reason render upgraded to `t(pin_reason_key, pin_reason_data ?? {})` when the key is present, falling back to the legacy raw `pin_reason` otherwise.
- `locales/{en,ru,ja}.json` — added `recs.pinReason.becauseYouFinished` with translations:
  - EN: `"Because you finished {name}"`
  - RU: `"Потому что вы посмотрели {name}"`
  - JA: `"{name}を観終わったので"`

End-to-end probe: `GET /api/users/recs` as `ui_audit_bot` returns the pinned item with `pin_reason_key=recs.pinReason.becauseYouFinished` and `pin_reason_data={name: "Grand Blue"}`. All three locale strings are present in the deployed JS bundle.

### Dockerfile fix (incidental)

`services/player/Dockerfile` was missing `COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/`. The v3.0 milestone added the scraper service to `go.work`, but the player Dockerfile wasn't updated. Phase 3 was the first phase to rebuild player after that, so the bug surfaced now. One-line addition before the `go mod download` step.

## Verification

See `03-VERIFICATION.md` for the success-criteria scorecard.

## Files touched

```
frontend/web/src/composables/useResumeStateMachine.ts    # +13 / -3 (clamp + ComputedRef)
frontend/web/src/composables/useRecs.ts                  # +6 / -4 (type)
frontend/web/src/views/Home.vue                          # +6 / -3 (render)
frontend/web/src/locales/en.json                         # +3 (key)
frontend/web/src/locales/ru.json                         # +3 (key)
frontend/web/src/locales/ja.json                         # +3 (key)
scripts/seed-ui-audit-user.sh                            # +28 / -4 (Step 0e backfill)
services/player/internal/handler/recs.go                  # +5 / -1 (type + emission)
services/player/internal/handler/admin_recs.go            # +5 / -1 (type + emission)
services/player/Dockerfile                                # +1 (scraper module copy)
.planning/workstreams/ui-ux-audit/phases/03-bug-fixes/
  03-CONTEXT.md      (new)
  03-PLAN.md         (new)
  03-SUMMARY.md      (this file)
  03-VERIFICATION.md (new)
```

## Notes for downstream phases

- The `pin_reason_key` + `pin_reason_data` pattern is the reference for any future backend-emitted user-facing strings that need localization. Phase 14 (marketing-surface polish) and Phase 19 (Grafana rebuild) may revisit this for status banners / follower-count copy.
- The Dockerfile scraper fix should be backported to other service Dockerfiles next time they're touched (auth, catalog, streaming, rooms, scheduler, gateway, themes, maintenance). Out of scope for Phase 3 — these will resolve themselves as those services next rebuild for their own reasons.
