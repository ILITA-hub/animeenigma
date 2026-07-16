package prober

import "github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"

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

const minSpeechFragments = 3

// AssembleAudio: all speech fragments agree AND mean prob ≥ threshold →
// verified (spec §3). Whisper returns ISO-639-1 codes (en/ru/ja) directly.
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
	v.Verified = len(speech) >= minSpeechFragments && mean >= domain.VerifiedThreshold
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
