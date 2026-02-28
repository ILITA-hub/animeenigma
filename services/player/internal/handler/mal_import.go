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

type MALImportHandler struct {
	listService *service.ListService
	syncRepo    *repo.SyncRepository
	httpClient  *http.Client
	catalogURL  string
	log         *logger.Logger
}

func NewMALImportHandler(listService *service.ListService, syncRepo *repo.SyncRepository, log *logger.Logger) *MALImportHandler {
	catalogURL := "http://catalog:8081"
	return &MALImportHandler{
		listService: listService,
		syncRepo:    syncRepo,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		catalogURL: catalogURL,
		log:        log,
	}
}

// MALImportRequest represents the import request
type MALImportRequest struct {
	Username string `json:"username"`
}

// MAL direct JSON endpoint response
type MALAnimeEntry struct {
	Status               int    `json:"status"`
	Score                int    `json:"score"`
	NumWatchedEpisodes   int    `json:"num_watched_episodes"`
	AnimeTitle           string `json:"anime_title"`
	AnimeTitleEng        string `json:"anime_title_eng"`
	AnimeID              int    `json:"anime_id"`
	AnimeImagePath       string `json:"anime_image_path"`
	AnimeNumEpisodes     int    `json:"anime_num_episodes"`
	Tags                 string `json:"tags"`
	IsRewatching         int    `json:"is_rewatching"`
	PriorityString       string `json:"priority_string"`
	Notes                string `json:"notes"`
	AnimeMediaTypeString string `json:"anime_media_type_string"`
	StartDateString      string `json:"start_date_string"`
	FinishDateString     string `json:"finish_date_string"`
}

// CatalogAnime represents anime from catalog service
type CatalogAnime struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	NameRU      string `json:"name_ru"`
	MalID       string `json:"mal_id"`
	ShikimoriID string `json:"shikimori_id"`
	PosterURL   string `json:"poster_url"`
}

// ImportMALList imports a user's MAL anime list asynchronously
func (h *MALImportHandler) ImportMALList(w http.ResponseWriter, r *http.Request) {
	claims, ok := authz.ClaimsFromContext(r.Context())
	if !ok || claims == nil {
		httputil.Unauthorized(w)
		return
	}

	var req MALImportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		httputil.BadRequest(w, "invalid request body")
		return
	}

	if req.Username == "" {
		httputil.BadRequest(w, "MAL username is required")
		return
	}

	// Clean username
	username := strings.TrimSpace(req.Username)

	// Check for existing active job
	activeJob, err := h.syncRepo.GetActiveByUserAndSource(r.Context(), claims.UserID, "mal")
	if err != nil {
		h.log.Errorw("failed to check active sync jobs",
			"user_id", claims.UserID,
			"error", err,
		)
		httputil.Error(w, errors.Wrap(err, errors.CodeInternal, "check active jobs"))
		return
	}
	if activeJob != nil {
		h.log.Infow("returning existing active MAL import job",
			"user_id", claims.UserID,
			"job_id", activeJob.ID,
		)
		httputil.OK(w, map[string]interface{}{
			"job_id": activeJob.ID,
			"total":  activeJob.Total,
		})
		return
	}

	h.log.Infow("starting MAL import",
		"user_id", claims.UserID,
		"mal_username", username,
	)

	// Fetch all MAL entries first to validate username and get total count
	entries, err := h.fetchAllMALEntries(r.Context(), username)
	if err != nil {
		h.log.Errorw("MAL import failed to fetch entries",
			"user_id", claims.UserID,
			"mal_username", username,
			"error", err,
		)
		httputil.Error(w, err)
		return
	}

	// Create sync job in DB
	job := &domain.SyncJob{
		UserID:         claims.UserID,
		Source:         "mal",
		SourceUsername: username,
		Status:         "processing",
		Total:          len(entries),
		StartedAt:      time.Now(),
	}
	if err := h.syncRepo.Create(r.Context(), job); err != nil {
		h.log.Errorw("failed to create sync job",
			"user_id", claims.UserID,
			"error", err,
		)
		httputil.Error(w, errors.Wrap(err, errors.CodeInternal, "create sync job"))
		return
	}

	// Record metric
	metrics.SyncJobsStartedTotal.WithLabelValues("mal").Inc()

	// Launch background processing
	go h.processMALImport(claims.UserID, job.ID, entries)

	h.log.Infow("MAL import job created",
		"user_id", claims.UserID,
		"job_id", job.ID,
		"total", len(entries),
	)

	httputil.OK(w, map[string]interface{}{
		"job_id": job.ID,
		"total":  len(entries),
	})
}

