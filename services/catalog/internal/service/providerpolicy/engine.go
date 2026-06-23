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
