# Admin Users Management Page — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an admin-only `/admin/users` page that lists all users, supports unified search (UUID / public_id / username / Telegram id / Telegram name), and lets an admin change a user's role — backed by a new list+role API in the `auth` service.

**Architecture:** Users live in the `auth` service. Add two nullable Telegram-display columns to the `User` model (persisted on every Telegram login), a `ListUsers`/`UpdateRole`/`UpdateTelegramProfile` repo trio, and an `AdminUsersHandler` mounted on the existing admin-gated `/admin/users` chi group. The gateway broadens its single `/admin/users/resolve` carve-out to the whole `/admin/users*` subtree → auth. The Vue frontend adds an `AdminUsers.vue` view (modeled on `AdminCollections.vue` + `useAdminFeedback.ts`), a `useAdminUsers` composable, a router entry, a dashboard card, and `admin.users.*` i18n keys in en/ru/ja.

**Tech Stack:** Go 1.x + chi + GORM (auth/gateway), Vue 3 `<script setup>` + TypeScript + axios + vue-i18n + reka-ui (frontend). Postgres. Tests: in-memory-fake handler tests (`go test`), integration-tagged repo tests (live PG), vitest for the composable, `/frontend-verify` for the FE gate.

## Global Constraints

- **Worktree only.** All edits happen in `/data/animeenigma/.claude/worktrees/admin-users/`. Never touch the base tree `/data/animeenigma`.
- **Response envelope:** auth handlers return `{success, data}` via `httputil.OK(w, data)`. List response `data` shape is `{items, total, page, page_size}` (mirrors `FeedbackListResponse`).
- **Role set:** assignable roles are exactly `user`, `librarian`, `admin` (from `libs/authz`: `authz.RoleUser`/`RoleLibrarian`/`RoleAdmin`). `guest` is ephemeral and MUST be rejected.
- **GORM AutoMigrate is additive only** — new columns appear on service restart; no manual migration. Never drop columns.
- **DS-lint is build-enforced.** Reuse the exact utility classes `AdminCollections.vue`/`AdminFeedback.vue` use (`bg-base`, `glass-card`, `text-white/70`, `bg-black/40`, `text-destructive`, `border-destructive/40`, Badge variants). No raw hex, no off-palette colors.
- **i18n parity gate:** every new key must exist in all three of `src/locales/{en,ru,ja}.json` with identical structure (`src/locales/__tests__/locale-parity.spec.ts` enforces it).
- **Self-lockout guard:** an admin may not change their own role (backend 403 + FE disabled control).
- **Metrics (per `.planning/CONVENTIONS.md`):** UXΔ = +2 (Better) · CDI = 0.03 * 21 · MVQ = Griffin 85%/80%. No time-effort units.
- **Commits:** every commit appends the three standard co-authors:
  ```
  Co-authored-by: Claude Code <noreply@anthropic.com>
  Co-authored-by: 0neymik0 <0neymik0@gmail.com>
  Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
  Stage by explicit pathspec (never `git add -A`).

Design spec: `docs/superpowers/specs/2026-07-24-admin-users-page-design.md`.

---

## Task 1: User model — Telegram-display columns + persist on login

**Files:**
- Modify: `services/auth/internal/domain/user.go` (User struct, after `TelegramID`)
- Modify: `services/auth/internal/repo/user.go` (add `UpdateTelegramProfile`)
- Modify: `services/auth/internal/service/auth.go` (`LoginWithTelegram`, before the final `return`)
- Test: `services/auth/internal/repo/user_admin_integration_test.go` (new, integration-tagged; covers this + Task 2)

**Interfaces:**
- Produces: `domain.User.TelegramUsername *string`, `domain.User.TelegramFirstName *string`; `(*repo.UserRepository).UpdateTelegramProfile(ctx, userID, tgUsername, tgFirstName string) error`.

- [ ] **Step 1: Add the two nullable columns to the User struct**

In `services/auth/internal/domain/user.go`, replace the `TelegramID` line (currently line 41) with:

```go
	TelegramID *int64 `gorm:"uniqueIndex" json:"telegram_id,omitempty"`
	// Telegram display identity, refreshed on every Telegram login (spec
	// 2026-07-24 admin users page). Distinct from Username, which is the
	// 32-char unique login handle (derived + de-duplicated). Nullable — blank
	// for existing users until their next Telegram login.
	TelegramUsername  *string `gorm:"size:64" json:"telegram_username,omitempty"`
	TelegramFirstName *string `gorm:"size:128" json:"telegram_first_name,omitempty"`
```

- [ ] **Step 2: Add the `UpdateTelegramProfile` repo method**

In `services/auth/internal/repo/user.go`, add after `UpdateCertAutoLogin` (after line 202):

```go
// UpdateTelegramProfile persists the Telegram display username / first name
// onto the user row (called on every Telegram login). Empty values are skipped
// so a login that omits a field never nulls a previously-stored value.
func (r *UserRepository) UpdateTelegramProfile(ctx context.Context, userID, tgUsername, tgFirstName string) error {
	updates := map[string]interface{}{}
	if tgUsername != "" {
		updates["telegram_username"] = tgUsername
	}
	if tgFirstName != "" {
		updates["telegram_first_name"] = tgFirstName
	}
	if len(updates) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update telegram profile: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("user")
	}
	return nil
}
```

- [ ] **Step 3: Persist on every Telegram login**

In `services/auth/internal/service/auth.go`, in `LoginWithTelegram`, replace the final line `return s.createSessionAndAuthResponse(ctx, user, sc)` (line 322) with:

```go
	// Persist the user's Telegram display identity on every login so admin
	// search (username / tg name) stays current. Best-effort: a cosmetic write
	// failure must never block login.
	if err := s.userRepo.UpdateTelegramProfile(ctx, user.ID, tgUser.Username, tgUser.FirstName); err != nil {
		s.log.Warnw("failed to persist telegram profile", "user_id", user.ID, "error", err)
	} else {
		if tgUser.Username != "" {
			v := tgUser.Username
			user.TelegramUsername = &v
		}
		if tgUser.FirstName != "" {
			v := tgUser.FirstName
			user.TelegramFirstName = &v
		}
	}

	return s.createSessionAndAuthResponse(ctx, user, sc)
```

- [ ] **Step 4: Write the integration test (red)** — this also seeds Task 2's `ListUsers`/`UpdateRole` tests

Create `services/auth/internal/repo/user_admin_integration_test.go`:

```go
//go:build integration

package repo_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/repo"
)

// usersDB connects to the dev postgres and migrates the users table (incl. the
// new telegram_username / telegram_first_name columns). Run via `make dev`, then:
//
//	INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -v
func usersDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := "host=localhost port=5432 user=postgres password=postgres dbname=animeenigma sslmode=disable"
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.User{}))
	return db
}

func containsID(users []domain.User, id string) bool {
	for _, u := range users {
		if u.ID == id {
			return true
		}
	}
	return false
}

