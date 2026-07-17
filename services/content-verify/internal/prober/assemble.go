package prober

import (
	"math"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// LID / Hardsub result types — the JSON contracts of analyzers/lid.py and
// analyzers/hardsub.py.
type LIDFragment struct {
	Path          string  `json:"path"`
	Lang          string  `json:"lang"`
	Prob          float64 `json:"prob"`
	SpeechSeconds float64 `json:"speech_seconds"`
	Speech        bool    `json:"speech"`
}
type LIDResult struct {
	Fragments []LIDFragment `json:"fragments"`
}
type HardsubResult struct {
	Frames        int     `json:"frames"`
	Tier1Hits     int     `json:"tier1_hits"`
	OCRReal       int     `json:"ocr_real"`
	Script        string  `json:"script"`
	TextStrokeP75 float64 `json:"text_stroke_p75"`
}

const (
	minSpeechFragments = 3

	// Unanimity compounding. Whisper-tiny's raw per-fragment LID prob is a
	// conservative floor on some languages — clean unambiguous RU dialogue
	// sits ≈0.92 (live finding 2026-07-17, kodik team 1978), so demanding a
	// raw mean ≥ 0.95 made honest RU dubs structurally unverifiable. But N
	// unanimous fragments are N near-independent looks at the same track:
	// each additional agreeing fragment shrinks the residual doubt by
	// corrDiscount (deliberately far from full independence — whisper errors
	// correlate on music/effects-heavy audio). 3 × ru@0.92 → 0.971 ✓;
	// 3 × 0.80 → 0.928 ✗ (stays honest); 6 × 0.80 → 0.984 ✓.
	// Compounding only applies above compoundFloor — weak evidence is
	// reported raw, never laundered.
	corrDiscount  = 0.6
	compoundFloor = 0.75
)

// AssembleAudio: all speech fragments agree AND the compounded confidence ≥
// threshold → verified (spec §3, unanimity-compounded 2026-07-17). Whisper
// returns ISO-639-1 codes (en/ru/ja) directly.
func AssembleAudio(frs []LIDFragment) *domain.AudioVerdict {
	var speech []LIDFragment
	for _, f := range frs {
		if f.Speech && f.Lang != "" {
			speech = append(speech, f)
		}
	}
	if len(speech) == 0 {
		return nil
	}
	lang := speech[0].Lang
	agree := true
	sum := 0.0
	for _, f := range speech {
		if f.Lang != lang {
			agree = false
		}
		sum += f.Prob
	}
	mean := sum / float64(len(speech))
	v := &domain.AudioVerdict{Lang: lang, Confidence: mean}
	if !agree {
		// Disagreement: report the majority-first lang but cap confidence.
		v.Confidence = mean * 0.5
		return v
	}
	if mean >= compoundFloor {
		v.Confidence = 1 - (1-mean)*math.Pow(corrDiscount, float64(len(speech)-1))
	}
	v.Verified = len(speech) >= minSpeechFragments && v.Confidence >= domain.VerifiedThreshold
	return v
}

// AssembleHardsub applies tools/subprobe's decision rule (verify_verdict.py):
// burned = tier1_hits >= max(2, frames/5) AND ocr_real >= 1. Verified
// (badge-grade) additionally needs ≥2 OCR confirmations + a known script.
func AssembleHardsub(h *HardsubResult) *domain.HardsubVerdict {
	if h == nil || h.Frames == 0 {
		return nil
	}
	minHits := h.Frames / 5
	if minHits < 2 {
		minHits = 2
	}
	present := h.Tier1Hits >= minHits && h.OCRReal >= 1
	v := &domain.HardsubVerdict{Present: present}
	if !present {
		v.Confidence = 0.9 // "looks clean" — informational, never badged
		return v
	}
	switch h.Script {
	case "cyrillic":
		v.Lang = "ru"
	case "latin":
		v.Lang = "en"
	case "cjk":
		v.Lang = "ja"
	}
	if h.OCRReal >= 2 && v.Lang != "" {
		v.Confidence = 0.96
		v.Verified = true
	} else {
		v.Confidence = 0.8
	}
	return v
}
