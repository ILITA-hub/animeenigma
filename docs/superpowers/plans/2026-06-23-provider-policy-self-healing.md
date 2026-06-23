# Provider Policy/Health Self-Healing Loop Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make the playability probe authoritative and DB-persisted so broken scraper providers auto-leave the failover chain (≤6h) and recovered ones auto-return after proving stability, and stop the maintenance bot's duplicate-escalation churn.

**Architecture:** `stream_providers` gains machine-managed `policy`(auto|manual|disabled) + `health`(up|recovering|down) + hysteresis timestamps. Catalog owns a pure `providerpolicy` state machine that applies probe verdicts (fast exclusion, slow demotion, strict gradual recovery). The analytics canary becomes cadence-tiered (UP 6h / recovering 12h / manual 24h), most-popular-first, fail-fast for recovering/down, and POSTs per-provider PASS/FAIL verdicts to catalog. Catalog derives the existing wire `status` from `(policy,health)` so the **scraper failover gate is unchanged** — the scraper only gains a `health` passthrough for the FE pill. Maintenance suppresses escalations for `policy!=auto` providers and dedups by issue.

**Tech Stack:** Go (catalog/analytics/scheduler/scraper/maintenance services, GORM, chi router, libs/cache), Vue 3 + TypeScript (frontend), Grafana JSON dashboards.

## Global Constraints

- **Spec:** `docs/superpowers/specs/2026-06-23-provider-policy-self-healing-design.md` — this plan implements it.
- **Golden rule:** all work in a git worktree off `origin/main`; never edit the base tree `/data/animeenigma` (except `.env`). Commit per task; push at phase boundaries with rebase-retry; deploy via `make redeploy-<svc>` from the worktree (copy `docker/.env` first).
- **Effort/impact metrics:** no time-units; score plans/CHANGELOG in UXΔ/CDI/MVQ (`.planning/CONVENTIONS.md`).
- **Eligibility invariant:** a provider is auto-failover-eligible IFF `policy==auto && health==up`. Encoded on the wire as `status==enabled`.
- **Wire-status derivation (single source):** `policy==disabled → "disabled"`; `policy==auto && health==up → "enabled"`; everything else registered → `"degraded"`.
- **`disabled` is the only hard admin lock** — the machine never probes or re-policies a `disabled` provider.
- **Policy machine scope:** only `scraper_operated==true` rows. Non-scraper rows (ae, kodik-*, animelib, hanime, raw) keep a fixed daily probe cadence and are never policied.
- **Go house style:** handwritten fakes in tests (no testify/mock); table-driven tests; structured logging via `libs/logger`; domain errors via `libs/errors`.
- **DS-lint:** the Recovering pill uses the **`lime`** brand hue (exempt from Rule 1; literally yellow-green) — no allowlist entry needed. i18n keys MUST land in all three locales (en/ru/ja) or the parity test fails.
- **Cadence/threshold defaults (env, catalog unless noted):** `PROBE_CADENCE_UP=6h`, `PROBE_CADENCE_RECOVERING=12h`, `PROBE_CADENCE_MANUAL=24h`, `PROVIDER_DEMOTE_AFTER=24h`, `PROVIDER_PROMOTE_AFTER=24h`, `PROBE_RECOVERING_SAMPLE=3`; scheduler `PLAYBACK_PROBE_CRON="0 */6 * * *"`.

---

## File Structure

**Catalog (owns model + state machine + endpoints):**
- `services/catalog/internal/domain/scraper_provider.go` — MODIFY: add `policy`/`health`/`*_since`/`last_probed_at` fields + `Policy`/`Health` types + `Eligible()`/`WireStatus()`/`ProbeCadence()`/`ProbeSample()` helpers.
- `services/catalog/internal/service/providerpolicy/engine.go` — CREATE: pure transition functions `ApplyHealth`/`ApplyPolicy`/`ApplyVerdict`.
- `services/catalog/internal/service/providerpolicy/engine_test.go` — CREATE.
- `services/catalog/internal/service/scraperprovider/migrate.go` — MODIFY: add guarded `BackfillPolicyHealth` migration.
- `services/catalog/internal/handler/internal_scraper_providers.go` — MODIFY: response DTO derives wire `status`, adds `policy`/`health`.
- `services/catalog/internal/handler/internal_provider_policy.go` — CREATE: `POST /internal/providers/probe-result`, `GET /internal/providers/probe-plan`.
- `services/catalog/internal/config/config.go` — MODIFY: add `ProviderPolicyConfig`.
- `services/catalog/cmd/catalog-api/main.go` + `internal/transport/router.go` — MODIFY: wire migration + handler + routes.

**Analytics (the prober):**
- `services/analytics/internal/probe/engine.go` — MODIFY: fetch probe-plan, most-popular-first, fail-fast, per-state sample, post verdict.
- `services/analytics/internal/probe/animeset.go` — MODIFY: popularity-ordered title list.
- `services/analytics/internal/probe/catalog_plan.go` — CREATE: probe-plan client + verdict poster.
- `services/analytics/internal/config/config.go` — MODIFY: catalog provider endpoints already reachable via existing catalog URL; add if missing.

**Scheduler:** `services/scheduler/internal/config/config.go` + `docker/docker-compose.yml` — MODIFY: 6h cron.

**Scraper (passthrough only):** `services/scraper/internal/config/providers_remote.go`, `internal/config/providers.go`, `internal/handler/scraper.go` — MODIFY: carry `health` through to `/health`.

**Maintenance (churn):** `services/maintenance/internal/config/config.go`, `cmd/maintenance/main.go`, `internal/state/manager.go` — MODIFY: suppress + dedup.

**Frontend:** `frontend/web/src/types/aePlayer.ts`, `composables/aePlayer/useProviderHealth.ts`, `components/player/aePlayer/SourcePanel.vue` + `ProviderChip.vue`, `components/ui/badge-variants.ts`, `locales/{en,ru,ja}.json`, plus `.spec.ts`.

**Grafana:** `docker/grafana/dashboards/playback-health.json` — panel 102.

---

# PHASE 1 — Catalog: state model + wire derivation

## Task 1: Add policy/health fields, types, and helpers to the domain model

**Files:**
- Modify: `services/catalog/internal/domain/scraper_provider.go`
- Test: `services/catalog/internal/domain/scraper_provider_test.go`

**Interfaces:**
- Produces: `ProviderPolicy`/`ProviderHealth` string types + constants (`PolicyAuto/Manual/Disabled`, `HealthUp/Recovering/Down`); methods `Eligible() bool`, `WireStatus() ProviderStatus`, `ProbeCadence(c CadenceConfig) time.Duration`, `ProbeSample(c CadenceConfig) (size int, failFast bool)`; struct `CadenceConfig{Up, Recovering, Manual time.Duration; RecoveringSample, FullSample int}`.

- [ ] **Step 1: Write the failing test**

Add to `scraper_provider_test.go`:

```go
func TestWireStatus(t *testing.T) {
	cases := []struct {
		name   string
		policy ProviderPolicy
		health ProviderHealth
		want   ProviderStatus
	}{
		{"auto+up eligible", PolicyAuto, HealthUp, StatusEnabled},
		{"auto+down failing", PolicyAuto, HealthDown, StatusDegraded},
		{"auto+recovering", PolicyAuto, HealthRecovering, StatusDegraded},
		{"manual+down", PolicyManual, HealthDown, StatusDegraded},
		{"manual+recovering", PolicyManual, HealthRecovering, StatusDegraded},
		{"disabled", PolicyDisabled, HealthDown, StatusDisabled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := ScraperProvider{Policy: c.policy, Health: c.health}
			if got := p.WireStatus(); got != c.want {
				t.Fatalf("WireStatus()=%q want %q", got, c.want)
			}
			if got := p.Eligible(); got != (c.want == StatusEnabled) {
				t.Fatalf("Eligible()=%v want %v", got, c.want == StatusEnabled)
			}
		})
	}
}

func TestProbeCadenceAndSample(t *testing.T) {
	cfg := CadenceConfig{Up: 6 * time.Hour, Recovering: 12 * time.Hour, Manual: 24 * time.Hour, RecoveringSample: 3, FullSample: 5}
	cases := []struct {
		name        string
		policy      ProviderPolicy
		health      ProviderHealth
		wantCadence time.Duration
		wantSize    int
		wantFF      bool
	}{
		{"up", PolicyAuto, HealthUp, 6 * time.Hour, 5, false},
		{"recovering", PolicyManual, HealthRecovering, 12 * time.Hour, 3, true},
		{"manual-down", PolicyManual, HealthDown, 24 * time.Hour, 1, true},
		{"failing auto-down", PolicyAuto, HealthDown, 6 * time.Hour, 5, true},
		{"disabled never", PolicyDisabled, HealthDown, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := ScraperProvider{Policy: c.policy, Health: c.health}
			if got := p.ProbeCadence(cfg); got != c.wantCadence {
				t.Fatalf("ProbeCadence()=%v want %v", got, c.wantCadence)
			}
			size, ff := p.ProbeSample(cfg)
			if size != c.wantSize || ff != c.wantFF {
				t.Fatalf("ProbeSample()=(%d,%v) want (%d,%v)", size, ff, c.wantSize, c.wantFF)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/domain/ -run 'TestWireStatus|TestProbeCadenceAndSample' -v`