func TestUpdateTelegramProfile_PersistsAndSkipsEmpty(t *testing.T) {
	db := usersDB(t)
	r := repo.NewUserRepository(db)
	ctx := context.Background()

	tg := int64(998811)
	u := &domain.User{Username: "tgprof_zzz", Role: authz.RoleUser, TelegramID: &tg}
	require.NoError(t, r.Create(ctx, u))
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", u.ID) })

	require.NoError(t, r.UpdateTelegramProfile(ctx, u.ID, "neo_tg", "Neo"))
	got, err := r.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TelegramUsername)
	require.Equal(t, "neo_tg", *got.TelegramUsername)
	require.NotNil(t, got.TelegramFirstName)
	require.Equal(t, "Neo", *got.TelegramFirstName)

	// Empty values must NOT null out previously-stored ones.
	require.NoError(t, r.UpdateTelegramProfile(ctx, u.ID, "", ""))
	got, err = r.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.NotNil(t, got.TelegramUsername)
	require.Equal(t, "neo_tg", *got.TelegramUsername)
}
```

- [ ] **Step 5: Build, then run the integration test**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && go build ./services/auth/...`
Expected: builds clean.

Run (requires `make dev` up / live Postgres on localhost:5432):
`INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -run TestUpdateTelegramProfile -v`
Expected: PASS. (If Postgres is not running, at minimum the build must pass; note this in your progress and run the integration tests once services are up.)

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add services/auth/internal/domain/user.go services/auth/internal/repo/user.go services/auth/internal/service/auth.go services/auth/internal/repo/user_admin_integration_test.go
git commit -F - <<'EOF'
feat(auth): persist Telegram display name on login + tg-profile repo method

Add nullable telegram_username / telegram_first_name columns to User,
refreshed on every Telegram login (best-effort), for the admin users page.

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 2: Repo `ListUsers` + `UpdateRole`

**Files:**
- Modify: `services/auth/internal/repo/user.go` (add two methods)
- Test: `services/auth/internal/repo/user_admin_integration_test.go` (extend)

**Interfaces:**
- Produces:
  - `(*repo.UserRepository).ListUsers(ctx, query, role string, limit, offset int) ([]domain.User, int64, error)` — newest-first page + total for the same filter.
  - `(*repo.UserRepository).UpdateRole(ctx, userID, role string) error` — `NotFound` when no row updated.

- [ ] **Step 1: Add `ListUsers` and `UpdateRole` to the repo**

In `services/auth/internal/repo/user.go`, add after `UpdateTelegramProfile` (from Task 1):

```go
// ListUsers returns a newest-first page of users, optionally filtered by a
// free-text query and/or an exact role, plus the total count for that filter.
// The query matches username / public_id / telegram_username / telegram_first_name
// by ILIKE substring, and the UUID id / telegram_id by exact string equality.
func (r *UserRepository) ListUsers(ctx context.Context, query, role string, limit, offset int) ([]domain.User, int64, error) {
	applyFilters := func(db *gorm.DB) *gorm.DB {
		if role != "" {
			db = db.Where("role = ?", role)
		}
		if query != "" {
			like := "%" + query + "%"
			db = db.Where(
				"username ILIKE ? OR public_id ILIKE ? OR telegram_username ILIKE ? OR telegram_first_name ILIKE ? OR id::text = ? OR CAST(telegram_id AS TEXT) = ?",
				like, like, like, like, query, query,
			)
		}
		return db
	}

	var total int64
	if err := applyFilters(r.db.WithContext(ctx).Model(&domain.User{})).Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	var users []domain.User
	if err := applyFilters(r.db.WithContext(ctx).Model(&domain.User{})).
		Order("created_at DESC").
		Limit(limit).
		Offset(offset).
		Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	return users, total, nil
}

// UpdateRole writes only the role column for the user.
func (r *UserRepository) UpdateRole(ctx context.Context, userID, role string) error {
	result := r.db.WithContext(ctx).Model(&domain.User{}).Where("id = ?", userID).Update("role", role)
	if result.Error != nil {
		return fmt.Errorf("update role: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("user")
	}
	return nil
}
```

> Note: `Count` and `Find` are built from two separate `applyFilters(...)` chains (not one reused `*gorm.DB`) to avoid GORM's shared-statement `SELECT count(*)` leakage footgun.

- [ ] **Step 2: Extend the integration test (red)**

Append to `services/auth/internal/repo/user_admin_integration_test.go`:

```go
func TestListUsers_SearchPaginateAndRoleFilter(t *testing.T) {
	db := usersDB(t)
	r := repo.NewUserRepository(db)
	ctx := context.Background()

	tg := int64(556677)
	u := &domain.User{Username: "neotokyo_zzz", Role: authz.RoleUser, TelegramID: &tg}
	require.NoError(t, r.Create(ctx, u))
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", u.ID) })
	require.NoError(t, r.UpdateTelegramProfile(ctx, u.ID, "neo_tg", "Neo"))

	// by username substring
	got, total, err := r.ListUsers(ctx, "neotokyo_zz", "", 10, 0)
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(1))
	require.True(t, containsID(got, u.ID))

	// by telegram_id exact
	got, _, err = r.ListUsers(ctx, "556677", "", 10, 0)
	require.NoError(t, err)
	require.True(t, containsID(got, u.ID))

	// by telegram name substring
	got, _, err = r.ListUsers(ctx, "neo_t", "", 10, 0)
	require.NoError(t, err)
	require.True(t, containsID(got, u.ID))

	// by exact UUID
	got, _, err = r.ListUsers(ctx, u.ID, "", 10, 0)
	require.NoError(t, err)
	require.True(t, containsID(got, u.ID))

	// role filter excludes a 'user' when filtering for 'admin'
	_, total, err = r.ListUsers(ctx, "neotokyo_zz", "admin", 10, 0)
	require.NoError(t, err)
	require.Equal(t, int64(0), total)
}

func TestUpdateRole_SuccessAndNotFound(t *testing.T) {
	db := usersDB(t)
	r := repo.NewUserRepository(db)
	ctx := context.Background()

	u := &domain.User{Username: "roletest_zzz", Role: authz.RoleUser}
	require.NoError(t, r.Create(ctx, u))
	t.Cleanup(func() { db.Exec("DELETE FROM users WHERE id = ?", u.ID) })

	require.NoError(t, r.UpdateRole(ctx, u.ID, string(authz.RoleAdmin)))
	got, err := r.GetByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, authz.RoleAdmin, got.Role)

	require.Error(t, r.UpdateRole(ctx, "00000000-0000-0000-0000-000000000000", string(authz.RoleUser)))
}
```

- [ ] **Step 3: Build + run the integration tests**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && go build ./services/auth/...`
Expected: clean.

Run (live PG): `INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -run 'ListUsers|UpdateRole|TelegramProfile' -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add services/auth/internal/repo/user.go services/auth/internal/repo/user_admin_integration_test.go
git commit -F - <<'EOF'
feat(auth): ListUsers (search+paginate) and UpdateRole repo methods

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 3: `AdminUsersHandler` (List + UpdateRole) with unit tests

**Files:**
- Create: `services/auth/internal/handler/admin_users.go`
- Test: `services/auth/internal/handler/admin_users_test.go`

