# RBAC and roulette — Phase 1: `policy-service` foundation — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up `policy-service` (:8098) — a runtime feature-access authority whose `FeatureFlag` store resolves `(userID, role) → can-access` for every dark-shipped/secret feature, exposed as an internal ruleset feed (for the gateway, Phase 2) and a per-user FE feed (for the SPA, Phase 4), gateway-routed and seeded so day-one behavior equals today's dark-ship defaults.

**Architecture:** A new Go microservice mirroring `services/recs` (self-validated JWT via `libs/authz`, own table in the shared `animeenigma` Postgres DB, chi router, Prometheus metrics). Pure domain resolver (`FeatureFlag.CanAccess`) + GORM repo + service layer + three HTTP surfaces (admin CRUD, `/api/policy/features/mine`, `/internal/policy/ruleset`). The gateway proxies policy's own routes in this phase; **enforcement middleware (`FeatureGate`) is Phase 2**, admin UI is Phase 3, FE cutover + provider-facade tab are Phase 4.

**Tech Stack:** Go 1.25, chi/v5, GORM (Postgres runtime / sqlite `:memory:` tests), `libs/{authz,cache,database,httputil,logger,metrics,tracing,errors}`, testify.

## Global Constraints

- **Port 8098.** Service name `policy-service`, module `github.com/ILITA-hub/animeenigma/services/policy`, binary `cmd/policy-api`.
- **libs/ module rule (go.work Dockerfile ripple):** adding `./services/policy` to `go.work` means EVERY other service Dockerfile must gain `COPY services/policy/go.mod services/policy/go.sum* ./services/policy/`, or its Docker build breaks. `stealth-scraper` (sidecar, own repo) and `maintenance` (no Dockerfile) are exempt.
- **No testcontainers.** Repo/service tests use `gorm.io/driver/sqlite` `:memory:` (codebase convention). `FeatureFlag`'s PK is a string (`key`), so no `BeforeCreate` UUID hook is needed.
- **GORM bool no-default gotcha:** never put a `default:` tag on a `bool`/rich field written explicitly — GORM omits zero-values that carry a default, so `Roulette:false` would silently store the default. All rows are written with explicit values via upsert; absence resolves fail-open in the service layer.
- **Fail-open / fail-static:** `/features/mine` is fail-open; the ruleset feed is a compact snapshot the gateway will cache fail-static (Phase 2). This phase must never introduce a hard-outage dependency.
- **Portable slice columns:** persist `[]string` audience lists as a custom `StringList` JSON-text type (`type:text`) — NOT Postgres `text[]`/`jsonb` — so the same column works on sqlite tests and Postgres runtime.
- **Worktree discipline:** all work in the `feat/rbac-and-roulette` worktree. In a worktree, `Write`/`Edit` with `/data/animeenigma/...` absolute paths edit the BASE tree — always target the worktree root (`…/.claude/worktrees/rbac-and-roulette/…`).
- **Commit co-authors (every commit):**
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Effort metrics, not time:** any plan/CHANGELOG scoring uses UXΔ / CDI / MVQ (never days/hours).

---

### Task 1: `FeatureFlag` domain + `CanAccess` resolver

**Files:**
- Create: `services/policy/go.mod`
- Create: `services/policy/internal/domain/feature_flag.go`
- Test: `services/policy/internal/domain/feature_flag_test.go`
- Modify: `go.work` (add `./services/policy` to the `use (...)` block)

**Interfaces:**
- Produces:
  - `domain.StringList` (`[]string` with `driver.Valuer`/`sql.Scanner`, JSON-text).
  - `domain.FeatureFlag{ Key string; Roles, AllowUsers, DenyUsers StringList; Roulette bool; FailSafe, Label string; UpdatedAt time.Time }`.
  - `func (f FeatureFlag) CanAccess(userID, role string) bool`
  - `func (f FeatureFlag) Audience() Audience` and `domain.Audience{ Roles, AllowUsers, DenyUsers []string }`
  - `domain.Ruleset{ RouletteEnabled bool; Flags map[string]Audience; FailSafe map[string]string; Roulette map[string]bool }`
  - `domain.MineResponse{ Visible, Roulette []string; RouletteEnabled bool }`
  - `func domain.SeedFlags() []FeatureFlag`
  - consts `RoleEveryone/RoleUser/RoleAdmin/RoleGuest`, `RouletteMasterKey = "__roulette__"`.

- [ ] **Step 1: Create the module file** `services/policy/go.mod`

```
module github.com/ILITA-hub/animeenigma/services/policy

go 1.25.0

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/prometheus/client_golang v1.23.2
	github.com/stretchr/testify v1.11.1
	gorm.io/driver/sqlite v1.5.6
	gorm.io/gorm v1.25.12
)
```
> Exact patch versions are workspace-resolved; after Step 3 run `go work sync` from repo root, then `cd services/policy && go mod tidy` to pin `go.sum`. If `go mod tidy` reports a different gorm/sqlite patch already in the workspace, accept it.

- [ ] **Step 2: Add the workspace entry** — edit `go.work`, insert `./services/policy` into the `use (...)` block in alphabetical position (after `./services/player`):

```
	./services/player
	./services/policy
	./services/recs
```

- [ ] **Step 3: Write the failing resolver test** `services/policy/internal/domain/feature_flag_test.go`

```go
package domain

import "testing"

func flag(roles, allow, deny []string) FeatureFlag {
	return FeatureFlag{Key: "f", Roles: roles, AllowUsers: allow, DenyUsers: deny}
}

func TestCanAccess(t *testing.T) {
	cases := []struct {
		name           string
		f              FeatureFlag
		userID, role   string
		want           bool
	}{
		{"admin flag, admin user", flag([]string{RoleAdmin}, nil, nil), "u1", RoleAdmin, true},
		{"admin flag, normal user", flag([]string{RoleAdmin}, nil, nil), "u1", RoleUser, false},
		{"admin flag, allow-listed user wins", flag([]string{RoleAdmin}, []string{"u1"}, nil), "u1", RoleUser, true},
		{"deny beats allow", flag([]string{RoleAdmin}, []string{"u1"}, []string{"u1"}), "u1", RoleAdmin, false},
		{"everyone flag, anonymous", flag([]string{RoleEveryone}, nil, nil), "", "", true},
		{"everyone flag, guest still denied", flag([]string{RoleEveryone}, nil, nil), "g1", RoleGuest, false},
		{"user flag, allow-listed guest still denied", flag([]string{RoleUser}, []string{"g1"}, nil), "g1", RoleGuest, false},
		{"empty audience denies", flag(nil, nil, nil), "u1", RoleUser, false},
		{"user flag, user role", flag([]string{RoleUser}, nil, nil), "u1", RoleUser, true},
		{"user flag, anonymous denied", flag([]string{RoleUser}, nil, nil), "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := c.f.CanAccess(c.userID, c.role); got != c.want {
				t.Fatalf("CanAccess(%q,%q) = %v, want %v", c.userID, c.role, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 4: Run it — expect FAIL** (compile error: `FeatureFlag`/`CanAccess` undefined)

Run: `cd services/policy && go test ./internal/domain/ -run TestCanAccess -v`
Expected: FAIL — undefined: `FeatureFlag`, `CanAccess`, role consts.

- [ ] **Step 5: Implement** `services/policy/internal/domain/feature_flag.go`

```go
// Package domain holds the policy-service feature-flag model + pure resolver.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// Role strings mirror libs/authz roles WITHOUT importing it, so the domain stays
// dependency-free and trivially unit-testable.
const (
	RoleEveryone = "everyone"
	RoleUser     = "user"
	RoleAdmin    = "admin"
	RoleGuest    = "guest"
)