Expected: FAIL — `undefined: PolicyAuto` / `p.WireStatus undefined`.

- [ ] **Step 3: Write minimal implementation**

In `scraper_provider.go`, add after the `ProviderStatus` block (keep `Status` field + helpers for migration):

```go
// ProviderPolicy is the admin/machine intent dimension. disabled is the only
// hard admin lock; auto<->manual are machine-driven by the probe state machine.
type ProviderPolicy string

const (
	PolicyAuto     ProviderPolicy = "auto"
	PolicyManual   ProviderPolicy = "manual"
	PolicyDisabled ProviderPolicy = "disabled"
)

// ProviderHealth is the probe-observed dimension.
type ProviderHealth string

const (
	HealthUp         ProviderHealth = "up"
	HealthRecovering ProviderHealth = "recovering"
	HealthDown       ProviderHealth = "down"
)

// CadenceConfig holds the tunable probe cadences + sample sizes (from env).
type CadenceConfig struct {
	Up               time.Duration
	Recovering       time.Duration
	Manual           time.Duration
	RecoveringSample int
	FullSample       int
}
```

Add the fields to the `ScraperProvider` struct (after `Status`):

```go
	// Policy/Health are the machine-managed self-healing dimensions (spec
	// 2026-06-23). Status above is DERIVED for the wire via WireStatus().
	Policy       ProviderPolicy `gorm:"size:16;default:'auto'" json:"policy"`
	Health       ProviderHealth `gorm:"size:16;default:'up'" json:"health"`
	HealthSince  time.Time      `json:"health_since"`
	PolicySince  time.Time      `json:"policy_since"`
	LastProbedAt time.Time      `json:"last_probed_at"`
```

Add the helpers at the bottom of the file:

```go
// Eligible reports auto-failover eligibility: policy auto AND health up.
func (p ScraperProvider) Eligible() bool { return p.Policy == PolicyAuto && p.Health == HealthUp }

// WireStatus derives the legacy tri-state the scraper failover gate consumes.
func (p ScraperProvider) WireStatus() ProviderStatus {
	switch p.Policy {
	case PolicyDisabled:
		return StatusDisabled
	case PolicyAuto:
		if p.Health == HealthUp {
			return StatusEnabled
		}
		return StatusDegraded
	default: // manual
		return StatusDegraded
	}
}

// ProbeCadence returns how often this provider should be probed; 0 = never.
func (p ScraperProvider) ProbeCadence(c CadenceConfig) time.Duration {
	if p.Policy == PolicyDisabled {
		return 0
	}
	switch p.Health {
	case HealthUp:
		return c.Up
	case HealthRecovering:
		return c.Recovering
	default: // down
		if p.Policy == PolicyManual {
			return c.Manual
		}
		return c.Up // auto+down (Failing): probe fast to confirm/recover
	}
}

// ProbeSample returns the title sample size + fail-fast flag for a run.
func (p ScraperProvider) ProbeSample(c CadenceConfig) (int, bool) {
	if p.Policy == PolicyDisabled {
		return 0, false
	}
	switch {
	case p.Health == HealthUp:
		return c.FullSample, false // full picture, no abort
	case p.Health == HealthRecovering:
		return c.RecoveringSample, true
	case p.Policy == PolicyManual: // manual+down
		return 1, true // cheapest "is it back?"
	default: // auto+down (Failing)
		return c.FullSample, true
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/domain/ -run 'TestWireStatus|TestProbeCadenceAndSample' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/domain/scraper_provider.go services/catalog/internal/domain/scraper_provider_test.go
git commit -m "feat(catalog): add policy/health dimensions + wire-status derivation to provider model"
```

---