**Interfaces:**
- Consumes: `domain.User`, `authz.{RoleUser,RoleLibrarian,RoleAdmin,UserIDFromContext,ContextWithClaims,Claims}`, `httputil.{OK,BadRequest,NotFound,Error,Bind}`, `liberrors.{Internal,Forbidden,NotFound}`, `chi.URLParam`, and the same-package `isNotFoundErr` (from `user_resolve.go`).
- Produces: `handler.NewAdminUsersHandler(repo adminUsersRepo, log *logger.Logger) *AdminUsersHandler` with methods `List(w, r)` and `UpdateRole(w, r)`; interface `adminUsersRepo` (satisfied by `*repo.UserRepository`).

- [ ] **Step 1: Write the failing handler tests (red)**

Create `services/auth/internal/handler/admin_users_test.go`. (`testLogger()` already exists in `user_resolve_test.go` — same package, do not redefine.)

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/go-chi/chi/v5"
)

type fakeAdminUsersRepo struct {
	users   []domain.User
	listErr error
}

func (f *fakeAdminUsersRepo) ListUsers(_ context.Context, query, role string, limit, offset int) ([]domain.User, int64, error) {
	if f.listErr != nil {
		return nil, 0, f.listErr
	}
	var out []domain.User
	for _, u := range f.users {
		if role != "" && string(u.Role) != role {
			continue
		}
		if query != "" && !strings.Contains(u.Username, query) {
			continue
		}
		out = append(out, u)
	}
	total := int64(len(out))
	if offset > len(out) {
		offset = len(out)
	}
	end := offset + limit
	if end > len(out) {
		end = len(out)
	}
	return out[offset:end], total, nil
}

func (f *fakeAdminUsersRepo) UpdateRole(_ context.Context, id, role string) error {
	for i := range f.users {
		if f.users[i].ID == id {
			f.users[i].Role = authz.Role(role)
			return nil
		}
	}
	return liberrors.NotFound("user")
}

func (f *fakeAdminUsersRepo) GetByID(_ context.Context, id string) (*domain.User, error) {
	for i := range f.users {
		if f.users[i].ID == id {
			u := f.users[i]
			return &u, nil
		}
	}
	return nil, liberrors.NotFound("user")
}

func seedAdminUsers() *fakeAdminUsersRepo {
	return &fakeAdminUsersRepo{users: []domain.User{
		{ID: "11111111-1111-1111-1111-111111111111", Username: "alice", PublicID: "pub-alice", Role: authz.RoleUser},
		{ID: "22222222-2222-2222-2222-222222222222", Username: "bob", PublicID: "pub-bob", Role: authz.RoleAdmin},
	}}
}

func TestAdminUsers_List(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?page=1&page_size=25", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Success bool `json:"success"`
		Data    struct {
			Items    []map[string]any `json:"items"`
			Total    int64            `json:"total"`
			Page     int              `json:"page"`
			PageSize int              `json:"page_size"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !env.Success || env.Data.Total != 2 || len(env.Data.Items) != 2 || env.Data.Page != 1 || env.Data.PageSize != 25 {
		t.Fatalf("unexpected envelope: %+v", env.Data)
	}
}

func TestAdminUsers_List_InvalidRole(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?role=wizard", nil)
	rec := httptest.NewRecorder()
	h.List(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func patchRoleReq(id, body, callerID string) *http.Request {
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/"+id+"/role", strings.NewReader(body))
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", id)
	ctx := context.WithValue(req.Context(), chi.RouteCtxKey, rctx)
	ctx = authz.ContextWithClaims(ctx, &authz.Claims{UserID: callerID, Role: authz.RoleAdmin})
	return req.WithContext(ctx)
}

func TestAdminUsers_UpdateRole_Success(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := patchRoleReq("11111111-1111-1111-1111-111111111111", `{"role":"admin"}`, "22222222-2222-2222-2222-222222222222")
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	var env struct {
		Data struct {
			ID   string `json:"id"`
			Role string `json:"role"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.Role != "admin" {
		t.Fatalf("role=%q want admin", env.Data.Role)
	}
}

func TestAdminUsers_UpdateRole_SelfLockout(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	id := "22222222-2222-2222-2222-222222222222"
	req := patchRoleReq(id, `{"role":"user"}`, id)
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("status=%d want 403", rec.Code)
	}
}

func TestAdminUsers_UpdateRole_InvalidRole(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := patchRoleReq("11111111-1111-1111-1111-111111111111", `{"role":"guest"}`, "22222222-2222-2222-2222-222222222222")
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status=%d want 400", rec.Code)
	}
}

