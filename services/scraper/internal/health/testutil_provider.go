package health

// FakeProvider lives in a non-test file so multiple test packages
// (services/scraper/internal/service, services/scraper/internal/handler,
// future probe tests) can share it without a `_test.go` import cycle.
//
// It is package-public but is only intended for test use; production callers
// should construct real providers via services/scraper/internal/providers/*.

import (
	"context"
	"sync/atomic"

	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// FakeProvider is a programmable domain.Provider for cache/probe/orchestrator
// tests. Each method-fn field is OPTIONAL — nil returns the zero value + nil
// error. Counters expose call counts for assertions.
//
// Pattern copied from the original services/scraper/internal/service/orchestrator_test.go
// fakeProvider and promoted to a non-test file so cross-package reuse works.
type FakeProvider struct {
	NameVal        string
	FindIDFn       func(ctx context.Context, ref domain.AnimeRef) (string, error)
	ListEpisodesFn func(ctx context.Context, providerID string) ([]domain.Episode, error)
	ListServersFn  func(ctx context.Context, providerID, episodeID string) ([]domain.Server, error)
	GetStreamFn    func(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error)
	HealthCheckFn  func(ctx context.Context) domain.Health

	findIDCalls       int32
	listEpisodesCalls int32
	listServersCalls  int32
	getStreamCalls    int32
}

// Compile-time interface assertion. Breaking this aborts the build, not just
// the tests.
var _ domain.Provider = (*FakeProvider)(nil)

// Name returns the configured NameVal verbatim.
func (f *FakeProvider) Name() string { return f.NameVal }

// FindID delegates to FindIDFn if set; otherwise returns zero values.
func (f *FakeProvider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	atomic.AddInt32(&f.findIDCalls, 1)
	if f.FindIDFn != nil {
		return f.FindIDFn(ctx, ref)
	}
	return "", nil
}

// ListEpisodes delegates to ListEpisodesFn if set; otherwise returns zero values.
func (f *FakeProvider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	atomic.AddInt32(&f.listEpisodesCalls, 1)
	if f.ListEpisodesFn != nil {
		return f.ListEpisodesFn(ctx, providerID)
	}
	return nil, nil
}

// ListServers delegates to ListServersFn if set; otherwise returns zero values.
func (f *FakeProvider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	atomic.AddInt32(&f.listServersCalls, 1)
	if f.ListServersFn != nil {
		return f.ListServersFn(ctx, providerID, episodeID)
	}
	return nil, nil
}

// GetStream delegates to GetStreamFn if set; otherwise returns zero values.
func (f *FakeProvider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	atomic.AddInt32(&f.getStreamCalls, 1)
	if f.GetStreamFn != nil {
		return f.GetStreamFn(ctx, providerID, episodeID, serverID, category)
	}
	return nil, nil
}

// HealthCheck delegates to HealthCheckFn if set; otherwise returns the
// default Health{Provider: NameVal}.
func (f *FakeProvider) HealthCheck(ctx context.Context) domain.Health {
	if f.HealthCheckFn != nil {
		return f.HealthCheckFn(ctx)
	}
	return domain.Health{Provider: f.NameVal}
}

// FindIDCalls returns the number of times FindID was invoked.
func (f *FakeProvider) FindIDCalls() int32 { return atomic.LoadInt32(&f.findIDCalls) }

// ListEpisodesCalls returns the number of times ListEpisodes was invoked.
func (f *FakeProvider) ListEpisodesCalls() int32 { return atomic.LoadInt32(&f.listEpisodesCalls) }

// ListServersCalls returns the number of times ListServers was invoked.
func (f *FakeProvider) ListServersCalls() int32 { return atomic.LoadInt32(&f.listServersCalls) }

// GetStreamCalls returns the number of times GetStream was invoked.
func (f *FakeProvider) GetStreamCalls() int32 { return atomic.LoadInt32(&f.getStreamCalls) }
