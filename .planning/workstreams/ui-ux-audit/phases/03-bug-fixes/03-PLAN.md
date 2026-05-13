# Phase 3 Plan: Bug fixes

**Status:** Active
**Plan #:** 1 (single plan, three discrete fixes)
**Created:** 2026-05-13

## Tasks

### UX-07 — Resume state machine cap (UA-110)

- [ ] `useResumeStateMachine.ts`: rename internal `lastWatched` ref → `rawLastWatched`. Add `lastWatched = computed(() => totalEpisodes > 0 && raw > totalEpisodes ? totalEpisodes : raw)`. Update `kind`, `startEpisode`, `finishedEpisode` consumers (already use `lastWatched.value`).
- [ ] Update interface: `lastWatched: ComputedRef<number>` (was `Ref<number>`).

### UX-08 — Seed-data sync (UA-111)

- [ ] `scripts/seed-ui-audit-user.sh`: add a new Step 0e (outside the watch_history conditional) that backfills `watch_progress` from existing `watch_history` rows. Generate per-episode rows (1..max(episode_number)) per anime with `completed=TRUE`, `progress=duration`, `ON CONFLICT DO NOTHING`.
- [ ] Add `progress_entries` to the Verification counts SELECT.

### UX-09 — Pinned-rec reason i18n (UA-057)

- [ ] Backend: `services/player/internal/handler/recs.go` — extend `RecItem` with `PinReasonKey string` + `PinReasonData map[string]any`; emit them in the s6 pin construction site (line ~498).
- [ ] Backend: `services/player/internal/handler/admin_recs.go` — same fields on `AdminRecRow`, same emission at admin pin construction (line ~388).
- [ ] Frontend: `useRecs.ts` — add `pin_reason_key?: string`, `pin_reason_data?: Record<string, unknown>` to `RecItem` type.
- [ ] Frontend: `Home.vue:40-43` — render via `t(pin_reason_key, pin_reason_data ?? {})` when key present, else fall back to raw `pin_reason`.
- [ ] Frontend: add `recs.pinReason.becauseYouFinished` key in en/ru/ja locale files.

### Verification

- [ ] `go build ./...` in `services/player/` — passes.
- [ ] `bunx vue-tsc --noEmit` in `frontend/web/` — passes.
- [ ] `make redeploy-player` succeeds (after fixing the pre-existing Dockerfile bug missing `COPY services/scraper/go.mod` — see Notes).
- [ ] `make redeploy-web` — frontend rebuilt and shipped.
- [ ] Run seed script — produces non-zero `progress_entries` count in verification output.
- [ ] Probe `/api/users/recs` as `ui_audit_bot` — confirm pinned item has `pin_reason_key="recs.pinReason.becauseYouFinished"` and `pin_reason_data.name` set.
- [ ] Probe deployed JS bundle for all 3 locale translations of `becauseYouFinished`.

## Notes

- Discovered a pre-existing build bug while redeploying: `services/player/Dockerfile` was missing `COPY services/scraper/go.mod` despite the v3.0 milestone adding the scraper service. This blocked `go mod download` because go.work references `./services/scraper`. Fixed inline (one-line addition) — Phase 3 is the first phase that actually rebuilds player after the scraper service was introduced, so it's the first time the bug surfaces.

## Files touched

```
frontend/web/src/composables/useResumeStateMachine.ts    # UX-07: rename + clamp
frontend/web/src/composables/useRecs.ts                  # UX-09: type
frontend/web/src/views/Home.vue                          # UX-09: render
frontend/web/src/locales/en.json                         # UX-09: key
frontend/web/src/locales/ru.json                         # UX-09: key
frontend/web/src/locales/ja.json                         # UX-09: key
scripts/seed-ui-audit-user.sh                            # UX-08: backfill
services/player/internal/handler/recs.go                 # UX-09: backend
services/player/internal/handler/admin_recs.go           # UX-09: backend
services/player/Dockerfile                               # pre-existing scraper module fix
.planning/workstreams/ui-ux-audit/phases/03-bug-fixes/
  03-CONTEXT.md
  03-PLAN.md
  03-SUMMARY.md
  03-VERIFICATION.md
```
