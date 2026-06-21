package handler

import (
	"context"
	"errors"
	"net/http"
	"strconv"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/library/internal/domain"
	"github.com/go-chi/chi/v5"
)

// EpisodeStoreReader is the slice of *repo.EpisodeRepository the
// handler needs. Pulled out so tests can inject a stub without
// spinning up Postgres.
type EpisodeStoreReader interface {
	GetByShikimoriEpisode(ctx context.Context, shikimoriID string, episodeNumber int) (*domain.Episode, error)
	List(ctx context.Context, shikimoriID string) ([]domain.Episode, error)
	ListRecentDistinct(ctx context.Context, limit int) ([]domain.Episode, error)
}

// URLBuilder is the slice of *minio.Writer the handler needs (the
// URLFor method). The episodes endpoint returns the playlist URL
// the streaming proxy fronts.
type URLBuilder interface {
	URLFor(path string) string
}

// EpisodesHandler implements GET /api/library/episodes/{shikimori_id}/{episode}.
// Read-only; consumed by Phase 5 admin UI + Phase 6 hybrid resolver.
type EpisodesHandler struct {
	episodeRepo EpisodeStoreReader
	urlBuilder  URLBuilder
	log         *logger.Logger
}

// NewEpisodesHandler constructs the handler.
func NewEpisodesHandler(episodeRepo EpisodeStoreReader, urlBuilder URLBuilder, log *logger.Logger) *EpisodesHandler {
	return &EpisodesHandler{
		episodeRepo: episodeRepo,
		urlBuilder:  urlBuilder,
		log:         log,
	}
}

// episodeResponse is the JSON shape locked in 04-SPEC.
type episodeResponse struct {
	MinioURL    string `json:"minio_url"`
	DurationSec int    `json:"duration_sec,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
}

// episodeListItem is one entry in the List response — episode number +
// its playlist URL, so the first-party ("ae") provider can render an
// episode list straight from what's actually in MinIO (no upstream
// metadata source needed).
type episodeListItem struct {
	EpisodeNumber int    `json:"episode_number"`
	MinioURL      string `json:"minio_url"`
	DurationSec   int    `json:"duration_sec,omitempty"`
}

// listResponse wraps the episode array under "episodes" (consistent
// with the jobs list envelope).
type listResponse struct {
	Episodes []episodeListItem `json:"episodes"`
}

// List handles GET /api/library/episodes/{shikimori_id} — every
// episode present in MinIO for that anime, ordered by number. Returns
// 200 with an empty array when none exist (NOT 404): "no local copies
// yet" is a legitimate empty state the caller renders as an empty
// provider, distinct from a bad request.
func (h *EpisodesHandler) List(w http.ResponseWriter, r *http.Request) {
	shikimoriID := chi.URLParam(r, "shikimori_id")
	if shikimoriID == "" {
		httputil.BadRequest(w, "shikimori_id is required")
		return
	}
	eps, err := h.episodeRepo.List(r.Context(), shikimoriID)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	items := make([]episodeListItem, 0, len(eps))
	for i := range eps {
		ep := &eps[i]
		item := episodeListItem{
			EpisodeNumber: ep.EpisodeNumber,
			MinioURL:      h.urlBuilder.URLFor(ep.MinioPath + "playlist.m3u8"),
		}
		if ep.DurationSec != nil {
			item.DurationSec = *ep.DurationSec
		}
		items = append(items, item)
	}
	httputil.OK(w, listResponse{Episodes: items})
}

// recentItem is one entry in the RecentEpisodes response — the (anime, episode)
// the probe should target. Minimal on purpose: the catalog ae-targets endpoint
// maps shikimori_id → catalog UUID + title.
type recentItem struct {
	ShikimoriID   string `json:"shikimori_id"`
	EpisodeNumber int    `json:"episode_number"`
}

type recentResponse struct {
	Episodes []recentItem `json:"episodes"`
}

// RecentEpisodes handles GET /internal/library/recent-episodes?limit=N — the
// newest episode of each distinct anime, newest upload first (Docker-network
// only; feeds the analytics playback probe's ae target set). Returns 200 with
// an empty array when the library holds nothing yet.
func (h *EpisodesHandler) RecentEpisodes(w http.ResponseWriter, r *http.Request) {
	limit := 3
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 {
			limit = n
		}
	}
	eps, err := h.episodeRepo.ListRecentDistinct(r.Context(), limit)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	items := make([]recentItem, 0, len(eps))
	for i := range eps {
		items = append(items, recentItem{
			ShikimoriID:   eps[i].ShikimoriID,
			EpisodeNumber: eps[i].EpisodeNumber,
		})
	}
	httputil.OK(w, recentResponse{Episodes: items})
}

// Get handles GET /api/library/episodes/{shikimori_id}/{episode}.
// Returns 200 + episodeResponse on hit, 404 on miss, 400 on bad
// episode arg, 500 on internal repo error.
func (h *EpisodesHandler) Get(w http.ResponseWriter, r *http.Request) {
	shikimoriID := chi.URLParam(r, "shikimori_id")
	if shikimoriID == "" {
		httputil.BadRequest(w, "shikimori_id is required")
		return
	}
	episodeStr := chi.URLParam(r, "episode")
	episode, err := strconv.Atoi(episodeStr)
	if err != nil || episode < 1 {
		httputil.BadRequest(w, "invalid episode")
		return
	}

	ep, err := h.episodeRepo.GetByShikimoriEpisode(r.Context(), shikimoriID, episode)
	if err != nil {
		// Map liberrors.NotFound → 404; everything else surfaces via the
		// liberrors helper which httputil.Error already understands.
		var appErr *liberrors.AppError
		if errors.As(err, &appErr) && appErr.Code == liberrors.CodeNotFound {
			httputil.NotFound(w, "episode")
			return
		}
		httputil.Error(w, err)
		return
	}

	// Build response. MinioPath already ends with "/" — we append
	// "playlist.m3u8".
	url := h.urlBuilder.URLFor(ep.MinioPath + "playlist.m3u8")
	resp := episodeResponse{MinioURL: url}
	if ep.DurationSec != nil {
		resp.DurationSec = *ep.DurationSec
	}
	if ep.SizeBytes != nil {
		resp.SizeBytes = *ep.SizeBytes
	}
	httputil.OK(w, resp)
}
