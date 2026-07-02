package miruro

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// newBrowserProvider builds a Provider in engine=browser mode whose BrowserFetch
// closure is supplied by the test. The browser path never touches p.http, so no
// httptest server is needed. Helpers (loadFixture, encodeObfuscatedBody,
// newInMemoryCache, stubIDMapper) are shared with client_test.go (same package).
func newBrowserProvider(t *testing.T, fetch BrowserFetchFunc) *Provider {
	t.Helper()
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	p, err := New(Deps{
		BaseURL:      "https://www.miruro.tv",
		HTTP:         base,
		Cache:        cache.Cache(newInMemoryCache()),
		IDMapping:    &stubIDMapper{},
		Log:          log,
		UseBrowser:   func() bool { return true },
		BrowserFetch: fetch,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return p
}

// The whole point of the Camoufox migration: the browser transport must pick the
// x-obfuscated codec from the RESPONSE HEADER the sidecar surfaces (the body
// carries no codec marker), then decode + parse exactly like the HTTP path.
func TestBrowserTransport_DecodesXObfuscatedFromHeader(t *testing.T) {
	body := loadFixture(t, "episodes_154587.json")
	var gotURL, gotProvider string
	fetch := func(ctx context.Context, provider, url string) (int, map[string]string, []byte, error) {
		gotURL, gotProvider = url, provider
		return 200, map[string]string{"x-obfuscated": "1"}, encodeObfuscatedBody(t, body), nil
	}
	p := newBrowserProvider(t, fetch)

	eps, err := p.ListEpisodes(context.Background(), "154587")
	if err != nil {
		t.Fatalf("ListEpisodes(browser): %v", err)
	}
	if len(eps) != 28 {
		t.Fatalf("expected 28 episodes, got %d", len(eps))
	}
	if gotProvider != providerName {
		t.Errorf("browser fetch provider = %q, want %q", gotProvider, providerName)
	}
	if !strings.HasPrefix(gotURL, "https://www.miruro.tv/api/secure/pipe?e=") {
		t.Errorf("browser fetch URL = %q; expected secure-pipe on www.miruro.tv", gotURL)
	}
}

// A response with no x-obfuscated header is plain JSON (DecodeObfuscatedResponse
// returns it verbatim) — the browser path must handle the empty-header case.
func TestBrowserTransport_PlainJSONWhenNoObfHeader(t *testing.T) {
	body := loadFixture(t, "episodes_154587.json")
	fetch := func(ctx context.Context, provider, url string) (int, map[string]string, []byte, error) {
		return 200, map[string]string{}, body, nil // plain JSON, empty headers
	}
	p := newBrowserProvider(t, fetch)

	eps, err := p.ListEpisodes(context.Background(), "154587")
	if err != nil {
		t.Fatalf("ListEpisodes(browser plain): %v", err)
	}
	if len(eps) != 28 {
		t.Fatalf("expected 28 episodes, got %d", len(eps))
	}
}

// A sidecar-level failure (challenge / pool exhausted / transport) is already
// wrapped as a typed domain error by the sidecar client; the transport must pass
// it through unchanged so the orchestrator's failover classifier still works.
func TestBrowserTransport_ProviderDownPassthrough(t *testing.T) {
	fetch := func(ctx context.Context, provider, url string) (int, map[string]string, []byte, error) {
		return 0, nil, nil, domain.WrapProviderDown(errors.New("no free browser profile"), "sidecar: fetch")
	}
	p := newBrowserProvider(t, fetch)

	_, err := p.ListEpisodes(context.Background(), "154587")
	if !errors.Is(err, domain.ErrProviderDown) {
		t.Fatalf("expected ErrProviderDown passthrough, got %v", err)
	}
}

// An upstream 4xx returned through the browser (e.g. the sources endpoint's
// "anilistId is required" 400) is a parse-layer failure → ErrExtractFailed, NOT
// a retryable ProviderDown.
func TestBrowserTransport_Upstream4xxIsExtractFailed(t *testing.T) {
	fetch := func(ctx context.Context, provider, url string) (int, map[string]string, []byte, error) {
		return 400, map[string]string{}, []byte(`{"error":"anilistId is required"}`), nil
	}
	p := newBrowserProvider(t, fetch)

	_, err := p.ListEpisodes(context.Background(), "154587")
	if !errors.Is(err, domain.ErrExtractFailed) {
		t.Fatalf("expected ErrExtractFailed for upstream 400, got %v", err)
	}
}

// browserEnabled requires BOTH the gate and the closure — a partial wiring must
// degrade to the HTTP fallback (browserEnabled()==false), never panic on a nil
// closure.
func TestBrowserEnabled_RequiresGateAndClosure(t *testing.T) {
	log := logger.Default()
	base := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	mk := func(use func() bool, fetch BrowserFetchFunc) *Provider {
		p, err := New(Deps{
			BaseURL: "https://www.miruro.tv", HTTP: base, Cache: cache.Cache(newInMemoryCache()),
			IDMapping: &stubIDMapper{}, Log: log, UseBrowser: use, BrowserFetch: fetch,
		})
		if err != nil {
			t.Fatalf("New: %v", err)
		}
		return p
	}
	nonNilFetch := func(context.Context, string, string) (int, map[string]string, []byte, error) {
		return 200, nil, nil, nil
	}
	if mk(nil, nil).browserEnabled() {
		t.Error("nil gate + nil closure: browserEnabled must be false")
	}
	if mk(func() bool { return true }, nil).browserEnabled() {
		t.Error("nil closure: browserEnabled must be false")
	}
	if mk(func() bool { return false }, nonNilFetch).browserEnabled() {
		t.Error("gate false: browserEnabled must be false")
	}
	if !mk(func() bool { return true }, nonNilFetch).browserEnabled() {
		t.Error("gate true + closure set: browserEnabled must be true")
	}
}
