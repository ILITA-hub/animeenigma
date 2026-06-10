package service

import (
	"context"
	"fmt"
	"math/rand/v2"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/config"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/gacha/internal/repo"
	"gorm.io/gorm"
)

// rarityOrder lists tiers from lowest to highest. "Below" a tier means a LOWER
// index (lower rarity); "above" means a higher index. Redistribution of a
// missing tier's weight prefers the nearest available tier BELOW (spec §5.3),
// falling back to the nearest above when nothing is below.
var rarityOrder = []domain.Rarity{domain.RarityN, domain.RarityR, domain.RaritySR, domain.RaritySSR}

// tierEntry is one row of the cumulative weight table. cumulative is the
// running sum of weights up to AND INCLUDING this tier; the table is ordered
// ascending by rarity so ranges are [prev.cumulative, this.cumulative).
type tierEntry struct {
	rarity     domain.Rarity
	cumulative int
}

// buildTierTable builds the cumulative weight table over the tiers that
// actually have cards in `available`. A missing tier's weight is redistributed
// to the nearest available tier BELOW (SSR→SR→R→N); if no tier is below, to the
// nearest available tier ABOVE. The returned table is ordered ascending by
// rarity (N, R, SR, SSR) and contains only tiers with cards. If no tier has
// cards, the table is empty.
func buildTierTable(weights map[domain.Rarity]int, available map[domain.Rarity][]domain.Card) []tierEntry {
	// Effective weight accumulated per available tier.
	eff := make(map[domain.Rarity]int)
	for _, r := range rarityOrder {
		if len(available[r]) > 0 {
			eff[r] = 0 // mark present
		}
	}

	hasCards := func(r domain.Rarity) bool { return len(available[r]) > 0 }

	for i, r := range rarityOrder {
		w := weights[r]
		if w == 0 {
			continue
		}
		target := r
		if !hasCards(r) {
			// Find nearest available tier below (lower index).
			found := false
			for j := i - 1; j >= 0; j-- {
				if hasCards(rarityOrder[j]) {
					target = rarityOrder[j]
					found = true
					break
				}
			}
			if !found {
				// Nothing below — go to nearest available above (higher index).
				for j := i + 1; j < len(rarityOrder); j++ {
					if hasCards(rarityOrder[j]) {
						target = rarityOrder[j]
						found = true
						break
					}
				}
			}
			if !found {
				// No tier has cards at all — drop this weight.
				continue
			}
		}
		eff[target] += w
	}

	table := make([]tierEntry, 0, len(eff))
	cum := 0
	for _, r := range rarityOrder {
		if !hasCards(r) {
			continue
		}
		cum += eff[r]
		table = append(table, tierEntry{rarity: r, cumulative: cum})
	}
	return table
}

// rollOne picks a tier by weight using randInt(total), then a uniform card from
// that tier. When forceTier is a non-empty rarity present in the pool, it
// overrides the weighted pick (used for pity-forced SSR and the x10 SR-floor).
// The table must be non-empty (guaranteed by the caller validating the pool).
func rollOne(table []tierEntry, pool map[domain.Rarity][]domain.Card, randInt func(int) int, forceTier domain.Rarity) domain.Card {
	var tier domain.Rarity
	if forceTier != "" && len(pool[forceTier]) > 0 {
		tier = forceTier
	} else {
		total := table[len(table)-1].cumulative
		pick := randInt(total)
		for _, e := range table {
			if pick < e.cumulative {
				tier = e.rarity
				break
			}
		}
		if tier == "" {
			// Defensive: shouldn't happen (pick < total), fall to top tier.
			tier = table[len(table)-1].rarity
		}
	}
	cards := pool[tier]
	idx := randInt(len(cards))
	return cards[idx]
}

// PulledCard is a single rolled card plus its post-pull collection status.
type PulledCard struct {
	Card  domain.Card `json:"card"`
	New   bool        `json:"new"`
	Count int         `json:"count"`
}

