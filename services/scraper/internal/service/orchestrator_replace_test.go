package service

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

func TestOrchestratorReplaceProvidersAddsRemovesAndReorders(t *testing.T) {
	o := NewOrchestrator(logger.Default(), domain.NewRegistry(), nil)
	a := &health.FakeProvider{NameVal: "a"}
	b := &health.FakeProvider{NameVal: "b"}
	c := &health.FakeProvider{NameVal: "c"}
	o.Register(a)

	o.ReplaceProviders([]domain.Provider{c, b}, map[string]bool{"b": true})

	if got := o.OrderedProviderNames("", false); len(got) != 1 || got[0] != "c" {
		t.Fatalf("auto order = %v, want [c]", got)
	}
	if got := o.OrderedProviderNames("b", true); len(got) != 1 || got[0] != "b" {
		t.Fatalf("preferred degraded order = %v, want [b]", got)
	}
	for _, p := range o.RegisteredProviders() {
		if p.Name() == "a" {
			t.Fatal("removed provider a remained registered")
		}
	}
}
