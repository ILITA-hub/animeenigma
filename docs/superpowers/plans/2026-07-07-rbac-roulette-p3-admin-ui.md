# RBAC and Roulette — Phase 3: Policy Admin UI + Standard User Resolver — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship the `/admin/policy` Features tab (runtime steering wheel for the P1/P2 policy engine) plus one standard user-ID resolver reused by policy + recs.

**Architecture:** Part A first (foundational resolver: auth endpoint → gateway route → shared FE component → recs adoption), then Part B (policy admin view consuming the resolver). Go microservices + Vue 3. All changes additive; no playback path touched.

**Tech Stack:** Go (chi, GORM, sqlite `:memory:` tests), Vue 3 (`<script setup lang="ts">`, Vitest, lucide-vue-next named imports), Neon-Tokyo DS.

## Global Constraints

- **Spec of record:** `docs/superpowers/specs/2026-07-07-rbac-roulette-p3-admin-ui-design.md`. Every task's requirements implicitly include it.
- **No `window.open`** anywhere. Feature open-links are plain `<a target="_blank" rel="noopener noreferrer">` with **same-origin relative** routes; standalone/PWA intercepts with `preventDefault` + `router.push`.
- **allow/deny lists store userID (UUID) strings.** The gateway (P2) matches them against JWT `sub`. Never store usernames.
- **Resolver identifier set:** UUID · username · public_id · telegram_id (numeric). Same set everywhere.
- **DS:** reuse `@/components/ui` primitives (`Tabs`, `Switch`, `Input`, `Button`, `Badge`, `Spinner`, `Card`/`glass-card`); lucide **named** imports; semantic tokens only; no hardcoded hex; `font-medium`/`font-semibold` only. Must pass `frontend/web/scripts/design-system-lint.sh`.
- **i18n:** every new UI string added to `en.json`, `ru.json`, **and `ja.json`** (parity gate fails the build otherwise).
- **Effort metrics** in any doc/changelog use UXΔ / CDI / MVQ — never days/hours.
- **Commit co-authors** on every commit: `Claude Code <noreply@anthropic.com>`, `0neymik0 <0neymik0@gmail.com>`, `NANDIorg <super.egor.mamonov@yandex.ru>`.
- Worktree only (`.claude/worktrees/rbac-and-roulette`); do not edit the base tree.

## File Structure

- `services/auth/internal/handler/user_resolve.go` — new resolve handler (canonical resolver).
- `services/auth/internal/repo/user.go` — (reuse existing `GetByID/GetByUsername/GetByPublicID/GetByTelegramID`; no new method needed).
- `services/auth/internal/transport/router.go` — mount `/api/admin/users/resolve`.
- `services/gateway/internal/transport/router.go` — proxy `/admin/users/resolve` → auth (admin-gated), before catalog catch-all.
- `services/recs/internal/handler/admin_recs.go` — add `telegram_id` to `resolveUserID` WHERE.
- `frontend/web/src/components/admin/UserResolveInput.vue` — shared resolver input.
- `frontend/web/src/views/admin/AdminRecsPicker.vue` — adopt `UserResolveInput`.
- `frontend/web/src/config/policyFeatures.ts` — feature registry (key→route).
- `frontend/web/src/composables/useAdminPolicy.ts` — admin policy API client.
- `frontend/web/src/composables/useOpenFeature.ts` — contextual open helper.
- `frontend/web/src/views/admin/AdminPolicy.vue` — the Features tab view.
- `frontend/web/src/router/index.ts` — `/admin/policy` route + `/admin/secret-features` redirect.
- `frontend/web/src/locales/{en,ru,ja}.json` — `admin.policy.*` namespace.

---

## Task 1: Auth — standard user-resolve endpoint

**Files:**
- Create: `services/auth/internal/handler/user_resolve.go`
- Test: `services/auth/internal/handler/user_resolve_test.go`
- Modify: `services/auth/internal/transport/router.go` (mount route)

