package allanimeokru

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// --- helpers --------------------------------------------------------------

// inMemoryCache is a tiny cache.Cache impl backed by a map. Tests do not
// need TTL semantics; we ignore the ttl argument entirely.
type inMemoryCache struct {
	m map[string][]byte
}

func newInMemoryCache() *inMemoryCache {
	return &inMemoryCache{m: make(map[string][]byte)}
}

func (c *inMemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
	v, ok := c.m[key]
	if !ok {
		return errors.New("miss")
	}
	return json.Unmarshal(v, dest)
}

func (c *inMemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	b, err := json.Marshal(value)
	if err != nil {
		return err
	}
	c.m[key] = b
	return nil
}

func (c *inMemoryCache) Delete(ctx context.Context, keys ...string) error {
	for _, k := range keys {
		delete(c.m, k)
	}
	return nil
}

func (c *inMemoryCache) GetDel(ctx context.Context, key string, dest interface{}) error {
	v, ok := c.m[key]
	if !ok {
		return errors.New("miss")
	}
	delete(c.m, key)
	return json.Unmarshal(v, dest)
}

func (c *inMemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := c.m[key]
	return ok, nil
}

func (c *inMemoryCache) Close() error                                         { return nil }
func (c *inMemoryCache) Invalidate(ctx context.Context, pattern string) error { return nil }
func (c *inMemoryCache) GetOrSet(ctx context.Context, key string, dest interface{}, ttl time.Duration, fn func() (interface{}, error)) error {
	if err := c.Get(ctx, key, dest); err == nil {
		return nil
	}
	val, err := fn()
	if err != nil {
		return err
	}
	if err := c.Set(ctx, key, val, ttl); err != nil {
		return err
	}
	return c.Get(ctx, key, dest)
}
func (c *inMemoryCache) SetNX(ctx context.Context, key string, value interface{}, ttl time.Duration) (bool, error) {
	if _, ok := c.m[key]; ok {
		return false, nil
	}
	return true, c.Set(ctx, key, value, ttl)
}

// newTestDiscovery builds a *discovery talking to httpSrv. Compresses retry
// backoff so unit tests don't sit waiting.
func newTestDiscovery(t *testing.T, httpSrv *httptest.Server) *discovery {
	t.Helper()
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log,
		domain.WithRetryWaitMin(1*time.Millisecond),
		domain.WithRetryWaitMax(5*time.Millisecond),
		domain.WithMaxRetries(0),
	)
	c := cache.Cache(newInMemoryCache())
	d, err := newDiscovery(discoveryDeps{
		BaseURL: httpSrv.URL,
		HTTP:    base,
		Cache:   c,
		Log:     log,
	})
	if err != nil {
		t.Fatalf("newDiscovery: %v", err)
	}
	return d
}

// --- newDiscovery() validation ---------------------------------------------

func TestNewDiscovery_RequiresHTTP(t *testing.T) {
	_, err := newDiscovery(discoveryDeps{Cache: newInMemoryCache()})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("expected HTTP required error, got %v", err)
	}
}

func TestNewDiscovery_RequiresCache(t *testing.T) {
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log)
	_, err := newDiscovery(discoveryDeps{HTTP: base})
	if err == nil || !strings.Contains(err.Error(), "Cache") {
		t.Fatalf("expected Cache required error, got %v", err)
	}
}

// --- FindID --------------------------------------------------------------

func TestFindID_Frieren(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.RawQuery, "variables=") {
			t.Errorf("expected variables query param")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"shows":{"edges":[{"_id":"frieren_show_id","name":"Frieren","englishName":"Frieren: Beyond Journey's End","nativeName":"葬送のフリーレン","thumbnail":"x","availableEpisodes":{"sub":28}}]}}}`))
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)
	id, err := d.FindID(context.Background(), domain.AnimeRef{
		ShikimoriID: "52991",
		Title:       "Frieren",
	})
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if id != "frieren_show_id" {
		t.Fatalf("expected show ID frieren_show_id, got %q", id)
	}
}

func TestFindID_NotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"shows":{"edges":[]}}}`))
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)
	_, err := d.FindID(context.Background(), domain.AnimeRef{
		Title: "Nonexistent Anime XYZ",
	})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestFindID_EmptyTitle(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should NOT be hit on empty title")
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)
	_, err := d.FindID(context.Background(), domain.AnimeRef{})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for empty title, got %v", err)
	}
}

