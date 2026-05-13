# Phase 3: Bug fixes — resume state + seed-data sync + pinned-rec i18n - Context

**Gathered:** 2026-05-13
**Status:** Ready for planning
**Mode:** Auto-generated (autonomous run, three discrete bug fixes with concrete audit citations)

<domain>
## Phase Boundary

Three independent bug fixes from the 2026-05-12 audit, bundled into one phase because each is small and they share the test-user surface:

1. **UX-07 / UA-110 — resume state machine contradiction.** On any anime where `lastWatched > totalEpisodes` (e.g. Chainsaw Man: Reze, with 1 cataloged episode but 12 completed eps in DB for `ui_audit_bot`), the page renders two contradictory banners: "Continue from ep 12" alongside "You finished — rewatch from ep 1". Cap `lastWatched` at `totalEpisodes` so all downstream computeds + the lastEpisode binding stay coherent.
2. **UX-08 / UA-111 — seed-data sync.** `scripts/seed-ui-audit-user.sh` populates `watch_history` for the audit bot, but `useResumeStateMachine.ts` reads from `/api/users/progress` which queries `watch_progress`. The two never get synced, so resume banners don't render on seeded data. Backfill `watch_progress` from `watch_history` in the seed script, idempotent on re-run.
3. **UX-09 / UA-057 — pinned-rec reason line localization.** Backend emits `pin_reason: "Because you finished {seed}"` as a raw English string. Add `pin_reason_key` + `pin_reason_data` fields so the frontend can route through `$t()` and render in RU/EN/JA. Keep `pin_reason` for back-compat with any cached payloads.

</domain>

<decisions>
## Implementation Decisions

### Claude's Discretion (autonomous mode)

- **UX-07 fix location**: cap inside `useResumeStateMachine.ts` rather than at each consumer. Convert the internal raw value to `rawLastWatched` (ref) and expose `lastWatched` as a `ComputedRef<number>` that clamps to `inputs.totalEpisodes.value`. Single source of truth for downstream computeds (`kind`, `startEpisode`, `finishedEpisode`) and consumers like Anime.vue's `lastEpisode` binding.
- **UX-08 idempotency**: split into two phases — Step 0d (the existing watch_history seed) is gated by `watch_history` count==0; new Step 0e (watch_progress backfill) runs UNCONDITIONALLY with `ON CONFLICT DO NOTHING`. This way users seeded BEFORE this fix get their watch_progress backfilled on re-run.
- **UX-09 backend shape**: add `pin_reason_key` (string) + `pin_reason_data` (map[string]any) alongside the existing `pin_reason`. Two emission sites: `services/player/internal/handler/recs.go` (public endpoint) and `services/player/internal/handler/admin_recs.go` (admin debug). Single i18n key `recs.pinReason.becauseYouFinished` parameterized by `{name}`.
- **UX-09 frontend fallback**: render `$t(pin_reason_key, pin_reason_data ?? {})` when the key is present; fall back to raw `pin_reason` otherwise. Don't break old cached responses or non-S6 future pin sources.

### Locked from ROADMAP

- All three fixes ship together as Phase 3 (depends only on Phase 1).
- No new dependency surface — keep within existing patterns (state machine composable, seed bash script, JSON envelope + i18n catalog).

</decisions>

<code_context>
## Existing Code Insights

- `frontend/web/src/composables/useResumeStateMachine.ts` — has `lastWatched: Ref<number>` already; consumers read `.value`. Conversion to `ComputedRef` is type-compatible.
- `frontend/web/src/views/Anime.vue:915` — `watch(() => resume.lastWatched.value, ...)` sets `lastEpisode.value = n` for the resumeAuth path. Once `lastWatched` is clamped at the source, this auto-corrects.
- `scripts/seed-ui-audit-user.sh` — bash with embedded SQL heredocs; CONFLICT-DO-NOTHING already used for anime_list and theme_ratings. Backfill pattern proven.
- `services/player/internal/handler/recs.go` — two pin emission sites (line 498 in `RecItem` build; and admin_recs.go line 396 for `AdminRecRow`). Both must emit the new fields.
- `frontend/web/src/composables/useRecs.ts` — `RecItem` TypeScript type lives here; add the two optional fields.
- `frontend/web/src/views/Home.vue:40-43` — single render site for the pin reason line. The `useRecs` composable's `RecItem` is the shared shape with `useAdminRecs.ts`.

</code_context>

<specifics>
## Specific Ideas

- Russian translation: "Потому что вы посмотрели {name}" matches the YouTube/Netflix RU localization pattern.
- Japanese: "{name}を観終わったので" — name precedes the particle; verb-final structure preserved.
- For UX-08: backfill all episodes 1..max(episode_number) per anime (not just the highest one), because the resume state machine expects per-episode rows and `/api/users/progress/{animeId}` returns the array.

</specifics>

<deferred>
## Deferred Ideas

- Backend pin_reason cache TTL purge if anyone cares about the legacy English string showing for ~60 seconds after a deploy — not worth the complexity; cached responses pre-fix will fall back to raw `pin_reason` and render in English for those ~60s, then refresh.
- A general-purpose `t_key` / `t_data` pattern across other backend-emitted user-facing strings — out of scope for Phase 3; revisit if more leaks surface.

</deferred>
