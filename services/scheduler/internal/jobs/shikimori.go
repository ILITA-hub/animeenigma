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

func NewShikimoriSyncJob(config *config.JobsConfig, log *logger.Logger) *ShikimoriSyncJob {
	return &ShikimoriSyncJob{
		config: config,
		client: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: log,
	}
}

// Run executes the Shikimori sync job
func (j *ShikimoriSyncJob) Run(ctx context.Context) error {
	j.log.Info("starting Shikimori sync job")

	// Fetch anime data from Shikimori API
	animeList, err := j.fetchAnimeList(ctx)
	if err != nil {
		return fmt.Errorf("fetch anime list: %w", err)
	}

	j.log.Infow("fetched anime list", "count", len(animeList))

	// In a real implementation, this would:
	// 1. Fetch anime data from Shikimori API
	// 2. Update local database with new anime
	// 3. Update existing anime information
	// 4. Handle pagination
	// 5. Respect API rate limits

	j.log.Info("Shikimori sync job completed")
	return nil
}

func (j *ShikimoriSyncJob) fetchAnimeList(ctx context.Context) ([]map[string]interface{}, error) {
	// Build request
	url := fmt.Sprintf("%s/animes?limit=50&order=ranked", j.config.ShikimoriAPIURL)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}

	// Set User-Agent header as required by Shikimori API
	req.Header.Set("User-Agent", j.config.ShikimoriAppName)

	// Execute request
	resp, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response
	var animeList []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&animeList); err != nil {
		return nil, err
	}

	return animeList, nil
}
