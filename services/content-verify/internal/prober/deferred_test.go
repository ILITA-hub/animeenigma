// Typed-503 probe handling (owner directive 2026-07-20): a 503 resolve is an
// answer ("provider down / negative-cached upstream"), not a cold start — no
// warm-up retries, no unreachable verdict, no Fails++.
package prober

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

func TestProbe503BecomesDeferredWithoutRetries(t *testing.T) {
	t.Parallel()
	hits := new(int64)
	mux := http.NewServeMux()
	mux.HandleFunc("/api/anime/a1/scraper/stream", func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt64(hits, 1)
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte(`{"success":false,"error":{"code":"PROVIDER_DOWN","message":"down","retry_after_seconds":1800},"meta":{"tried":["miruro"]}}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	cat := catalogclient.New(srv.URL, srv.URL, srv.Client())
	ffmpeg := writeFakeFFmpeg(t, t.TempDir())
	runner := &fakeRunner{lid: threeAgreeingEnFragments()}
	p := New(cat, "https://gw.example", ffmpeg, t.TempDir(), runner, nil)
	p.retryWait = 0

	u := queue.Unit{AnimeID: "a1", Provider: "miruro", EpisodeID: "ep-1",
		Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 3}
	v := p.Probe(context.Background(), u, 2)

	if v.Status != domain.StatusDeferred {
		t.Fatalf("status = %q; want deferred: %+v", v.Status, v)
	}
	if v.RetryAfter != 1800*time.Second {
		t.Errorf("RetryAfter = %s; want 30m0s", v.RetryAfter)
	}
	if v.Fails != 0 {
		t.Errorf("Fails = %d; want 0 (a down provider is not a failing episode)", v.Fails)
	}
	if got := atomic.LoadInt64(hits); got != 1 {
		t.Errorf("stream endpoint hits = %d; want 1 (typed 503 must not warm-up retry)", got)
	}
}
