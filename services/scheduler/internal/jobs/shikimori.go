package jobs

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

type ShikimoriSyncJob struct {
	config *config.JobsConfig
	client *http.Client
	log    *logger.Logger
}

// OngoingAnimeResponse represents the response from catalog service
type OngoingAnimeResponse struct {
	Data []OngoingAnime `json:"data"`
	Meta struct {
		Page       int   `json:"page"`
		PageSize   int   `json:"page_size"`
		TotalCount int64 `json:"total_count"`
		TotalPages int   `json:"total_pages"`
	} `json:"meta"`
}

type OngoingAnime struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func NewShikimoriSyncJob(config *config.JobsConfig, log *logger.Logger) *ShikimoriSyncJob {
	return &ShikimoriSyncJob{
		config: config,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		log: log,
	}
}

// Run executes the Shikimori sync job - updates all ongoing anime from Shikimori
func (j *ShikimoriSyncJob) Run(ctx context.Context) error {
	j.log.Info("starting ongoing anime sync job")

	// Fetch all ongoing anime from catalog service
	ongoingAnime, err := j.fetchOngoingAnime(ctx)
	if err != nil {
		return fmt.Errorf("fetch ongoing anime: %w", err)
	}

	j.log.Infow("fetched ongoing anime list", "count", len(ongoingAnime))

	if len(ongoingAnime) == 0 {
		j.log.Info("no ongoing anime to update")
		return nil
	}

	// Update each anime with rate limiting
	var successCount, failCount int
	for i, anime := range ongoingAnime {
		select {
		case <-ctx.Done():
			j.log.Warn("sync job cancelled")
			return ctx.Err()
		default:
		}

		j.log.Infow("refreshing anime",
			"progress", fmt.Sprintf("%d/%d", i+1, len(ongoingAnime)),
			"id", anime.ID,
			"name", anime.Name,
		)

		if err := j.refreshAnime(ctx, anime.ID); err != nil {
			j.log.Warnw("failed to refresh anime",
				"id", anime.ID,
				"name", anime.Name,
				"error", err,
			)
			failCount++
		} else {
			successCount++
		}

		// Rate limiting: wait between requests to respect Shikimori API limits
		// Shikimori allows ~5 requests per second, we'll be more conservative
		time.Sleep(500 * time.Millisecond)
	}

	j.log.Infow("ongoing anime sync completed",
		"total", len(ongoingAnime),
		"success", successCount,
		"failed", failCount,
	)

	return nil
}

// fetchOngoingAnime fetches all ongoing anime from the catalog service
func (j *ShikimoriSyncJob) fetchOngoingAnime(ctx context.Context) ([]OngoingAnime, error) {
	var allAnime []OngoingAnime
	page := 1
	pageSize := 100

	for {
		url := fmt.Sprintf("%s/api/anime/ongoing?page=%d&page_size=%d",
			j.config.CatalogServiceURL, page, pageSize)

		req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
		if err != nil {
			return nil, err
		}

		resp, err := j.client.Do(req)
		if err != nil {
			return nil, err
		}

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			return nil, fmt.Errorf("catalog service returned status %d: %s", resp.StatusCode, string(body))
		}

		var response OngoingAnimeResponse
		if err := json.NewDecoder(resp.Body).Decode(&response); err != nil {
			resp.Body.Close()
			return nil, err
		}
		resp.Body.Close()

		allAnime = append(allAnime, response.Data...)

		// Check if we've fetched all pages
		if page >= response.Meta.TotalPages || len(response.Data) == 0 {
			break
		}

		page++
	}

	return allAnime, nil
}

// refreshAnime calls the catalog service to refresh a single anime from Shikimori
func (j *ShikimoriSyncJob) refreshAnime(ctx context.Context, animeID string) error {
	url := fmt.Sprintf("%s/api/anime/%s/refresh", j.config.CatalogServiceURL, animeID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return err
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("refresh returned status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}