// --- ListEpisodes --------------------------------------------------------

func TestListEpisodes_Frieren(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"show":{"_id":"frieren_show_id","availableEpisodesDetail":{"sub":["1","2","3"],"dub":[],"raw":[]}}}}`))
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)
	eps, err := d.ListEpisodes(context.Background(), "frieren_show_id")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 3 {
		t.Fatalf("expected 3 episodes, got %d", len(eps))
	}
	if eps[0].Number != 1 || eps[0].ID != "frieren_show_id:1" {
		t.Fatalf("ep[0] = %+v", eps[0])
	}
}

func TestListEpisodes_RealEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"show":{"_id":"x","availableEpisodesDetail":{"sub":[]}}}}`))
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)
	eps, err := d.ListEpisodes(context.Background(), "x")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("expected 0 episodes (real empty), got %d", len(eps))
	}
}

// --- doGraphQL transport semantics ---------------------------------------

func TestDoGraphQL_5xxIsProviderDown(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("upstream timeout"))
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)
	_, err := d.FindID(context.Background(), domain.AnimeRef{Title: "Anything"})
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("expected ErrProviderDown for 502, got %v", err)
	}
}

func TestDoGraphQL_4xxIsExtractFailed(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"errors":["PersistedQueryNotFound"]}`))
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)
	_, err := d.FindID(context.Background(), domain.AnimeRef{Title: "Anything"})
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed for 400, got %v", err)
	}
}

func TestSplitEpisodeID(t *testing.T) {
	cases := []struct {
		in      string
		showID  string
		episode string
	}{
		{"abc:1", "abc", "1"},
		{"frieren_show_id:28", "frieren_show_id", "28"},
		{"invalid_no_colon", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		got1, got2 := splitEpisodeID(tc.in)
		if got1 != tc.showID || got2 != tc.episode {
			t.Errorf("splitEpisodeID(%q) = (%q, %q), want (%q, %q)", tc.in, got1, got2, tc.showID, tc.episode)
		}
	}
}

func TestDecodeSourceURL_Passthrough(t *testing.T) {
	in := "https://cdn.example.com/abc.m3u8"
	if out := decodeSourceURL(in); out != in {
		t.Fatalf("expected passthrough, got %q", out)
	}
}

// --- episodeSourceURLs (consumed by the provider half, client.go) ---------

func TestEpisodeSourceURLs_FiltersAndDecodes(t *testing.T) {
	// Upstream returns one plain "Ok" source + one "--"-encoded clock source.
	// episodeSourceURLs must return BOTH, decoded, with names intact.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// minimal SourceUrls response: an Ok ok.ru embed + a Default clock source
		fmt.Fprint(w, `{"data":{"episode":{"episodeString":"1","sourceUrls":[`+
			`{"sourceUrl":"https://ok.ru/videoembed/123","sourceName":"Ok","type":"iframe"},`+
			`{"sourceUrl":"https://cdn.example/x.m3u8","sourceName":"Default","type":"iframe"}`+
			`]}}}`)
	}))
	defer srv.Close()

	d := newTestDiscovery(t, srv)

	got, err := d.episodeSourceURLs(context.Background(), "SHOW:1", domain.CategorySub)
	if err != nil {
		t.Fatalf("episodeSourceURLs: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("want 2 sources, got %d: %+v", len(got), got)
	}
	if got[0].Name != "Ok" || got[0].URL != "https://ok.ru/videoembed/123" {
		t.Errorf("source[0] = %+v", got[0])
	}
}

func TestEpisodeSourceURLs_ForeignID_NotFound(t *testing.T) {
	log := logger.Default()
	d, err := newDiscovery(discoveryDeps{HTTP: domain.NewBaseHTTPClient(log), Cache: newInMemoryCache(), Log: log})
	if err != nil {
		t.Fatal(err)
	}
	_, err = d.episodeSourceURLs(context.Background(), "gogoanime-slug-no-colon", domain.CategorySub)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
