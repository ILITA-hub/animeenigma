package autocache

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"

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
//
//	(a) is RAW — per isRAW's allowlist-gated policy (WR-04): allowlisted uploader
//	    OR an explicit positive raw token, AND no negative (dub/dual/multi/eng-
//	    audio/hardsub) token,
//	(b) parses to a resolution ≤ qualityCap,
//	(c) has Seeders ≥ minSeeders,
//	(d) parses to EXACTLY the wanted episode (episode-exact, ALWAYS — a release
//	    whose episode can't be parsed or differs is rejected), and
//	(e) is the right ANIME: a release carrying a MAL-ID is trusted iff that ID ==
//	    malID; a release with no MAL-ID (keyword Jackett/Nyaa hit) must instead
//	    title-match one of the demand titles (name_jp → romaji → name_en).
//
// (d) + (e) were added (v4.1 fix) because the keyword search could otherwise grab
// a popular unrelated release — e.g. "Kanojo, Okarishimasu - 59" for a Bookworm
// ep-10 demand — and cache the wrong anime/episode. Because the input is seeder-
// ranked DESC, the first fully-qualifying release is the best-seeded valid RAW.
// Returns (zero, false) when none qualify (the demand retries next tick).
func selectRAW(releases []domain.Release, qualityCap, minSeeders, wantEpisode, malID int, titles []string) (domain.Release, bool) {
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
		// (d) episode-exact — the strongest false-match guard (kills both a wrong
		// anime whose absolute numbering differs AND an off-by-one same-anime pick).
		if ep, ok := episodeOf(r.Title); !ok || ep != wantEpisode {
			continue
		}
		// (e) anime identity — MAL-ID if the release carries one, else title match.
		if !animeMatches(r, malID, titles) {
			continue
		}
		return r, true
	}
	return domain.Release{}, false
}

// episodeRegexes extract an episode number from a release title, tried in order
// of specificity: SxxEyy (Tsundere-Raws/CR-style), an explicit EP/Episode token,
// then the generic "- NN (" / "- NN [" / "- NN " form (SubsPlease/Erai/Ohys). The
// generic library filename.Detector only knows the last form, so the autocache
// guard needs this broader parser to handle the SxxEyy releases its title search
// surfaces. First match wins.
var episodeRegexes = []*regexp.Regexp{
	regexp.MustCompile(`(?i)\bs\d{1,2}e(\d{1,4})\b`),
	regexp.MustCompile(`(?i)\b(?:ep|episode)\.?\s*(\d{1,4})\b`),
	regexp.MustCompile(`(?:^|[\s_\]\)])-\s*(\d{1,4})(?:v\d+)?(?:\s|$|[\(\[])`),
}

// episodeOf parses the episode number from a release title. ok=false when no
// recognized pattern matches (episode-exact then rejects — we cannot confirm it
// is the wanted episode). Numbers are clamped to [1, 9999].
func episodeOf(title string) (int, bool) {
	for _, re := range episodeRegexes {
		if m := re.FindStringSubmatch(title); len(m) >= 2 {
			if n, err := strconv.Atoi(m[1]); err == nil && n >= 1 && n <= 9999 {
				return n, true
			}
		}
	}
	return 0, false
}

// animeMatches reports whether a release is the requested anime. A release that
// carries a MAL-ID (AnimeTosho-sourced) is trusted iff that ID matches; one with
// no MAL-ID (a keyword Jackett/Nyaa hit) must title-match a demand title. This is
// the "prefer MAL-ID, fall back to title (name_jp → romaji → name_en)" rule.
func animeMatches(r domain.Release, malID int, titles []string) bool {
	if r.MALID > 0 {
		return malID > 0 && r.MALID == malID
	}
	return titleMatches(r.Title, titles)
}

// matchStopwords are tokens too generic to carry anime identity — excluded from
// the token-overlap test so "of/the/no/season" can't inflate a match.
var matchStopwords = map[string]bool{
	"the": true, "of": true, "no": true, "a": true, "to": true, "e": true,
	"wa": true, "ga": true, "season": true, "part": true, "cour": true,
	"1st": true, "2nd": true, "3rd": true, "4th": true, "5th": true,
}

// titleMatches reports whether a release title corresponds to any of the ordered
// demand titles (name_jp → romaji → name_en). A title matches when the normalized
// demand title is a substring of the normalized release title (covers a full
// romaji/English title embedded in the release, as Tsundere-Raws/CR releases do),
// OR ≥70% of the demand title's significant tokens appear in the release. Empty
// demand titles (legacy/title-less rows) match nothing — only a MAL-ID-verified
// release can satisfy such a row.
func titleMatches(releaseTitle string, demandTitles []string) bool {
	rel := normalizeForMatch(releaseTitle)
	if rel == "" {
		return false
	}
	relTokens := make(map[string]bool)
	for _, tok := range strings.Fields(rel) {
		relTokens[tok] = true
	}
	for _, dt := range demandTitles {
		nt := normalizeForMatch(dt)
		if nt == "" {
			continue
		}
		if strings.Contains(rel, nt) {
			return true
		}
		var sig, hit int
		for _, tok := range strings.Fields(nt) {
			if len(tok) < 2 || matchStopwords[tok] {
				continue
			}
			sig++
			if relTokens[tok] {
				hit++
			}
		}
		if sig > 0 && hit*100 >= sig*70 {
			return true
		}
	}
	return false
}

// normalizeForMatch lowercases and reduces a title to space-separated
// alphanumeric runs (Unicode-aware, so kana/kanji are preserved as letters),
// dropping bracket tags, punctuation, and separators so romaji/English titles
// compare cleanly across release-naming styles.
func normalizeForMatch(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
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
