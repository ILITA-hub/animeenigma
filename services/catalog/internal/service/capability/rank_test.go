package capability

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestRankEN_OrdersByWeightHealthPlayable(t *testing.T) {
	up, down := "up", "down"
	yes, no := true, false
	a := rankEN(domain.ScraperProvider{PreferenceWeight: 90, QualityCeiling: "1080p", SubDelivery: "hard"}, up, &yes)
	b := rankEN(domain.ScraperProvider{PreferenceWeight: 40, QualityCeiling: "720p", SubDelivery: "hard"}, up, &yes)
	if a <= b {
		t.Errorf("weight 90 (%v) should outrank weight 40 (%v)", a, b)
	}
	downHi := rankEN(domain.ScraperProvider{PreferenceWeight: 90, QualityCeiling: "1080p"}, down, &no)
	upLo := rankEN(domain.ScraperProvider{PreferenceWeight: 40, QualityCeiling: "720p"}, up, &yes)
	if downHi >= upLo {
		t.Errorf("down provider (%v) must rank below up provider (%v)", downHi, upLo)
	}
}

func TestVariantsFromTraits(t *testing.T) {
	row := domain.ScraperProvider{SupportsSub: true, SupportsDub: true, SubDelivery: "hard", QualityCeiling: "1080p"}
	vs := variantsFromTraits(row)
	var sub, dub *domain.Variant
	for i := range vs {
		switch vs[i].Category {
		case "sub":
			sub = &vs[i]
		case "dub":
			dub = &vs[i]
		}
	}
	if sub == nil || dub == nil {
		t.Fatalf("want sub+dub variants, got %+v", vs)
	}
	if sub.SubDelivery != "hard" || sub.Source != "trait" || len(sub.Qualities) != 1 || sub.Qualities[0] != "1080p" {
		t.Errorf("sub variant wrong: %+v", sub)
	}
	if dub.SubDelivery != "none" {
		t.Errorf("dub sub_delivery = %q, want none", dub.SubDelivery)
	}
}
