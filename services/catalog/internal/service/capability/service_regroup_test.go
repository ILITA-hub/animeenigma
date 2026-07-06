package capability

import (
	"reflect"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestRegroupFamilies(t *testing.T) {
	in := []domain.SourceFamily{
		{Family: "ae", Providers: []domain.ProviderCap{{Provider: "ae"}}},
		{Family: "ourenglish", Providers: []domain.ProviderCap{{Provider: "gogoanime"}, {Provider: "okru"}}},
		{Family: "adult", Providers: []domain.ProviderCap{{Provider: "18anime"}}},
		{Family: "kodik", Providers: []domain.ProviderCap{{Provider: "kodik"}}},
		{Family: "hanime", Providers: []domain.ProviderCap{{Provider: "hanime"}}},
		{Family: "animejoy-sibnet", Providers: []domain.ProviderCap{{Provider: "animejoy-sibnet"}}},
	}
	out := regroupFamilies(in)

	if len(out) != 3 {
		t.Fatalf("want 3 wire families, got %d", len(out))
	}
	// First-seen label order: ae→aeProvider, ourenglish→others, adult→18+.
	if out[0].Family != "aeProvider" || out[1].Family != "others" || out[2].Family != "18+" {
		t.Fatalf("labels/order = %q, %q, %q", out[0].Family, out[1].Family, out[2].Family)
	}
	// "others" collects EN chain + kodik + animejoy, in input order.
	var others []string
	for _, p := range out[1].Providers {
		others = append(others, p.Provider)
	}
	if want := []string{"gogoanime", "okru", "kodik", "animejoy-sibnet"}; !reflect.DeepEqual(others, want) {
		t.Fatalf("others = %v, want %v", others, want)
	}
	// "18+" collects adult + hanime.
	if len(out[2].Providers) != 2 {
		t.Fatalf("18+ providers = %d, want 2", len(out[2].Providers))
	}
}
