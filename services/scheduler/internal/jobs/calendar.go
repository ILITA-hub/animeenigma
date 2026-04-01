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

type CalendarSyncJob struct {
	config *config.JobsConfig
	client *http.Client
	log    *logger.Logger
}

type calendarSyncResponse struct {
	Data struct {
		Imported int `json:"imported"`
		Updated  int `json:"updated"`
		Failed   int `json:"failed"`
		Total    int `json:"total"`
	} `json:"data"`
}

func NewCalendarSyncJob(config *config.JobsConfig, log *logger.Logger) *CalendarSyncJob {
	return &CalendarSyncJob{
		config: config,
		client: &http.Client{
			Timeout: 600 * time.Second, // Calendar sync can be slow (rate-limited)
		},
		log: log,
	}
}

// Run executes the calendar sync job by calling the catalog calendar-sync endpoint.
func (j *CalendarSyncJob) Run(ctx context.Context) error {
	j.log.Info("starting calendar sync job")

	url := fmt.Sprintf("%s/api/anime/calendar-sync", j.config.CatalogServiceURL)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("calendar sync request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("calendar-sync returned status %d: %s", resp.StatusCode, string(body))
	}

	var result calendarSyncResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	j.log.Infow("calendar sync completed",
		"imported", result.Data.Imported,
		"updated", result.Data.Updated,
		"failed", result.Data.Failed,
	)

	return nil
}
