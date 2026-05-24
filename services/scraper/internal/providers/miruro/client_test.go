package miruro

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/idmapping"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// Compile-time interface assertion. Failing this is a BUILD ERROR — the
// strongest possible test that Provider satisfies domain.Provider.
var _ domain.Provider = (*Provider)(nil)

// --- helpers --------------------------------------------------------------

// inMemoryCache is a tiny cache.Cache impl backed by a map. Mirrors the
// allanime test helper.
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
func (c *inMemoryCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := c.m[key]
	return ok, nil
}
func (c *inMemoryCache) Close() error                                       { return nil }
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

// stubIDMapper is a programmable IDMapper for FindID tests.
type stubIDMapper struct {
	resolve func(id string) (*idmapping.MappingResult, error)
}

func (s *stubIDMapper) ResolveByShikimoriID(id string) (*idmapping.MappingResult, error) {
	if s.resolve == nil {
		return nil, nil
	}
	return s.resolve(id)
}

// loadFixture reads a fixture from testdata/ and returns its bytes.
func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name)
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return b
}

// encodeObfuscatedBody packs `body` the way Miruro's upstream packs
// responses with x-obfuscated=1: gzip → base64url(no padding).
func encodeObfuscatedBody(t *testing.T, body []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(body); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return []byte(base64.RawURLEncoding.EncodeToString(buf.Bytes()))
}

// newTestProvider builds a Provider whose HTTP client points at httpSrv
// and which uses the supplied IDMapper stub. The Provider's baseURL is
// set to httpSrv.URL so BuildSecurePipeURL routes there.
func newTestProvider(t *testing.T, httpSrv *httptest.Server, idMap IDMapper) *Provider {
	t.Helper()
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log,
		domain.WithRetryWaitMin(1*time.Millisecond),
		domain.WithRetryWaitMax(5*time.Millisecond),
		domain.WithMaxRetries(0),
	)
	c := cache.Cache(newInMemoryCache())
	p, err := New(Deps{
		BaseURL:   httpSrv.URL,
		ProxyURL:  httpSrv.URL,
		HTTP:      base,
		Cache:     c,
		IDMapping: idMap,
		Log:       log,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// --- New() validation -----------------------------------------------------

func TestNew_RequiresHTTP(t *testing.T) {
	_, err := New(Deps{Cache: newInMemoryCache(), IDMapping: &stubIDMapper{}})
	if err == nil || !strings.Contains(err.Error(), "HTTP") {
		t.Fatalf("expected HTTP required error, got %v", err)
	}
}

func TestNew_RequiresCache(t *testing.T) {
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log)
	_, err := New(Deps{HTTP: base, IDMapping: &stubIDMapper{}})
	if err == nil || !strings.Contains(err.Error(), "Cache") {
		t.Fatalf("expected Cache required error, got %v", err)
	}
}

func TestNew_RequiresIDMapping(t *testing.T) {
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log)
	_, err := New(Deps{HTTP: base, Cache: newInMemoryCache()})
	if err == nil || !strings.Contains(err.Error(), "IDMapping") {
		t.Fatalf("expected IDMapping required error, got %v", err)
	}
}

func TestNew_Name(t *testing.T) {
	log := logger.Default()
	p, err := New(Deps{
		HTTP:      domain.NewBaseHTTPClient(log),
		Cache:     newInMemoryCache(),
		IDMapping: &stubIDMapper{},
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if p.Name() != "miruro" {
		t.Fatalf("expected Name() = miruro, got %s", p.Name())
	}
}

// --- FindID ---------------------------------------------------------------

// TestFindID_ARMResolves verifies the canonical happy path: ARM maps a
// Shikimori/MAL ID to an AniList ID, and FindID returns the AniList ID
// as a decimal string.
func TestFindID_ARMResolves(t *testing.T) {
	anilistID := 154587
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("upstream should NOT be hit for FindID — ARM is the only network call")
	}))
	defer srv.Close()

	idMap := &stubIDMapper{
		resolve: func(id string) (*idmapping.MappingResult, error) {
			if id != "52991" {
				t.Errorf("ARM called with id=%q, want 52991", id)
			}
			anilistVal := anilistID
			return &idmapping.MappingResult{AniList: &anilistVal}, nil
		},
	}
	p := newTestProvider(t, srv, idMap)
	got, err := p.FindID(context.Background(), domain.AnimeRef{ShikimoriID: "52991"})
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if got != "154587" {
		t.Fatalf("FindID = %q, want 154587", got)
	}
}

