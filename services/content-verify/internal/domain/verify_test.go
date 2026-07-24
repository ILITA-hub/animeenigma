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
		{Status: StatusVerified, Episodes: 12, Audio: &AudioVerdict{Lang: "ja", Confidence: 0.98, Verified: true}},
		{Status: StatusVerified, Episodes: 28, Audio: &AudioVerdict{Lang: "en", Confidence: 0.97, Verified: true},
			Hardsub: &HardsubVerdict{Present: true, Lang: "ru", Confidence: 0.96, Verified: true}},
		{Status: StatusInconclusive},
	}
	s := Summarize(units)
	if s.Status != "partial" || !s.Raw {
		t.Fatalf("summary = %+v", s)
	}
	if s.Episodes != 28 { // max across units, unknown (0) units ignored
		t.Fatalf("episodes = %d, want 28", s.Episodes)
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

// TestSummarizeHardsubOnlyPartial covers a unit whose audio verdict is
// inconclusive (no verified count, no Raw, no DubLangs) but whose hardsub
// verdict IS verified — a burned-in-subs provider the audio prober couldn't
// pin down. Before this fix the partial condition only checked
// verified/Raw/DubLangs, so this unit fell through to "unverified" even
// though HardsubLangs was non-empty — the FE would then render the
// unverified marker AND a verified hardsub badge on the same row.
func TestSummarizeHardsubOnlyPartial(t *testing.T) {
	units := []UnitVerdict{
		{Status: StatusInconclusive, Hardsub: &HardsubVerdict{Present: true, Lang: "en", Confidence: 0.96, Verified: true}},
	}
	s := Summarize(units)
	if s.Status != "partial" {
		t.Fatalf("status = %q, want partial", s.Status)
	}
	if len(s.HardsubLangs) != 1 || s.HardsubLangs[0] != "en" {
		t.Fatalf("hardsub_langs = %v, want [en]", s.HardsubLangs)
	}
	if s.Raw || len(s.DubLangs) != 0 {
		t.Fatalf("unexpected raw/dub_langs = raw=%v dub_langs=%v", s.Raw, s.DubLangs)
	}
}

// TestSummarizeUnreachable locks the provider-level "may not work" rollup: the
// flag is set ONLY when every probed unit is StatusUnreachable. Any reachable
// unit (verified or inconclusive) clears it, and an empty unit list is never
// flagged (never probed ≠ unreachable).
func TestSummarizeUnreachable(t *testing.T) {
	allDead := []UnitVerdict{
		{Status: StatusUnreachable, Fails: 3},
		{Status: StatusUnreachable, Fails: 1},
	}
	if s := Summarize(allDead); !s.Unreachable {
		t.Fatalf("all-unreachable must flag unreachable: %+v", s)
	}
	// One inconclusive unit means the provider is at least partially reachable.
	mixed := []UnitVerdict{
		{Status: StatusUnreachable},
		{Status: StatusInconclusive},
	}
	if s := Summarize(mixed); s.Unreachable {
		t.Fatalf("mixed reachability must NOT flag unreachable: %+v", s)
	}
	// A verified unit obviously works.
	withVerified := []UnitVerdict{
		{Status: StatusUnreachable},
		{Status: StatusVerified, Audio: &AudioVerdict{Lang: "en", Verified: true}},
	}
	if s := Summarize(withVerified); s.Unreachable {
		t.Fatalf("any verified unit must clear unreachable: %+v", s)
	}
	// Never probed → not flagged.
	if Summarize(nil).Unreachable {
		t.Fatal("empty unit list must not flag unreachable")
	}
}
