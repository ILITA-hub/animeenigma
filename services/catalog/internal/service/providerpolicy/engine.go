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