// RouletteMasterKey is the reserved flag key holding the global on/off switch for
// the «Секретная фича» roulette. Double-underscore sentinel so it can never
// collide with a real feature key. Its Roulette field carries the master state.
const RouletteMasterKey = "__roulette__"

// StringList is a []string persisted as JSON text so the same column works on
// both Postgres (runtime) and the sqlite in-memory DB used by repo tests. Using
// type:text (not Postgres text[]/jsonb) keeps it dialect-neutral.
type StringList []string

func (s StringList) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	b, err := json.Marshal([]string(s))
	return string(b), err
}

func (s *StringList) Scan(v any) error {
	if v == nil {
		*s = StringList{}
		return nil
	}
	var b []byte
	switch t := v.(type) {
	case []byte:
		b = t
	case string:
		b = []byte(t)
	default:
		return errors.New("StringList: unsupported Scan type")
	}
	if len(b) == 0 {
		*s = StringList{}
		return nil
	}
	return json.Unmarshal(b, (*[]string)(s))
}

// FeatureFlag is one admin-managed access rule. Key is the PK (string), so no
// UUID hook is needed for sqlite tests.
//
// GORM gotcha: NO `default:` tag on Roulette — GORM omits a zero-value bool that
// carries a default, so Roulette:false would silently store true. Rows are always
// written explicitly via the repo upsert; absence resolves fail-open in service.
type FeatureFlag struct {
	Key        string     `gorm:"primaryKey;size:64" json:"key"`
	Roles      StringList `gorm:"type:text" json:"roles"`
	AllowUsers StringList `gorm:"type:text" json:"allowUsers"`
	DenyUsers  StringList `gorm:"type:text" json:"denyUsers"`
	Roulette   bool       `gorm:"not null" json:"roulette"`
	FailSafe   string     `gorm:"size:16;not null" json:"failSafe"` // "admin" | "everyone"
	Label      string     `gorm:"size:128" json:"label"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}

// Audience is the resolved targeting rule (JSON-facing, no GORM tags).
type Audience struct {
	Roles      []string `json:"roles"`
	AllowUsers []string `json:"allowUsers"`
	DenyUsers  []string `json:"denyUsers"`
}

func (f FeatureFlag) Audience() Audience {
	return Audience{Roles: f.Roles, AllowUsers: f.AllowUsers, DenyUsers: f.DenyUsers}
}

// CanAccess resolves whether (userID, role) may access this flag. Pure and
// order-sensitive: guest-deny → deny-list → allow-list → everyone → role.
func (f FeatureFlag) CanAccess(userID, role string) bool {
	if role == RoleGuest {
		return false
	}
	if userID != "" && contains(f.DenyUsers, userID) {
		return false
	}
	if userID != "" && contains(f.AllowUsers, userID) {
		return true
	}
	if contains(f.Roles, RoleEveryone) {
		return true
	}
	if role != "" && contains(f.Roles, role) {
		return true
	}
	return false
}

func contains(list StringList, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// Ruleset is the compact all-flags snapshot the gateway caches (Phase 2). The
// reserved master key is collapsed into RouletteEnabled and excluded from Flags.
type Ruleset struct {
	RouletteEnabled bool                `json:"rouletteEnabled"`
	Flags           map[string]Audience `json:"flags"`
	FailSafe        map[string]string   `json:"failSafe"`
	Roulette        map[string]bool     `json:"roulette"`
}

// MineResponse is the per-user FE feed (Phase 4 consumer).
type MineResponse struct {
	Visible         []string `json:"visible"`
	Roulette        []string `json:"roulette"`
	RouletteEnabled bool     `json:"rouletteEnabled"`
}

// SeedFlags returns the insert-if-absent defaults so day-one behavior equals the
// pre-RBAC dark-ship state. admin(): admin-only (mirrors *_ADMIN_ONLY=true).
// everyone(): all-authenticated + roulette-eligible (the current SECRET_FEATURES
// roster). gacha is admin-access AND roulette-eligible but seeded roulette-OFF
// (mirrors catalog SecretFeatureDefaultsDisabled). The __roulette__ master is
// seeded separately by the service (defaults ON).
func SeedFlags() []FeatureFlag {
	admin := func(key, label string) FeatureFlag {
		return FeatureFlag{Key: key, Roles: StringList{RoleAdmin}, FailSafe: "admin", Label: label}
	}
	everyone := func(key, label string) FeatureFlag {
		return FeatureFlag{Key: key, Roles: StringList{RoleEveryone}, Roulette: true, FailSafe: "everyone", Label: label}
	}
	return []FeatureFlag{
		admin("fanfic", "Fanfic engine"),
		admin("profile-wall", "Profile showcase wall"),
		{Key: "gacha", Roles: StringList{RoleAdmin}, Roulette: false, FailSafe: "admin", Label: "Gacha «Лудка»"},
		everyone("anidle", "Anidle"),
		everyone("status", "Status page"),
		everyone("themes", "OP/ED themes"),
		everyone("game", "Game rooms"),
		everyone("downloads", "Downloads"),
		everyone("showcase-editor", "Showcase editor"),
		everyone("my-feedback", "My feedback"),
	}
}
```

- [ ] **Step 6: Run it — expect PASS**

Run: `cd services/policy && go work sync >/dev/null 2>&1; go mod tidy && go test ./internal/domain/ -v`
Expected: PASS (all `TestCanAccess` subtests).

- [ ] **Step 7: Commit**

```bash
git add go.work services/policy/go.mod services/policy/go.sum services/policy/internal/domain/
git commit -m "feat(policy): FeatureFlag domain + CanAccess resolver" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: `FeatureFlagRepository` (GORM, sqlite-tested)

**Files:**
- Create: `services/policy/internal/repo/feature_flag.go`
- Test: `services/policy/internal/repo/feature_flag_test.go`

**Interfaces:**
- Consumes: `domain.FeatureFlag`, `domain.StringList`.
- Produces:
  - `repo.NewFeatureFlagRepository(db *gorm.DB) *FeatureFlagRepository`
  - `(*FeatureFlagRepository) GetAll(ctx) ([]domain.FeatureFlag, error)`
  - `(*FeatureFlagRepository) Upsert(ctx, domain.FeatureFlag) error` — full create-or-replace by key.
  - `(*FeatureFlagRepository) SeedIfAbsent(ctx, domain.FeatureFlag) error` — insert only when key absent (idempotent).

- [ ] **Step 1: Write the failing test** `services/policy/internal/repo/feature_flag_test.go`

```go
package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestRepo(t *testing.T) *FeatureFlagRepository {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.FeatureFlag{}))
	return NewFeatureFlagRepository(db)
}

