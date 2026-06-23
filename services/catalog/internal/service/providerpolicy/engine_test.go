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
