package capability

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// aeFam wraps a single first-party ae cap in a report.
func aeFam(lang, state string) domain.CapabilityReport {
	return domain.CapabilityReport{AnimeID: "anime-1", Families: []domain.SourceFamily{{
		Family: "aeProvider",
		Providers: []domain.ProviderCap{{
			Provider: "ae", Group: "firstparty", State: state, Lang: lang,
		}},
	}}}
}

// kodikFam wraps a single kodik cap (real-team variants) in a report. episodes
// is the cap-level ready count (max across teams, as kodikFamily sets natively).
func kodikFam(state string, episodes int, variants ...domain.Variant) domain.CapabilityReport {
	return domain.CapabilityReport{AnimeID: "anime-1", Families: []domain.SourceFamily{{
		Family: "others",
		Providers: []domain.ProviderCap{{
			Provider: "kodik", Group: "ru", State: state, Episodes: episodes, Variants: variants,
		}},
	}}}
}

func findVerdict(vs []providerVerdictWire, provider string) (providerVerdictWire, bool) {
	for _, v := range vs {
		if v.Provider == provider {
			return v, true
		}
	}
	return providerVerdictWire{}, false
}

func TestSynthVerdicts_AE_En(t *testing.T) {
	vs := synthVerdicts(aeFam("en", "active"))
	ae, ok := findVerdict(vs, "ae")
	if !ok {
		t.Fatalf("expected an ae verdict, got %+v", vs)
	}
	if len(ae.Units) != 1 {
		t.Fatalf("expected 1 ae unit, got %d", len(ae.Units))
	}
	u := ae.Units[0]
	if u.Key != (unitKeyWire{Track: "default"}) {
		t.Fatalf("ae unit key = %+v, want Track:default", u.Key)
	}
	if u.Status != "verified" || u.Audio == nil || u.Audio.Lang != "en" || !u.Audio.Verified || u.Audio.Confidence != 1.0 {
		t.Fatalf("ae unit audio = %+v (status %q)", u.Audio, u.Status)
	}
	if u.ProbedAt.IsZero() {
		t.Fatal("ae unit probed_at should be stamped")
	}
	want := providerSummaryWire{Status: "verified", Raw: false, DubLangs: []string{"en"}, HardsubLangs: []string{}}
	if !reflect.DeepEqual(ae.Summary, want) {
		t.Fatalf("ae summary = %+v, want %+v", ae.Summary, want)
	}
}

func TestSynthVerdicts_AE_Ja(t *testing.T) {
	// Empty Lang → original audio "ja" → raw, no dub langs.
	vs := synthVerdicts(aeFam("", "active"))
	ae, ok := findVerdict(vs, "ae")
	if !ok {
		t.Fatalf("expected an ae verdict, got %+v", vs)
	}
	if ae.Units[0].Audio.Lang != "ja" {
		t.Fatalf("ae ja unit lang = %q, want ja", ae.Units[0].Audio.Lang)
	}
	want := providerSummaryWire{Status: "verified", Raw: true, DubLangs: []string{}, HardsubLangs: []string{}}
	if !reflect.DeepEqual(ae.Summary, want) {
		t.Fatalf("ae ja summary = %+v, want %+v", ae.Summary, want)
	}
}