func TestUpsertAndGetAll_roundTripsSlicesAndBool(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()

	in := domain.FeatureFlag{
		Key: "fanfic", Roles: domain.StringList{"admin"},
		AllowUsers: domain.StringList{"u1", "u2"}, Roulette: false, FailSafe: "admin", Label: "Fanfic",
	}
	require.NoError(t, r.Upsert(ctx, in))

	all, err := r.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, "fanfic", all[0].Key)
	require.Equal(t, domain.StringList{"admin"}, all[0].Roles)
	require.Equal(t, domain.StringList{"u1", "u2"}, all[0].AllowUsers)
	require.False(t, all[0].Roulette) // GORM zero-value bool must persist as false
}

func TestUpsert_replacesExisting(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, r.Upsert(ctx, domain.FeatureFlag{Key: "gacha", Roles: domain.StringList{"admin"}, FailSafe: "admin"}))
	require.NoError(t, r.Upsert(ctx, domain.FeatureFlag{Key: "gacha", Roles: domain.StringList{"user"}, Roulette: true, FailSafe: "everyone"}))
	all, err := r.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.Equal(t, domain.StringList{"user"}, all[0].Roles)
	require.True(t, all[0].Roulette)
}

func TestSeedIfAbsent_doesNotClobber(t *testing.T) {
	r := newTestRepo(t)
	ctx := context.Background()
	require.NoError(t, r.Upsert(ctx, domain.FeatureFlag{Key: "gacha", Roulette: true, FailSafe: "admin"}))
	require.NoError(t, r.SeedIfAbsent(ctx, domain.FeatureFlag{Key: "gacha", Roulette: false, FailSafe: "admin"}))
	all, err := r.GetAll(ctx)
	require.NoError(t, err)
	require.Len(t, all, 1)
	require.True(t, all[0].Roulette) // seed must NOT overwrite the admin's toggle
}
```

- [ ] **Step 2: Run it — expect FAIL** (undefined `NewFeatureFlagRepository`)

Run: `cd services/policy && go test ./internal/repo/ -v`
Expected: FAIL — undefined repo symbols.

- [ ] **Step 3: Implement** `services/policy/internal/repo/feature_flag.go`

```go
// Package repo persists policy-service feature flags.
package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type FeatureFlagRepository struct{ db *gorm.DB }

func NewFeatureFlagRepository(db *gorm.DB) *FeatureFlagRepository {
	return &FeatureFlagRepository{db: db}
}

// GetAll returns every flag row (including the reserved __roulette__ master).
func (r *FeatureFlagRepository) GetAll(ctx context.Context) ([]domain.FeatureFlag, error) {
	var flags []domain.FeatureFlag
	if err := r.db.WithContext(ctx).Find(&flags).Error; err != nil {
		return nil, err
	}
	return flags, nil
}

// Upsert writes a flag by key (create or full replace of all columns).
func (r *FeatureFlagRepository) Upsert(ctx context.Context, f domain.FeatureFlag) error {
	f.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&f).Error
}

// SeedIfAbsent inserts a default only when the key has no row (idempotent boot seed).
func (r *FeatureFlagRepository) SeedIfAbsent(ctx context.Context, f domain.FeatureFlag) error {
	f.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&f).Error
}
```

- [ ] **Step 4: Run it — expect PASS**

Run: `cd services/policy && go test ./internal/repo/ -v`
Expected: PASS (3 tests).

- [ ] **Step 5: Commit**

```bash
git add services/policy/internal/repo/ services/policy/go.sum
git commit -m "feat(policy): FeatureFlagRepository (GORM upsert/seed/getall, sqlite-tested)" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: `PolicyService` (ruleset / per-user resolution / seed / admin ops)

**Files:**
- Create: `services/policy/internal/service/policy.go`
- Test: `services/policy/internal/service/policy_test.go`

**Interfaces:**
- Consumes: `repo.FeatureFlagRepository`, `domain.*`, `libs/logger`.
- Produces:
  - `service.NewPolicyService(r *repo.FeatureFlagRepository, log *logger.Logger) *PolicyService`
  - `(*PolicyService) SeedDefaults(ctx) error`
  - `(*PolicyService) Ruleset(ctx) (domain.Ruleset, error)`
  - `(*PolicyService) ResolveForUser(ctx, userID, role string) (domain.MineResponse, error)`
  - `(*PolicyService) ListFlags(ctx) (flags []domain.FeatureFlag, rouletteEnabled bool, err error)` — master excluded from `flags`.
  - `(*PolicyService) SetFlag(ctx, key string, a domain.Audience, roulette bool, failSafe, label string) error`
  - `(*PolicyService) SetRoulette(ctx, enabled bool) error`

- [ ] **Step 1: Write the failing test** `services/policy/internal/service/policy_test.go`

```go
package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newSvc(t *testing.T) *PolicyService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.FeatureFlag{}))
	return NewPolicyService(repo.NewFeatureFlagRepository(db), logger.Default())
}

func TestSeedDefaults_isIdempotent_andMasterOn(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))
	require.NoError(t, s.SeedDefaults(ctx)) // second call must not duplicate/clobber
	rs, err := s.Ruleset(ctx)
	require.NoError(t, err)
	require.True(t, rs.RouletteEnabled)
	require.Contains(t, rs.Flags, "fanfic")
	require.NotContains(t, rs.Flags, domain.RouletteMasterKey) // master collapsed out
	require.Equal(t, "admin", rs.FailSafe["fanfic"])
	require.False(t, rs.Roulette["gacha"]) // seeded roulette-OFF
	require.True(t, rs.Roulette["anidle"])
}

func TestResolveForUser_visibleAndRoulette(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))

	// normal user: sees everyone-flags, not admin-only fanfic/gacha.
	mine, err := s.ResolveForUser(ctx, "u1", domain.RoleUser)
	require.NoError(t, err)
	require.Contains(t, mine.Visible, "anidle")
	require.NotContains(t, mine.Visible, "fanfic")
	require.Contains(t, mine.Roulette, "anidle")
	require.NotContains(t, mine.Roulette, "gacha") // roulette-OFF
	require.True(t, mine.RouletteEnabled)

	// admin: also sees fanfic + gacha.
	adminMine, err := s.ResolveForUser(ctx, "a1", domain.RoleAdmin)
	require.NoError(t, err)
	require.Contains(t, adminMine.Visible, "fanfic")
	require.Contains(t, adminMine.Visible, "gacha")
}

func TestSetFlag_thenAllowUserGetsAccess(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))
	// grant fanfic to a single non-admin user (the "Oronemu first" case).
	require.NoError(t, s.SetFlag(ctx, "fanfic",
		domain.Audience{Roles: []string{"admin"}, AllowUsers: []string{"oronemu"}}, false, "admin", "Fanfic engine"))
	mine, err := s.ResolveForUser(ctx, "oronemu", domain.RoleUser)
	require.NoError(t, err)
	require.Contains(t, mine.Visible, "fanfic")
}

func TestSetFlag_rejectsMasterKeyAndBadFailSafe(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.Error(t, s.SetFlag(ctx, domain.RouletteMasterKey, domain.Audience{}, false, "admin", ""))
	require.Error(t, s.SetFlag(ctx, "x", domain.Audience{}, false, "bogus", ""))
}

func TestSetRoulette_masterOff(t *testing.T) {
	s := newSvc(t)
	ctx := context.Background()
	require.NoError(t, s.SeedDefaults(ctx))
	require.NoError(t, s.SetRoulette(ctx, false))
	rs, err := s.Ruleset(ctx)
	require.NoError(t, err)
	require.False(t, rs.RouletteEnabled)
}
```

