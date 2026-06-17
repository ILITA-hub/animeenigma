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
// separators between "dual" / "multi" / "eng" and "audio"/"dub". Expanded for
// WR-04 with bare \beng\b / \benglish\b audio markers and "multi-sub" / "bd-eng"
// forms that earlier slipped through the small denylist.
var negativeTokenRegex = regexp.MustCompile(`(?i)\b(dub|dual[ .-]?audio|multi[ .-]?audio|multi[ .-]?subs?|eng[ .-]?dub|eng(?:lish)?[ .-]?audio|bd[ .-]?eng|hardsub)\b`)

// positiveRawTokenRegex is the WR-04 positive signal: a title that explicitly
// advertises a raw / JP-audio release. When the uploader is NOT in the
// allowlist, isRAW REQUIRES one of these tokens rather than defaulting open —
// see isRAW's policy comment.
var positiveRawTokenRegex = regexp.MustCompile(`(?i)\b(raws?|web[ .-]?rip[ .-]?jp|jp[ .-]?audio)\b`)

// qualityTokenRegex parses the leading resolution token. Mirrors the
// animetosho client's qualityRegex so the parse stays consistent across the
// codebase. Only the four recognized tokens are accepted; anything else is
// "unknown" and treated conservatively (rejected — cannot prove ≤ cap).
var qualityTokenRegex = regexp.MustCompile(`(?i)\b(2160|1080|720|480)p\b`)

// selectRAW iterates the (already seeder-ranked DESC) release slice and returns
// the first release that:
//   (a) is RAW — per isRAW's allowlist-gated policy (WR-04): allowlisted uploader
//       OR an explicit positive raw token, AND no negative (dub/dual/multi/eng-
//       audio/hardsub) token,
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

// isRAW reports whether (title, uploader) looks like a JP-audio raw.
//
// WR-04 policy (fail-CLOSED for unknown uploaders). A negative token in the
// title ALWAYS disqualifies (even for an allowlisted uploader, so a dub variant
// from a normally-raw group is still rejected). With no negative token, the
// release qualifies as RAW only when there is a POSITIVE signal:
//
//   - the uploader is in the known RAW-uploader allowlist (the primary gate —
//     most real raws come from these groups), OR
//   - the title carries an explicit positive raw token (raw/raws/webrip-jp/
//     jp-audio), which lets a legitimate raw from an unrecognized group through.
//
// "unknown uploader + no negative token + no positive raw token" is treated as
// NOT RAW (skip, leave the demand for a later better-seeded allowlisted release).
// This inverts the previous default-OPEN behavior, which admitted any title that
// merely lacked one of a few negative tokens — the riskiest default for an
// automated downloader, since a wrong pick burns disk budget and is served to
// users as raw JP. The allowlist is intentionally short; expanding it is the
// supported way to admit a new trusted RAW group (it is the documented tunable).
func isRAW(title, uploader string) bool {
	if negativeTokenRegex.MatchString(title) {
		return false
	}
	if rawUploaderAllowlist[strings.ToLower(strings.TrimSpace(uploader))] {
		return true
	}
	// Unknown uploader: require an explicit positive raw token rather than
	// defaulting open (WR-04 fail-closed).
	return positiveRawTokenRegex.MatchString(title)
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
