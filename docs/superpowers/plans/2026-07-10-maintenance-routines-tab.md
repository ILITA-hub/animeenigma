# Maintenance Routines Tab — Implementation Plan (P1 backend + P2 FE)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a third "Maintenance" tab to `/admin/policy` that lists background
maintenance routines, lets an admin pause/resume each and tune safe knobs, and shows
each routine's last-run status — backed by a new `maintenance_routines` table in
policy-service.

**Architecture:** Pull-config. policy-service (:8098) owns a `maintenance_routines`
table (admin intent + status). The Maintenance tab reads/writes it via the **existing**
`/api/admin/policy/*` gateway proxy group (routes nested under `/api/admin/policy/maintenance/*`
→ **zero gateway change**). Internal `/internal/maintenance/*` endpoints (gate + status
write-back) are added for the P3 routine wiring but are **not consumed in this plan** —
after P1+P2 the toggles persist and render but enforcement lands in the separate P3 plan.
Day-one seed = today's real defaults, so first boot changes nothing.

**Tech Stack:** Go 1.x + GORM + chi (policy-service, mirrors `feature_flag.*`); Vue 3 +
TypeScript + Pinia-free composables + vue-i18n + shadcn-vue primitives (frontend/web).

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-07-10-maintenance-routines-tab-design.md`.
- **GORM false-bool gotcha:** never put `default:` on the `Enabled` bool; seed writes
  it explicitly; updates use a **column-scoped `Updates(map[string]any{...})`** so
  `false` is persisted (a zero-value struct field would be skipped).
- **Fail-open:** the gate endpoint is only *read* in P3; unknown id ⇒ 404, and P3
  callers treat any non-200 as `enabled=true`. Nothing in this plan may pause a routine.
- **Seed parity:** all routines seed `enabled=true` with the real current knob values.
- **No gateway change:** admin routes MUST nest under `/api/admin/policy/maintenance/*`
  (reuses the `/admin/policy/*` proxy group + its JWT+admin gates). Do NOT add a new
  `/api/admin/maintenance/*` group — it would fall through to catalog's `/api/admin/*`
  catch-all.
- **Go tests:** handwritten fakes / sqlite in-memory; **no testify/mock** (house style).
- **DS lint (build-enforced):** no off-palette colors; bind status variants to semantic
  Badge variants; no native `<select>` (use `SegmentedControl`); only `font-medium`/
  `font-semibold`. Run `bash frontend/web/scripts/design-system-lint.sh` — ERRORS>0 fails.
- **i18n parity gate:** every new key MUST exist in `en.json`, `ru.json`, AND `ja.json`
  (the parity spec fails on any mismatch).
- **Commit co-authors** on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- **Worktree:** all work in `/data/ae-maintenance-panel` (branch `feat/maintenance-routines-tab`).
  Use worktree-relative paths; never edit `/data/animeenigma` directly.

---

## Phase 1 — policy-service backend

### Task 1: Domain model + seed (`domain/maintenance.go`)

**Files:**
- Create: `services/policy/internal/domain/maintenance.go`
- Test: `services/policy/internal/domain/maintenance_test.go`

**Interfaces:**
- Produces: type `SettingsJSON []byte` (GORM `Valuer`/`Scanner` + JSON marshal), struct
  `MaintenanceRoutine{ID string; Enabled bool; Settings SettingsJSON; LastRunAt *time.Time;
  LastOK *bool; LastSummary string; NextRunAt *time.Time; UpdatedAt time.Time}`, and
  `func SeedRoutines() []MaintenanceRoutine`.

- [ ] **Step 1: Write the failing test**

```go
// services/policy/internal/domain/maintenance_test.go
package domain

import (
	"encoding/json"
	"testing"
)

func TestSeedRoutines_ParityDefaults(t *testing.T) {
	rows := SeedRoutines()
	if len(rows) != 9 {
		t.Fatalf("want 9 seeded routines, got %d", len(rows))
	}
	byID := map[string]MaintenanceRoutine{}
	for _, r := range rows {
		if !r.Enabled {
			t.Errorf("routine %q seeded disabled; day-one parity requires enabled=true", r.ID)
		}
		if !json.Valid(r.Settings.raw()) {
			t.Errorf("routine %q settings is not valid JSON: %s", r.ID, string(r.Settings))
		}
		byID[r.ID] = r
	}
	for _, id := range []string{
		"maintenance_bot", "provider_recovery", "git_autosync", "disk_prune",
		"build_cache_prune", "subtitle_probe", "shikimori_sync", "playability_canary",
		"provider_self_heal",
	} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing seeded routine %q", id)
		}
	}
	var bot map[string]any
	if err := json.Unmarshal(byID["maintenance_bot"].Settings.raw(), &bot); err != nil {
		t.Fatalf("bot settings unmarshal: %v", err)
	}
	if bot["auto_apply_max_risk"] != "medium" {
		t.Errorf("bot auto_apply_max_risk = %v; want medium (current behavior)", bot["auto_apply_max_risk"])
	}
}

func TestSettingsJSON_ScanValueRoundTrip(t *testing.T) {
	var s SettingsJSON
	if err := s.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if string(s) != "{}" {
		t.Errorf("nil scan = %q; want {}", string(s))
	}
	if err := s.Scan(`{"model":"opus"}`); err != nil {
		t.Fatalf("scan string: %v", err)
	}
	v, err := s.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	if v.(string) != `{"model":"opus"}` {
		t.Errorf("value = %v; want {\"model\":\"opus\"}", v)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/policy && go test ./internal/domain/ -run TestSeedRoutines_ParityDefaults -v`
Expected: FAIL — `SeedRoutines`/`MaintenanceRoutine`/`SettingsJSON` undefined.

- [ ] **Step 3: Write the implementation**

```go
// services/policy/internal/domain/maintenance.go
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"time"
)

// SettingsJSON is a free-form knob object persisted as JSON text (dialect-neutral,
// like StringList — works on Postgres runtime AND the sqlite in-memory test DB).
// Empty ⇒ "{}". It round-trips verbatim: the service validates it is a JSON object,
// the FE owns its shape via the descriptor registry.
type SettingsJSON []byte

func (s SettingsJSON) raw() []byte {
	if len(s) == 0 {
		return []byte("{}")
	}
	return s
}

func (s SettingsJSON) Value() (driver.Value, error) { return string(s.raw()), nil }

func (s *SettingsJSON) Scan(v any) error {
	switch t := v.(type) {
	case nil:
		*s = SettingsJSON("{}")
	case []byte:
		*s = SettingsJSON(append([]byte(nil), t...))
	case string:
		*s = SettingsJSON(t)
	default:
		return errors.New("SettingsJSON: unsupported Scan type")
	}
	if len(*s) == 0 {
		*s = SettingsJSON("{}")
	}
	return nil
}

// MarshalJSON emits the raw object (so the wire shows `"settings":{...}`, not a
// base64 []byte). UnmarshalJSON stores the incoming object bytes verbatim.
func (s SettingsJSON) MarshalJSON() ([]byte, error)  { return s.raw(), nil }
func (s *SettingsJSON) UnmarshalJSON(b []byte) error { *s = SettingsJSON(append([]byte(nil), b...)); return nil }

// MaintenanceRoutine is one admin-controllable background routine's intent+status.
//
// GORM gotcha: NO `default:` tag on Enabled — GORM omits a zero-value bool carrying
// a default, so a future false would silently store true. Seed writes it explicitly;
// updates go through a column-scoped Updates map (see repo).
type MaintenanceRoutine struct {
	ID          string       `gorm:"primaryKey;size:64" json:"id"`
	Enabled     bool         `gorm:"not null" json:"enabled"`
	Settings    SettingsJSON `gorm:"type:text" json:"settings"`
	LastRunAt   *time.Time   `json:"lastRunAt"`
	LastOK      *bool        `json:"lastOk"`
	LastSummary string       `gorm:"size:512" json:"lastSummary"`
	NextRunAt   *time.Time   `json:"nextRunAt"`
	UpdatedAt   time.Time    `json:"updatedAt"`
}

// SeedRoutines returns insert-if-absent defaults — all enabled, knob values equal to
// today's real behavior, so first boot changes nothing. Slice order = admin-list
// display order (host routines first, then in-cluster).
func SeedRoutines() []MaintenanceRoutine {
	r := func(id, settings string) MaintenanceRoutine {
		return MaintenanceRoutine{ID: id, Enabled: true, Settings: SettingsJSON(settings)}
	}
	return []MaintenanceRoutine{
		r("maintenance_bot", `{"auto_apply_max_risk":"medium","suppressed_alerts":[]}`),
		r("provider_recovery", `{"model":"sonnet"}`),
		r("git_autosync", `{}`),
		r("disk_prune", `{"high_water_pct":80}`),
		r("build_cache_prune", `{}`),
		r("subtitle_probe", `{}`),
		r("shikimori_sync", `{}`),
		r("playability_canary", `{}`),
		r("provider_self_heal", `{"demote_after":"24h","probe_every":"6h"}`),
	}
}

// Compile-time proof SettingsJSON satisfies the GORM interfaces.
var _ driver.Valuer = SettingsJSON(nil)
var _ json.Marshaler = SettingsJSON(nil)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/policy && go test ./internal/domain/ -v`
Expected: PASS (both new tests + existing feature_flag domain tests).

- [ ] **Step 5: Commit**

```bash
git -C /data/ae-maintenance-panel add services/policy/internal/domain/maintenance.go services/policy/internal/domain/maintenance_test.go
git -C /data/ae-maintenance-panel commit -m "feat(policy): maintenance routine domain model + seed parity

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: Repository (`repo/maintenance.go`)

**Files:**
- Create: `services/policy/internal/repo/maintenance.go`
- Test: `services/policy/internal/repo/maintenance_test.go`

**Interfaces:**
- Consumes: `domain.MaintenanceRoutine`, `domain.SettingsJSON`, `domain.SeedRoutines()`.
- Produces: `type MaintenanceRepository`; `NewMaintenanceRepository(db *gorm.DB) *MaintenanceRepository`;
  methods `GetAll(ctx) ([]MaintenanceRoutine, error)`, `GetByID(ctx, id) (*MaintenanceRoutine, error)`
  (returns `gorm.ErrRecordNotFound` when absent), `SeedIfAbsent(ctx, m) error`,
  `SetIntent(ctx, id, enabled bool, settings SettingsJSON) error`,
  `SetStatus(ctx, id string, ok bool, summary string, next *time.Time) error`.

- [ ] **Step 1: Write the failing test** (mirror the sqlite setup used by `feature_flag` repo tests)

```go
// services/policy/internal/repo/maintenance_test.go
package repo

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newMaintDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.MaintenanceRoutine{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return db
}

func TestMaintenanceRepo_SeedGetSetIntent(t *testing.T) {
	db := newMaintDB(t)
	r := NewMaintenanceRepository(db)
	ctx := context.Background()

	for _, m := range domain.SeedRoutines() {
		if err := r.SeedIfAbsent(ctx, m); err != nil {
			t.Fatalf("seed %s: %v", m.ID, err)
		}
	}
	// Idempotent: second seed must not overwrite an admin edit.
	if err := r.SetIntent(ctx, "provider_recovery", false, domain.SettingsJSON(`{"model":"opus"}`)); err != nil {
		t.Fatalf("set intent: %v", err)
	}
	for _, m := range domain.SeedRoutines() {
		_ = r.SeedIfAbsent(ctx, m) // no-op on conflict
	}
	got, err := r.GetByID(ctx, "provider_recovery")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Enabled {
		t.Errorf("enabled = true; want false (admin paused, survived re-seed)")
	}
	if string(got.Settings) != `{"model":"opus"}` {
		t.Errorf("settings = %s; want {\"model\":\"opus\"}", string(got.Settings))
	}

	all, err := r.GetAll(ctx)
	if err != nil || len(all) != 9 {
		t.Fatalf("getall len = %d err = %v; want 9", len(all), err)
	}
}

func TestMaintenanceRepo_SetStatus(t *testing.T) {
	db := newMaintDB(t)
	r := NewMaintenanceRepository(db)
	ctx := context.Background()
	_ = r.SeedIfAbsent(ctx, domain.MaintenanceRoutine{ID: "git_autosync", Enabled: true, Settings: domain.SettingsJSON("{}")})

	if err := r.SetStatus(ctx, "git_autosync", true, "in-sync · HEAD abc123", nil); err != nil {
		t.Fatalf("set status: %v", err)
	}
	got, _ := r.GetByID(ctx, "git_autosync")
	if got.LastOK == nil || !*got.LastOK {
		t.Errorf("lastOk = %v; want true", got.LastOK)
	}
	if got.LastRunAt == nil {
		t.Errorf("lastRunAt not stamped")
	}
	if got.LastSummary != "in-sync · HEAD abc123" {
		t.Errorf("summary = %q", got.LastSummary)
	}
	if !got.Enabled { // status write must not touch intent
		t.Errorf("enabled clobbered by status write")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/policy && go test ./internal/repo/ -run TestMaintenanceRepo -v`
Expected: FAIL — `NewMaintenanceRepository` undefined.

- [ ] **Step 3: Write the implementation**

```go
// services/policy/internal/repo/maintenance.go
package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MaintenanceRepository struct{ db *gorm.DB }

func NewMaintenanceRepository(db *gorm.DB) *MaintenanceRepository {
	return &MaintenanceRepository{db: db}
}

func (r *MaintenanceRepository) GetAll(ctx context.Context) ([]domain.MaintenanceRoutine, error) {
	var rows []domain.MaintenanceRoutine
	if err := r.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *MaintenanceRepository) GetByID(ctx context.Context, id string) (*domain.MaintenanceRoutine, error) {
	var row domain.MaintenanceRoutine
	if err := r.db.WithContext(ctx).First(&row, "id = ?", id).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// SeedIfAbsent inserts a default only when the id has no row (idempotent boot seed).
func (r *MaintenanceRepository) SeedIfAbsent(ctx context.Context, m domain.MaintenanceRoutine) error {
	m.UpdatedAt = time.Now()
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&m).Error
}

// SetIntent writes enabled+settings via a column-scoped Updates map so the
// zero-value `false` is always persisted (a struct Save would skip it) and the
// status columns are left untouched.
func (r *MaintenanceRepository) SetIntent(ctx context.Context, id string, enabled bool, settings domain.SettingsJSON) error {
	return r.db.WithContext(ctx).Model(&domain.MaintenanceRoutine{}).
		Where("id = ?", id).
		Updates(map[string]any{"enabled": enabled, "settings": settings, "updated_at": time.Now()}).Error
}

// SetStatus stamps last-run fields only; never touches enabled/settings.
func (r *MaintenanceRepository) SetStatus(ctx context.Context, id string, ok bool, summary string, next *time.Time) error {
	now := time.Now()
	return r.db.WithContext(ctx).Model(&domain.MaintenanceRoutine{}).
		Where("id = ?", id).
		Updates(map[string]any{
			"last_run_at": now, "last_ok": ok, "last_summary": summary,
			"next_run_at": next, "updated_at": now,
		}).Error
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/policy && go test ./internal/repo/ -v`
Expected: PASS. (Driver `gorm.io/driver/sqlite` + `sqlite.Open(":memory:")` — confirmed
the same helper `internal/repo/feature_flag_test.go` already uses.)

- [ ] **Step 5: Commit**

```bash
git -C /data/ae-maintenance-panel add services/policy/internal/repo/maintenance.go services/policy/internal/repo/maintenance_test.go
git -C /data/ae-maintenance-panel commit -m "feat(policy): maintenance routine repository (intent + status)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: Service (`service/maintenance.go`)

**Files:**
- Create: `services/policy/internal/service/maintenance.go`
- Test: `services/policy/internal/service/maintenance_test.go`

**Interfaces:**
- Consumes: `repo.MaintenanceRepository`, `domain.*`, `liberrors` (`NotFound`, `InvalidInput`),
  `gorm.ErrRecordNotFound`.
- Produces: `type MaintenanceService`; `NewMaintenanceService(r *repo.MaintenanceRepository, log *logger.Logger) *MaintenanceService`;
  `SeedDefaults(ctx) error`, `List(ctx) ([]domain.MaintenanceRoutine, error)`,
  `Gate(ctx, id) (*domain.MaintenanceRoutine, error)`,
  `SetRoutine(ctx, id string, enabled bool, settings domain.SettingsJSON) error`,
  `SetStatus(ctx, id string, ok bool, summary string, next *time.Time) error`.

- [ ] **Step 1: Write the failing test**

```go
// services/policy/internal/service/maintenance_test.go
package service

import (
	"context"
	"errors"
	"testing"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// isNotFound reports whether err is a libs/errors NotFound AppError (there is no
// IsNotFound helper — assert on the code).
func isNotFound(err error) bool {
	var ae *liberrors.AppError
	return errors.As(err, &ae) && ae.Code == liberrors.CodeNotFound
}

func newMaintSvc(t *testing.T) *MaintenanceService {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := db.AutoMigrate(&domain.MaintenanceRoutine{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewMaintenanceService(repo.NewMaintenanceRepository(db), logger.Default())
}

func TestMaintenanceService_SeedListSet(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	if err := svc.SeedDefaults(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
	rows, err := svc.List(ctx)
	if err != nil || len(rows) != 9 {
		t.Fatalf("list len=%d err=%v", len(rows), err)
	}
	// list is sorted by id
	if rows[0].ID > rows[1].ID {
		t.Errorf("list not sorted by id")
	}
	if err := svc.SetRoutine(ctx, "maintenance_bot", false, domain.SettingsJSON(`{"auto_apply_max_risk":"low"}`)); err != nil {
		t.Fatalf("set: %v", err)
	}
	g, err := svc.Gate(ctx, "maintenance_bot")
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if g.Enabled {
		t.Errorf("gate enabled=true; want false")
	}
}

func TestMaintenanceService_UnknownID_NotFound(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	_ = svc.SeedDefaults(ctx)
	if _, err := svc.Gate(ctx, "nope"); !isNotFound(err) {
		t.Errorf("gate unknown err = %v; want NotFound", err)
	}
	if err := svc.SetRoutine(ctx, "nope", true, domain.SettingsJSON("{}")); !isNotFound(err) {
		t.Errorf("set unknown err = %v; want NotFound", err)
	}
}

func TestMaintenanceService_RejectsInvalidSettings(t *testing.T) {
	svc := newMaintSvc(t)
	ctx := context.Background()
	_ = svc.SeedDefaults(ctx)
	if err := svc.SetRoutine(ctx, "git_autosync", true, domain.SettingsJSON(`not json`)); err == nil {
		t.Errorf("expected invalid-settings error, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/policy && go test ./internal/service/ -run TestMaintenanceService -v`
Expected: FAIL — `NewMaintenanceService` undefined.

- [ ] **Step 3: Write the implementation**

```go
// services/policy/internal/service/maintenance.go
package service

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"gorm.io/gorm"
)

type MaintenanceService struct {
	repo *repo.MaintenanceRepository
	log  *logger.Logger
}

func NewMaintenanceService(r *repo.MaintenanceRepository, log *logger.Logger) *MaintenanceService {
	return &MaintenanceService{repo: r, log: log}
}

// SeedDefaults inserts the parity defaults (insert-if-absent, idempotent).
func (s *MaintenanceService) SeedDefaults(ctx context.Context) error {
	for _, m := range domain.SeedRoutines() {
		if err := s.repo.SeedIfAbsent(ctx, m); err != nil {
			return err
		}
	}
	return nil
}

// List returns all routines sorted by id (stable admin-list order).
func (s *MaintenanceService) List(ctx context.Context) ([]domain.MaintenanceRoutine, error) {
	rows, err := s.repo.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ID < rows[j].ID })
	return rows, nil
}

// Gate returns the enforcement view a routine reads each run. Unknown id ⇒ NotFound
// (P3 callers treat any non-200 as fail-open enabled=true).
func (s *MaintenanceService) Gate(ctx context.Context, id string) (*domain.MaintenanceRoutine, error) {
	row, err := s.repo.GetByID(ctx, id)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, liberrors.NotFound("routine not found")
	}
	return row, err
}

// SetRoutine replaces enabled+settings for an existing routine.
func (s *MaintenanceService) SetRoutine(ctx context.Context, id string, enabled bool, settings domain.SettingsJSON) error {
	if err := s.mustExist(ctx, id); err != nil {
		return err
	}
	if !json.Valid(nonEmpty(settings)) {
		return liberrors.InvalidInput("settings must be valid JSON")
	}
	return s.repo.SetIntent(ctx, id, enabled, settings)
}

// SetStatus stamps last-run fields (consumed by P3 routines).
func (s *MaintenanceService) SetStatus(ctx context.Context, id string, ok bool, summary string, next *time.Time) error {
	if err := s.mustExist(ctx, id); err != nil {
		return err
	}
	if len(summary) > 512 {
		summary = summary[:512]
	}
	return s.repo.SetStatus(ctx, id, ok, summary, next)
}

func (s *MaintenanceService) mustExist(ctx context.Context, id string) error {
	if _, err := s.repo.GetByID(ctx, id); errors.Is(err, gorm.ErrRecordNotFound) {
		return liberrors.NotFound("routine not found")
	} else if err != nil {
		return err
	}
	return nil
}

func nonEmpty(s domain.SettingsJSON) []byte {
	if len(s) == 0 {
		return []byte("{}")
	}
	return []byte(s)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/policy && go test ./internal/service/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /data/ae-maintenance-panel add services/policy/internal/service/maintenance.go services/policy/internal/service/maintenance_test.go
git -C /data/ae-maintenance-panel commit -m "feat(policy): maintenance service (seed/list/gate/set/status)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: HTTP handlers (`handler/admin_maintenance.go` + `handler/internal_maintenance.go`)

**Files:**
- Create: `services/policy/internal/handler/admin_maintenance.go`
- Create: `services/policy/internal/handler/internal_maintenance.go`
- Test: `services/policy/internal/handler/maintenance_handler_test.go`

**Interfaces:**
- Consumes: `service.MaintenanceService`, `httputil` (`OK`/`Error`/`BadRequest`), `chi.URLParam`.
- Produces: `AdminMaintenanceHandler{List, SetRoutine}` via `NewAdminMaintenanceHandler(svc, log)`;
  `InternalMaintenanceHandler{Gate, SetStatus}` via `NewInternalMaintenanceHandler(svc, log)`.
  Admin `List` returns `{routines: []MaintenanceRoutine}`; `Gate` returns `{enabled, settings}`.

- [ ] **Step 1: Write the failing test** (spin the two handlers over a seeded sqlite service)

```go
// services/policy/internal/handler/maintenance_handler_test.go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func seededMaintHandlers(t *testing.T) (*AdminMaintenanceHandler, *InternalMaintenanceHandler) {
	t.Helper()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = db.AutoMigrate(&domain.MaintenanceRoutine{})
	svc := service.NewMaintenanceService(repo.NewMaintenanceRepository(db), logger.Default())
	_ = svc.SeedDefaults(context.Background())
	return NewAdminMaintenanceHandler(svc, logger.Default()), NewInternalMaintenanceHandler(svc, logger.Default())
}

func TestAdminMaintenance_ListAndSet(t *testing.T) {
	admin, internal := seededMaintHandlers(t)

	rec := httptest.NewRecorder()
	admin.List(rec, httptest.NewRequest(http.MethodGet, "/api/admin/policy/maintenance/routines", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("list code = %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "maintenance_bot") {
		t.Errorf("list body missing maintenance_bot: %s", rec.Body.String())
	}

	// PUT enabled=false + settings; chi URL param must be injected.
	body := strings.NewReader(`{"enabled":false,"settings":{"model":"opus"}}`)
	req := httptest.NewRequest(http.MethodPut, "/api/admin/policy/maintenance/routines/provider_recovery", body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("id", "provider_recovery")
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	rec = httptest.NewRecorder()
	admin.SetRoutine(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("set code = %d body=%s", rec.Code, rec.Body.String())
	}

	// Gate reflects the write.
	greq := httptest.NewRequest(http.MethodGet, "/internal/maintenance/routines/provider_recovery", nil)
	grctx := chi.NewRouteContext()
	grctx.URLParams.Add("id", "provider_recovery")
	greq = greq.WithContext(context.WithValue(greq.Context(), chi.RouteCtxKey, grctx))
	grec := httptest.NewRecorder()
	internal.Gate(grec, greq)
	var env struct {
		Data struct {
			Enabled  bool            `json:"enabled"`
			Settings json.RawMessage `json:"settings"`
		} `json:"data"`
	}
	if err := json.Unmarshal(grec.Body.Bytes(), &env); err != nil {
		t.Fatalf("gate decode: %v (%s)", err, grec.Body.String())
	}
	if env.Data.Enabled {
		t.Errorf("gate enabled=true; want false")
	}
	if !strings.Contains(string(env.Data.Settings), "opus") {
		t.Errorf("gate settings missing opus: %s", string(env.Data.Settings))
	}
}
```

> The `data` envelope wrapper assumes `httputil.OK` wraps in `{success,data}` (it does
> — see `admin_flags.go`). If your `httputil.OK` shape differs, match it.

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/policy && go test ./internal/handler/ -run TestAdminMaintenance -v`
Expected: FAIL — handler constructors undefined.

- [ ] **Step 3: Write the two handler files**

```go
// services/policy/internal/handler/admin_maintenance.go
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

// AdminMaintenanceHandler is the admin CRUD surface for maintenance routines
// (JWT + admin gated at the router; the gateway re-applies both). Nested under
// /api/admin/policy/maintenance so it reuses the existing policy admin proxy group.
type AdminMaintenanceHandler struct {
	svc *service.MaintenanceService
	log *logger.Logger
}

func NewAdminMaintenanceHandler(svc *service.MaintenanceService, log *logger.Logger) *AdminMaintenanceHandler {
	return &AdminMaintenanceHandler{svc: svc, log: log}
}

type listRoutinesResponse struct {
	Routines []domain.MaintenanceRoutine `json:"routines"`
}

func (h *AdminMaintenanceHandler) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.svc.List(r.Context())
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, listRoutinesResponse{Routines: rows})
}

type setRoutineRequest struct {
	Enabled  bool                `json:"enabled"`
	Settings domain.SettingsJSON `json:"settings"`
}

func (h *AdminMaintenanceHandler) SetRoutine(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req setRoutineRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	if err := h.svc.SetRoutine(r.Context(), id, req.Enabled, req.Settings); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"id": id})
}
```

```go
// services/policy/internal/handler/internal_maintenance.go
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/policy/internal/service"
)

// InternalMaintenanceHandler is the Docker-network + host-loopback surface a
// routine uses to read its gate and write back status. NOT gateway-proxied.
type InternalMaintenanceHandler struct {
	svc *service.MaintenanceService
	log *logger.Logger
}

func NewInternalMaintenanceHandler(svc *service.MaintenanceService, log *logger.Logger) *InternalMaintenanceHandler {
	return &InternalMaintenanceHandler{svc: svc, log: log}
}

type gateResponse struct {
	Enabled  bool                `json:"enabled"`
	Settings domain.SettingsJSON `json:"settings"`
}

func (h *InternalMaintenanceHandler) Gate(w http.ResponseWriter, r *http.Request) {
	row, err := h.svc.Gate(r.Context(), chi.URLParam(r, "id"))
	if err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, gateResponse{Enabled: row.Enabled, Settings: row.Settings})
}

type statusRequest struct {
	OK        bool       `json:"ok"`
	Summary   string     `json:"summary"`
	NextRunAt *time.Time `json:"next_run_at"`
}

func (h *InternalMaintenanceHandler) SetStatus(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	var req statusRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid JSON body")
		return
	}
	if err := h.svc.SetStatus(r.Context(), id, req.OK, req.Summary, req.NextRunAt); err != nil {
		httputil.Error(w, err)
		return
	}
	httputil.OK(w, map[string]string{"id": id})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/policy && go test ./internal/handler/ -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git -C /data/ae-maintenance-panel add services/policy/internal/handler/admin_maintenance.go services/policy/internal/handler/internal_maintenance.go services/policy/internal/handler/maintenance_handler_test.go
git -C /data/ae-maintenance-panel commit -m "feat(policy): maintenance HTTP handlers (admin CRUD + internal gate/status)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: Wire routes + migrate + seed (`transport/router.go`, `cmd/policy-api/main.go`)

**Files:**
- Modify: `services/policy/internal/transport/router.go`
- Modify: `services/policy/cmd/policy-api/main.go`
- Test: `services/policy/internal/transport/router_maintenance_test.go`

**Interfaces:**
- Consumes: `handler.AdminMaintenanceHandler`, `handler.InternalMaintenanceHandler` (Task 4).
- Produces: `NewRouter` extended to accept the two new handlers; routes
  `GET/PUT /api/admin/policy/maintenance/routines[/{id}]` (admin-gated) and
  `GET /internal/maintenance/routines/{id}` + `POST .../{id}/status`.

- [ ] **Step 1: Write the failing router test** (JWT admin path proxies through, mirroring the existing policy router tests)

```go
// services/policy/internal/transport/router_maintenance_test.go
package transport

// Add a test asserting an ADMIN-JWT GET /api/admin/policy/maintenance/routines
// returns 200 and a non-admin GET returns 403, mirroring the existing
// TestRouter_* cases in this package. Reuse the same JWT-minting + AutoMigrate
// helper the sibling router tests use; register &domain.MaintenanceRoutine{} in
// that helper's AutoMigrate and seed via service.SeedDefaults before serving.
//
// Assertions:
//   - admin token → 200, body contains "maintenance_bot"
//   - user token  → 403
//   - GET /internal/maintenance/routines/git_autosync (no auth) → 200
```

> This test file is a stub describing the assertions because the JWT/test harness
> here is package-local. Implement it by copying the nearest existing
> `router_*_test.go` case (e.g. the `TestRouter_Policy_AdminFlags_*` referenced in
> the gateway comment) and swapping the path + seed. Keep it concrete before Step 2.

- [ ] **Step 2: Run to verify it fails**

Run: `cd services/policy && go test ./internal/transport/ -run Maintenance -v`
Expected: FAIL (route not registered → 404, or compile error until wired).

- [ ] **Step 3: Extend `NewRouter`**

In `services/policy/internal/transport/router.go`, add the two handlers to the
signature and register routes:

```go
func NewRouter(
	adminH *handler.AdminFlagsHandler,
	publicH *handler.PublicFlagsHandler,
	internalH *handler.InternalRulesetHandler,
	adminMaintH *handler.AdminMaintenanceHandler,       // NEW
	internalMaintH *handler.InternalMaintenanceHandler, // NEW
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
	// ... unchanged setup ...

	// Docker-network + host-loopback only (never gateway-proxied).
	r.Get("/internal/policy/ruleset", internalH.GetRuleset)
	r.Get("/internal/maintenance/routines/{id}", internalMaintH.Gate)          // NEW
	r.Post("/internal/maintenance/routines/{id}/status", internalMaintH.SetStatus) // NEW

	r.Route("/api", func(r chi.Router) {
		r.Route("/policy/features", func(r chi.Router) {
			r.Use(OptionalAuthMiddleware(jwtConfig))
			r.Get("/mine", publicH.GetMine)
		})
		r.Route("/admin/policy", func(r chi.Router) {
			r.Use(AuthMiddleware(jwtConfig))
			r.Use(AdminRoleMiddleware)
			r.Get("/flags", adminH.List)
			r.Put("/flags/{key}", adminH.SetFlag)
			r.Put("/roulette", adminH.SetRoulette)
			// NEW — maintenance routines share the /admin/policy JWT+admin gate,
			// so the gateway needs NO new proxy route (/admin/policy/* already
			// forwards here).
			r.Get("/maintenance/routines", adminMaintH.List)
			r.Put("/maintenance/routines/{id}", adminMaintH.SetRoutine)
		})
	})
	return r
}
```

- [ ] **Step 4: Wire `main.go`**

In `services/policy/cmd/policy-api/main.go`:

```go
// migrate: add the new model
if err := db.AutoMigrate(&domain.FeatureFlag{}, &domain.MaintenanceRoutine{}); err != nil {
	log.Fatalw("failed to migrate database", "error", err)
}

// after policySvc seed:
maintSvc := service.NewMaintenanceService(repo.NewMaintenanceRepository(db.DB), log)
if err := maintSvc.SeedDefaults(context.Background()); err != nil {
	log.Fatalw("failed to seed maintenance routines", "error", err)
}

adminMaintH := handler.NewAdminMaintenanceHandler(maintSvc, log)
internalMaintH := handler.NewInternalMaintenanceHandler(maintSvc, log)

router := transport.NewRouter(adminH, publicH, internalH, adminMaintH, internalMaintH, cfg.JWT, log, metrics.NewCollector("policy"))
```

- [ ] **Step 5: Run the whole service test suite + build**

Run: `cd services/policy && go build ./... && go test ./... -count=1`
Expected: PASS across domain/repo/service/handler/transport.

- [ ] **Step 6: Commit**

```bash
git -C /data/ae-maintenance-panel add services/policy/internal/transport/router.go services/policy/internal/transport/router_maintenance_test.go services/policy/cmd/policy-api/main.go
git -C /data/ae-maintenance-panel commit -m "feat(policy): wire maintenance routes, migrate + seed on boot

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Phase 2 — Frontend Maintenance tab

### Task 6: FE data layer (api client + composable + descriptor registry)

**Files:**
- Modify: `frontend/web/src/api/client.ts` (add types + two `adminApi` methods)
- Create: `frontend/web/src/composables/useAdminMaintenance.ts`
- Create: `frontend/web/src/config/maintenanceRoutines.ts`
- Test: `frontend/web/src/config/__tests__/maintenanceRoutines.spec.ts`

**Interfaces:**
- Produces (client.ts): `interface MaintenanceRoutineWire { id; enabled; settings: Record<string,unknown>;
  lastRunAt: string|null; lastOk: boolean|null; lastSummary: string; nextRunAt: string|null; updatedAt: string }`,
  `interface MaintenanceRoutinesResponse { routines: MaintenanceRoutineWire[] }`,
  `adminApi.getMaintenanceRoutines()`, `adminApi.setMaintenanceRoutine(id, {enabled, settings})`.
- Produces (composable): `useAdminMaintenance()` → `{ list(), setRoutine(id, body) }`.
- Produces (registry): `MAINTENANCE_ROUTINES: MaintenanceRoutineDescriptor[]`,
  `routineDescriptor(id)`, types `MaintenanceKnob`, `MaintenanceRoutineDescriptor`.

- [ ] **Step 1: Add api client types + methods** (`frontend/web/src/api/client.ts`)

Place the types next to `ScraperProviderWire` (~line 633) and the methods next to
`setScraperProviderPolicy` (~line 748) inside the `adminApi` object:

```ts
// Maintenance routines (Maintenance tab). Nested under /admin/policy/* so it
// reuses the existing gateway admin-policy proxy group (no gateway change).
export interface MaintenanceRoutineWire {
  id: string
  enabled: boolean
  settings: Record<string, unknown>
  lastRunAt: string | null
  lastOk: boolean | null
  lastSummary: string
  nextRunAt: string | null
  updatedAt: string
}
export interface MaintenanceRoutinesResponse {
  routines: MaintenanceRoutineWire[]
}
```

```ts
  // --- inside the adminApi object literal ---
  getMaintenanceRoutines: () =>
    apiClient.get<{ data: MaintenanceRoutinesResponse } | MaintenanceRoutinesResponse>(
      '/admin/policy/maintenance/routines',
    ),
  setMaintenanceRoutine: (id: string, body: { enabled: boolean; settings: Record<string, unknown> }) =>
    apiClient.put<{ data: { id: string } } | { id: string }>(
      `/admin/policy/maintenance/routines/${encodeURIComponent(id)}`,
      body,
    ),
```

- [ ] **Step 2: Create the composable** (`frontend/web/src/composables/useAdminMaintenance.ts`)

```ts
import { adminApi } from '@/api/client'
import type { MaintenanceRoutineWire, MaintenanceRoutinesResponse } from '@/api/client'

// Maintenance tab composable, mirroring useAdminProviders.ts. Responses use the
// standard {success,data} envelope, so we unwrap `res.data?.data ?? res.data`.
export type { MaintenanceRoutineWire }

function unwrap<T>(data: unknown): T {
  const d = data as { data?: T }
  return d && typeof d === 'object' && 'data' in d ? (d.data as T) : (data as T)
}

export function useAdminMaintenance() {
  async function list(): Promise<MaintenanceRoutineWire[]> {
    const res = await adminApi.getMaintenanceRoutines()
    return unwrap<MaintenanceRoutinesResponse>(res.data).routines
  }
  async function setRoutine(
    id: string,
    body: { enabled: boolean; settings: Record<string, unknown> },
  ): Promise<void> {
    await adminApi.setMaintenanceRoutine(id, body)
  }
  return { list, setRoutine }
}
```

- [ ] **Step 3: Create the descriptor registry** (`frontend/web/src/config/maintenanceRoutines.ts`)

```ts
/**
 * Static registry mapping each maintenance routine id (services/policy
 * MaintenanceRoutine.ID) to its display group, i18n name key, staleness
 * threshold, and the safe knobs the admin may tune. The Maintenance tab renders
 * cards from the BACKEND list, using this registry for labels + knob controls;
 * a backend row with no descriptor still renders (enable toggle only).
 *
 * Keep the ids in sync with domain.SeedRoutines(). Select-knob option values are
 * literal tokens (model names, durations, risk levels) shown verbatim — NOT i18n.
 */
