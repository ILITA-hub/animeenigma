package domain_test

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestWireStatus(t *testing.T) {
	cases := []struct {
		name   string
		policy domain.ProviderPolicy
		health domain.ProviderHealth
		want   domain.ProviderStatus
	}{
		{"auto+up eligible", domain.PolicyAuto, domain.HealthUp, domain.StatusEnabled},
		{"auto+degraded stays in failover", domain.PolicyAuto, domain.HealthDegraded, domain.StatusEnabled},
		{"auto+down failing", domain.PolicyAuto, domain.HealthDown, domain.StatusDegraded},
		{"auto+recovering", domain.PolicyAuto, domain.HealthRecovering, domain.StatusDegraded},
		{"manual+up", domain.PolicyManual, domain.HealthUp, domain.StatusDegraded},
		{"manual+down", domain.PolicyManual, domain.HealthDown, domain.StatusDegraded},
		{"manual+recovering", domain.PolicyManual, domain.HealthRecovering, domain.StatusDegraded},
		{"disabled", domain.PolicyDisabled, domain.HealthDown, domain.StatusDisabled},
		{"disabled+up", domain.PolicyDisabled, domain.HealthUp, domain.StatusDisabled},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := domain.ScraperProvider{Policy: c.policy, Health: c.health}
			if got := p.WireStatus(); got != c.want {
				t.Fatalf("WireStatus()=%q want %q", got, c.want)
			}
			if got := p.Eligible(); got != (c.want == domain.StatusEnabled) {
				t.Fatalf("Eligible()=%v want %v", got, c.want == domain.StatusEnabled)
			}
		})
	}
}

func TestProbeCadenceAndSample(t *testing.T) {
	cfg := domain.CadenceConfig{Up: 6 * time.Hour, Recovering: 12 * time.Hour, Manual: 24 * time.Hour, RecoveringSample: 3, FullSample: 5}
	cases := []struct {
		name        string
		policy      domain.ProviderPolicy
		health      domain.ProviderHealth
		wantCadence time.Duration
		wantSize    int
		wantFF      bool
	}{
		// Recovery is anchor-gated now (2026-07-13): no branch fail-fasts on a
		// non-anchor miss, so per-title providers can climb back on the anchor.
		{"up", domain.PolicyAuto, domain.HealthUp, 6 * time.Hour, 5, false},
		{"degraded re-probes next cycle", domain.PolicyAuto, domain.HealthDegraded, 6 * time.Hour, 5, false},
		{"recovering", domain.PolicyManual, domain.HealthRecovering, 12 * time.Hour, 3, false},
		{"manual-down", domain.PolicyManual, domain.HealthDown, 24 * time.Hour, 1, false},
		// down is always manual under the health-driven policy; if a stale auto+down
		// row exists it still samples just the anchor (1, no fail-fast).
		{"down samples anchor", domain.PolicyAuto, domain.HealthDown, 6 * time.Hour, 1, false},
		{"disabled never", domain.PolicyDisabled, domain.HealthDown, 0, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := domain.ScraperProvider{Policy: c.policy, Health: c.health}
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

func TestScraperProviderSchema_AutoMigrate(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatalf("automigrate: %v", err)
	}
	if !db.Migrator().HasTable("stream_providers") {
		t.Fatal("stream_providers table not created")
	}
	for _, col := range []string{"name", "status", "group", "supports_sub", "supports_dub", "supports_raw", "sub_delivery", "quality_ceiling", "preference_weight", "scraper_operated"} {
		if !db.Migrator().HasColumn(&domain.ScraperProvider{}, col) {
			t.Errorf("missing column %q", col)
		}
	}
	row := domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, SubDelivery: "hard", PreferenceWeight: 90}
	if err := db.Create(&row).Error; err != nil {
		t.Fatalf("create: %v", err)
	}
	var got domain.ScraperProvider
	if err := db.First(&got, "name = ?", "allanime").Error; err != nil {
		t.Fatalf("first: %v", err)
	}
	if got.PreferenceWeight != 90 || got.SubDelivery != "hard" || !got.IsEnabled() {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestScraperProviderDatabaseDefaultsAreDisabled(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	if err := db.Exec(`INSERT INTO stream_providers (name) VALUES (?)`, "new-provider").Error; err != nil {
		t.Fatal(err)
	}
	var row domain.ScraperProvider
	if err := db.First(&row, "name = ?", "new-provider").Error; err != nil {
		t.Fatal(err)
	}
	if row.Status != domain.StatusDisabled || row.Policy != domain.PolicyDisabled || row.Health != domain.HealthDown {
		t.Fatalf("DB defaults = status %q policy %q health %q; want disabled/disabled/down", row.Status, row.Policy, row.Health)
	}
}

// TestDerivedStateAndCode covers the deliberate axis split (see
// scraper_provider.go): DerivedState/DerivedStateCode are the HEALTH display
// (a parked manual provider shows its live health; only policy=disabled reads
// as Disabled), while StateCode is the FAILOVER-PARTICIPATION encoding behind
// the fleet alerts (manual AND disabled collapse to 0).
func TestDerivedStateAndCode(t *testing.T) {
	cases := []struct {
		name         string
		policy       domain.ProviderPolicy
		health       domain.ProviderHealth
		wantState    string  // DerivedState (health display label)
		wantHealth   float64 // DerivedStateCode (health timeline gauge)
		wantFailover float64 // StateCode (failover-participation alert gauge)
	}{
		{"auto+up", domain.PolicyAuto, domain.HealthUp, domain.StateUP, 4, 4},
		{"auto+recovering", domain.PolicyAuto, domain.HealthRecovering, domain.StateRecovering, 3, 3},
		{"auto+degraded", domain.PolicyAuto, domain.HealthDegraded, domain.StateDegrading, 2, 2},
		{"auto+down", domain.PolicyAuto, domain.HealthDown, domain.StateDown, 1, 1},
		// manual: DISPLAY shows live health (4-state), but the alert gauge is 0
		// (parked out of auto-failover).
		{"manual+up", domain.PolicyManual, domain.HealthUp, domain.StateUP, 4, 0},
		{"manual+degraded", domain.PolicyManual, domain.HealthDegraded, domain.StateDegrading, 2, 0},
		{"manual+down", domain.PolicyManual, domain.HealthDown, domain.StateDown, 1, 0},
		{"manual+recovering", domain.PolicyManual, domain.HealthRecovering, domain.StateRecovering, 3, 0},
		// disabled: Disabled on both axes.
		{"disabled+down", domain.PolicyDisabled, domain.HealthDown, domain.StateDisabled, 0, 0},
		{"disabled+up", domain.PolicyDisabled, domain.HealthUp, domain.StateDisabled, 0, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := domain.ScraperProvider{Policy: c.policy, Health: c.health}
			if got := p.DerivedState(); got != c.wantState {
				t.Fatalf("DerivedState()=%q want %q", got, c.wantState)
			}
			if got := p.DerivedStateCode(); got != c.wantHealth {
				t.Fatalf("DerivedStateCode()=%v want %v", got, c.wantHealth)
			}
			if got := p.StateCode(); got != c.wantFailover {
				t.Fatalf("StateCode()=%v want %v", got, c.wantFailover)
			}
		})
	}
}
