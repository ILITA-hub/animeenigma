# Phase 3 — Context Index

**Phase:** `03-frontend-bell-dropdown-toast`
**Workstream:** `notifications`
**Milestone:** `v1.0`
**Status:** planning → ready-for-execution

This file is the executor's reading list. Read in this order:

## Workstream-level (must read first)

1. `../../PROJECT.md` — workstream scope, locked design decisions (#1–#9), out-of-scope items
2. `../../REQUIREMENTS.md` — full text of NOTIF-UI-01..08 and NOTIF-NF-03 (this phase's requirements)
3. `../../ROADMAP.md` — Phase 3 section + the 8 success criteria (your verification matrix)

## Phase 1 & 2 (already shipped — DO NOT re-implement)

4. `../01-notifications-foundation/SUMMARY.md` — service on port 8090, 6 public routes, response envelope is `{success, data}` (`libs/httputil.JSON`), tables, auth via JWT extracted by gateway
5. `../02-detector-and-catalog-endpoint/02-SUMMARY.md` — payload semantics, watch_url shape `/anime/{id}/watch?player=X&episode=N&translation=Y` (NB: the live frontend route is `/anime/:id` with `?episode=N` query — your click handler must translate)
6. `/data/animeenigma/services/notifications/internal/handler/notification.go` — verbatim response shapes (ListResponse / UnreadCountResponse / MarkAllReadResponse)
7. `/data/animeenigma/services/notifications/internal/domain/notification.go` — `UserNotification` + `NewEpisodePayload` (translate to TS types verbatim)

## Source design doc (background — skim only)

8. `/data/animeenigma/docs/superpowers/specs/2026-05-11-notifications-engine-design.md`

## Frontend conventions (must read before writing any component)

9. `/data/animeenigma/CLAUDE.md` — frontend uses **bun** (not npm/pnpm). Playwright uses **bunx**. `make redeploy-web` for live redeploy.
10. `/data/animeenigma/.planning/CONVENTIONS.md` — score in UXΔ / CDI / MVQ, no day/hour/sprint estimates
11. `/data/animeenigma/frontend/web/package.json` — Vite + Vue 3.4 + TS + Pinia 2 + vue-i18n 11 + vue-router 4 + Tailwind 4 + axios; e2e via `@playwright/test`
12. `/data/animeenigma/frontend/web/src/App.vue` — `<Navbar />` is the header (lines 4 and 75 import); `<Toaster />` already mounted at line 53 (the new `<NotificationToast />` mounts next to it, NOT on top of it)
13. `/data/animeenigma/frontend/web/src/components/layout/Navbar.vue` — mount `<NotificationBell />` between the language selector (line 122-149) and the user-avatar block (line 152-172) for desktop; in the mobile drawer body, surface a "Notifications" link to `/notifications` (deferred — only the bell ships in v1.0, mobile users open via the desktop-style header which IS visible on mobile too at line 7 — verify during execution)
14. `/data/animeenigma/frontend/web/src/api/client.ts` — `apiClient` (axios) auto-attaches `Bearer ${token}`; response envelope unwrap pattern is `response.data?.data || response.data` (line 70-71 in auth.ts)
15. `/data/animeenigma/frontend/web/src/stores/auth.ts` — `isAuthenticated` computed, `'auth:expired'` window event for soft-logout
16. `/data/animeenigma/frontend/web/src/stores/watchlist.ts` — Pinia composition-API store pattern with auth-gated fetch
17. `/data/animeenigma/frontend/web/src/composables/useToast.ts` — existing toast queue (do NOT collide — the new NotificationToast is its own component, not a queue)
18. `/data/animeenigma/frontend/web/src/components/ui/Toaster.vue` — visual reference for toast styling (rounded-lg, shadow-lg, glass background)
19. `/data/animeenigma/frontend/web/src/router/index.ts` — current routes (`/anime/:id` is the watch route; Anime.vue parses `?episode=N` query at line 1251)
20. `/data/animeenigma/frontend/web/src/locales/en.json` + `ru.json` + `ja.json` — 958-line each, nested JSON; current top-level keys include `nav`, `system`, `watchlist`, `collections`. New keys go under a new top-level `notifications` block.
21. `/data/animeenigma/frontend/web/e2e/auth.spec.ts` + `watchlist.spec.ts` — Playwright pattern: `localStorage.setItem('token', ...)` in `beforeEach`. For real auth (NOTIF-NF-03 path) call `fetch('/api/auth/login', ...)` inside `page.evaluate` so the refresh cookie sets, then mirror into `localStorage.token` (see CLAUDE.md "Auth" guidance under UI/UX Audit Framework).

## Memory & feedback notes (live)

22. `/root/.claude/projects/-data-animeenigma/memory/MEMORY.md` — `ui_audit_bot` API key in `docker/.env`, password `audit_bot_test_password_2026`, snake_case JSON for backend
23. `feedback_replace_dont_preserve.md` — N/A here, additive feature
24. `feedback_no_days_metric.md` — score with UXΔ / CDI / MVQ

## This phase's outputs

- `./03-PLAN.md` — the executable phase plan (this is what the executor opens)
- `./03-SUMMARY.md` — created by the executor at end of phase
