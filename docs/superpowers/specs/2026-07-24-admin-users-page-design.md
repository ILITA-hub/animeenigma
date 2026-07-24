# Admin Users Management Page — Design

**Date:** 2026-07-24
**Status:** Approved (design), pending implementation
**Scope (this iteration):** List all users · unified search · change role. Persist + search Telegram name.

## Problem

`/admin/` has dashboards, recs, collections, policy, feedback, gacha, and raw-library — but **no way to see or manage users**. Today, finding a user means a single-record `resolve` lookup, and changing a role means raw `UPDATE users SET role=...` SQL. Admins need a searchable user directory and in-UI role management.

## Goals

- Paginated list of all users.
- One search box matching **user id (UUID + `public_id`), username, Telegram id, Telegram name**.
- Change a user's role (`user` / `librarian` / `admin`) from the UI.
- Persist Telegram username + first name (currently only the numeric `telegram_id` is stored) so tg-name search works.

## Non-goals (deferred)

- Ban / suspend / deactivate (no status column exists; only GORM soft-delete). Out of scope this iteration.
- Hard delete of users.
- Session invalidation on role change (roles are baked into JWTs; see Known Limitations).
- Bulk actions, CSV export, per-user detail drill-down page.

## Current-state facts (verified against code)

- **Users live in the `auth` service (8080)** — `services/auth/internal/domain/user.go`, `repo/user.go`. Catalog (8081) has no user model.
- `User` fields: `ID` (uuid), `Username`, `PasswordHash`, `TelegramID *int64` (unique), `PublicID` (`user1234567`), `PublicStatuses`, `ActivityVisibility`, `Avatar`, `Timezone`, `ApiKeyHash`, `CertAutoLogin`, `Role` (`user|admin|librarian`, default `user`), `CreatedAt`, `UpdatedAt`, `DeletedAt` (soft-delete).
- **No ban/status column.** **No `ListUsers` repo method.** **No `UpdateRole` path** — role only ever set at signup.
- Telegram: single nullable `telegram_id` column; tg username/first/last pass through **transiently** during login (`TelegramWebhookUser`, Redis `TelegramAuthSession`) and are **not persisted**.
- Roles read from the **JWT claim** at request time, not re-queried per request.
- Gateway carves out only the static `/api/admin/users/resolve` → `ProxyToAuth`; the rest of `/api/admin/*` → catalog.
- Auth middleware: `AuthMiddleware` + `AdminMiddleware` already wrap `r.Route("/admin/users")`.

## Design

### Backend — `auth` service (8080)

**1. Model (`domain/user.go`)** — add two nullable fields:
```go
TelegramUsername  *string `gorm:"size:64"  json:"telegram_username,omitempty"`
TelegramFirstName *string `gorm:"size:128" json:"telegram_first_name,omitempty"`
```
GORM `AutoMigrate` adds the columns on service restart (no manual migration; additive only).

**2. Persist tg name on login (`service/auth.go` → `LoginWithTelegram`)** — when the tg auth session carries username / first name, write them onto the user row for **both** newly-provisioned and returning users, so the values refresh on every Telegram login. Existing users backfill on their next login (blank until then).

**3. Repo (`repo/user.go`)** — add:
- `ListUsers(ctx, params ListUsersParams) (users []User, total int64, err error)`:
  - `params`: `Query string`, `Role string` (optional exact filter), `Limit int`, `Offset int`.
  - Search WHERE (when `Query != ""`): `username ILIKE %q% OR public_id ILIKE %q% OR telegram_username ILIKE %q% OR telegram_first_name ILIKE %q% OR id::text = q OR CAST(telegram_id AS TEXT) = q`. (ILIKE for text; exact for the id/tg-id numeric+uuid forms.)
  - `Order("created_at DESC")`, `Limit`, `Offset`. Separate `Count(*)` over the same WHERE for `total`.
- `UpdateRole(ctx, id string, role authz.Role) error` — `Model(&User{}).Where("id = ?", id).Update("role", role)`; returns `NotFound` if 0 rows.

**4. Handlers (new `handler/admin_users.go`, `AdminUsersHandler`)**:
- `GET /admin/users?q=&role=&page=&page_size=` →
  ```json
  { "success": true, "data": { "items": [AdminUserView], "total": 123, "page": 1, "page_size": 25 } }
  ```
  `page` defaults 1, `page_size` defaults 25 (clamp 1..100). `AdminUserView` is an **admin-only** projection: `id, username, public_id, role, telegram_id, telegram_username, telegram_first_name, avatar, created_at` (NOT the sanitized public projection — admins need role + tg).
- `PATCH /admin/users/{id}/role` body `{ "role": "user|librarian|admin" }`:
  - Validate role ∈ {user, librarian, admin} → 400 on invalid.
  - **Self-lockout guard:** if `{id}` == caller's own user id (from JWT) → 403 `cannot change your own role`.
  - 404 if user not found. On success return the updated `AdminUserView`.

**5. Router (`transport/router.go`)** — inside the existing `r.Route("/admin/users")` group (Auth+Admin gated):
```go
r.Get("/", adminUsersHandler.List)
r.Patch("/{id}/role", adminUsersHandler.UpdateRole)
r.Get("/resolve", userResolveHandler.Resolve) // unchanged
```