// TestFindID_EmptyShikimori_NoAniList verifies that an empty MAL ID
// with no AniList ID returns ErrNotFound — fuzzy title search is not
// supported.
func TestFindID_EmptyShikimori_NoAniList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should NOT be hit on empty refs")
	}))
	defer srv.Close()

	p := newTestProvider(t, srv, &stubIDMapper{})
	_, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Some Anime"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// TestFindID_AniListIDFastPath: caller passing AniListID directly skips
// the ARM call (zero network requests).
func TestFindID_AniListIDFastPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should NOT be hit when AniListID is supplied")
	}))
	defer srv.Close()

	armCalls := 0
	idMap := &stubIDMapper{
		resolve: func(id string) (*idmapping.MappingResult, error) {
			armCalls++
			return nil, nil
		},
	}
	p := newTestProvider(t, srv, idMap)
	got, err := p.FindID(context.Background(), domain.AnimeRef{AniListID: "154587"})
	if err != nil {
		t.Fatalf("FindID: %v", err)
	}
	if got != "154587" {
		t.Fatalf("FindID = %q, want 154587", got)
	}
	if armCalls != 0 {
		t.Errorf("ARM was called %d times; expected 0 on AniListID fast path", armCalls)
	}
}

// TestFindID_NoMapping: ARM returns no AniList ID → ErrNotFound.
func TestFindID_NoMapping(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	idMap := &stubIDMapper{
		resolve: func(id string) (*idmapping.MappingResult, error) {
			return &idmapping.MappingResult{}, nil // AniList=nil
		},
	}
	p := newTestProvider(t, srv, idMap)
	_, err := p.FindID(context.Background(), domain.AnimeRef{ShikimoriID: "52991"})
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound for nil AniList field, got %v", err)
	}
}

// TestFindID_ARMError: ARM lookup failure wrapped as ProviderDown.
func TestFindID_ARMError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()

	idMap := &stubIDMapper{
		resolve: func(id string) (*idmapping.MappingResult, error) {
			return nil, errors.New("ARM 500")
		},
	}
	p := newTestProvider(t, srv, idMap)
	_, err := p.FindID(context.Background(), domain.AnimeRef{ShikimoriID: "52991"})
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("expected ErrProviderDown for ARM error, got %v", err)
	}
}

// --- ListEpisodes ---------------------------------------------------------

// pipeHandler is the standard httptest handler that emulates Miruro's
// secure-pipe endpoint. It dispatches based on the decoded request
// descriptor's `path` field and serves the named fixture bytes (gzip
// + base64url encoded with x-obfuscated=1).
func pipeHandler(t *testing.T, routes map[string][]byte) http.HandlerFunc {
	t.Helper()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the `e=` query param.
		raw := r.URL.Query().Get("e")
		if raw == "" {
			t.Errorf("missing e= query param")
			http.Error(w, "missing e", 400)
			return
		}
		pad := len(raw) % 4
		if pad != 0 {
			raw += strings.Repeat("=", 4-pad)
		}
		raw = strings.ReplaceAll(raw, "-", "+")
		raw = strings.ReplaceAll(raw, "_", "/")
		desc, err := base64.StdEncoding.DecodeString(raw)
		if err != nil {
			t.Errorf("decode e=: %v", err)
			http.Error(w, "bad e", 400)
			return
		}
		var d struct {
			Path string `json:"path"`
		}
		if err := json.Unmarshal(desc, &d); err != nil {
			t.Errorf("unmarshal descriptor: %v", err)
			http.Error(w, "bad json", 400)
			return
		}
		// Match by exact path or prefix.
		var body []byte
		for k, v := range routes {
			if d.Path == k || strings.HasPrefix(d.Path, k+"/") || d.Path == k+"/" {
				body = v
				break
			}
		}
		if body == nil {
			t.Errorf("no route for path=%q", d.Path)
			http.Error(w, "no route", 404)
			return
		}
		w.Header().Set("x-obfuscated", "1")
		w.Header().Set("Content-Type", "application/octet-stream")
		_, _ = w.Write(encodeObfuscatedBody(t, body))
	})
}