- [ ] **Step 2: Run it — expect FAIL** (undefined `NewPolicyService`)

Run: `cd services/policy && go test ./internal/service/ -v`
Expected: FAIL — undefined service symbols.

- [ ] **Step 3: Implement** `services/policy/internal/service/policy.go`

```go
// Package service holds policy-service business logic: seeding defaults,
// resolving the compact ruleset (for the gateway) and the per-user feed (for the
// SPA), and admin writes. Everything defaults fail-open (empty store ⇒ roulette
// on, nothing gated beyond seed defaults).
package service

import (
	"context"
	"sort"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
)

type PolicyService struct {
	repo *repo.FeatureFlagRepository
	log  *logger.Logger
}

func NewPolicyService(r *repo.FeatureFlagRepository, log *logger.Logger) *PolicyService {
	return &PolicyService{repo: r, log: log}
}

// SeedDefaults inserts the dark-ship-equivalent defaults (insert-if-absent) plus
// the __roulette__ master (defaults ON). Idempotent across restarts.
func (s *PolicyService) SeedDefaults(ctx context.Context) error {
	for _, f := range domain.SeedFlags() {
		if err := s.repo.SeedIfAbsent(ctx, f); err != nil {
			return err
		}
	}
	return s.repo.SeedIfAbsent(ctx, domain.FeatureFlag{
		Key: domain.RouletteMasterKey, Roulette: true, FailSafe: "everyone",
	})
}

// Ruleset is the compact snapshot the gateway caches (Phase 2).
func (s *PolicyService) Ruleset(ctx context.Context) (domain.Ruleset, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return domain.Ruleset{}, err
	}
	rs := domain.Ruleset{
		RouletteEnabled: true, // fail-open when master row absent
		Flags:           map[string]domain.Audience{},
		FailSafe:        map[string]string{},
		Roulette:        map[string]bool{},
	}
	for _, f := range rows {
		if f.Key == domain.RouletteMasterKey {
			rs.RouletteEnabled = f.Roulette
			continue
		}
		rs.Flags[f.Key] = f.Audience()
		rs.FailSafe[f.Key] = f.FailSafe
		rs.Roulette[f.Key] = f.Roulette
	}
	return rs, nil
}

// ResolveForUser computes the per-user visible + roulette-eligible key sets.
// Anonymous callers pass userID="" role="" and see only everyone-flags.
func (s *PolicyService) ResolveForUser(ctx context.Context, userID, role string) (domain.MineResponse, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return domain.MineResponse{}, err
	}
	out := domain.MineResponse{RouletteEnabled: true, Visible: []string{}, Roulette: []string{}}
	for _, f := range rows {
		if f.Key == domain.RouletteMasterKey {
			out.RouletteEnabled = f.Roulette
			continue
		}
		if !f.CanAccess(userID, role) {
			continue
		}
		out.Visible = append(out.Visible, f.Key)
		if f.Roulette {
			out.Roulette = append(out.Roulette, f.Key)
		}
	}
	sort.Strings(out.Visible)
	sort.Strings(out.Roulette)
	return out, nil
}

// ListFlags returns the admin view: all non-master flags + the resolved master.
func (s *PolicyService) ListFlags(ctx context.Context) ([]domain.FeatureFlag, bool, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, false, err
	}
	rouletteEnabled := true
	flags := make([]domain.FeatureFlag, 0, len(rows))
	for _, f := range rows {
		if f.Key == domain.RouletteMasterKey {
			rouletteEnabled = f.Roulette
			continue
		}
		flags = append(flags, f)
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].Key < flags[j].Key })
	return flags, rouletteEnabled, nil
}

// SetFlag upserts one feature flag. Rejects the reserved master key and any
// failSafe outside {admin, everyone}.
func (s *PolicyService) SetFlag(ctx context.Context, key string, a domain.Audience, roulette bool, failSafe, label string) error {
	if key == "" || len(key) > 64 {
		return liberrors.InvalidInput("invalid feature key")
	}
	if key == domain.RouletteMasterKey {
		return liberrors.InvalidInput("reserved key")
	}
	if failSafe != "admin" && failSafe != "everyone" {
		return liberrors.InvalidInput("failSafe must be 'admin' or 'everyone'")
	}
	return s.repo.Upsert(ctx, domain.FeatureFlag{
		Key: key, Roles: a.Roles, AllowUsers: a.AllowUsers, DenyUsers: a.DenyUsers,
		Roulette: roulette, FailSafe: failSafe, Label: label,
	})
}

// SetRoulette flips the global master switch.
func (s *PolicyService) SetRoulette(ctx context.Context, enabled bool) error {
	return s.repo.Upsert(ctx, domain.FeatureFlag{
		Key: domain.RouletteMasterKey, Roulette: enabled, FailSafe: "everyone",
	})
}
```
> `liberrors.InvalidInput` is the shared-errors constructor (confirm the exact name against `libs/errors`; the codebase uses `errors.InvalidInput`/`errors.NotFound`). If the constructor differs, use the matching one — do NOT invent a new error package.

- [ ] **Step 4: Run it — expect PASS**

Run: `cd services/policy && go test ./internal/service/ -v`
Expected: PASS (5 tests).

- [ ] **Step 5: Commit**