## Task 2: Guarded migration — back-fill `status` → `(policy, health)`

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/migrate.go`
- Test: `services/catalog/internal/service/scraperprovider/migrate_test.go`

**Interfaces:**
- Consumes: the guarded-migration ledger pattern (`catalog_migration_guards`) already used by `AllAnimeDegrade` in this file.
- Produces: `func BackfillPolicyHealth(db *gorm.DB) error` (idempotent, guard key `backfill_policy_health_v1`).

- [ ] **Step 1: Write the failing test**

Add to `migrate_test.go` (mirror the existing in-memory/sqlite or test-db setup used by the other migrate tests in this file):

```go
func TestBackfillPolicyHealth(t *testing.T) {
	db := newTestDB(t) // existing helper in this package's tests
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	seed := []domain.ScraperProvider{
		{Name: "gogoanime", Status: domain.StatusEnabled, UpdatedAt: now},
		{Name: "allanime", Status: domain.StatusDegraded, UpdatedAt: now},
		{Name: "deadguy", Status: domain.StatusDisabled, UpdatedAt: now},
	}
	if err := db.Create(&seed).Error; err != nil {
		t.Fatal(err)
	}

	if err := BackfillPolicyHealth(db); err != nil {
		t.Fatalf("backfill: %v", err)
	}

	want := map[string][2]string{
		"gogoanime": {"auto", "up"},
		"allanime":  {"manual", "down"},
		"deadguy":   {"disabled", "down"},
	}
	for name, exp := range want {
		var p domain.ScraperProvider
		db.First(&p, "name = ?", name)
		if string(p.Policy) != exp[0] || string(p.Health) != exp[1] {
			t.Fatalf("%s: got (%s,%s) want (%s,%s)", name, p.Policy, p.Health, exp[0], exp[1])
		}
		if p.HealthSince.IsZero() || p.PolicySince.IsZero() {
			t.Fatalf("%s: timestamps not set", name)
		}
	}

	// Idempotent: second run is a no-op (guard).
	db.Model(&domain.ScraperProvider{}).Where("name = ?", "gogoanime").Update("policy", "manual")
	if err := BackfillPolicyHealth(db); err != nil {
		t.Fatal(err)
	}
	var g domain.ScraperProvider
	db.First(&g, "name = ?", "gogoanime")
	if g.Policy != "manual" {
		t.Fatalf("guard failed: backfill re-ran and clobbered operator edit")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestBackfillPolicyHealth -v`
Expected: FAIL — `undefined: BackfillPolicyHealth`.

- [ ] **Step 3: Write minimal implementation**

Add to `migrate.go` (follow the exact guard pattern of `AllAnimeDegrade` in the same file):

```go
const guardBackfillPolicyHealth = "backfill_policy_health_v1"

// BackfillPolicyHealth maps the legacy status tri-state onto the new
// (policy, health) dimensions exactly once. Guarded so it never clobbers
// later machine/operator writes on reboot.
func BackfillPolicyHealth(db *gorm.DB) error {
	if err := db.AutoMigrate(&migrationGuard{}); err != nil {
		return err
	}
	var guards int64
	db.Model(&migrationGuard{}).Where("key = ?", guardBackfillPolicyHealth).Count(&guards)
	if guards > 0 {
		return nil
	}
	now := time.Now().UTC()
	// enabled -> auto/up ; degraded -> manual/down ; disabled -> disabled/down
	if err := db.Model(&domain.ScraperProvider{}).Where("status = ?", domain.StatusEnabled).
		Updates(map[string]any{"policy": "auto", "health": "up", "health_since": now, "policy_since": now}).Error; err != nil {
		return err
	}
	if err := db.Model(&domain.ScraperProvider{}).Where("status = ?", domain.StatusDegraded).
		Updates(map[string]any{"policy": "manual", "health": "down", "health_since": now, "policy_since": now}).Error; err != nil {
		return err
	}
	if err := db.Model(&domain.ScraperProvider{}).Where("status = ?", domain.StatusDisabled).
		Updates(map[string]any{"policy": "disabled", "health": "down", "health_since": now, "policy_since": now}).Error; err != nil {
		return err
	}
	return db.Create(&migrationGuard{Key: guardBackfillPolicyHealth}).Error
}
```

> If the guard type is named differently in this file (e.g. `MigrationGuard`), match the existing name. Confirm by reading `AllAnimeDegrade`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/scraperprovider/ -run TestBackfillPolicyHealth -v`
Expected: PASS.

- [ ] **Step 5: Wire into boot + commit**

In `services/catalog/cmd/catalog-api/main.go`, after the `AutoMigrate(... &domain.ScraperProvider{} ...)` block and alongside the existing `AllAnimeDegrade`/`AnimefeverDeclaim` calls (~line 236-246), add:

```go
	if err := scraperprovider.BackfillPolicyHealth(db.DB); err != nil {
		log.Errorw("policy/health backfill failed (continuing)", "error", err)
	}
```

```bash
git add services/catalog/internal/service/scraperprovider/migrate.go services/catalog/internal/service/scraperprovider/migrate_test.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): guarded backfill of status -> (policy,health) + boot wiring"
```

---

## Task 3: Derive wire `status` + expose `policy`/`health` in `/internal/scraper/providers`

**Files:**
- Modify: `services/catalog/internal/handler/internal_scraper_providers.go`
- Test: `services/catalog/internal/handler/internal_scraper_providers_test.go` (create if absent)

**Interfaces:**
- Produces: the JSON each provider row emits now has `status` = `WireStatus()` (not the stored column) plus additive `policy` + `health` fields. All other fields unchanged.

- [ ] **Step 1: Write the failing test**

```go
func TestList_DerivesWireStatus(t *testing.T) {
	db := newHandlerTestDB(t) // existing helper or inline sqlite
	db.AutoMigrate(&domain.ScraperProvider{})
	db.Create(&[]domain.ScraperProvider{
		{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, ScraperOperated: true},
		{Name: "allanime", Policy: domain.PolicyManual, Health: domain.HealthRecovering, ScraperOperated: true},
	})
	h := NewInternalScraperProvidersHandler(db, testLogger())
	rr := httptest.NewRecorder()
	h.List(rr, httptest.NewRequest("GET", "/internal/scraper/providers", nil))

	var body struct {
		Data struct {
			Providers []map[string]any `json:"providers"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &body)
	got := map[string]map[string]any{}
	for _, p := range body.Data.Providers {
		got[p["name"].(string)] = p
	}
	if got["gogoanime"]["status"] != "enabled" {
		t.Fatalf("gogoanime status=%v want enabled", got["gogoanime"]["status"])
	}
	if got["allanime"]["status"] != "degraded" {
		t.Fatalf("allanime status=%v want degraded", got["allanime"]["status"])
	}
	if got["allanime"]["health"] != "recovering" || got["allanime"]["policy"] != "manual" {
		t.Fatalf("allanime missing policy/health: %+v", got["allanime"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestList_DerivesWireStatus -v`
Expected: FAIL — `status` is the stored value (likely empty) and `policy`/`health` keys behavior unverified.

- [ ] **Step 3: Write minimal implementation**

In `internal_scraper_providers.go`, replace the direct model marshalling in `List` with a DTO that derives status:

```go
type providerWire struct {
	Name             string `json:"name"`
	Status           string `json:"status"` // DERIVED via WireStatus()
	Policy           string `json:"policy"`
	Health           string `json:"health"`
	Group            string `json:"group"`
	Reason           string `json:"reason"`
	Description      string `json:"description"`
	ScraperOperated  bool   `json:"scraper_operated"`
	SupportsSub      bool   `json:"supports_sub"`
	SupportsDub      bool   `json:"supports_dub"`
	SupportsRaw      bool   `json:"supports_raw"`
	SubDelivery      string `json:"sub_delivery"`
	QualityCeiling   string `json:"quality_ceiling"`
	PreferenceWeight int    `json:"preference_weight"`
	Engine           string `json:"engine"`
	BaseURL          string `json:"base_url"`
}

func toWire(p domain.ScraperProvider) providerWire {
	return providerWire{
		Name: p.Name, Status: string(p.WireStatus()), Policy: string(p.Policy), Health: string(p.Health),
		Group: p.Group, Reason: p.Reason, Description: p.Description, ScraperOperated: p.ScraperOperated,
		SupportsSub: p.SupportsSub, SupportsDub: p.SupportsDub, SupportsRaw: p.SupportsRaw,
		SubDelivery: p.SubDelivery, QualityCeiling: p.QualityCeiling, PreferenceWeight: p.PreferenceWeight,
		Engine: p.Engine, BaseURL: p.BaseURL,
	}
}
```

Map rows through `toWire` before encoding the `{providers:[...]}` envelope.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/handler/ -run TestList_DerivesWireStatus -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/handler/internal_scraper_providers.go services/catalog/internal/handler/internal_scraper_providers_test.go
git commit -m "feat(catalog): derive wire status from policy/health + expose both in /internal/scraper/providers"
```

> **Phase 1 push:** `git fetch origin && git rebase origin/main && git push origin HEAD:main` (rebase-retry). Phase 1 ships a backward-compatible DB+wire change: scraper sees identical `status` semantics; nothing drives transitions yet.

---

# PHASE 2 — Catalog: the state machine + endpoints

## Task 4: providerpolicy engine — health transitions (pure)

**Files:**
- Create: `services/catalog/internal/service/providerpolicy/engine.go`
- Test: `services/catalog/internal/service/providerpolicy/engine_test.go`

**Interfaces:**
- Produces: `func ApplyHealth(p *domain.ScraperProvider, pass bool, now time.Time, promoteAfter time.Duration)` — mutates `Health`+`HealthSince` per the spec's health rules.

- [ ] **Step 1: Write the failing test**

```go
package providerpolicy

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestApplyHealth(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		name        string
		start       domain.ProviderHealth
		since       time.Time
		pass        bool
		now         time.Time
		wantHealth  domain.ProviderHealth
		sinceMoved  bool
	}{
		{"down->recovering on pass", domain.HealthDown, t0, true, t0.Add(day), domain.HealthRecovering, true},
		{"recovering stays before promote window", domain.HealthRecovering, t0, true, t0.Add(day - time.Minute), domain.HealthRecovering, false},
		{"recovering->up after promote window", domain.HealthRecovering, t0, true, t0.Add(day), domain.HealthUp, true},
		{"recovering->down on fail", domain.HealthRecovering, t0, false, t0.Add(time.Hour), domain.HealthDown, true},
		{"up->down on fail", domain.HealthUp, t0, false, t0.Add(time.Hour), domain.HealthDown, true},
		{"up stays on pass", domain.HealthUp, t0, true, t0.Add(time.Hour), domain.HealthUp, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := &domain.ScraperProvider{Health: c.start, HealthSince: c.since}
			ApplyHealth(p, c.pass, c.now, day)
			if p.Health != c.wantHealth {
				t.Fatalf("Health=%s want %s", p.Health, c.wantHealth)
			}
			moved := !p.HealthSince.Equal(c.since)
			if moved != c.sinceMoved {
				t.Fatalf("HealthSince moved=%v want %v", moved, c.sinceMoved)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/providerpolicy/ -run TestApplyHealth -v`
Expected: FAIL — package/func undefined.

- [ ] **Step 3: Write minimal implementation**

```go
package providerpolicy

import (
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// ApplyHealth advances a provider's health from a probe pass/fail.
// down --pass--> recovering ; recovering --pass after promoteAfter--> up ;
// any fail --> down. HealthSince is reset only on a real state change.
func ApplyHealth(p *domain.ScraperProvider, pass bool, now time.Time, promoteAfter time.Duration) {
	prev := p.Health
	if !pass {
		p.Health = domain.HealthDown
	} else {
		switch p.Health {
		case domain.HealthDown:
			p.Health = domain.HealthRecovering
		case domain.HealthRecovering:
			if now.Sub(p.HealthSince) >= promoteAfter {
				p.Health = domain.HealthUp
			}
		case domain.HealthUp:
			// stay
		default: // unseeded
			p.Health = domain.HealthRecovering
		}
	}
	if p.Health != prev {
		p.HealthSince = now
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/providerpolicy/ -run TestApplyHealth -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/providerpolicy/engine.go services/catalog/internal/service/providerpolicy/engine_test.go
git commit -m "feat(catalog): providerpolicy health transition engine"
```

---

## Task 5: providerpolicy engine — policy transitions + `ApplyVerdict`

**Files:**
- Modify: `services/catalog/internal/service/providerpolicy/engine.go`
- Test: `services/catalog/internal/service/providerpolicy/engine_test.go`

**Interfaces:**
- Produces: `func ApplyPolicy(p *domain.ScraperProvider, now time.Time, demoteAfter time.Duration)`; `func ApplyVerdict(p *domain.ScraperProvider, pass bool, now time.Time, demoteAfter, promoteAfter time.Duration)` — calls ApplyHealth then ApplyPolicy then sets `LastProbedAt=now`.

- [ ] **Step 1: Write the failing test**

```go
func TestApplyPolicy(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	t.Run("auto+down demotes after window", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthDown, HealthSince: t0}
		ApplyPolicy(p, t0.Add(day), day)
		if p.Policy != domain.PolicyManual {
			t.Fatalf("policy=%s want manual", p.Policy)
		}
	})
	t.Run("auto+down stays before window", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthDown, HealthSince: t0}
		ApplyPolicy(p, t0.Add(day-time.Minute), day)
		if p.Policy != domain.PolicyAuto {
			t.Fatalf("policy=%s want auto", p.Policy)
		}
	})
	t.Run("manual+up promotes", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyManual, Health: domain.HealthUp, HealthSince: t0}
		ApplyPolicy(p, t0, day)
		if p.Policy != domain.PolicyAuto {
			t.Fatalf("policy=%s want auto", p.Policy)
		}
	})
	t.Run("disabled immune", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyDisabled, Health: domain.HealthDown, HealthSince: t0}
		ApplyPolicy(p, t0.Add(10*day), day)
		if p.Policy != domain.PolicyDisabled {
			t.Fatalf("policy=%s want disabled", p.Policy)
		}
	})
}

func TestApplyVerdict_FullDemoteThenRecover(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	p := &domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthUp, HealthSince: t0}

	ApplyVerdict(p, false, t0.Add(time.Hour), day, day) // first fail -> down, still auto
	if p.Health != domain.HealthDown || p.Policy != domain.PolicyAuto || !p.Eligible() == false {
		t.Fatalf("after first fail: %+v", p)
	}
	ApplyVerdict(p, false, t0.Add(time.Hour+day), day, day) // down >1d -> demote manual
	if p.Policy != domain.PolicyManual {
		t.Fatalf("expected demote, got %s", p.Policy)
	}
	ApplyVerdict(p, true, t0.Add(2*day), day, day) // manual probe passes -> recovering
	if p.Health != domain.HealthRecovering {
		t.Fatalf("expected recovering, got %s", p.Health)
	}
	ApplyVerdict(p, true, t0.Add(3*day+time.Minute), day, day) // recovering >1d -> up + promote
	if p.Health != domain.HealthUp || p.Policy != domain.PolicyAuto || !p.Eligible() {
		t.Fatalf("expected promoted+eligible, got %+v", p)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/service/providerpolicy/ -run 'TestApplyPolicy|TestApplyVerdict' -v`
Expected: FAIL — `ApplyPolicy`/`ApplyVerdict` undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `engine.go`:

```go
// ApplyPolicy advances policy from sustained health. disabled is immune.
func ApplyPolicy(p *domain.ScraperProvider, now time.Time, demoteAfter time.Duration) {
	switch p.Policy {
	case domain.PolicyAuto:
		if p.Health == domain.HealthDown && now.Sub(p.HealthSince) >= demoteAfter {
			p.Policy = domain.PolicyManual
			p.PolicySince = now
		}
	case domain.PolicyManual:
		if p.Health == domain.HealthUp {
			p.Policy = domain.PolicyAuto
			p.PolicySince = now
		}
	}
}

// ApplyVerdict is the full per-probe transition: health, then policy, then stamp.
func ApplyVerdict(p *domain.ScraperProvider, pass bool, now time.Time, demoteAfter, promoteAfter time.Duration) {
	ApplyHealth(p, pass, now, promoteAfter)
	ApplyPolicy(p, now, demoteAfter)
	p.LastProbedAt = now
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/service/providerpolicy/ -v`
Expected: PASS (all engine tests).

- [ ] **Step 5: Commit**

```bash
git add services/catalog/internal/service/providerpolicy/engine.go services/catalog/internal/service/providerpolicy/engine_test.go
git commit -m "feat(catalog): providerpolicy policy transitions + ApplyVerdict full step"
```

---

## Task 6: Config + `POST /internal/providers/probe-result` endpoint

**Files:**
- Modify: `services/catalog/internal/config/config.go`
- Create: `services/catalog/internal/handler/internal_provider_policy.go`
- Test: `services/catalog/internal/handler/internal_provider_policy_test.go`
- Modify: `services/catalog/internal/transport/router.go`, `services/catalog/cmd/catalog-api/main.go`

**Interfaces:**
- Consumes: `domain.CadenceConfig`, `providerpolicy.ApplyVerdict`.
- Produces: `ProviderPolicyConfig{Cadence domain.CadenceConfig; DemoteAfter, PromoteAfter time.Duration}` in catalog config; HTTP `POST /internal/providers/probe-result` body `{"provider":"gogoanime","pass":false,"reason":"status_403"}` → loads row, `ApplyVerdict`, persists, returns `{success:true,data:{provider,policy,health}}`.

- [ ] **Step 1: Write the failing test**

```go
func TestProbeResult_FlipsRow(t *testing.T) {
	db := newHandlerTestDB(t)
	db.AutoMigrate(&domain.ScraperProvider{})
	t0 := time.Now().Add(-48 * time.Hour)
	db.Create(&domain.ScraperProvider{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthDown, HealthSince: t0, ScraperOperated: true})

	h := NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"gogoanime","pass":false,"reason":"status_403"}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var p domain.ScraperProvider
	db.First(&p, "name = ?", "gogoanime")
	if p.Policy != domain.PolicyManual { // down >24h + fail -> demote
		t.Fatalf("policy=%s want manual", p.Policy)
	}
	if p.LastProbedAt.IsZero() {
		t.Fatal("last_probed_at not stamped")
	}
}
```

(Add a small `testProviderPolicyCfg()` returning `ProviderPolicyConfig` with 24h/24h + cadence defaults.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestProbeResult_FlipsRow -v`
Expected: FAIL — handler undefined.

- [ ] **Step 3: Write minimal implementation**

Add to `config.go` (using `getEnvDuration`/`getEnvInt`):

```go
type ProviderPolicyConfig struct {
	Cadence      domain.CadenceConfig
	DemoteAfter  time.Duration
	PromoteAfter time.Duration
}
```
In `Load()`:
```go
		ProviderPolicy: ProviderPolicyConfig{
			Cadence: domain.CadenceConfig{
				Up:               getEnvDuration("PROBE_CADENCE_UP", 6*time.Hour),
				Recovering:       getEnvDuration("PROBE_CADENCE_RECOVERING", 12*time.Hour),
				Manual:           getEnvDuration("PROBE_CADENCE_MANUAL", 24*time.Hour),
				RecoveringSample: getEnvInt("PROBE_RECOVERING_SAMPLE", 3),
				FullSample:       getEnvInt("PROBE_FULL_SAMPLE", 5),
			},
			DemoteAfter:  getEnvDuration("PROVIDER_DEMOTE_AFTER", 24*time.Hour),
			PromoteAfter: getEnvDuration("PROVIDER_PROMOTE_AFTER", 24*time.Hour),
		},
```
(Add `ProviderPolicy ProviderPolicyConfig` to the `Config` struct; import `domain`.)

Create `internal_provider_policy.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/providerpolicy"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

type InternalProviderPolicyHandler struct {
	db  *gorm.DB
	cfg config.ProviderPolicyConfig
	log *logger.Logger
}

func NewInternalProviderPolicyHandler(db *gorm.DB, cfg config.ProviderPolicyConfig, log *logger.Logger) *InternalProviderPolicyHandler {
	return &InternalProviderPolicyHandler{db: db, cfg: cfg, log: log}
}

type probeResultReq struct {
	Provider string `json:"provider"`
	Pass     bool   `json:"pass"`
	Reason   string `json:"reason"`
}

func (h *InternalProviderPolicyHandler) ProbeResult(w http.ResponseWriter, r *http.Request) {
	var req probeResultReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Provider == "" {
		http.Error(w, `{"success":false,"error":"bad request"}`, http.StatusBadRequest)
		return
	}
	var p domain.ScraperProvider
	if err := h.db.First(&p, "name = ?", req.Provider).Error; err != nil {
		http.Error(w, `{"success":false,"error":"unknown provider"}`, http.StatusNotFound)
		return
	}
	if p.Policy == domain.PolicyDisabled || !p.ScraperOperated {
		// disabled is the hard lock; non-scraper rows are not policied.
		writeJSON(w, map[string]any{"success": true, "data": map[string]any{"provider": p.Name, "policy": p.Policy, "health": p.Health, "skipped": true}})
		return
	}
	now := time.Now().UTC()
	if req.Reason != "" {
		p.Reason = req.Reason
	}
	providerpolicy.ApplyVerdict(&p, req.Pass, now, h.cfg.DemoteAfter, h.cfg.PromoteAfter)
	if err := h.db.Model(&domain.ScraperProvider{}).Where("name = ?", p.Name).
		Updates(map[string]any{"policy": p.Policy, "health": p.Health, "health_since": p.HealthSince, "policy_since": p.PolicySince, "last_probed_at": p.LastProbedAt, "reason": p.Reason}).Error; err != nil {
		http.Error(w, `{"success":false,"error":"persist failed"}`, http.StatusInternalServerError)
		return
	}
	writeJSON(w, map[string]any{"success": true, "data": map[string]any{"provider": p.Name, "policy": p.Policy, "health": p.Health}})
}
```

(`writeJSON` — reuse the package's existing JSON helper; if none, inline `w.Header().Set("Content-Type","application/json"); json.NewEncoder(w).Encode(v)`.)

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/handler/ -run TestProbeResult_FlipsRow -v`
Expected: PASS.

- [ ] **Step 5: Wire route + DI + commit**

In `main.go` (near the other internal handlers ~line 429): `internalProviderPolicyHandler := handler.NewInternalProviderPolicyHandler(db.DB, cfg.ProviderPolicy, log)` and pass to the router. In `router.go` (near line 94): `r.Post("/internal/providers/probe-result", internalProviderPolicyHandler.ProbeResult)`.

```bash
git add services/catalog/internal/config/config.go services/catalog/internal/handler/internal_provider_policy.go services/catalog/internal/handler/internal_provider_policy_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): probe-result endpoint applies verdict + persists policy/health"
```

---

## Task 7: `GET /internal/providers/probe-plan` (due-set + sample/fail-fast)

**Files:**
- Modify: `services/catalog/internal/handler/internal_provider_policy.go`
- Test: `services/catalog/internal/handler/internal_provider_policy_test.go`
- Modify: `services/catalog/internal/transport/router.go`

**Interfaces:**
- Produces: `GET /internal/providers/probe-plan` → `{success:true,data:{plan:[{"provider":"gogoanime","sample_size":5,"fail_fast":false}]}}`. A provider is in the plan iff `now - last_probed_at >= ProbeCadence(state)`; `scraper_operated` providers use the state cadence; non-scraper providers use a fixed 24h cadence with `sample_size:1, fail_fast:true`; `disabled` providers are excluded.

- [ ] **Step 1: Write the failing test**

```go
func TestProbePlan_DueSet(t *testing.T) {
	db := newHandlerTestDB(t)
	db.AutoMigrate(&domain.ScraperProvider{})
	now := time.Now().UTC()
	db.Create(&[]domain.ScraperProvider{
		{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, LastProbedAt: now.Add(-7 * time.Hour), ScraperOperated: true},   // due (6h)
		{Name: "miruro", Policy: domain.PolicyAuto, Health: domain.HealthUp, LastProbedAt: now.Add(-1 * time.Hour), ScraperOperated: true},       // not due
		{Name: "allanime", Policy: domain.PolicyManual, Health: domain.HealthDown, LastProbedAt: now.Add(-25 * time.Hour), ScraperOperated: true}, // due (24h), sample 1 ff
		{Name: "deadguy", Policy: domain.PolicyDisabled, Health: domain.HealthDown, LastProbedAt: now.Add(-99 * time.Hour), ScraperOperated: true}, // excluded
	})
	h := NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testLogger())
	rr := httptest.NewRecorder()
	h.ProbePlan(rr, httptest.NewRequest("GET", "/internal/providers/probe-plan", nil))

	var body struct {
		Data struct {
			Plan []struct {
				Provider   string `json:"provider"`
				SampleSize int    `json:"sample_size"`
				FailFast   bool   `json:"fail_fast"`
			} `json:"plan"`
		} `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &body)
	got := map[string]struct{ size int; ff bool }{}
	for _, e := range body.Data.Plan {
		got[e.Provider] = struct{ size int; ff bool }{e.SampleSize, e.FailFast}
	}
	if _, ok := got["miruro"]; ok {
		t.Fatal("miruro not due — should be absent")
	}
	if _, ok := got["deadguy"]; ok {
		t.Fatal("disabled must be excluded")
	}
	if got["gogoanime"] != (struct{ size int; ff bool }{5, false}) {
		t.Fatalf("gogoanime plan=%+v want {5,false}", got["gogoanime"])
	}
	if got["allanime"] != (struct{ size int; ff bool }{1, true}) {
		t.Fatalf("allanime plan=%+v want {1,true}", got["allanime"])
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/catalog && go test ./internal/handler/ -run TestProbePlan_DueSet -v`
Expected: FAIL — `ProbePlan` undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `internal_provider_policy.go`:

```go
type probePlanEntry struct {
	Provider   string `json:"provider"`
	SampleSize int    `json:"sample_size"`
	FailFast   bool   `json:"fail_fast"`
}

func (h *InternalProviderPolicyHandler) ProbePlan(w http.ResponseWriter, r *http.Request) {
	var rows []domain.ScraperProvider
	if err := h.db.Order("name asc").Find(&rows).Error; err != nil {
		http.Error(w, `{"success":false,"error":"db"}`, http.StatusInternalServerError)
		return
	}
	now := time.Now().UTC()
	plan := make([]probePlanEntry, 0, len(rows))
	for _, p := range rows {
		if p.Policy == domain.PolicyDisabled {
			continue
		}
		var cadence time.Duration
		var size int
		var ff bool
		if p.ScraperOperated {
			cadence = p.ProbeCadence(h.cfg.Cadence)
			size, ff = p.ProbeSample(h.cfg.Cadence)
		} else {
			cadence = h.cfg.Cadence.Manual // non-scraper: fixed daily
			size, ff = 1, true
		}
		if cadence <= 0 || now.Sub(p.LastProbedAt) < cadence {
			continue
		}
		plan = append(plan, probePlanEntry{Provider: p.Name, SampleSize: size, FailFast: ff})
	}
	writeJSON(w, map[string]any{"success": true, "data": map[string]any{"plan": plan}})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/catalog && go test ./internal/handler/ -run TestProbePlan_DueSet -v`
Expected: PASS.

- [ ] **Step 5: Route + commit**

In `router.go`: `r.Get("/internal/providers/probe-plan", internalProviderPolicyHandler.ProbePlan)`.

```bash
git add services/catalog/internal/handler/internal_provider_policy.go services/catalog/internal/handler/internal_provider_policy_test.go services/catalog/internal/transport/router.go
git commit -m "feat(catalog): probe-plan endpoint computes cadence-gated due-set"
```

> **Phase 2 push + deploy:** rebase-retry push; `make redeploy-catalog` from the worktree (copy `docker/.env` first). The state machine is live but inert until the prober calls it (Phase 3).

---

# PHASE 3 — Analytics: cadence-tiered, fail-fast, popularity-ordered prober

## Task 8: Order probe titles most-popular-first

**Files:**
- Modify: `services/analytics/internal/probe/animeset.go` (or `engine.go` where the per-provider title list is assembled)
- Test: `services/analytics/internal/probe/animeset_test.go`

**Interfaces:**
- Consumes: each candidate title carries an anime `Score`/popularity (fetch from catalog popular pool — already used for re-rolls).
- Produces: the assembled title slice is sorted by descending popularity before probing.

- [ ] **Step 1: Write the failing test**

```go
func TestTitlesPopularityOrdered(t *testing.T) {
	in := []ProbeTitle{
		{UUID: "a", Score: 6.1},
		{UUID: "b", Score: 8.9},
		{UUID: "c", Score: 7.5},
	}
	out := sortByPopularity(in)
	if out[0].UUID != "b" || out[1].UUID != "c" || out[2].UUID != "a" {
		t.Fatalf("order=%v want b,c,a", []string{out[0].UUID, out[1].UUID, out[2].UUID})
	}
}
```

(If the title type lacks a `Score` field, add it to the probe-title struct and populate it where the anime set is built — the catalog spotlight/popular responses include `score`.)

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/analytics && go test ./internal/probe/ -run TestTitlesPopularityOrdered -v`
Expected: FAIL — `sortByPopularity` / `Score` undefined.

- [ ] **Step 3: Write minimal implementation**

```go
import "sort"

func sortByPopularity(ts []ProbeTitle) []ProbeTitle {
	out := make([]ProbeTitle, len(ts))
	copy(out, ts)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Score > out[j].Score })
	return out
}
```

Call `sortByPopularity(...)` on the per-provider title list right before the probe loop in `engine.go`.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/analytics && go test ./internal/probe/ -run TestTitlesPopularityOrdered -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/analytics/internal/probe/animeset.go services/analytics/internal/probe/animeset_test.go services/analytics/internal/probe/engine.go
git commit -m "feat(analytics): probe titles ordered most-popular-first"
```

---

## Task 9: Probe-plan client, fail-fast sampling, and verdict computation

**Files:**
- Create: `services/analytics/internal/probe/catalog_plan.go`
- Test: `services/analytics/internal/probe/catalog_plan_test.go`
- Modify: `services/analytics/internal/probe/engine.go`

**Interfaces:**
- Consumes: catalog `GET /internal/providers/probe-plan` and `POST /internal/providers/probe-result`.
- Produces: `type PlanEntry struct{ Provider string; SampleSize int; FailFast bool }`; `func FetchPlan(ctx, catalogURL, client) ([]PlanEntry, error)`; `func PostVerdict(ctx, catalogURL, client, provider string, pass bool, reason string) error`; engine probes per-plan with fail-fast and computes per-provider `pass`.

- [ ] **Step 1: Write the failing test**

```go
func TestFetchPlanAndPostVerdict(t *testing.T) {
	var posted struct {
		Provider string `json:"provider"`
		Pass     bool   `json:"pass"`
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/internal/providers/probe-plan":
			w.Write([]byte(`{"success":true,"data":{"plan":[{"provider":"gogoanime","sample_size":3,"fail_fast":true}]}}`))
		case "/internal/providers/probe-result":
			json.NewDecoder(r.Body).Decode(&posted)
			w.Write([]byte(`{"success":true}`))
		}
	}))
	defer srv.Close()

	plan, err := FetchPlan(context.Background(), srv.URL, srv.Client())
	if err != nil || len(plan) != 1 || plan[0].Provider != "gogoanime" || plan[0].SampleSize != 3 || !plan[0].FailFast {
		t.Fatalf("plan=%+v err=%v", plan, err)
	}
	if err := PostVerdict(context.Background(), srv.URL, srv.Client(), "gogoanime", false, "status_403"); err != nil {
		t.Fatal(err)
	}
	if posted.Provider != "gogoanime" || posted.Pass != false {
		t.Fatalf("posted=%+v", posted)
	}
}

func TestProbeProviderFailFast(t *testing.T) {
	// titles[0] passes, titles[1] fails -> with fail_fast, probe stops; verdict=fail; titles[2] not tried.
	titles := []ProbeTitle{{UUID: "pop1"}, {UUID: "pop2"}, {UUID: "pop3"}}
	calls := 0
	probeOne := func(provider string, tt ProbeTitle) bool { // fake validator
		calls++
		return tt.UUID != "pop2"
	}
	pass, tried := probeProvider("gogoanime", titles, 3, true, probeOne)
	if pass {
		t.Fatal("want fail (pop2 failed)")
	}
	if tried != 2 {
		t.Fatalf("tried=%d want 2 (fail-fast abort before pop3)", tried)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/analytics && go test ./internal/probe/ -run 'TestFetchPlan|TestProbeProviderFailFast' -v`
Expected: FAIL — symbols undefined.

- [ ] **Step 3: Write minimal implementation**

Create `catalog_plan.go`:

```go
package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

type PlanEntry struct {
	Provider   string `json:"provider"`
	SampleSize int    `json:"sample_size"`
	FailFast   bool   `json:"fail_fast"`
}

func FetchPlan(ctx context.Context, catalogURL string, c *http.Client) ([]PlanEntry, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, catalogURL+"/internal/providers/probe-plan", nil)
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	var body struct {
		Data struct {
			Plan []PlanEntry `json:"plan"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Data.Plan, nil
}

func PostVerdict(ctx context.Context, catalogURL string, c *http.Client, provider string, pass bool, reason string) error {
	payload, _ := json.Marshal(map[string]any{"provider": provider, "pass": pass, "reason": reason})
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, catalogURL+"/internal/providers/probe-result", bytes.NewReader(payload))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("probe-result status %d", resp.StatusCode)
	}
	return nil
}

// probeProvider probes titles most-popular-first up to sampleSize. With
// failFast it aborts on the first failed title (verdict=fail). Without
// failFast (UP runs) it probes all and the verdict is the FIRST (most
// popular) title's result. Returns (pass, titlesTried).
func probeProvider(provider string, titles []ProbeTitle, sampleSize int, failFast bool, probeOne func(string, ProbeTitle) bool) (bool, int) {
	n := sampleSize
	if n > len(titles) || n <= 0 {
		n = len(titles)
	}
	topPass := true
	tried := 0
	for i := 0; i < n; i++ {
		ok := probeOne(provider, titles[i])
		tried++
		if i == 0 {
			topPass = ok
		}
		if !ok && failFast {
			return false, tried
		}
	}
	if failFast {
		return true, tried // reached end with no failure
	}
	return topPass, tried // UP: verdict is the most-popular title's result
}
```

In `engine.go` `RunOnce`: fetch the plan, and for each `PlanEntry` resolve+order titles (Task 8), call `probeProvider(...)` wiring `probeOne` to the existing resolve→validate pipeline (returns true on `StagePlayback`/playable), still writing per-title rows to ClickHouse for `not_tried` titles record `Stage="not_tried"` (skip writing or write a sentinel row — match existing row-write shape), then `PostVerdict`. Providers absent from the plan are skipped this tick.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/analytics && go test ./internal/probe/ -run 'TestFetchPlan|TestProbeProviderFailFast' -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add services/analytics/internal/probe/catalog_plan.go services/analytics/internal/probe/catalog_plan_test.go services/analytics/internal/probe/engine.go
git commit -m "feat(analytics): cadence-plan-driven fail-fast probe + verdict POST to catalog"
```

> **Phase 3 push + deploy:** rebase-retry; `make redeploy-analytics`. The loop is now live end-to-end on the existing daily tick. Phase 4 widens the cadence.

---

# PHASE 4 — Scheduler: 6h base tick

## Task 10: Raise the playback-probe cron to every 6h

**Files:**
- Modify: `services/scheduler/internal/config/config.go:146`
- Modify: `services/scheduler/internal/config/config_test.go:17`
- Modify: `docker/docker-compose.yml` (scheduler env)
- Test: existing `config_test.go`

- [ ] **Step 1: Update the test to the new default**

In `config_test.go`, change the expected default:

```go
	if got, want := cfg.Jobs.PlaybackProbeCron, "0 */6 * * *"; got != want {
		t.Fatalf("PlaybackProbeCron=%q want %q", got, want)
	}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/scheduler && go test ./internal/config/ -run TestLoad_PlaybackProbeDefaults -v`
Expected: FAIL — still `"0 3 * * *"`.

- [ ] **Step 3: Change the default**

`config.go:146`: `PlaybackProbeCron: getEnv("PLAYBACK_PROBE_CRON", "0 */6 * * *"),`
`docker-compose.yml`: under the scheduler service env set `PLAYBACK_PROBE_CRON: "0 */6 * * *"` (and remove/ignore the stale `SCRAPER_PLAYABILITY_CANARY_CRON` if still present on scheduler).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/scheduler && go test ./internal/config/ -v`
Expected: PASS.

- [ ] **Step 5: Commit + deploy**

```bash
git add services/scheduler/internal/config/config.go services/scheduler/internal/config/config_test.go docker/docker-compose.yml
git commit -m "feat(scheduler): raise playback probe to every 6h (policy-machine base tick)"
```
Deploy: `make redeploy-scheduler`.

---

# PHASE 5 — Scraper: health passthrough (no failover change)

## Task 11: Carry `health` through to the scraper `/health` response

**Files:**
- Modify: `services/scraper/internal/config/providers_remote.go` (remoteProvider + statusOf unchanged), `services/scraper/internal/config/providers.go` (ProviderMeta), `services/scraper/internal/handler/scraper.go` (providerEnriched)
- Test: `services/scraper/internal/handler/scraper_test.go`

**Interfaces:**
- Produces: scraper `/anime/_/scraper/health` per-provider object gains `health` (`up|recovering|down`). Failover gate stays on the existing `status`/degraded logic — NO change to `orderedProviders`/`ApplyStatuses`.

- [ ] **Step 1: Write the failing test**

Add to `scraper_test.go` (extend whatever asserts GetHealth output):

```go
func TestGetHealth_IncludesHealthField(t *testing.T) {
	cfg := config.ProvidersConfigWith([]config.ProviderMeta{
		{Name: "gogoanime", Status: config.StatusEnabled, Health: "recovering"},
	}) // use the package's existing test constructor
	h := NewScraperHandler(/* deps */).WithProvidersConfig(&cfg)
	rr := httptest.NewRecorder()
	h.GetHealth(rr, httptest.NewRequest("GET", "/health", nil))
	if !strings.Contains(rr.Body.String(), `"health":"recovering"`) {
		t.Fatalf("health field missing: %s", rr.Body.String())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/scraper && go test ./internal/handler/ -run TestGetHealth_IncludesHealthField -v`
Expected: FAIL — no `Health` field.

- [ ] **Step 3: Write minimal implementation**

- `providers_remote.go`: add `Health string \`json:"health"\`` to `remoteProvider`; carry it into `ProviderMeta` when building entries in `LoadProvidersRemote` (alongside Status/Reason/etc.).
- `providers.go`: add `Health string` to `ProviderMeta`; add `func (p ProvidersConfig) Health(name string) string { return p.Meta(name).Health }`.
- `scraper.go`: add `Health string \`json:"health,omitempty"\`` to `providerEnriched`; in `GetHealth` set `entry.Health = h.providersCfg.Health(prov)` next to the existing `entry.Status = ...` (~line 464).

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/scraper && go test ./internal/handler/ -run TestGetHealth_IncludesHealthField -v`
Expected: PASS.

- [ ] **Step 5: Commit + deploy**

```bash
git add services/scraper/internal/config/providers_remote.go services/scraper/internal/config/providers.go services/scraper/internal/handler/scraper.go services/scraper/internal/handler/scraper_test.go
git commit -m "feat(scraper): pass provider health through to /health for the FE pill (failover gate unchanged)"
```
Deploy: `make redeploy-scraper`.

---

# PHASE 6 — Maintenance: churn suppression

## Task 12: Suppress escalations for `policy!=auto` providers + dedup open issues

**Files:**
- Modify: `services/maintenance/internal/config/config.go` (add `CatalogURL`)
- Modify: `services/maintenance/cmd/maintenance/main.go` (suppress gate + dedup)
- Modify: `services/maintenance/internal/state/manager.go` (ensure `FindOpenIssueByAlert` is used)
- Test: `services/maintenance/internal/state/manager_test.go` + a small unit for the suppress predicate

**Interfaces:**
- Consumes: catalog `/internal/scraper/providers` (`policy` field).
- Produces: `func (s *service) shouldSuppressForProvider(provider string) bool` (true when the provider's `policy != "auto"`); dedup reuses an existing open issue via `state.FindOpenIssueByAlert(name, service)` instead of creating a duplicate.

- [ ] **Step 1: Write the failing test**

```go
func TestFindOpenIssueByAlert_Dedup(t *testing.T) {
	m := newManagerWithIssues(t, []domain.Issue{
		{ID: "AUTO-100", Status: domain.StatusEscalated, AffectedService: "allanime", Source: "grafana_alert", Title: "allanime stream_segment DOWN"},
	})
	got := m.FindOpenIssueByAlert("Service Unreachable", "allanime")
	if got == nil || got.ID != "AUTO-100" {
		t.Fatalf("dedup miss: %+v", got)
	}
	// resolved issues must NOT match (re-fire after fix should open a new one)
	m2 := newManagerWithIssues(t, []domain.Issue{{ID: "AUTO-1", Status: domain.StatusResolved, AffectedService: "allanime"}})
	if m2.FindOpenIssueByAlert("x", "allanime") != nil {
		t.Fatal("resolved issue must not dedup-match")
	}
}

func TestShouldSuppressForProvider(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"data":{"providers":[{"name":"allanime","policy":"manual"},{"name":"gogoanime","policy":"auto"}]}}`))
	}))
	defer srv.Close()
	s := &service{cfg: config.Config{CatalogURL: srv.URL}, http: srv.Client()}
	if !s.shouldSuppressForProvider("allanime") {
		t.Fatal("manual provider should be suppressed")
	}
	if s.shouldSuppressForProvider("gogoanime") {
		t.Fatal("auto provider must NOT be suppressed")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd services/maintenance && go test ./... -run 'TestFindOpenIssueByAlert_Dedup|TestShouldSuppressForProvider' -v`
Expected: FAIL — `shouldSuppressForProvider`/`CatalogURL` undefined (and confirm `FindOpenIssueByAlert` filters by open status).

- [ ] **Step 3: Write minimal implementation**

- `config.go`: add `CatalogURL string` to `Config`; in `Load()`: `CatalogURL: getEnv("CATALOG_URL", "http://catalog:8081")`.
- `main.go`: add an `http *http.Client` to the `service` struct (init in `main`). Implement:

```go
func (s *service) shouldSuppressForProvider(provider string) bool {
	if provider == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.cfg.CatalogURL+"/internal/scraper/providers", nil)
	resp, err := s.http.Do(req)
	if err != nil {
		return false // fail-open: catalog blip must not block real escalations
	}
	defer resp.Body.Close()
	var body struct {
		Data struct {
			Providers []struct {
				Name   string `json:"name"`
				Policy string `json:"policy"`
			} `json:"providers"`
		} `json:"data"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return false
	}
	for _, p := range body.Data.Providers {
		if strings.EqualFold(p.Name, provider) {
			return p.Policy != "" && p.Policy != "auto"
		}
	}
	return false
}
```

Insert the gate in `processWork` after the cooldown check (~line 607), before the firing notification / `Analyze`:

```go
	if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
		if s.shouldSuppressForProvider(msg.Alerts[0].Service) {
			log.Infow("suppressing escalation: provider already managed (policy!=auto)", "provider", msg.Alerts[0].Service, "alert", msg.Alerts[0].Name)
			continue
		}
	}
```

Before `CreateIssue` (~line 1084), add the dedup reuse:

```go
	if msg.Type == domain.MessageAlertFiring && len(msg.Alerts) > 0 {
		if existing := s.state.FindOpenIssueByAlert(msg.Alerts[0].Name, msg.Alerts[0].Service); existing != nil {
			log.Infow("reusing open issue instead of duplicate", "issue", existing.ID, "service", msg.Alerts[0].Service)
			s.state.UpdateIssue(existing.ID, func(i *domain.Issue) { i.Status = domain.IssueStatus(result.Issue.Status) })
			issueID = existing.ID
		} else {
			issueID = s.state.CreateIssue(domain.Issue{ /* existing fields */ })
		}
	} else {
		issueID = s.state.CreateIssue(domain.Issue{ /* existing fields */ })
	}
```

- `manager.go`: ensure `FindOpenIssueByAlert` matches on `AffectedService == service` AND status ∈ {open, investigating, escalated} (NOT resolved/wont_fix). Fix if it currently ignores status.

- [ ] **Step 4: Run test to verify it passes**

Run: `cd services/maintenance && go test ./... -run 'TestFindOpenIssueByAlert_Dedup|TestShouldSuppressForProvider' -v`
Expected: PASS.

- [ ] **Step 5: Commit + deploy**

```bash
git add services/maintenance/internal/config/config.go services/maintenance/cmd/maintenance/main.go services/maintenance/internal/state/manager.go services/maintenance/internal/state/manager_test.go
git commit -m "feat(maintenance): suppress escalations for managed (policy!=auto) providers + dedup open issues"
```
Deploy: `make build-maintenance` then restart the systemd unit per the maintenance deploy runbook (atomic binary swap).

---

# PHASE 7 — Frontend pill + Grafana

## Task 13: Add the `recovering` (lime / yellow-green) provider chip state

**Files:**
- Modify: `frontend/web/src/types/aePlayer.ts:28`, `frontend/web/src/composables/aePlayer/useProviderHealth.ts`, `frontend/web/src/components/player/aePlayer/SourcePanel.vue` (STATE_RANK), `frontend/web/src/components/player/aePlayer/ProviderChip.vue` (badge + selectable), `frontend/web/src/components/ui/badge-variants.ts`, `frontend/web/src/locales/{en,ru,ja}.json`
- Test: `frontend/web/src/components/player/aePlayer/__tests__/ProviderChip.spec.ts` (extend) + `useProviderHealth.spec.ts`

**Interfaces:**
- Consumes: scraper `/health` per-provider `health` field (Task 11).
- Produces: `ChipState` gains `'recovering'`; `useProviderHealth` maps `health === 'recovering'` → `state:'recovering'`; the chip shows a lime "Recovering" badge; recovering is selectable in hacker mode and ranked between active and degraded.

- [ ] **Step 1: Write the failing test**

In `useProviderHealth.spec.ts`:

```ts
it('maps health=recovering to recovering chip state', () => {
  const rows = computeProviderRows(
    [{ def: { id: 'gogoanime' }, /* ...minimal def */ }] as any,
    { gogoanime: { name: 'gogoanime', status: 'degraded', health: 'recovering', enabled: true, up: true, reason: 'back online' } } as any,
    { /* filters that keep it relevant */ } as any,
  )
  expect(rows.find(r => r.def.id === 'gogoanime')?.state).toBe('recovering')
})
```

In `ProviderChip.spec.ts`:

```ts
it('renders a Recovering badge for recovering state', () => {
  const w = mount(ProviderChip, { props: { row: { def: { id: 'x', label: 'X', hue: '#0f0' }, state: 'recovering', reason: 'r' }, hackerMode: true }, global: { plugins: [i18n] } })
  expect(w.text()).toContain('Recovering')
  expect(w.find('[data-test="provider-chip"]').attributes('class')).toContain('lime') // or assert clickable
})
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderHealth.spec.ts src/components/player/aePlayer/__tests__/ProviderChip.spec.ts`
Expected: FAIL — `'recovering'` not in `ChipState`; no recovering branch/badge.

- [ ] **Step 3: Write minimal implementation**

- `types/aePlayer.ts:28`: `export type ChipState = 'active' | 'recovering' | 'degraded' | 'disabled' | 'down' | 'irrelevant' | 'wip'`. Add `health?: 'up' | 'recovering' | 'down'` to `ScraperProviderHealth`.
- `useProviderHealth.ts`: in the scraper branch, BEFORE the `status === 'degraded'` check (~line 50), add:
  ```ts
  if (h && h.health === 'recovering') return { def, state: 'recovering', reason: h.reason || h.description }
  ```
  and read `health` off the API object (~line 102) alongside `status`.
- `SourcePanel.vue` STATE_RANK (~line 244): add `recovering: 1` and bump `degraded` to `2`, `wip: 3`, `down: 4`, `disabled: 5`, `irrelevant: 6` (recovering ranks just below active, above degraded).
- `ProviderChip.vue` badge block (~line 78): add before the degraded branch:
  ```vue
  <Badge v-else-if="row.state === 'recovering'" variant="recovering">{{ $t('player.sources.recovering') }}</Badge>
  ```
  and selectable (~line 112): include recovering with degraded — `state === 'active' || ((state === 'degraded' || state === 'recovering') && hackerMode)`.
- `badge-variants.ts`: add `recovering: 'bg-lime-500/20 text-lime-400'` (lime is an exempt brand hue; `badge-variants.ts` is exempt from DS-lint Rule 1).
- i18n — add `"recovering"` under the `player.sources` namespace in all three:
  - en: `"recovering": "Recovering"`
  - ru: `"recovering": "Восстанавливается"`
  - ja: `"recovering": "回復中"`

- [ ] **Step 4: Run tests + DS/i18n gates to verify they pass**

Run: `cd frontend/web && bunx vitest run src/composables/aePlayer/useProviderHealth.spec.ts src/components/player/aePlayer/__tests__/ProviderChip.spec.ts && bash scripts/design-system-lint.sh && bunx vue-tsc --noEmit`
Expected: PASS; DS-lint ERRORS=0; type-check clean. (Use the `frontend-verify` skill for the full pre-flight.)

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/types/aePlayer.ts frontend/web/src/composables/aePlayer/useProviderHealth.ts frontend/web/src/components/player/aePlayer/SourcePanel.vue frontend/web/src/components/player/aePlayer/ProviderChip.vue frontend/web/src/components/ui/badge-variants.ts frontend/web/src/locales/en.json frontend/web/src/locales/ru.json frontend/web/src/locales/ja.json frontend/web/src/composables/aePlayer/useProviderHealth.spec.ts frontend/web/src/components/player/aePlayer/__tests__/ProviderChip.spec.ts
git commit -m "feat(web): recovering (lime) provider chip state wired to backend health"
```

---

## Task 14: Grafana — surface policy/health + Recovering on the Provider Roster panel

**Files:**
- Modify: `docker/grafana/dashboards/playback-health.json` (panel id 102)

- [ ] **Step 1: Update the panel SQL + mappings**

In panel 102's postgres query (the `status AS "Policy"` SELECT), change to surface the real dimensions:

```sql
SELECT name AS provider,
       "group" AS "Group",
       policy AS "Policy",
       health AS "Health",
       CASE
         WHEN policy = 'disabled' THEN 'Off'
         WHEN policy = 'auto' AND health = 'up' THEN 'UP'
         WHEN health = 'recovering' THEN 'Recovering'
         WHEN policy = 'auto' AND health = 'down' THEN 'Failing'
         ELSE 'Manual-only'
       END AS "State",
       last_probed_at AS "Last probed",
       CASE WHEN scraper_operated AND engine = 'browser' THEN 'scraper · Camoufox'
            WHEN scraper_operated THEN 'scraper · HTTP'
            WHEN name = 'ae' THEN 'self-hosted'
            WHEN name = 'kodik-iframe' THEN 'FE iframe'
            ELSE 'catalog parser' END AS "Resolved via",
       reason AS "Note"
FROM stream_providers
ORDER BY scraper_operated DESC, "group", name
```

Update the value-mapping override on the **State** column to add the Recovering hue (lime/yellow-green) alongside the existing colors:

```json
{ "UP": { "color": "green", "text": "UP" },
  "Recovering": { "color": "yellow", "text": "Recovering" },
  "Failing": { "color": "orange", "text": "Failing" },
  "Manual-only": { "color": "red", "text": "Manual-only" },
  "Off": { "color": "dark-red", "text": "Off" } }
```

(Grafana lacks a literal "lime"; `yellow` is the closest stock value-mapping color for the yellow-green Recovering pill — matches the FE intent.)

- [ ] **Step 2: Validate the JSON**

Run: `python3 -c "import json; json.load(open('docker/grafana/dashboards/playback-health.json')); print('valid')"`
Expected: `valid`.

- [ ] **Step 3: Hot-reload + eyeball**

Reload the dashboard into Grafana (provisioned dashboards reload on file change, or `make restart-grafana`). Confirm panel 102 shows State/Policy/Health/Last probed columns with the Recovering color. (No automated test — JSON dashboard.)

- [ ] **Step 4: Commit**

```bash
git add docker/grafana/dashboards/playback-health.json
git commit -m "feat(grafana): provider roster shows policy/health/state + last-probed + Recovering"
```

> **Phase 7 push + deploy:** rebase-retry; `make redeploy-web`; reload Grafana. Run `/animeenigma-after-update` to lint/build/deploy + changelog the whole feature.

---

## Self-Review (completed by plan author)

**Spec coverage:**
- §3 state model → Task 1 (fields/types/helpers) ✓
- §4 transitions → Tasks 4–5 (ApplyHealth/ApplyPolicy/ApplyVerdict) ✓
- §5 probe engine (cadence/fail-fast/popularity/sample) → Tasks 7 (plan), 8 (popularity), 9 (fail-fast+verdict) + Task 1 (ProbeCadence/ProbeSample) ✓
- §6 service flow → Tasks 3 (wire status), 6 (probe-result), 9 (post), 10 (scheduler), 11 (scraper passthrough) ✓
- §7 churn suppression → Task 12 ✓
- §8 migration → Task 2 ✓
- §9 testing → table-driven tests in every backend task ✓
- FE pill (Recovering) → Task 13; Grafana → Task 14 ✓

**Placeholder scan:** no TBD/TODO; each code step shows real code. Two intentional "match existing helper" notes (test DB constructor, `writeJSON`) reference patterns the explore maps confirmed exist — the implementer adopts the package's existing helper.

**Type consistency:** `Policy`/`Health` types + `Eligible()`/`WireStatus()`/`ProbeCadence()`/`ProbeSample()` defined in Task 1 are consumed identically in Tasks 3/6/7; `ApplyVerdict(p, pass, now, demoteAfter, promoteAfter)` signature consistent across Tasks 5/6; `PlanEntry{Provider,SampleSize,FailFast}` consistent across Tasks 7 (catalog JSON) and 9 (analytics struct); `ChipState 'recovering'` consistent across Task 13.

---

## Effort & Impact

- **UXΔ = +3 (Better)** — broken providers leave the chain in ≤6h; recovered ones auto-return; users hit fewer dead providers; admins get an honest live roster.
- **CDI = 0.04 × 21** — 6 services, but each change is localized to the provider-state seam (one new catalog service + additive probe params + a derived FE pill + passthrough). Effort 21 (Fibonacci).
- **MVQ = Phoenix 88% / 85%** — a literal self-healing/resurrection loop; high slop-resistance via exhaustive table-driven state tests.
