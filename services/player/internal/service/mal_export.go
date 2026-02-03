package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// MALExportService handles MAL export operations
type MALExportService struct {
	httpClient       *http.Client
	schedulerURL     string
	log              *logger.Logger
}

// NewMALExportService creates a new MAL export service
func NewMALExportService(log *logger.Logger) *MALExportService {
	schedulerURL := "http://scheduler:8085"
	return &MALExportService{
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		schedulerURL: schedulerURL,
		log:          log,
	}
}

// MALAnimeEntry represents an anime entry from MAL
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

// ExportJobRequest is the request to create an export job
type ExportJobRequest struct {
	MALUsername string `json:"mal_username"`
}

// ExportJobResponse is the response from creating an export job
type ExportJobResponse struct {
	ID              string    `json:"id"`
	MALUsername     string    `json:"mal_username"`
	Status          string    `json:"status"`
	TotalAnime      int       `json:"total_anime"`
	ProcessedAnime  int       `json:"processed_anime"`
	LoadedAnime     int       `json:"loaded_anime"`
	SkippedAnime    int       `json:"skipped_anime"`
	FailedAnime     int       `json:"failed_anime"`
	ProgressPercent float64   `json:"progress_percent"`
	ErrorMessage    string    `json:"error_message,omitempty"`
	StartedAt       *time.Time `json:"started_at,omitempty"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// TaskInput represents a single anime to be loaded
type TaskInput struct {
	MALID         int    `json:"mal_id"`
	Title         string `json:"title"`
	TitleJapanese string `json:"title_japanese,omitempty"`
	TitleEnglish  string `json:"title_english,omitempty"`
}

// InitiateExport starts a new MAL export job
func (s *MALExportService) InitiateExport(ctx context.Context, userID, malUsername string) (*ExportJobResponse, error) {
	s.log.Infow("initiating MAL export",
		"user_id", userID,
		"mal_username", malUsername,
	)

	// First fetch the MAL list to get anime count and details
	entries, err := s.fetchMALList(ctx, malUsername)
	if err != nil {
		return nil, err
	}

	if len(entries) == 0 {
		return nil, errors.NotFound("MAL list is empty or private")
	}

	// Create export job in scheduler service
	job, err := s.createExportJob(ctx, userID, malUsername)
	if err != nil {
		return nil, err
	}

	// Create anime load tasks
	tasks := make([]TaskInput, 0, len(entries))
	for _, entry := range entries {
		tasks = append(tasks, TaskInput{
			MALID:         entry.AnimeID,
			Title:         entry.AnimeTitle,
			TitleJapanese: "", // MAL doesn't return Japanese title in list endpoint
			TitleEnglish:  entry.AnimeTitleEng,
		})
	}

	if err := s.createTasks(ctx, job.ID, userID, tasks); err != nil {
		s.log.Warnw("failed to create tasks, but job was created",
			"job_id", job.ID,
			"error", err,
		)
	}

	job.TotalAnime = len(entries)
	return job, nil
}

// GetExportStatus retrieves the status of an export job
func (s *MALExportService) GetExportStatus(ctx context.Context, exportID string) (*ExportJobResponse, error) {
	url := fmt.Sprintf("%s/api/v1/tasks/anime-load/status/%s", s.schedulerURL, exportID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "create request")
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "scheduler service")
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, errors.NotFound("export job not found")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.Internal(fmt.Sprintf("scheduler returned status %d", resp.StatusCode))
	}

	var result struct {
		Data ExportJobResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "decode response")
	}

	return &result.Data, nil
}

// GetUserExports retrieves all export jobs for a user
func (s *MALExportService) GetUserExports(ctx context.Context, userID string) ([]*ExportJobResponse, error) {
	// Note: The scheduler service would need an endpoint to list exports by user
	// For now, we'll return an empty list and implement this when needed
	return []*ExportJobResponse{}, nil
}

// CancelExport cancels an active export job
func (s *MALExportService) CancelExport(ctx context.Context, userID, exportID string) error {
	// TODO: Implement cancel endpoint in scheduler service
	return nil
}

// fetchMALList fetches a user's complete MAL anime list
func (s *MALExportService) fetchMALList(ctx context.Context, username string) ([]MALAnimeEntry, error) {
	var allEntries []MALAnimeEntry
	offset := 0
	batchSize := 300

	for {
		entries, err := s.fetchMALPage(ctx, username, offset)
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

// fetchMALPage fetches a single page of MAL anime list
func (s *MALExportService) fetchMALPage(ctx context.Context, username string, offset int) ([]MALAnimeEntry, error) {
	// MAL direct JSON endpoint: status=7 means all statuses
	url := fmt.Sprintf("https://myanimelist.net/animelist/%s/load.json?status=7&offset=%d", username, offset)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "create request")
	}

	// Set User-Agent to avoid being blocked
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "fetch MAL list")
	}
	defer resp.Body.Close()

	if resp.StatusCode == 400 {
		return nil, errors.NotFound("MAL user not found or list is private")
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

// createExportJob creates an export job in the scheduler service
func (s *MALExportService) createExportJob(ctx context.Context, userID, malUsername string) (*ExportJobResponse, error) {
	url := fmt.Sprintf("%s/api/v1/tasks/anime-load", s.schedulerURL)

	body := map[string]string{
		"user_id":      userID,
		"mal_username": malUsername,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "create request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, errors.CodeExternalAPI, "scheduler service")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, errors.Internal(fmt.Sprintf("scheduler returned status %d", resp.StatusCode))
	}

	var result struct {
		Data ExportJobResponse `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.Wrap(err, errors.CodeInternal, "decode response")
	}

	return &result.Data, nil
}

// createTasks creates anime load tasks in the scheduler service
func (s *MALExportService) createTasks(ctx context.Context, exportJobID, userID string, tasks []TaskInput) error {
	url := fmt.Sprintf("%s/api/v1/tasks/anime-load/tasks", s.schedulerURL)

	body := map[string]interface{}{
		"export_job_id": exportJobID,
		"user_id":       userID,
		"tasks":         tasks,
	}
	jsonBody, _ := json.Marshal(body)

	req, err := http.NewRequestWithContext(ctx, "POST", url, strings.NewReader(string(jsonBody)))
	if err != nil {
		return errors.Wrap(err, errors.CodeInternal, "create request")
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return errors.Wrap(err, errors.CodeExternalAPI, "scheduler service")
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return errors.Internal(fmt.Sprintf("scheduler returned status %d", resp.StatusCode))
	}

	return nil
}