func TestSynthVerdicts_Kodik_DubAndSub(t *testing.T) {
	report := kodikFam("active", 28,
		domain.Variant{Category: "dub", Team: &domain.Team{ID: "5", Name: "AniDub"}},
		domain.Variant{Category: "sub", Team: &domain.Team{ID: "6", Name: "SubTeam"}},
	)
	vs := synthVerdicts(report)
	k, ok := findVerdict(vs, "kodik")
	if !ok {
		t.Fatalf("expected a kodik verdict, got %+v", vs)
	}
	if len(k.Units) != 2 {
		t.Fatalf("expected 2 kodik units, got %d: %+v", len(k.Units), k.Units)
	}

	// dub team (voice): ru audio verified.
	dub := k.Units[0]
	if dub.Key != (unitKeyWire{Team: "5", Category: "dub"}) {
		t.Fatalf("dub key = %+v", dub.Key)
	}
	if dub.Audio == nil || dub.Audio.Lang != "ru" || !dub.Audio.Verified || dub.Episode != 28 || dub.Episodes != 28 {
		t.Fatalf("dub unit = %+v", dub)
	}
	// sub team (subtitles): original audio + burned ru hardsub, no audio verdict.
	sub := k.Units[1]
	if sub.Key != (unitKeyWire{Team: "6", Category: "sub"}) {
		t.Fatalf("sub key = %+v", sub.Key)
	}
	if !sub.RawAudio || sub.Audio != nil {
		t.Fatalf("sub unit should be raw_audio with no audio verdict: %+v", sub)
	}
	if sub.Hardsub == nil || !sub.Hardsub.Present || !sub.Hardsub.Verified || sub.Hardsub.Lang != "ru" {
		t.Fatalf("sub hardsub = %+v", sub.Hardsub)
	}

	want := providerSummaryWire{Status: "verified", Raw: true, DubLangs: []string{"ru"}, HardsubLangs: []string{"ru"}, Episodes: 28}
	if !reflect.DeepEqual(k.Summary, want) {
		t.Fatalf("kodik summary = %+v, want %+v", k.Summary, want)
	}
}

func TestSynthVerdicts_NoContentAndAdult_Skipped(t *testing.T) {
	// no_content ae (empty library) is not synthesized.
	if vs := synthVerdicts(aeFam("en", "no_content")); len(vs) != 0 {
		t.Fatalf("no_content ae should not synth, got %+v", vs)
	}
	// no_content kodik whose variants are trait-only (no real team) is not synthesized.
	report := kodikFam("no_content", 0, domain.Variant{Category: "dub"}, domain.Variant{Category: "sub"})
	if vs := synthVerdicts(report); len(vs) != 0 {
		t.Fatalf("no_content kodik should not synth, got %+v", vs)
	}
	// A kodik cap in content state but carrying only teamless trait variants
	// yields no unit → dropped entirely (defensive).
	teamless := kodikFam("active", 0, domain.Variant{Category: "dub"})
	if vs := synthVerdicts(teamless); len(vs) != 0 {
		t.Fatalf("teamless kodik should not synth, got %+v", vs)
	}
	// Adult group is skipped by the guard (defensive parity with old enumerate).
	adult := domain.CapabilityReport{Families: []domain.SourceFamily{{
		Family:    "18+",
		Providers: []domain.ProviderCap{{Provider: "hanime", Group: "adult", State: "active"}},
	}}}
	if vs := synthVerdicts(adult); len(vs) != 0 {
		t.Fatalf("adult cap should not synth, got %+v", vs)
	}
}

// TestSynthSummaries_Parity locks the ported Summarize outputs (the public
// summary shape service.go blends) across the four synth cases.
func TestSynthSummaries_Parity(t *testing.T) {
	cases := []struct {
		name   string
		report domain.CapabilityReport
		key    string
		want   domain.VerifySummary
	}{
		{"ae-en", aeFam("en", "active"), "ae",
			domain.VerifySummary{Status: "verified", Raw: false, DubLangs: []string{"en"}, HardsubLangs: []string{}}},
		{"ae-ja", aeFam("", "active"), "ae",
			domain.VerifySummary{Status: "verified", Raw: true, DubLangs: []string{}, HardsubLangs: []string{}}},
		{"kodik-dub", kodikFam("active", 28, domain.Variant{Category: "dub", Team: &domain.Team{ID: "5"}}), "kodik",
			domain.VerifySummary{Status: "verified", Raw: false, DubLangs: []string{"ru"}, HardsubLangs: []string{}, Episodes: 28}},
		{"kodik-sub", kodikFam("active", 28, domain.Variant{Category: "sub", Team: &domain.Team{ID: "6"}}), "kodik",
			domain.VerifySummary{Status: "verified", Raw: true, DubLangs: []string{}, HardsubLangs: []string{"ru"}, Episodes: 28}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SynthSummaries(tc.report)
			s, ok := got[tc.key]
			if !ok {
				t.Fatalf("missing %q summary in %+v", tc.key, got)
			}
			// ae/kodik synth units are always verified → never "may not work".
			if s.Unreachable {
				t.Fatalf("synth summary must never be unreachable: %+v", s)
			}
			if !reflect.DeepEqual(s, tc.want) {
				t.Fatalf("summary = %+v, want %+v", s, tc.want)
			}
		})
	}
}

