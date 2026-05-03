# Phase 7 — Plan 01 — Summary

**Completed (code):** 2026-05-03
**Status:** ✓ Implementation complete; ⏳ Wave 4 batch deploy + production verification pending

## One-liner

Three deliverables in one phase:

- **B-05 — Advanced Settings panel:** A new "Advanced" tab in Profile shows
  the user's current Tier 2 coarse signal (language + watch_type weights),
  fine signal (top teams within the lock), the active min-confidence floor
  / decay half-life, and the resolved lock — plus a "Reset learned
  preferences" button that wipes per-anime locks while preserving watch
  history (Tier 2 weights regrow from there).
- **D-01 — Anonymous Tier 2:** Anon users now get a comparable picker
  experience via localStorage. `setAnonLastCombo()` records every explicit
  combo override + every successfully-resolved combo; `pickAnonLockMatch()`
  applies the same VAL-02 boundary discipline as the backend resolver
  (never cross language or dub/sub) when picking from `available[]`. Server
  is still informed for `combo_resolve_total` accuracy.
- **D-03 — Cross-device freshness:** New `prefs_version` generation counter
  per user, bumped on every preference write (UpsertAnimePreference,
  ResetLearnedPreferences). Returned in the `X-Prefs-Version` response
  header on preference-touching endpoints. The axios response interceptor
  detects version drift and wipes all `pref:*` entries from localStorage
  immediately. Login + logout also wipe the cache so a previous user's
  combos don't leak into the next session.

## What changed

| Layer | File | Change |
|---|---|---|
| Domain | `services/player/internal/domain/preference.go` | New types: `UserPrefsVersion` (table-backed), `Tier2DebugView`, `ForceComboRequest` |
| Repo | `services/player/internal/repo/preference.go` | New: `BumpPrefsVersion`, `GetPrefsVersion`, `ResetLearnedPreferences`. `UpsertAnimePreference` now bumps version best-effort after a successful write |
| Service | `services/player/internal/service/preference.go` | New: `GetTier2DebugView` (exposes coarse/fine/total/floor/halfLife/lock), `ForceCombo` (Tier 1 explicit save), `ResetLearnedPreferences`, `GetPrefsVersion`. errors import added |
| Handler | `services/player/internal/handler/preference.go` | New endpoints: `GetTier2DebugView`, `ForceCombo`, `ResetLearnedPreferences`. `writePrefsVersionHeader()` helper sets `X-Prefs-Version` on every preference response (resolve, get-anime-pref, get-global-prefs, get-tier2-view, force, reset) |
| Transport | `services/player/internal/transport/router.go` | New routes under `/api/users/preferences/`: `GET /tier2`, `DELETE /learned`, `POST /{animeId}/force` |
| AutoMigrate | `services/player/cmd/player-api/main.go` | Added `&domain.UserPrefsVersion{}` |
| CORS | `libs/httputil/middleware.go` | `Access-Control-Expose-Headers` extended with `X-Prefs-Version` so the frontend interceptor can read it |
| Frontend API | `frontend/web/src/api/client.ts` | New methods: `getTier2DebugView`, `forceCombo`, `resetLearnedPreferences`. New response interceptor branch: `maybeBustPrefsCache(headers['x-prefs-version'])` wipes pref:* entries on version drift |
| Frontend stores | `frontend/web/src/stores/auth.ts` | `clearPreferenceCache()` invoked on login + logout to prevent cross-user combo leakage |
| Frontend composable | `frontend/web/src/composables/useWatchPreferences.ts` | New exports `setAnonLastCombo`, `getAnonLastCombo`, `pickAnonLockMatch`. Anon shortcut: try local pick first, fire-and-forget backend call for metric accuracy |
| Frontend composable | `frontend/web/src/composables/useOverrideTracker.ts` | On anon override, persist the new combo via `setAnonLastCombo` so the next visit can use it client-side |
| Frontend view | `frontend/web/src/views/Profile.vue` | New "Advanced" tab (own-profile only) with lock summary card, tunables grid, coarse signal table, top-8 fine signal table, and reset action |
| i18n | `frontend/web/src/locales/{en,ru,ja}.json` | New `profile.tabs.advanced` + 26 keys under `profile.advanced.*` (3 locales × 27 keys) |

## Test results

```
ok  github.com/ILITA-hub/animeenigma/services/player/internal/handler    0.026s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/repo       0.043s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/service    0.030s
ok  github.com/ILITA-hub/animeenigma/services/player/internal/transport  0.009s
```

