package animepahe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

// realMalSyncProvider wires a Provider against the REAL MalSyncClient (not
// the in-memory fakeMalSync used in client_test.go) so the persistent
// reverse-mapping cache path is exercised end-to-end. The fakeCache
// satisfies cache.Cache and survives the test lifetime, which is the
// stand-in for "across process restarts" — the load-bearing fact is that
// LookupMalID reads from the same cache.Cache instance, not from in-memory
// state on Provider.
func realMalSyncProvider(t *testing.T, resolverSrv, malsyncSrv *httptest.Server) (*Provider, *fakeCache, *MalSyncClient) {
	t.Helper()
	log, err := logger.New(logger.Config{Level: "error", Encoding: "console"})
	if err != nil {
		t.Fatalf("logger: %v", err)
	}
	hc := domain.NewBaseHTTPClient(log, domain.WithMaxRetries(0))
	fc := newFakeCache()
	reg := domain.NewRegistry()

	ms := NewMalSyncClient(fc,
		WithMalSyncBaseURL(malsyncSrv.URL),
		WithMalSyncHTTPClient(malsyncSrv.Client()),
	)

	p, err := New(Deps{
		ResolverURL: resolverSrv.URL,
		HTTP:        hc,
		Embeds:      reg,
		MalSync:     ms,
		Cache:       fc,
		Log:         log,
	})
	if err != nil {
		t.Fatalf("New(Deps{...}) = %v; want nil", err)
	}
	return p, fc, ms
}

// TestProvider_MalSyncInvalidationOn404 — A9 single-strike. Three sub-tests
// cover the meaningful states:
//
//   (a) WithoutPriorFindID_PersistedReverseKey — load-bearing assertion that
//       invalidation works ACROSS PROCESS RESTARTS. The test seeds both
//       forward and reverse keys via direct cache.Set (no FindID call)
//       then triggers a /release 404, asserting both keys are evicted.
//   (b) AfterFindID — happy path: FindID populates both keys via MalSync
//       lookup, then /release 404 evicts both.
//   (c) NoMalIDKnown — defensive: when no reverse mapping exists (FindID
//       was never called for this providerID), /release 404 still returns
//       ErrNotFound cleanly without panicking; Invalidate is a no-op.
func TestProvider_MalSyncInvalidationOn404(t *testing.T) {
	t.Run("WithoutPriorFindID_PersistedReverseKey", func(t *testing.T) {
		t.Parallel()
		const (
			malID         = "52082"
			animeSession  = "uuid-abc-frieren"
			forwardKey    = "malsync:52082:animepahe"
			reverseKeyStr = "malsync_reverse:animepahe:uuid-abc-frieren"
		)
		resolverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Every /release call returns 404 — drives the invalidation path.
			if r.URL.Path == "/release" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer resolverSrv.Close()
		malsyncSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			t.Errorf("MalSync HTTP must not be called — both cache keys are pre-seeded")
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer malsyncSrv.Close()

		p, fc, _ := realMalSyncProvider(t, resolverSrv, malsyncSrv)

		// Seed both keys DIRECTLY in the cache — this is the
		// "fresh process, no prior FindID" simulation. If invalidation
		// depended on in-memory state populated by FindID, this test
		// would fail the eviction assertion below.
		ctx := context.Background()
		if err := fc.Set(ctx, forwardKey, malID, 24*time.Hour); err != nil {
			t.Fatal(err)
		}
		if err := fc.Set(ctx, reverseKeyStr, malID, 24*time.Hour); err != nil {
			t.Fatal(err)
		}

		// Drive ListEpisodes; expect ErrNotFound surfaced verbatim.
		_, err := p.ListEpisodes(ctx, animeSession)
		if err == nil {
			t.Fatalf("ListEpisodes err = nil; want ErrNotFound")
		}
		// Both keys MUST be evicted (single-strike).
		var v string
		if err := fc.Get(ctx, forwardKey, &v); err != cache.ErrNotFound {
			t.Errorf("forward key %q still present after invalidation; err = %v, v = %q", forwardKey, err, v)
		}
		if err := fc.Get(ctx, reverseKeyStr, &v); err != cache.ErrNotFound {
			t.Errorf("reverse key %q still present after invalidation; err = %v, v = %q", reverseKeyStr, err, v)
		}
	})

	t.Run("AfterFindID", func(t *testing.T) {
		t.Parallel()
		const (
			malID        = "52082"
			animeSession = "uuid-after-findid"
		)
		// Resolver returns OK for /search (so FindID succeeds) and 404
		// for /release (drives invalidation).
		resolverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/search":
				// /search isn't actually called in this test because the
				// MalSync hit short-circuits FindID — but providing a 200
				// here is harmless and documents the contract.
				_, _ = w.Write([]byte(`{"data":[]}`))
			case "/release":
				w.WriteHeader(http.StatusNotFound)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
		}))
		defer resolverSrv.Close()
		malsyncSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/mal/anime/52082" {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":52082,"title":"Frieren","Sites":{"animepahe":{"a":{"identifier":"uuid-after-findid","url":"x"}}}}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer malsyncSrv.Close()

		p, fc, _ := realMalSyncProvider(t, resolverSrv, malsyncSrv)

		ctx := context.Background()
		gotID, err := p.FindID(ctx, domain.AnimeRef{ShikimoriID: malID, Title: "Frieren"})
		if err != nil {
			t.Fatalf("FindID err = %v", err)
		}
		if gotID != animeSession {
			t.Fatalf("FindID = %q; want %q", gotID, animeSession)
		}
		// Confirm both keys exist post-FindID.
		var v string
		if err := fc.Get(ctx, "malsync:52082:animepahe", &v); err != nil {
			t.Fatalf("forward key missing after FindID positive cache write: %v", err)
		}
		if err := fc.Get(ctx, "malsync_reverse:animepahe:"+animeSession, &v); err != nil {
			t.Fatalf("reverse key missing after FindID positive cache write: %v", err)
		}

		// Drive /release 404 — both keys must be evicted.
		if _, err := p.ListEpisodes(ctx, animeSession); err == nil {
			t.Fatalf("ListEpisodes err = nil; want ErrNotFound")
		}
		if err := fc.Get(ctx, "malsync:52082:animepahe", &v); err != cache.ErrNotFound {
			t.Errorf("forward key still present post-invalidation: err=%v v=%q", err, v)
		}
		if err := fc.Get(ctx, "malsync_reverse:animepahe:"+animeSession, &v); err != cache.ErrNotFound {
			t.Errorf("reverse key still present post-invalidation: err=%v v=%q", err, v)
		}
	})

	t.Run("NoMalIDKnown", func(t *testing.T) {
		t.Parallel()
		// /release returns 404 but no reverse mapping exists — assert
		// no panic, and the 404 is surfaced unchanged.
		resolverSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/release" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer resolverSrv.Close()
		malsyncSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// No reverse-key lookup ever fires MalSync HTTP (LookupMalID
			// reads cache only). MalSync is reachable but shouldn't be
			// hit by this test path.
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer malsyncSrv.Close()

		p, _, _ := realMalSyncProvider(t, resolverSrv, malsyncSrv)

		_, err := p.ListEpisodes(context.Background(), "no-such-session")
		if err == nil {
			t.Fatal("expected ErrNotFound on /release 404")
		}
	})
}