export type MaintenanceKnob =
  | { key: string; type: 'select'; labelKey: string; options: string[] }
  | { key: string; type: 'number'; labelKey: string; min?: number; max?: number }
  | { key: string; type: 'chips'; labelKey: string; placeholderKey: string }

export interface MaintenanceRoutineDescriptor {
  id: string
  group: 'host' | 'cluster'
  nameKey: string
  /** ms after lastRunAt beyond which the status badge reads "stale"; omit to disable. */
  staleAfterMs?: number
  knobs: MaintenanceKnob[]
}

const HOUR = 3_600_000
const DAY = 24 * HOUR

export const MAINTENANCE_ROUTINES: MaintenanceRoutineDescriptor[] = [
  {
    id: 'maintenance_bot',
    group: 'host',
    nameKey: 'admin.policy.maintenance.routines.maintenance_bot',
    knobs: [
      { key: 'auto_apply_max_risk', type: 'select', labelKey: 'admin.policy.maintenance.knobs.autoApplyMaxRisk', options: ['none', 'low', 'medium'] },
      { key: 'suppressed_alerts', type: 'chips', labelKey: 'admin.policy.maintenance.knobs.suppressedAlerts', placeholderKey: 'admin.policy.maintenance.knobs.suppressedAlertsPlaceholder' },
    ],
  },
  {
    id: 'provider_recovery',
    group: 'host',
    nameKey: 'admin.policy.maintenance.routines.provider_recovery',
    staleAfterMs: 2 * DAY,
    knobs: [{ key: 'model', type: 'select', labelKey: 'admin.policy.maintenance.knobs.model', options: ['sonnet', 'opus'] }],
  },
  { id: 'git_autosync', group: 'host', nameKey: 'admin.policy.maintenance.routines.git_autosync', staleAfterMs: HOUR, knobs: [] },
  {
    id: 'disk_prune',
    group: 'host',
    nameKey: 'admin.policy.maintenance.routines.disk_prune',
    staleAfterMs: 2 * DAY,
    knobs: [{ key: 'high_water_pct', type: 'number', labelKey: 'admin.policy.maintenance.knobs.highWaterPct', min: 50, max: 95 }],
  },
  { id: 'build_cache_prune', group: 'host', nameKey: 'admin.policy.maintenance.routines.build_cache_prune', staleAfterMs: 8 * DAY, knobs: [] },
  { id: 'subtitle_probe', group: 'cluster', nameKey: 'admin.policy.maintenance.routines.subtitle_probe', staleAfterMs: HOUR, knobs: [] },
  { id: 'shikimori_sync', group: 'cluster', nameKey: 'admin.policy.maintenance.routines.shikimori_sync', staleAfterMs: 2 * DAY, knobs: [] },
  { id: 'playability_canary', group: 'cluster', nameKey: 'admin.policy.maintenance.routines.playability_canary', staleAfterMs: 2 * DAY, knobs: [] },
  {
    id: 'provider_self_heal',
    group: 'cluster',
    nameKey: 'admin.policy.maintenance.routines.provider_self_heal',
    knobs: [
      { key: 'demote_after', type: 'select', labelKey: 'admin.policy.maintenance.knobs.demoteAfter', options: ['12h', '24h', '48h'] },
      { key: 'probe_every', type: 'select', labelKey: 'admin.policy.maintenance.knobs.probeEvery', options: ['3h', '6h', '12h'] },
    ],
  },
]

