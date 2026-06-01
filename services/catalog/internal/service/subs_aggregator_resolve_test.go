package service

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/opensubtitles"
)

func resolveTestRedis(t *testing.T) *cache.RedisCache {
	t.Helper()
	host := os.Getenv("REDIS_HOST")
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if p := os.Getenv("REDIS_PORT"); p != "" {
		fmt.Sscanf(p, "%d", &port)
	}
	c, err := cache.New(cache.Config{Host: host, Port: port, DB: 12})
	if err != nil {
		t.Skipf("redis unreachable (%v); skipping resolve test", err)
	}
	_ = c.Client().FlushDB(context.Background()).Err()
	t.Cleanup(func() { _ = c.Client().FlushDB(context.Background()).Err(); _ = c.Close() })
	return c
}

func TestResolveOpenSubtitlesFile_CachesAfterFirstHit(t *testing.T) {
	calls := 0
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/download" {
			calls++
			fmt.Fprintf(w, `{"link":%q,"file_name":"ep.srt","remaining":99}`, srv.URL+"/file")
			return
		}
		_, _ = w.Write([]byte("1\n00:00:01,000 --> 00:00:02,000\nhi\n"))
	}))
	defer srv.Close()

	osc := opensubtitles.NewClient(opensubtitles.Config{APIKey: "k", BaseURL: srv.URL})
	agg := NewSubsAggregator(nil, osc, nil, nil, resolveTestRedis(t), logger.Default())

	body, format, err := agg.ResolveOpenSubtitlesFile(context.Background(), 42)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// Format is derived from file_name ("ep.srt") via formatFromName, NOT the
	// body content — so it must be "srt" here.
	if string(body) == "" || format != "srt" {
		t.Fatalf("body=%q format=%q", string(body), format)
	}
	// Second call must be served from cache — no new /download hit.
	if _, _, err := agg.ResolveOpenSubtitlesFile(context.Background(), 42); err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("download calls = %d, want 1 (second served from cache)", calls)
	}
}

func TestResolveOpenSubtitlesFile_QuotaPropagates(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"message":"download limit","remaining":0}`))
	}))
	defer srv.Close()
	osc := opensubtitles.NewClient(opensubtitles.Config{APIKey: "k", BaseURL: srv.URL})
	agg := NewSubsAggregator(nil, osc, nil, nil, resolveTestRedis(t), logger.Default())
	_, _, err := agg.ResolveOpenSubtitlesFile(context.Background(), 7)
	if err == nil {
		t.Fatal("want quota error, got nil")
	}
}
