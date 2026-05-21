// Workstream hero-spotlight v1.0 Phase 3 — Plan 02 Task 2.
//
// AdaptiveSlice implements the HSB-BE-30 1-2-3 layout rule used by 4
// multi-item resolvers: personal_pick (Plan 03), latest_news (Phase 1 —
// Plan 04 retrofits), now_watching (Plan 03), telegram_news (Plan 03).
// Pulling this out into a generic helper avoids 4 copies of the same
// len-switch logic and gives the N=2 random-pick branch a single
// well-tested implementation.

package spotlight

import "math/rand"

// AdaptiveSlice applies the spotlight 1-2-3 layout rule to items:
//
//   - len(items) == 0 → returns nil. Resolvers MUST treat nil as
//     eligibility=false and return (nil, nil) from Resolve.
//   - len(items) == 1 → returns items unchanged (single-element passthrough).
//   - len(items) == 2 → returns a fresh single-element slice holding one
//     randomly-picked element via rng.Intn(2). rng MUST be non-nil; passing
//     nil panics with a descriptive message because the random pick is a
//     correctness requirement of the spec, not a fallback.
//   - len(items) >= 3 → returns items[:3] (sub-slice; callers MUST treat as
//     read-only — modifying the returned slice would mutate the caller's
//     backing array).
//
// The N==2 random pick is the only non-trivial branch. The rng is injected
// so resolver tests can pin the choice deterministically via
// rand.New(rand.NewSource(seed)).
//
// math/rand (non-crypto) is fine here — the choice is presentational
// (which of two equally-eligible items to show on the carousel) and has no
// privacy or security implication (see Plan 02 threat-model T-03-09).
//
// Per HSB-BE-30 + 03-CONTEXT.md `<decisions>` Adaptive 1-2-3 layout section.
func AdaptiveSlice[T any](items []T, rng *rand.Rand) []T {
	switch len(items) {
	case 0:
		return nil
	case 1:
		return items
	case 2:
		if rng == nil {
			panic("spotlight.AdaptiveSlice: rng is required when len(items) == 2 (the random-pick branch)")
		}
		idx := rng.Intn(2)
		return []T{items[idx]}
	default:
		return items[:3]
	}
}
