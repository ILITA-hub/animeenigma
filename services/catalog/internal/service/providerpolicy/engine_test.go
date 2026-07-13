package providerpolicy

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestApplyVerdict_HysteresisThenRecover(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	p := &domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthUp, HealthSince: t0}

	ApplyVerdict(p, false, t0.Add(time.Hour), day) // first fail -> degraded, NOT down, still eligible
	if p.Health != domain.HealthDegraded || p.Policy != domain.PolicyAuto || !p.Eligible() {
		t.Fatalf("after first fail: %+v", p)
	}
	ApplyVerdict(p, false, t0.Add(2*time.Hour), day) // second consecutive fail -> down, health-parked to manual
	if p.Health != domain.HealthDown || p.Policy != domain.PolicyManual || p.Eligible() {
		t.Fatalf("after second fail: %+v", p)
	}
	ApplyVerdict(p, false, t0.Add(2*day), day) // sustained down stays parked manual
	if p.Policy != domain.PolicyManual {
		t.Fatalf("policy=%s want manual (down is health-parked)", p.Policy)
	}
	ApplyVerdict(p, true, t0.Add(3*day), day) // down probe passes -> recovering, un-parks to auto
	if p.Health != domain.HealthRecovering || p.Policy != domain.PolicyAuto {
		t.Fatalf("expected recovering+auto, got %+v", p)
	}
	ApplyVerdict(p, true, t0.Add(4*day+time.Minute), day) // recovering >1d -> up
	if p.Health != domain.HealthUp || p.Policy != domain.PolicyAuto || !p.Eligible() {
		t.Fatalf("expected up+eligible, got %+v", p)
	}
	if !p.LastProbedAt.Equal(t0.Add(4*day + time.Minute)) {
		t.Fatalf("last_probed_at not stamped: %v", p.LastProbedAt)
	}
}

func TestApplyVerdict_DegradedClearsOnPass(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	p := &domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthDegraded, HealthSince: t0}
	ApplyVerdict(p, true, t0.Add(time.Hour), day) // false alarm cleared -> straight back up
	if p.Health != domain.HealthUp || p.Policy != domain.PolicyAuto {
		t.Fatalf("degraded+pass: %+v", p)
	}
}

// TestApplyVerdict_ReconcilesPolicy covers the 2026-07-13 health-driven policy:
// down⇒manual, everything else⇒auto, disabled immune.
func TestApplyVerdict_ReconcilesPolicy(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	t.Run("parked manual un-parks to auto on recovery", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyManual, Health: domain.HealthDown, HealthSince: t0}
		ApplyVerdict(p, true, t0.Add(day), day) // down -> recovering, un-park
		if p.Health != domain.HealthRecovering || p.Policy != domain.PolicyAuto {
			t.Fatalf("after first pass: %+v", p)
		}
		ApplyVerdict(p, true, t0.Add(3*day), day) // recovering >1d -> up, stays auto
		if p.Health != domain.HealthUp || p.Policy != domain.PolicyAuto {
			t.Fatalf("after promote: %+v", p)
		}
	})
	t.Run("auto parks to manual on confirmed down", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthUp, HealthSince: t0}
		ApplyVerdict(p, false, t0.Add(day), day) // up -> degraded, STAYS auto (one-blip buffer)
		if p.Health != domain.HealthDegraded || p.Policy != domain.PolicyAuto {
			t.Fatalf("after first fail (degraded): %+v", p)
		}
		ApplyVerdict(p, false, t0.Add(2*day), day) // degraded -> down, park manual
		if p.Health != domain.HealthDown || p.Policy != domain.PolicyManual {
			t.Fatalf("after second fail (down): %+v", p)
		}
	})
	t.Run("disabled is immune to health reconcile", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyDisabled, Health: domain.HealthDown, HealthSince: t0}
		ApplyVerdict(p, true, t0.Add(day), day) // health advances but policy stays disabled
		if p.Policy != domain.PolicyDisabled {
			t.Fatalf("policy=%s want disabled (admin hard-lock)", p.Policy)
		}
	})
}

func TestApplyHealth(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	cases := []struct {
		name       string
		start      domain.ProviderHealth
		since      time.Time
		pass       bool
		now        time.Time
		wantHealth domain.ProviderHealth
		sinceMoved bool
	}{
		{"down->recovering on pass", domain.HealthDown, t0, true, t0.Add(day), domain.HealthRecovering, true},
		{"down stays on fail", domain.HealthDown, t0, false, t0.Add(time.Hour), domain.HealthDown, false},
		{"recovering stays before promote window", domain.HealthRecovering, t0, true, t0.Add(day - time.Minute), domain.HealthRecovering, false},
		{"recovering->up after promote window", domain.HealthRecovering, t0, true, t0.Add(day), domain.HealthUp, true},
		{"recovering->down on fail", domain.HealthRecovering, t0, false, t0.Add(time.Hour), domain.HealthDown, true},
		{"up->degraded on first fail", domain.HealthUp, t0, false, t0.Add(time.Hour), domain.HealthDegraded, true},
		{"up stays on pass", domain.HealthUp, t0, true, t0.Add(time.Hour), domain.HealthUp, false},
		{"degraded->down on second fail", domain.HealthDegraded, t0, false, t0.Add(time.Hour), domain.HealthDown, true},
		{"degraded->up on pass", domain.HealthDegraded, t0, true, t0.Add(time.Hour), domain.HealthUp, true},
		{"unseeded->recovering on pass", "", t0, true, t0.Add(time.Hour), domain.HealthRecovering, true},
		{"unseeded->down on fail", "", t0, false, t0.Add(time.Hour), domain.HealthDown, true},
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