### Gateway (8000)

**6. `transport/router.go`** — broaden the `/api/admin/users/resolve` carve-out to the whole subtree so list + role routes reach auth:
```go
r.Route("/api/admin/users", func(r chi.Router) {
    r.Use(JWTValidationMiddleware, AdminRoleMiddleware) // same gate as today
    r.Handle("/*", ProxyToAuth)
})
```
Keep the existing admin gate. Verify chi longest-prefix still routes `/api/admin/users/resolve` here (it will, since it's the same subtree) and the generic `/api/admin/*` → catalog is unaffected for other paths.

### Frontend (`frontend/web`)

**7. API (`src/api/client.ts`)** — in `adminApi`:
- `listUsers(params: { q?: string; role?: string; page?: number; page_size?: number })` → `GET /admin/users`.
- `updateUserRole(id: string, role: string)` → `PATCH /admin/users/{id}/role`.
- Add `AdminUser` type (mirrors `AdminUserView`).

**8. View `src/views/admin/AdminUsers.vue`** — shell modeled on `AdminCollections.vue`; filter/pagination pattern from `AdminFeedback.vue` (+ optional `useAdminUsers.ts` composable owning `q/role/page/pageSize/items/total/refresh`):
- Header `<h1>{{ $t('admin.users.title') }}</h1>`.
- Controls row: debounced search `Input` (placeholder lists what it matches), role-filter `Select` (all / user / librarian / admin).
- `.glass-card overflow-x-auto` table, columns:
  1. **User** — `Avatar` + `username`.
  2. **Public ID** — `public_id` (mono).
  3. **Role** — `Badge` + inline role dropdown to change (see 9).
  4. **Telegram** — `telegram_id` + `telegram_username`/`telegram_first_name` when present; em-dash when not linked.
  5. **Joined** — `created_at` (locale date).
- States: `Spinner` (loading) · `EmptyState` (no results) · 403 card (non-admin) · generic error card. Responses unwrapped `res.data?.data ?? res.data`.
- `PaginationBar :currentPage :totalPages @update:currentPage` at the bottom (`totalPages = ceil(total / pageSize)`).

**9. Role-change UX** — per-row role dropdown → `useConfirm()` ("Change {username} to {role}?") → `adminApi.updateUserRole` → on success refresh the row/list; on error show `Alert`. The control is **disabled on the current admin's own row** (mirrors the backend self-lockout guard) with a tooltip.

**10. Router (`src/router/index.ts`)** — add to the `/admin/*` block:
```ts
{ path: '/admin/users', name: 'admin-users',
  component: () => import('@/views/admin/AdminUsers.vue'),
  meta: { titleKey: 'admin.users.title', requiresAuth: true, requiresAdmin: true } }
```

**11. Dashboard (`src/views/admin/AdminDashboard.vue`)** — add a "Users" tool card (icon + `admin.users.title`/`admin.users.subtitle`, route `admin-users`) to the `tools` array.

**12. i18n** — `admin.users.*` keys in `en`, `ru`, `ja` (title, subtitle, search placeholder, role filter labels, column headers, role names, confirm copy, empty/error states, self-role-disabled tooltip).

## Known limitations (accepted)

- **Role change latency:** a target user's new role only takes effect after their JWT refreshes/expires (roles are baked into the token). Acceptable for a small self-hosted group; session-invalidation-on-change is a possible later add.
- **tg-name backfill:** existing users show blank Telegram name until their next Telegram login.

## Testing

**Go (`services/auth`):**
- Repo: `ListUsers` — search matches each field (username/public_id/uuid/tg-id/tg-name), role filter, pagination (limit/offset), `total` count correctness; `UpdateRole` — success, `NotFound` on missing id.
- Handler: list happy-path + pagination envelope; role update success; **self-lockout → 403**; invalid role → 400; missing user → 404. testcontainers for DB per existing patterns; mock external.

**Frontend:** `/frontend-verify` — DS-lint, i18n en/ru/ja parity, real `bun run build`. (Chrome smoke opt-in only.)

## Metrics (per `.planning/CONVENTIONS.md`)

- **UXΔ = +2 (Better)** — admins get a real searchable user directory + in-UI role management, replacing raw SQL.
- **CDI = 0.03 * 21** — auth model/repo/handler/router + gateway + FE view/router/api/i18n; low spread×shift, moderate effort.
- **MVQ = Griffin 85%/80%.**

## File touch-list

**Backend (auth):** `internal/domain/user.go` · `internal/service/auth.go` · `internal/repo/user.go` · `internal/handler/admin_users.go` (new) · `internal/transport/router.go` (+ handler tests).
**Gateway:** `internal/transport/router.go`.
**Frontend:** `src/api/client.ts` · `src/views/admin/AdminUsers.vue` (new) · `src/composables/useAdminUsers.ts` (new, optional) · `src/router/index.ts` · `src/views/admin/AdminDashboard.vue` · `src/i18n/locales/{en,ru,ja}.json` (or equivalent locale files).
