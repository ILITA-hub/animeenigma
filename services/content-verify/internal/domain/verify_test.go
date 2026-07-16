package domain

import "testing"

func TestUnitKeyString(t *testing.T) {
	k := UnitKey{Server: "hd-1", Category: "sub"}
	if got := k.String(); got != "category=sub|server=hd-1" {
		t.Fatalf("key = %q", got)
	}
	if (UnitKey{Team: "610"}).String() != "team=610" {
		t.Fatal("team-only key wrong")
	}
}

func TestSummarize(t *testing.T) {
	units := []UnitVerdict{
		{Status: StatusVerified, Audio: &AudioVerdict{Lang: "ja", Confidence: 0.98, Verified: true}},
		{Status: StatusVerified, Audio: &AudioVerdict{Lang: "en", Confidence: 0.97, Verified: true},
			Hardsub: &HardsubVerdict{Present: true, Lang: "ru", Confidence: 0.96, Verified: true}},
		{Status: StatusInconclusive},
	}
	s := Summarize(units)
	if s.Status != "partial" || !s.Raw {
		t.Fatalf("summary = %+v", s)
	}
	if len(s.DubLangs) != 1 || s.DubLangs[0] != "en" {
		t.Fatalf("dub_langs = %v", s.DubLangs)
	}
	if len(s.HardsubLangs) != 1 || s.HardsubLangs[0] != "ru" {
		t.Fatalf("hardsub_langs = %v", s.HardsubLangs)
	}
	if Summarize(nil).Status != "unverified" {
		t.Fatal("empty must be unverified")
	}
	all := []UnitVerdict{{Status: StatusVerified, Audio: &AudioVerdict{Lang: "en", Verified: true}}}
	if Summarize(all).Status != "verified" {
		t.Fatal("all-verified must be verified")
	}
}
