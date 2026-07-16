package prober

import (
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