// TestListEpisodes_Frieren confirms that the 4-inner-provider fixture
// surfaces as 28 unique sub episodes via the preferred-provider (kiwi)
// pick.
func TestListEpisodes_Frieren(t *testing.T) {
	srv := httptest.NewServer(pipeHandler(t, map[string][]byte{
		"episodes": loadFixture(t, "episodes_154587.json"),
	}))
	defer srv.Close()

	p := newTestProvider(t, srv, &stubIDMapper{})
	eps, err := p.ListEpisodes(context.Background(), "154587")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 28 {
		t.Fatalf("expected 28 episodes, got %d", len(eps))
	}
	if eps[0].Number != 1 {
		t.Errorf("ep[0].Number = %d, want 1", eps[0].Number)
	}
	if eps[27].Number != 28 {
		t.Errorf("ep[27].Number = %d, want 28", eps[27].Number)
	}
	// Preferred provider = kiwi (first in defaultEpisodePreference).
	if !strings.HasPrefix(eps[0].ID, "kiwi_") {
		t.Errorf("ep[0].ID = %q; expected kiwi_ prefix from preferred-provider pick", eps[0].ID)
	}
}

// TestListEpisodes_RealEmpty: when no provider block has a sub track,
// return ([]Episode{}, nil) — not an error.
func TestListEpisodes_RealEmpty(t *testing.T) {
	emptyBody, _ := json.Marshal(episodesResponse{
		Providers: map[string]providerEpisodeBlock{
			"dune": {Episodes: map[string][]rawEpisode{}},
		},
	})
	srv := httptest.NewServer(pipeHandler(t, map[string][]byte{
		"episodes": emptyBody,
	}))
	defer srv.Close()

	p := newTestProvider(t, srv, &stubIDMapper{})
	eps, err := p.ListEpisodes(context.Background(), "154587")
	if err != nil {
		t.Fatalf("ListEpisodes: %v", err)
	}
	if len(eps) != 0 {
		t.Fatalf("expected 0 episodes (real empty), got %d", len(eps))
	}
}

// TestListEpisodes_FractionalEpisodeNumber — ISS-015 regression-lock.
// One Piece's `1004.5` recap special was rejected by the previous
// `Number int` DTO with `cannot unmarshal number 1004.5 into Go struct
// field rawEpisode.providers.episodes.number of type int`. The new
// episodeNumber type (float64-backed) MUST accept the fractional
// upstream value and surface it truncated to int (1004.5 → 1004)
// without breaking the parse of the surrounding episodes.
func TestListEpisodes_FractionalEpisodeNumber(t *testing.T) {
	// Hand-craft a synthetic episodes response containing the kiwi
	// preferred-provider block with one int episode and one fractional
	// recap special — mimicking the shape that broke One Piece.
	body := []byte(`{
		"providers": {
			"kiwi": {
				"episodes": {
					"sub": [
						{"id":"k_1","number":1004,"title":"Real Episode","audio":"sub","filler":false,"airDate":"2026-01-01","duration":1440},
						{"id":"k_1_5","number":1004.5,"title":"Recap Special","audio":"sub","filler":true,"airDate":"2026-01-08","duration":600},
						{"id":"k_2","number":1005,"title":"Next Real Episode","audio":"sub","filler":false,"airDate":"2026-01-15","duration":1440}
					]
				}
			}
		}
	}`)
	srv := httptest.NewServer(pipeHandler(t, map[string][]byte{"episodes": body}))
	defer srv.Close()

	p := newTestProvider(t, srv, &stubIDMapper{})
	eps, err := p.ListEpisodes(context.Background(), "21")
	if err != nil {
		t.Fatalf("ListEpisodes: %v (must not reject parse on fractional number — ISS-015)", err)
	}
	if len(eps) != 3 {
		t.Fatalf("expected 3 episodes (real + recap + next-real), got %d", len(eps))
	}
	// Verify the fractional episode is preserved at the truncated int.
	var seenIDs []string
	for _, e := range eps {
		seenIDs = append(seenIDs, e.ID)
	}
	if eps[0].Number != 1004 || eps[0].ID != "k_1" {
		t.Errorf("eps[0] = {%d,%q}; want {1004,\"k_1\"}", eps[0].Number, eps[0].ID)
	}
	if eps[1].Number != 1004 || eps[1].ID != "k_1_5" {
		t.Errorf("eps[1] = {%d,%q}; want {1004,\"k_1_5\"} (recap special truncated to 1004)",
			eps[1].Number, eps[1].ID)
	}
	if eps[2].Number != 1005 || eps[2].ID != "k_2" {
		t.Errorf("eps[2] = {%d,%q}; want {1005,\"k_2\"}", eps[2].Number, eps[2].ID)
	}
}

