package autocache

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// raw_filter.go implements TRIG-05: select a JP-audio RAW release at or below
// the configured quality cap with enough seeders, rejecting DUB / dual-audio /
// hardsub releases. There is NO structured release-type field on domain.Release
// (see release.go) — RAW vs DUB is inferred from the title/uploader, so this is
// a best-effort heuristic (RESEARCH Pitfall 3, "Realistic limitation"). The
// uploader allowlist + negative-token list below are the documented tunable.

// rawUploaderAllowlist is the set of known JP-audio raw-video groups (lowercased
// for a case-insensitive compare). Ohys-Raws / Leopard-Raws / ARC-Raws are
// raw-only; SubsPlease / Erai-raws carry soft subs but ship JP audio (RAW video),
// which is acceptable under D3 (one RAW video serves SUB demand via the overlay).
// Membership short-circuits the title scan to "is RAW" — UNLESS the title carries
// a negative token (a dub/hardsub variant from even a normally-raw group), in
// which case the negative filter still rejects it.
var rawUploaderAllowlist = map[string]bool{
	"ohys-raws":    true,
	"leopard-raws": true,
	"arc-raws":     true,
	"subsplease":   true,
	"erai-raws":    true,
}

// negativeTokenRegex rejects titles implying non-JP audio or burned-in subs.
// Word-boundaried + case-insensitive; tolerant of the common space/dot/dash
// separators between "dual" / "multi" / "eng" and "audio"/"dub".
var negativeTokenRegex = regexp.MustCompile(`(?i)\b(dub|dual[ .-]?audio|multi[ .-]?audio|eng[ .-]?dub|hardsub)\b`)

// qualityTokenRegex parses the leading resolution token. Mirrors the
// animetosho client's qualityRegex so the parse stays consistent across the
// codebase. Only the four recognized tokens are accepted; anything else is
// "unknown" and treated conservatively (rejected — cannot prove ≤ cap).
var qualityTokenRegex = regexp.MustCompile(`(?i)\b(2160|1080|720|480)p\b`)

// selectRAW iterates the (already seeder-ranked DESC) release slice and returns
// the first release that:
//   (a) is RAW — uploader in the allowlist OR title carries no dub/dual-audio/
//       multi-audio/eng-dub/hardsub token,
//   (b) parses to a resolution ≤ qualityCap, and
//   (c) has Seeders ≥ minSeeders.
//
// Because the input is seeder-ranked DESC, the first qualifying release is the
// best-seeded eligible RAW. Returns (zero, false) when none qualify. A release
// whose quality token is missing/unparseable is rejected (conservative — we
// cannot prove it is ≤ cap).
func selectRAW(releases []domain.Release, qualityCap, minSeeders int) (domain.Release, bool) {
	for _, r := range releases {
		if !isRAW(r.Title, r.Uploader) {
			continue
		}
		res, ok := resolutionOf(r.Quality)
		if !ok || res > qualityCap {
			continue
		}
		if r.Seeders < minSeeders {
			continue
		}
		return r, true
	}
	return domain.Release{}, false
}

// isRAW reports whether (title, uploader) looks like a JP-audio raw. A negative
// token in the title ALWAYS disqualifies (even for an allowlisted uploader, so a
// dub variant from a normally-raw group is still rejected); otherwise an
// allowlisted uploader OR a title free of negative tokens qualifies.
func isRAW(title, uploader string) bool {
	if negativeTokenRegex.MatchString(title) {
		return false
	}
	if rawUploaderAllowlist[strings.ToLower(strings.TrimSpace(uploader))] {
		return true
	}
	// No negative token and not an explicitly-known raw group: accept as RAW.
	// This is the best-effort heuristic — false positives are bounded by the
	// negative-token filter above and the quality/seeder gates in selectRAW.
	return true
}

// resolutionOf parses the leading resolution token of a quality string (e.g.
// "1080p" → 1080). ok=false when no recognized token is present, which the
// caller treats as ineligible (conservative).
func resolutionOf(quality string) (int, bool) {
	m := qualityTokenRegex.FindStringSubmatch(quality)
	if len(m) < 2 {
		return 0, false
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		return 0, false
	}
	return n, true
}