```bash
git add services/policy/internal/service/
git commit -m "feat(policy): PolicyService — ruleset, per-user resolve, seed, admin ops" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: HTTP handlers + router + middleware + `main.go` (service boots end-to-end)

**Files:**
- Create: `services/policy/internal/config/config.go`
- Create: `services/policy/internal/transport/middleware.go`
- Create: `services/policy/internal/transport/router.go`
- Create: `services/policy/internal/handler/admin_flags.go`
- Create: `services/policy/internal/handler/public_flags.go`
- Create: `services/policy/internal/handler/internal_ruleset.go`
- Test: `services/policy/internal/handler/handlers_test.go`
- Create: `services/policy/cmd/policy-api/main.go`

**Interfaces:**
- Consumes: `service.PolicyService`, `libs/{authz,httputil,logger,metrics}`.
- Produces (HTTP contract):
  - `GET  /health` → `{"status":"ok"}`
  - `GET  /metrics`
  - `GET  /internal/policy/ruleset` → `domain.Ruleset` (Docker-network-only)
  - `GET  /api/policy/features/mine` → `domain.MineResponse` (optional JWT)
  - `GET  /api/admin/policy/flags` → `{"flags":[...],"rouletteEnabled":bool}` (JWT+admin)
  - `PUT  /api/admin/policy/flags/{key}` body `{roles,allowUsers,denyUsers,roulette,failSafe,label}` (JWT+admin)
  - `PUT  /api/admin/policy/roulette` body `{"enabled":bool}` (JWT+admin)

- [ ] **Step 1: Create `config.go`** — mirror `services/recs/internal/config/config.go` exactly, with `SERVER_PORT` default `8098` and drop `CatalogURL` (not needed until Phase 4). Full file:

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server   ServerConfig
	Database database.Config
	Redis    cache.Config
	JWT      authz.JWTConfig
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	return &Config{
		Server: ServerConfig{Host: getEnv("SERVER_HOST", "0.0.0.0"), Port: getEnvInt("SERVER_PORT", 8098)},
		Database: database.Config{
			Host: getEnv("DB_HOST", "localhost"), Port: getEnvInt("DB_PORT", 5432),
			User: getEnv("DB_USER", "postgres"), Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"), SSLMode: getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host: getEnv("REDIS_HOST", "redis"), Port: getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""), DB: getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret: getEnv("JWT_SECRET", ""), Issuer: getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL: getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
	}, nil
}

func getEnv(k, d string) string { if v := os.Getenv(k); v != "" { return v }; return d }
func getEnvInt(k string, d int) int { if v := os.Getenv(k); v != "" { if i, err := strconv.Atoi(v); err == nil { return i } }; return d }
func getEnvDuration(k string, d time.Duration) time.Duration { if v := os.Getenv(k); v != "" { if x, err := time.ParseDuration(v); err == nil { return x } }; return d }
```

- [ ] **Step 2: Create `middleware.go`** — copy `services/recs/internal/transport/middleware.go` verbatim (it already contains `AdminRoleMiddleware` + `OptionalAuthMiddleware`), changing only the package doc references from "recs" to "policy". Then create `router.go`'s `AuthMiddleware` (copied from recs `router.go`). To avoid duplication, place `AuthMiddleware`, `AdminRoleMiddleware`, `OptionalAuthMiddleware` together in `middleware.go`:

```go
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// AuthMiddleware rejects missing/invalid JWTs with 401 and attaches claims.
func AuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	m := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			token := httputil.BearerToken(r)
			if token == "" {
				httputil.Unauthorized(w)
				return
			}
			claims, err := m.ValidateAccessToken(token)
			if err != nil {
				httputil.Unauthorized(w)
				return
			}
			next.ServeHTTP(w, r.WithContext(authz.ContextWithClaims(r.Context(), claims)))
		})
	}
}

// OptionalAuthMiddleware attaches claims IF a valid token is present; never rejects.
func OptionalAuthMiddleware(jwtConfig authz.JWTConfig) func(http.Handler) http.Handler {
	m := authz.NewJWTManager(jwtConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if token := httputil.BearerToken(r); token != "" {
				if claims, err := m.ValidateAccessToken(token); err == nil {
					r = r.WithContext(authz.ContextWithClaims(r.Context(), claims))
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

// AdminRoleMiddleware requires Role==admin (mount AFTER AuthMiddleware).
func AdminRoleMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !authz.IsAdmin(r.Context()) {
			httputil.Forbidden(w)
			return
		}
		next.ServeHTTP(w, r)
	})
}
```

- [ ] **Step 3: Create the three handler files.**

`services/policy/internal/handler/internal_ruleset.go`:

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// InternalRulesetHandler serves the compact ruleset the gateway caches.
// Reachable only from the Docker network (gateway does NOT proxy /internal/*).
type InternalRulesetHandler struct {
	svc *service.PolicyService
	log *logger.Logger
}

func NewInternalRulesetHandler(svc *service.PolicyService, log *logger.Logger) *InternalRulesetHandler {
	return &InternalRulesetHandler{svc: svc, log: log}
}

func (h *InternalRulesetHandler) GetRuleset(w http.ResponseWriter, r *http.Request) {
	rs, err := h.svc.Ruleset(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, rs)
}
```

`services/policy/internal/handler/public_flags.go`:

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// PublicFlagsHandler serves the per-user visibility feed for the SPA. JWT is
// optional — anonymous callers resolve to everyone-flags only. Fail-open.
type PublicFlagsHandler struct {
	svc *service.PolicyService
	log *logger.Logger
}

func NewPublicFlagsHandler(svc *service.PolicyService, log *logger.Logger) *PublicFlagsHandler {
	return &PublicFlagsHandler{svc: svc, log: log}
}

func (h *PublicFlagsHandler) GetMine(w http.ResponseWriter, r *http.Request) {
	userID := authz.UserIDFromContext(r.Context())
	role := string(authz.RoleFromContext(r.Context()))
	mine, err := h.svc.ResolveForUser(r.Context(), userID, role)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, mine)
}
```

`services/policy/internal/handler/admin_flags.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// AdminFlagsHandler is the admin CRUD surface (JWT + admin gated at the router).
type AdminFlagsHandler struct {
	svc *service.PolicyService
	log *logger.Logger
}

func NewAdminFlagsHandler(svc *service.PolicyService, log *logger.Logger) *AdminFlagsHandler {
	return &AdminFlagsHandler{svc: svc, log: log}
}

type listFlagsResponse struct {
	Flags           []domain.FeatureFlag `json:"flags"`
	RouletteEnabled bool                 `json:"rouletteEnabled"`
}

func (h *AdminFlagsHandler) List(w http.ResponseWriter, r *http.Request) {
	flags, rouletteEnabled, err := h.svc.ListFlags(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, listFlagsResponse{Flags: flags, RouletteEnabled: rouletteEnabled})
}

type setFlagRequest struct {
	Roles      []string `json:"roles"`
	AllowUsers []string `json:"allowUsers"`
	DenyUsers  []string `json:"denyUsers"`
	Roulette   bool     `json:"roulette"`
	FailSafe   string   `json:"failSafe"`
	Label      string   `json:"label"`
}

func (h *AdminFlagsHandler) SetFlag(w http.ResponseWriter, r *http.Request) {
	key := chi.URLParam(r, "key")
	var req setFlagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	err := h.svc.SetFlag(r.Context(), key,
		domain.Audience{Roles: req.Roles, AllowUsers: req.AllowUsers, DenyUsers: req.DenyUsers},
		req.Roulette, req.FailSafe, req.Label)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"key": key})
}

type setRouletteRequest struct {
	Enabled bool `json:"enabled"`
}

func (h *AdminFlagsHandler) SetRoulette(w http.ResponseWriter, r *http.Request) {
	var req setRouletteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	if err := h.svc.SetRoulette(r.Context(), req.Enabled); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]bool{"enabled": req.Enabled})
}
```

