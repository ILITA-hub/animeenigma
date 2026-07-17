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

// AnnouncementsSyncJob triggers catalog's announcement discovery
// (spec 2026-07-17): top-popularity anons import + franchise enrichment.
type AnnouncementsSyncJob struct {
	config *config.JobsConfig
	client *http.Client
	log    *logger.Logger
}

type announcementsSyncResponse struct {
	Data struct {
		Imported  int `json:"imported"`
		Refreshed int `json:"refreshed"`
		Enriched  int `json:"enriched"`
		Failed    int `json:"failed"`
	} `json:"data"`
}

func NewAnnouncementsSyncJob(config *config.JobsConfig, log *logger.Logger) *AnnouncementsSyncJob {
	return &AnnouncementsSyncJob{
		config: config,
		client: &http.Client{
			// Rate-limited Shikimori fan-out (franchise REST calls) can be slow.
			Timeout: 600 * time.Second,
		},
		log: log,
	}
}

// Run calls the catalog announcements-sync endpoint.
func (j *AnnouncementsSyncJob) Run(ctx context.Context) error {
	j.log.Info("starting announcements sync job")

	url := fmt.Sprintf("%s/api/anime/announcements-sync", j.config.CatalogServiceURL)
	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("announcements sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("announcements-sync returned status %d: %s", resp.StatusCode, string(body))
	}

	var result announcementsSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}
	j.log.Infow("announcements sync completed",
		"imported", result.Data.Imported,
		"refreshed", result.Data.Refreshed,
		"enriched", result.Data.Enriched,
		"failed", result.Data.Failed,
	)
	return nil
}
