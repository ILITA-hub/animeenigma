package domain_test

import (
	"encoding/json"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestCapabilityReport_RoundTrip(t *testing.T) {
	pb := true
	in := domain.CapabilityReport{
		AnimeID: "uuid-1",
		Families: []domain.SourceFamily{{
			Family: "ourenglish",
			Providers: []domain.ProviderCap{{
				Provider: "allanime", DisplayName: "AllAnime", Enabled: true,
				Health: "up", Playable: &pb, Rank: 130,
				Variants: []domain.Variant{{
					Category: "dub", SubDelivery: "none", Qualities: []string{"1080p"},
					QualitySource: "trait", Source: "trait",
				}},
			}},
		}},
	}
	b, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out domain.CapabilityReport
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if out.Families[0].Providers[0].Variants[0].Category != "dub" || out.Families[0].Providers[0].Rank != 130 {
		t.Errorf("round-trip mismatch: %+v", out)
	}
	if out.Families[0].Providers[0].Playable == nil || !*out.Families[0].Providers[0].Playable {
		t.Errorf("playable not preserved")
	}
}
