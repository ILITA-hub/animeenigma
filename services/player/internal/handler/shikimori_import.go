package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/player/internal/service"
)

type ShikimoriImportHandler struct {
	listService      *service.ListService
	syncRepo         *repo.SyncRepository
	httpClient       *http.Client
	catalogURL       string
	shikimoriBaseURL string
	log              *logger.Logger
}

type shikimoriAnimeRate struct {
	ID        int    `json:"id"`
	Score     int    `json:"score"`
	Status    string `json:"status"`
	Episodes  int    `json:"episodes"`
	Rewatches int    `json:"rewatches"`
	Anime     struct {
		ID      int    `json:"id"`
		Name    string `json:"name"`
		Russian string `json:"russian"`
		Image   struct {
			Original string `json:"original"`
		} `json:"image"`
		Kind          string `json:"kind"`
		Episodes      int    `json:"episodes"`
		EpisodesAired int    `json:"episodes_aired"`
	} `json:"anime"`
}

func NewShikimoriImportHandler(listService *service.ListService, syncRepo *repo.SyncRepository, log *logger.Logger) *ShikimoriImportHandler {
	return &ShikimoriImportHandler{
		listService: listService,
		syncRepo:    syncRepo,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		catalogURL:       "http://catalog:8081",
		shikimoriBaseURL: "https://shikimori.one",
		log:              log,
	}
}

