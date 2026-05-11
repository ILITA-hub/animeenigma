// Package service contains the scraper orchestrator. Phase 15 ships the
// sequential failover loop + embed-registry plumbing + health snapshot;
// Phase 16+ registers real providers without changing any code here.
//
// SCRAPER-FOUND-04: providers know nothing about each other. The orchestrator
// owns failover order, error categorization, and observability of fallback
// events.
package service

import (
	"context"
	"errors"
	"sync"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// Orchestrator runs business methods against a registered list of providers
// with sequential failover. It also owns the embed extractor registry so the
// HTTP handler layer has a single dependency (the orchestrator) for every
// scraper concern.
//
// Construct via NewOrchestrator. Zero value is not usable.
type Orchestrator struct {
	mu        sync.RWMutex
	providers []domain.Provider
	registry  *domain.Registry
	log       *logger.Logger
}

// NewOrchestrator builds an orchestrator with zero providers. Use Register
// to add providers (Phase 16+).
func NewOrchestrator(log *logger.Logger, registry *domain.Registry) *Orchestrator {
	if registry == nil {
		registry = domain.NewRegistry()
	}
	return &Orchestrator{
		providers: make([]domain.Provider, 0, 4),
		registry:  registry,
		log:       log,
	}
}

// Register appends a provider to the failover chain. Order matters: the
// first-registered provider is tried first unless overridden by a `prefer`
// argument at call time.
func (o *Orchestrator) Register(p domain.Provider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.providers = append(o.providers, p)
}

// EmbedRegistry exposes the embed extractor registry for callers that need
// to enumerate or invoke extractors directly (e.g. /scraper/health).
func (o *Orchestrator) EmbedRegistry() *domain.Registry {
	return o.registry
}

// orderedProviders returns the provider iteration order honoring `prefer`.
// If prefer matches a registered provider's Name(), that provider is moved
// to position 0; the remainder stays in registration order. Unknown prefer
// values are silently ignored (caller-supplied input is not trusted).
//
// The implementation tracks the preferred provider by INDEX so the second
// loop can skip it unconditionally — a previous version compared against
// `len(out) == 1` which produced a duplicate once the loop appended any
// non-preferred provider (advancing len(out) to 2). See REVIEW.md CR-01.
func (o *Orchestrator) orderedProviders(prefer string) []domain.Provider {
	o.mu.RLock()
	defer o.mu.RUnlock()
	if len(o.providers) == 0 {
		return nil
	}
	out := make([]domain.Provider, 0, len(o.providers))
	preferredIdx := -1
	if prefer != "" {
		for i, p := range o.providers {
			if p.Name() == prefer {
				preferredIdx = i
				out = append(out, p)
				break
			}
		}
	}
	for i, p := range o.providers {
		if i == preferredIdx {
			continue // already inserted at position 0
		}
		out = append(out, p)
	}
	return out
}

// failoverDecision classifies an error to decide whether the loop continues.
// On retryable errors we return (true, "<reason>"); on terminal errors we
// return (false, ""). Terminal errors short-circuit the loop and propagate
// to the caller verbatim.
func failoverDecision(err error) (retry bool, kind string) {
	switch {
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		// Context cancellation is terminal — the caller asked us to stop.
		return false, ""
	case errors.Is(err, domain.ErrNotFound):
		return true, "not_found"
	case errors.Is(err, domain.ErrProviderDown):
		return true, "provider_down"
	case errors.Is(err, domain.ErrExtractFailed):
		return true, "extract_failed"
	default:
		// Defensive: unknown error → treat as provider_down for failover.
		return true, "unknown"
	}
}

// summarizeFailover collapses N per-provider errors into a single error the
// caller can match via errors.Is. Priority: any non-NotFound error (ProviderDown,
// ExtractFailed) wins over NotFound; if every provider returned NotFound we
// return plain ErrNotFound.
//
// PRECONDITION (REVIEW.md WR-07): errs may be empty ONLY when there are zero
// providers. runFailover's loop is guaranteed to either append to errs or
// return early on a terminal error. Future maintainers adding a new terminal-
// error category to failoverDecision must preserve this invariant; otherwise
// a non-empty provider list with empty errs would silently return ErrNotFound
// instead of the real failure cause.
func summarizeFailover(errs []error) error {
	if len(errs) == 0 {
		return domain.ErrNotFound
	}
	var lastNonNotFound error
	allNotFound := true
	for _, e := range errs {
		if !errors.Is(e, domain.ErrNotFound) {
			allNotFound = false
			lastNonNotFound = e
		}
	}
	if allNotFound {
		return domain.ErrNotFound
	}
	return lastNonNotFound
}

// runFailover is the generic sequential-failover loop. T is the result type
// (e.g. []Episode, *Stream). On the first successful provider call it returns
// (result, nil). On exhaustion it returns the summarized error.
//
// Each retryable failure increments parser_fallback_total{from,to} where
// `to` is the next provider's Name() (or "" if this was the last one).
func runFailover[T any](
	ctx context.Context,
	log *logger.Logger,
	providers []domain.Provider,
	call func(p domain.Provider) (T, error),
) (T, error) {
	var zero T
	if len(providers) == 0 {
		return zero, domain.ErrNotFound
	}

	errs := make([]error, 0, len(providers))
	for i, p := range providers {
		// Context check before each attempt — fast bail on cancellation.
		if err := ctx.Err(); err != nil {
			return zero, err
		}

		result, err := call(p)
		if err == nil {
			return result, nil
		}

		retry, kind := failoverDecision(err)
		if !retry {
			// Terminal error (ctx canceled / deadline) — surface as-is.
			return zero, err
		}

		// Retryable: emit fallback metric (to = next provider name, or "" if last).
		next := ""
		if i+1 < len(providers) {
			next = providers[i+1].Name()
		}
		metrics.ParserFallbackTotal.WithLabelValues(p.Name(), next).Inc()
		if log != nil {
			log.Warnw("scraper: provider failover",
				"from", p.Name(),
				"to", next,
				"kind", kind,
				"error", err.Error(),
			)
		}
		errs = append(errs, err)
	}

	return zero, summarizeFailover(errs)
}

// ListEpisodes runs the provider chain for episode listing.
func (o *Orchestrator) ListEpisodes(ctx context.Context, providerID, prefer string) ([]domain.Episode, error) {
	return runFailover(ctx, o.log, o.orderedProviders(prefer),
		func(p domain.Provider) ([]domain.Episode, error) {
			return p.ListEpisodes(ctx, providerID)
		})
}

// ListServers runs the provider chain for server listing for one episode.
func (o *Orchestrator) ListServers(ctx context.Context, providerID, episodeID, prefer string) ([]domain.Server, error) {
	return runFailover(ctx, o.log, o.orderedProviders(prefer),
		func(p domain.Provider) ([]domain.Server, error) {
			return p.ListServers(ctx, providerID, episodeID)
		})
}

// GetStream runs the provider chain to pull a playable Stream.
func (o *Orchestrator) GetStream(ctx context.Context, providerID, episodeID, serverID string, cat domain.Category, prefer string) (*domain.Stream, error) {
	return runFailover(ctx, o.log, o.orderedProviders(prefer),
		func(p domain.Provider) (*domain.Stream, error) {
			return p.GetStream(ctx, providerID, episodeID, serverID, cat)
		})
}

// HealthSnapshot calls HealthCheck on every registered provider and returns
// a name→Health map. Phase 15 does not cache (the snapshot is cheap and the
// failure modes are well-understood); Phase 17 introduces a 60s in-memory
// cache for liveness-aware failover skipping.
//
// The returned map is always non-nil; empty for zero providers.
//
// Locking discipline (REVIEW.md CR-02): snapshot the provider slice under
// the read lock, then RELEASE the lock before invoking p.HealthCheck(ctx).
// Holding the orchestrator's RLock across provider HealthCheck calls would
// block any concurrent Register() (write lock) for the duration of every
// health check — and a future regression where a provider's HealthCheck
// does network I/O would turn into a global service stall.
func (o *Orchestrator) HealthSnapshot(ctx context.Context) map[string]domain.Health {
	o.mu.RLock()
	providers := make([]domain.Provider, len(o.providers))
	copy(providers, o.providers)
	o.mu.RUnlock()

	out := make(map[string]domain.Health, len(providers))
	for _, p := range providers {
		out[p.Name()] = p.HealthCheck(ctx)
	}
	return out
}
