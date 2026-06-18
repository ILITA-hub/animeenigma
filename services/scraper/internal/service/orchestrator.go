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
	"fmt"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// Orchestrator runs business methods against a registered list of providers
// with sequential failover. It also owns the embed extractor registry so the
// HTTP handler layer has a single dependency (the orchestrator) for every
// scraper concern.
//
// Construct via NewOrchestrator. Zero value is not usable.
//
// Phase 17 (SCRAPER-OBS-03): the optional `cache` field is the in-memory
// provider-health cache. When non-nil, runFailover consults it before
// dispatching to a provider — a cache reading DOWN causes the orchestrator
// to skip the provider and increment parser_fallback_total. nil cache
// preserves Phase 16 behaviour (no skipping).
type Orchestrator struct {
	mu        sync.RWMutex
	providers []domain.Provider
	// degraded holds the names of soft-degraded providers (AUTO-484): registered
	// (so an explicit `prefer` can still reach them) but EXCLUDED from the natural
	// auto-failover order — they are never auto-fallen-back to.
	degraded  map[string]bool
	registry  *domain.Registry
	log       *logger.Logger
	cache     *health.InMemoryHealthCache

	// providerTimeout bounds how long the failover loop waits on a SINGLE
	// provider before moving on. Zero disables the cap (preserving the original
	// behaviour for tests constructed via NewOrchestrator without a setter).
	// Set via SetProviderTimeout from config (SCRAPER_PROVIDER_TIMEOUT). ISS-022.
	providerTimeout time.Duration
}

// NewOrchestrator builds an orchestrator with zero providers. Use Register
// to add providers (Phase 16+).
//
// Phase 17: `cache` is the in-memory provider-health cache (Plan 17-01).
// Pass nil to disable skip-unhealthy behaviour — existing tests use nil so
// they exercise Phase 16's loop verbatim.
func NewOrchestrator(log *logger.Logger, registry *domain.Registry, cache *health.InMemoryHealthCache) *Orchestrator {
	if registry == nil {
		registry = domain.NewRegistry()
	}
	return &Orchestrator{
		providers: make([]domain.Provider, 0, 4),
		degraded:  make(map[string]bool),
		registry:  registry,
		log:       log,
		cache:     cache,
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

// RegisterDegraded registers a soft-degraded provider (AUTO-484): it is added to
// the provider set so an explicit `prefer` (hacker-mode pin) can still reach it,
// but it is EXCLUDED from the natural auto-failover order — the failover chain
// never auto-falls-back to it.
func (o *Orchestrator) RegisterDegraded(p domain.Provider) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.providers = append(o.providers, p)
	if o.degraded == nil {
		o.degraded = make(map[string]bool)
	}
	o.degraded[p.Name()] = true
}

// SetProviderTimeout sets the per-provider failover budget (ISS-022). When
// d > 0, each provider call in the failover chain runs under a sub-context
// deadline of d; a provider that blows its budget is treated as down and the
// loop fails over to the next provider (instead of letting one hung provider
// consume the whole request budget). d <= 0 disables the cap. Safe to call
// once at startup before serving traffic.
func (o *Orchestrator) SetProviderTimeout(d time.Duration) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.providerTimeout = d
}

