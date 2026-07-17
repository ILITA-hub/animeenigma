package probe

import "context"

// AllanimeOkruAnimeSet wraps the shared spotlight AnimeSet but pins
// allanime-okru's anchor slot (index 0 — the only ref probed under the
// down-state single-sample plan, and the sole driver of pass/fail since this
// provider always runs fail_fast=false, see ScraperProvider.ProbeSample) to a
// title with a working ok.ru copy.
//
// The platform-wide anchor is permanently copyright-blocked on ok.ru for this
// provider specifically ("no data-options" on extraction — a per-title block,
// not an outage), so the automated probe can never observe allanime-okru as
// healthy even though it serves real content for the vast majority of titles.
// See docs/issues/provider-recovery-log.md 2026-07-07 and 2026-07-15.
//
// The override is placed first unconditionally rather than relying on the
// inner set's Score-descending sort: the override title's own popularity
// score can be outranked by featured/random spotlight picks on any given day,
// which would silently un-pin it from index 0 and defeat the fix.
type AllanimeOkruAnimeSet struct {
	inner                      AnimeSetResolver
	overrideUUID, overrideName string
}

// NewAllanimeOkruAnimeSet builds the decorator. overrideUUID/overrideName
// should be a title independently verified to have a real, playable ok.ru
// copy (see the recovery log for the current pick and why).
func NewAllanimeOkruAnimeSet(inner AnimeSetResolver, overrideUUID, overrideName string) *AllanimeOkruAnimeSet {
	return &AllanimeOkruAnimeSet{inner: inner, overrideUUID: overrideUUID, overrideName: overrideName}
}

func (a *AllanimeOkruAnimeSet) Resolve(ctx context.Context) ([]AnimeRef, error) {
	refs, _ := a.inner.Resolve(ctx) // inner errors are non-fatal — the override anchor never depends on spotlight
	out := make([]AnimeRef, 0, len(refs)+1)
	out = append(out, AnimeRef{UUID: a.overrideUUID, Name: a.overrideName, Slot: SlotAnchor})
	for _, r := range refs {
		if r.Slot == SlotAnchor {
			continue // drop the shared anchor — already replaced above
		}
		out = append(out, r)
	}
	return out, nil
}
