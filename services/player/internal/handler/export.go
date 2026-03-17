package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
)

type ExportHandler struct {
	listService    *service.ListService
	log            *logger.Logger
	lastExport     sync.Map // userID -> time.Time
	exportCooldown time.Duration
}

func NewExportHandler(listService *service.ListService, log *logger.Logger) *ExportHandler {
	h := &ExportHandler{
		listService:    listService,
		log:            log,
		exportCooldown: 60 * time.Second,
	}

	// Cleanup stale entries every 5 minutes
	go func() {
		ticker := time.NewTicker(5 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			h.lastExport.Range(func(key, value any) bool {
				if time.Since(value.(time.Time)) > 5*time.Minute {
					h.lastExport.Delete(key)
				}
				return true
			})
		}
	}()

	return h
}

type exportEntry struct {
	AnimeenigmaID  string   `json:"animeenigma_id"`
	MalID          *int     `json:"mal_id"`
	ShikimoriID    *int     `json:"shikimori_id"`
	Title          string   `json:"title"`
	TitleRU        string   `json:"title_ru,omitempty"`
	TitleJP        string   `json:"title_jp,omitempty"`
	PosterURL      string   `json:"poster_url,omitempty"`
	EpisodesTotal  int      `json:"episodes_total"`
	EpisodesAired  int      `json:"episodes_aired"`
	Genres         []string `json:"genres"`
	Status         string   `json:"status"`
	Score          int      `json:"score"`
	EpisodesWatched int    `json:"episodes_watched"`
	Notes          string   `json:"notes,omitempty"`
	Tags           string   `json:"tags,omitempty"`
	IsRewatching   bool     `json:"is_rewatching"`
	Priority       string   `json:"priority,omitempty"`
	StartedAt      *time.Time `json:"started_at"`
	CompletedAt    *time.Time `json:"completed_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type exportResponse struct {
	ExportedAt   time.Time     `json:"exported_at"`
	User         string        `json:"user"`
	TotalEntries int           `json:"total_entries"`
	Entries      []exportEntry `json:"entries"`
}

// ExportJSON exports the user's full watchlist as a downloadable JSON file.
func (h *ExportHandler) ExportJSON(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	// Per-user rate limit
	if lastTime, ok := h.lastExport.Load(claims.UserID); ok {
		if time.Since(lastTime.(time.Time)) < h.exportCooldown {
			httputil.TooManyRequests(w)
			return
		}
	}
	h.lastExport.Store(claims.UserID, time.Now())

	entries, err := h.listService.GetUserList(r.Context(), claims.UserID, "")
	if err != nil {
		h.log.Errorw("failed to fetch user list for export", "user_id", claims.UserID, "error", err)
		httputil.Error(w, err)
		return
	}

	export := exportResponse{
		ExportedAt:   time.Now().UTC(),
		User:         claims.Username,
		TotalEntries: len(entries),
		Entries:      make([]exportEntry, 0, len(entries)),
	}

	for _, e := range entries {
		entry := toExportEntry(e)
		export.Entries = append(export.Entries, entry)
	}

	filename := fmt.Sprintf("animeenigma-export-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, filename))
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(export); err != nil {
		h.log.Errorw("failed to encode export JSON", "user_id", claims.UserID, "error", err)
	}
}

func toExportEntry(e *domain.AnimeListEntry) exportEntry {
	entry := exportEntry{
		AnimeenigmaID:   e.AnimeID,
		MalID:           e.MalID,
		ShikimoriID:     e.MalID, // Shikimori IDs = MAL IDs
		Status:          e.Status,
		Score:           e.Score,
		EpisodesWatched: e.Episodes,
		Notes:           e.Notes,
		Tags:            e.Tags,
		IsRewatching:    e.IsRewatching,
		Priority:        e.Priority,
		StartedAt:       e.StartedAt,
		CompletedAt:     e.CompletedAt,
		CreatedAt:       e.CreatedAt,
		UpdatedAt:       e.UpdatedAt,
	}

	if e.Anime != nil {
		entry.Title = e.Anime.Name
		entry.TitleRU = e.Anime.NameRU
		entry.TitleJP = e.Anime.NameJP
		entry.PosterURL = e.Anime.PosterURL
		entry.EpisodesTotal = e.Anime.EpisodesCount
		entry.EpisodesAired = e.Anime.EpisodesAired

		genres := make([]string, 0, len(e.Anime.Genres))
		for _, g := range e.Anime.Genres {
			genres = append(genres, g.Name)
		}
		entry.Genres = genres
	}

	if entry.Genres == nil {
		entry.Genres = []string{}
	}

	return entry
}