func TestAdminUsers_UpdateRole_NotFound(t *testing.T) {
	h := NewAdminUsersHandler(seedAdminUsers(), testLogger())
	req := patchRoleReq("99999999-9999-9999-9999-999999999999", `{"role":"admin"}`, "22222222-2222-2222-2222-222222222222")
	rec := httptest.NewRecorder()
	h.UpdateRole(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d want 404", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && go test ./services/auth/internal/handler/ -run AdminUsers -v`
Expected: FAIL — `undefined: NewAdminUsersHandler`.

- [ ] **Step 3: Write the handler implementation**

Create `services/auth/internal/handler/admin_users.go`:

```go
package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/domain"
	"github.com/go-chi/chi/v5"
)

const (
	defaultUsersPageSize = 25
	maxUsersPageSize     = 100
)

// adminUsersRepo is the minimal repo surface AdminUsersHandler needs
// (satisfied by *repo.UserRepository).
type adminUsersRepo interface {
	ListUsers(ctx context.Context, query, role string, limit, offset int) ([]domain.User, int64, error)
	UpdateRole(ctx context.Context, id, role string) error
	GetByID(ctx context.Context, id string) (*domain.User, error)
}

// AdminUsersHandler backs the admin-only user directory:
// GET /api/admin/users and PATCH /api/admin/users/{id}/role.
type AdminUsersHandler struct {
	repo adminUsersRepo
	log  *logger.Logger
}

func NewAdminUsersHandler(repo adminUsersRepo, log *logger.Logger) *AdminUsersHandler {
	return &AdminUsersHandler{repo: repo, log: log}
}

// adminUserView is the admin-only projection of a user — it deliberately
// includes role + telegram fields, unlike domain.PublicUser.
type adminUserView struct {
	ID                string    `json:"id"`
	Username          string    `json:"username"`
	PublicID          string    `json:"public_id"`
	Role              string    `json:"role"`
	TelegramID        *int64    `json:"telegram_id,omitempty"`
	TelegramUsername  *string   `json:"telegram_username,omitempty"`
	TelegramFirstName *string   `json:"telegram_first_name,omitempty"`
	Avatar            string    `json:"avatar,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
}

func toAdminUserView(u *domain.User) adminUserView {
	return adminUserView{
		ID:                u.ID,
		Username:          u.Username,
		PublicID:          u.PublicID,
		Role:              string(u.Role),
		TelegramID:        u.TelegramID,
		TelegramUsername:  u.TelegramUsername,
		TelegramFirstName: u.TelegramFirstName,
		Avatar:            u.Avatar,
		CreatedAt:         u.CreatedAt,
	}
}

type adminUsersListResponse struct {
	Items    []adminUserView `json:"items"`
	Total    int64           `json:"total"`
	Page     int             `json:"page"`
	PageSize int             `json:"page_size"`
}

// isAssignableRole reports whether role is one an admin may store on a user.
// guest is ephemeral (never a DB row) and is rejected.
func isAssignableRole(role string) bool {
	switch role {
	case string(authz.RoleUser), string(authz.RoleAdmin), string(authz.RoleLibrarian):
		return true
	}
	return false
}

func parsePositiveInt(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil || n < 1 {
		return def
	}
	return n
}

// List handles GET /api/admin/users?q=&role=&page=&page_size=.
func (h *AdminUsersHandler) List(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	role := strings.TrimSpace(r.URL.Query().Get("role"))
	if role != "" && !isAssignableRole(role) {
		httputil.BadRequest(w, "invalid role filter")
		return
	}
	page := parsePositiveInt(r.URL.Query().Get("page"), 1)
	pageSize := parsePositiveInt(r.URL.Query().Get("page_size"), defaultUsersPageSize)
	if pageSize > maxUsersPageSize {
		pageSize = maxUsersPageSize
	}
	offset := (page - 1) * pageSize

	users, total, err := h.repo.ListUsers(r.Context(), q, role, pageSize, offset)
	if err != nil {
		h.log.Errorw("admin list users failed", "q", q, "role", role, "error", err)
		httputil.Error(w, liberrors.Internal("failed to list users"))
		return
	}

	views := make([]adminUserView, 0, len(users))
	for i := range users {
		views = append(views, toAdminUserView(&users[i]))
	}
	httputil.OK(w, adminUsersListResponse{
		Items:    views,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	})
}

// UpdateRole handles PATCH /api/admin/users/{id}/role with body {"role":"..."}.
func (h *AdminUsersHandler) UpdateRole(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(chi.URLParam(r, "id"))
	if id == "" {
		httputil.BadRequest(w, "id is required")
		return
	}
	var body struct {
		Role string `json:"role"`
	}
	if err := httputil.Bind(r, &body); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}
	role := strings.TrimSpace(body.Role)
	if !isAssignableRole(role) {
		httputil.BadRequest(w, "invalid role")
		return
	}
	// Self-lockout guard: an admin may not change their own role (prevents
	// accidentally demoting yourself out of admin).
	if id == authz.UserIDFromContext(r.Context()) {
		httputil.Error(w, liberrors.Forbidden("cannot change your own role"))
		return
	}
	if err := h.repo.UpdateRole(r.Context(), id, role); err != nil {
		if isNotFoundErr(err) {
			httputil.NotFound(w, "user")
			return
		}
		h.log.Errorw("admin update role failed", "user_id", id, "role", role, "error", err)
		httputil.Error(w, liberrors.Internal("failed to update role"))
		return
	}
	u, err := h.repo.GetByID(r.Context(), id)
	if err != nil {
		h.log.Errorw("admin update role reload failed", "user_id", id, "error", err)
		httputil.Error(w, liberrors.Internal("failed to load user"))
		return
	}
	httputil.OK(w, toAdminUserView(u))
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && go test ./services/auth/internal/handler/ -run AdminUsers -v`
Expected: PASS (all 6 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add services/auth/internal/handler/admin_users.go services/auth/internal/handler/admin_users_test.go
git commit -F - <<'EOF'
feat(auth): AdminUsersHandler — list/search users + guarded role change

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 4: Wire the handler into DI + auth router

**Files:**
- Modify: `services/auth/internal/transport/router.go` (`NewRouter` signature + `/admin/users` group)
- Modify: `services/auth/cmd/auth-api/main.go` (construct handler + pass to `NewRouter`)

**Interfaces:**
- Consumes: `handler.NewAdminUsersHandler` (Task 3), the existing `userRepo` var, `AuthMiddleware`, `AdminMiddleware`.
- Produces: routes `GET /api/admin/users` → `List`, `PATCH /api/admin/users/{id}/role` → `UpdateRole` (both admin-gated).

- [ ] **Step 1: Add the handler param to `NewRouter` and register the routes**

In `services/auth/internal/transport/router.go`, add `adminUsersHandler *handler.AdminUsersHandler` to the `NewRouter` signature, immediately after `userResolveHandler *handler.UserResolveHandler` (line 21):

```go
	userResolveHandler *handler.UserResolveHandler,
	adminUsersHandler *handler.AdminUsersHandler,
	passkeyHandler *handler.PasskeyHandler,
```

Then replace the existing `/admin/users` group (lines 128-132) with:

```go
		// Admin-only user management + canonical resolve endpoint. List/search
		// all users, change a user's role, and resolve a single identifier
		// (UUID/username/public_id/telegram_id) to the canonical record.
		r.Route("/admin/users", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminMiddleware)
			r.Get("/", adminUsersHandler.List)
			r.Patch("/{id}/role", adminUsersHandler.UpdateRole)
			r.Get("/resolve", userResolveHandler.Resolve)
		})
```

- [ ] **Step 2: Construct the handler and thread it into `NewRouter` in `main.go`**

In `services/auth/cmd/auth-api/main.go`, add after the `userResolveHandler` line (line 129):

```go
	userResolveHandler := handler.NewUserResolveHandler(userRepo, log)
	adminUsersHandler := handler.NewAdminUsersHandler(userRepo, log)
```

Then update the `transport.NewRouter(...)` call (line 137) to pass `adminUsersHandler` right after `userResolveHandler`:

```go
	router := transport.NewRouter(authHandler, telegramBotHandler, userHandler, sessionsHandler, magicLinkHandler, userResolveHandler, adminUsersHandler, passkeyHandler, certHandler, cfg.JWT, log, metricsCollector)
```

- [ ] **Step 3: Build the whole auth service**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && go build ./services/auth/... && go vet ./services/auth/internal/handler/ ./services/auth/internal/transport/`
Expected: clean build, no vet errors. (Existing auth handler tests still pass: `go test ./services/auth/internal/handler/`.)

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add services/auth/internal/transport/router.go services/auth/cmd/auth-api/main.go
git commit -F - <<'EOF'
feat(auth): mount admin users list + role routes on /api/admin/users

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 5: Gateway — broaden `/admin/users` proxy to auth

**Files:**
- Modify: `services/gateway/internal/transport/router.go` (the `/admin/users/resolve` group, ~line 662-667)
- Test: `services/gateway/internal/transport/router_resolve_test.go` (extend)

**Interfaces:**
- Consumes: `proxyHandler.ProxyToAuth`, the existing admin middleware chain, and the test harness `buildResolveGatewayRouter` / `signTestJWT` / `gw.authGotURL`.
- Produces: `/api/admin/users` and `/api/admin/users/*` proxy to the auth service (resolve continues to work under the wildcard).

- [ ] **Step 1: Write the failing gateway tests (red)**

Append to `services/gateway/internal/transport/router_resolve_test.go`:

