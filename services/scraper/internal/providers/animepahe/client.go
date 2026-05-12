package animepahe

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// malSyncClient is the malsync lookup contract — abstracted so tests can
// inject a fake.
type malSyncClient interface {
	Lookup(ctx context.Context, malID, provider string) (string, bool, error)
}

// Deps is the constructor input for New(). All fields must be non-nil
// except Log (will default to a no-op).
type Deps struct {
	// BaseURL is the AnimePahe base URL (default https://animepahe.ru, see
	// CONTEXT.md). Plan 16-05 wires this from ANIMEPAHE_BASE_URL env var.
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Embeds  *domain.Registry
	MalSync malSyncClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider for the AnimePahe upstream.
//
// RED stub — see Task 2 GREEN commit for the real implementation.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	embeds  *domain.Registry
	malsync malSyncClient
	cache   cache.Cache
	log     *logger.Logger
}

// New constructs a Provider. RED stub.
func New(d Deps) *Provider {
	return &Provider{
		baseURL: d.BaseURL,
		http:    d.HTTP,
		embeds:  d.Embeds,
		malsync: d.MalSync,
		cache:   d.Cache,
		log:     d.Log,
	}
}

// Name returns "animepahe". RED stub.
func (p *Provider) Name() string { panic("not implemented (RED)") }

// FindID resolves an AnimeRef to an AnimePahe anime ID. RED stub.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	panic("not implemented (RED)")
}

// ListEpisodes returns all episodes for the given AnimePahe anime ID. RED stub.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	panic("not implemented (RED)")
}

// ListServers returns the kwik.cx servers for one episode. RED stub.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	panic("not implemented (RED)")
}

// GetStream returns the playable Stream for one episode + server. RED stub.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	panic("not implemented (RED)")
}

// HealthCheck returns the in-memory health snapshot. RED stub.
func (p *Provider) HealthCheck(ctx context.Context) domain.Health {
	panic("not implemented (RED)")
}

// Compile-time assertion: *Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)
