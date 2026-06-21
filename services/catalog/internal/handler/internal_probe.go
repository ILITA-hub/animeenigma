package handler

// Playback-probe support — ae target set.
//
// GET /internal/probe/ae-targets?limit=N
//
// Returns the (anime UUID, display title, episode) tuples the analytics
// playback probe should hit for the self-hosted "ae" library player: the
// newest distinct-anime library uploads, mapped from the library's
// shikimori_id space into catalog UUIDs. shikimori_ids with no catalog row are
// skipped (the probe can only resolve a stream via /api/anime/{uuid}/ae/stream).
//
// Mounted OUTSIDE /api at the root router with NO middleware — same
// gateway-non-routing security model as the other /internal/* endpoints (the
// gateway does not proxy /internal/*).
//
// Response 200: {"targets":[{"uuid":"...","name":"...","episode":3}]}
// Response 500: library/HTTP failure.

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/library"
)

// libraryRecentLister is the one library-client method the handler needs.
type libraryRecentLister interface {
	RecentEpisodes(ctx context.Context, limit int) ([]library.RecentEpisode, error)
}

// animeByShikimori is the one anime-repo method the handler needs. The repo
// returns (nil, nil) when no row matches, which the handler treats as "skip".
type animeByShikimori interface {
	GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error)
}

// AeTarget is one probe target: a catalog anime + the uploaded episode.
type AeTarget struct {
	UUID    string `json:"uuid"`
	Name    string `json:"name"`
	Episode int    `json:"episode"`
}

type aeTargetsResponse struct {
	Targets []AeTarget `json:"targets"`
}

// InternalProbeHandler implements GET /internal/probe/ae-targets.
type InternalProbeHandler struct {
	lib  libraryRecentLister
	repo animeByShikimori
	log  *logger.Logger
}

// NewInternalProbeHandler constructs the handler.
func NewInternalProbeHandler(lib libraryRecentLister, repo animeByShikimori, log *logger.Logger) *InternalProbeHandler {
	return &InternalProbeHandler{lib: lib, repo: repo, log: log}
}

// AeTargets handles GET /internal/probe/ae-targets?limit=N.
func (h *InternalProbeHandler) AeTargets(w http.ResponseWriter, r *http.Request) {
	limit := 3
	if q := r.URL.Query().Get("limit"); q != "" {
		if n, err := strconv.Atoi(q); err == nil && n > 0 && n <= 20 {
			limit = n
		}
	}
	recent, err := h.lib.RecentEpisodes(r.Context(), limit)
	if err != nil {
		httputil.Error(w, err)
		return
	}
	out := make([]AeTarget, 0, len(recent))
	for _, re := range recent {
		a, err := h.repo.GetByShikimoriID(r.Context(), re.ShikimoriID)
		if err != nil {
			// A lookup failure for one anime shouldn't sink the whole set; log + skip.
			if h.log != nil {
				h.log.Warnw("ae-targets: shikimori lookup failed", "shikimori_id", re.ShikimoriID, "error", err)
			}
			continue
		}
		if a == nil {
			continue // not in catalog → can't resolve an ae stream by UUID
		}
		out = append(out, AeTarget{UUID: a.ID, Name: pickAnimeName(a), Episode: re.EpisodeNumber})
	}
	httputil.OK(w, aeTargetsResponse{Targets: out})
}

// pickAnimeName prefers the Russian title, then English, then the primary
// (romaji/original) name — matching the probe's reason-column preference.
func pickAnimeName(a *domain.Anime) string {
	switch {
	case a.NameRU != "":
		return a.NameRU
	case a.NameEN != "":
		return a.NameEN
	default:
		return a.Name
	}
}