```go
func TestRouter_AdminUsersList_AdminJWT_ProxiesToAuth(t *testing.T) {
	gw := buildResolveGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/users?q=test&page=1", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.RemoteAddr = "10.0.0.32:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.authGotURL:
		if got != "/api/admin/users" {
			t.Errorf("auth backend received path = %q; want /api/admin/users", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auth backend never received /api/admin/users")
	}
}

func TestRouter_AdminUsersRole_AdminJWT_ProxiesToAuth(t *testing.T) {
	gw := buildResolveGatewayRouter(t)
	defer gw.teardown()

	token := signTestJWT(t, authz.RoleAdmin)
	req := httptest.NewRequest(http.MethodPatch, "/api/admin/users/abc-123/role", strings.NewReader(`{"role":"admin"}`))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = "10.0.0.32:1234"
	rec := httptest.NewRecorder()
	gw.router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d; want 200 (body=%q)", rec.Code, rec.Body.String())
	}
	select {
	case got := <-gw.authGotURL:
		if got != "/api/admin/users/abc-123/role" {
			t.Errorf("auth backend received path = %q; want /api/admin/users/abc-123/role", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("auth backend never received the role PATCH")
	}
}
```

> If `strings` is not already imported in this test file, add it. The existing `TestRouter_AdminUsersResolve_*` tests must continue to pass (resolve now matches the `/admin/users/*` wildcard).

- [ ] **Step 2: Run to verify they fail**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && go test ./services/gateway/internal/transport/ -run 'AdminUsersList|AdminUsersRole' -v`
Expected: FAIL — the list/role paths fall through to the generic `/admin/*` → catalog group, so the auth backend never receives them (test times out / catalog gets it).

- [ ] **Step 3: Broaden the carve-out**

In `services/gateway/internal/transport/router.go`, in the `/admin/users/resolve` group, replace the single line:

```go
			r.HandleFunc("/admin/users/resolve", proxyHandler.ProxyToAuth)
```

with:

```go
			// List + role management + resolve all live in auth. The bare path
			// and the wildcard are both registered (chi's /* does not match the
			// slash-less bare path). A more-specific static/param subtree still
			// wins over the generic /admin/* -> catalog group.
			r.HandleFunc("/admin/users", proxyHandler.ProxyToAuth)
			r.HandleFunc("/admin/users/*", proxyHandler.ProxyToAuth)
```

Also update that group's leading comment (the block above it referencing "static /admin/users/resolve path") to note it now covers the whole `/admin/users` subtree.

- [ ] **Step 4: Run to verify they pass (incl. the pre-existing resolve tests)**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && go test ./services/gateway/internal/transport/ -run AdminUsers -v`
Expected: PASS — `AdminUsersList`, `AdminUsersRole`, and the three existing `AdminUsersResolve_*` tests all green.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add services/gateway/internal/transport/router.go services/gateway/internal/transport/router_resolve_test.go
git commit -F - <<'EOF'
feat(gateway): proxy /api/admin/users* subtree to auth (list + role + resolve)

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 6: Frontend API client — `listUsers` + `updateUserRole` + types

**Files:**
- Modify: `frontend/web/src/api/client.ts` (add types near `ResolvedUser`; add methods in `adminApi`)

**Interfaces:**
- Produces: `AdminUser`, `AdminUsersListResponse` types; `adminApi.listUsers(params)`, `adminApi.updateUserRole(id, role)`.

- [ ] **Step 1: Add the types**

In `frontend/web/src/api/client.ts`, directly after the `ResolvedUser` interface (ends line 619), add:

```ts
/** Admin-only user projection (includes role + telegram, unlike PublicUser). */
export interface AdminUser {
  id: string
  username: string
  public_id: string
  role: string
  telegram_id?: number
  telegram_username?: string
  telegram_first_name?: string
  avatar?: string
  created_at: string
}

/** Paginated admin users list envelope (matches the auth service response). */
export interface AdminUsersListResponse {
  items: AdminUser[]
  total: number
  page: number
  page_size: number
}
```

- [ ] **Step 2: Add the two `adminApi` methods**

In the `adminApi` object (starts line 733), add next to `resolveUser` (line 738):

```ts
  // Admin user directory — list/search all users (auth service, /api/admin/users).
  listUsers: (params?: { q?: string; role?: string; page?: number; page_size?: number }) =>
    apiClient.get<AdminUsersListResponse | { data: AdminUsersListResponse }>('/admin/users', { params }),
  // Change a user's role (user | librarian | admin). Backend refuses to change
  // the caller's own role (403).
  updateUserRole: (id: string, role: string) =>
    apiClient.patch<AdminUser | { data: AdminUser }>(`/admin/users/${encodeURIComponent(id)}/role`, { role }),
```

- [ ] **Step 3: Type-check**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users/frontend/web && bunx tsc --noEmit`
Expected: no new errors referencing `client.ts` / the new types.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add frontend/web/src/api/client.ts
git commit -F - <<'EOF'
feat(web): adminApi.listUsers + updateUserRole + AdminUser types

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 7: `useAdminUsers` composable + unit tests

**Files:**
- Create: `frontend/web/src/composables/useAdminUsers.ts`
- Test: `frontend/web/src/composables/__tests__/useAdminUsers.spec.ts`

**Interfaces:**
- Consumes: `adminApi.listUsers`, `adminApi.updateUserRole`, `AdminUser`, `AdminUsersListResponse`.
- Produces: `useAdminUsers()` → `{ items, total, page, pageSize, isLoading, error, query, roleFilter, refresh, applyFilters, setPage, changeRole }`.

- [ ] **Step 1: Write the failing composable spec (red)**

Create `frontend/web/src/composables/__tests__/useAdminUsers.spec.ts`:

```ts
import { describe, it, expect, vi, beforeEach } from 'vitest'

const listUsers = vi.fn()
const updateUserRole = vi.fn()
vi.mock('@/api/client', () => ({
  adminApi: {
    listUsers: (...a: unknown[]) => listUsers(...a),
    updateUserRole: (...a: unknown[]) => updateUserRole(...a),
  },
}))

import { useAdminUsers } from '@/composables/useAdminUsers'

describe('useAdminUsers', () => {
  beforeEach(() => {
    listUsers.mockReset()
    updateUserRole.mockReset()
  })

  it('maps the list envelope and normalizes filters (all -> undefined, trims q)', async () => {
    listUsers.mockResolvedValue({
      data: { success: true, data: { items: [{ id: 'u1', username: 'a', public_id: 'p', role: 'user', created_at: '' }], total: 1, page: 1, page_size: 25 } },
    })
    const u = useAdminUsers()
    u.query.value = '  neo  '
    u.roleFilter.value = 'all'
    await u.refresh()
    expect(listUsers).toHaveBeenCalledWith({ q: 'neo', role: undefined, page: 1, page_size: 25 })
    expect(u.items.value).toHaveLength(1)
    expect(u.total.value).toBe(1)
  })

  it('maps a 403 to the "403" sentinel and clears items', async () => {
    listUsers.mockRejectedValue({ response: { status: 403 } })
    const u = useAdminUsers()
    await u.refresh()
    expect(u.error.value).toBe('403')
    expect(u.items.value).toEqual([])
  })

  it('changeRole replaces the row with the updated user', async () => {
    listUsers.mockResolvedValue({
      data: { data: { items: [{ id: 'u1', username: 'a', public_id: 'p', role: 'user', created_at: '' }], total: 1, page: 1, page_size: 25 } },
    })
    updateUserRole.mockResolvedValue({ data: { data: { id: 'u1', username: 'a', public_id: 'p', role: 'admin', created_at: '' } } })
    const u = useAdminUsers()
    await u.refresh()
    await u.changeRole('u1', 'admin')
    expect(updateUserRole).toHaveBeenCalledWith('u1', 'admin')
    expect(u.items.value[0].role).toBe('admin')
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users/frontend/web && bunx vitest run src/composables/__tests__/useAdminUsers.spec.ts`
Expected: FAIL — cannot resolve `@/composables/useAdminUsers`.

