package service

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// regateProvider is a minimal registered provider for re-gate order assertions.
func regateProvider(name string) *fakeProvider {
	return &fakeProvider{
		nameVal: name,
		listEpisodesFn: func(ctx context.Context, id string) ([]domain.Episode, error) {
			return []domain.Episode{{ID: name}}, nil
		},
	}
}

// TestOrchestrator_ApplyStatuses_EnabledToDegraded: a provider enabled at boot
// is dropped from the auto-failover order after ApplyStatuses marks it degraded
// (but stays reachable via an explicit prefer).
func TestOrchestrator_ApplyStatuses_EnabledToDegraded(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(logger.Default(), domain.NewRegistry(), nil)
	a := regateProvider("gogoanime")
	b := regateProvider("nineanime")
	o.Register(a)
	o.Register(b)

	// Both in the auto order initially.
	if got := o.OrderedProviderNames("", false); len(got) != 2 {
		t.Fatalf("initial auto order = %v; want both", got)
	}

	o.ApplyStatuses(map[string]string{"gogoanime": "enabled", "nineanime": "degraded"})

	auto := o.OrderedProviderNames("", false)
	if len(auto) != 1 || auto[0] != "gogoanime" {
		t.Errorf("auto order = %v; want [gogoanime] (nineanime degraded out)", auto)
	}
	// Still reachable via explicit prefer.
	pref := o.OrderedProviderNames("nineanime", false)
	if len(pref) != 2 || pref[0] != "nineanime" {
		t.Errorf("prefer order = %v; want [nineanime, gogoanime]", pref)
	}
}

// TestOrchestrator_ApplyStatuses_DegradedToEnabled: a provider degraded at boot
// rejoins the auto order after ApplyStatuses marks it enabled.
func TestOrchestrator_ApplyStatuses_DegradedToEnabled(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(logger.Default(), domain.NewRegistry(), nil)
	a := regateProvider("gogoanime")
	d := regateProvider("nineanime")
	o.Register(a)
	o.RegisterDegraded(d) // boot-degraded

	if got := o.OrderedProviderNames("", false); len(got) != 1 || got[0] != "gogoanime" {
		t.Fatalf("initial auto order = %v; want [gogoanime] only", got)
	}

	o.ApplyStatuses(map[string]string{"gogoanime": "enabled", "nineanime": "enabled"})

	auto := o.OrderedProviderNames("", false)
	if len(auto) != 2 {
		t.Errorf("auto order = %v; want both after re-enable", auto)
	}
}

// TestOrchestrator_ApplyStatuses_UnknownAndDisabledIgnored: names not registered
// in the orchestrator, and a "disabled"/garbage status value, are no-ops (no
// panic, no spurious add). Registered providers absent from the map keep their
// current degraded state.
func TestOrchestrator_ApplyStatuses_UnknownAndDisabledIgnored(t *testing.T) {
	t.Parallel()
	o := NewOrchestrator(logger.Default(), domain.NewRegistry(), nil)
	a := regateProvider("gogoanime")
	d := regateProvider("nineanime")
	o.Register(a)
	o.RegisterDegraded(d)

	o.ApplyStatuses(map[string]string{
		"gogoanime":   "enabled",
		"nineanime":   "disabled", // not enabled/degraded -> ignored, stays degraded
		"allanime":    "enabled",  // not registered -> ignored, must NOT be added
		"bogus_value": "weird",
	})

	auto := o.OrderedProviderNames("", false)
	if len(auto) != 1 || auto[0] != "gogoanime" {
		t.Errorf("auto order = %v; want [gogoanime] (nineanime stays degraded, allanime never added)", auto)
	}
	// allanime must not have been added to the provider set.
	for _, n := range o.OrderedProviderNames("allanime", false) {
		if n == "allanime" {
			t.Errorf("allanime appeared in provider set; ApplyStatuses must not add unregistered providers")
		}
	}
}
