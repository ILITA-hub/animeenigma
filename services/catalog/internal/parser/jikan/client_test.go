package jikan

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestGetAnimeByID_ParsesPopularityFields verifies the popularity fields added
// for the recs relative-MAL-popularity signal decode from the Jikan envelope.
func TestGetAnimeByID_ParsesPopularityFields(t *testing.T) {
	const body = `{"data":{
		"mal_id":52991,
		"title":"Sousou no Frieren",
		"title_english":"Frieren: Beyond Journey's End",
		"members":1234567,
		"favorites":78901,
		"popularity":42,
		"images":{"jpg":{"image_url":"https://img/s.jpg","large_image_url":"https://img/l.jpg"}}
	}}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()

	c := &Client{
		httpClient:  srv.Client(),
		baseURL:     srv.URL,
		rateLimiter: newRateLimiter(2),
	}

	info, err := c.GetAnimeByID(context.Background(), "52991")
	if err != nil {
		t.Fatalf("GetAnimeByID: %v", err)
	}
	if info.Members != 1234567 {
		t.Errorf("Members = %d, want 1234567", info.Members)
	}
	if info.Favorites != 78901 {
		t.Errorf("Favorites = %d, want 78901", info.Favorites)
	}
	if info.Popularity != 42 {
		t.Errorf("Popularity = %d, want 42", info.Popularity)
	}
	// Sanity: existing fields still parse.
	if info.MalID != 52991 || info.PosterURL() != "https://img/l.jpg" {
		t.Errorf("regression: mal_id=%d poster=%q", info.MalID, info.PosterURL())
	}
}
