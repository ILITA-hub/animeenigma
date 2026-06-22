package okru

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/allanime"
)

// fakeLister stands in for the internal allanime discovery (sourceLister).
type fakeLister struct {
	sources map[string][]allanime.NamedSource // keyed by episodeID
	foreign bool                              // when true, EpisodeSourceURLs returns NotFound
}

func (f *fakeLister) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
	return "SHOW", nil
}

func (f *fakeLister) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
	return nil, nil
}

func (f *fakeLister) EpisodeSourceURLs(ctx context.Context, episodeID string, category domain.Category) ([]allanime.NamedSource, error) {
	if f.foreign {
		return nil, domain.WrapNotFound(errors.New("foreign id"), "fakeLister: EpisodeSourceURLs")
	}
	return f.sources[episodeID], nil
}

// fakeExtractor stands in for the ok.ru resolver (streamExtractor).
type fakeExtractor struct {
	ok bool // when true, Extract returns a one-source Stream; otherwise an error
}

func (f *fakeExtractor) Extract(ctx context.Context, embedURL string, headers http.Header) (*domain.Stream, error) {
	if !f.ok {
		return nil, errors.New("fakeExtractor: extract failed")
	}
	return &domain.Stream{
		Sources: []domain.Source{{URL: "https://vd1.okcdn.ru/video.m3u8", Type: "hls", Quality: "auto"}},
		Headers: map[string]string{"Referer": "https://ok.ru/"},
	}, nil
}

// newTestProvider builds a *Provider directly with fakes (same package → the
// unexported fields are reachable), bypassing the network.
func newTestProvider(t *testing.T, sources map[string][]allanime.NamedSource, extractorOK bool) *Provider {
	t.Helper()
	foreign := sources == nil
	p := &Provider{
		disc:      &fakeLister{sources: sources, foreign: foreign},
		extractor: &fakeExtractor{ok: extractorOK},
		log:       logger.Default(),
		stages:    make(map[string]domain.StageHealth, len(stageNames)),
	}
	for _, s := range stageNames {
		p.stages[s] = domain.StageHealth{Up: true}
	}
	return p
}

func TestGetStream_OnlyOkSources(t *testing.T) {
	p := newTestProvider(t, map[string][]allanime.NamedSource{
		"SHOW:1": {
			{Name: "Default", URL: "https://cdn.example/clock.m3u8"}, // must be IGNORED
			{Name: "Ok", URL: "https://ok.ru/videoembed/123"},        // the only one resolved
		},
	}, /*extractorOK=*/ true)

	st, err := p.GetStream(context.Background(), "SHOW", "SHOW:1", "Ok", domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if len(st.Sources) == 0 {
		t.Fatal("no sources resolved")
	}
}

func TestGetStream_NoOkSource_NotFound(t *testing.T) {
	p := newTestProvider(t, map[string][]allanime.NamedSource{
		"SHOW:1": {{Name: "Default", URL: "https://cdn.example/clock.m3u8"}},
	}, true)
	_, err := p.GetStream(context.Background(), "SHOW", "SHOW:1", "Ok", domain.CategorySub)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetStream_ForeignID_NotFound(t *testing.T) {
	p := newTestProvider(t, nil, true)
	_, err := p.GetStream(context.Background(), "x", "foreign-no-colon", "Ok", domain.CategorySub)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