**Interfaces:**
- Consumes: `UserRepository.{GetByID,GetByUsername,GetByPublicID,GetByTelegramID}` (existing), `httputil.{OK,BadRequest,Error}`, `authz` middleware.
- Produces: `GET /api/admin/users/resolve?q=<identifier>` → `200 {id,username,public_id,telegram_id?}` · `404` none · `400` empty. Handler type `UserResolveHandler` with `Resolve(w,r)`.

- [ ] **Step 1: Write the failing test**

`services/auth/internal/handler/user_resolve_test.go` — table test with a fake repo (handwritten, no testify/mock) implementing the four getters over an in-memory slice of users `{id:"11111111-1111-1111-1111-111111111111", username:"oronemu", public_id:"orovanity", telegram_id:100200300}`:

```go
func TestResolve(t *testing.T) {
	cases := []struct{ name, q, wantID string; wantStatus int }{
		{"by uuid", "11111111-1111-1111-1111-111111111111", "11111111-1111-1111-1111-111111111111", 200},
		{"by username", "oronemu", "11111111-1111-1111-1111-111111111111", 200},
		{"by public_id", "orovanity", "11111111-1111-1111-1111-111111111111", 200},
		{"by telegram_id", "100200300", "11111111-1111-1111-1111-111111111111", 200},
		{"not found", "ghost", "", 404},
		{"empty q", "", "", 400},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			h := NewUserResolveHandler(newFakeUserRepo(), testLogger())
			req := httptest.NewRequest(http.MethodGet, "/api/admin/users/resolve?q="+url.QueryEscape(c.q), nil)
			rec := httptest.NewRecorder()
			h.Resolve(rec, req)
			if rec.Code != c.wantStatus { t.Fatalf("status=%d want %d", rec.Code, c.wantStatus) }
			if c.wantStatus == 200 {
				var env struct{ Data struct{ ID string `json:"id"` } `json:"data"` }
				_ = json.Unmarshal(rec.Body.Bytes(), &env)
				if env.Data.ID != c.wantID { t.Fatalf("id=%q want %q", env.Data.ID, c.wantID) }
			}
		})
	}
}
```

Add the fake repo + `testLogger()` helper in the test file. Match the `httputil.OK` envelope shape (`{success,data}`) used elsewhere in auth (grep an existing handler test for the exact assertion pattern and mirror it).

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/auth && go test ./internal/handler/ -run TestResolve -v`
Expected: FAIL (undefined `NewUserResolveHandler`).

- [ ] **Step 3: Implement the handler**

`services/auth/internal/handler/user_resolve.go`:

```go
package handler

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
)

var uuidRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)
var digitsRe = regexp.MustCompile(`^[0-9]+$`)

// userResolveRepo is the minimal surface the resolver needs (satisfied by *repo.UserRepository).
type userResolveRepo interface {
	GetByID(ctx context.Context, id string) (*domain.User, error)
	GetByUsername(ctx context.Context, username string) (*domain.User, error)
	GetByPublicID(ctx context.Context, publicID string) (*domain.User, error)
	GetByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error)
}

type UserResolveHandler struct {
	repo userResolveRepo
	log  *logger.Logger
}

func NewUserResolveHandler(repo userResolveRepo, log *logger.Logger) *UserResolveHandler {
	return &UserResolveHandler{repo: repo, log: log}
}

type resolvedUser struct {
	ID         string `json:"id"`
	Username   string `json:"username"`
	PublicID   string `json:"public_id"`
	TelegramID *int64 `json:"telegram_id,omitempty"`
}

// Resolve turns any of {UUID, username, public_id, telegram_id} into the canonical user.
func (h *UserResolveHandler) Resolve(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		httputil.BadRequest(w, "q is required")
		return
	}
	u := h.lookup(r.Context(), q)
	if u == nil {
		httputil.Error(w, errors.NotFound("user not found"))
		return
	}
	httputil.OK(w, resolvedUser{ID: u.ID, Username: u.Username, PublicID: u.PublicID, TelegramID: u.TelegramID})
}

