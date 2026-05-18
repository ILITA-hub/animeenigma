// Package service holds the library service's domain orchestration —
// the merger, queueing, and (in later phases) the encoder lifecycle.
//
// SearchAggregator fans out a single admin search across both the Nyaa
// and AnimeTosho parsers, then merges + dedupes + ranks the result.
// Fail-soft is the headline contract: when one provider goes down the
// other's hits still flow back to the caller, and the dead provider's
// name appears in Result.ProvidersDown. Only when BOTH fail does
// FetchAll return a non-nil error (the handler maps that to 502).
package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
)

// SearchParams is the per-call input. Limit is clamped to [1, 200]
// (default 50 when <= 0). Either Query or MALID (or both) must be
// non-zero — the handler validates this before calling FetchAll.
type SearchParams struct {
	Query string
	MALID int
	Limit int
}

// Result wraps the merged release slice and the list of providers that
// failed during this call. ProvidersDown is non-nil but possibly empty.
type Result struct {
	Releases      []domain.Release
	ProvidersDown []string
}

// AnimeToshoParams is a local mirror of the AnimeTosho client's
// SearchParams type. We re-declare it here instead of importing the
// parser package so the AnimeToshoSearcher interface (and tests) can
// satisfy the contract with a hand-rolled fake. The shape matches
// services/library/internal/parser/animetosho.SearchParams exactly.
type AnimeToshoParams struct {
	MALID int
	Query string
	Limit int
}

// NyaaSearcher is the local interface satisfied by *nyaa.Client. The
// service constructor takes this interface (not the concrete type) so
// tests can inject a fake without spinning up an httptest server for
// every assertion.
type NyaaSearcher interface {
	Search(ctx context.Context, query string, limit int) ([]domain.Release, error)
}

// AnimeToshoSearcher is the local interface satisfied by
// *animetosho.Client. The parser's concrete SearchParams type is
// assignable to AnimeToshoParams (they have identical fields), but to
// keep the boundary clean we accept an adapter in main.go that
// translates the call.
type AnimeToshoSearcher interface {
	Search(ctx context.Context, p AnimeToshoParams) ([]domain.Release, error)
}

const (
	defaultLimit = 50
	maxLimit     = 200
)

// SearchAggregator runs both clients in parallel and merges the results.
type SearchAggregator struct {
	nyaa NyaaSearcher
	at   AnimeToshoSearcher
	log  *logger.Logger
}

// NewAggregator wires the two clients and a logger. None of the
// arguments may be nil.
func NewAggregator(nyaa NyaaSearcher, at AnimeToshoSearcher, log *logger.Logger) *SearchAggregator {
	return &SearchAggregator{nyaa: nyaa, at: at, log: log}
}

// FetchAll fans out to Nyaa + AnimeTosho in parallel, merges by
// InfoHash, dedupes, ranks AnimeTosho-with-matching-MAL-ID first, and
// clamps to the requested limit.
//
// Fail-soft semantics: each goroutine catches its own error and
// reports it via the providersDown channel instead of propagating it
// to the caller. The function returns a non-nil error only when BOTH
// providers fail (the handler maps that to a 502 ExternalAPI error).
func (a *SearchAggregator) FetchAll(ctx context.Context, p SearchParams) (Result, error) {
	limit := clampLimit(p.Limit)

	var (
		wg            sync.WaitGroup
		mu            sync.Mutex
		nyaaReleases  []domain.Release
		atReleases    []domain.Release
		providersDown []string
		nyaaErr       error
		atErr         error
	)

	wg.Add(2)
	go func() {
		defer wg.Done()
		rs, err := a.nyaa.Search(ctx, p.Query, limit)
		if err != nil {
			a.log.Warnw("library search provider failed", "provider", "nyaa", "error", err)
			mu.Lock()
			providersDown = append(providersDown, "nyaa")
			nyaaErr = err
			mu.Unlock()
			return
		}
		mu.Lock()
		nyaaReleases = rs
		mu.Unlock()
	}()

	go func() {
		defer wg.Done()
		rs, err := a.at.Search(ctx, AnimeToshoParams{
			MALID: p.MALID,
			Query: p.Query,
			Limit: limit,
		})
		if err != nil {
			a.log.Warnw("library search provider failed", "provider", "animetosho", "error", err)
			mu.Lock()
			providersDown = append(providersDown, "animetosho")
			atErr = err
			mu.Unlock()
			return
		}
		mu.Lock()
		atReleases = rs
		mu.Unlock()
	}()

	wg.Wait()

	// Both providers failed → surface to caller as a hard error so the
	// handler can return 502. ProvidersDown is still returned for
	// observability in the error path.
	if nyaaErr != nil && atErr != nil {
		return Result{ProvidersDown: providersDown},
			fmt.Errorf("both providers failed: nyaa=%v animetosho=%v", nyaaErr, atErr)
	}

	merged := mergeAndRank(nyaaReleases, atReleases, p.MALID, a.log)
	if len(merged) > limit {
		merged = merged[:limit]
	}
	return Result{Releases: merged, ProvidersDown: providersDown}, nil
}

// mergeAndRank dedupes by InfoHash, prefers AnimeTosho on collision,
// then splits into "matching-MAL-ID animetosho hits first" + "everything
// else", sorting each sub-slice by FoundAt DESC. Entries with empty
// InfoHash are dropped (the merger cannot dedupe them and downstream
// job-queueing also requires an info hash).
func mergeAndRank(nyaa, at []domain.Release, malID int, log *logger.Logger) []domain.Release {
	// Process AnimeTosho FIRST so that on InfoHash collision the
	// AnimeTosho copy wins (richer metadata per SPEC). Map preserves
	// uniqueness, but we walk both slices in a defined order so the
	// `Source=="animetosho"` preference is deterministic.
	merged := make(map[string]domain.Release, len(nyaa)+len(at))

	insert := func(r domain.Release, source string) {
		key := strings.ToLower(strings.TrimSpace(r.InfoHash))
		if key == "" {
			log.Debugw("library: dropping release with empty InfoHash",
				"source", source, "title", r.Title)
			return
		}
		existing, ok := merged[key]
		if !ok {
			merged[key] = r
			return
		}
		// Collision: prefer animetosho copy. If both are the same source,
		// keep the first occurrence (existing) — no-op.
		if r.Source == "animetosho" && existing.Source != "animetosho" {
			merged[key] = r
		}
	}

	for _, r := range at {
		insert(r, "animetosho")
	}
	for _, r := range nyaa {
		insert(r, "nyaa")
	}

	var headed, tail []domain.Release
	for _, r := range merged {
		if malID > 0 && r.Source == "animetosho" && r.MALID == malID {
			headed = append(headed, r)
		} else {
			tail = append(tail, r)
		}
	}

	sort.Slice(headed, func(i, j int) bool { return headed[i].FoundAt.After(headed[j].FoundAt) })
	sort.Slice(tail, func(i, j int) bool { return tail[i].FoundAt.After(tail[j].FoundAt) })

	out := make([]domain.Release, 0, len(headed)+len(tail))
	out = append(out, headed...)
	out = append(out, tail...)
	return out
}

// clampLimit normalizes a caller-provided Limit to [1, maxLimit].
func clampLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > maxLimit {
		return maxLimit
	}
	return limit
}
