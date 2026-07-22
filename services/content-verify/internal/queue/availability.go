// availability.go — provider-availability gating for the claim + enumerate
// paths (owner directive 2026-07-20).
//
// Two independent signals decide that probing a provider right now is a
// waste of a probe slot (and, for browser-engine providers, of a Camoufox
// session against a walled site):
//
//  1. Deferrals: a scraper endpoint answered 503 — the chain for that
//     (anime, provider) is down or negative-cached upstream. We defer the
//     pair for exactly the server-advertised retry_after_seconds, i.e.
//     until the upstream negative-cache entry is gone.
//  2. Roster health: the catalog's authoritative provider roster says the
//     provider's probe-driven health is "down" (confirmed failing). Gated
//     to scraper-operated rows — kodik/animejoy/ae health is not maintained
//     by that probe and must not be muted by it.
//
// Both are fail-open: no roster (fetch error) and no deferral ⇒ probe as
// before. State is in-process only, matching the engine's lease model
// (replicas MUST stay at 1 — see Worker doc).
package queue

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
)

// rosterTTL bounds how often the roster refetches. Well under the scraper
// probe cadence (~15min) so a recovery is picked up quickly, well over the
// 10s claim cadence so the roster route isn't hammered.
const rosterTTL = 2 * time.Minute

func deferKey(animeID, provider string) string { return provider + "|" + animeID }

// Defer marks (anime, provider) unavailable for retryAfter. Called by the
// worker when a probe resolve got a 503, and by the enumerate hooks.
func (e *Engine) Defer(animeID, provider string, retryAfter time.Duration) {
	if retryAfter <= 0 {
		return
	}
	until := e.now().Add(retryAfter)
	e.mu.Lock()
	e.deferUntil[deferKey(animeID, provider)] = until
	e.mu.Unlock()
	cvmetrics.ProviderDeferralsTotal.WithLabelValues(provider).Inc()
	if e.log != nil {
		e.log.Infow("provider deferred (upstream 503)", "anime_id", animeID,
			"provider", provider, "until", until.UTC().Format(time.RFC3339))
	}
}

// deferred reports whether (anime, provider) is inside a deferral window.
// Expired entries are pruned on read.
func (e *Engine) deferred(animeID, provider string) bool {
	k := deferKey(animeID, provider)
	now := e.now()
	e.mu.Lock()
	defer e.mu.Unlock()
	until, ok := e.deferUntil[k]
	if !ok {
		return false
	}
	if now.After(until) {
		delete(e.deferUntil, k)
		return false
	}
	return true
}

// providerDown reports whether the roster marks the provider health="down".
// The roster is refreshed lazily behind rosterTTL; the HTTP call runs
// WITHOUT holding mu (same pattern as interest()). Any fetch error keeps the
// previous snapshot (fail-open — a roster outage must not blank probing).
func (e *Engine) providerDown(ctx context.Context, provider string) bool {
	e.mu.Lock()
	fresh := e.now().Sub(e.rosterAt) < rosterTTL && e.rosterDown != nil
	down := e.rosterDown[provider]
	e.mu.Unlock()
	if fresh {
		return down
	}

	rows, err := e.cat.ScraperRoster(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("scraper roster fetch failed; keeping previous availability", "error", err)
		}
		e.mu.Lock()
		e.rosterAt = e.now() // back off rosterTTL before retrying the fetch
		down = e.rosterDown[provider]
		e.mu.Unlock()
		return down
	}
	downSet := map[string]bool{}
	for _, r := range rows {
		if r.ScraperOperated && r.Health == "down" {
			downSet[r.Name] = true
		}
	}
	e.mu.Lock()
	e.rosterDown, e.rosterAt = downSet, e.now()
	e.mu.Unlock()
	return downSet[provider]
}

// unavailable is the combined gate used by the claim + enumerate paths.
func (e *Engine) unavailable(ctx context.Context, animeID, provider string) bool {
	return e.deferred(animeID, provider) || e.providerDown(ctx, provider)
}

// filterAvailableUnits drops verify units whose provider is currently
// unavailable. Every remaining unit is a real probe (ae/kodik known-truth
// verdicts are synthesized at read time in catalog, never enumerated here).
func (e *Engine) filterAvailableUnits(ctx context.Context, units []Unit) []Unit {
	out := units[:0:0]
	for _, u := range units {
		if e.unavailable(ctx, u.AnimeID, u.Provider) {
			continue
		}
		out = append(out, u)
	}
	return out
}

// filterAvailableSkips is filterAvailableUnits for the skip lane.
func (e *Engine) filterAvailableSkips(ctx context.Context, units []SkipUnit) []SkipUnit {
	out := units[:0:0]
	for _, u := range units {
		if e.unavailable(ctx, u.AnimeID, u.Provider) {
			continue
		}
		out = append(out, u)
	}
	return out
}
