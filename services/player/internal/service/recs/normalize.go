package recs

// epsilon prevents division-by-zero in degenerate pools where max == min.
// See spec §2.3: ε = 1e-9.
const epsilon = 1e-9

// MinMaxNormalize maps raw scores to NormalizedScore in [0, 1] over the
// candidate pool. Degenerate pools (max-min < epsilon, including empty,
// single-element, and all-equal pools) return all zeros — never NaN.
// Candidates absent from the raw map default to NormalizedScore(0).
//
// See spec §2.3 for why this is per-signal-pool rather than global:
// it makes signal weights coherent across raw scales (S3 ~[0,large_int]
// vs S5 ~[0,0.05]) so the weight registry is the single knob that decides
// each signal's relative contribution.
func MinMaxNormalize(raw map[AnimeID]RawScore, pool []AnimeID) map[AnimeID]NormalizedScore {
	out := make(map[AnimeID]NormalizedScore, len(pool))
	if len(pool) == 0 {
		return out
	}

	min, max, ok := findMinMax(raw, pool)
	if !ok || (max-min) < epsilon {
		for _, id := range pool {
			out[id] = 0
		}
		return out
	}

	span := max - min
	for _, id := range pool {
		v, present := raw[id]
		if !present {
			out[id] = 0
			continue
		}
		out[id] = NormalizedScore((float64(v) - min) / span)
	}
	return out
}

// findMinMax inspects raw entries indexed by pool. Returns ok=false when
// the pool has no entries that exist in raw (treated as fully degenerate).
func findMinMax(raw map[AnimeID]RawScore, pool []AnimeID) (min, max float64, ok bool) {
	for _, id := range pool {
		v, present := raw[id]
		if !present {
			continue
		}
		f := float64(v)
		if !ok {
			min, max = f, f
			ok = true
			continue
		}
		if f < min {
			min = f
		}
		if f > max {
			max = f
		}
	}
	return min, max, ok
}