// providerBudget returns the current per-provider timeout under the read lock.
func (o *Orchestrator) providerBudget() time.Duration {
	o.mu.RLock()
	defer o.mu.RUnlock()
	return o.providerTimeout
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
				out = append(out, p) // an explicit prefer reaches degraded providers too
				break
			}
		}
	}
	for i, p := range o.providers {
		if i == preferredIdx {
			continue // already inserted at position 0
		}
		// Soft-degraded providers (AUTO-484) are excluded from the auto-failover
		// order; they are reachable ONLY as the explicit prefer (handled above).
		if o.degraded[p.Name()] {
			continue
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
//
// Phase 17 (SCRAPER-OBS-03): when `cache` is non-nil, each provider is
// gated by cache.IsHealthy(name) BEFORE the call() dispatch. A cached DOWN
// state causes the orchestrator to:
//   - skip the call entirely (no upstream request)
//   - emit parser_fallback_total{from, to=next}  — same metric the failure
//     path uses, so dashboards see the skip as a fallback event
//   - append a "skipped: health gauge 0" error to errs so summarizeFailover
//     can return ErrProviderDown if every provider is skipped
//
// The cache is fail-open (missing or stale entries return true), so a probe
// outage does NOT blank the service — only an actively-reported DOWN does.
func runFailover[T any](
	ctx context.Context,
	log *logger.Logger,
	providers []domain.Provider,
	cache *health.InMemoryHealthCache,
	providerTimeout time.Duration,
	operation string,
	call func(ctx context.Context, p domain.Provider) (T, error),
) (T, error) {
	v, _, err := runFailoverNamed(ctx, log, providers, cache, providerTimeout, operation, call)
	return v, err
}

// providerCall runs fn under a per-provider deadline when budget > 0, or under
// the parent ctx directly when budget <= 0 (preserving the original uncapped
// behaviour). The sub-context is always cancelled before returning. ISS-022.
func providerCall[T any](ctx context.Context, budget time.Duration, fn func(context.Context) (T, error)) (T, error) {
	if budget <= 0 {
		return fn(ctx)
	}
	pctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	return fn(pctx)
}

// classifyProviderErr decides whether the failover loop should advance after a
// provider returned a non-nil err, accounting for the per-provider budget.
//
//   - retry == false → stop the loop and surface terminalErr (never nil here).
//   - retry == true  → advance to the next provider (kind labels the reason).
//
// ISS-022 subtlety: a per-provider budget timeout surfaces as a bare
// context.DeadlineExceeded, which failoverDecision would (correctly, in the
// uncapped world) call terminal. But when the PARENT ctx is still alive, that
// deadline is the per-provider cap firing — NOT caller cancellation — so we
// re-classify it as a retryable "provider_timeout" and fail over. Only a
// genuinely cancelled/expired PARENT ctx is terminal.
func classifyProviderErr(parent context.Context, err error) (retry bool, kind string, terminalErr error) {
	if parent.Err() != nil {
		return false, "", parent.Err()
	}
	retry, kind = failoverDecision(err)
	if !retry && (errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)) {
		return true, "provider_timeout", nil
	}
	return retry, kind, err
}

// runFailoverNamed is runFailover that also returns the name of the provider
// whose call succeeded. The winner name lets the HTTP handler surface
// `meta.provider` so the client can pin subsequent calls (servers/stream) to
// the SAME provider — essential because provider episode/server IDs are opaque
// and only resolve on the provider that produced them. On failure the returned
// name is "".
func runFailoverNamed[T any](
	ctx context.Context,
	log *logger.Logger,
	providers []domain.Provider,
	cache *health.InMemoryHealthCache,
	providerTimeout time.Duration,
	operation string,
	call func(ctx context.Context, p domain.Provider) (T, error),
) (T, string, error) {
	var zero T
	if len(providers) == 0 {
		return zero, "", domain.ErrNotFound
	}

	errs := make([]error, 0, len(providers))
	for i, p := range providers {
		// Context check before each attempt — fast bail on cancellation.
		if err := ctx.Err(); err != nil {
			return zero, "", err
		}

		// SCRAPER-OBS-03: skip providers flagged DOWN by the in-memory health
		// cache. Cache is fail-open (missing/stale entries return true), so
		// this only skips when the probe confirmed at least 3 consecutive
		// failures within the last 15 min on the stream_segment stage.
		if cache != nil && !cache.IsHealthy(p.Name()) {
			next := ""
			if i+1 < len(providers) {
				next = providers[i+1].Name()
			}
			metrics.ParserFallbackTotal.WithLabelValues(p.Name(), next).Inc()
			// Wrap with ErrProviderDown so summarizeFailover() picks it up as
			// a non-NotFound failure (otherwise an all-skipped chain would
			// degrade to ErrNotFound, which would be a wrong signal — the
			// providers exist; they're just gated unhealthy).
			errs = append(errs, fmt.Errorf("provider %s skipped: health gauge 0: %w", p.Name(), domain.ErrProviderDown))
			if log != nil {
				log.Debugw("scraper: provider skipped (health cache says down)",
					"from", p.Name(), "to", next)
			}
			continue
		}

		result, err := func() (res T, e error) {
			defer metrics.ObserveParser(p.Name(), operation, time.Now(), &e)
			return providerCall(ctx, providerTimeout, func(c context.Context) (T, error) {
				return call(c, p)
			})
		}()
		if err == nil {
			return result, p.Name(), nil
		}

		retry, kind, terminalErr := classifyProviderErr(ctx, err)
		if !retry {
			// Terminal: parent ctx canceled/expired — surface that error.
			return zero, "", terminalErr
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

	return zero, "", summarizeFailover(errs)
}

// OrderedProviderNames returns the names of registered providers in the
// failover order the orchestrator would use for a `prefer` argument:
// preferred name first if it matches a registered provider, then the
// rest in registration order. Unknown prefer values are silently ignored.
//
// Phase 16 plan 05 (SCRAPER-NF-05 backend half): exposed publicly so the
// HTTP handler can render `meta.tried` on every response — both success
// and error — without poking the orchestrator's internal lock again.
// Returns an empty (non-nil) slice when no providers are registered so
// the handler can encode `"tried":[]` unconditionally.
func (o *Orchestrator) OrderedProviderNames(prefer string) []string {
	ps := o.orderedProviders(prefer)
	if len(ps) == 0 {
		return []string{}
	}
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = p.Name()
	}
	return out
}

// FindID runs the provider chain for AnimeRef → provider-internal ID
// resolution. The returned ID is the value to pass to ListEpisodes /
// ListServers / GetStream for the provider that succeeded. Failover
// semantics match the business methods (ErrNotFound is retryable; ctx
// errors are terminal).
//
// Phase 16 plan 05: the scraper HTTP handler calls this before the
// per-method business calls so the catalog can pass `mal_id` and the
// orchestrator resolves it through the registered provider chain.
func (o *Orchestrator) FindID(ctx context.Context, ref domain.AnimeRef, prefer string) (string, error) {
	id, _, err := o.FindIDNamed(ctx, ref, prefer)
	return id, err
}

// FindIDNamed is FindID that also returns the name of the provider that
// resolved the ID. The returned providerID is opaque and provider-specific,
// so the caller MUST pin the subsequent ListEpisodes/ListServers/GetStream
// stage to this provider (pass the winner as `prefer`). Otherwise the next
// stage's failover chain restarts at the head of the order and hands the
// foreign ID to the wrong provider — which may return an empty-but-no-error
// result that short-circuits failover and yields a bogus "0 episodes" success
// attributed to the wrong provider (the "English sources show 0 episodes for
// titles only allanime/miruro can resolve" bug, e.g. "91 Days": gogoanime's
// search misses, so allanime wins FindID, but gogoanime.ListEpisodes returns
// ([],nil) for the allanime ID).
func (o *Orchestrator) FindIDNamed(ctx context.Context, ref domain.AnimeRef, prefer string) (string, string, error) {
	return runFailoverNamed(ctx, o.log, o.orderedProviders(prefer), o.cache, o.providerBudget(), "find_id",
		func(c context.Context, p domain.Provider) (string, error) {
			return p.FindID(c, ref)
		})
}

// ListEpisodes runs the provider chain for episode listing.
func (o *Orchestrator) ListEpisodes(ctx context.Context, providerID, prefer string) ([]domain.Episode, error) {
	eps, _, err := o.ListEpisodesNamed(ctx, providerID, prefer)
	return eps, err
}

// ListEpisodesNamed is ListEpisodes that also returns the name of the provider
// that produced the episode list. The episode IDs in the result are opaque and
// provider-specific, so the caller must pin subsequent servers/stream calls to
// this same provider (surfaced to the client via meta.provider) — otherwise a
// different provider re-resolved from the failover order would receive episode
// IDs it cannot parse.
func (o *Orchestrator) ListEpisodesNamed(ctx context.Context, providerID, prefer string) ([]domain.Episode, string, error) {
	return runFailoverNamed(ctx, o.log, o.orderedProviders(prefer), o.cache, o.providerBudget(), "list_episodes",
		func(c context.Context, p domain.Provider) ([]domain.Episode, error) {
			return p.ListEpisodes(c, providerID)
		})
}

// ListServers runs the provider chain for server listing for one episode.
func (o *Orchestrator) ListServers(ctx context.Context, providerID, episodeID, prefer string) ([]domain.Server, error) {
	return runFailover(ctx, o.log, o.orderedProviders(prefer), o.cache, o.providerBudget(), "list_servers",
		func(c context.Context, p domain.Provider) ([]domain.Server, error) {
			return p.ListServers(c, providerID, episodeID)
		})
}

// GetStream runs the provider chain to pull a playable Stream.
func (o *Orchestrator) GetStream(ctx context.Context, providerID, episodeID, serverID string, cat domain.Category, prefer string) (*domain.Stream, error) {
	return runFailover(ctx, o.log, o.orderedProviders(prefer), o.cache, o.providerBudget(), "get_stream",
		func(c context.Context, p domain.Provider) (*domain.Stream, error) {
			return p.GetStream(c, providerID, episodeID, serverID, cat)
		})
}

// gatedProvider is the optional interface a provider implements when it
// can run a playability gate (Phase 21 SCRAPER-HEAL-04). Only
// gogoanime.Provider satisfies this today; animepahe + animekai do not
// (they're treated as gated=false fallback by GetStreamGated).
type gatedProvider interface {
	domain.Provider
	GetStreamWithGate(
		ctx context.Context,
		providerID, episodeID, serverID string,
		category domain.Category,
		servers []domain.Server,
	) (*domain.Stream, bool, error)
}

// GetStreamGated runs the failover chain and returns a Stream plus a gated
// bool indicating whether the playability gate ran on this call.
//
// Per-provider semantics:
//   - Providers implementing gatedProvider (gogoanime today): call
//     ListServers + GetStreamWithGate; surface the gated bool the provider
//     returned (true on cold path, false on warm-cache hit / caller pin).
//   - Plain providers (animepahe, animekai): fall back to GetStream with
//     gated=false. Phase 21 only wires the gate into gogoanime; the other
//     providers stay unchanged.
//
// Failover semantics mirror Orchestrator.GetStream — health-cache-DOWN
// providers are skipped with a parser_fallback_total emit; ErrProviderDown
// / ErrExtractFailed / ErrNotFound are all retryable; ctx errors terminal.
//
// SCRAPER-HEAL-04.
func (o *Orchestrator) GetStreamGated(
	ctx context.Context,
	providerID, episodeID, serverID string,
	cat domain.Category, prefer string,
) (*domain.Stream, bool, error) {
	providers := o.orderedProviders(prefer)
	if len(providers) == 0 {
		return nil, false, domain.ErrNotFound
	}

	budget := o.providerBudget()
	errs := make([]error, 0, len(providers))
	for i, p := range providers {
		if err := ctx.Err(); err != nil {
			return nil, false, err
		}

		// Health-cache gate (matches runFailover semantics so the failover
		// metric stays consistent between gated and non-gated paths).
		if o.cache != nil && !o.cache.IsHealthy(p.Name()) {
			next := ""
			if i+1 < len(providers) {
				next = providers[i+1].Name()
			}
			metrics.ParserFallbackTotal.WithLabelValues(p.Name(), next).Inc()
			errs = append(errs, fmt.Errorf("provider %s skipped: health gauge 0: %w", p.Name(), domain.ErrProviderDown))
			continue
		}

		// Per-provider budget (ISS-022): cap each provider so one hung provider
		// cannot starve the chain past the caller's timeout. A budget timeout
		// while the parent ctx is alive is reclassified as a failover, not a
		// terminal error, by classifyProviderErr below.
		stream, gated, err := o.attemptGatedStream(ctx, budget, p, providerID, episodeID, serverID, cat)
		if err == nil {
			return stream, gated, nil
		}

		retry, _, terminalErr := classifyProviderErr(ctx, err)
		if !retry {
			return nil, false, terminalErr
		}
		next := ""
		if i+1 < len(providers) {
			next = providers[i+1].Name()
		}
		metrics.ParserFallbackTotal.WithLabelValues(p.Name(), next).Inc()
		errs = append(errs, err)
	}

	return nil, false, summarizeFailover(errs)
}

// attemptGatedStream runs ONE provider's gated (or plain) stream resolution
// under an optional per-provider deadline (budget > 0). Gated providers
// (gogoanime today) list servers then run GetStreamWithGate; plain providers
// fall back to GetStream with gated=false. The per-provider sub-context is
// always cancelled before returning. ISS-022.
func (o *Orchestrator) attemptGatedStream(
	ctx context.Context, budget time.Duration, p domain.Provider,
	providerID, episodeID, serverID string, cat domain.Category,
) (*domain.Stream, bool, error) {
	pctx := ctx
	if budget > 0 {
		var cancel context.CancelFunc
		pctx, cancel = context.WithTimeout(ctx, budget)
		defer cancel()
	}

	if gp, ok := p.(gatedProvider); ok {
		// Gated path: list servers first so the provider can iterate
		// priority + probe internally.
		servers, err := gp.ListServers(pctx, providerID, episodeID)
		if err != nil {
			return nil, false, err
		}
		return gp.GetStreamWithGate(pctx, providerID, episodeID, serverID, cat, servers)
	}

	// Non-gated provider fallback (animepahe / animekai).
	stream, err := p.GetStream(pctx, providerID, episodeID, serverID, cat)
	return stream, false, err
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

// RegisteredProviders returns a snapshot of currently-registered providers.
// Locking discipline (REVIEW.md CR-02): snapshot under RLock and release
// before any caller-side iteration that may do I/O. Phase 17 plans 02 and 03
// consume this — the probe runner uses it to spawn one goroutine per
// provider, and the admin handler uses it to enumerate names without
// touching the orchestrator's internal lock from a request handler.
func (o *Orchestrator) RegisteredProviders() []domain.Provider {
	o.mu.RLock()
	defer o.mu.RUnlock()
	out := make([]domain.Provider, len(o.providers))
	copy(out, o.providers)
	return out
}