const BY_ID = new Map(MAINTENANCE_ROUTINES.map((d) => [d.id, d]))
export function routineDescriptor(id: string): MaintenanceRoutineDescriptor | undefined {
  return BY_ID.get(id)
}
```

- [ ] **Step 4: Write + run the registry spec**

```ts
// frontend/web/src/config/__tests__/maintenanceRoutines.spec.ts
import { describe, it, expect } from 'vitest'
import { MAINTENANCE_ROUTINES, routineDescriptor } from '@/config/maintenanceRoutines'

describe('maintenanceRoutines registry', () => {
  it('covers the 9 seeded routine ids', () => {
    const ids = MAINTENANCE_ROUTINES.map((d) => d.id).sort()
    expect(ids).toEqual([
      'build_cache_prune', 'disk_prune', 'git_autosync', 'maintenance_bot',
      'playability_canary', 'provider_recovery', 'provider_self_heal',
      'shikimori_sync', 'subtitle_probe',
    ])
  })
  it('assigns every routine to a known group', () => {
    for (const d of MAINTENANCE_ROUTINES) expect(['host', 'cluster']).toContain(d.group)
  })
  it('resolves a descriptor by id and returns undefined for unknown', () => {
    expect(routineDescriptor('maintenance_bot')?.group).toBe('host')
    expect(routineDescriptor('nope')).toBeUndefined()
  })
  it('select knobs carry non-empty literal option lists', () => {
    for (const d of MAINTENANCE_ROUTINES) {
      for (const k of d.knobs) {
        if (k.type === 'select') expect(k.options.length).toBeGreaterThan(0)
      }
    }
  })
})
```

Run: `cd frontend/web && bunx vitest run src/config/__tests__/maintenanceRoutines.spec.ts`
Expected: PASS. Then `bunx tsc --noEmit` — Expected: clean.

- [ ] **Step 5: Commit**

```bash
git -C /data/ae-maintenance-panel add frontend/web/src/api/client.ts frontend/web/src/composables/useAdminMaintenance.ts frontend/web/src/config/maintenanceRoutines.ts frontend/web/src/config/__tests__/maintenanceRoutines.spec.ts
git -C /data/ae-maintenance-panel commit -m "feat(web): maintenance tab data layer (api + composable + registry)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 7: Maintenance tab UI + i18n (`AdminPolicy.vue`, locales) + component spec