func (h *UserResolveHandler) lookup(ctx context.Context, q string) *domain.User {
	if uuidRe.MatchString(q) {
		if u, err := h.repo.GetByID(ctx, q); err == nil && u != nil { return u }
	}
	if digitsRe.MatchString(q) {
		if n, err := strconv.ParseInt(q, 10, 64); err == nil {
			if u, err := h.repo.GetByTelegramID(ctx, n); err == nil && u != nil { return u }
		}
	}
	if u, err := h.repo.GetByUsername(ctx, q); err == nil && u != nil { return u }
	if u, err := h.repo.GetByPublicID(ctx, q); err == nil && u != nil { return u }
	return nil
}
```

Add the missing imports (`context`, `github.com/ILITA-hub/animeenigma/libs/errors`). Verify `httputil.Error` renders `errors.NotFound` as HTTP 404 (grep an existing auth handler that returns `errors.NotFound` and confirm the status). If auth uses a different not-found helper, use that instead.

- [ ] **Step 4: Mount the route**

In `services/auth/internal/transport/router.go`, inside the `/api` group, add an admin-gated group (mirror the existing `AdminMiddleware` usage):

```go
r.Route("/admin/users", func(r chi.Router) {
	r.Use(AuthMiddleware(jwtConfig)) // same auth middleware used by the /users protected group
	r.Use(AdminMiddleware)
	r.Get("/resolve", userResolveHandler.Resolve)
})
```

Wire `userResolveHandler` through `NewRouter`'s signature and construct it in `services/auth/cmd/auth-api/main.go` (pass the existing `UserRepository` + logger). Confirm the exact auth-middleware symbol name by reading the file (the protected `/users` group shows the pattern).

- [ ] **Step 5: Run tests**

Run: `cd services/auth && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add services/auth
git commit -m "feat(auth): standard user-resolve endpoint (uuid/username/public_id/telegram_id)"
```

---

## Task 2: Gateway — proxy `/api/admin/users/resolve` → auth

**Files:**
- Modify: `services/gateway/internal/transport/router.go`
- Test: `services/gateway/internal/transport/router_*_test.go` (add/extend a routing test if the suite asserts admin routes; otherwise a focused new test file)

**Interfaces:**
- Consumes: `proxyHandler.ProxyToAuth`, `JWTValidationMiddleware`, `AdminRoleMiddleware`, `getServiceURL("auth")` (all existing).
- Produces: gateway route `GET /api/admin/users/resolve` reaching auth, admin-gated, ordered before the catalog `/api/admin/*` catch-all.

- [ ] **Step 1: Write the failing test**

Read the existing gateway router tests (`services/gateway/internal/transport/`) to find how admin proxy routes are asserted (e.g. a stub upstream + expected target). Add a test that a request to `/api/admin/users/resolve?q=x` with an admin JWT is proxied to the **auth** upstream (not catalog). If no such harness exists, assert route registration by matching the chi route tree for `/api/admin/users/resolve`.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/gateway && go test ./internal/transport/ -run Resolve -v`
Expected: FAIL (route → catalog or 404).

- [ ] **Step 3: Register the route**

In `router.go`, in the admin-gated API group (same group style as the `/admin/policy/*` → `ProxyToPolicy` block; JWT + `AdminRoleMiddleware`), add **before** the catalog `/api/admin/*` catch-all:

```go
r.HandleFunc("/admin/users/resolve", proxyHandler.ProxyToAuth)
```

Match the surrounding group's middleware exactly. Add a comment noting it must precede the catalog admin catch-all.

- [ ] **Step 4: Run tests**

Run: `cd services/gateway && go test ./... -count=1`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/gateway
git commit -m "feat(gateway): route /api/admin/users/resolve to auth (admin-gated)"
```

---

## Task 3: recs — add telegram_id to `resolveUserID`

**Files:**
- Modify: `services/recs/internal/handler/admin_recs.go`
- Test: `services/recs/internal/handler/admin_recs_test.go` (extend)

**Interfaces:**
- Consumes: existing `resolveUserID` + users-table fixture.
- Produces: `resolveUserID` also matches `telegram_id` (numeric) — a `/admin/recs/{tgid}` deep link resolves.

- [ ] **Step 1: Write the failing test**

Extend the existing resolve test with a case: a users-table fixture row having `telegram_id = 100200300`, then `resolveUserID(ctx, "100200300")` returns that user's UUID. (Mirror the existing username/public_id test setup in the file.)

- [ ] **Step 2: Run it, verify it fails**

Run: `cd services/recs && go test ./internal/handler/ -run Resolve -v`
Expected: FAIL (telegram not matched).

- [ ] **Step 3: Implement**

In `resolveUserID`, change the lookup to also match telegram when the raw is all-digits:

```go
// existing: Where("username = ? OR public_id = ?", raw, raw)
q := h.db.WithContext(ctx).Table("users").Select("id")
if digitsRe.MatchString(raw) {
	q = q.Where("username = ? OR public_id = ? OR telegram_id = ?", raw, raw, raw)
} else {
	q = q.Where("username = ? OR public_id = ?", raw, raw)
}
err := q.Limit(1).Scan(&id).Error
```

Add a `digitsRe = regexp.MustCompile(`^[0-9]+$`)` package var (or reuse if present). Keep the UUID passthrough and users-table-missing fall-through untouched.

- [ ] **Step 4: Run tests**

Run: `cd services/recs && go test ./... -count=1`
Expected: PASS (all prior cases + new telegram case).

- [ ] **Step 5: Commit**

```bash
git add services/recs
git commit -m "feat(recs): resolve /admin/recs/{id} by telegram_id too"
```

---

## Task 4: FE — `UserResolveInput.vue` shared component

**Files:**
- Create: `frontend/web/src/components/admin/UserResolveInput.vue`
- Test: `frontend/web/src/components/admin/UserResolveInput.spec.ts`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`admin.userResolve.*`)

**Interfaces:**
- Consumes: `Input`, `Spinner`, `Button` primitives; `apiClient` (grep how other composables call the gateway — reuse the same axios/fetch wrapper); i18n `$t`.
- Produces: component emitting `@resolve="(user: {id,username,public_id,telegram_id})"`. Prop `mode?: 'chip' | 'nav'` (default `'chip'`). Slot-free.

- [ ] **Step 1: Write the failing test**

`UserResolveInput.spec.ts` (Vitest + `@vue/test-utils`, mock the api client):
- typing `oronemu` + Enter → calls `GET /api/admin/users/resolve?q=oronemu`, emits `resolve` with the mocked user, clears input.
- 404 → shows the not-found message (i18n key present), no emit.
- empty input + Enter → no request.
(≥3 assertions; mirror an existing admin composable's api mock from another `.spec.ts`.)

- [ ] **Step 2: Run it, verify it fails**

Run: `cd frontend/web && bunx vitest run src/components/admin/UserResolveInput.spec.ts`
Expected: FAIL (component missing).

- [ ] **Step 3: Implement the component**

Build with the `Input` primitive, an in-flight `Spinner`, an inline destructive-token error line, Enter-to-resolve + a `+` `Button`. Placeholder + error copy via `$t('admin.userResolve.placeholder'|'.notFound')`. On success `emit('resolve', user)` and reset. lucide named import for any icon. No `window.open`, no hardcoded hex.

- [ ] **Step 4: Add i18n keys (en/ru/ja)**

Add `admin.userResolve.{placeholder,notFound,add,resolving}` to all three locales. Example ru: `"placeholder": "username, public_id, Telegram ID или UUID"`.

- [ ] **Step 5: Run tests + DS lint**

Run: `cd frontend/web && bunx vitest run src/components/admin/UserResolveInput.spec.ts && bash scripts/design-system-lint.sh && bunx tsc --noEmit`
Expected: PASS, 0 DS errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/components/admin/UserResolveInput.vue frontend/web/src/components/admin/UserResolveInput.spec.ts frontend/web/src/locales
git commit -m "feat(admin-web): shared UserResolveInput (username/public_id/tg/uuid → user)"
```

---

## Task 5: FE — recs picker adopts `UserResolveInput`

**Files:**
- Modify: `frontend/web/src/views/admin/AdminRecsPicker.vue`
- Test: `frontend/web/src/views/admin/AdminRecs.spec.ts` (or a picker spec — extend/verify)

**Interfaces:**
- Consumes: `UserResolveInput` (`@resolve`), `useRouter`, `authStore`.
- Produces: picker navigates to `/admin/recs/{resolvedUUID}` on resolve; self quick-action unchanged.

- [ ] **Step 1: Write/adjust the failing test**

Assert that resolving a user emits → `router.push('/admin/recs/<uuid>')`. Reuse the existing picker test scaffolding; mock `UserResolveInput` to emit a known user.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd frontend/web && bunx vitest run src/views/admin/AdminRecs.spec.ts`
Expected: FAIL until wired.

- [ ] **Step 3: Implement**

Replace the raw `Input` + `go()` submit with `UserResolveInput` (`mode="nav"`): on `@resolve="(u) => router.push('/admin/recs/'+u.id)"`. Keep the "You" self quick-action + autofocus + i18n. Remove the now-unused free-text submit path (the resolver validates before navigation, so no more 404 landing pages).

- [ ] **Step 4: Run tests + DS lint**

Run: `cd frontend/web && bunx vitest run src/views/admin/AdminRecs.spec.ts && bash scripts/design-system-lint.sh && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/views/admin/AdminRecsPicker.vue frontend/web/src/views/admin/AdminRecs.spec.ts
git commit -m "refactor(admin-web): recs picker uses shared UserResolveInput (+ tg support)"
```

---

## Task 6: FE — feature registry, open helper, policy API composable

**Files:**
- Create: `frontend/web/src/config/policyFeatures.ts`
- Create: `frontend/web/src/composables/useOpenFeature.ts`
- Create: `frontend/web/src/composables/useAdminPolicy.ts`
- Test: `frontend/web/src/composables/useOpenFeature.spec.ts`, `frontend/web/src/composables/useAdminPolicy.spec.ts`

**Interfaces:**
- Produces:
  - `policyFeatures.ts`: `export const POLICY_FEATURE_ROUTES: Record<string,string>` (key→same-origin relative route) + `featureRoute(key): string | undefined`.
  - `useOpenFeature()`: `{ isStandalone: boolean, openFeature(e: MouseEvent, route: string): void }` — standalone → `preventDefault` + `router.push`; else no-op (native anchor).
  - `useAdminPolicy()`: `{ list(): Promise<{flags: FeatureFlag[], rouletteEnabled: boolean}>, setFlag(key, payload): Promise<void>, setRoulette(enabled): Promise<void> }` and a `FeatureFlag` TS type `{key,roles:string[],allowUsers:string[],denyUsers:string[],roulette:boolean,failSafe:'admin'|'everyone',label:string,updatedAt:string}`.

- [ ] **Step 1: Write the failing tests**

`useOpenFeature.spec.ts`: with `matchMedia` mocked to standalone=true, `openFeature(ev,'/fanfics')` calls `ev.preventDefault()` + `router.push('/fanfics')`; with standalone=false it does neither (native anchor handles it).
`useAdminPolicy.spec.ts`: `list()` GETs `/api/admin/policy/flags` and returns parsed `{flags,rouletteEnabled}`; `setFlag('fanfic', payload)` PUTs `/api/admin/policy/flags/fanfic` with the body; `setRoulette(true)` PUTs `/api/admin/policy/roulette` `{enabled:true}`. Mock the api client.

- [ ] **Step 2: Run them, verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/useOpenFeature.spec.ts src/composables/useAdminPolicy.spec.ts`
Expected: FAIL (missing modules).

- [ ] **Step 3: Implement the three modules**

- `policyFeatures.ts`: the static map from the spec (`fanfic→/fanfics`, `gacha→/gacha`, `profile-wall→/profile`, `anidle→/anidle`, `showcase-editor→/profile#showcase`, `themes→/themes`, `status→/status`, `game→/game`, `downloads→/downloads`, `my-feedback→/my-feedback`) + `featureRoute(key)`.
- `useOpenFeature.ts`: `isStandalone = window.matchMedia?.('(display-mode: standalone)').matches || (navigator as any).standalone === true`; `openFeature(e,route){ if(isStandalone){ e.preventDefault(); router.push(route) } }`.
- `useAdminPolicy.ts`: three methods over the shared api client (reuse the wrapper other admin composables use — grep `useAdminRecs.ts`/`useAdminFeedback.ts` for the exact import), unwrapping the `{success,data}` envelope.

- [ ] **Step 4: Run tests + tsc**

Run: `cd frontend/web && bunx vitest run src/composables/useOpenFeature.spec.ts src/composables/useAdminPolicy.spec.ts && bunx tsc --noEmit`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/config/policyFeatures.ts frontend/web/src/composables/useOpenFeature.ts frontend/web/src/composables/useAdminPolicy.ts frontend/web/src/composables/useOpenFeature.spec.ts frontend/web/src/composables/useAdminPolicy.spec.ts
git commit -m "feat(admin-web): policy feature registry + open helper + admin API composable"
```

---

## Task 7: FE — `AdminPolicy.vue` Features tab

**Files:**
- Create: `frontend/web/src/views/admin/AdminPolicy.vue`
- Test: `frontend/web/src/views/admin/AdminPolicy.spec.ts`
- Modify: `frontend/web/src/locales/{en,ru,ja}.json` (`admin.policy.*`)

**Interfaces:**
- Consumes: `useAdminPolicy`, `UserResolveInput`, `useOpenFeature`, `featureRoute`, primitives (`Tabs`, `Switch`, `Badge`, `Button`, `Card`), lucide `ExternalLink`.
- Produces: the `/admin/policy` view (Features tab functional; Providers tab placeholder).

- [ ] **Step 1: Write the failing test**

`AdminPolicy.spec.ts` (mock `useAdminPolicy` returning two flags — one `roles:['admin']` with an allow user, one `roles:['everyone'] roulette:true`):
- renders one card per flag with its `label` + failSafe badge.
- toggling a role chip and clicking Save calls `setFlag` with the mutated `roles`.
- toggling the master switch calls `setRoulette`.
- toggling a per-flag roulette Switch + Save includes `roulette` in the `setFlag` payload.
- the access-preview marks an `everyone` flag visible for an anonymous identity and an `admin`-only flag hidden.
(≥5 assertions.)

- [ ] **Step 2: Run it, verify it fails**

Run: `cd frontend/web && bunx vitest run src/views/admin/AdminPolicy.spec.ts`
Expected: FAIL (view missing).

- [ ] **Step 3: Implement the view**

Build per the spec §B3–B6:
- `Tabs` — Фичи (active) / Провайдеры (placeholder empty-state, no API).
- Master roulette `Switch` bound to `rouletteEnabled` → `setRoulette`.
- Per-flag `glass-card`: header (`label`, `key` + `featureRoute(key)`, open-link `<a :href="featureRoute(key)" target="_blank" rel="noopener noreferrer" @click="openFeature($event, featureRoute(key)!)">` with lucide `ExternalLink`, failSafe `Badge`, roulette `Switch`); body (role toggle chips; allow/deny chip lists + `UserResolveInput` per list). Per-flag dirty-state + Save `Button` → `setFlag`.
- On load, resolve each allow/deny UUID → username for chips via the same api client (`/api/admin/users/resolve?q=<uuid>`); deleted user → raw id.
- Access preview (§B5): `UserResolveInput` + role/anon/guest selector → client-side `canAccess` mirror (guest→deny · deny→deny · allow→allow · everyone→allow · role→allow · else deny) → per-flag виден/скрыт.
- All strings via `$t('admin.policy.*')`.

- [ ] **Step 4: Add i18n keys (en/ru/ja)**

Add the full `admin.policy.*` namespace to all three locales (tabs, master, roles, allow/deny, failSafe, openAria, save, toasts, preview, providersPlaceholder).

- [ ] **Step 5: Run tests + DS lint + tsc**

Run: `cd frontend/web && bunx vitest run src/views/admin/AdminPolicy.spec.ts && bash scripts/design-system-lint.sh && bunx tsc --noEmit`
Expected: PASS, 0 DS errors.

- [ ] **Step 6: Commit**

```bash
git add frontend/web/src/views/admin/AdminPolicy.vue frontend/web/src/views/admin/AdminPolicy.spec.ts frontend/web/src/locales
git commit -m "feat(admin-web): /admin/policy Features tab — audience editor + access preview"
```

---

## Task 8: FE — router wire-up + secret-features redirect + cleanup

**Files:**
- Modify: `frontend/web/src/router/index.ts`
- Modify: any nav/link referencing `/admin/secret-features`
- Delete (or leave orphaned): `frontend/web/src/views/admin/AdminSecretFeatures.vue` + `.spec.ts` (per finishing cleanup)

**Interfaces:**
- Consumes: existing admin route meta guard.
- Produces: `/admin/policy` route; `/admin/secret-features` → redirect `/admin/policy`.

- [ ] **Step 1: Write/adjust the failing test**

If the router has a test asserting admin routes, add: resolving `/admin/secret-features` redirects to `/admin/policy`, and `/admin/policy` maps to `AdminPolicy.vue` with admin meta. Otherwise add a focused router spec.

- [ ] **Step 2: Run it, verify it fails**

Run: `cd frontend/web && bunx vitest run src/router` (or the router spec path)
Expected: FAIL.

- [ ] **Step 3: Implement**

- Add the `/admin/policy` route (lazy import `AdminPolicy.vue`, same admin meta as `/admin/recs`/`/admin/secret-features`).
- Replace the `/admin/secret-features` route with a `redirect: '/admin/policy'` (keep the path alive for bookmarks).
- Grep for `/admin/secret-features` and `AdminSecretFeatures` across `src/` and update nav entries/links to point at `/admin/policy`.
- Remove `AdminSecretFeatures.vue` + its spec if nothing else imports them (the roulette controls are absorbed).

- [ ] **Step 4: Run the full FE gate**

Run: `cd frontend/web && bunx vitest run && bash scripts/design-system-lint.sh && bunx tsc --noEmit && bun run build`
Expected: PASS, build succeeds, 0 DS errors.

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/router frontend/web/src
git commit -m "feat(admin-web): route /admin/policy; redirect /admin/secret-features; drop AdminSecretFeatures"
```

---

## Self-Review notes (plan author)

- Spec coverage: A1→T1/T2, A2→T4, A3→T3/T5; B1/B2/B4→T6, B3/B5/B6→T7, B7/B8→T7/T8. Providers-tab facade content + roulette footer cutover are explicitly P4 (out of scope).
- Types consistent: `FeatureFlag` shape identical to P1 domain JSON; `allowUsers/denyUsers` are UUID arrays throughout; resolver returns `{id,username,public_id,telegram_id?}` in T1 and consumed identically in T4/T7.
- Verify exact symbol names during execution: auth `AuthMiddleware`/`AdminMiddleware`, gateway admin group + `ProxyToAuth`, the FE api-client wrapper, and the router admin-meta guard — each task says to grep the neighbor that already uses it.
- Final: run `/frontend-verify` then `/animeenigma-after-update` after Task 8 (whole-branch review + deploy is a separate, owner-gated step per project workflow).