// PullResult is the outcome of a x1/x10 pull request.
type PullResult struct {
	Cards   []PulledCard `json:"cards"`
	Balance int64        `json:"balance"`
	Pity    int          `json:"pity"`
}

// PullService is the pull-engine use-case layer. randInt is injected so tests
// can be deterministic; production uses math/rand/v2 (see NewSecureRand).
type PullService struct {
	db      *gorm.DB
	pull    *repo.PullRepository
	banners *repo.BannerRepository
	econ    config.EconomyConfig
	randInt func(int) int
	log     *logger.Logger
}

// NewPullService wires the pull engine. The DB is taken from the banner repo's
// connection so the orchestration transaction shares the same handle.
func NewPullService(
	pullRepo *repo.PullRepository,
	bannerRepo *repo.BannerRepository,
	econ config.EconomyConfig,
	randInt func(int) int,
	log *logger.Logger,
) *PullService {
	return &PullService{
		db:      bannerRepo.DB(),
		pull:    pullRepo,
		banners: bannerRepo,
		econ:    econ,
		randInt: randInt,
		log:     log,
	}
}

// NewSecureRand returns the production randInt backed by math/rand/v2
// (auto-seeded, not cryptographic but server-side-only and statistically fine
// for gacha odds — the spec forbids weak client-side rolls, not math/rand/v2).
func NewSecureRand() func(int) int {
	return func(n int) int {
		if n <= 0 {
			return 0
		}
		return rand.IntN(n)
	}
}

// Pull executes a x1 or x10 pull on a banner: it atomically debits the cost,
// rolls rarity-weighted cards with the x10 SR-floor and per-banner hard-pity,
// records the collection, and returns what was pulled. The ENTIRE operation is
// one transaction — insufficient funds or any error rolls everything back with
// no side effects.
func (s *PullService) Pull(ctx context.Context, userID, bannerID, mode string) (*PullResult, error) {
	// Mode → count + cost + ledger reason.
	var count int
	var cost int64
	var reason string
	switch mode {
	case "x1":
		count, cost, reason = 1, s.econ.PullCostX1, domain.ReasonPullX1
	case "x10":
		count, cost, reason = 10, s.econ.PullCostX10, domain.ReasonPullX10
	default:
		return nil, apperrors.InvalidInput("mode must be x1 or x10")
	}

	// Banner gating: must exist, be active now, and have a non-empty pool.
	banner, err := s.banners.GetBanner(ctx, bannerID)
	if err != nil {
		return nil, err
	}
	if !bannerActiveNow(banner, time.Now()) {
		return nil, apperrors.InvalidInput("banner is not active")
	}
	pool, err := s.pull.CardsByRarity(ctx, bannerID)
	if err != nil {
		return nil, err
	}
	if poolSize(pool) == 0 {
		return nil, apperrors.InvalidInput("banner has no cards")
	}

	table := buildTierTable(econWeights(s.econ), pool)
	if len(table) == 0 {
		return nil, apperrors.InvalidInput("banner has no rollable cards")
	}

	pullID := newPullID()
	var result PullResult

	txErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// a. Conditional debit + ledger. Insufficient funds aborts everything.
		if err := repo.DebitTx(tx, userID, cost, reason, pullID); err != nil {
			return err
		}

		// b. Lock the pity row for this (user, banner).
		pity, err := s.pull.GetPityForUpdate(tx, userID, bannerID)
		if err != nil {
			return err
		}

		// c. Roll each card.
		rolled := make([]domain.Card, 0, count)
		for i := 0; i < count; i++ {
			pity.PullsSinceSSR++
			var force domain.Rarity
			if pity.PullsSinceSSR >= s.econ.PityThreshold {
				// Hard pity: force SSR if available, else the highest tier present.
				if len(pool[domain.RaritySSR]) > 0 {
					force = domain.RaritySSR
				} else {
					force = highestAvailable(pool)
				}
			}
			c := rollOne(table, pool, s.randInt, force)
			if c.Rarity == domain.RaritySSR {
				pity.PullsSinceSSR = 0
			}
			rolled = append(rolled, c)
		}

		// d. x10 SR-floor: guarantee ≥1 SR+ in a pack of ten.
		if count == 10 && !hasSRPlus(rolled) {
			var floor domain.Rarity
			switch {
			case len(pool[domain.RaritySR]) > 0:
				floor = domain.RaritySR
			case len(pool[domain.RaritySSR]) > 0:
				floor = domain.RaritySSR
			}
			if floor != "" {
				c := rollOne(table, pool, s.randInt, floor)
				rolled[len(rolled)-1] = c
				if c.Rarity == domain.RaritySSR {
					// Forced-SSR floor also resets pity.
					pity.PullsSinceSSR = 0
				}
			} else {
				// Pool has neither SR nor SSR — floor is unsatisfiable.
				s.log.Warnw("x10 SR-floor unsatisfiable (pool has no SR/SSR)", "banner_id", bannerID)
			}
		}

		// e. Record collection + persist pity.
		ids := make([]string, len(rolled))
		for i, c := range rolled {
			ids[i] = c.ID
		}
		newIDs, counts, err := s.pull.AddToCollectionTx(tx, userID, ids)
		if err != nil {
			return err
		}
		if err := s.pull.SavePityTx(tx, pity); err != nil {
			return err
		}

		// Build the result (inside the tx so it reflects committed state).
		// Mark New only on the FIRST occurrence of a freshly-obtained card.
		newSeen := make(map[string]bool)
		result.Cards = make([]PulledCard, len(rolled))
		for i, c := range rolled {
			isNew := false
			if newIDs[c.ID] && !newSeen[c.ID] {
				isNew = true
				newSeen[c.ID] = true
			}
			result.Cards[i] = PulledCard{Card: c, New: isNew, Count: counts[c.ID]}
		}
		result.Pity = pity.PullsSinceSSR

		// Read the post-debit balance.
		var w domain.Wallet
		if err := tx.First(&w, "user_id = ?", userID).Error; err != nil {
			return err
		}
		result.Balance = w.Balance
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return &result, nil
}