**Files:**
- Modify: `frontend/web/src/views/admin/AdminPolicy.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`
- Modify: `frontend/web/src/views/admin/AdminPolicy.spec.ts` (extend the existing
  harness — do NOT create a new spec file; the file already wires i18n, the
  `switchStub`, `confirmMock`, and mocks the sibling composables)

**Interfaces:**
- Consumes: `useAdminMaintenance`, `MAINTENANCE_ROUTINES`/`routineDescriptor`, `MaintenanceRoutineWire`,
  `useConfirm`, `useToast`; UI primitives `Card/CardHeader/CardTitle/CardContent/Badge/Switch/
  SegmentedControl/Input/Chip/Button/Spinner/EmptyState` (all already exported from `@/components/ui`).

- [ ] **Step 1: Add the tab definition + script state** (`<script setup>` of `AdminPolicy.vue`)

Add imports (extend the existing `@/components/ui` import and lucide/composable imports):

```ts
import { useAdminMaintenance } from '@/composables/useAdminMaintenance'
import type { MaintenanceRoutineWire } from '@/composables/useAdminMaintenance'
import { MAINTENANCE_ROUTINES, routineDescriptor } from '@/config/maintenanceRoutines'
// extend the existing `from '@/components/ui'` import with: Input
```

Add `'maintenance'` to `tabDefs`:

