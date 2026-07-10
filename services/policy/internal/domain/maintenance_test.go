package domain

import (
	"encoding/json"
	"testing"
)

func TestSeedRoutines_ParityDefaults(t *testing.T) {
	rows := SeedRoutines()
	if len(rows) != 9 {
		t.Fatalf("want 9 seeded routines, got %d", len(rows))
	}
	byID := map[string]MaintenanceRoutine{}
	for _, r := range rows {
		if !r.Enabled {
			t.Errorf("routine %q seeded disabled; day-one parity requires enabled=true", r.ID)
		}
		if !json.Valid(r.Settings.raw()) {
			t.Errorf("routine %q settings is not valid JSON: %s", r.ID, string(r.Settings))
		}
		byID[r.ID] = r
	}
	for _, id := range []string{
		"maintenance_bot", "provider_recovery", "git_autosync", "disk_prune",
		"build_cache_prune", "subtitle_probe", "shikimori_sync", "playability_canary",
		"provider_self_heal",
	} {
		if _, ok := byID[id]; !ok {
			t.Errorf("missing seeded routine %q", id)
		}
	}
	var bot map[string]any
	if err := json.Unmarshal(byID["maintenance_bot"].Settings.raw(), &bot); err != nil {
		t.Fatalf("bot settings unmarshal: %v", err)
	}
	if bot["auto_apply_max_risk"] != "medium" {
		t.Errorf("bot auto_apply_max_risk = %v; want medium (current behavior)", bot["auto_apply_max_risk"])
	}
}

func TestSettingsJSON_ScanValueRoundTrip(t *testing.T) {
	var s SettingsJSON
	if err := s.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if string(s) != "{}" {
		t.Errorf("nil scan = %q; want {}", string(s))
	}
	if err := s.Scan(`{"model":"opus"}`); err != nil {
		t.Fatalf("scan string: %v", err)
	}
	v, err := s.Value()
	if err != nil {
		t.Fatalf("value: %v", err)
	}
	if v.(string) != `{"model":"opus"}` {
		t.Errorf("value = %v; want {\"model\":\"opus\"}", v)
	}
}
