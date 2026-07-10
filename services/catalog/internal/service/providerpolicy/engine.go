package providerpolicy

import (
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// ApplyHealth advances a provider's health from a probe pass/fail, with
// one-fail hysteresis on the way down:
// up --fail--> degraded (pending confirmation) --fail--> down ;
// degraded --pass--> up (false alarm cleared) ;
// down --pass--> recovering ; recovering --pass after promoteAfter--> up ;
// recovering/down --fail--> down. HealthSince is reset only on a real
// state change.
func ApplyHealth(p *domain.ScraperProvider, pass bool, now time.Time, promoteAfter time.Duration) {
	prev := p.Health
	if !pass {
		switch p.Health {
		case domain.HealthUp:
			p.Health = domain.HealthDegraded // first fail: warn, stay failover-trusted
		default: // degraded, recovering, down, unseeded
			p.Health = domain.HealthDown
		}
	} else {
		switch p.Health {
		case domain.HealthDown:
			p.Health = domain.HealthRecovering
		case domain.HealthRecovering:
			if now.Sub(p.HealthSince) >= promoteAfter {
				p.Health = domain.HealthUp
			}
		case domain.HealthDegraded:
			p.Health = domain.HealthUp // one pass clears the pending warning
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

// ApplyVerdict is the full per-probe transition: health, then stamp. Policy
// is admin-only and never mutated here (the 24h auto→manual demotion and
// manual→auto promotion were retired 2026-07-08 with the hysteresis redesign).
func ApplyVerdict(p *domain.ScraperProvider, pass bool, now time.Time, promoteAfter time.Duration) {
	ApplyHealth(p, pass, now, promoteAfter)
	p.LastProbedAt = now
}