```ts
const tabDefs = computed(() => [
  { value: 'features', label: t('admin.policy.tabs.features') },
  { value: 'providers', label: t('admin.policy.tabs.providers') },
  { value: 'maintenance', label: t('admin.policy.tabs.maintenance') },
])
```

Add the maintenance state + logic (place after the providers section):

```ts
// ─── MAINTENANCE TAB ────────────────────────────────────────────────────
interface MaintenanceRow extends MaintenanceRoutineWire {
  saving: boolean
  draft: Record<string, unknown>      // knob edit buffer
  original: string                    // JSON of settings at load/save (dirty check)
}

const maintenance = useAdminMaintenance()
const isMaintLoading = ref(true)
const maintError = ref<string | null>(null)
const maintRows = ref<MaintenanceRow[]>([])
const alertDraft = ref<Record<string, string>>({}) // per-routine "add suppressed alert" input

function toMaintRow(w: MaintenanceRoutineWire): MaintenanceRow {
  return { ...w, saving: false, draft: { ...w.settings }, original: JSON.stringify(w.settings ?? {}) }
}

async function loadMaintenance(): Promise<void> {
  isMaintLoading.value = true
  maintError.value = null
  try {
    const list = await maintenance.list()
    // Render in registry order (host first, then cluster); unknown ids appended.
    const order = new Map(MAINTENANCE_ROUTINES.map((d, i) => [d.id, i]))
    maintRows.value = list
      .map(toMaintRow)
      .sort((a, b) => (order.get(a.id) ?? 999) - (order.get(b.id) ?? 999))
  } catch (e) {
    maintError.value = maintErrText(e)
  } finally {
    isMaintLoading.value = false
  }
}

function maintErrText(e: unknown): string {
  const err = e as { response?: { status?: number; data?: { error?: { message?: string } } }; message?: string }
  return err.response?.status === 403 ? '403' : (err.response?.data?.error?.message || err.message || t('admin.policy.maintenance.loadError'))
}

const maintGroups = computed(() => {
  const host = maintRows.value.filter((r) => (routineDescriptor(r.id)?.group ?? 'host') === 'host')
  const cluster = maintRows.value.filter((r) => routineDescriptor(r.id)?.group === 'cluster')
  return [
    { key: 'host', titleKey: 'admin.policy.maintenance.groups.host', rows: host },
    { key: 'cluster', titleKey: 'admin.policy.maintenance.groups.cluster', rows: cluster },
  ].filter((g) => g.rows.length > 0)
})

function routineName(id: string): string {
  const d = routineDescriptor(id)
  return d ? t(d.nameKey) : id
}

// ── status badge ──
type MaintStatus = 'ok' | 'failed' | 'stale' | 'never'
function routineStatus(row: MaintenanceRow): MaintStatus {
  if (!row.lastRunAt) return 'never'
  const stale = routineDescriptor(row.id)?.staleAfterMs
  if (stale && Date.now() - new Date(row.lastRunAt).getTime() > stale) return 'stale'
  return row.lastOk === false ? 'failed' : 'ok'
}
const STATUS_VARIANT: Record<MaintStatus, NonNullable<BadgeVariants['variant']>> = {
  ok: 'success', failed: 'destructive', stale: 'warning', never: 'default',
}
function statusVariant(row: MaintenanceRow) { return STATUS_VARIANT[routineStatus(row)] }
function statusLabel(row: MaintenanceRow) { return t(`admin.policy.maintenance.status.${routineStatus(row)}`) }

// ── enable/pause (instant, confirm-gated on pause) ──
async function onToggleRoutine(row: MaintenanceRow, enabled: boolean): Promise<void> {
  const name = routineName(row.id)
  if (!enabled) {
    const ok = await confirm({
      title: t('admin.policy.maintenance.confirmPauseTitle', { name }),
      description: t('admin.policy.maintenance.confirmPauseBody', { name }),
      confirmText: t('admin.policy.maintenance.pauseAction'),
      cancelText: t('common.cancel'),
      variant: 'destructive',
    })
    if (!ok) return
  }
  await applyRoutine(row, enabled, currentSettings(row), enabled ? 'toastEnable' : 'toastPause')
}

// ── save knobs (explicit) ──
function currentSettings(row: MaintenanceRow): Record<string, unknown> {
  // Coerce number knobs from their string Input values.
  const out: Record<string, unknown> = { ...row.settings, ...row.draft }
  for (const k of routineDescriptor(row.id)?.knobs ?? []) {
    if (k.type === 'number' && out[k.key] !== undefined && out[k.key] !== '') out[k.key] = Number(out[k.key])
  }
  return out
}
function isKnobDirty(row: MaintenanceRow): boolean {
  return JSON.stringify(currentSettings(row)) !== row.original
}
async function saveKnobs(row: MaintenanceRow): Promise<void> {
  await applyRoutine(row, row.enabled, currentSettings(row), 'toastSave')
}

async function applyRoutine(row: MaintenanceRow, enabled: boolean, settings: Record<string, unknown>, kind: 'toastEnable' | 'toastPause' | 'toastSave'): Promise<void> {
  const prevEnabled = row.enabled
  const name = routineName(row.id)
  row.saving = true
  row.enabled = enabled
  try {
    await maintenance.setRoutine(row.id, { enabled, settings })
    row.settings = settings
    row.draft = { ...settings }
    row.original = JSON.stringify(settings)
    toast.push(t(`admin.policy.maintenance.${kind}Success`, { name }), 'success')
  } catch {
    row.enabled = prevEnabled
    toast.push(t(`admin.policy.maintenance.${kind}Error`, { name }), 'error')
  } finally {
    row.saving = false
  }
}

// ── suppressed-alerts chip editor (maintenance_bot) ──
function alertList(row: MaintenanceRow): string[] {
  const v = row.draft['suppressed_alerts']
  return Array.isArray(v) ? (v as string[]) : []
}
function addAlert(row: MaintenanceRow): void {
  const val = (alertDraft.value[row.id] || '').trim()
  if (!val) return
  const cur = alertList(row)
  if (!cur.includes(val)) row.draft['suppressed_alerts'] = [...cur, val]
  alertDraft.value[row.id] = ''
}
function removeAlert(row: MaintenanceRow, v: string): void {
  row.draft['suppressed_alerts'] = alertList(row).filter((x) => x !== v)
}

function segOptions(opts: string[]) { return opts.map((o) => ({ value: o, label: o })) }
```