// TestSynthUnit_WireShape asserts the synthesized JSON carries content-verify's
// snake_case tags (byte-shape the FE + old store shared): key.track, audio with
// confidence/verified, probed_at, and a zero-valued sample object.
func TestSynthUnit_WireShape(t *testing.T) {
	vs := synthVerdicts(aeFam("ru", "active"))
	b, err := json.Marshal(vs[0])
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	units := m["units"].([]any)
	u := units[0].(map[string]any)
	key := u["key"].(map[string]any)
	if key["track"] != "default" {
		t.Fatalf("key.track = %v", key["track"])
	}
	audio := u["audio"].(map[string]any)
	if audio["lang"] != "ru" || audio["verified"] != true || audio["confidence"].(float64) != 1 {
		t.Fatalf("audio = %+v", audio)
	}
	if _, ok := u["probed_at"]; !ok {
		t.Fatal("expected probed_at on the wire")
	}
	if _, ok := u["sample"].(map[string]any); !ok {
		t.Fatalf("expected sample object, got %v", u["sample"])
	}
}

func TestMergeSynthVerdicts_DropsStaleAndAppends(t *testing.T) {
	// Upstream still returns a stale kodik entry plus a real gogoanime entry.
	raw := json.RawMessage(`{"anime_id":"abc","providers":[` +
		`{"provider":"gogoanime","summary":{"status":"verified","raw":true,"dub_langs":[],"hardsub_langs":[]},"units":[]},` +
		`{"provider":"kodik","summary":{"status":"verified","raw":false,"dub_langs":["ru"],"hardsub_langs":[]},"units":[]}` +
		`]}`)
	report := kodikFam("active", 12, domain.Variant{Category: "dub", Team: &domain.Team{ID: "9", Name: "T"}})

	merged := MergeSynthVerdicts("abc", raw, report)
	var out struct {
		AnimeID   string `json:"anime_id"`
		Providers []struct {
			Provider string `json:"provider"`
			Units    []struct {
				Key struct {
					Team string `json:"team"`
				} `json:"key"`
			} `json:"units"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("decode: %v (merged=%s)", err, merged)
	}
	if out.AnimeID != "abc" {
		t.Fatalf("anime_id = %q", out.AnimeID)
	}
	// Exactly one gogoanime (verbatim) and exactly one kodik (synth, not stale).
	byProv := map[string]int{}
	var kodikTeam string
	for _, p := range out.Providers {
		byProv[p.Provider]++
		if p.Provider == "kodik" && len(p.Units) == 1 {
			kodikTeam = p.Units[0].Key.Team
		}
	}
	if byProv["gogoanime"] != 1 || byProv["kodik"] != 1 {
		t.Fatalf("provider counts = %+v (want one each, stale kodik dropped)", byProv)
	}
	if kodikTeam != "9" {
		t.Fatalf("kodik synth team = %q, want 9 (proves synth replaced the stale empty entry)", kodikTeam)
	}
}

func TestMergeSynthVerdicts_EmptyRaw_SynthOnly(t *testing.T) {
	// content-verify down (nil raw): still surface ae/kodik synth.
	merged := MergeSynthVerdicts("xyz", nil, aeFam("en", "active"))
	var out struct {
		AnimeID   string `json:"anime_id"`
		Providers []struct {
			Provider string `json:"provider"`
		} `json:"providers"`
	}
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AnimeID != "xyz" || len(out.Providers) != 1 || out.Providers[0].Provider != "ae" {
		t.Fatalf("expected synth-only ae payload, got %+v", out)
	}
}

func TestMergeSynthVerdicts_EmptyRawNoReport_EmptyProviders(t *testing.T) {
	// Both down: degrade to {providers:[]} (never null).
	merged := MergeSynthVerdicts("none", nil, domain.CapabilityReport{})
	var out struct {
		AnimeID   string            `json:"anime_id"`
		Providers []json.RawMessage `json:"providers"`
	}
	if err := json.Unmarshal(merged, &out); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if out.AnimeID != "none" || out.Providers == nil || len(out.Providers) != 0 {
		t.Fatalf("expected empty non-null providers, got %+v (merged=%s)", out, merged)
	}
}
