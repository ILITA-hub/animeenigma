package allanimeokru

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/embeds"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

const providerName = "allanime-okru"

var stageNames = health.AllStages

// sourceLister is the discovery surface the provider needs from the internal
// discovery client (test seam).
type sourceLister interface {
	FindID(ctx context.Context, ref domain.AnimeRef) (string, error)
	ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error)
	episodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]namedSource, error)
}

// streamExtractor is the ok.ru resolver (test seam).
type streamExtractor interface {
	Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error)
}

// Deps is the constructor input for New().
type Deps struct {
	BaseURL string // forwarded to the AllAnime discovery client; empty ⇒ default
	HTTP    *domain.BaseHTTPClient
	Cache   cache.Cache
	Log     *logger.Logger
}

// Provider implements domain.Provider, serving AllAnime's ok.ru "Ok" sources.
type Provider struct {
	disc      sourceLister
	extractor streamExtractor
	log       *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs the provider: an internal AllAnime discovery client + the
// ok.ru extractor. Dependencies validated eagerly (mirrors newDiscovery).
func New(d Deps) (*Provider, error) {
	if d.HTTP == nil {
		return nil, errors.New("allanime-okru: Deps.HTTP is required")
	}
	if d.Cache == nil {
		return nil, errors.New("allanime-okru: Deps.Cache is required")
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	disc, err := newDiscovery(discoveryDeps{BaseURL: d.BaseURL, HTTP: d.HTTP, Cache: d.Cache, Log: d.Log})
	if err != nil {
		return nil, fmt.Errorf("allanime-okru: internal discovery: %w", err)
	}
	p := &Provider{
		disc:      disc,
		extractor: embeds.NewOkruExtractor(),
		log:       d.Log,
		stages:    make(map[string]domain.StageHealth, len(stageNames)),
	}
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{Up: true}
	}
	return p, nil
}

func (p *Provider) Name() string { return providerName }

func (p *Provider) markStage(stage string, err error) {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	sh := p.stages[stage]
	if err == nil {
		sh.Up, sh.LastOK, sh.LastErr = true, time.Now(), ""
	} else {
		sh.Up, sh.LastErr = false, err.Error()
	}
	p.stages[stage] = sh
}

func (p *Provider) HealthCheck(ctx context.Context) domain.Health {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	snap := make(map[string]domain.StageHealth, len(p.stages))
	for k, v := range p.stages {
		snap[k] = v
	}
	return domain.Health{Provider: providerName, Stages: snap}
}

// FindID / ListEpisodes delegate to the shared discovery.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	id, err := p.disc.FindID(ctx, ref)
	p.markStage(health.StageSearch, err)
	return id, err
}

func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	eps, err := p.disc.ListEpisodes(ctx, providerID)
	p.markStage(health.StageEpisodes, err)
	return eps, err
}

// isOk reports whether a source name is AllAnime's ok.ru family.
func isOk(name string) bool { return strings.EqualFold(strings.TrimSpace(name), "ok") }

// ListServers returns only the Ok servers across sub+dub (dub best-effort).
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	var all []domain.Server
	var firstErr error
	for _, cat := range []domain.Category{domain.CategorySub, domain.CategoryDub} {
		srcs, err := p.disc.episodeSourceURLs(ctx, episodeID, cat)
		if err != nil {
			if cat == domain.CategorySub {
				firstErr = err
			}
			continue
		}
		for _, s := range srcs {
			if !isOk(s.Name) {
				continue
			}
			id := "Ok"
			if cat != domain.CategorySub {
				id = "Ok-" + string(cat)
			}
			all = append(all, domain.Server{ID: id, Name: "OK.ru", Type: cat})
		}
	}
	if len(all) == 0 {
		err := firstErr
		if err == nil {
			err = domain.WrapNotFound(fmt.Errorf("no Ok source for %s", episodeID), "allanime-okru: ListServers")
		}
		p.markStage(health.StageServers, err)
		return nil, err
	}
	p.markStage(health.StageServers, nil)
	return all, nil
}

// GetStream resolves the first playable Ok source for the episode+category.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
	srcs, err := p.disc.episodeSourceURLs(ctx, episodeID, category)
	if err != nil {
		// Foreign-ID / not-found bubbles up as NotFound so the orchestrator skips us.
		p.markStage(health.StageStream, err)
		return nil, err
	}
	var lastErr error
	for _, s := range srcs {
		if !isOk(s.Name) {
			continue
		}
		stream, exErr := p.extractor.Extract(ctx, s.URL, nil)
		if exErr != nil {
			lastErr = exErr
			continue
		}
		if stream != nil && len(stream.Sources) > 0 {
			p.markStage(health.StageStream, nil)
			return stream, nil
		}
	}
	if lastErr != nil {
		err = domain.WrapExtractFailed(lastErr, "allanime-okru: GetStream")
	} else {
		err = domain.WrapNotFound(fmt.Errorf("no Ok source for %s", episodeID), "allanime-okru: GetStream")
	}
	p.markStage(health.StageStream, err)
	return nil, err
}

// Compile-time assertion.
var _ domain.Provider = (*Provider)(nil)
