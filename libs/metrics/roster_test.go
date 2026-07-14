package metrics

import (
	"testing"

	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestEmitProviderRoster_SetsInfoAndEnabled(t *testing.T) {
	EmitProviderRoster([]RosterEntry{
		{Name: "t1_enabled", Status: "enabled", Reason: "r-en", Description: "d-en"},
		{Name: "t1_degraded", Status: "degraded", Reason: "r-deg", Description: "d-deg"},
		{Name: "t1_disabled", Status: "disabled", Reason: "r-dis", Description: "d-dis"},
	})

	// provider_enabled: 1 ONLY for status=="enabled".
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("t1_enabled")); got != 1 {
		t.Errorf("provider_enabled{t1_enabled} = %v, want 1", got)
	}
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("t1_degraded")); got != 0 {
		t.Errorf("provider_enabled{t1_degraded} = %v, want 0 (degraded is not enabled)", got)
	}
	if got := testutil.ToFloat64(ProviderEnabled.WithLabelValues("t1_disabled")); got != 0 {
		t.Errorf("provider_enabled{t1_disabled} = %v, want 0", got)
	}

	// provider_info: always 1, emitted for ALL rows incl. disabled (roster reflection).
	if got := testutil.ToFloat64(ProviderInfo.WithLabelValues("t1_disabled", "disabled", "r-dis", "d-dis")); got != 1 {
		t.Errorf("provider_info{t1_disabled,...} = %v, want 1 (disabled rows still reflected)", got)
	}
	if got := testutil.ToFloat64(ProviderInfo.WithLabelValues("t1_enabled", "enabled", "r-en", "d-en")); got != 1 {
		t.Errorf("provider_info{t1_enabled,...} = %v, want 1", got)
	}
}

func TestEmitProviderRoster_EmptyIsNoop(t *testing.T) {
	EmitProviderRoster(nil) // must not panic
}

func TestProviderUnwiredGauge(t *testing.T) {
	ProviderUnwired.WithLabelValues("newprov", "scraper").Set(1)
	if v := testutil.ToFloat64(ProviderUnwired.WithLabelValues("newprov", "scraper")); v != 1 {
		t.Fatalf("provider_unwired = %v, want 1", v)
	}
	ProviderUnwired.WithLabelValues("newprov", "scraper").Set(0)
	if v := testutil.ToFloat64(ProviderUnwired.WithLabelValues("newprov", "scraper")); v != 0 {
		t.Fatalf("provider_unwired = %v, want 0", v)
	}
}
