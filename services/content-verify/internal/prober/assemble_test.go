package prober

import (
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"testing"
)

func TestAssembleAudioVerified(t *testing.T) {
	frs := []LIDFragment{
		{Lang: "en", Prob: 0.99, Speech: true, SpeechSeconds: 24},
		{Lang: "en", Prob: 0.97, Speech: true, SpeechSeconds: 22},
		{Lang: "en", Prob: 0.95, Speech: true, SpeechSeconds: 25},
		{Lang: "ja", Prob: 0.40, Speech: false}, // non-speech ignored
	}
	v := AssembleAudio(frs)
	if v == nil || !v.Verified || v.Lang != "en" {
		t.Fatalf("verdict: %+v", v)
	}
	if v.Confidence < 0.95 {
		t.Fatalf("confidence: %f", v.Confidence)
	}
}

func TestAssembleAudioDisagreementNotVerified(t *testing.T) {
	frs := []LIDFragment{
		{Lang: "en", Prob: 0.99, Speech: true}, {Lang: "ru", Prob: 0.98, Speech: true}, {Lang: "en", Prob: 0.97, Speech: true},
	}
	if v := AssembleAudio(frs); v == nil || v.Verified {
		t.Fatalf("disagreement must not verify: %+v", v)
	}
}

func TestAssembleAudioTooFewSpeech(t *testing.T) {
	frs := []LIDFragment{{Lang: "en", Prob: 0.99, Speech: true}, {Lang: "en", Prob: 0.99, Speech: true}}
	if v := AssembleAudio(frs); v != nil && v.Verified {
		t.Fatal("2 speech fragments must not verify")
	}
}

func TestAssembleHardsub(t *testing.T) {
	h := &HardsubResult{Frames: 15, Tier1Hits: 6, OCRReal: 3, Script: "cyrillic"}
	v := AssembleHardsub(h)
	if !v.Present || !v.Verified || v.Lang != "ru" || v.Confidence < 0.95 {
		t.Fatalf("verdict: %+v", v)
	}
	clean := AssembleHardsub(&HardsubResult{Frames: 15, Tier1Hits: 0, OCRReal: 0, Script: "none"})
	if clean.Present || clean.Verified {
		t.Fatalf("clean must be present=false, verified=false (negative claims are never badged): %+v", clean)
	}
	weak := AssembleHardsub(&HardsubResult{Frames: 15, Tier1Hits: 3, OCRReal: 1, Script: "latin"})
	if !weak.Present || weak.Verified {
		t.Fatalf("1 OCR hit → present but NOT verified: %+v", weak)
	}
}

// Live 2026-07-17 case (kodik team 1978, Dream Cast RU dub): whisper-tiny
// caps clean RU speech at ≈0.92 raw — three unanimous fragments must
// compound past the 0.95 verified threshold.
func TestAssembleAudioUnanimousCompounding(t *testing.T) {
	frs := []LIDFragment{
		{Lang: "ru", Prob: 0.92, Speech: true, SpeechSeconds: 24},
		{Lang: "ru", Prob: 0.92, Speech: true, SpeechSeconds: 23},
		{Lang: "ru", Prob: 0.92, Speech: true, SpeechSeconds: 24},
	}
	v := AssembleAudio(frs)
	if v == nil || !v.Verified || v.Lang != "ru" {
		t.Fatalf("unanimous ru@0.92 must verify via compounding: %+v", v)
	}
	if v.Confidence < 0.95 || v.Confidence > 1.0 {
		t.Fatalf("compounded confidence out of range: %f", v.Confidence)
	}
}

// Weak evidence must NOT be laundered by compounding: unanimous but below
// the compounding floor stays at its raw mean, unverified.
func TestAssembleAudioWeakUnanimousStaysRaw(t *testing.T) {
	frs := []LIDFragment{
		{Lang: "ru", Prob: 0.70, Speech: true}, {Lang: "ru", Prob: 0.70, Speech: true}, {Lang: "ru", Prob: 0.70, Speech: true},
	}
	v := AssembleAudio(frs)
	if v == nil || v.Verified {
		t.Fatalf("weak unanimous must stay unverified: %+v", v)
	}
	if v.Confidence < 0.699 || v.Confidence > 0.701 {
		t.Fatalf("weak evidence must keep its raw mean (~0.70), got %f", v.Confidence)
	}
}

func TestNeedsMoreFragments(t *testing.T) {
	speech3 := []LIDFragment{{Speech: true, Lang: "ru"}, {Speech: true, Lang: "ru"}, {Speech: true, Lang: "ru"}}
	// borderline band: unanimous, compounded but < threshold
	if !needsMoreFragments(&domain.AudioVerdict{Confidence: 0.90}, speech3) {
		t.Fatal("borderline unanimous verdict must request more fragments")
	}
	// already verified: no extension
	if needsMoreFragments(&domain.AudioVerdict{Confidence: 0.97, Verified: true}, speech3) {
		t.Fatal("verified verdict must not request more fragments")
	}
	// weak / conflicting (< floor): answered, no extension
	if needsMoreFragments(&domain.AudioVerdict{Confidence: 0.49}, speech3) {
		t.Fatal("sub-floor verdict must not request more fragments")
	}
	// speech shortage always extends
	if !needsMoreFragments(nil, speech3[:1]) {
		t.Fatal("speech shortage must request more fragments")
	}
}

// Live FP 2026-07-17 (kodik AniRise): episode typography / title cards hit a
// few frames and can OCR "real" — that must stay below the verified bar,
// which demands MOST frames texty (real dialogue subs) — tier1 >= frames/3.
func TestAssembleHardsubTypographyNotVerified(t *testing.T) {
	fp := AssembleHardsub(&HardsubResult{Frames: 15, Tier1Hits: 4, OCRReal: 3, Script: "latin"})
	if !fp.Present || fp.Verified {
		t.Fatalf("sparse texty frames (typography) must be present but NOT verified: %+v", fp)
	}
}
