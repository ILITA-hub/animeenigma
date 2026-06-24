package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/anime365"
)

func TestResolveAnime365File_CachesAfterFirstHit(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/episodeTranslations/") {
			calls++
			_, _ = w.Write([]byte("[Script Info]\n\n[Events]\nDialogue: 0,0:00:01.00,0:00:02.00,Default,,0,0,0,,hi\n"))
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	a365 := anime365.NewClient(anime365.Config{BaseURL: srv.URL, Enabled: true})
	agg := NewSubsAggregator(nil, nil, a365, nil, nil, resolveTestRedis(t), nil, logger.Default())

	body, format, err := agg.ResolveAnime365File(context.Background(), 5819457)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if format != "ass" || !strings.Contains(string(body), "Dialogue:") {
		t.Fatalf("body=%q format=%q", string(body), format)
	}
	if _, _, err := agg.ResolveAnime365File(context.Background(), 5819457); err != nil {
		t.Fatalf("resolve 2: %v", err)
	}
	if calls != 1 {
		t.Fatalf("upstream calls = %d, want 1 (second served from cache)", calls)
	}
}
