package capability_test

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/capability"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fakeHealth struct {
	up       map[string]bool
	playable map[string]bool
}

func (f fakeHealth) ProviderHealth(_ context.Context) (map[string]capability.HealthInfo, error) {
	out := map[string]capability.HealthInfo{}
	for n, u := range f.up {
		hi := capability.HealthInfo{Up: u}
		if pb, ok := f.playable[n]; ok {
			v := pb
			hi.Playable = &v
		}
		out[n] = hi
	}
	return out, nil
}

func newDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err := db.AutoMigrate(&domain.ScraperProvider{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestBuildENFamily_RanksAndFiltersDisabled(t *testing.T) {
	db := newDB(t)
	db.Create(&domain.ScraperProvider{Name: "allanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, SupportsDub: true, SubDelivery: "hard", QualityCeiling: "1080p", PreferenceWeight: 90})
	db.Create(&domain.ScraperProvider{Name: "nineanime", Status: domain.StatusEnabled, Group: "en", SupportsSub: true, SubDelivery: "hard", QualityCeiling: "720p", PreferenceWeight: 40})
	db.Create(&domain.ScraperProvider{Name: "animepahe", Status: domain.StatusDisabled, Group: "en", SupportsSub: true, SupportsDub: true, SubDelivery: "hard", PreferenceWeight: 30})
	db.Create(&domain.ScraperProvider{Name: "18anime", Status: domain.StatusEnabled, Group: "adult", SupportsRaw: true, PreferenceWeight: 0})

	svc := capability.NewService(db, fakeHealth{
		up:       map[string]bool{"allanime": true, "nineanime": true},
		playable: map[string]bool{"allanime": true},
	}, nil, nil, nil, nil, nil)

	fam, err := svc.BuildENFamily(context.Background())
	if err != nil {
		t.Fatalf("BuildENFamily: %v", err)
	}
	if fam.Family != "ourenglish" {
		t.Errorf("family = %q", fam.Family)
	}
	if len(fam.Providers) != 2 {
		t.Fatalf("want 2 providers, got %d (%+v)", len(fam.Providers), fam.Providers)
	}
	if fam.Providers[0].Provider != "allanime" {
		t.Errorf("rank order wrong: %+v", fam.Providers)
	}
	var sawDub bool
	for _, v := range fam.Providers[0].Variants {
		if v.Category == "dub" {
			sawDub = true
		}
	}
	if !sawDub {
		t.Errorf("allanime should advertise a dub variant: %+v", fam.Providers[0].Variants)
	}
}

func TestBuildENFamilyPopulatesFeedFields(t *testing.T) {
	db := newDB(t)
	db.Create(&domain.ScraperProvider{
		Name: "gogoanime", Status: domain.StatusEnabled, Policy: domain.PolicyAuto, Health: domain.HealthUp,
		Group: "en", PreferenceWeight: 85, SupportsSub: true, SupportsDub: true, Reason: "live",
	})
	db.Create(&domain.ScraperProvider{
		// A degraded provider is pinned out of the auto chain via policy=manual; the
		// stored status column mirrors it here but the feed derives state from policy.
		Name: "animefever", Status: domain.StatusDegraded, Policy: domain.PolicyManual, Health: domain.HealthDown,
		Group: "en", PreferenceWeight: 60, SupportsSub: true, Reason: "ad-substitution",
	})

	svc := capability.NewService(db, nil, nil, nil, nil, nil, nil)
	fam, err := svc.BuildENFamily(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	byName := map[string]domain.ProviderCap{}
	for _, p := range fam.Providers {
		byName[p.Provider] = p
	}
	gg := byName["gogoanime"]
	if gg.State != "active" || !gg.Selectable || gg.HackerOnly || gg.Order != 85 ||
		gg.Group != "en" || len(gg.Audios) != 2 {
		t.Fatalf("gogoanime feed fields wrong: %+v", gg)
	}
	af := byName["animefever"]
	if af.State != "degraded" || !af.Selectable || !af.HackerOnly || af.Reason != "ad-substitution" {
		t.Fatalf("animefever feed fields wrong: %+v", af)
	}
}
