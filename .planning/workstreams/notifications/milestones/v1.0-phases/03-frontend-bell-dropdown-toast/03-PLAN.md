---
phase: 03-frontend-bell-dropdown-toast
plan: 03
workstream: notifications
milestone: v1.0
type: execute
depends_on:
  - 01-notifications-foundation
  - 02-detector-and-catalog-endpoint
files_modified:
  - frontend/web/src/App.vue
  - frontend/web/src/components/layout/Navbar.vue
  - frontend/web/src/locales/en.json
  - frontend/web/src/locales/ru.json
  - frontend/web/src/locales/ja.json
  - frontend/web/src/router/index.ts
  - frontend/web/.env.example
  - docker/.env.example
files_created:
  - frontend/web/src/types/notification.ts
  - frontend/web/src/api/notifications.ts
  - frontend/web/src/stores/notifications.ts
  - frontend/web/src/lib/relativeTime.ts
  - frontend/web/src/lib/notification-renderers.ts
  - frontend/web/src/components/NotificationBell.vue
  - frontend/web/src/components/NotificationDropdown.vue
  - frontend/web/src/components/NotificationToast.vue
  - frontend/web/src/components/notifications/NewEpisodeCard.vue
  - frontend/web/src/components/notifications/UnknownNotificationCard.vue
  - frontend/web/e2e/notifications.spec.ts
autonomous: true
requirements:
  - NOTIF-UI-01
  - NOTIF-UI-02
  - NOTIF-UI-03
  - NOTIF-UI-04
  - NOTIF-UI-05
  - NOTIF-UI-06
  - NOTIF-UI-07
  - NOTIF-UI-08
  - NOTIF-NF-03
score:
  UXΔ: "+4 (Better)"
  CDI: "0.03 * 8"
  MVQ: "Griffin 88%/85%"
must_haves:
  truths:
    - "Logged-in user with one unread notification sees a red badge on the header bell within ≤ 60s of mount."
    - "Clicking the bell opens a dropdown rendering one NewEpisodeCard with poster + title + ep range + translation source + relative-time."
    - "Clicking the card POSTs /click and navigates to the watch URL (/anime/{id}?episode=N&player=X&translation=Y) for the right episode."
    - "Foregrounded toast slides in once per session per notification; auto-hides at 8s; pauses on hover."
    - "Toast is suppressed when the user is already on the matching anime's route (route param `id` === payload.anime_id)."
    - "Mark-all-read clears badge to 0 atomically; dismiss removes the row from the dropdown and triggers POST /:id/dismiss."
    - "Logged-out user: bell is NOT rendered and zero /api/notifications calls fire."
    - "Tab hidden → polling pauses within ~1s. Tab regains visibility → immediate fetch then resume 60s interval."
    - "Logout calls stopPolling() + clears in-memory state — no further /api/notifications calls."
    - "Unknown payload type renders UnknownNotificationCard in the dropdown; toast suppresses unknown types entirely."
    - "Feature flag VITE_NOTIFICATIONS_ENABLED=false hides the bell and prevents store from starting polling (rollback safety)."
  artifacts:
    - path: "frontend/web/src/types/notification.ts"
      provides: "UserNotification, NewEpisodePayload, NotificationListResponse, UnreadCountResponse TS types — mirrors services/notifications/internal/domain/notification.go verbatim"
    - path: "frontend/web/src/api/notifications.ts"
      provides: "Typed axios wrappers for the 6 public routes; unwraps {success,data} envelope consistently"
    - path: "frontend/web/src/stores/notifications.ts"
      provides: "useNotificationsStore Pinia store (state + actions + getters + polling lifecycle)"
    - path: "frontend/web/src/lib/notification-renderers.ts"
      provides: "renderers: Record<string, Component> registry — keyed by payload.type; default UnknownNotificationCard"
    - path: "frontend/web/src/lib/relativeTime.ts"
      provides: "formatRelativeTime(iso, locale, t) — pure function, no new dependency, uses Intl.RelativeTimeFormat"
    - path: "frontend/web/src/components/NotificationBell.vue"
      provides: "Header-mounted bell with badge; opens dropdown on click; ARIA-labeled"
    - path: "frontend/web/src/components/NotificationDropdown.vue"
      provides: "Anchored scrollable list with empty-state, mark-all-read footer, outside-click + Esc close"
    - path: "frontend/web/src/components/NotificationToast.vue"
      provides: "Slide-in toast with 8s auto-hide, pause-on-hover, suppression by route param"
    - path: "frontend/web/src/components/notifications/NewEpisodeCard.vue"
      provides: "Renderer for type='new_episode' — poster 52x72, title, range, source, dismiss x"
    - path: "frontend/web/src/components/notifications/UnknownNotificationCard.vue"
      provides: "Graceful fallback renderer for unrecognized types"
    - path: "frontend/web/e2e/notifications.spec.ts"
      provides: "Playwright spec covering logged-out + logged-in + click-navigate + dismiss + tab-hide + unknown-type"
  key_links:
    - from: "App.vue onMounted + watch(authStore.isAuthenticated)"
      to: "notificationsStore.startPolling() / stopPolling() + fetchUnread()"
      via: "watch effect; gated on VITE_NOTIFICATIONS_ENABLED"
    - from: "Navbar.vue header right-section"
      to: "<NotificationBell />"
      via: "single import + element insertion between language selector and avatar"
    - from: "NotificationBell.vue click"
      to: "NotificationDropdown.vue open state"
      via: "local ref toggle in NotificationBell, dropdown teleported below bell"
    - from: "NewEpisodeCard.vue click"
      to: "notificationsStore.handleClick(notification)"
      via: "POST /:id/click then router.push(translateWatchUrl(payload.watch_url))"
    - from: "translateWatchUrl helper"
      to: "vue-router /anime/:id route"
      via: "parse /anime/{id}/watch?player=X&episode=N&translation=Y into { path: '/anime/'+id, query: { episode, player, translation } } — frontend uses /anime/:id with episode= query (see Anime.vue:1251)"
    - from: "store.startPolling()"
      to: "60s setInterval + visibilitychange listener"
      via: "lifecycle hooks; immediate fetchUnread() on visibility regain"