// fetchAllMALEntries fetches all entries from a MAL user's anime list
func (h *MALImportHandler) fetchAllMALEntries(ctx context.Context, username string) ([]MALAnimeEntry, error) {
	var allEntries []MALAnimeEntry
	offset := 0
	batchSize := 300

	for {
		entries, err := h.fetchMALPage(ctx, username, offset)
		if err != nil {
			return nil, err
		}

		if len(entries) == 0 {
			break
		}

		allEntries = append(allEntries, entries...)

		if len(entries) < batchSize {
			break
		}

		offset += batchSize
		// Small delay to be nice to MAL servers
		time.Sleep(200 * time.Millisecond)
	}

	return allEntries, nil
}

// processMALImport processes MAL entries in the background
func (h *MALImportHandler) processMALImport(userID, jobID string, entries []MALAnimeEntry) {
	ctx := context.Background()
	startTime := time.Now()
	imported := 0
	skipped := 0

	// Recover from panics so we can mark the job as failed
	defer func() {
		if r := recover(); r != nil {
			errMsg := fmt.Sprintf("panic: %v", r)
			h.log.Errorw("MAL import panicked",
				"user_id", userID,
				"job_id", jobID,
				"error", errMsg,
			)
			if err := h.syncRepo.Complete(ctx, jobID, "failed", errMsg, imported, skipped); err != nil {
				h.log.Errorw("failed to mark sync job as failed after panic",
					"job_id", jobID,
					"error", err,
				)
			}
			metrics.SyncJobsTotal.WithLabelValues("mal", "failed").Inc()
			metrics.SyncJobDurationSeconds.WithLabelValues("mal").Observe(time.Since(startTime).Seconds())
		}
	}()

	for i, entry := range entries {
		status := h.convertMALStatus(entry.Status)
		if status == "" {
			skipped++
			metrics.SyncJobEntriesTotal.WithLabelValues("mal", "skipped").Inc()

			// Update progress every 10 entries
			if (i+1)%10 == 0 {
				if err := h.syncRepo.UpdateProgress(ctx, jobID, imported, skipped); err != nil {
					h.log.Errorw("failed to update sync progress",
						"job_id", jobID,
						"error", err,
					)
				}
			}
			continue
		}

		// Try to find anime in catalog by MAL ID
		catalogAnime := h.searchCatalogByMALID(ctx, entry.AnimeID)
		if catalogAnime == nil {
			h.log.Infow("skipping unresolved MAL entry",
				"mal_id", entry.AnimeID,
				"title", entry.AnimeTitle,
			)
			skipped++
			metrics.SyncJobEntriesTotal.WithLabelValues("mal", "skipped").Inc()

			// Update progress every 10 entries
			if (i+1)%10 == 0 {
				if err := h.syncRepo.UpdateProgress(ctx, jobID, imported, skipped); err != nil {
					h.log.Errorw("failed to update sync progress",
						"job_id", jobID,
						"error", err,
					)
				}
			}

			// Small delay to be nice to catalog service
			time.Sleep(200 * time.Millisecond)
			continue
		}

		listReq := &domain.UpdateListRequest{
			AnimeID: catalogAnime.ID,
			Status:  status,
			MalID:   &entry.AnimeID,
		}

		if entry.Score > 0 {
			listReq.Score = &entry.Score
		}

		if entry.NumWatchedEpisodes > 0 {
			listReq.Episodes = &entry.NumWatchedEpisodes
		}

		if entry.Tags != "" {
			listReq.Tags = &entry.Tags
		}

		if entry.Notes != "" {
			listReq.Notes = &entry.Notes
		}

		isRewatching := entry.IsRewatching == 1
		listReq.IsRewatching = &isRewatching

		priority := strings.ToLower(entry.PriorityString)
		if priority != "" {
			listReq.Priority = &priority
		}

		// Parse and set start/finish dates from MAL
		if entry.StartDateString != "" && entry.StartDateString != "-" {
			if startDate := h.parseMALDate(entry.StartDateString); startDate != nil {
				listReq.StartedAt = startDate
			}
		}

		if entry.FinishDateString != "" && entry.FinishDateString != "-" {
			if finishDate := h.parseMALDate(entry.FinishDateString); finishDate != nil {
				listReq.CompletedAt = finishDate
			}
		}

		if _, err := h.listService.UpdateListEntry(ctx, userID, "", listReq); err != nil {
			skipped++
			metrics.SyncJobEntriesTotal.WithLabelValues("mal", "skipped").Inc()
		} else {
			imported++
			metrics.SyncJobEntriesTotal.WithLabelValues("mal", "imported").Inc()
		}

		// Update progress every 10 entries
		if (i+1)%10 == 0 {
			if err := h.syncRepo.UpdateProgress(ctx, jobID, imported, skipped); err != nil {
				h.log.Errorw("failed to update sync progress",
					"job_id", jobID,
					"error", err,
				)
			}
		}

		// Small delay to be nice to catalog service
		time.Sleep(200 * time.Millisecond)
	}

	// Mark job as completed
	if err := h.syncRepo.Complete(ctx, jobID, "completed", "", imported, skipped); err != nil {
		h.log.Errorw("failed to mark sync job as completed",
			"job_id", jobID,
			"error", err,
		)
	}

	// Record metrics
	metrics.SyncJobsTotal.WithLabelValues("mal", "completed").Inc()
	metrics.SyncJobDurationSeconds.WithLabelValues("mal").Observe(time.Since(startTime).Seconds())

	h.log.Infow("MAL import completed",
		"user_id", userID,
		"job_id", jobID,
		"imported", imported,
		"skipped", skipped,
		"duration", time.Since(startTime).String(),
	)
}

