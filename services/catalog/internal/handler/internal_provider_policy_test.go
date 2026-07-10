package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// newHandlerTestDB returns an in-memory SQLite DB for handler unit tests.
func newHandlerTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	return db
}

func testProviderPolicyCfg() config.ProviderPolicyConfig {
	return config.ProviderPolicyConfig{
		PromoteAfter: 24 * time.Hour,
		Cadence: domain.CadenceConfig{
			Up:               6 * time.Hour,
			Recovering:       12 * time.Hour,
			Manual:           24 * time.Hour,
			RecoveringSample: 3,
			FullSample:       5,
		},
	}
}

func TestProbeResult_FlipsRow(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	t0 := time.Now().Add(-48 * time.Hour)
	if err := db.Create(&domain.ScraperProvider{
		Name:            "gogoanime",
		Policy:          domain.PolicyAuto,
		Health:          domain.HealthDown,
		HealthSince:     t0,
		ScraperOperated: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"gogoanime","pass":false,"reason":"status_403"}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var p domain.ScraperProvider
	db.First(&p, "name = ?", "gogoanime")
	if p.Health != domain.HealthDown {
		t.Fatalf("health=%s want down", p.Health)
	}
	if p.Policy != domain.PolicyAuto { // sustained down never demotes — policy is admin-only
		t.Fatalf("policy=%s want auto", p.Policy)
	}
	if p.LastProbedAt.IsZero() {
		t.Fatal("last_probed_at not stamped")
	}
}

// TestProbeResult_FirstFailGoesDegraded is the hysteresis headline: a single
// failed probe against a healthy auto provider lands `degraded` (a warning,
// still in auto-failover), NOT `down`, and never touches policy.
func TestProbeResult_FirstFailGoesDegraded(t *testing.T) {
	db := newHandlerTestDB(t)
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name:            "miruro",
		Policy:          domain.PolicyAuto,
		Health:          domain.HealthUp,
		HealthSince:     time.Now().Add(-72 * time.Hour),
		ScraperOperated: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"miruro","pass":false,"reason":"canary_false_negative"}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Data struct {
			Policy string `json:"policy"`
			Health string `json:"health"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rr.Body.String())
	}
	if resp.Data.Health != string(domain.HealthDegraded) || resp.Data.Policy != string(domain.PolicyAuto) {
		t.Fatalf("response (policy,health)=(%s,%s) want (auto,degraded)", resp.Data.Policy, resp.Data.Health)
	}
	var p domain.ScraperProvider
	db.First(&p, "name = ?", "miruro")
	if p.Health != domain.HealthDegraded || p.Policy != domain.PolicyAuto {
		t.Fatalf("persisted (policy,health)=(%s,%s) want (auto,degraded)", p.Policy, p.Health)
	}
	if !p.Eligible() {
		t.Fatal("degraded provider must stay failover-eligible")
	}
}

func TestProbeResult_SkipsDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name:            "nineanime",
		Policy:          domain.PolicyDisabled,
		Health:          domain.HealthDown,
		ScraperOperated: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"nineanime","pass":false}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	// Policy must remain disabled — we skipped mutation.
	var p domain.ScraperProvider
	db.First(&p, "name = ?", "nineanime")
	if p.Policy != domain.PolicyDisabled {
		t.Fatalf("policy=%s want disabled (should have skipped)", p.Policy)
	}
}

// TestProbeResult_RaceWithAdminDisable_DoesNotClobber is the TOCTOU regression
// for the ProbeResult hard-lock fix: ApplyVerdict/the mutation decision are
// computed off the row read at the TOP of the handler (h.db.First), but if an
// admin's SetPolicy(disabled) commits in the window before this handler's own
// Updates() runs, the probe write must NOT silently revert that disable.
//
// A single-threaded test can't literally interleave two HTTP requests, so a
// gorm "gorm:before_update" hook simulates the race deterministically: right
// before ProbeResult's own UPDATE statement executes, it performs the
// concurrent admin disable out-of-band. The guarded `Updates(...).Where("policy
// <> disabled")` must then affect 0 rows, and the handler must report
// "skipped" instead of persisting (or gauge-reporting) the stale pre-disable
// verdict.
func TestProbeResult_RaceWithAdminDisable_DoesNotClobber(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name:            "gogoanime",
		Policy:          domain.PolicyAuto,
		Health:          domain.HealthUp,
		ScraperOperated: true,
	}).Error; err != nil {
		t.Fatal(err)
	}

	var raced bool
	if err := db.Callback().Update().Before("gorm:update").Register("test:race_admin_disable", func(tx *gorm.DB) {
		if raced {
			return // guard against recursing into our own simulated write
		}
		raced = true
		// Run on `tx` (the transaction gorm's own Update wraps itself in), not
		// the outer `db` — a fresh query on `db` would grab a SEPARATE pooled
		// connection, which for sqlite ":memory:" (no shared-cache DSN here) is
		// an entirely different, empty database and errors "no such table".
		// Raw SQL also bypasses gorm's Model/Update callback chain (no
		// recursion into this same hook).
		if err := tx.Exec("UPDATE stream_providers SET policy = ? WHERE name = ?",
			domain.PolicyDisabled, "gogoanime").Error; err != nil {
			t.Fatal(err)
		}
	}); err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"gogoanime","pass":false,"reason":"status_403"}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 200 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !raced {
		t.Fatal("race hook never fired — test setup is broken")
	}

	var resp struct {
		Data struct {
			Skipped bool `json:"skipped"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v (body=%s)", err, rr.Body.String())
	}
	if !resp.Data.Skipped {
		t.Fatalf("expected skipped:true once the row was disabled mid-flight, body=%s", rr.Body.String())
	}

	var p domain.ScraperProvider
	if err := db.First(&p, "name = ?", "gogoanime").Error; err != nil {
		t.Fatal(err)
	}
	if p.Policy != domain.PolicyDisabled {
		t.Fatalf("policy = %q, want disabled to survive the race (probe must not clobber a concurrent admin disable)", p.Policy)
	}
}

func TestProbeResult_UnknownProvider(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}

	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	body := strings.NewReader(`{"provider":"doesnotexist","pass":true}`)
	h.ProbeResult(rr, httptest.NewRequest("POST", "/internal/providers/probe-result", body))

	if rr.Code != 404 {
		t.Fatalf("code=%d want 404", rr.Code)
	}
}

func TestProbePlan_DueSet(t *testing.T) {
	db := newHandlerTestDB(t)
	db.AutoMigrate(&domain.ScraperProvider{})
	now := time.Now().UTC()
	db.Create(&[]domain.ScraperProvider{
		{Name: "gogoanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, LastProbedAt: now.Add(-7 * time.Hour), ScraperOperated: true},    // due (6h cadence for Up)
		{Name: "miruro", Policy: domain.PolicyAuto, Health: domain.HealthUp, LastProbedAt: now.Add(-1 * time.Hour), ScraperOperated: true},        // not due
		{Name: "allanime", Policy: domain.PolicyManual, Health: domain.HealthDown, LastProbedAt: now.Add(-25 * time.Hour), ScraperOperated: true}, // due (24h manual cadence), size=1, ff=true
		{Name: "deadguy", Policy: domain.PolicyDisabled, Health: domain.HealthDown, LastProbedAt: now.Add(-99 * time.Hour), ScraperOperated: true}, // excluded
	})
	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
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
	got := map[string]struct {
		size int
		ff   bool
	}{}
	for _, e := range body.Data.Plan {
		got[e.Provider] = struct {
			size int
			ff   bool
		}{e.SampleSize, e.FailFast}
	}
	if _, ok := got["miruro"]; ok {
		t.Fatal("miruro not due — should be absent")
	}
	if _, ok := got["deadguy"]; ok {
		t.Fatal("disabled must be excluded")
	}
	if got["gogoanime"] != (struct {
		size int
		ff   bool
	}{5, false}) {
		t.Fatalf("gogoanime plan=%+v want {5,false}", got["gogoanime"])
	}
	if got["allanime"] != (struct {
		size int
		ff   bool
	}{1, true}) {
		t.Fatalf("allanime plan=%+v want {1,true}", got["allanime"])
	}
}

func TestProbeResultPersistsMetrics(t *testing.T) {
	db := newHandlerTestDB(t)
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&domain.ScraperProvider{
		Name: "miruro", Policy: domain.PolicyManual, Health: domain.HealthDown,
		Engine: "browser", ScraperOperated: true, Group: "en",
	}).Error; err != nil {
		t.Fatalf("seed: %v", err)
	}
	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())

	body := `{"provider":"miruro","pass":true,"reason":"","metrics":{"warmup_ms":9800,"resolve_ms":1900,"cdn_host":"kwik.cx"}}`
	rr := httptest.NewRecorder()
	h.ProbeResult(rr, httptest.NewRequest(http.MethodPost, "/internal/providers/probe-result", strings.NewReader(body)))
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr.Code, rr.Body.String())
	}

	var p domain.ScraperProvider
	if err := db.First(&p, "name = ?", "miruro").Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !strings.Contains(p.LastTickMetrics, `"cdn_host":"kwik.cx"`) {
		t.Fatalf("last_tick_metrics not persisted: %q", p.LastTickMetrics)
	}

	// A verdict WITHOUT metrics must not wipe the stored summary.
	rr2 := httptest.NewRecorder()
	h.ProbeResult(rr2, httptest.NewRequest(http.MethodPost, "/internal/providers/probe-result",
		strings.NewReader(`{"provider":"miruro","pass":false,"reason":"cdn_unreachable"}`)))
	_ = db.First(&p, "name = ?", "miruro")
	if !strings.Contains(p.LastTickMetrics, "kwik.cx") {
		t.Fatalf("metrics wiped by metrics-less verdict: %q", p.LastTickMetrics)
	}

	// A verdict with an explicit JSON null metrics payload must also not wipe
	// the stored summary (json.RawMessage("null") has len 4 > 0).
	rr3 := httptest.NewRecorder()
	h.ProbeResult(rr3, httptest.NewRequest(http.MethodPost, "/internal/providers/probe-result",
		strings.NewReader(`{"provider":"miruro","pass":true,"reason":"","metrics":null}`)))
	if rr3.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rr3.Code, rr3.Body.String())
	}
	_ = db.First(&p, "name = ?", "miruro")
	if !strings.Contains(p.LastTickMetrics, "kwik.cx") {
		t.Fatalf("metrics wiped by null-metrics verdict: %q", p.LastTickMetrics)
	}
}

func TestProbePlanIncludesEngine(t *testing.T) {
	db := newHandlerTestDB(t)
	db.AutoMigrate(&domain.ScraperProvider{})
	// Two scraper-operated rows, due to probe (LastProbedAt far in the past),
	// one browser one http.
	old := time.Now().Add(-48 * time.Hour).UTC()
	for _, p := range []domain.ScraperProvider{
		{Name: "miruro", Policy: domain.PolicyManual, Health: domain.HealthDown, Engine: "browser", ScraperOperated: true, Group: "en", LastProbedAt: old},
		{Name: "allanime", Policy: domain.PolicyAuto, Health: domain.HealthUp, Engine: "http", ScraperOperated: true, Group: "en", LastProbedAt: old},
	} {
		if err := db.Create(&p).Error; err != nil {
			t.Fatalf("seed %s: %v", p.Name, err)
		}
	}
	h := handler.NewInternalProviderPolicyHandler(db, testProviderPolicyCfg(), testNopLogger())
	rr := httptest.NewRecorder()
	h.ProbePlan(rr, httptest.NewRequest("GET", "/internal/providers/probe-plan", nil))

	var body struct {
		Data struct {
			Plan []struct {
				Provider string `json:"provider"`
				Engine   string `json:"engine"`
			} `json:"plan"`
		} `json:"data"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := map[string]string{}
	for _, e := range body.Data.Plan {
		got[e.Provider] = e.Engine
	}
	if got["miruro"] != "browser" {
		t.Fatalf("miruro engine = %q, want browser (plan=%+v)", got["miruro"], body.Data.Plan)
	}
	if got["allanime"] != "http" {
		t.Fatalf("allanime engine = %q, want http", got["allanime"])
	}
}
