// golden.go — static "always-popular" anime pool the liveness probe
// exercises (per D2). Each probe tick randomly selects ONE entry; over
// the course of an hour the probe touches ~4 of the 5 entries, which gives
// us enough variety to catch upstream selector drift specific to one
// anime's metadata shape without amplifying request volume.
//
// Maintenance: MAL IDs were verified against jikan.moe on 2026-05-12.
// Wrong MAL IDs cause permanent false-negatives (the probe's FindID stage
// can't resolve the anime, so search-stage failures pile up and flip the
// gauge to 0 even when upstream is healthy). This list is intentionally
// short and hand-curated — generating it from a "top 100 of all time"
// API would couple probe health to a third-party catalog change.
//
// NOTE on field naming: domain.AnimeRef has NO `MalID` field — instead
// `ShikimoriID` IS the MAL ID per the upstream contract (Shikimori reuses
// MAL's numbering). The acceptance-criteria grep below verifies the
// numeric IDs are stored under their canonical field name; the value
// itself is the MAL ID (verified against jikan.moe).
package health

import (
	"math/rand/v2"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// goldenEntry pairs an AnimeRef with its MAL ID metadata for documentation
// + test assertions. The MalID field is the same numeric value as
// AnimeRef.ShikimoriID — duplicated here ONLY so probe-pool maintenance
// can be reasoned about by anime ID without grepping for the dual-purpose
// ShikimoriID semantics. The probe uses entry.Ref directly.
type goldenEntry struct {
	MalID string // numeric MAL ID (verified 2026-05-12). Same value as Ref.ShikimoriID.
	Ref   domain.AnimeRef
}

// goldenEntries is the canonical maintenance list (with MalID kept visible
// for human review). DefaultGoldenPool is derived from this — code paths
// should consume DefaultGoldenPool, not this slice.
var goldenEntries = []goldenEntry{
	{MalID: "20", Ref: domain.AnimeRef{ShikimoriID: "20", Title: "Naruto", Year: 2002}},
	{MalID: "21", Ref: domain.AnimeRef{ShikimoriID: "21", Title: "One Piece", Year: 1999}},
	{MalID: "16498", Ref: domain.AnimeRef{ShikimoriID: "16498", Title: "Attack on Titan", Year: 2013}},
	{MalID: "38000", Ref: domain.AnimeRef{ShikimoriID: "38000", Title: "Demon Slayer", Year: 2019}},
	{MalID: "40748", Ref: domain.AnimeRef{ShikimoriID: "40748", Title: "Jujutsu Kaisen", Year: 2020}},
}

// DefaultGoldenPool is the static 5-entry list of AnimeRefs the liveness
// probe rotates through. Order is not significant; Pick() draws uniformly
// at random. Derived from goldenEntries above.
var DefaultGoldenPool = func() []domain.AnimeRef {
	out := make([]domain.AnimeRef, 0, len(goldenEntries))
	for _, e := range goldenEntries {
		out = append(out, e.Ref)
	}
	return out
}()

// Pick returns a random AnimeRef from the pool. Caller MUST inject `rng`
// so tests are deterministic. For an empty pool, returns the zero AnimeRef
// (a probe with an empty pool will record FindID failures, which is the
// correct behavior — a misconfigured probe should look unhealthy).
func Pick(pool []domain.AnimeRef, rng *rand.Rand) domain.AnimeRef {
	if len(pool) == 0 {
		return domain.AnimeRef{}
	}
	return pool[rng.IntN(len(pool))]
}
