package domain

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/tracing"
)

// TestProviderTagContext — ProviderContext round-trips through both the
// scraper-domain read path (ProviderFromContext) AND tracing's read path
// (tracing.ProviderFromContext, which the recording transport uses). Absent
// the tag, both return "" (general egress carries no provider, D-01).
func TestProviderTagContext(t *testing.T) {
	t.Parallel()

	ctx := ProviderContext(context.Background(), "miruro")

	if got := ProviderFromContext(ctx); got != "miruro" {
		t.Errorf("ProviderFromContext = %q; want %q", got, "miruro")
	}
	// The recording transport reads tracing's private provider value — assert
	// the tag actually reaches it (target = provider + host depends on this).
	if got := tracing.ProviderFromContext(ctx); got != "miruro" {
		t.Errorf("tracing.ProviderFromContext = %q; want %q (recorder would miss the provider)", got, "miruro")
	}

	// Absent tag → empty on both paths.
	if got := ProviderFromContext(context.Background()); got != "" {
		t.Errorf("ProviderFromContext(empty) = %q; want \"\"", got)
	}
	if got := tracing.ProviderFromContext(context.Background()); got != "" {
		t.Errorf("tracing.ProviderFromContext(empty) = %q; want \"\"", got)
	}

	// Empty provider is a no-op (does not stash an empty value).
	if got := ProviderFromContext(ProviderContext(context.Background(), "")); got != "" {
		t.Errorf("ProviderContext(\"\") should be a no-op, got %q", got)
	}
}

// providerStubTransport records intercepted hosts + the provider tag it sees on
// each request context (the exact thing the production recording transport
// reads). Stands in for tracing.WrapRecording.
type providerStubTransport struct {
	inner     http.RoundTripper
	hosts     []string
	providers []string
}

func (s *providerStubTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	s.hosts = append(s.hosts, req.URL.Host)
	s.providers = append(s.providers, tracing.ProviderFromContext(req.Context()))
	return s.inner.RoundTrip(req)
}

// TestBaseHTTPClientTransport — a BaseHTTPClient built WithTransport(stub) +
// WithProvider routes outbound through the stub (the production recording seam)
// and the provider tag rides the request context so the recorder can read it.
func TestBaseHTTPClientTransport(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	stub := &providerStubTransport{inner: http.DefaultTransport}
	c := NewBaseHTTPClient(testLogger(t),
		WithTransport(stub),
		WithProvider("gogoanime"),
	)

	resp, err := c.Get(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("Get error: %v", err)
	}
	_ = resp.Body.Close()

	if len(stub.hosts) == 0 {
		t.Fatalf("injected transport never invoked — egress recording would never fire")
	}
	wantHost := strings.TrimPrefix(srv.URL, "http://")
	if stub.hosts[0] != wantHost {
		t.Errorf("recorder saw host %q, want %q", stub.hosts[0], wantHost)
	}
	if stub.providers[0] != "gogoanime" {
		t.Errorf("recorder saw provider %q, want %q (D-02/D-09 target=provider+host)", stub.providers[0], "gogoanime")
	}
}
