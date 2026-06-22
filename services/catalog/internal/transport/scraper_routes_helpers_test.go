package transport

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/handler"
	"github.com/go-chi/chi/v5"
)

// stubScraperSvc satisfies handler.ScraperServiceAPI for route tests. It
// returns a fixed status + body. Used so the route test doesn't need a
// real GORM repo or HTTP scraper.
type stubScraperSvc struct {
	status int
	body   []byte
	err    error
}

func (s *stubScraperSvc) GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error) {
	return s.status, s.body, s.err
}
func (s *stubScraperSvc) GetScraperServers(ctx context.Context, animeID, episodeID, prefer string, exclusive bool) (int, []byte, error) {
	return s.status, s.body, s.err
}
func (s *stubScraperSvc) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool) (int, []byte, error) {
	return s.status, s.body, s.err
}
func (s *stubScraperSvc) GetScraperHealth(ctx context.Context) (int, []byte, error) {
	return s.status, s.body, s.err
}

// buildScraperOnlyRouter mirrors transport.NewRouter's /api/anime/{animeId}/scraper/*
// block but only registers the four scraper routes. This isolates the
// route-resolution test from the rest of the catalog routes (which need
// a real *handler.CatalogHandler backed by GORM).
func buildScraperOnlyRouter(h *handler.ScraperEndpointsHandler) chi.Router {
	r := chi.NewRouter()
	r.Route("/api", func(r chi.Router) {
		r.Route("/anime", func(r chi.Router) {
			r.Get("/{animeId}/scraper/episodes", h.GetScraperEpisodes)
			r.Get("/{animeId}/scraper/servers", h.GetScraperServers)
			r.Get("/{animeId}/scraper/stream", h.GetScraperStream)
			r.Get("/{animeId}/scraper/health", h.GetScraperHealth)
		})
	})
	return r
}
