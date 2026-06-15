package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadProviders_ParsesTraits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "providers.yaml")
	yaml := `providers:
  - name: allanime
    enabled: true
    supports_sub: true
    supports_dub: true
    supports_raw: false
    sub_delivery: hard
    quality_ceiling: 1080p
    preference_weight: 90
  - name: nineanime
    enabled: true
`
	if err := os.WriteFile(path, []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	pc, err := LoadProviders(path)
	if err != nil {
		t.Fatalf("LoadProviders: %v", err)
	}
	all := pc.Meta("allanime")
	if !all.SupportsSub || !all.SupportsDub || all.SupportsRaw {
		t.Errorf("allanime sub/dub/raw = %v/%v/%v, want true/true/false", all.SupportsSub, all.SupportsDub, all.SupportsRaw)
	}
	if all.SubDelivery != "hard" || all.QualityCeiling != "1080p" || all.PreferenceWeight != 90 {
		t.Errorf("allanime traits = %q/%q/%d", all.SubDelivery, all.QualityCeiling, all.PreferenceWeight)
	}
	nine := pc.Meta("nineanime")
	if nine.SupportsSub || nine.SubDelivery != "hard" {
		t.Errorf("nineanime defaults = sub %v delivery %q, want false/hard", nine.SupportsSub, nine.SubDelivery)
	}
}
