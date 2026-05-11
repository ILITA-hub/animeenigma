package domain

import (
	"context"
	"errors"
	"net/http"
	"sync"
)

// ErrNoMatchingExtractor is returned by Registry.Find when no registered
// EmbedExtractor matches the given embed URL. Callers should treat this as
// "tried all known extractors, give up" — typically wrapping it with
// WrapExtractFailed and surfacing to the orchestrator.
var ErrNoMatchingExtractor = errors.New("scraper: no matching embed extractor")

// EmbedExtractor pulls a *Stream out of a third-party embed URL (e.g.
// megacloud, kwik, streamtape). Provider scrapers call into the Registry
// to find the right extractor for the embed URLs their upstream returns.
//
// Adding a new embed family in Phase 16+ is one struct implementing this
// interface plus one Register call at service startup.
type EmbedExtractor interface {
	// Name is a stable identifier used in logs and observability labels
	// (e.g. "megacloud", "kwik"). MUST be lowercase, no spaces.
	Name() string

	// Matches reports whether this extractor knows how to handle the given
	// embed URL. Implementations typically do a substring or regex check
	// on the URL host or path. MUST be side-effect free.
	Matches(embedURL string) bool

	// Extract fetches the embed page (with optional request headers, e.g.
	// Referer) and decrypts / parses out the playable Stream DTO.
	// Returns ErrExtractFailed (wrapped) on parse / decrypt failure.
	Extract(ctx context.Context, embedURL string, headers http.Header) (*Stream, error)
}

// Registry holds an ordered list of EmbedExtractors. Find iterates in
// registration order and returns the first whose Matches reports true.
//
// The zero value is NOT usable; construct via NewRegistry.
type Registry struct {
	mu         sync.RWMutex
	extractors []EmbedExtractor
}

// NewRegistry allocates an empty Registry ready to accept Register calls.
func NewRegistry() *Registry {
	return &Registry{extractors: make([]EmbedExtractor, 0, 4)}
}

// Register appends an extractor to the registry. Registration order is
// preserved and affects Find: the first-registered extractor whose Matches
// reports true wins (see TestRegistry_FindReturnsFirstMatch).
func (r *Registry) Register(e EmbedExtractor) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.extractors = append(r.extractors, e)
}

// Find returns the first registered extractor whose Matches(embedURL) reports
// true, or (nil, ErrNoMatchingExtractor) if none match.
func (r *Registry) Find(embedURL string) (EmbedExtractor, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, e := range r.extractors {
		if e.Matches(embedURL) {
			return e, nil
		}
	}
	return nil, ErrNoMatchingExtractor
}

// Names returns a copy of registered extractor names, in registration order.
// The returned slice is always non-nil (empty slice for empty registry) so
// callers can JSON-marshal it without a nil check.
func (r *Registry) Names() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.extractors))
	for i, e := range r.extractors {
		out[i] = e.Name()
	}
	return out
}
