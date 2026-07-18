package queue

import (
	"slices"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// aniskipgate.go: the AniSkip probe gate (owner directive 2026-07-18).
// Where crowdsourced AniSkip data already covers a side, probing it again
// buys nothing — the catalog serves AniSkip first and our detected windows
// only fill the gaps. The gate has two levels:
//
//   - planner level (FilterAniskipCovered): units whose every probe-able
//     kind is covered are dropped from skip planning entirely — no claim
//     tick, no row, no extraction;
//   - probe level (SkipTask.CoveredKinds, honored by prober.SkipProber):
//     a partially-covered unit still probes its uncovered kind, but skips
//     the covered kind's window extraction and records it as the terminal
//     "aniskip" status.
//
// Known residual: a title AniSkip covers except one episode can't
// pair-bootstrap a fingerprint from that lone episode (pairs need two), so
// that episode idles on the pending_fp cycle until another family
// contributes a fingerprint — the same bounded residual class as the
// movie/single-episode case in spec §9.

// AniskipCoverage maps episode → the kinds ("op"/"ed") AniSkip already has
// usable data for. A key present with an empty slice means "checked, not
// covered"; an absent key means "not checked" — Engine.aniskipCoverage
// relies on that distinction to fetch each episode at most once per TTL.
type AniskipCoverage map[int][]string

// CoveredKinds returns cov's kinds for ep (nil when unchecked/uncovered).
func (cov AniskipCoverage) CoveredKinds(ep int) []string {
	if cov == nil {
		return nil
	}
	return cov[ep]
}

func (cov AniskipCoverage) has(ep int, kind string) bool {
	return slices.Contains(cov.CoveredKinds(ep), kind)
}

// FilterAniskipCovered drops units AniSkip fully covers: op covered AND
// (ed covered OR an animejoy leg — those are mp4, where ED is terminal
// no_match by design (v1 can't absolutize mp4 tail times), so op was the
// only probe-able kind anyway). Units with rows already stored are filtered
// too: that retires pending_fp/unreachable retry churn for episodes AniSkip
// has since covered.
func FilterAniskipCovered(units []SkipUnit, cov AniskipCoverage) []SkipUnit {
	if len(cov) == 0 {
		return units
	}
	out := units[:0:0]
	for _, u := range units {
		opCovered := cov.has(u.Episode, domain.SkipKindOp)
		edCovered := cov.has(u.Episode, domain.SkipKindEd)
		if opCovered && (edCovered || isAnimejoyLeg(u.Provider)) {
			continue
		}
		out = append(out, u)
	}
	return out
}

// PreferProvider stably moves units of the given provider to the front —
// the CV_PIN_ANIME "uuid:provider" form, so a pinned title's preferred
// provider family is planned first.
func PreferProvider(units []SkipUnit, provider string) []SkipUnit {
	if provider == "" {
		return units
	}
	out := make([]SkipUnit, 0, len(units))
	var rest []SkipUnit
	for _, u := range units {
		if u.Provider == provider {
			out = append(out, u)
		} else {
			rest = append(rest, u)
		}
	}
	return append(out, rest...)
}
