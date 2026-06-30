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
		{"auto+down failing", domain.PolicyAuto, domain.HealthDown, domain.StatusDegraded},
		{"auto+recovering", domain.PolicyAuto, domain.HealthRecovering, domain.StatusDegraded},
		{"manual+down", domain.PolicyManual, domain.HealthDown, domain.StatusDegraded},
		{"manual+recovering", domain.PolicyManual, domain.HealthRecovering, domain.StatusDegraded},
		{"disabled", domain.PolicyDisabled, domain.HealthDown, domain.StatusDisabled},
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
		{"up", domain.PolicyAuto, domain.HealthUp, 6 * time.Hour, 5, false},
		{"recovering", domain.PolicyManual, domain.HealthRecovering, 12 * time.Hour, 3, true},
		{"manual-down", domain.PolicyManual, domain.HealthDown, 24 * time.Hour, 1, true},
		{"failing auto-down", domain.PolicyAuto, domain.HealthDown, 6 * time.Hour, 5, true},
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

func TestDerivedStateAndCode(t *testing.T) {
	cases := []struct {
		name   string
		policy domain.ProviderPolicy
		health domain.ProviderHealth
		want   string
		code   float64
	}{
		{"auto+up", domain.PolicyAuto, domain.HealthUp, domain.StateUP, 4},
		{"auto+recovering", domain.PolicyAuto, domain.HealthRecovering, domain.StateRecovering, 3},
		{"auto+down", domain.PolicyAuto, domain.HealthDown, domain.StateDown, 1},
		{"manual+up", domain.PolicyManual, domain.HealthUp, domain.StateDegraded, 2},
		{"manual+down", domain.PolicyManual, domain.HealthDown, domain.StateDegraded, 2},
		{"manual+recovering", domain.PolicyManual, domain.HealthRecovering, domain.StateRecovering, 3},
		{"disabled+down", domain.PolicyDisabled, domain.HealthDown, domain.StateDisabled, 0},
		{"disabled+up", domain.PolicyDisabled, domain.HealthUp, domain.StateDisabled, 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := domain.ScraperProvider{Policy: c.policy, Health: c.health}
			if got := p.DerivedState(); got != c.want {
				t.Fatalf("DerivedState()=%q want %q", got, c.want)
			}
			if got := p.StateCode(); got != c.code {
				t.Fatalf("StateCode()=%v want %v", got, c.code)
			}
		})
	}
}