Extend `onMounted` to also load maintenance:

```ts
onMounted(() => {
  load()
  loadProviders()
  loadMaintenance()
})
```

- [ ] **Step 2: Add the `#maintenance` template block** (inside `<Tabs>`, after the `#providers` template)

```vue
<!-- ─── MAINTENANCE TAB ─────────────────────────────────────────────
     Pull-config control board: pause/resume each routine + tune safe
     knobs + read last-run status. Host routines' toggles are a PAUSE
     (the systemd timer still fires; the script early-exits). -->
<template #maintenance>
  <div v-if="maintError === '403'" class="glass-card p-4 mb-6 border border-destructive/40">
    <p class="text-destructive">{{ $t('admin.policy.error403') }}</p>
  </div>
  <div v-else-if="maintError" class="glass-card p-4 mb-6 border border-destructive/40">
    <p class="text-destructive">{{ maintError }}</p>
  </div>

  <div v-if="isMaintLoading" class="flex justify-center py-12">
    <Spinner size="lg" />
  </div>

  <template v-else>
    <div class="mb-6">
      <h2 class="text-base font-semibold text-white">{{ $t('admin.policy.maintenance.title') }}</h2>
      <p class="text-white/60 text-sm mt-1">{{ $t('admin.policy.maintenance.intro') }}</p>
    </div>

    <EmptyState v-if="maintRows.length === 0" class="mb-8">
      {{ $t('admin.policy.maintenance.loadError') }}
    </EmptyState>

    <div v-for="group in maintGroups" :key="group.key" class="mb-8">
      <p class="text-xs uppercase tracking-wide text-white/50 mb-3">{{ $t(group.titleKey) }}</p>
      <div class="grid gap-4">
        <Card v-for="row in group.rows" :key="row.id" padding="none" data-testid="routine-card">
          <CardHeader class="flex flex-row flex-wrap items-start justify-between gap-3">
            <div class="min-w-0">
              <CardTitle class="text-base">{{ routineName(row.id) }}</CardTitle>
              <p class="mt-1 flex flex-wrap items-center gap-x-2 gap-y-1 text-xs text-white/40">
                <span class="font-mono">{{ row.id }}</span>
              </p>
              <p class="mt-2 flex flex-wrap items-center gap-2 text-xs text-white/60">
                <Badge :variant="statusVariant(row)" :data-testid="`routine-status-${row.id}`">
                  {{ statusLabel(row) }}
                </Badge>
                <span v-if="row.lastSummary">{{ row.lastSummary }}</span>
              </p>
            </div>
            <div class="flex items-center gap-3 shrink-0">
              <span class="text-xs text-white/60">
                {{ row.enabled ? $t('admin.policy.maintenance.enabled') : $t('admin.policy.maintenance.paused') }}
              </span>
              <Switch
                :model-value="row.enabled"
                :disabled="row.saving"
                :aria-label="$t('admin.policy.maintenance.toggleLabel', { name: routineName(row.id) })"
                :data-testid="`routine-switch-${row.id}`"
                @update:model-value="(v: boolean) => onToggleRoutine(row, v)"
              />
            </div>
          </CardHeader>

          <CardContent v-if="row.enabled && (routineDescriptor(row.id)?.knobs.length ?? 0) > 0" class="pt-0">
            <div class="grid gap-4 sm:grid-cols-2">
              <div v-for="knob in routineDescriptor(row.id)!.knobs" :key="knob.key">
                <p class="text-xs uppercase tracking-wide text-white/50 mb-2">{{ $t(knob.labelKey) }}</p>

                <SegmentedControl
                  v-if="knob.type === 'select'"
                  :model-value="String(row.draft[knob.key] ?? '')"
                  :options="segOptions(knob.options)"
                  :aria-label="$t(knob.labelKey)"
                  @update:model-value="(v: string) => (row.draft[knob.key] = v)"
                />

                <Input
                  v-else-if="knob.type === 'number'"
                  type="number"
                  :min="knob.min"
                  :max="knob.max"
                  :model-value="String(row.draft[knob.key] ?? '')"
                  :aria-label="$t(knob.labelKey)"
                  @update:model-value="(v: string | number) => (row.draft[knob.key] = v)"
                />

                <div v-else-if="knob.type === 'chips'">
                  <div class="flex flex-wrap gap-2 mb-2">
                    <Chip
                      v-for="a in alertList(row)"
                      :key="a"
                      removable
                      size="sm"
                      @remove="removeAlert(row, a)"
                    >
                      {{ a }}
                    </Chip>
                  </div>
                  <Input
                    v-model="alertDraft[row.id]"
                    type="text"
                    :placeholder="$t(knob.placeholderKey)"
                    :aria-label="$t(knob.labelKey)"
                    @keyup.enter="addAlert(row)"
                  />
                </div>
              </div>
            </div>

            <div class="mt-4 flex justify-end">
              <Button
                size="sm"
                :loading="row.saving"
                :disabled="row.saving || !isKnobDirty(row)"
                :data-testid="`routine-save-${row.id}`"
                @click="saveKnobs(row)"
              >
                {{ $t('admin.policy.save') }}
              </Button>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  </template>
</template>
```