- [ ] **Step 4: Create `router.go`**

```go
package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/handler"
)

func NewRouter(
	adminH *handler.AdminFlagsHandler,
	publicH *handler.PublicFlagsHandler,
	internalH *handler.InternalRulesetHandler,
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(metricsCollector.Middleware)
	r.Use(httputil.RequestLogger(log))
	r.Use(httputil.Recoverer(log))
	r.Use(httputil.CORS([]string{"*"}))
	r.Use(middleware.RealIP)

	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		httputil.OK(w, map[string]string{"status": "ok"})
	})
	r.Get("/metrics", func(w http.ResponseWriter, r *http.Request) {
		metrics.Handler().ServeHTTP(w, r)
	})

	// Docker-network-only ruleset feed (gateway does NOT proxy /internal/*).
	r.Get("/internal/policy/ruleset", internalH.GetRuleset)

	r.Route("/api", func(r chi.Router) {
		// Per-user visibility feed — JWT OPTIONAL (anonymous ⇒ everyone-flags).
		r.Route("/policy/features", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/mine", publicH.GetMine)
		})
		// Admin CRUD — 401 (AuthMiddleware) then 403 (AdminRoleMiddleware).
		// Defense-in-depth: the gateway applies the same gates.
		r.Route("/admin/policy", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminRoleMiddleware)
			r.Get("/flags", adminH.List)
			r.Put("/flags/{key}", adminH.SetFlag)
			r.Put("/roulette", adminH.SetRoulette)
		})
	})

	return r
}
```

- [ ] **Step 5: Write the failing handler test** `services/policy/internal/handler/handlers_test.go`

```go
package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/transport"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newTestServer(t *testing.T) (http.Handler, authz.JWTConfig) {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&domain.FeatureFlag{}))
	svc := service.NewPolicyService(repo.NewFeatureFlagRepository(db), logger.Default())
	require.NoError(t, svc.SeedDefaults(context.Background()))
	jwtCfg := authz.JWTConfig{Secret: "test-secret", Issuer: "animeenigma"}
	router := transport.NewRouter(
		handler.NewAdminFlagsHandler(svc, logger.Default()),
		handler.NewPublicFlagsHandler(svc, logger.Default()),
		handler.NewInternalRulesetHandler(svc, logger.Default()),
		jwtCfg, logger.Default(), metrics.NewCollector("policy_test"),
	)
	return router, jwtCfg
}

func adminToken(t *testing.T, cfg authz.JWTConfig) string {
	t.Helper()
	m := authz.NewJWTManager(cfg)
	pair, err := m.GenerateTokenPair("a1", "admin", authz.RoleAdmin, "sess1")
	require.NoError(t, err)
	return pair.AccessToken
}

func TestInternalRuleset_ok(t *testing.T) {
	router, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/internal/policy/ruleset", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), "fanfic")
}

func TestMine_anonymousSeesEveryoneOnly(t *testing.T) {
	router, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/policy/features/mine", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	require.Contains(t, body, "anidle")
	require.NotContains(t, body, "fanfic") // admin-only
}

func TestAdminFlags_requiresAdmin(t *testing.T) {
	router, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/admin/policy/flags", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnauthorized, rec.Code) // no token
}

func TestAdminSetFlag_thenMineReflects(t *testing.T) {
	router, cfg := newTestServer(t)
	tok := adminToken(t, cfg)

	body := `{"roles":["admin"],"allowUsers":["oronemu"],"denyUsers":[],"roulette":false,"failSafe":"admin","label":"Fanfic"}`
	req := httptest.NewRequest(http.MethodPut, "/api/admin/policy/flags/fanfic", strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer "+tok)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)

	// oronemu (a normal user) now resolves fanfic as visible via allowUsers.
	m := authz.NewJWTManager(cfg)
	pair, err := m.GenerateTokenPair("oronemu", "oronemu", authz.RoleUser, "s2")
	require.NoError(t, err)
	req2 := httptest.NewRequest(http.MethodGet, "/api/policy/features/mine", nil)
	req2.Header.Set("Authorization", "Bearer "+pair.AccessToken)
	rec2 := httptest.NewRecorder()
	router.ServeHTTP(rec2, req2)
	require.Equal(t, http.StatusOK, rec2.Code)
	var mine domain.MineResponse
	require.NoError(t, json.Unmarshal(rec2.Body.Bytes(), &mine))
	require.Contains(t, mine.Visible, "fanfic")
}
```
> `httputil.OK` wraps payloads in the standard `{success,data}` envelope used repo-wide. `json.Unmarshal` into `MineResponse` works only if `/mine` returns the raw object — verify `httputil.OK`'s shape; if it wraps in `{"data":...}`, unmarshal into a `struct{ Data domain.MineResponse `json:"data"` }` instead. Adjust the two decode-based assertions to the actual envelope; the `Contains`-on-body assertions are envelope-agnostic and are the primary gate.

- [ ] **Step 6: Run it — expect FAIL then implement `main.go`, then PASS**

Run: `cd services/policy && go test ./internal/handler/ -v`
Expected: FAIL (compile — router/handlers not all wired) until Step 7 files exist; once Tasks-4 files compile, PASS.

- [ ] **Step 7: Create `cmd/policy-api/main.go`** — mirror `services/recs/cmd/recs-api/main.go`'s boot skeleton, trimmed (no crons, no FK backfill, AutoMigrate only `FeatureFlag`, call `SeedDefaults` after migrate):

