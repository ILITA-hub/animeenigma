package animekai

// client.go — AnimeKai domain.Provider stub (Phase 19 — ESCAPE HATCH).
//
// Every Provider method returns an error that wraps the ErrProviderDown
// sentinel from the domain package. errors.Is(err, domain.ErrProviderDown)
// is true on every return path, so the orchestrator's failover treats this
// as a SOFT skip and lands the user on AnimePahe → Gogoanime without any
// visible regression.
//
// SCRAPER-KAI-01..04 (full implementation) are carried to v3.1. The struct
// shape (Deps, Provider, fields) matches gogoanime so the v3.1 fill-in PR
// is body-only.

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// providerName is the stable identifier returned by Name() and used as the
// orchestrator's registry key. Backend slug = "animekai"; display label is
// also "AnimeKai" (no rebrand).
const providerName = "animekai"

// stageNames is the canonical stage list used by the escape-hatch stub. It
// is an alias of health.AllStages (all five canonical stages, INCLUDING
// stream_segment) — WR-04. Earlier drafts kept a 4-stage local copy "because
// stream_segment is owned by the probe runner", but that silent divergence
// from health.AllStages is a footgun: a maintainer iterating stageNames
// would miss stream_segment — precisely the stage SCRAPER-OBS-04 alerts on.
// For the escape-hatch invariant to hold end-to-end, every stage the metric
// surface knows about must also be present in the in-memory snapshot, so
// Grafana cannot show a green panel for any animekai stage at boot.
var stageNames = health.AllStages

// errAnimeKaiStub is the canonical stub cause. Wrapping it with the
// provider-down wrapper makes errors.Is(err, domain.ErrProviderDown) AND
// errors.Is(err, errAnimeKaiStub) both return true (orchestrator failover
// semantics) while preserving a clear cause string in the log.
//
// 19-RESEARCH.md Pitfall 4: returning ErrProviderDown (not ErrExtractFailed,
// not ErrNotFound) ensures the orchestrator treats this as a SOFT skip —
// next provider in chain, no alert spam — and the probe runner flips
// provider_health_up{provider="animekai"} to 0 after 3 consecutive 501s
// from the sidecar.
var errAnimeKaiStub = errors.New("animekai: escape-hatch stub (SCRAPER-KAI-01..04 carried to v3.1)")

// malSyncClient is the malsync lookup contract — kept here for forward-compat
// with the v3.1 fill-in PR. The escape-hatch stub never calls it on the
// success path, but New() requires a non-nil value (WR-01) so a v3.1 fill-in
// PR cannot silently land a nil-pointer-deref footgun by forgetting to wire
// the real client in main.go.
type malSyncClient interface {
	Lookup(ctx context.Context, malID, provider string) (string, bool, error)
}

// noopMalSync is a sentinel malSyncClient that returns (empty, false, nil)
// for every Lookup. main.go uses it for the Phase 19 stub so Deps.MalSync
// is non-nil at boot; the v3.1 fill-in PR replaces this with a real
// NewMalSyncClient(redisCache) mirroring gogoanime.
type noopMalSync struct{}

func (noopMalSync) Lookup(ctx context.Context, malID, provider string) (string, bool, error) {
	return "", false, nil
}

// NewNoopMalSync returns a no-op malSyncClient suitable for the Phase 19
// escape-hatch stub. The stub never calls Lookup on the success path, but
// New() requires Deps.MalSync to be non-nil so a v3.1 maintainer adding
// body-only logic to FindID cannot accidentally introduce a nil-pointer
// dereference by forgetting to wire a real malsync client.
//
// When the v3.1 fill-in PR lands, replace `animekai.NewNoopMalSync()` in
// main.go with `animekai.NewMalSyncClient(redisCache)` (mirroring gogoanime).
func NewNoopMalSync() malSyncClient { //nolint:revive // exported for main.go wire-up
	return noopMalSync{}
}