- [ ] **Step 3: Add i18n keys — `en.json`** (merge into `admin.policy`)

Add `"maintenance": "Maintenance"` to `admin.policy.tabs`, and this block under `admin.policy`:

```json
"maintenance": {
  "title": "Maintenance routines",
  "intro": "Pause or resume background routines and tune their safe knobs. Pausing a host routine skips its next run — the schedule stays intact.",
  "loadError": "Failed to load maintenance routines.",
  "groups": { "host": "Host routines", "cluster": "In-cluster routines" },
  "enabled": "Enabled",
  "paused": "Paused",
  "toggleLabel": "Toggle {name}",
  "confirmPauseTitle": "Pause {name}?",
  "confirmPauseBody": "{name} will skip its runs until you resume it. Auto-recovery and cleanup won't happen while paused.",
  "pauseAction": "Pause",
  "toastEnableSuccess": "{name} resumed",
  "toastEnableError": "Couldn't resume {name}",
  "toastPauseSuccess": "{name} paused",
  "toastPauseError": "Couldn't pause {name}",
  "toastSaveSuccess": "Saved {name} settings",
  "toastSaveError": "Couldn't save {name} settings",
  "status": { "ok": "OK", "failed": "Failed", "stale": "Stale", "never": "No runs yet" },
  "routines": {
    "maintenance_bot": "Maintenance bot",
    "provider_recovery": "Daily provider recovery",
    "git_autosync": "Git autosync",
    "disk_prune": "Disk prune",
    "build_cache_prune": "Build-cache prune",
    "subtitle_probe": "Subtitle probe",
    "shikimori_sync": "Shikimori sync",
    "playability_canary": "Playability canary",
    "provider_self_heal": "Provider self-heal"
  },
  "knobs": {
    "autoApplyMaxRisk": "Auto-apply max risk",
    "suppressedAlerts": "Suppressed alerts",
    "suppressedAlertsPlaceholder": "Alert name, then Enter",
    "model": "Model",
    "highWaterPct": "Disk high-water %",
    "demoteAfter": "Demote after",
    "probeEvery": "Probe every"
  }
}
```

- [ ] **Step 4: Add i18n keys — `ru.json`** (same structure, add `"maintenance": "Обслуживание"` to `tabs`)

```json
"maintenance": {
  "title": "Регламентные процессы",
  "intro": "Приостанавливайте или возобновляйте фоновые процессы и настраивайте их безопасные параметры. Пауза хост-процесса пропускает следующий запуск — расписание сохраняется.",
  "loadError": "Не удалось загрузить регламентные процессы.",
  "groups": { "host": "Хост-процессы", "cluster": "Процессы в кластере" },
  "enabled": "Включён",
  "paused": "На паузе",
  "toggleLabel": "Переключить {name}",
  "confirmPauseTitle": "Поставить {name} на паузу?",
  "confirmPauseBody": "{name} не будет запускаться, пока вы не возобновите его. Авто-восстановление и очистка на паузе не выполняются.",
  "pauseAction": "Пауза",
  "toastEnableSuccess": "{name} возобновлён",
  "toastEnableError": "Не удалось возобновить {name}",
  "toastPauseSuccess": "{name} на паузе",
  "toastPauseError": "Не удалось поставить на паузу {name}",
  "toastSaveSuccess": "Настройки {name} сохранены",
  "toastSaveError": "Не удалось сохранить настройки {name}",
  "status": { "ok": "OK", "failed": "Сбой", "stale": "Устарело", "never": "Ещё не запускался" },
  "routines": {
    "maintenance_bot": "Бот обслуживания",
    "provider_recovery": "Ежедневное восстановление провайдеров",
    "git_autosync": "Git-автосинхронизация",
    "disk_prune": "Очистка диска",
    "build_cache_prune": "Очистка кэша сборки",
    "subtitle_probe": "Проба субтитров",
    "shikimori_sync": "Синхронизация Shikimori",
    "playability_canary": "Канарейка воспроизводимости",
    "provider_self_heal": "Само-восстановление провайдеров"
  },
  "knobs": {
    "autoApplyMaxRisk": "Макс. риск авто-применения",
    "suppressedAlerts": "Подавленные алерты",
    "suppressedAlertsPlaceholder": "Название алерта, затем Enter",
    "model": "Модель",
    "highWaterPct": "Порог заполнения диска, %",
    "demoteAfter": "Понизить через",
    "probeEvery": "Проба каждые"
  }
}
```

- [ ] **Step 5: Add i18n keys — `ja.json`** (add `"maintenance": "メンテナンス"` to `tabs`)

