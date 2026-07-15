package config

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestProvidersConfig_ReplaceIsAtomic(t *testing.T) {
	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "allanime", Status: StatusEnabled}})
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					_ = pc.IsEnabled("allanime")
					_ = pc.Meta("allanime")
				}
			}
		}()
	}
	for i := 0; i < 100; i++ {
		st := StatusEnabled
		if i%2 != 0 {
			st = StatusDisabled
		}
		pc.Replace([]ProviderMeta{{Name: "allanime", Status: st}})
	}
	pc.Replace([]ProviderMeta{{Name: "allanime", Status: StatusDisabled}})
	close(stop)
	wg.Wait()
	if pc.IsEnabled("allanime") {
		t.Error("expected allanime disabled after final Replace")
	}
}

// TestStartProvidersRefresher_FiresCallbackEachPoll verifies the onRefresh
// callback runs after each successful refresh (so the orchestrator re-gate
// reflects the freshly-loaded catalog status without a restart).
func TestStartProvidersRefresher_FiresCallbackEachPoll(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"providers":[
			{"name":"gogoanime","status":"enabled","scraper_operated":true}
		]}}`))
	}))
	defer srv.Close()

	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "gogoanime", Status: StatusEnabled}})
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartProvidersRefresher(ctx, &pc, srv.URL, 20*time.Millisecond, nil, func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	})

	// Wait for at least 2 polls.
	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&calls) < 2 {
		select {
		case <-deadline:
			t.Fatalf("callback fired %d times; want >= 2", atomic.LoadInt32(&calls))
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// TestStartProvidersRefresher_CallbackNotFiredOnFailure verifies a failed
// refresh (catalog 500) does NOT fire the callback (last-good config kept).
func TestStartProvidersRefresher_CallbackNotFiredOnFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "gogoanime", Status: StatusEnabled}})
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartProvidersRefresher(ctx, &pc, srv.URL, 20*time.Millisecond, nil, func() error {
		atomic.AddInt32(&calls, 1)
		return nil
	})
	time.Sleep(200 * time.Millisecond)
	if got := atomic.LoadInt32(&calls); got != 0 {
		t.Errorf("callback fired %d times on failing refresh; want 0", got)
	}
}

func TestStartProvidersRefresher_CallbackRejectionRestoresLastGood(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"providers":[
			{"name":"gogoanime","status":"disabled","scraper_operated":true}
		]}}`))
	}))
	defer srv.Close()

	pc := NewProvidersConfigForTest([]ProviderMeta{{Name: "gogoanime", Status: StatusEnabled}})
	var calls int32
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	StartProvidersRefresher(ctx, &pc, srv.URL, 20*time.Millisecond, nil, func() error {
		atomic.AddInt32(&calls, 1)
		return errors.New("unsupported engine kind")
	})

	deadline := time.After(2 * time.Second)
	for atomic.LoadInt32(&calls) == 0 {
		select {
		case <-deadline:
			t.Fatal("rejection callback did not fire")
		case <-time.After(10 * time.Millisecond):
		}
	}
	if !pc.IsEnabled("gogoanime") {
		t.Fatal("rejected refresh replaced the last-good provider metadata")
	}
}