// TestEpisodeNumber_UnmarshalAcceptsBothShapes — direct unit test for
// the JSON-flexible numeric field that backs rawEpisode.Number.
func TestEpisodeNumber_UnmarshalAcceptsBothShapes(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"0", 0},
		{"1", 1},
		{"1004", 1004},
		{"1.0", 1},
		{"1004.5", 1004},
		{"0.7", 0},
		{"-1.2", -1},
	}
	for _, tc := range cases {
		var e episodeNumber
		if err := e.UnmarshalJSON([]byte(tc.in)); err != nil {
			t.Errorf("UnmarshalJSON(%q): unexpected error %v", tc.in, err)
			continue
		}
		if got := e.Int(); got != tc.want {
			t.Errorf("UnmarshalJSON(%q).Int() = %d; want %d", tc.in, got, tc.want)
		}
	}
	// Non-number input must error (preserves the rest of the strict-
	// schema contract).
	var bad episodeNumber
	if err := bad.UnmarshalJSON([]byte(`"not-a-number"`)); err == nil {
		t.Error("UnmarshalJSON of string literal must error; got nil")
	}
}

// TestListEpisodes_EmptyAniListID: empty providerID → ExtractFailed.
func TestListEpisodes_EmptyAniListID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("upstream should NOT be hit on empty AniList ID")
	}))
	defer srv.Close()

	p := newTestProvider(t, srv, &stubIDMapper{})
	_, err := p.ListEpisodes(context.Background(), "")
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed, got %v", err)
	}
}

// TestListEpisodes_Upstream5xx: 5xx → ProviderDown.
func TestListEpisodes_Upstream5xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "upstream broken", 502)
	}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})
	_, err := p.ListEpisodes(context.Background(), "154587")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("expected ErrProviderDown for 5xx, got %v", err)
	}
}

// --- ListServers ----------------------------------------------------------

// TestListServers_Frieren_Ep1: episode ID picked from the fixture; we
// should see all 4 inner providers' sub blocks plus the 3 dub blocks
// (kiwi/hop/bee).
func TestListServers_Frieren_Ep1(t *testing.T) {
	srv := httptest.NewServer(pipeHandler(t, map[string][]byte{
		"episodes": loadFixture(t, "episodes_154587.json"),
	}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})
	servers, err := p.ListServers(context.Background(), "154587", "kiwi_ep1_id")
	if err != nil {
		t.Fatalf("ListServers: %v", err)
	}
	if len(servers) == 0 {
		t.Fatalf("expected at least 1 server, got 0")
	}
	// First server should be the preferred (kiwi) sub track.
	if servers[0].Name != "kiwi" {
		t.Errorf("servers[0].Name = %q; want kiwi (preferred-provider order)", servers[0].Name)
	}
	if servers[0].Type != domain.CategorySub {
		t.Errorf("servers[0].Type = %q; want sub", servers[0].Type)
	}
}

