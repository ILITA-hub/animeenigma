package capability

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestDeriveProviderView(t *testing.T) {
	cases := []struct {
		name       string
		status     domain.ProviderStatus
		health     domain.ProviderHealth
		hasContent bool
		wantState  string
		wantSel    bool
		wantHacker bool
	}{
		{"enabled up with content", domain.StatusEnabled, domain.HealthUp, true, "active", true, false},
		{"enabled down with content stays active", domain.StatusEnabled, domain.HealthDown, true, "active", true, false},
		{"enabled recovering", domain.StatusEnabled, domain.HealthRecovering, true, "recovering", true, false},
		{"enabled up no content", domain.StatusEnabled, domain.HealthUp, false, "no_content", false, false},
		{"degraded is hacker-only", domain.StatusDegraded, domain.HealthDown, true, "degraded", true, true},
		{"degraded ignores content", domain.StatusDegraded, domain.HealthUp, false, "degraded", true, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			row := domain.ScraperProvider{Status: c.status, Health: c.health}
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
}
