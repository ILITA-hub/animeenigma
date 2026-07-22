package capability

import (
	"encoding/json"
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

// synthStatusVerified mirrors content-verify's domain.StatusVerified value.
// Kept as a local literal for the same reason as SkipTimingRow / skipStatusDetected:
// the constant lives under content-verify's internal/ tree, which Go's
// internal-import rule forbids catalog from importing.
const synthStatusVerified = "verified"

// --- content-verify verdict WIRE mirror -------------------------------------
//
// These structs reproduce services/content-verify/internal/domain's verdict
// wire shape (UnitKey / AudioVerdict / HardsubVerdict / SoftTrack / SampleInfo /
// UnitVerdict / ProviderSummary) field-for-field, tag-for-tag. We cannot import
// that domain (internal/ tree), so — exactly like the hand-kept SkipTimingRow in
// verify_client.go — these are maintained by hand and MUST stay in sync with the
// source snake_case JSON tags so the synthesized ae/kodik entries are
// byte-compatible with what content-verify used to emit on the /content-verify
// feed. FE reads only key/episode/status/audio/hardsub/episodes/probed_at
// (frontend/web/src/types/contentVerify.ts); sample/fails stay zero-valued.

type unitKeyWire struct {
	Team     string `json:"team,omitempty"`
	Server   string `json:"server,omitempty"`
	Category string `json:"category,omitempty"`
	Track    string `json:"track,omitempty"`
}

type audioVerdictWire struct {
	Lang       string  `json:"lang,omitempty"`
	Confidence float64 `json:"confidence"`
	Verified   bool    `json:"verified"`
}

type hardsubVerdictWire struct {
	Present    bool    `json:"present"`
	Lang       string  `json:"lang,omitempty"`
	Confidence float64 `json:"confidence"`
	Verified   bool    `json:"verified"`
}

type softTrackWire struct {
	Lang string `json:"lang,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type sampleInfoWire struct {
	Fragments     int     `json:"fragments"`
	SpeechSeconds float64 `json:"speech_seconds"`
}

type unitVerdictWire struct {
	Key      unitKeyWire         `json:"key"`
	Episode  int                 `json:"episode"`
	Status   string              `json:"status"`
	Audio    *audioVerdictWire   `json:"audio,omitempty"`
	Hardsub  *hardsubVerdictWire `json:"hardsub,omitempty"`
	Softsubs []softTrackWire     `json:"softsubs,omitempty"`
	RawAudio bool                `json:"raw_audio,omitempty"`
	Episodes int                 `json:"episodes,omitempty"`
	ProbedAt time.Time           `json:"probed_at"`
	Sample   sampleInfoWire      `json:"sample"`
	Fails    int                 `json:"fails,omitempty"`
}

type providerSummaryWire struct {
	Status       string   `json:"status"`
	Raw          bool     `json:"raw"`
	DubLangs     []string `json:"dub_langs"`
	HardsubLangs []string `json:"hardsub_langs"`
	Episodes     int      `json:"episodes,omitempty"`
}

type providerVerdictWire struct {
	Provider string              `json:"provider"`
	Summary  providerSummaryWire `json:"summary"`
	Units    []unitVerdictWire   `json:"units"`
}

// summarizeSynth is content-verify's domain.Summarize ported VERBATIM
// (services/content-verify/internal/domain/verify.go) onto the wire mirror
// structs, so the synthesized ae/kodik provider summaries are identical to what
// content-verify would have produced from the same units. Keep in sync by hand
// if that source ever changes. Rules it encodes: audio ja → raw; audio en/ru →
// dub_langs; verified raw_audio → raw; hardsub counted only for NON-dub units.
func summarizeSynth(units []unitVerdictWire) providerSummaryWire {
	s := providerSummaryWire{Status: "unverified", DubLangs: []string{}, HardsubLangs: []string{}}
	if len(units) == 0 {
		return s
	}
	dub := map[string]bool{}
	hs := map[string]bool{}
	verified := 0
	for _, u := range units {
		if u.Status == synthStatusVerified {
			verified++
		}
		s.Episodes = max(s.Episodes, u.Episodes)
		if u.Status == synthStatusVerified && u.RawAudio {
			s.Raw = true // provider-native original-audio claim (kodik subtitles teams)
		}
		if u.Audio != nil && u.Audio.Verified {
			if u.Audio.Lang == "ja" {
				s.Raw = true
			} else if u.Audio.Lang == "en" || u.Audio.Lang == "ru" {
				dub[u.Audio.Lang] = true
			}
		}
		if u.Key.Category != "dub" && u.Hardsub != nil && u.Hardsub.Verified && u.Hardsub.Present && u.Hardsub.Lang != "" {
			hs[u.Hardsub.Lang] = true
		}
	}
	for l := range dub {
		s.DubLangs = append(s.DubLangs, l)
	}
	for l := range hs {
		s.HardsubLangs = append(s.HardsubLangs, l)
	}
	sort.Strings(s.DubLangs)
	sort.Strings(s.HardsubLangs)
	switch {
	case verified == len(units):
		s.Status = "verified"
	case verified > 0 || s.Raw || len(s.DubLangs) > 0 || len(s.HardsubLangs) > 0:
		s.Status = "partial"
	}
	return s
}

// synthVerdicts synthesizes the ae + kodik provider verdicts from the capability
// report. content-verify no longer enumerates or stores these two providers
// (they are known truth, not probe targets); the wire entries are reconstructed
// here at read time so the /content-verify feed and the /capabilities blend both
// carry them, byte-compatible with the old content-verify output.
//
// It mirrors the OLD content-verify enumerate.go semantics exactly (guard +
// per-provider synth), reading the ae lang and kodik teams straight from the
// report (never re-fetching the kodik roster):
//   - guard: caps in state no_content or group adult were never enumerated.
//   - ae (group "firstparty"): one Track:"default" unit, audio lang = cap.Lang
//     ("" → "ja"), verified.
//   - kodik dub team (variant category "dub"): Team + Category:"dub" unit, ru
//     audio verified.
//   - kodik sub team (variant category "sub"): Team + Category:"sub" unit,
//     raw_audio + burned-in ru hardsub verified.
//
// Kodik units carry the cap-level episodes-ready count (see synthKodik); ae
// carries none (matching the old enumerate, which set no ae episode count).
func synthVerdicts(report domain.CapabilityReport) []providerVerdictWire {
	now := time.Now().UTC()
	var out []providerVerdictWire
	for _, fam := range report.Families {
		for _, pc := range fam.Providers {
			// Mirror the old enumerate guard: no_content and adult caps were
			// never synthesized.
			if pc.State == "no_content" || pc.Group == "adult" {
				continue
			}
			switch {
			case pc.Group == "firstparty":
				out = append(out, synthAE(pc, now))
			case pc.Provider == "kodik":
				if v, ok := synthKodik(pc, now); ok {
					out = append(out, v)
				}
			}
		}
	}
	return out
}

// synthAE builds the single first-party ("ae") verdict: the self-hosted library
// audio language is known truth from ingest. lang falls back to "ja" (original
// audio) when the cap carries no localized-dub language override.
func synthAE(pc domain.ProviderCap, now time.Time) providerVerdictWire {
	lang := pc.Lang
	if lang == "" {
		lang = "ja"
	}
	units := []unitVerdictWire{{
		Key:      unitKeyWire{Track: "default"},
		Status:   synthStatusVerified,
		Audio:    &audioVerdictWire{Lang: lang, Confidence: 1.0, Verified: true},
		ProbedAt: now,
	}}
	return providerVerdictWire{Provider: pc.Provider, Summary: summarizeSynth(units), Units: units}
}

// synthKodik builds the kodik verdict from the report's per-team variants. Each
// variant carries a real Team{ID,Name} and its category (dub/sub) — exactly the
// (team, category) pairs the old enumerate derived from the kodik roster. voice
// teams (category "dub") synth ru-dub audio; subtitle teams (category "sub")
// synth original audio + burned-in ru subs. ok=false when no real team variant
// exists (a defensive guard — content state always carries teams).
//
// Episodes-ready is the cap-level count (max across teams, set natively by
// kodikFamily) — the report carries no per-team counts. It flows into the
// summary's episodes, which the FE ProviderChip PREFERS over ProviderCap.episodes
// (`verify?.episodes || cap?.episodes`), so a wrong value here would mislabel the
// chip. 0 (unknown) is left as 0 so the chip falls back to the cap count rather
// than showing a fabricated "1"; the sample Episode number floors at 1.
func synthKodik(pc domain.ProviderCap, now time.Time) (providerVerdictWire, bool) {
	ready := pc.Episodes
	sample := ready
	if sample < 1 {
		sample = 1
	}
	var units []unitVerdictWire
	for _, v := range pc.Variants {
		if v.Team == nil || v.Team.ID == "" {
			continue // no real team → trait-only variant, not a synthesizable unit
		}
		if v.Category == "sub" {
			units = append(units, unitVerdictWire{
				Key:      unitKeyWire{Team: v.Team.ID, Category: "sub"},
				Episode:  sample,
				Episodes: ready,
				Status:   synthStatusVerified,
				RawAudio: true,
				Hardsub:  &hardsubVerdictWire{Present: true, Verified: true, Lang: "ru", Confidence: 1.0},
				ProbedAt: now,
			})
		} else {
			units = append(units, unitVerdictWire{
				Key:      unitKeyWire{Team: v.Team.ID, Category: "dub"},
				Episode:  sample,
				Episodes: ready,
				Status:   synthStatusVerified,
				Audio:    &audioVerdictWire{Lang: "ru", Confidence: 1.0, Verified: true},
				ProbedAt: now,
			})
		}
	}
	if len(units) == 0 {
		return providerVerdictWire{}, false
	}
	return providerVerdictWire{Provider: pc.Provider, Summary: summarizeSynth(units), Units: units}, true
}

// SynthSummaries returns the summary-only ae/kodik rollups synthesized from the
// report, keyed by provider id — the shape service.go blends onto ProviderCap.Verify.
// Independent of content-verify availability (ae/kodik are known truth).
func SynthSummaries(report domain.CapabilityReport) map[string]domain.VerifySummary {
	verdicts := synthVerdicts(report)
	out := make(map[string]domain.VerifySummary, len(verdicts))
	for _, v := range verdicts {
		out[v.Provider] = domain.VerifySummary{
			Status:       v.Summary.Status,
			Raw:          v.Summary.Raw,
			DubLangs:     v.Summary.DubLangs,
			HardsubLangs: v.Summary.HardsubLangs,
			Episodes:     v.Summary.Episodes,
		}
	}
	return out
}

// MergeSynthVerdicts takes the raw content-verify /internal/verify/verdicts data
// payload ({anime_id, providers:[...]}) and returns a re-marshaled payload with
// the ae/kodik entries REPLACED by the read-time synth: any stale ae/kodik entry
// content-verify still returns is dropped, then the synthesized entries are
// appended. Every OTHER provider entry is preserved VERBATIM (kept as raw JSON,
// so any content-verify field this mirror doesn't model survives untouched).
//
// Robust to a missing/unparseable raw payload (content-verify down or the kill
// switch off): it then returns a synth-only payload, so ae/kodik truth still
// surfaces. animeID seeds anime_id when the raw payload carries none.
func MergeSynthVerdicts(animeID string, raw json.RawMessage, report domain.CapabilityReport) json.RawMessage {
	synth := synthVerdicts(report)
	synthIDs := make(map[string]bool, len(synth))
	for _, s := range synth {
		synthIDs[s.Provider] = true
	}

	// Parse the upstream payload loosely so each non-synth provider entry stays
	// verbatim raw JSON.
	var head struct {
		AnimeID   string            `json:"anime_id"`
		Providers []json.RawMessage `json:"providers"`
	}
	outAnime := animeID
	kept := []json.RawMessage{}
	if len(raw) > 0 && json.Unmarshal(raw, &head) == nil {
		if head.AnimeID != "" {
			outAnime = head.AnimeID
		}
		for _, p := range head.Providers {
			var id struct {
				Provider string `json:"provider"`
			}
			if json.Unmarshal(p, &id) == nil && synthIDs[id.Provider] {
				continue // drop stale ae/kodik entry — the read-time synth replaces it
			}
			kept = append(kept, p)
		}
	}

	for _, s := range synth {
		b, err := json.Marshal(s)
		if err != nil {
			continue
		}
		kept = append(kept, json.RawMessage(b))
	}

	out := struct {
		AnimeID   string            `json:"anime_id"`
		Providers []json.RawMessage `json:"providers"`
	}{AnimeID: outAnime, Providers: kept}
	b, err := json.Marshal(out)
	if err != nil {
		return raw // last-resort: hand back the original payload
	}
	return b
}
