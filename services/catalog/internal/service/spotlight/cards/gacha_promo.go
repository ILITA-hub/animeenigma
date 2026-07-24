package cards

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// gachaPromoPriority pins the gacha promo card to the front of the
// carousel: the frontend treats any priority >= its PINNED_PRIORITY_MIN
// (2.0, see frontend/web/src/components/home/spotlight/weightedRandom.ts)
// as "always open on this card and order it first" — unlike curated's 1.5,
// which only biases the random opening pick.
const gachaPromoPriority = 5.0

// Locked gacha economy constants (spec 2026-06-09 «Лудка»): x1/x10 pull
// costs in Энигмы and the hard-pity pull number that guarantees an SSR.
// They are product decisions mirrored from services/gacha's pull engine —
// surfaced in the payload so the card copy can't drift from the real rules.
const (
	gachaPullCostSingle = 100
	gachaPullCostTen    = 900
	gachaPitySSRAt      = 90
)

// GachaPromoResolver implements spotlight.Resolver for the `gacha_promo`
// card — a static feature-launch promo for «Лудка» (the gacha). It has no
// upstream calls and no per-day variance, so it needs neither cache nor
// logger and can never miss the 800ms per-card deadline.
type GachaPromoResolver struct{}

// NewGachaPromoResolver constructs the resolver.
func NewGachaPromoResolver() *GachaPromoResolver { return &GachaPromoResolver{} }

// Type returns the card discriminator consumed by the frontend union.
func (r *GachaPromoResolver) Type() string { return "gacha_promo" }

// Resolve returns the promo card unconditionally — the feature is rolled
// out to everyone (policy-service flag `gacha`), and the /gacha route
// itself handles auth. userID is ignored.
func (r *GachaPromoResolver) Resolve(_ context.Context, _ *string) (*spotlight.Card, error) {
	return &spotlight.Card{
		Type:     r.Type(),
		Priority: gachaPromoPriority,
		Data: spotlight.GachaPromoData{
			PullCostSingle: gachaPullCostSingle,
			PullCostTen:    gachaPullCostTen,
			PitySSRAt:      gachaPitySSRAt,
		},
	}, nil
}