```json
"maintenance": {
  "title": "メンテナンスルーチン",
  "intro": "バックグラウンドルーチンを一時停止・再開し、安全な設定を調整します。ホストルーチンの一時停止は次回実行をスキップしますが、スケジュールは維持されます。",
  "loadError": "メンテナンスルーチンの読み込みに失敗しました。",
  "groups": { "host": "ホストルーチン", "cluster": "クラスター内ルーチン" },
  "enabled": "有効",
  "paused": "一時停止中",
  "toggleLabel": "{name} を切り替え",
  "confirmPauseTitle": "{name} を一時停止しますか？",
  "confirmPauseBody": "再開するまで {name} は実行されません。一時停止中は自動復旧やクリーンアップは行われません。",
  "pauseAction": "一時停止",
  "toastEnableSuccess": "{name} を再開しました",
  "toastEnableError": "{name} を再開できませんでした",
  "toastPauseSuccess": "{name} を一時停止しました",
  "toastPauseError": "{name} を一時停止できませんでした",
  "toastSaveSuccess": "{name} の設定を保存しました",
  "toastSaveError": "{name} の設定を保存できませんでした",
  "status": { "ok": "OK", "failed": "失敗", "stale": "古い", "never": "実行履歴なし" },
  "routines": {
    "maintenance_bot": "メンテナンスボット",
    "provider_recovery": "毎日のプロバイダ復旧",
    "git_autosync": "Git 自動同期",
    "disk_prune": "ディスク整理",
    "build_cache_prune": "ビルドキャッシュ整理",
    "subtitle_probe": "字幕プローブ",
    "shikimori_sync": "Shikimori 同期",
    "playability_canary": "再生カナリア",
    "provider_self_heal": "プロバイダ自己修復"
  },
  "knobs": {
    "autoApplyMaxRisk": "自動適用の最大リスク",
    "suppressedAlerts": "抑制するアラート",
    "suppressedAlertsPlaceholder": "アラート名を入力して Enter",
    "model": "モデル",
    "highWaterPct": "ディスク上限 %",
    "demoteAfter": "降格までの時間",
    "probeEvery": "プローブ間隔"
  }
}
```

- [ ] **Step 6: Extend `AdminPolicy.spec.ts`** with a Maintenance-tab block that
  reuses the file's existing harness (`i18n`, `switchStub`, `confirmMock`,
  `mountComponent`, and tab activation via `#tab-<value>`).

  **6a — Add the composable mock** (next to the `useAdminProviders` mock block near
  the top; `mock`-prefixed names are allowed inside `vi.mock` factories, matching the
  existing `mockList`/`mockProvidersList`):

```ts
const mockMaintenanceList = vi.fn()
const mockSetRoutine = vi.fn()

vi.mock('@/composables/useAdminMaintenance', async (importOriginal) => {
  const actual = await importOriginal<typeof import('@/composables/useAdminMaintenance')>()
  return {
    ...actual,
    useAdminMaintenance: () => ({ list: mockMaintenanceList, setRoutine: mockSetRoutine }),
  }
})

function maintRow(over: Record<string, unknown> = {}) {
  return {
    id: 'provider_recovery', enabled: true, settings: { model: 'sonnet' },
    lastRunAt: '2026-07-10T00:00:00Z', lastOk: true, lastSummary: 'adopted okru · exit 0',
    nextRunAt: null, updatedAt: '2026-07-10T00:00:00Z', ...over,
  }
}
```

  **6b — Give the existing `beforeEach` a default** so mounts for the Features/Providers
  tests don't reject in `loadMaintenance()` (add this line inside the current
  `beforeEach`, after the `mockSetPolicy.mockResolvedValue(...)` line):

```ts
    mockMaintenanceList.mockResolvedValue([maintRow()])
    mockSetRoutine.mockResolvedValue(undefined)
```

  **6c — Add the describe block** (inside the top-level `describe('AdminPolicy', ...)`,
  after the Providers-tab tests). Note `provider_recovery`'s staleAfterMs is 2 days;
  the fixed `lastRunAt` is old, so pin `lastRunAt` to "now-ish" where the test needs a
  non-stale status by overriding it):

```ts
  describe('Maintenance tab', () => {
    async function mountAndOpenMaintenanceTab() {
      const w = mountComponent()
      await flushPromises()
      await w.find('#tab-maintenance').trigger('click')
      await flushPromises()
      return w
    }

    it('renders a card per routine from the composable', async () => {
      mockMaintenanceList.mockResolvedValue([
        maintRow({ lastRunAt: new Date().toISOString() }),
        maintRow({ id: 'git_autosync', settings: {}, lastRunAt: new Date().toISOString() }),
      ])
      const w = await mountAndOpenMaintenanceTab()
      expect(w.findAll('[data-testid="routine-card"]').length).toBe(2)
    })

    it('confirms before pausing and calls setRoutine with enabled=false', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastRunAt: new Date().toISOString() })])
      confirmMock.mockResolvedValue(true)
      const w = await mountAndOpenMaintenanceTab()
      // Switch is stubbed as a <button> that emits update:modelValue(!current).
      await w.find('[data-testid="routine-switch-provider_recovery"]').trigger('click')
      await flushPromises()
      expect(confirmMock).toHaveBeenCalled()
      expect(mockSetRoutine).toHaveBeenCalledWith('provider_recovery', expect.objectContaining({ enabled: false }))
    })

    it('does NOT pause when confirm is declined', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastRunAt: new Date().toISOString() })])
      confirmMock.mockResolvedValue(false)
      const w = await mountAndOpenMaintenanceTab()
      await w.find('[data-testid="routine-switch-provider_recovery"]').trigger('click')
      await flushPromises()
      expect(mockSetRoutine).not.toHaveBeenCalled()
    })

    it('maps a failed last run to the destructive status badge', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastOk: false, lastRunAt: new Date().toISOString() })])
      const w = await mountAndOpenMaintenanceTab()
      expect(w.find('[data-testid="routine-status-provider_recovery"]').text()).toContain('Failed')
    })

    it('disables Save until a knob changes', async () => {
      mockMaintenanceList.mockResolvedValue([maintRow({ lastRunAt: new Date().toISOString() })])
      const w = await mountAndOpenMaintenanceTab()
      expect(w.find('[data-testid="routine-save-provider_recovery"]').attributes('disabled')).toBeDefined()
    })
  })
```

> If `#tab-maintenance` doesn't resolve, confirm the `Tabs` primitive renders trigger
> ids as `#tab-<value>` (the Providers tests use `#tab-providers`) — the new tab's
> value is `maintenance`, so the id is `#tab-maintenance`.

- [ ] **Step 7: Run FE checks**

```bash
cd frontend/web
bunx vitest run src/views/admin/AdminPolicy.spec.ts src/config/__tests__/maintenanceRoutines.spec.ts src/locales/__tests__
bunx tsc --noEmit
bash scripts/design-system-lint.sh
```
Expected: specs PASS, tsc clean, DS-lint `ERRORS: 0`. Fix any i18n parity failure by
aligning en/ru/ja key sets.

- [ ] **Step 8: Commit**

```bash
git -C /data/ae-maintenance-panel add frontend/web/src/views/admin/AdminPolicy.vue frontend/web/src/views/admin/AdminPolicy.spec.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json
git -C /data/ae-maintenance-panel commit -m "feat(web): Maintenance tab on /admin/policy (pause + knobs + status)

Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Final verification (before ship)

- [ ] `cd services/policy && go build ./... && go test ./... -count=1` → all PASS.
- [ ] `cd frontend/web && bunx vitest run src/views/admin src/config src/locales && bunx tsc --noEmit && bash scripts/design-system-lint.sh` → PASS + `ERRORS: 0`.
- [ ] Run `/frontend-verify` (DS + i18n en/ru/ja parity + real `bun run build`).
- [ ] Deploy from the worktree: `make redeploy-policy` then `make redeploy-web`; `make health` → policy + web UP.
- [ ] Manual smoke (admin): open `/admin/policy` → **Maintenance** tab → 9 routines in two
      groups; toggle one off (confirm dialog) → persists across reload; edit a knob → Save
      enables → persists; status badge = "No runs yet" until P3 wiring reports.
- [ ] Then `/animeenigma-after-update` (simplify → changelog Trump-mode → commit → push).

## Deferred to the P3 plan (separate)

Enforcement wiring, one routine at a time — each reads
`GET localhost:8098/internal/maintenance/routines/{id}` (host) or
`http://policy:8098/...` (Docker), early-exits when `enabled=false` (fail-open on any
non-200), and POSTs `.../{id}/status` after running:
`infra/host/animeenigma-provider-recovery.sh`, `animeenigma-git-autosync.sh`, the prune
crons, the `maintenance` daemon (+ `auto_apply_max_risk` in `decideAutoApply`), the
`scheduler` per-job gate, and catalog's self-heal engine. Host-script installs to
`/usr/local/bin` are an owner step (auto-mode classifier blocks AI self-wiring of host units).
