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
	ApplyVerdict(p, false, t0.Add(2*time.Hour), day) // second consecutive fail -> down, still auto
	if p.Health != domain.HealthDown || p.Policy != domain.PolicyAuto || p.Eligible() {
		t.Fatalf("after second fail: %+v", p)
	}
	ApplyVerdict(p, false, t0.Add(2*day), day) // sustained fails never demote policy
	if p.Policy != domain.PolicyAuto {
		t.Fatalf("policy=%s want auto (auto demotion is retired)", p.Policy)
	}
	ApplyVerdict(p, true, t0.Add(3*day), day) // down probe passes -> recovering
	if p.Health != domain.HealthRecovering {
		t.Fatalf("expected recovering, got %s", p.Health)
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

func TestApplyVerdict_PolicyNeverMutated(t *testing.T) {
	t0 := time.Date(2026, 6, 23, 0, 0, 0, 0, time.UTC)
	day := 24 * time.Hour
	t.Run("manual stays manual through sustained passes", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyManual, Health: domain.HealthDown, HealthSince: t0}
		ApplyVerdict(p, true, t0.Add(day), day)   // down -> recovering
		ApplyVerdict(p, true, t0.Add(3*day), day) // recovering >1d -> up
		if p.Health != domain.HealthUp {
			t.Fatalf("health=%s want up", p.Health)
		}
		if p.Policy != domain.PolicyManual {
			t.Fatalf("policy=%s want manual (auto promotion is retired)", p.Policy)
		}
	})
	t.Run("auto stays auto through fail-fail-fail", func(t *testing.T) {
		p := &domain.ScraperProvider{Policy: domain.PolicyAuto, Health: domain.HealthUp, HealthSince: t0}
		for i := 1; i <= 3; i++ {
			ApplyVerdict(p, false, t0.Add(time.Duration(i)*day), day)
		}
		if p.Health != domain.HealthDown || p.Policy != domain.PolicyAuto {
			t.Fatalf("after fail x3: %+v", p)
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