- [ ] **Step 3: Write the composable**

Create `frontend/web/src/composables/useAdminUsers.ts`:

```ts
import { ref } from 'vue'
import { adminApi, type AdminUser, type AdminUsersListResponse } from '@/api/client'

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

function mapErr(e: unknown): string {
  const obj = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  if (obj?.response?.status === 403) return '403'
  return obj?.response?.data?.error?.message || obj?.message || 'admin.users.errorGeneric'
}

export function useAdminUsers() {
  const items = ref<AdminUser[]>([])
  const total = ref(0)
  const page = ref(1)
  const pageSize = ref(25)
  const isLoading = ref(false)
  const error = ref<string | null>(null)

  const query = ref('')
  // 'all' is the "no filter" sentinel — reka-ui Select forbids empty-string values.
  const roleFilter = ref('all')

  async function refresh(): Promise<void> {
    isLoading.value = true
    error.value = null
    try {
      const res = await adminApi.listUsers({
        q: query.value.trim() || undefined,
        role: roleFilter.value && roleFilter.value !== 'all' ? roleFilter.value : undefined,
        page: page.value,
        page_size: pageSize.value,
      })
      const env = unwrap<AdminUsersListResponse>(res.data)
      items.value = env.items ?? []
      total.value = env.total ?? 0
      page.value = env.page ?? 1
      pageSize.value = env.page_size ?? pageSize.value
    } catch (e: unknown) {
      error.value = mapErr(e)
      items.value = []
      total.value = 0
    } finally {
      isLoading.value = false
    }
  }

  // applyFilters resets to page 1 and reloads — call from filter @change / search.
  function applyFilters(): Promise<void> {
    page.value = 1
    return refresh()
  }

  function setPage(p: number): Promise<void> {
    if (p < 1) return Promise.resolve()
    page.value = p
    return refresh()
  }

  // changeRole calls the API and swaps the updated row in place. Throws on
  // failure so the caller can surface the error + refresh.
  async function changeRole(id: string, role: string): Promise<void> {
    const res = await adminApi.updateUserRole(id, role)
    const updated = unwrap<AdminUser>(res.data)
    const idx = items.value.findIndex((u) => u.id === id)
    if (idx !== -1 && updated) items.value[idx] = updated
  }

  return { items, total, page, pageSize, isLoading, error, query, roleFilter, refresh, applyFilters, setPage, changeRole }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users/frontend/web && bunx vitest run src/composables/__tests__/useAdminUsers.spec.ts`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add frontend/web/src/composables/useAdminUsers.ts frontend/web/src/composables/__tests__/useAdminUsers.spec.ts
git commit -F - <<'EOF'
feat(web): useAdminUsers composable (list/search/paginate + role change)

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 8: `AdminUsers.vue` view

**Files:**
- Create: `frontend/web/src/views/admin/AdminUsers.vue`

**Interfaces:**
- Consumes: `useAdminUsers`, `useConfirm`, `useAuthStore` (for `auth.user?.id`), UI components `Input`, `Select`, `Spinner`, `Badge`, `Avatar`, `PaginationBar`, and `admin.users.*` / `admin.recs.error403` / `common.cancel` i18n keys (added in Task 10).