```go
// Package main is the policy-service entrypoint (port 8098).
//
// policy-service — runtime feature-access authority (RBAC + roulette). Owns the
// feature_flags table; resolves (userID,role)->access for the gateway (ruleset
// feed) and the SPA (/features/mine). Enforcement middleware lives in the
// gateway (Phase 2). Boot: logger -> config -> db -> redis -> AutoMigrate ->
// SeedDefaults -> router -> serve.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/config"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "policy")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()

	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}
	if sqlDB, err := db.DB.DB(); err == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}

	if err := db.AutoMigrate(&domain.FeatureFlag{}); err != nil {
		log.Fatalw("failed to migrate database", "error", err)
	}

	// Redis is reserved for the Phase-2 ruleset invalidate pub/sub; connect
	// non-fatally so policy-service boots even if Redis is briefly down.
	if redisCache, err := cache.New(cfg.Redis); err != nil {
		log.Warnw("redis unavailable at boot (ruleset pub/sub disabled)", "error", err)
	} else {
		defer redisCache.Close()
	}

	policySvc := service.NewPolicyService(repo.NewFeatureFlagRepository(db.DB), log)
	if err := policySvc.SeedDefaults(context.Background()); err != nil {
		log.Fatalw("failed to seed default flags", "error", err)
	}

	adminH := handler.NewAdminFlagsHandler(policySvc, log)
	publicH := handler.NewPublicFlagsHandler(policySvc, log)
	internalH := handler.NewInternalRulesetHandler(policySvc, log)

	router := transport.NewRouter(adminH, publicH, internalH, cfg.JWT, log, metrics.NewCollector("policy"))

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("policy")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting policy service", "address", cfg.Server.Address(), "db_name", cfg.Database.Database)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Info("shutting down policy service...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	log.Info("policy service stopped")
}
```

- [ ] **Step 8: Build + test the whole module**

Run: `cd services/policy && go build ./... && go test ./... -v`
Expected: build OK; all domain/repo/service/handler tests PASS.

- [ ] **Step 9: Commit**

```bash
git add services/policy/internal/config services/policy/internal/transport services/policy/internal/handler services/policy/cmd services/policy/go.sum
git commit -m "feat(policy): HTTP surfaces (admin CRUD, /features/mine, /internal/ruleset) + main" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Infra — Dockerfile, go.work Dockerfile ripple, compose, Makefile, Prometheus

**Files:**
- Create: `services/policy/Dockerfile`
- Modify: every `services/*/Dockerfile` (add the policy `COPY` line)
- Modify: `docker/docker-compose.yml`, `docker/docker-compose.prod.yml`
- Modify: `Makefile` (SERVICES list + health target)
- Modify: `docker/prometheus/prometheus.yml`

**Interfaces:** none (infra). Deliverable: `make redeploy-policy` builds + boots; `curl localhost:8098/health` → ok.

- [ ] **Step 1: Create `services/policy/Dockerfile`** — copy `services/recs/Dockerfile` verbatim, then (a) add `COPY services/policy/go.mod services/policy/go.sum* ./services/policy/` alongside the other service COPYs, (b) replace every `recs` token with `policy` in the build/copy/output lines, (c) `EXPOSE 8098`, (d) build `./cmd/policy-api` → `/policy-api`. Final CMD `["./policy-api"]`.

- [ ] **Step 2: Ripple the `COPY` into every other service Dockerfile** — the go.work now lists `./services/policy`, so each service's `go mod download` needs policy's go.mod present:

```bash
cd /data/animeenigma/.claude/worktrees/rbac-and-roulette
for df in services/*/Dockerfile; do
  grep -q 'services/policy/go.mod' "$df" && continue
  sed -i '/COPY services\/fanfic\/go.mod/a COPY services/policy/go.mod services/policy/go.sum* ./services/policy/' "$df"
done
git diff --stat -- 'services/*/Dockerfile'
```
Expected: ~17 Dockerfiles changed (each gains one `COPY` line after the fanfic line). `services/stealth-scraper` (no Go Dockerfile of this shape) and `services/maintenance` (no Dockerfile) are untouched. Verify none is missing the line: `grep -L 'services/policy/go.mod' services/*/Dockerfile` should print only Dockerfiles that legitimately lack the fanfic anchor (investigate any hit).

- [ ] **Step 3: Add the compose service block** to `docker/docker-compose.yml` (after the `recs:` block), mirroring recs with port 8098 and no `CATALOG_URL`:

```yaml
  policy:
    logging: *default-logging
    build:
      context: ..
      dockerfile: services/policy/Dockerfile
    container_name: animeenigma-policy
    mem_limit: 256m
    restart: unless-stopped
    environment:
      SERVER_PORT: 8098
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: ${DB_USER:-postgres}
      DB_PASSWORD: ${DB_PASSWORD:-postgres}
      DB_NAME: ${DB_NAME:-animeenigma}
      JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
      REDIS_HOST: redis
      TRACING_ENABLED: "true"
    ports:
      - "127.0.0.1:8098:8098"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "-O", "/dev/null", "http://localhost:8098/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```
Then add the analogous block to `docker/docker-compose.prod.yml`, matching that file's existing per-service style (image/build + env). Diff the recs block in prod first and mirror it.

- [ ] **Step 4: Makefile** — add `policy` to the `SERVICES :=` list (line ~9) and a health line near the other health checks:

```make
	@curl -sf http://localhost:8098/health > /dev/null && echo "✓ policy:8098" || echo "✗ policy:8098"
```

- [ ] **Step 5: Prometheus scrape target** — add to `docker/prometheus/prometheus.yml` a job mirroring recs:
```yaml
  - job_name: policy
    static_configs:
      - targets: ['policy:8098']
```
> Prometheus does NOT hot-reload a bind-mounted config — after deploy, `docker compose up -d --force-recreate prometheus` (a plain restart won't pick it up).

- [ ] **Step 6: Build + boot locally**

Run: `make redeploy-policy && sleep 5 && curl -s localhost:8098/health && echo && curl -s localhost:8098/internal/policy/ruleset | head -c 400`
Expected: health `{"success":true,...ok...}`; ruleset JSON containing `fanfic`,`anidle`.

- [ ] **Step 7: Commit**

```bash
git add services/policy/Dockerfile services/*/Dockerfile docker/docker-compose.yml docker/docker-compose.prod.yml docker/prometheus/prometheus.yml Makefile
git commit -m "build(policy): Dockerfile + go.work ripple + compose + Makefile + prometheus" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 6: Gateway routing (proxy-only; enforcement is Phase 2)

**Files:**
- Modify: `services/gateway/internal/config/config.go` (add `PolicyService` URL)
- Modify: `services/gateway/internal/service/proxy.go` (add `case "policy"`)
- Modify: `services/gateway/internal/handler/proxy.go` (add `ProxyToPolicy`)
- Modify: `services/gateway/internal/transport/router.go` (mount policy routes + `/admin/policy` SPA fall-through)
- Test: `services/gateway/internal/transport/router_policy_test.go`

**Interfaces:**
- Consumes: policy-service HTTP contract from Task 4.
- Produces gateway routes: `/api/policy/features/mine` (optional JWT → policy), `/api/admin/policy/*` (JWT+admin → policy), `/admin/policy` + `/admin/policy/*` (→ web SPA).

- [ ] **Step 1: config** — in `ServiceURLs` add `PolicyService string`, and in the `Services:` literal add `PolicyService: getEnv("POLICY_SERVICE_URL", "http://policy-service:8098"),`. (Match the `RecsService` lines exactly.)

- [ ] **Step 2: proxy resolver** — in `getServiceURL`'s switch (`services/gateway/internal/service/proxy.go`) add:
```go
	case "policy":
		return s.serviceURLs.PolicyService, nil
```

