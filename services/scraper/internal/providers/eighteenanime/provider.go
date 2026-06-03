package eighteenanime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)

// providerName is the stable identifier returned by Name(); it is also the
// scraper-providers.yaml key and the Prometheus `provider` label value.
const providerName = "18anime"

// Deps is the constructor input for New(). All fields optional — sensible
// defaults are applied (real 18anime.me base, 8s HTTP client, default logger).
type Deps struct {
	HTTP       *http.Client
	Log        *logger.Logger
	BaseURL    string // override for tests (search + page fetches); defaults to baseURL
	SearchBase string // override for tests (search only); defaults to BaseURL
}

// Provider implements domain.Provider for 18anime.me.
type Provider struct {
	http       *http.Client
	base       string
	searchBase string
	log        *logger.Logger

	stagesMu sync.Mutex
	stages   map[string]domain.StageHealth
}

// New constructs the 18anime provider.
func New(d Deps) *Provider {
	if d.HTTP == nil {
		d.HTTP = &http.Client{Timeout: 8 * time.Second}
	}
	if d.Log == nil {
		d.Log = logger.Default()
	}
	if d.BaseURL == "" {
		d.BaseURL = baseURL
	}
	if d.SearchBase == "" {
		d.SearchBase = d.BaseURL
	}
	return &Provider{
		http:       d.HTTP,
		base:       strings.TrimRight(d.BaseURL, "/"),
		searchBase: strings.TrimRight(d.SearchBase, "/"),
		log:        d.Log,
		stages:     make(map[string]domain.StageHealth, len(health.AllStages)),
	}
}

func (p *Provider) Name() string { return providerName }

// --- HTTP plumbing -----------------------------------------------------------

func (p *Provider) fetch(ctx context.Context, u, referer string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	if referer != "" {
		req.Header.Set("Referer", referer)
	}
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("eighteenanime: GET %s -> %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	return string(b), err
}

// searchHTML POSTs the DataLife Engine search form (18anime.me is title-searched).
func (p *Provider) searchHTML(ctx context.Context, query string) (string, error) {
	form := url.Values{"do": {"search"}, "subaction": {"search"}, "story": {query}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		p.searchBase+"/index.php?do=search", strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := p.http.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("eighteenanime: search -> %d", resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}

// --- domain.Provider ---------------------------------------------------------

// FindID searches 18anime by the catalog title (and any alt titles) and returns
// the matched series' base slug as the provider ID.
func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	titles := make([]string, 0, 1+len(ref.AltTitles))
	if ref.Title != "" {
		titles = append(titles, ref.Title)
	}
	titles = append(titles, ref.AltTitles...)

	var lastErr error
	for _, t := range titles {
		page, err := p.searchHTML(ctx, t)
		if err != nil {
			lastErr = err
			continue
		}
		if hit := bestMatch(t, parseSearchResults(page)); hit != nil {
			base, _ := baseSlugAndEpisode(hit.Slug)
			p.markStage(health.StageSearch, nil)
			return base, nil
		}
	}
	p.markStage(health.StageSearch, orNotFound(lastErr))
	if lastErr != nil {
		return "", lastErr
	}
	return "", domain.ErrNotFound
}

// ListEpisodes re-searches by the base slug (18anime has no series page) and
// returns every episode whose slug shares the exact base, sorted ascending.
func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	query := strings.ReplaceAll(providerID, "-", " ")
	page, err := p.searchHTML(ctx, query)
	if err != nil {
		p.markStage(health.StageEpisodes, err)
		return nil, err
	}
	var eps []domain.Episode
	for _, h := range parseSearchResults(page) {
		base, num := baseSlugAndEpisode(h.Slug)
		if base != providerID {
			continue
		}
		eps = append(eps, domain.Episode{ID: h.Slug, Number: num, Title: fmt.Sprintf("Episode %d", num)})
	}
	if len(eps) == 0 {
		p.markStage(health.StageEpisodes, domain.ErrNotFound)
		return nil, domain.ErrNotFound
	}
	sort.Slice(eps, func(i, j int) bool { return eps[i].Number < eps[j].Number })
	p.markStage(health.StageEpisodes, nil)
	return eps, nil
}

// ListServers fetches the episode page and exposes each supported mirror
// (mp4upload, turbovid) as a server in failover order.
func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
	page, err := p.fetch(ctx, EpisodeURL(episodeID), p.base+"/")
	if err != nil {
		p.markStage(health.StageServers, err)
		return nil, err
	}
	var servers []domain.Server
	for _, m := range supportedMirrors(parseEpisodeMirrors(page)) {
		id := serverIDFor(m.Link)
		servers = append(servers, domain.Server{ID: id, Name: id, Type: domain.CategorySub})
	}
	if len(servers) == 0 {
		p.markStage(health.StageServers, domain.ErrNotFound)
		return nil, domain.ErrNotFound
	}
	p.markStage(health.StageServers, nil)
	return servers, nil
}

// GetStream resolves a playable source for an episode. An empty serverID runs
// the mp4upload->turbovid failover; a non-empty serverID pins that mirror.
func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, _ domain.Category) (*domain.Stream, error) {
	page, err := p.fetch(ctx, EpisodeURL(episodeID), p.base+"/")
	if err != nil {
		p.markStage(health.StageStream, err)
		return nil, err
	}
	src, err := p.resolveStream(ctx, parseEpisodeMirrors(page), serverID)
	if err != nil {
		p.markStage(health.StageStream, err)
		return nil, err
	}
	srcType := "mp4"
	if src.IsHLS {
		srcType = "hls"
	}
	stream := &domain.Stream{
		Sources: []domain.Source{{URL: src.URL, Type: srcType, Quality: src.Quality}},
	}
	if src.Referer != "" {
		stream.Headers = map[string]string{"Referer": src.Referer}
	}
	p.markStage(health.StageStream, nil)
	return stream, nil
}

// resolveStream tries the requested server, or all supported mirrors in
// failover order when serverID is empty; first successful extraction wins.
func (p *Provider) resolveStream(ctx context.Context, mirrors []Mirror, serverID string) (*ExtractedSource, error) {
	supported := supportedMirrors(mirrors)
	if serverID != "" {
		filtered := supported[:0:0]
		for _, m := range supported {
			if serverIDFor(m.Link) == serverID {
				filtered = append(filtered, m)
			}
		}
		supported = filtered
	}
	if len(supported) == 0 {
		return nil, fmt.Errorf("eighteenanime: no supported mirrors")
	}
	var lastErr error
	for _, m := range supported {
		ex := extractorFor(m.Link)
		if ex == nil {
			continue
		}
		embedPage, err := p.fetch(ctx, m.Link, p.base+"/")
		if err != nil {
			lastErr = err
			continue
		}
		src, err := ex(embedPage)
		if err != nil {
			lastErr = err
			continue
		}
		return src, nil
	}
	if lastErr != nil {
		return nil, fmt.Errorf("eighteenanime: all mirrors failed: %w", lastErr)
	}
	return nil, fmt.Errorf("eighteenanime: no supported mirrors")
}

func (p *Provider) HealthCheck(_ context.Context) domain.Health {
	p.stagesMu.Lock()
	defer p.stagesMu.Unlock()
	snap := make(map[string]domain.StageHealth, len(p.stages))
	for k, v := range p.stages {
		snap[k] = v
	}
	return domain.Health{Provider: providerName, Stages: snap}
}

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

func orNotFound(err error) error {
	if err == nil {
		return domain.ErrNotFound
	}
	return err
}