---

# Phase 3 — Frontend: Bell + Dropdown + Toast + Pinia Store + Renderer Registry + i18n

## Goal

Make the notifications engine visible. After this phase, a logged-in user with 3–8 ongoing shows in `anime_list.status = 'watching'` will see a red badge on the header bell within ≤ 60s of the hourly detector materializing a `new_episode` row for one of their tracked combos. If the tab is foregrounded, a toast slides in (once per session per notification). One click on either the toast or the dropdown card takes them to the right episode on the right player and translation — the same `(player, language, watch_type, translation_id)` combo they were already watching with. No new backend work; everything ships on top of Phase 1 + Phase 2's live API. The renderer registry pattern is what makes a future `new_comment` or `system_announcement` type in v1.1 additive-only: a new renderer + a new payload schema, with zero changes to bell, dropdown, toast, or store.

## Requirements covered

| Req            | Component / file                              |
| -------------- | --------------------------------------------- |
| NOTIF-UI-01    | `stores/notifications.ts`                     |
| NOTIF-UI-02    | `components/NotificationBell.vue`             |
| NOTIF-UI-03    | `components/NotificationDropdown.vue`         |
| NOTIF-UI-04    | `components/NotificationToast.vue`            |
| NOTIF-UI-05    | `components/notifications/NewEpisodeCard.vue` |
| NOTIF-UI-06    | `lib/notification-renderers.ts` + `components/notifications/UnknownNotificationCard.vue` |
| NOTIF-UI-07    | `locales/{en,ru,ja}.json` `notifications.*`   |
| NOTIF-UI-08    | `App.vue` + `components/layout/Navbar.vue` (one-line bell insertion) |
| NOTIF-NF-03    | `e2e/notifications.spec.ts` + SUMMARY E2E doc |

## UI design decisions (with rationale)

> Each decision is locked here so the executor has no scope to re-debate styling. All decisions follow the project's existing Navbar.vue + Toaster.vue + Modal.vue patterns to keep visual coherence.

1. **D-UI-01 — Bell icon source.** No icon library installed; the project uses inline SVG `path` elements directly (see Navbar.vue lines 49, 67, 115, 129, 184 — search/close/chevron/menu icons are all hand-written SVG paths). Mirror this. Use the outlined Heroicons-style bell path:
   ```
   <path d="M15 17h5l-1.4-1.4A2 2 0 0118 14.2V11a6 6 0 10-12 0v3.2a2 2 0 01-.6 1.4L4 17h5m6 0a3 3 0 11-6 0" />
   ```
   `stroke="currentColor"` `stroke-width="2"` `stroke-linecap="round"` `stroke-linejoin="round"` — matches the existing `w-5 h-5` sizing of sibling header icons. No npm dependency added.

2. **D-UI-02 — Badge color + position.** No existing badge precedent in the navbar (search button, language pill, avatar are all unbadged). Use Tailwind `bg-pink-500 text-white` for the badge (matches `text-pink-400` used in App.vue line 9 for error highlighting and `text-pink-400` for auth errors). Positioned absolute top-right of the bell, `-top-1 -right-1`, `min-w-[18px] h-[18px] rounded-full px-1 text-[10px] font-semibold leading-none flex items-center justify-center`. Hidden when `unreadCount === 0` (`v-if`, not `v-show` — no DOM cost). Render `99+` when `unreadCount > 99`.

3. **D-UI-03 — Toast position + animation.** Existing `Toaster.vue` lives at `fixed top-20 right-4 z-50` with a `translateX(20px)` slide. **DO NOT collide:** mount the new `NotificationToast` at a non-overlapping position. Desktop (≥768px): `fixed bottom-6 right-6 z-50 w-[360px]`. Mobile (<768px): `fixed top-16 left-3 right-3 z-50` (clears the 64px-tall navbar). Animation: 200ms ease-out slide-in (`translateY(20px) opacity-0 → translateY(0) opacity-100`), 150ms slide-out. Auto-hide 8000ms; pause-on-hover via `@mouseenter`/`@mouseleave` toggling a paused ref that gates `setTimeout` cleanup. No new dependency. Use Tailwind classes + `<style scoped>` for keyframes — same pattern as Toaster.vue.

