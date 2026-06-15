// jackett_tier.go layers the Jackett multi-indexer aggregator over the
// existing Nyaa+AnimeTosho SearchAggregator as a strict PRIMARY → FALLBACK
// tier (not a merge).
//
// Rationale: Jackett already dedupes across ~20 indexers internally, so
// re-merging its output with the two legacy providers would be redundant
// and would dilute Jackett's seeder-ranked ordering. Instead we try
// Jackett first; only when it errors or returns nothing do we run the
// legacy aggregator. This preserves the aggregator's fail-soft contract
// (a 502 only when BOTH legacy providers are down) and keeps the dead-swarm
// fix — Jackett's Seeders ranking — intact on the happy path.
package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// JackettSearcher is the local interface satisfied directly by
// *jackett.Client. Kept here (not imported from the parser package) so the
// service layer stays free of any parser import — main.go owns the wiring.
type JackettSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]domain.Release, error)
}

// FallbackSearcher is the legacy two-provider aggregator's contract.
// *SearchAggregator satisfies it.
type FallbackSearcher interface {
	FetchAll(ctx context.Context, p SearchParams) (Result, error)
}

// TieredSearcher runs Jackett as the primary tier and falls back to the
// legacy aggregator. A nil jackett (operator left JACKETT_API_KEY empty)
// makes it a transparent pass-through to the fallback — identical to the
// pre-Jackett behaviour.
type TieredSearcher struct {
	jackett  JackettSearcher
	fallback FallbackSearcher
	log      *logger.Logger
}

// NewTieredSearcher wires the primary + fallback. jackett may be nil
// (Jackett disabled); fallback and log must be non-nil.
func NewTieredSearcher(jackett JackettSearcher, fallback FallbackSearcher, log *logger.Logger) *TieredSearcher {
	return &TieredSearcher{jackett: jackett, fallback: fallback, log: log}
}

// FetchAll implements the handler's search contract.
//
// Tier logic:
//   - Jackett is skipped entirely when disabled (nil) or when the call is
//     MAL-ID-only (Query==""), since Jackett has no MAL feed — those go
//     straight to the fallback (AnimeTosho's MAL path).
//   - Otherwise Jackett runs first. A non-empty, error-free result wins
//     outright (already seeder-ranked + limit-capped by the client).
//   - On Jackett error OR zero results, control falls through to the
//     legacy aggregator. When Jackett errored, "jackett" is prepended to
//     the fallback's ProvidersDown for observability.
func (t *TieredSearcher) FetchAll(ctx context.Context, p SearchParams) (Result, error) {
	jackettDown := false

	if t.jackett != nil && p.Query != "" {
		releases, err := t.jackett.Search(ctx, p.Query, p.Limit)
		switch {
		case err != nil:
			t.log.Warnw("library search: jackett primary failed, falling back",
				"q", p.Query, "error", err)
			jackettDown = true
		case len(releases) > 0:
			return Result{Releases: releases, ProvidersDown: []string{}}, nil
		default:
			t.log.Debugw("library search: jackett returned no results, falling back",
				"q", p.Query)
		}
	}

	res, err := t.fallback.FetchAll(ctx, p)
	if jackettDown {
		res.ProvidersDown = append([]string{"jackett"}, res.ProvidersDown...)
	}
	return res, err
}