// Deps is the constructor input for New(). Mirrors gogoanime.Deps so main.go
// wires the two providers with identical literal patterns. For the Phase 19
// stub, only HTTP / Embeds / Cache are validated; MalSync may be nil.
type Deps struct {
	BaseURL string
	HTTP    *domain.BaseHTTPClient
	Embeds  *domain.Registry
	MalSync malSyncClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider for the AnimeKai upstream.
type Provider struct {
	baseURL string
	http    *domain.BaseHTTPClient
	embeds  *domain.Registry
	malsync malSyncClient
	cache   cache.Cache
	log     *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs a Provider. Required dependencies (HTTP, Embeds, Cache,
// MalSync) are validated eagerly — main.go fatals on a non-nil error so
// misconfiguration surfaces at boot, not as a confusing nil-pointer 502
// minutes later.
//
// Deps.MalSync is REQUIRED (WR-01). The Phase 19 stub never calls Lookup
// on the success path, but accepting nil here would let a v3.1 fill-in PR
// that adds body-only logic to FindID land a silent nil-pointer-deref
// footgun. Use `animekai.NewNoopMalSync()` for the stub wire-up; the v3.1
// PR replaces it with `animekai.NewMalSyncClient(redisCache)`.
//
// Default BaseURL is https://anikai.to (the canonical AnimeKai mirror as of
// 2026-05-12; animekai.to 301s here).
//
// CRITICAL: stages are pre-seeded with Up=false (NOT Up=true) so Grafana
// does not show a green panel for the ~15 min before the first probe tick
// fires when the flag is on. The escape-hatch stub never recovers from this
// state — every method call markStage()s the corresponding stage as down.
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("animekai: Deps.HTTP is required")
	}
	if d.Embeds == nil {
		return nil, errors.New("animekai: Deps.Embeds is required")
	}
	if d.Cache == nil {
		return nil, errors.New("animekai: Deps.Cache is required")
	}
	if d.MalSync == nil {
		return nil, errors.New("animekai: Deps.MalSync is required (use animekai.NewNoopMalSync() for stub wire-up)")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	base := d.BaseURL
	if base == "" {
		base = "https://anikai.to"
	}
	p := &Provider{
		baseURL: strings.TrimRight(base, "/"),
		http:    d.HTTP,
		embeds:  d.Embeds,
		malsync: d.MalSync,
		cache:   d.Cache,
		log:     d.Log,
		stages:  make(map[string]domain.StageHealth, len(stageNames)),
	}
	// Escape-hatch seed: every stage is DOWN from boot. The probe runner
	// will confirm this on the first tick, but Grafana never sees a green
	// panel for the flag-on window.
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{
			Up:      false,
			LastErr: "escape-hatch stub: SCRAPER-KAI-01..04 carried to v3.1",
		}
	}
	return p, nil
}

// Name returns the stable identifier "animekai".
func (p *Provider) Name() string { return providerName }

// markStage records the success/failure of one stage. Called on every
// method exit path. Copied verbatim from gogoanime/client.go (the logic is
// provider-agnostic).
func (p *Provider) markStage(stage string, err error) {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	sh := p.stages[stage]
	if err == nil {
		sh.Up = true
		sh.LastOK = time.Now()
		sh.LastErr = ""
	} else {
		sh.Up = false
		sh.LastErr = err.Error()
	}
	p.stages[stage] = sh
}

// HealthCheck returns a snapshot of the in-memory stage health. Copied
// verbatim from gogoanime/client.go.
func (p *Provider) HealthCheck(ctx context.Context) domain.Health {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	snap := make(map[string]domain.StageHealth, len(p.stages))
	for k, v := range p.stages {
		snap[k] = v
	}
	return domain.Health{Provider: providerName, Stages: snap}
}

// FindID — STUB. Returns wrapped ErrProviderDown. The orchestrator treats
// this as a soft skip and falls through to the next provider.
// SCRAPER-KAI-01 is carried to v3.1.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: FindID not implemented")
	p.markStage(health.StageSearch, err)
	return "", err
}

// ListEpisodes — STUB. SCRAPER-KAI-02 is carried to v3.1.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: ListEpisodes not implemented")
	p.markStage(health.StageEpisodes, err)
	return nil, err
}

// ListServers — STUB. SCRAPER-KAI-03 is carried to v3.1.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: ListServers not implemented")
	p.markStage(health.StageServers, err)
	return nil, err
}

// GetStream — STUB. SCRAPER-KAI-04 (sidecar /animekai-token wiring) is
// carried to v3.1; the sidecar route exists but returns HTTP 501.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: GetStream not implemented")
	p.markStage(health.StageStream, err)
	return nil, err
}

// Compile-time assertion: Provider satisfies domain.Provider. Failing this
// assertion is a build error — the strongest possible interface-conformance
// test.
var _ domain.Provider = (*Provider)(nil)
