package config

import "testing"

func TestProvidersConfig_RegistrationTriState(t *testing.T) {
	pc := NewProvidersConfigForTest([]ProviderMeta{
		{Name: "gogoanime", Status: StatusEnabled},
		{Name: "animepahe", Status: StatusDisabled},
		{Name: "allanime", Status: StatusDegraded},
	})
	// enabled → registered + in auto-failover.
	if !pc.IsRegistered("gogoanime") || !pc.IsEnabled("gogoanime") {
		t.Error("gogoanime is enabled; must be registered and in auto-failover")
	}
	// disabled → not registered.
	if pc.IsRegistered("animepahe") {
		t.Error("animepahe is disabled; must NOT be registered")
	}
	// soft-degraded → registered but excluded from auto-failover.
	if !pc.IsRegistered("allanime") || pc.IsEnabled("allanime") {
		t.Error("allanime is soft-degraded; must be registered but NOT in auto-failover")
	}
	if !pc.IsSoftDegraded("allanime") || pc.IsSoftDegraded("gogoanime") {
		t.Error("IsSoftDegraded wrong: only allanime should be soft-degraded")
	}
}
