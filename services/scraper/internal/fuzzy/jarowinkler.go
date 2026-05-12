package fuzzy

// JaroWinkler is a verbatim relocation of animepahe.cache.go::jaroWinkler.
// Returns a similarity score in [0,1] between two strings, using the standard
// Jaro-Winkler algorithm with prefix scale p=0.1 and prefix length capped at 4.
//
// Per RESEARCH.md (Phase 16) Pitfall 5 + Assumption A6, the AnimePahe
// fuzzy-fallback threshold is 0.85. Stdlib only — no new deps.
func JaroWinkler(a, b string) float64 {
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