- [ ] **Step 1: Verify the auth store exposes the current user id**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && grep -n "user" frontend/web/src/stores/auth.ts | head -30`
Confirm the store exposes the authenticated user object with an `id` field (used as `auth.user?.id`). If the accessor differs (e.g. `currentUser`), adjust the `myId` line in Step 2 accordingly.

- [ ] **Step 2: Create the view**

Create `frontend/web/src/views/admin/AdminUsers.vue`:

```vue
<template>
  <div class="min-h-screen bg-base">
    <div class="container mx-auto px-4 py-8 max-w-7xl">
      <!-- Header -->
      <div class="flex flex-wrap items-center justify-between gap-3 mb-6">
        <h1 class="text-3xl font-semibold text-white">{{ $t('admin.users.title') }}</h1>
        <span v-if="!isLoading && error !== '403'" class="text-sm text-white/50">
          {{ $t('admin.users.totalCount', { count: total }) }}
        </span>
      </div>

      <!-- Filters -->
      <div class="flex flex-wrap items-end gap-3 mb-6">
        <div class="flex-1 min-w-[220px]">
          <Input
            v-model="query"
            size="sm"
            type="search"
            clearable
            :label="$t('admin.users.searchLabel')"
            :placeholder="$t('admin.users.searchPlaceholder')"
          />
        </div>
        <Select
          v-model="roleFilter"
          size="sm"
          :options="roleFilterOptions"
          :label="$t('admin.users.roleFilter')"
          @change="applyFilters"
        />
      </div>

      <!-- Error states -->
      <div v-if="error === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ $t('admin.recs.error403') }}</p>
      </div>
      <div v-else-if="error" class="glass-card p-4 mb-6 border border-destructive/40">
        <p class="text-destructive">{{ error }}</p>
      </div>

      <!-- Loading -->
      <div v-if="isLoading" class="flex justify-center py-12">
        <Spinner size="lg" />
      </div>

      <!-- Empty -->
      <div v-else-if="items.length === 0" class="glass-card p-8 text-center text-white/60">
        <p>{{ $t('admin.users.empty') }}</p>
      </div>

      <!-- Table -->
      <div v-else class="glass-card overflow-x-auto">
        <table class="w-full text-sm text-white">
          <thead class="bg-black/40 backdrop-blur">
            <tr class="text-white/70 text-xs uppercase">
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colUser') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colPublicId') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colRole') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colTelegram') }}</th>
              <th scope="col" class="px-3 py-2 text-left">{{ $t('admin.users.colJoined') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="u in items" :key="u.id" class="border-t border-white/10 hover:bg-white/5">
              <td class="px-3 py-2">
                <div class="flex items-center gap-2">
                  <Avatar :src="u.avatar" :name="u.username" size="sm" />
                  <span>{{ u.username }}</span>
                </div>
              </td>
              <td class="px-3 py-2 font-mono text-white/70 text-xs">{{ u.public_id }}</td>
              <td class="px-3 py-2">
                <div class="flex items-center gap-2">
                  <Badge :variant="roleBadgeVariant(u.role)" size="sm">{{ roleLabel(u.role) }}</Badge>
                  <Select
                    :model-value="u.role"
                    size="xs"
                    :options="assignableRoleOptions"
                    :disabled="u.id === myId"
                    :aria-label="$t('admin.users.changeRoleAria', { user: u.username })"
                    @change="(v) => onRoleChange(u, String(v))"
                  />
                </div>
              </td>
              <td class="px-3 py-2 text-white/70 text-xs">
                <template v-if="u.telegram_id">
                  <span class="font-mono">{{ u.telegram_id }}</span>
                  <span v-if="tgName(u)" class="text-white/50"> · {{ tgName(u) }}</span>
                </template>
                <span v-else class="text-white/30">—</span>
              </td>
              <td class="px-3 py-2 text-white/60 text-xs">{{ formatDate(u.created_at) }}</td>
            </tr>
          </tbody>
        </table>
      </div>

      <!-- Pagination -->
      <div class="mt-6 flex justify-center">
        <PaginationBar :current-page="page" :total-pages="totalPages" @update:current-page="setPage" />
      </div>
    </div>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, onUnmounted, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import Input from '@/components/ui/Input.vue'
import Select from '@/components/ui/Select.vue'
import { Spinner, Badge, Avatar, PaginationBar } from '@/components/ui'
import { useConfirm } from '@/composables/useConfirm'
import { useAdminUsers } from '@/composables/useAdminUsers'
import { useAuthStore } from '@/stores/auth'
import type { AdminUser } from '@/api/client'

const { t } = useI18n()
const { confirm } = useConfirm()
const auth = useAuthStore()
const myId = computed(() => auth.user?.id)

const {
  items, total, page, pageSize, isLoading, error,
  query, roleFilter,
  refresh, applyFilters, setPage, changeRole,
} = useAdminUsers()

const totalPages = computed(() => Math.max(1, Math.ceil(total.value / pageSize.value)))

const roleFilterOptions = computed(() => [
  { value: 'all', label: t('admin.users.roleAll') },
  { value: 'user', label: t('admin.users.roleUser') },
  { value: 'librarian', label: t('admin.users.roleLibrarian') },
  { value: 'admin', label: t('admin.users.roleAdmin') },
])
const assignableRoleOptions = computed(() => [
  { value: 'user', label: t('admin.users.roleUser') },
  { value: 'librarian', label: t('admin.users.roleLibrarian') },
  { value: 'admin', label: t('admin.users.roleAdmin') },
])

function roleLabel(role: string): string {
  const key = `admin.users.role${role.charAt(0).toUpperCase()}${role.slice(1)}`
  const label = t(key)
  return label === key ? role : label
}
function roleBadgeVariant(role: string): 'primary' | 'warning' | 'default' {
  if (role === 'admin') return 'primary'
  if (role === 'librarian') return 'warning'
  return 'default'
}
function tgName(u: AdminUser): string {
  return u.telegram_username || u.telegram_first_name || ''
}
function formatDate(iso: string): string {
  if (!iso) return '—'
  const d = new Date(iso)
  return Number.isNaN(d.getTime()) ? '—' : d.toLocaleDateString()
}

async function onRoleChange(u: AdminUser, role: string) {
  if (role === u.role) return
  const ok = await confirm({
    title: t('admin.users.confirmRoleTitle'),
    description: t('admin.users.confirmRoleDesc', { user: u.username, role: roleLabel(role) }),
    confirmText: t('admin.users.confirmRoleConfirm'),
    cancelText: t('common.cancel'),
  })
  if (!ok) {
    // Reset the Select's optimistic value by re-fetching the current rows.
    await refresh()
    return
  }
  try {
    await changeRole(u.id, role)
  } catch (e: unknown) {
    const err = e as { response?: { data?: { error?: { message?: string } } }; message?: string }
    error.value = err.response?.data?.error?.message || err.message || t('admin.users.errorGeneric')
    await refresh()
  }
}

// Debounce free-text search (300ms) — mirrors AdminFeedback.
let searchDebounce: ReturnType<typeof setTimeout> | null = null
watch(query, () => {
  if (searchDebounce) clearTimeout(searchDebounce)
  searchDebounce = setTimeout(() => applyFilters(), 300)
})
onUnmounted(() => {
  if (searchDebounce) clearTimeout(searchDebounce)
})

onMounted(refresh)
</script>
```

- [ ] **Step 3: DS-lint + type-check the new view**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && bash frontend/web/scripts/design-system-lint.sh 2>&1 | tail -20`
Expected: `ERRORS: 0` (the PostToolUse DS hook also runs on save).

Run: `cd /data/animeenigma/.claude/worktrees/admin-users/frontend/web && bunx tsc --noEmit`
Expected: no errors in `AdminUsers.vue`. (If `auth.user?.id` errors, correct the accessor per Step 1.)

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add frontend/web/src/views/admin/AdminUsers.vue
git commit -F - <<'EOF'
feat(web): AdminUsers.vue — searchable user directory with role management

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 9: Router entry + dashboard tool card

**Files:**
- Modify: `frontend/web/src/router/index.ts` (add `/admin/users` route)
- Modify: `frontend/web/src/views/admin/AdminDashboard.vue` (add `users` tool card)

**Interfaces:**
- Consumes: `AdminUsers.vue` (Task 8), `admin.users.title` / `admin.dashboard.usersDesc` keys (Task 10).

- [ ] **Step 1: Add the route**

In `frontend/web/src/router/index.ts`, add inside the `/admin/*` block (next to the `admin-feedback` entry, ~line 260):

```ts
    {
      // Admin users — list, search and role management.
      path: '/admin/users',
      name: 'admin-users',
      component: () => import('@/views/admin/AdminUsers.vue'),
      meta: { titleKey: 'admin.users.title', requiresAuth: true, requiresAdmin: true }
    },
```

- [ ] **Step 2: Add the dashboard tool card**

In `frontend/web/src/views/admin/AdminDashboard.vue`, add an entry to the `tools` array (after the `collections` entry):

```ts
  {
    key: 'users',
    to: '/admin/users',
    label: 'admin.users.title',
    desc: 'admin.dashboard.usersDesc',
    accent: 'cyan',
    icon: 'M17 20h5v-2a4 4 0 00-3-3.87M9 20H4v-2a4 4 0 013-3.87m6-1.13a4 4 0 10-4-4 4 4 0 004 4z',
  },
```

- [ ] **Step 3: Type-check**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users/frontend/web && bunx tsc --noEmit`
Expected: no new errors.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add frontend/web/src/router/index.ts frontend/web/src/views/admin/AdminDashboard.vue
git commit -F - <<'EOF'
feat(web): route /admin/users + dashboard Users card

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 10: i18n — `admin.users` block (en/ru/ja) + `usersDesc`

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ru.json`
- Modify: `frontend/web/src/locales/ja.json`

**Interfaces:**
- Produces: `admin.users.*` keys + `admin.dashboard.usersDesc` in all three locales (identical key structure).

- [ ] **Step 1: Add the block to `en.json`**

In `src/locales/en.json`, add `"usersDesc": "Browse, search and manage user accounts and roles."` inside the existing `admin.dashboard` object, and add this sibling under `admin` (e.g. after `collections`):

```json
    "users": {
      "title": "Users (admin)",
      "searchLabel": "Search",
      "searchPlaceholder": "Username, public ID, UUID, Telegram ID or name",
      "roleFilter": "Role",
      "roleAll": "All roles",
      "roleUser": "User",
      "roleLibrarian": "Librarian",
      "roleAdmin": "Admin",
      "colUser": "User",
      "colPublicId": "Public ID",
      "colRole": "Role",
      "colTelegram": "Telegram",
      "colJoined": "Joined",
      "totalCount": "{count} users",
      "empty": "No users match your search.",
      "changeRoleAria": "Change role for {user}",
      "confirmRoleTitle": "Change role",
      "confirmRoleDesc": "Change {user} to {role}?",
      "confirmRoleConfirm": "Change role",
      "errorGeneric": "Something went wrong."
    },
```

- [ ] **Step 2: Add the block to `ru.json`**

`admin.dashboard.usersDesc`: `"Просмотр, поиск и управление аккаунтами и ролями пользователей."` and:

```json
    "users": {
      "title": "Пользователи (админ)",
      "searchLabel": "Поиск",
      "searchPlaceholder": "Имя, public ID, UUID, Telegram ID или имя",
      "roleFilter": "Роль",
      "roleAll": "Все роли",
      "roleUser": "Пользователь",
      "roleLibrarian": "Библиотекарь",
      "roleAdmin": "Администратор",
      "colUser": "Пользователь",
      "colPublicId": "Public ID",
      "colRole": "Роль",
      "colTelegram": "Telegram",
      "colJoined": "Регистрация",
      "totalCount": "Пользователей: {count}",
      "empty": "Нет пользователей по вашему запросу.",
      "changeRoleAria": "Изменить роль для {user}",
      "confirmRoleTitle": "Изменить роль",
      "confirmRoleDesc": "Изменить роль {user} на {role}?",
      "confirmRoleConfirm": "Изменить роль",
      "errorGeneric": "Что-то пошло не так."
    },
```

- [ ] **Step 3: Add the block to `ja.json`**

`admin.dashboard.usersDesc`: `"ユーザーアカウントとロールの閲覧・検索・管理。"` and:

```json
    "users": {
      "title": "ユーザー（管理）",
      "searchLabel": "検索",
      "searchPlaceholder": "ユーザー名、公開ID、UUID、Telegram IDまたは名前",
      "roleFilter": "ロール",
      "roleAll": "すべてのロール",
      "roleUser": "ユーザー",
      "roleLibrarian": "ライブラリアン",
      "roleAdmin": "管理者",
      "colUser": "ユーザー",
      "colPublicId": "公開ID",
      "colRole": "ロール",
      "colTelegram": "Telegram",
      "colJoined": "登録日",
      "totalCount": "ユーザー数: {count}",
      "empty": "条件に一致するユーザーがいません。",
      "changeRoleAria": "{user} のロールを変更",
      "confirmRoleTitle": "ロールの変更",
      "confirmRoleDesc": "{user} のロールを {role} に変更しますか？",
      "confirmRoleConfirm": "ロールを変更",
      "errorGeneric": "エラーが発生しました。"
    },
```

- [ ] **Step 4: Verify locale parity + JSON validity**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users/frontend/web && bunx vitest run src/locales/__tests__/locale-parity.spec.ts`
Expected: PASS (all three locales have identical `admin.users.*` keys). Fix any missing-key mismatch it reports.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma/.claude/worktrees/admin-users
git add frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git commit -F - <<'EOF'
i18n(web): admin.users.* keys (en/ru/ja) + dashboard usersDesc

Co-authored-by: Claude Code <noreply@anthropic.com>
Co-authored-by: 0neymik0 <0neymik0@gmail.com>
Co-authored-by: NANDIorg <super.egor.mamonov@yandex.ru>
EOF
```

---

## Task 11: Full verification + after-update

**Files:** none (verification only)

- [ ] **Step 1: Backend build + unit tests**

Run:
```bash
cd /data/animeenigma/.claude/worktrees/admin-users
go build ./services/auth/... ./services/gateway/...
go test ./services/auth/internal/handler/ ./services/gateway/internal/transport/
```
Expected: clean build; handler + gateway tests PASS.

- [ ] **Step 2: Repo integration tests (if `make dev` / live PG available)**

Run: `INTEGRATION=1 go test -tags=integration ./services/auth/internal/repo/... -run 'ListUsers|UpdateRole|TelegramProfile' -v`
Expected: PASS. If PG is not up, note this and run after redeploy.

- [ ] **Step 3: Frontend gate**

Run: `cd /data/animeenigma/.claude/worktrees/admin-users && bash bin/ae-fe-verify.sh frontend/web/src/views/admin/AdminUsers.vue frontend/web/src/composables/useAdminUsers.ts frontend/web/src/api/client.ts`
Then, because locale JSON changed, also run i18n parity + locale specs:
`cd frontend/web && bash scripts/i18n-lint.sh && bunx vitest run src/locales/__tests__/locale-parity.spec.ts`
Expected: DS-lint 0 errors, eslint clean, `bun run build` succeeds, vitest specs PASS.

- [ ] **Step 4: Land + after-update**

Hand off to `/animeenigma-after-update` (from this worktree) to `/simplify` the changed code, lint/build, redeploy `auth`, `gateway`, and `web`, run health checks, add a Russian Trump-mode changelog entry, and commit + push to `main`. Then remove the worktree once after-update is green.

Manual smoke (owner, post-deploy): open `/admin/users`, confirm the list loads, search by username / Telegram id resolves, and changing another user's role works while your own row's control is disabled.

---

## Self-Review

**Spec coverage:**
- List all users → Task 2 (`ListUsers`) + Task 3 (`List`) + Task 8 (table). ✓
- Unified search (uuid/public_id/username/tg id/tg name) → Task 2 SQL + Task 6/7 params. ✓
- Change role → Task 2 (`UpdateRole`) + Task 3 (`UpdateRole` handler, self-lockout) + Task 8 (inline Select + confirm). ✓
- Persist + search tg name → Task 1 (columns + login write) + Task 2 (ILIKE over tg name). ✓
- Gateway exposure → Task 5. ✓
- Route + dashboard card + i18n → Tasks 9, 10. ✓
- Admin gate → reuses `AuthMiddleware`+`AdminMiddleware` (auth) and `JWTValidationMiddleware`+`AdminRoleMiddleware` (gateway); `requiresAdmin` meta on the route. ✓
- Known limitations (role-latency via JWT, tg backfill) → documented in the spec; no code owed this iteration. ✓

**Placeholder scan:** No TBD/TODO; every code step has complete code; every command has an expected result. ✓

**Type consistency:** `adminUsersRepo` interface (Task 3) exactly matches the repo methods produced in Tasks 1–2 (`ListUsers(ctx, query, role string, limit, offset int)`, `UpdateRole(ctx, id, role string)`, `GetByID`). FE `AdminUsersListResponse {items,total,page,page_size}` matches the backend `adminUsersListResponse` JSON tags and the `useAdminUsers` unwrap. `NewRouter` gains one param, wired identically at the `main.go` call site. ✓