4. **D-UI-04 — Dropdown styling.** Mirror the existing language-switcher dropdown (Navbar.vue line 134-148): `bg-gray-950/95 backdrop-blur-xl border border-white/10 shadow-2xl rounded-xl`. Desktop width `w-[380px]`; mobile width `w-[calc(100vw-1.5rem)]`. Max height `max-h-[480px] overflow-y-auto` for the list region. Anchored right-edge to the bell via `absolute right-0 top-full mt-2`. Close on outside-click (use `onClickOutside` from `@vueuse/core` — already in use in Navbar.vue:464). Close on Esc via a global `useEventListener(document, 'keydown', ...)` (same pattern as Navbar.vue line 362). Use the existing `dropdown` Transition (already defined in Navbar.vue line 484-491 — copy-paste the named transition into NotificationBell.vue's `<style scoped>`).

5. **D-UI-05 — Polling strategy.** 60s `setInterval` started in `startPolling()`. Listener: `document.addEventListener('visibilitychange', onVisibilityChange)` set up at store-init time. On hidden: `clearInterval` + flip `paused=true`. On visible: immediate `fetchUnread()` then re-arm `setInterval(60_000)`. Single-flight guard on `fetchUnread()` (in-flight ref) to prevent overlapping requests if visibility-regain races a tick. Initial fetch (line-zero on auth-success) runs immediately, then the 60s interval ticks.

6. **D-UI-06 — Auth state listener.** In `App.vue`'s `<script setup>`, use `watch(() => authStore.isAuthenticated, (v) => v ? notificationsStore.start() : notificationsStore.stop(), { immediate: true })`. The store's `start()` is idempotent (no-op if already polling), and `stop()` clears the interval, removes the visibility listener, drops `notifications=[]`, `unreadCount=0`, and `shownToastIds=new Set()`. Also wire `window.addEventListener('auth:expired', stop)` inside the store init (the auth client dispatches this on confirmed 401 from `/auth/refresh` — see `api/client.ts` line 63-67).

7. **D-UI-07 — Response envelope unwrap.** `services/notifications` uses `libs/httputil.JSON` → every response is `{success: bool, data: T}`. Mirror the existing project pattern: `response.data?.data ?? response.data` (see `stores/auth.ts:70`, `stores/watchlist.ts:46`). Centralize in `api/notifications.ts` so callers never see the envelope. All 6 functions return the unwrapped `T`.

8. **D-UI-08 — Relative-time helper.** No existing helper found at `frontend/web/src/utils/` or `frontend/web/src/composables/`. Add `frontend/web/src/lib/relativeTime.ts` as a tiny pure-function module (~30 lines) using `Intl.RelativeTimeFormat` (zero new dependency, supported in all targeted browsers). Signature: `formatRelativeTime(iso: string, locale: 'en' | 'ru' | 'ja'): string`. Thresholds: <60s → "just now" (use i18n key), <60min → `N min ago` (via `Intl.RelativeTimeFormat(locale, {numeric: 'auto'}).format(-n, 'minute')`), <24h → hours, <30d → days, else absolute date. The "just now" string comes from `notifications.time.justNow` i18n key; everything else from `Intl.RelativeTimeFormat` which handles locale-correct plurals natively.

9. **D-UI-09 — Playwright e2e scope.** Run against `BASE_URL=https://animeenigma.ru` (production-like) or the local dev server. Login via real `POST /api/auth/login` inside `page.evaluate` (uses `audit_bot_test_password_2026`, mirrors auth client's refresh-cookie semantics — see CLAUDE.md UI Audit Framework). Cover:
   - **TC-01** logged-out: bell NOT rendered; no `/api/notifications` request fires in 5s (verified via `page.on('request')`).
   - **TC-02** logged-in zero notifications: bell renders, no badge; one `/api/notifications?status=unread` fires on mount; second fires ~60s later.
   - **TC-03** logged-in with seed: run `scripts/seed-notification-for-ui-audit-user.sh` in `beforeAll`, reload page; badge shows "1" within 60s; toast slides in once; clicking toast navigates to `/anime/{id}` with `?episode=`, `?player=`, `?translation=` query params; toast is then absent from a second visit-and-reload in the same session.
   - **TC-04** toast suppression: navigate to `/anime/{anime_id}` first, seed notification, reload, verify NO toast appears (badge still appears).
   - **TC-05** mark-all-read: open dropdown, click mark-all-read, badge → 0, dropdown shows empty state.
   - **TC-06** dismiss: open dropdown, click ×, row disappears from list, badge → 0.
   - **TC-07** tab hide: `await page.evaluate(() => Object.defineProperty(document, 'hidden', {value: true, configurable: true}); document.dispatchEvent(new Event('visibilitychange')))`, wait 65s, verify only ONE additional request was made on the regain (not two from the missed tick + the regain). Then unhide and verify immediate fetch.
   - **TC-08** unknown type: insert via `docker compose exec` a notification with `type='future_type'`, open dropdown, verify UnknownNotificationCard renders, verify NO toast appears for it.

   Use `bunx playwright test e2e/notifications.spec.ts` per CLAUDE.md tooling rule.

## Touch list (with exact paths)

**New (10 files):**

| Path | Purpose |
|------|---------|
| `frontend/web/src/types/notification.ts` | TS mirror of `services/notifications/internal/domain` |
| `frontend/web/src/api/notifications.ts` | Typed axios wrappers, envelope-unwrap |
| `frontend/web/src/stores/notifications.ts` | Pinia store, polling lifecycle, click handler |
| `frontend/web/src/lib/relativeTime.ts` | Pure helper, `Intl.RelativeTimeFormat` |
| `frontend/web/src/lib/notification-renderers.ts` | `Record<string, Component>` registry |
| `frontend/web/src/components/NotificationBell.vue` | Header bell + badge + dropdown anchor |
| `frontend/web/src/components/NotificationDropdown.vue` | Dropdown list + empty state + mark-all-read |
| `frontend/web/src/components/NotificationToast.vue` | App-root toast + suppression logic |
| `frontend/web/src/components/notifications/NewEpisodeCard.vue` | Renderer for `type='new_episode'` |
| `frontend/web/src/components/notifications/UnknownNotificationCard.vue` | Graceful fallback renderer |

**Modified (8 files):**

| Path | Change |
|------|--------|
| `frontend/web/src/App.vue` | Add `<NotificationToast v-if="notifEnabled" />` next to `<Toaster />`; add `watch(authStore.isAuthenticated, ...)` to start/stop store |
| `frontend/web/src/components/layout/Navbar.vue` | Insert `<NotificationBell v-if="notifEnabled" />` between language selector (line 149) and avatar (line 152) on desktop; insert mobile-drawer entry under the divider (line 218) |
| `frontend/web/src/router/index.ts` | Add `/anime/:id/watch` route as an alias that internally redirects to `/anime/:id` preserving query params (so `payload.watch_url` resolves without 404) |
| `frontend/web/src/locales/en.json` | New top-level `notifications.*` block (~12 keys) |
| `frontend/web/src/locales/ru.json` | Same keys, RU strings |
| `frontend/web/src/locales/ja.json` | Same keys, JA strings |
| `frontend/web/.env.example` | Add `VITE_NOTIFICATIONS_ENABLED=true` |
| `docker/.env.example` | Add `VITE_NOTIFICATIONS_ENABLED=true` (for the web build args block, if present; document the flag either way) |

**E2E new (1 file):**

| Path | Purpose |
|------|---------|
| `frontend/web/e2e/notifications.spec.ts` | TC-01..08 above |

**Total:** 10 new components/modules + 1 new e2e spec + 8 modifications.

## Wave / Task breakdown

### Wave 1 — Foundations (parallelizable, no UI yet)

**Task 1.1 — Types + i18n keys + relative-time helper + env flag**
*Files:* `frontend/web/src/types/notification.ts`, `frontend/web/src/lib/relativeTime.ts`, `frontend/web/src/locales/{en,ru,ja}.json`, `frontend/web/.env.example`, `docker/.env.example`
*Action:*
- Create `types/notification.ts` mirroring `services/notifications/internal/domain/notification.go` (`UserNotification` + `NewEpisodePayload` + `NotificationType` union + response types `ListResponse { notifications, unread_count, total }`, `UnreadCountResponse { unread_count }`, `MarkAllReadResponse { updated }`). All fields snake_case to match backend JSON.
- Create `lib/relativeTime.ts` as a tiny pure module using `Intl.RelativeTimeFormat`. Signature `formatRelativeTime(iso: string, locale: 'en'|'ru'|'ja', justNowLabel: string): string`. Thresholds per D-UI-08.
- Add `notifications.*` block to all three locale files. Required keys (per NOTIF-UI-07 + extras for empty/loading/aria states):
  - `notifications.bell.tooltip` ("Notifications" / "Уведомления" / "通知")
  - `notifications.bell.ariaLabelWithCount` (ICU-style with `{count}` interpolation: "Notifications, {count} unread" / "Уведомления, {count} непрочитанных" / "通知、未読 {count} 件")
  - `notifications.bell.ariaLabel` ("Notifications" / "Уведомления" / "通知")
  - `notifications.dropdown.markAllRead`
  - `notifications.dropdown.empty`
  - `notifications.dropdown.loading` ("Loading…")
  - `notifications.newEpisode.singleEp` ("Episode {n} is out" / etc — uses vue-i18n `{n}` syntax)
  - `notifications.newEpisode.rangeEp` ("Episodes {n}–{m} are out" / etc)
  - `notifications.newEpisode.via` ("via {translation}" / etc)
  - `notifications.unknown.title` ("New notification — view in dropdown")
  - `notifications.toast.dismissAria` ("Dismiss")
  - `notifications.time.justNow` ("just now" / "только что" / "たった今")
- Add `VITE_NOTIFICATIONS_ENABLED=true` to both env-example files with a one-line comment: `# Set to false to hide the notification bell + disable polling (Phase 3 rollback flag).`
*Verify:*
- `bun run type-check` is green for the new types file (no Vue, pure TS)
- `bunx vitest run` if any unit test gets added (optional in this task)
- `bun run lint` green for the locales + helper
*Done:* The three TS modules + locale entries + env flags exist; type-check + lint pass.

**Task 1.2 — API client + Pinia store**
*Files:* `frontend/web/src/api/notifications.ts`, `frontend/web/src/stores/notifications.ts`
*Action:*
- Create `api/notifications.ts` exporting:
  - `listNotifications(status: 'unread'|'all' = 'unread', limit = 20, offset = 0): Promise<ListResponse>`
  - `getUnreadCount(): Promise<number>` (returns just `.unread_count`)
  - `markRead(id: string): Promise<void>`
  - `markAllRead(): Promise<number>` (returns `.updated`)
  - `dismiss(id: string): Promise<void>`
  - `click(id: string): Promise<void>`
  - All use `apiClient` from `@/api/client` and unwrap envelope per D-UI-07.
- Create `stores/notifications.ts` using `defineStore('notifications', () => { ... })` composition-API form. State refs: `notifications: Ref<UserNotification[]>`, `unreadCount: Ref<number>`, `shownToastIds: Ref<Set<string>>` (session-only, NOT persisted to localStorage), `polling: Ref<boolean>`, `lastFetchAt: Ref<number>`. Internal: `intervalId: number | null`, `inFlight: boolean`, `visListenerAttached: boolean`.
- Actions:
  - `fetchUnread()` — single-flight; updates `notifications` + `unreadCount`; bails out if disabled by feature flag
  - `markRead(id)` — optimistic: set `read_at = now`; rollback on error
  - `dismiss(id)` — optimistic: splice from `notifications`, bump `unreadCount` down if was unread; rollback on error
  - `markAllRead()` — optimistic: zero badge + read_at for all; rollback on error
  - `handleClick(notification)` — fire-and-forget `click(id)` (don't await), then `router.push(translateWatchUrl(payload.watch_url))`. Use `useRouter()` from inside the action — note: composables aren't usable directly inside `defineStore` actions; instead, accept `router` as a param to `handleClick(notification, router)` OR have the calling component pass `router.push` as a callback. Use the callback approach to keep the store framework-agnostic.
  - `start()` — idempotent; reads `import.meta.env.VITE_NOTIFICATIONS_ENABLED`; if `'false'` no-op; else immediate `fetchUnread()` then `setInterval(fetchUnread, 60_000)`; attaches `visibilitychange` listener if not already attached
  - `stop()` — clears interval, removes listener (if attached AND no other tabs need it — listener can stay attached since it's a no-op when polling is stopped), resets all state to empty
- Getters: `latestUndismissedToast = computed(() => notifications.value.find(n => !shownToastIds.value.has(n.id) && !n.read_at && !n.dismissed_at))`
- Helper export (not a getter): `translateWatchUrl(url: string): { path: string; query: Record<string, string> }` — parses `/anime/{id}/watch?player=X&episode=N&translation=Y` into `{ path: '/anime/'+id, query: { episode, player, translation } }`. The Anime.vue view reads `?episode=N` at line 1251 to deep-link to a player. **Document inline:** "Backend's watch_url uses /anime/{id}/watch/... but the frontend route is /anime/:id; this helper translates."
*Verify:*
- `bun run type-check` green
- Manual smoke (in dev console after build): `useNotificationsStore().fetchUnread()` returns successfully against the live API (logged in as `ui_audit_bot`)
*Done:* Store + API client exist; type-check passes; manual smoke against `/api/notifications?status=unread` returns an array.

### Wave 2 — Renderers + registry (parallelizable, depends on Wave 1 types)

**Task 2.1 — NewEpisodeCard + UnknownNotificationCard + registry**
*Files:* `frontend/web/src/components/notifications/NewEpisodeCard.vue`, `frontend/web/src/components/notifications/UnknownNotificationCard.vue`, `frontend/web/src/lib/notification-renderers.ts`
*Action:*
- `NewEpisodeCard.vue` props: `notification: UserNotification`. Cast `notification.payload` to `NewEpisodePayload`. Layout (Tailwind):
  ```
  <button @click="onClick" class="w-full flex items-start gap-3 p-3 hover:bg-white/5 transition-colors text-left">
    <img class="w-[52px] h-[72px] rounded object-cover flex-shrink-0" :src="payload.anime_poster_url" :alt="payload.anime_title" />
    <div class="flex-1 min-w-0">
      <p class="text-white text-sm font-medium truncate">{{ payload.anime_title }}</p>
      <p class="text-cyan-400 text-xs mt-0.5">
        {{ rangeText }} <!-- "Episode 6 is out" or "Episodes 6–8 are out" -->
      </p>
      <p class="text-white/50 text-xs mt-0.5 truncate">
        {{ sourceText }} <!-- "via AniLibria (RU dub)" -->
      </p>
      <p class="text-white/40 text-[10px] mt-1">{{ relativeTime }}</p>
    </div>
    <button @click.stop="onDismiss" class="text-white/40 hover:text-white text-lg leading-none p-1" :aria-label="$t('notifications.toast.dismissAria')">×</button>
  </button>
  ```
- `rangeText` uses `notifications.newEpisode.singleEp` when `first_unwatched_episode === latest_available_episode` else `rangeEp`. `sourceText` composes `payload.translation_title || payload.translation_id` + parenthesized language/watch_type label (e.g. "RU dub"). `relativeTime` uses `formatRelativeTime(notification.created_at, locale, t('notifications.time.justNow'))`.
- `onClick` calls `store.handleClick(notification, router)`. `onDismiss` calls `store.dismiss(notification.id)` (and emits a `close` event so the parent dropdown/toast can close).
- `UnknownNotificationCard.vue` props: `notification: UserNotification`. Single-liner: `<p>{{ $t('notifications.unknown.title') }}</p>` with the same dismiss × button.
- `lib/notification-renderers.ts`:
  ```ts
  import type { Component } from 'vue'
  import NewEpisodeCard from '@/components/notifications/NewEpisodeCard.vue'
  import UnknownNotificationCard from '@/components/notifications/UnknownNotificationCard.vue'
  export const renderers: Record<string, Component> = {
    new_episode: NewEpisodeCard,
  }
  export function resolveRenderer(type: string): Component {
    return renderers[type] || UnknownNotificationCard
  }
  export function isKnownType(type: string): boolean {
    return type in renderers
  }
  ```
  The `isKnownType` check is used by `NotificationToast.vue` to suppress unknown types entirely (per NOTIF-UI-06).
*Verify:*
- `bun run type-check` green
- Manual visual smoke against a seeded `new_episode` notification (rendered in isolation via a quick view, OR after Task 3.x lands)
*Done:* Two card components + registry file exist; type-check passes; renderers map correctly.

### Wave 3 — Bell + Dropdown (sequential — bell hosts dropdown)

**Task 3.1 — NotificationBell + NotificationDropdown**
*Files:* `frontend/web/src/components/NotificationBell.vue`, `frontend/web/src/components/NotificationDropdown.vue`
*Action:*
- `NotificationBell.vue`:
  - Inline SVG bell icon per D-UI-01 inside a `<button>` matching Navbar.vue search-button styling (`p-2 text-white/70 hover:text-white hover:bg-white/10 rounded-lg transition-colors`).
  - Pink badge per D-UI-02 — absolute-positioned, `v-if="unreadCount > 0"`. Render `unreadCount > 99 ? '99+' : unreadCount`.
  - `aria-label` from `$t('notifications.bell.ariaLabelWithCount', { count: unreadCount })` when > 0 else `$t('notifications.bell.ariaLabel')`. Add `aria-haspopup="true"` `:aria-expanded="open"`.
  - Local ref `open = ref(false)`. Click toggles. `onClickOutside` from `@vueuse/core` on the wrapper closes (mirror Navbar.vue:464). Global `keydown.Escape` listener via `useEventListener(document, 'keydown', ...)` closes when open (mirror Navbar.vue:362).
  - When opened: `await store.fetchUnread()` is fire-and-forget refresh so the dropdown is up-to-the-second.
  - Renders `<NotificationDropdown v-if="open" @close="open = false" />` absolutely-positioned right-anchored below the bell.
  - Uses the same `dropdown` named transition as Navbar.vue.
- `NotificationDropdown.vue`:
  - Container: `bg-gray-950/95 backdrop-blur-xl border border-white/10 shadow-2xl rounded-xl w-[380px] md:w-[380px] max-w-[calc(100vw-1.5rem)]`. Sticky-bottom footer.
  - Body: `max-h-[420px] overflow-y-auto`. If `store.notifications.length === 0`: localized empty-state with `notifications.dropdown.empty` and a muted bell icon. Else: `<component :is="resolveRenderer(n.type)" v-for="n in store.notifications" :key="n.id" :notification="n" @close="$emit('close')" />`.
  - Footer: `<button @click="store.markAllRead(); $emit('close')" class="...">{{ $t('notifications.dropdown.markAllRead') }}</button>` — hidden when `unreadCount === 0`.
  - Emits `close` event (consumed by NotificationBell to flip `open=false`).
*Verify:*
- `bun run type-check` green; `bun run lint` green
- Manually open the dropdown in dev; verify empty-state renders; verify card renders against seeded notification.
*Done:* Bell + dropdown render; outside-click + Esc close; mark-all-read works against live API.

### Wave 4 — Toast + App.vue wiring + Header mount + Router alias

**Task 4.1 — NotificationToast + App.vue + Navbar mount + router watch URL alias**
*Files:* `frontend/web/src/components/NotificationToast.vue`, `frontend/web/src/App.vue`, `frontend/web/src/components/layout/Navbar.vue`, `frontend/web/src/router/index.ts`
*Action:*
- `NotificationToast.vue`:
  - Reads `store.latestUndismissedToast` reactively. When it becomes non-null AND `isKnownType(value.type)` (suppress unknowns) AND not suppressed by route, mount the toast.
  - **Route suppression** (NOTIF-UI-04): `const route = useRoute(); const animeIdFromRoute = computed(() => route.params.id as string | undefined)`. Suppress when `animeIdFromRoute.value === payload.anime_id`.
  - Position: per D-UI-03. Use a media query via `useMediaQuery('(min-width: 768px)')` from `@vueuse/core` (already imported in Navbar.vue).
  - 8s auto-hide: `let timer = window.setTimeout(() => onDismiss(), 8000)`. Pause-on-hover: `@mouseenter` clears the timer, `@mouseleave` restarts it (resetting to a fresh 8000ms, not resuming — simpler and matches user expectation).
  - Click: `store.handleClick(notification, router); store.shownToastIds.add(id)` (and the toast unmounts because `latestUndismissedToast` advances). Dismiss ×: `store.shownToastIds.add(id)` (does NOT call `/api/notifications/{id}/dismiss` — dismiss-from-toast is session-only suppression, not server-side hard dismiss; the user keeps it in the dropdown for now).
  - When `latestUndismissedToast` goes from non-null → null (because user dismissed or marked-read), clear timer + emit close.
- `App.vue` modifications:
  - Import: `import NotificationToast from '@/components/NotificationToast.vue'`. Import: `import { useNotificationsStore } from '@/stores/notifications'`.
  - In `<script setup>`: `const notifEnabled = import.meta.env.VITE_NOTIFICATIONS_ENABLED !== 'false'` (default true). `const notifStore = useNotificationsStore()`. `watch(() => authStore.isAuthenticated, (v) => { if (!notifEnabled) return; if (v) notifStore.start(); else notifStore.stop() }, { immediate: true })`.
  - Template: add `<NotificationToast v-if="notifEnabled" />` right BEFORE the closing `</main>` end (line ~50) — i.e. above the `Toaster.vue` mount but below `<router-view>`. Mounting outside `<main>` ensures it survives route transitions.
- `Navbar.vue` modifications:
  - Import: `import NotificationBell from '@/components/NotificationBell.vue'`.
  - Define `const notifEnabled = import.meta.env.VITE_NOTIFICATIONS_ENABLED !== 'false'`.
  - In template, **desktop right-section** (between language selector close `</div>` at line 149 and User Avatar template at line 152): add `<NotificationBell v-if="notifEnabled && authStore.isAuthenticated" class="hidden md:flex" />`.
  - In the **mobile drawer** (between divider line 218 and Profile link line 222): wrap the bell again for mobile — `<NotificationBell v-if="notifEnabled && authStore.isAuthenticated" class="md:hidden" />` so mobile users get the same surface. (The bell's dropdown is full-width on mobile per D-UI-04.)
- `router/index.ts` modifications:
  - Add route alias so backend-shipped `watch_url=/anime/{id}/watch?...` resolves:
    ```ts
    {
      path: '/anime/:id/watch',
      redirect: (to) => ({ path: `/anime/${to.params.id}`, query: to.query }),
    }
    ```
  - Place this BEFORE the existing `/anime/:id` route to take precedence. The redirect preserves all query params (`player`, `episode`, `translation`) which Anime.vue already consumes at line 1251.
  - (Belt + suspenders: the `translateWatchUrl` helper in the store ALREADY produces the `/anime/:id?...` shape directly without going through `/watch`. The alias is defense for any code path that might pass the raw URL into `router.push` instead — e.g. future deep links from email/Telegram delivery channels.)
*Verify:*
- `bun run build` is green (production build catches type + template errors)
- `bun run lint` green
- `make redeploy-web` + manual smoke: log in as `ui_audit_bot`, run seed script, see badge + toast within 60s, click → land on `/anime/{id}?episode=N&player=...&translation=...`.
*Done:* All 4 success paths work end-to-end against live backend.

### Wave 5 — E2E spec + manual gauntlet + SUMMARY

**Task 5.1 — Playwright spec + verification matrix + SUMMARY**
*Files:* `frontend/web/e2e/notifications.spec.ts`, `.planning/workstreams/notifications/phases/03-frontend-bell-dropdown-toast/03-SUMMARY.md`
*Action:*
- Implement TC-01..08 per D-UI-09. Use `test.describe` blocks. For authenticated tests, share a `test.beforeAll` that calls `/api/auth/login` via `request.post` (Playwright's request fixture; gets the cookies), then stash the access token, then in each test's `beforeEach` set `localStorage.token` + `localStorage.user` to the cached values. Mirror the auth-helper pattern in `e2e/watchlist.spec.ts` (already uses mock-token; this spec uses real-token).
- For TC-03 / TC-04 / TC-08 the spec must seed a notification. Implement a helper `seedNotification(payload)` that POSTs to `/internal/notifications` via `docker compose exec` (test-only, gated behind `process.env.E2E_INTERNAL_SEED === 'true'`). For CI environments without docker access, the spec skips with `test.skip(!process.env.E2E_INTERNAL_SEED)`.
- TC-07 (tab-hide) needs the visibility-change simulation; document the technique inline (`Object.defineProperty(document, 'hidden', ...)` + `dispatchEvent('visibilitychange')`). Use `page.waitForRequest` with a 65-second timeout to assert exactly one regain-fetch.
- Write `03-SUMMARY.md` at the end of execution following the format used by `01-SUMMARY.md` and `02-SUMMARY.md`: front-matter (status, completed, commits, requirements_resolved), verification matrix table (SC1..8 with verbatim command + result), deviations from plan, risks materialized, touched files summary, score block, next-steps.
- Embed the NOTIF-NF-03 manual E2E doc INSIDE the SUMMARY (per REQUIREMENTS): the exact 6-step sequence (login → seed → reload → see bell → click → land → dismiss → 0).
*Verify:*
- `bunx playwright test e2e/notifications.spec.ts --reporter=list` runs (8 specs, all pass or all skipped if `E2E_INTERNAL_SEED` unset — but at least TC-01 and TC-02 always run)
- `make health` reports notifications + web both healthy after final redeploy
- Manual NOTIF-NF-03 gauntlet completes end-to-end
*Done:* Spec file exists, SUMMARY file exists, manual gauntlet recorded in SUMMARY, all 8 ROADMAP success criteria PASS.

## Verification matrix (matches ROADMAP Phase 3 success criteria 1–8)

| SC  | Requirement | Verification |
| --- | --- | --- |
| **SC1** | Logged-out: no bell, no `/api/notifications` requests | TC-01 in Playwright spec; manual: open browser devtools Network tab on `https://animeenigma.ru/` in incognito, watch for 30s, confirm zero `/api/notifications/*` calls + confirm `<button aria-label*="Notifications">` is absent in DOM |
| **SC2** | Logged-in zero notifications: bell renders, no badge, one fetch on mount + 60s ticks, tab-hide pauses within ~1s | TC-02 + TC-07 in Playwright spec; manual: log in, watch Network tab — `/api/notifications?status=unread` fires immediately and again ~60s later; minimize tab and watch — next tick is suppressed |
| **SC3** | Logged-in `ui_audit_bot` with one seed: badge "1" within 60s, toast slides in once, auto-hides at 8s, click navigates, no toast re-show in session | TC-03 in Playwright spec; manual: `scripts/seed-notification-for-ui-audit-user.sh && open https://animeenigma.ru/`, wait, observe |
| **SC4** | Click bell → dropdown opens, click card → `POST /:id/click` + `router.push(watch_url)` + dropdown closes; mark-all-read fires `POST /mark-all-read` and zeroes badge | TC-05 + TC-03 click-path in spec; manual: log in with seed, click bell, click card, verify URL became `/anime/{id}?episode=N&player=X&translation=Y` |
| **SC5** | Toast does NOT appear when route param `id === payload.anime_id` | TC-04 in Playwright spec; manual: navigate to `/anime/{seed_anime_id}` first, then seed + reload, verify NO toast (but badge still appears) |
| **SC6** | Unknown type renders via UnknownNotificationCard in dropdown; toast suppresses unknown types | TC-08 in Playwright spec; manual: `docker compose exec notifications wget -qO- --post-data='{"user_id":"<id>","type":"future_type","dedupe_key":"manual-test","payload":{}}' localhost:8090/internal/notifications`, reload, open dropdown |
| **SC7** | All 3 locales render dropdown + card without falling back to translation keys | Manual: switch language toggle through ru → ja → en, screenshot the dropdown each time, verify no raw `notifications.bell.tooltip` strings visible |
| **SC8** | Logout → `stopPolling()` + clears state; no further `/api/notifications` calls | Manual: log in, observe Network tab, click Logout, verify zero further `/api/notifications/*` requests in 90s; bell disappears from header |

All 8 SCs must PASS for the phase to be considered complete. Failures land as Phase-3 gaps in a `--gaps` follow-up.

## Risks + mitigations (Phase-3 specific)

| ID | Risk | Mitigation |
| -- | ---- | ---------- |
| **R-03-01** | `visibilitychange` race on slow tabs — fetch fires on regain WHILE the previous interval tick is still in-flight, creating duplicate requests + double-render. | Single-flight guard `inFlight: boolean` in `fetchUnread()`. Concurrent calls return the existing promise. Spec TC-07 explicitly verifies only one regain-fetch happens. |
| **R-03-02** | Toast suppression edge case: user navigates from `/browse` to `/anime/{id}` while a toast for that anime is mid-flight. Animation could double-fire or toast could become stuck. | The toast component reactively reads `route.params.id`; when that changes to match `payload.anime_id`, the toast unmounts (transition handles the slide-out). When it changes away, a NEW toast does NOT spawn for the same notification because `shownToastIds.add(id)` was set when the user first interacted (or auto-hide fired). Test path is covered by TC-04 (load route first) but the mid-flight transition is not — defer to manual visual smoke; if user feedback shows jank, add a 200ms debounce on the route watcher. |
| **R-03-03** | Polling cost on idle users — 1440 requests/day per logged-in idle tab. | `visibilitychange` pause is the primary mitigation. Backend cost is one cached `SELECT COUNT(*) WHERE dismissed_at IS NULL AND read_at IS NULL` per call — `idx_user_unread` is a partial index sized to live unread rows only (likely <100 per user). Acceptable at v1.0 scale. If v1.1 usage data shows >10K users, switch to SSE (per ROADMAP v1.2). |
| **R-03-04** | Accessibility regressions in bell focus state — the badge `<span>` could intercept the focus ring or break SR announcement order. | Badge is `aria-hidden="true"` (SR reads the count via the button's `aria-label` instead). Button has explicit `focus-visible:ring-2 focus-visible:ring-cyan-400 focus-visible:outline-none` (mirrors search button at Navbar.vue:43-52). Manual SR test (VoiceOver / NVDA) at end of Wave 3. |
| **R-03-05** | Mobile viewport overlap with existing top-bar — the bell renders inside Navbar.vue which is `fixed top-0` 64px-tall; on mobile (<768px) the bell currently shows only via the mobile drawer (hamburger), so toast at `top-16` covers the navbar's lower edge but NOT the bell because the bell is inside the (closed) drawer. | Audit during Wave 4 manual smoke: log in on mobile viewport, check the bell is reachable WITHOUT opening the hamburger (acceptable per NOTIF-UI-02 wording "header" — drawer-only is acceptable for v1.0). If users complain, lift the bell into the always-visible mobile bar via a Phase-3.x follow-up. |
| **R-03-06** | Backend `watch_url` shape (`/anime/{id}/watch?...`) doesn't match frontend route (`/anime/:id?...`). | Two-fold mitigation: (1) `translateWatchUrl` helper in store does the parse + reshape; (2) router alias `/anime/:id/watch` redirects to `/anime/:id` preserving query params (defense for any future code path that calls `router.push(payload.watch_url)` directly). |
| **R-03-07** | `latestUndismissedToast` could keep returning the same notification if `shownToastIds.add(id)` is missed (e.g. user navigates away mid-show). | The toast always calls `shownToastIds.add(id)` on every exit path (click, dismiss, auto-hide, route-suppress unmount). Belt + suspenders: when `notifications` array updates from `fetchUnread()`, prune `shownToastIds` to only IDs still present in the array (prevents unbounded Set growth across long sessions). |
| **R-03-08** | E2E spec for tab-hide is flaky — the visibility simulation via `Object.defineProperty` is non-standard and Playwright Firefox vs Chromium handle it differently. | Run TC-07 only on Chromium project in `playwright.config.ts`; document why in the spec comment. Firefox skips. Acceptable for v1.0 (the production behavior is identical across browsers — only the test mock differs). |

## Rollback

**Flag:** `VITE_NOTIFICATIONS_ENABLED=false` (defaults `true`).

When set to `false` at build time:

- `App.vue` skips mounting `<NotificationToast />` and skips calling `notifStore.start()` in the auth watcher.
- `Navbar.vue` skips rendering `<NotificationBell />` in both desktop and mobile drawer slots.
- The store is still importable (zero crash risk) but `start()` no-ops and `notifications`, `unreadCount` stay at their empty defaults.
- Result: zero `/api/notifications/*` requests, zero bell in DOM, zero toast capability. Equivalent to "Phase 3 was never shipped" from the user's perspective.

Rollback procedure:
1. Edit `docker/.env` (or the deployment env): set `VITE_NOTIFICATIONS_ENABLED=false`.
2. `make redeploy-web` (rebuilds the Vite bundle with the new flag baked in).
3. Verify: `curl -s https://animeenigma.ru/ | grep -c "NotificationBell"` returns `0`.

Backend (Phases 1+2) is unaffected — the notifications service keeps producing rows; only the user-facing surface is hidden. This is intentional: flipping the flag back on later restores the visible UX without losing any notification history.

## Score (per project convention)

- **UXΔ:** **+4 (Better)** — directly fixes constant low-grade friction (manual "did ep N drop yet?" refresh checks). Not +5 because it doesn't unlock anything new — it removes a chore. Matches the workstream-level score.
- **CDI:** `0.03 × 8` — Spread: frontend-only (1 store, 1 API client, 5 components, 1 helper, 1 registry, 3 locales, App+Navbar wiring, 1 router alias). Shift: low (additive — no breaking changes to existing components, no schema changes; bell mount is one line in Navbar.vue; toast mount is one line in App.vue). Effort: 8 Fibonacci (10 new files + 8 modifications + Playwright spec with 8 test cases + 3-locale i18n + visibility-change/single-flight subtleties + URL-translation defense — solidly 8, not 5).
- **MVQ:** **Griffin 88%/85%** — Griffin (graceful + reliable + visible) is the right shape: noble surface (bell + toast in the header crown), strong wings (Pinia store + polling lifecycle + envelope unwrap + URL translation defense). 88% match — the type-pluggable registry is unusually Griffin-like (a single surface that other types soar onto without breaking). 85% slop-resistance — `shownToastIds` session-tracking + `inFlight` single-flight + route-suppression + envelope-unwrap centralization + feature-flag rollback all combine to make accidental defects unlikely. Could push to 92% if we add `vitest` unit tests for `translateWatchUrl` + `formatRelativeTime` (deferred — covered by E2E).

## Definition of done

Phase 3 is complete when ALL of the following are true:

- [ ] All 10 new files exist and compile
- [ ] All 8 modified files compile + lint clean (`bun run build` + `bun run lint` green)
- [ ] `bun run type-check` (vue-tsc) green for the whole frontend
- [ ] `make redeploy-web` succeeds and `make health` reports `✓ web` + `✓ notifications`
- [ ] Playwright spec `e2e/notifications.spec.ts` runs end-to-end (TC-01 + TC-02 always; TC-03..08 when `E2E_INTERNAL_SEED=true`)
- [ ] All 8 ROADMAP Phase 3 success criteria PASS in the verification matrix (recorded verbatim in SUMMARY)
- [ ] Manual NOTIF-NF-03 gauntlet completes — login, seed, see, click, dismiss
- [ ] `03-SUMMARY.md` written following the Phase 1/2 SUMMARY format (frontmatter + verification matrix + deviations + score + next-steps)
- [ ] Feature flag rollback verified — flip to `false`, redeploy, confirm zero bell/toast/network
- [ ] CHANGELOG entry added via `/animeenigma-after-update` (informative + enthusiastic tone)
- [ ] Commit + push (worktree-based per project convention) with co-authors per MEMORY.md
- [ ] Workstream STATE.md updated to mark Phase 3 complete; ROADMAP.md milestone v1.0 marked ready for `/gsd-complete-milestone`
