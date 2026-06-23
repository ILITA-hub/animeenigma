package providerpolicy

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

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