func (h *MALImportHandler) fetchMALPage(ctx context.Context, username string, offset int) ([]MALAnimeEntry, error) {
	// MAL direct JSON endpoint: status=7 means all statuses
	url := fmt.Sprintf("https://myanimelist.net/animelist/%s/load.json?status=7&offset=%d", username, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "create request")
	}

	// Set User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := h.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "fetch MAL list")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		return nil, errors.New(errors.CodeNotFound, "MAL user not found or list is private")
	}

	if resp.StatusCode != 200 {
		return nil, errors.Internal(fmt.Sprintf("MAL error: %d", resp.StatusCode))
	}

	var entries []MALAnimeEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "decode response")
	}

	return entries, nil
}

// convertMALStatus converts MAL numeric status to our string status
// MAL statuses: 1=watching, 2=completed, 3=on_hold, 4=dropped, 6=plan_to_watch
func (h *MALImportHandler) convertMALStatus(malStatus int) string {
	switch malStatus {
	case 1:
		return "watching"
	case 2:
		return "completed"
	case 3:
		return "on_hold"
	case 4:
		return "dropped"
	case 6:
		return "plan_to_watch"
	default:
		return ""
	}
}

// parseMALDate parses MAL date strings in various formats
// MAL can return dates like "01-15-2024", "2024-01-15", "-" (not set), etc.
func (h *MALImportHandler) parseMALDate(dateStr string) *time.Time {
	if dateStr == "" || dateStr == "-" {
		return nil
	}

	// Try common MAL date formats
	formats := []string{
		"01-02-2006",  // MM-DD-YYYY
		"2006-01-02",  // YYYY-MM-DD
		"Jan 2, 2006", // Month Day, Year
		"02-01-2006",  // DD-MM-YYYY
	}

	for _, format := range formats {
		if t, err := time.Parse(format, dateStr); err == nil {
			return &t
		}
	}

	h.log.Debugw("could not parse MAL date", "date_string", dateStr)
	return nil
}

// searchCatalogByMALID searches the catalog service for anime by MAL ID.
// The catalog now returns a MALResolveResult with status "resolved" or "ambiguous".
func (h *MALImportHandler) searchCatalogByMALID(ctx context.Context, malID int) *CatalogAnime {
	url := fmt.Sprintf("%s/api/anime/mal/%d", h.catalogURL, malID)

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
		Data struct {
			Status string       `json:"status"`
			Anime  CatalogAnime `json:"anime"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if result.Data.Status == "resolved" && result.Data.Anime.ID != "" {
		return &result.Data.Anime
	}

	return nil
}
