package animejoy

import (
	"regexp"
	"strings"
)

// jaroWinkler returns a similarity score in [0,1] using the standard
// Jaro-Winkler algorithm (prefix scale p=0.1, prefix length capped at 4).
//
// Ported verbatim from services/scraper/internal/fuzzy/jarowinkler.go: that copy
// lives in the SCRAPER module under internal/, so the catalog module cannot
// import it. Kept small and stdlib-only here so the animejoy package is
// self-contained and offline-testable. The 0.85 match threshold lives in
// search.go (scoreAndPick), matching the scraper's fuzzy fallback.
func jaroWinkler(a, b string) float64 {
	if a == b {
		return 1.0
	}
	if len(a) == 0 || len(b) == 0 {
		return 0.0
	}
	ar := []rune(a)
	br := []rune(b)

	// Match window: floor(max(la,lb)/2) - 1, but at least 0.
	window := len(ar)
	if len(br) > window {
		window = len(br)
	}
	window = window/2 - 1
	if window < 0 {
		window = 0
	}

	matchesA := make([]bool, len(ar))
	matchesB := make([]bool, len(br))
	matches := 0
	for i := 0; i < len(ar); i++ {
		lo := i - window
		if lo < 0 {
			lo = 0
		}
		hi := i + window + 1
		if hi > len(br) {
			hi = len(br)
		}
		for j := lo; j < hi; j++ {
			if matchesB[j] {
				continue
			}
			if ar[i] != br[j] {
				continue
			}
			matchesA[i] = true
			matchesB[j] = true
			matches++
			break
		}
	}
	if matches == 0 {
		return 0.0
	}
	// Transpositions.
	k := 0
	transpositions := 0
	for i := 0; i < len(ar); i++ {
		if !matchesA[i] {
			continue
		}
		for !matchesB[k] {
			k++
		}
		if ar[i] != br[k] {
			transpositions++
		}
		k++
	}
	t := float64(transpositions) / 2.0
	m := float64(matches)
	jaro := (m/float64(len(ar)) + m/float64(len(br)) + (m-t)/m) / 3.0
	// Winkler boost: count common prefix up to 4 chars.
	prefix := 0
	limit := 4
	if limit > len(ar) {
		limit = len(ar)
	}
	if limit > len(br) {
		limit = len(br)
	}
	for i := 0; i < limit && ar[i] == br[i]; i++ {
		prefix++
	}
	return jaro + float64(prefix)*0.1*(1.0-jaro)
}

// seasonRe matches the season/part markers that AnimeJoy's Russian titles carry
// in parentheses or inline, plus the romaji/english forms that synonyms use.
// Cyrillic: "N сезон", "сезон N", "часть N", "ТВ-N" / "TV-N". Latin: "season N",
// "part N", "Nnd/Nrd/Nth season". Case-insensitive; (?i) covers the ASCII parts
// and we lower-case Cyrillic ourselves first.
// Note: Go's regexp (RE2) treats \b as an ASCII-only word boundary, so it does
// NOT fire between a Cyrillic letter and a space/digit. We therefore match the
// Cyrillic season words WITHOUT \b anchors and rely on the literal word text
// (сезон/часть/тв) being specific enough; the Latin forms keep loose spacing.
var (
	seasonRe = regexp.MustCompile(`(?i)(?:\d+\s*(?:сезон|сезона|season|часть|part)|(?:сезон|season|часть|part)\s*\d+|тв\s*[-–]?\s*\d+|tv\s*[-–]?\s*\d+|\d+(?:nd|rd|st|th)\s+season)`)
	// bracketCountRe strips the trailing "[28 из 28]" / "[10 из 10]" episode
	// counter AnimeJoy appends to search titles.
	bracketCountRe = regexp.MustCompile(`\[[^\]]*\]`)
	// punctRe collapses separator punctuation (Latin + the Cyrillic-flavoured
	// dashes/quotes AnimeJoy uses) into spaces.
	punctRe = regexp.MustCompile(`[:·—–\-«»"'!?,()\[\]]+`)
	wsRe    = regexp.MustCompile(`\s+`)
)

// foldSeason normalises a title for cross-language fuzzy matching. It is the
// Cyrillic-aware analogue of the shared fuzzy.NormalizeTitle (which folds ENGLISH
// season words only): it lower-cases, strips the "[N из M]" counter, removes
// "N сезон / сезон N / часть N / ТВ-N" (and the Latin season/part forms),
// collapses punctuation, and squeezes whitespace. Season disambiguation is done
// separately by scoreAndPick via the parsed section/number — folding here lets a
// query for "Фрирен (2 сезон)" still fuzzy-match the bare candidate "Фрирен".
func foldSeason(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = bracketCountRe.ReplaceAllString(s, " ")
	s = seasonRe.ReplaceAllString(s, " ")
	// Unify Cyrillic/Latin homoglyphs AFTER the season markers are stripped (those
	// regexes match the clean Cyrillic/Latin forms; folding first would mangle
	// "сезон"/"part" past recognition). See foldConfusables.
	s = foldConfusables(s)
	s = punctRe.ReplaceAllString(s, " ")
	s = wsRe.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// latinToCyrConfusables maps each Latin letter that shares a glyph with a
// Cyrillic letter to that Cyrillic letter. AnimeJoy publishes some title rows
// with these homoglyphs swapped in — e.g. news_id 5600 renders "Реинкарнация" as
// "Peинкapнaция", mixing Latin P/e/a/p/o/c/x/y/k into the Cyrillic word. Because
// jaroWinkler compares runes by codepoint, one such swap silently tanks the
// fuzzy score (0.99 → 0.75), dropping the row below the 0.85 gate so the whole
// title resolves to "no match" (report 2026-07-09T06-40-52: AllVideo + Sibnet
// hidden for Mushoku Tensei III).
//
// Cyrillic is the canonical target because these rows are Russian: a clean
// catalog title stays byte-identical (its letters are never in the map), so the
// obfuscated candidate is what folds back onto it. Only the identical-glyph set
// is mapped (upper- and lower-case share the shape), keeping the blast radius on
// genuinely-Latin romaji titles minimal — and harmless regardless, since the
// fold is deterministic and applied to BOTH query and candidate. The map is 1:1,
// so it only unifies scripts, never loses signal.
var latinToCyrConfusables = map[rune]rune{
	'a': 'а', 'c': 'с', 'e': 'е', 'o': 'о',
	'p': 'р', 'x': 'х', 'y': 'у', 'k': 'к',
}

// foldConfusables rewrites the Latin/Cyrillic homoglyphs above to the canonical
// Cyrillic script. Input MUST already be lower-cased (uppercase homoglyphs are
// folded by foldSeason's ToLower first). Applied to BOTH query and candidate
// titles, so any pair is transformed identically and can never regress.
func foldConfusables(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if m, ok := latinToCyrConfusables[r]; ok {
			b.WriteRune(m)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}