// TestListServers_UnknownEpisode: episode ID matches no inner provider
// → ErrNotFound (orchestrator can fall through).
func TestListServers_UnknownEpisode(t *testing.T) {
	srv := httptest.NewServer(pipeHandler(t, map[string][]byte{
		"episodes": loadFixture(t, "episodes_154587.json"),
	}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})
	_, err := p.ListServers(context.Background(), "154587", "nonexistent_id")
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

// --- GetStream ------------------------------------------------------------

// TestGetStream_HappyPath: sources endpoint returns 2 quality variants;
// we should pick the 1080p one.
func TestGetStream_HappyPath(t *testing.T) {
	srv := httptest.NewServer(pipeHandler(t, map[string][]byte{
		"sources": loadFixture(t, "sources_154587_ep1.json"),
	}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})
	stream, err := p.GetStream(context.Background(), "154587", "kiwi_ep1_id", "kiwi", domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if len(stream.Sources) == 0 {
		t.Fatalf("expected ≥1 source")
	}
	s := stream.Sources[0]
	if !strings.HasPrefix(s.URL, "https://") {
		t.Errorf("source URL = %q; expected https://", s.URL)
	}
	if s.Type != "hls" {
		t.Errorf("source Type = %q; expected hls", s.Type)
	}
	if s.Quality != "1080p" {
		t.Errorf("source Quality = %q; expected 1080p (best of 1080p + 720p)", s.Quality)
	}
	if stream.Headers["Referer"] == "" {
		t.Errorf("expected Referer header from upstream stream entry")
	}
}

// TestGetStream_EmptyStreams: streams[] empty → ExtractFailed.
func TestGetStream_EmptyStreams(t *testing.T) {
	emptyBody, _ := json.Marshal(sourcesResponse{Streams: []rawStream{}})
	srv := httptest.NewServer(pipeHandler(t, map[string][]byte{
		"sources": emptyBody,
	}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})
	_, err := p.GetStream(context.Background(), "154587", "ep1_id", "kiwi", domain.CategorySub)
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed for empty streams, got %v", err)
	}
}

// TestGetStream_DefaultsToPreferredServer: empty serverID → kiwi.
func TestGetStream_DefaultsToPreferredServer(t *testing.T) {
	got := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Decode the descriptor to extract `query.provider`.
		raw := r.URL.Query().Get("e")
		pad := len(raw) % 4
		if pad != 0 {
			raw += strings.Repeat("=", 4-pad)
		}
		raw = strings.ReplaceAll(raw, "-", "+")
		raw = strings.ReplaceAll(raw, "_", "/")
		desc, _ := base64.StdEncoding.DecodeString(raw)
		var d struct {
			Path  string `json:"path"`
			Query map[string]any `json:"query"`
		}
		_ = json.Unmarshal(desc, &d)
		if d.Path == "sources" {
			if p, ok := d.Query["provider"].(string); ok {
				got = p
			}
		}
		w.Header().Set("x-obfuscated", "1")
		_, _ = w.Write(encodeObfuscatedBody(t, loadFixture(t, "sources_154587_ep1.json")))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})
	_, err := p.GetStream(context.Background(), "154587", "ep1_id", "", domain.CategorySub)
	if err != nil {
		t.Fatalf("GetStream: %v", err)
	}
	if got != "kiwi" {
		t.Errorf("default server = %q; want kiwi (preferred default)", got)
	}
}

// --- HealthCheck + markStage ---------------------------------------------

func TestMarkStage_SuccessAndFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})

	// Initial seeded state: all stages Up=true.
	snap := p.HealthCheck(context.Background())
	for stage, s := range snap.Stages {
		if !s.Up {
			t.Errorf("stage %s seeded Up=false; want true", stage)
		}
	}

	// Failure flips Up=false and records LastErr.
	p.markStage("episodes", errors.New("boom"))
	snap = p.HealthCheck(context.Background())
	if snap.Stages["episodes"].Up {
		t.Errorf("episodes stage Up=true after error; want false")
	}
	if snap.Stages["episodes"].LastErr != "boom" {
		t.Errorf("LastErr = %q; want boom", snap.Stages["episodes"].LastErr)
	}

	// Success restores Up=true and clears LastErr.
	p.markStage("episodes", nil)
	snap = p.HealthCheck(context.Background())
	if !snap.Stages["episodes"].Up {
		t.Errorf("episodes Up=false after success; want true")
	}
	if snap.Stages["episodes"].LastErr != "" {
		t.Errorf("LastErr = %q; want empty after success", snap.Stages["episodes"].LastErr)
	}
}

// --- Cache hit short-circuits --------------------------------------------

// TestListEpisodes_CachedHitShortCircuits: a populated cache short-circuits
// the upstream call. Asserted by counting upstream hits.
func TestListEpisodes_CachedHitShortCircuits(t *testing.T) {
	upstreamHits := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamHits++
		w.Header().Set("x-obfuscated", "1")
		_, _ = w.Write(encodeObfuscatedBody(t, loadFixture(t, "episodes_154587.json")))
	}))
	defer srv.Close()
	p := newTestProvider(t, srv, &stubIDMapper{})

	if _, err := p.ListEpisodes(context.Background(), "154587"); err != nil {
		t.Fatalf("first ListEpisodes: %v", err)
	}
	if _, err := p.ListEpisodes(context.Background(), "154587"); err != nil {
		t.Fatalf("second ListEpisodes (should be cached): %v", err)
	}
	if upstreamHits != 1 {
		t.Errorf("upstream hit %d times; expected 1 (second call should hit cache)", upstreamHits)
	}
}