- [ ] **Step 3: proxy handler** — in `services/gateway/internal/handler/proxy.go` add a `ProxyToPolicy` method mirroring `ProxyToRecs` (same body, `"policy"` service name). Locate `ProxyToRecs` and copy its exact shape.

- [ ] **Step 4: Write the failing gateway test** `services/gateway/internal/transport/router_policy_test.go` — mirror `router_spotlight_test.go`/`router_worker_test.go` structure: stand up the router with a stub upstream, assert `/api/policy/features/mine` reaches the policy backend and `/api/admin/policy/flags` without a token returns 401. (Copy an existing router test's harness verbatim; swap the path + backend name.)

Run: `cd services/gateway && go test ./internal/transport/ -run Policy -v`
Expected: FAIL (routes not mounted).

- [ ] **Step 5: Mount the routes** in `services/gateway/internal/transport/router.go` — inside the `/api` route group, mirroring the recs mounts (`/users/recs` optional-JWT + `/admin/recs/*` JWT+admin):

```go
		// policy-service (RBAC + roulette). Per-user visibility feed is
		// JWT-OPTIONAL; admin CRUD is JWT + admin (defense-in-depth — policy
		// re-applies both gates). Enforcement of OTHER features via the ruleset
		// (FeatureGate) lands in Phase 2.
		r.Group(func(r chi.Router) {
			r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.HandleFunc("/policy/features/mine", proxyHandler.ProxyToPolicy)
		})
		r.Group(func(r chi.Router) {
			r.Use(JWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
			r.Use(AdminRoleMiddleware)
			r.HandleFunc("/admin/policy/*", proxyHandler.ProxyToPolicy)
		})
```
> Use the SAME middleware constructor names the recs mounts use in this file (`OptionalJWTValidationMiddleware`, `JWTValidationMiddleware`, `AdminRoleMiddleware`) — grep the recs block (`/users/recs`, `/admin/recs/*`) and copy its exact middleware calls; names/signatures differ from the per-service router.

- [ ] **Step 6: SPA fall-through** — add `/admin/policy` to the web fall-through group alongside `/admin/recs` (so `AdminPolicy.vue`, Phase 3, renders):
```go
		r.HandleFunc("/admin/policy", proxyHandler.ProxyToWeb)
		r.HandleFunc("/admin/policy/*", proxyHandler.ProxyToWeb)
```
Place it in the same block as the existing `/admin/recs` fall-through (search `ProxyToWeb` + `/recs`).

- [ ] **Step 7: Run gateway tests — expect PASS**

Run: `cd services/gateway && go test ./... `
Expected: PASS (new `router_policy_test.go` + no regressions).

- [ ] **Step 8: Redeploy gateway + smoke**

Run:
```bash
make redeploy-gateway && sleep 4
curl -s localhost:8000/api/policy/features/mine | head -c 300          # anonymous: everyone-flags
curl -s -o /dev/null -w '%{http_code}\n' localhost:8000/api/admin/policy/flags   # -> 401 (no token)
```
Expected: `mine` returns JSON with `anidle`; admin route → `401`.

- [ ] **Step 9: Commit**

```bash
git add services/gateway/
git commit -m "feat(gateway): route /api/policy/* + /admin/policy SPA fall-through (proxy-only)" \
  -m "Co-Authored-By: Claude Code <noreply@anthropic.com>" \
  -m "Co-Authored-By: 0neymik0 <0neymik0@gmail.com>" \
  -m "Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase-1 exit criteria

- `policy-service` boots on 8098; `feature_flags` table auto-migrated + seeded to dark-ship-equivalent defaults.
- `GET /internal/policy/ruleset` returns the compact snapshot; `GET /api/policy/features/mine` resolves per-user (anonymous ⇒ everyone-flags) through the gateway; admin CRUD gated 401/403.
- `go test ./...` green in `services/policy` and `services/gateway`; `make health` shows `✓ policy:8098`.
- No behavior change for end users (nothing consumes the feeds yet — that's P2/P4).

## Follow-on plans (separate files, written after P1 lands against its real interfaces)

- **P2 — gateway enforcement:** ruleset cache + `POLICY_RULESET_REFRESH` poll + Redis pub/sub invalidate; `FeatureGate(key)` middleware; cut `fanfic`/`gacha`/`profile-wall` route groups from `*AdminOnly` bools to `FeatureGate`; cold-start `failSafe`, fail-static; remove the env bools.
- **P3 — admin Features tab:** `/admin/policy` `AdminPolicy.vue` Features tab (audience editor: role checkboxes + allow/deny user chips + roulette toggle + master switch); absorb `AdminSecretFeatures.vue`; i18n en/ru/ja; `/frontend-verify`.
- **P4 — FE visibility cutover + Providers facade tab:** `useFeatureVisible(key)` replaces `fanficGate.ts`/`gachaGate.ts`/`profileWallGate.ts` (fail-open to build-time defaults); roulette reads `/features/mine`; retire catalog `SecretFeatureFlag`; add the Providers facade (policy-service `GET/PUT /api/admin/policy/providers` → catalog admin endpoint) + Providers tab. **Deferred TODO (spec §11):** move provider-policy DATA out of catalog into policy-service's own DB — file as ISS-NNN.

## Self-review

- **Spec coverage (Phase 1 scope):** `FeatureFlag` model + resolver (Task 1) ✓; store (Task 2) ✓; ruleset + per-user + seed + admin ops (Task 3) ✓; three HTTP surfaces (Task 4) ✓; day-one seeding (Task 3/4) ✓; new service infra + go.work ripple (Task 5) ✓; gateway routing (Task 6) ✓. Enforcement/admin-UI/FE-cutover/provider-facade are explicitly P2–P4.
- **Placeholder scan:** two flagged verification notes (httputil envelope shape in the Task-4 test; `liberrors` constructor name) are *verify-against-code* instructions with a concrete fallback, not unresolved work. Gateway middleware/handler copies (Tasks 4/6) name the exact source to mirror. No "TBD"/"handle edge cases".
- **Type consistency:** `CanAccess(userID, role string)`, `Audience{Roles,AllowUsers,DenyUsers}`, `Ruleset{RouletteEnabled,Flags,FailSafe,Roulette}`, `MineResponse{Visible,Roulette,RouletteEnabled}`, `SetFlag(ctx,key,Audience,roulette,failSafe,label)` are used identically across domain → service → handler → tests. `StringList` is the single slice type end-to-end.

## Metrics

- **UXΔ** = `+1 (Ambiguous)` for Phase 1 alone (no user-visible change; infra enabling later phases). Full feature UXΔ = `+2 (Better)`.
- **CDI** = `0.04 * 21` (new service + go.work ripple across ~17 Dockerfiles + gateway wiring; contained, additive).
- **MVQ** = `Griffin 88%/82%`.
