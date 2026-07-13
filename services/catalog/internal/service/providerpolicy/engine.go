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

// ReconcilePolicyFromHealth drives the failover-participation policy from the
// (already-advanced) health, health-first: a confirmed-down provider is parked
// to manual (out of auto-failover, still hacker-selectable); anything else
// (up/degraded/recovering) is auto. disabled is the admin hard-lock and is
// NEVER touched here — only the admin Auto/Disabled toggle sets it. PolicySince
// is stamped only on a real change. This is the 2026-07-13 reversal of the
// 2026-07-08 "policy admin-only" decision: the owner asked that degraded/down
// providers auto-park (owner refined it to "down only") so the /admin/policy
// panel is machine-controlled and the admin toggles only probe on/off.
func ReconcilePolicyFromHealth(p *domain.ScraperProvider, now time.Time) {
	if p.Policy == domain.PolicyDisabled {
		return
	}
	want := domain.PolicyAuto
	if p.Health == domain.HealthDown {
		want = domain.PolicyManual
	}
	if p.Policy != want {
		p.Policy = want
		p.PolicySince = now
	}
}

// ApplyVerdict is the full per-probe transition: health, then health-driven
// policy reconciliation, then stamp. Unlike the 2026-07-08→2026-07-13 window,
// policy IS advanced here now (down⇒manual, else auto; disabled untouched).
func ApplyVerdict(p *domain.ScraperProvider, pass bool, now time.Time, promoteAfter time.Duration) {
	ApplyHealth(p, pass, now, promoteAfter)
	ReconcilePolicyFromHealth(p, now)
	p.LastProbedAt = now
}
