package shikimori

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

func TestGetAnnouncedAnime_QueryShapeAndMapping(t *testing.T) {
	var gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/graphql" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		var req struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(body, &req)
		gotQuery = req.Query
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"animes":[{
			"id":"60001","name":"Frieren 2","russian":"Фрирен 2","japanese":"葬送のフリーレン2",
			"status":"anons","score":0,
			"poster":{"originalUrl":"https://x/p.jpg"},
			"genres":[{"id":"8","name":"Drama","russian":"Драма"}],
			"studios":[{"id":"11","name":"Madhouse"}]
		}]}}`))
	}))
	defer srv.Close()

	got, err := newTestClient(srv.URL).GetAnnouncedAnime(context.Background(), 1, 30)
	if err != nil {
		t.Fatalf("GetAnnouncedAnime: %v", err)
	}
	if !strings.Contains(gotQuery, `status: "anons"`) {
		t.Errorf("query must filter status anons, got: %s", gotQuery)
	}
	if !strings.Contains(gotQuery, "order: popularity") {
		t.Errorf("query must order by popularity, got: %s", gotQuery)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 anime, got %d", len(got))
	}
	a := got[0]
	if a.Status != domain.StatusAnnounced {
		t.Errorf("status: want %q got %q", domain.StatusAnnounced, a.Status)
	}
	if a.ShikimoriID != "60001" || len(a.Genres) != 1 || len(a.Studios) != 1 {
		t.Errorf("mapping incomplete: %+v", a)
	}
}