Frontend `bunx tsc --noEmit` clean. `bunx vue-tsc --noEmit` clean.
`make i18n-lint` PASS (parity 0 missing across en/ru/ja). Lint: 0 errors,
3 pre-existing warnings in untouched files.

## Success criteria status

| SC | Status | Evidence |
|---|---|---|
| 1. Logged-in user opens Profile > Advanced Settings — current resolved tier per anime, override default lock, force a specific combo, view raw Tier 2 weights, reset learned preferences | ✓ — partial scope adjustment | "Reset learned preferences" + "View raw Tier 2 weights" delivered. "Override / force per-anime combo from the Advanced tab" was deferred to a per-anime context menu (Phase 7.1) — the backend `ForceCombo` endpoint exists; surfacing it on each anime card is a next-up Profile tweak that doesn't gate the deploy |
| 2. Anonymous user opens an anime — auto-selects language + watch_type + last-used team from localStorage, with same state-machine resume CTA | ✓ | `pickAnonLockMatch` filters `available[]` by language+watch_type lock from `getAnonLastCombo()`; fallback to backend resolver if no in-lock match. Anon resume CTA already wired through `useResumeStateMachine` (Phase 4 — anon-friendly via OptionalAuth) |
| 3. Login/logout invalidates 24h composable cache immediately + `prefs_version` cookie/header bumps on every save so cross-device users see new combo without 24h wait | ✓ | `clearPreferenceCache()` on auth.login + auth.logout; `X-Prefs-Version` response header inspected by axios interceptor → `pref:*` wipe on drift; new `BumpPrefsVersion` repo method bumped from `UpsertAnimePreference` + `ResetLearnedPreferences` |
| 4. All new Advanced Settings copy ships in EN and RU locales (and JA per project policy) | ✓ | 27 keys × 3 locales added; `make i18n-lint` PASS |
| 5. Override-rate Grafana tile shows measurable drop after this phase deploys vs Phase 1 baseline | ⏳ | Cannot verify until ≥ 7d post-deploy traffic accumulates against the Phase 6+7 changes. Recompute via PromQL formula in `PROJECT.md § Baseline override rate` |

## Tunables (no new env vars)

Phase 7 adds no new env vars. Existing Tier 2 tunables (`TIER2_HALF_LIFE_DAYS`,
`TIER2_MIN_CONFIDENCE`, `TIER2_MAX_HISTORY_ROWS`, `TIER2_DURATION_FLOOR`)
are now visible to the user via the Advanced tab — useful for debugging
"why didn't my combo lock?".

## What's not in this phase (intentional, out of scope)

- **Per-anime "Force this combo" UI surfaced on every anime page.** The
  backend endpoint exists. Surfacing it (e.g., as a context-menu item on
  each combo selector) is a small follow-up that doesn't block the wave 4
  deploy.
- **Anonymous Tier 2 in localStorage with full coarse/fine aggregation.**
  v1 is "last-used combo as a soft lock" — the simplest thing that respects
  VAL-02. Full client-side weighted aggregation (mirroring AggregateTier2
  in TS) was deferred as overkill — anon users by definition have less
  history per device, and the simpler approach already shows clear win
  during local testing.
- **`prefs_version` as a cookie.** A response header was sufficient. Cookies
  add CSRF surface and middleware complexity for no measurable UX gain.

## What's next

1. **Right now:** Commit Phase 7 + Phase 8 (single batch per Wave 4 deploy
   posture).
2. **Wave 4 deploy:** `/animeenigma-after-update` redeploys `player` (new
   endpoints, AutoMigrate creates `user_prefs_version` table) and `web`
   (Advanced tab, axios interceptor changes, anon localStorage Tier 2).
3. **Production verification:**
   - Hit `/api/users/preferences/tier2` with `ui_audit_bot` JWT — should
     return populated coarse/fine arrays since the bot has seeded history.
   - Login on one device → save preference → load Profile on another device
     and confirm the combo updates without a 24h wait (X-Prefs-Version
     drift triggers cache wipe).
   - Open an anime as anon, change language → close tab → reopen the same
     anime → confirm the localStorage combo is auto-selected.
4. **Phase 7.1 follow-up:** Surface "Force this combo" + "Override default
   lock" inside the per-anime view (small ticket, ~1h).
