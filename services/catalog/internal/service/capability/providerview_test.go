package capability

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestDeriveProviderView(t *testing.T) {
	cases := []struct {
		name       string
		status     domain.ProviderStatus // STALE column — set deliberately to prove it is NOT read
		policy     domain.ProviderPolicy // LIVE authority
		health     domain.ProviderHealth // LIVE authority
		hasContent bool
		wantState  string
		wantSel    bool
		wantHacker bool
	}{
		// auto policy — the normal auto-failover chain.
		{"auto up with content", domain.StatusEnabled, domain.PolicyAuto, domain.HealthUp, true, "active", true, false},
		{"auto down with content stays active (grace window)", domain.StatusEnabled, domain.PolicyAuto, domain.HealthDown, true, "active", true, false},
		{"auto recovering", domain.StatusEnabled, domain.PolicyAuto, domain.HealthRecovering, true, "recovering", true, false},
		{"auto up no content (ae not in library)", domain.StatusEnabled, domain.PolicyAuto, domain.HealthUp, false, "no_content", false, false},
		// manual policy — pinned out of the auto chain (admin soft-degrade or machine auto-demote).
		{"manual down is hacker-only", domain.StatusDegraded, domain.PolicyManual, domain.HealthDown, true, "degraded", true, true},
		{"manual ignores content", domain.StatusDegraded, domain.PolicyManual, domain.HealthUp, false, "degraded", true, true},
		{"manual recovering is still degraded (admin-pinned out of chain)", domain.StatusDegraded, domain.PolicyManual, domain.HealthRecovering, true, "degraded", true, true},
		// DRIFT CASES — the stale `status` column disagrees with the live (policy, health).
		// These are the cases the original bug got wrong by reading `status`.
		{"AUTO-DEMOTED: status still 'enabled' but policy=manual/health=down → degraded", domain.StatusEnabled, domain.PolicyManual, domain.HealthDown, true, "degraded", true, true},
		{"AUTO-PROMOTED: status still 'degraded' but policy=auto/health=up → active", domain.StatusDegraded, domain.PolicyAuto, domain.HealthUp, true, "active", true, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := domain.ScraperProvider{Status: c.status, Policy: c.policy, Health: c.health}
			gotState, gotSel, gotHacker := deriveProviderView(row, c.hasContent)
			if gotState != c.wantState || gotSel != c.wantSel || gotHacker != c.wantHacker {
				t.Fatalf("got (%q,%v,%v) want (%q,%v,%v)",
					gotState, gotSel, gotHacker, c.wantState, c.wantSel, c.wantHacker)
			}
		})
	}
}

func TestAudiosFromTraits(t *testing.T) {
	row := domain.ScraperProvider{SupportsSub: true, SupportsDub: true}
	got := audiosFromTraits(row)
	if len(got) != 2 || got[0] != "sub" || got[1] != "dub" {
		t.Fatalf("got %v want [sub dub]", got)
	}
	if a := audiosFromTraits(domain.ScraperProvider{SupportsRaw: true}); len(a) != 1 || a[0] != "raw" {
		t.Fatalf("raw-only got %v want [raw]", a)
	}
	if a := audiosFromTraits(domain.ScraperProvider{}); len(a) != 0 {
		t.Fatalf("empty traits: got %v want []", a)
	}
}

func TestWireGroup(t *testing.T) {
	if got := wireGroup(""); got != "en" {
		t.Fatalf("empty: got %q want %q", got, "en")
	}
	if got := wireGroup("firstparty"); got != "firstparty" {
		t.Fatalf("firstparty: got %q want %q", got, "firstparty")
	}
	if got := wireGroup("ru"); got != "ru" {
		t.Fatalf("ru: got %q want %q", got, "ru")
	}
}