// ImportShikimoriList starts an async import of a user's Shikimori anime list
func (h *ShikimoriImportHandler) ImportShikimoriList(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	var req struct {
		Nickname string `json:"nickname"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	nickname := strings.TrimSpace(req.Nickname)
	if nickname == "" {
		httputil.BadRequest(w, "Shikimori nickname is required")
		return
	}

	// Check for an already active job
	activeJob, err := h.syncRepo.GetActiveByUserAndSource(r.Context(), claims.UserID, "shikimori")
	if err != nil {
		httputil.Error(w, errors.Wrap(err, errors.CodeInternal, "check active job"))
		return
	}
	if activeJob != nil {
		httputil.OK(w, map[string]interface{}{
			"job_id": activeJob.ID,
			"total":  activeJob.Total,
		})
		return
	}

	h.log.Infow("starting Shikimori import",
		"user_id", claims.UserID,
		"shikimori_nickname", nickname,
	)

	// Fetch the full list synchronously to validate the nickname
	entries, err := h.fetchAllShikimoriRates(r.Context(), nickname)
	if err != nil {
		h.log.Errorw("failed to fetch Shikimori list",
			"user_id", claims.UserID,
			"nickname", nickname,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	// Create job in DB
	job := &domain.SyncJob{
		UserID:         claims.UserID,
		Source:         "shikimori",
		SourceUsername: nickname,
		Status:         "processing",
		Total:          len(entries),
		StartedAt:      time.Now(),
	}
	if err := h.syncRepo.Create(r.Context(), job); err != nil {
		httputil.Error(w, errors.Wrap(err, errors.CodeInternal, "create sync job"))
		return
	}

	metrics.SyncJobsStartedTotal.WithLabelValues("shikimori").Inc()

	// Process in background
	go h.processImport(claims.UserID, job.ID, entries)

	httputil.OK(w, map[string]interface{}{
		"job_id": job.ID,
		"total":  job.Total,
	})
}

// MigrateShikimoriEntries migrates all shiki_* watchlist entries to real catalog IDs
func (h *ShikimoriImportHandler) MigrateShikimoriEntries(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	entries, err := h.listService.GetUserList(r.Context(), claims.UserID, "")
	if err != nil {
		httputil.Error(w, err)
		return
	}

	migrated := 0
	failed := 0

	for _, entry := range entries {
		if !strings.HasPrefix(entry.AnimeID, "shiki_") {
			continue
		}

		shikimoriID := strings.TrimPrefix(entry.AnimeID, "shiki_")
		var shikiIDInt int
		if _, err := fmt.Sscanf(shikimoriID, "%d", &shikiIDInt); err != nil {
			failed++
			continue
		}
		catalogAnime := h.searchCatalogByShikimoriID(r.Context(), shikiIDInt)
		if catalogAnime == nil {
			failed++
			continue
		}

		if _, err := h.listService.MigrateListEntry(r.Context(), claims.UserID, entry.AnimeID, catalogAnime.ID); err != nil {
			failed++
		} else {
			migrated++
		}

		time.Sleep(200 * time.Millisecond)
	}

	httputil.OK(w, map[string]int{
		"migrated": migrated,
		"failed":   failed,
	})
}

func (h *ShikimoriImportHandler) processImport(userID, jobID string, entries []shikimoriAnimeRate) {
	ctx := context.Background()
	startTime := time.Now()
	imported := 0
	skipped := 0

	defer func() {
		if r := recover(); r != nil {
			h.log.Errorw("Shikimori import panicked",
				"user_id", userID,
				"job_id", jobID,
				"panic", r,
			)
			_ = h.syncRepo.Complete(ctx, jobID, "failed", fmt.Sprintf("panic: %v", r), imported, skipped)
			metrics.SyncJobsTotal.WithLabelValues("shikimori", "failed").Inc()
			metrics.SyncJobDurationSeconds.WithLabelValues("shikimori").Observe(time.Since(startTime).Seconds())
		}
	}()

	processed := 0
	for _, entry := range entries {
		status := convertShikimoriStatus(entry.Status)
		if status == "" {
			skipped++
			metrics.SyncJobEntriesTotal.WithLabelValues("shikimori", "skipped").Inc()
			processed++
			if processed%10 == 0 {
				_ = h.syncRepo.UpdateProgress(ctx, jobID, imported, skipped)
			}
			continue
		}

		// Always resolve via catalog — get real UUID, English title, poster
		catalogAnime := h.searchCatalogByShikimoriID(ctx, entry.Anime.ID)
		if catalogAnime == nil {
			skipped++
			metrics.SyncJobEntriesTotal.WithLabelValues("shikimori", "skipped").Inc()
			processed++
			if processed%10 == 0 {
				_ = h.syncRepo.UpdateProgress(ctx, jobID, imported, skipped)
			}
			time.Sleep(200 * time.Millisecond)
			continue
		}

		animeID := catalogAnime.ID

		listReq := &domain.UpdateListRequest{
			AnimeID: animeID,
			Status:  status,
		}

		if entry.Score > 0 {
			listReq.Score = &entry.Score
		}

		if entry.Episodes > 0 {
			listReq.Episodes = &entry.Episodes
		}

		isRewatching := entry.Status == "rewatching"
		if isRewatching {
			listReq.IsRewatching = &isRewatching
		}

		if _, err := h.listService.UpdateListEntry(ctx, userID, "", listReq); err != nil {
			skipped++
			metrics.SyncJobEntriesTotal.WithLabelValues("shikimori", "skipped").Inc()
		} else {
			imported++
			metrics.SyncJobEntriesTotal.WithLabelValues("shikimori", "imported").Inc()
		}

		processed++
		if processed%10 == 0 {
			_ = h.syncRepo.UpdateProgress(ctx, jobID, imported, skipped)
		}

		time.Sleep(200 * time.Millisecond)
	}

	_ = h.syncRepo.Complete(ctx, jobID, "completed", "", imported, skipped)

	metrics.SyncJobsTotal.WithLabelValues("shikimori", "completed").Inc()
	metrics.SyncJobDurationSeconds.WithLabelValues("shikimori").Observe(time.Since(startTime).Seconds())

	h.log.Infow("Shikimori import completed",
		"user_id", userID,
		"job_id", jobID,
		"imported", imported,
		"skipped", skipped,
	)
}

func (h *ShikimoriImportHandler) fetchAllShikimoriRates(ctx context.Context, nickname string) ([]shikimoriAnimeRate, error) {
	var allEntries []shikimoriAnimeRate
	page := 1

	for {
		entries, err := h.fetchShikimoriPage(ctx, nickname, page)
		if err != nil {
			return nil, err
		}

		allEntries = append(allEntries, entries...)

		// Shikimori returns up to 5000 per page; if we get less, we're done
		if len(entries) < 5000 {
			break
		}

		page++
		time.Sleep(200 * time.Millisecond)
	}

	return allEntries, nil
}

func (h *ShikimoriImportHandler) fetchShikimoriPage(ctx context.Context, nickname string, page int) ([]shikimoriAnimeRate, error) {
	url := fmt.Sprintf("%s/api/users/%s/anime_rates?limit=5000&page=%d", h.shikimoriBaseURL, nickname, page)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "create request")
	}

	req.Header.Set("User-Agent", "AnimeEnigma/1.0")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "fetch Shikimori list")
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case 404:
		return nil, errors.New(errors.CodeNotFound, "Shikimori user not found")
	case 403:
		return nil, errors.New(errors.CodeForbidden, "Shikimori profile is private")
	}

	if resp.StatusCode != 200 {
		return nil, errors.Internal(fmt.Sprintf("Shikimori API error: %d", resp.StatusCode))
	}

	var entries []shikimoriAnimeRate
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "decode Shikimori response")
	}

	return entries, nil
}

func convertShikimoriStatus(status string) string {
	switch status {
	case "planned":
		return "plan_to_watch"
	case "watching":
		return "watching"
	case "rewatching":
		return "watching"
	case "completed":
		return "completed"
	case "on_hold":
		return "on_hold"
	case "dropped":
		return "dropped"
	default:
		return ""
	}
}

func (h *ShikimoriImportHandler) searchCatalogByShikimoriID(ctx context.Context, shikimoriID int) *CatalogAnime {
	url := fmt.Sprintf("%s/api/anime/shikimori/%d", h.catalogURL, shikimoriID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil
	}

	var result struct {
		Data CatalogAnime `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if result.Data.ID != "" {
		return &result.Data
	}

	return nil
}