// econWeights returns the tier-weight map from config.
func econWeights(e config.EconomyConfig) map[domain.Rarity]int {
	return map[domain.Rarity]int{
		domain.RarityN:   e.WeightN,
		domain.RarityR:   e.WeightR,
		domain.RaritySR:  e.WeightSR,
		domain.RaritySSR: e.WeightSSR,
	}
}

func poolSize(pool map[domain.Rarity][]domain.Card) int {
	n := 0
	for _, cs := range pool {
		n += len(cs)
	}
	return n
}

func hasSRPlus(cards []domain.Card) bool {
	for _, c := range cards {
		if c.Rarity == domain.RaritySR || c.Rarity == domain.RaritySSR {
			return true
		}
	}
	return false
}

// highestAvailable returns the highest-rarity tier that has cards in the pool.
func highestAvailable(pool map[domain.Rarity][]domain.Card) domain.Rarity {
	for i := len(rarityOrder) - 1; i >= 0; i-- {
		if len(pool[rarityOrder[i]]) > 0 {
			return rarityOrder[i]
		}
	}
	return ""
}

// bannerActiveNow mirrors BannerRepository.ActiveNow's predicate for a single
// banner at a given instant.
func bannerActiveNow(b *domain.Banner, now time.Time) bool {
	if !b.Enabled {
		return false
	}
	if b.ActiveFrom != nil && b.ActiveFrom.After(now) {
		return false
	}
	if b.ActiveTo != nil && b.ActiveTo.Before(now) {
		return false
	}
	return true
}

// newPullID generates a unique ref for the pull's ledger entry (audit trail).
func newPullID() string {
	return fmt.Sprintf("pull-%d-%d", time.Now().UnixNano(), rand.IntN(1_000_000))
}
