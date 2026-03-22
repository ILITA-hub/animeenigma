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

type TopAnimeSyncJob struct {
	config *config.JobsConfig
	client *http.Client
	log    *logger.Logger
}

func NewTopAnimeSyncJob(config *config.JobsConfig, log *logger.Logger) *TopAnimeSyncJob {
	return &TopAnimeSyncJob{
		config: config,
		client: &http.Client{
			Timeout: 60 * time.Second,
		},
		log: log,
	}
}

// Run fetches top-20 trending anime from the catalog service, which triggers
// Shikimori fetch + cache warm on cache miss.
func (j *TopAnimeSyncJob) Run(ctx context.Context) error {
	j.log.Info("starting top anime sync job")

	url := fmt.Sprintf("%s/api/anime/trending?page_size=20", j.config.CatalogServiceURL)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch trending anime: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("catalog service returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to count results
	var result struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	j.log.Infow("top anime sync completed", "count", len(result.Data))
	return nil
}
