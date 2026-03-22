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

// batchRefreshResponse represents the response from the catalog batch-refresh endpoint
type batchRefreshResponse struct {
	Data struct {
		Refreshed int    `json:"refreshed"`
		Failed    int    `json:"failed"`
		Total     int    `json:"total"`
		Status    string `json:"status"`
	} `json:"data"`
}

func NewShikimoriSyncJob(config *config.JobsConfig, log *logger.Logger) *ShikimoriSyncJob {
	return &ShikimoriSyncJob{
		config: config,
		client: &http.Client{
			Timeout: 300 * time.Second,
		},
		log: log,
	}
}

// Run executes the Shikimori sync job using batch refresh for all anime statuses.
func (j *ShikimoriSyncJob) Run(ctx context.Context) error {
	j.log.Info("starting batch anime metadata sync")

	phases := []struct {
		status     string
		staleHours int
	}{
		{"ongoing", j.config.OngoingStaleHours},
		{"announced", j.config.AnnouncedStaleHours},
		{"released", j.config.ReleasedStaleHours},
	}

	var totalRefreshed, totalFailed int

	for _, phase := range phases {
		select {
		case <-ctx.Done():
			j.log.Warn("sync job cancelled")
			return ctx.Err()
		default:
		}

		j.log.Infow("starting sync phase", "status", phase.status, "stale_hours", phase.staleHours)

		result, err := j.batchRefresh(ctx, phase.status, phase.staleHours)
		if err != nil {
			j.log.Errorw("batch refresh phase failed",
				"status", phase.status,
				"error", err,
			)
			continue
		}

		j.log.Infow("sync phase completed",
			"status", phase.status,
			"refreshed", result.Data.Refreshed,
			"failed", result.Data.Failed,
		)

		totalRefreshed += result.Data.Refreshed
		totalFailed += result.Data.Failed
	}

	j.log.Infow("batch anime metadata sync completed",
		"total_refreshed", totalRefreshed,
		"total_failed", totalFailed,
	)

	return nil
}

// batchRefresh calls the catalog batch-refresh endpoint for a given status.
func (j *ShikimoriSyncJob) batchRefresh(ctx context.Context, status string, staleHours int) (*batchRefreshResponse, error) {
	url := fmt.Sprintf("%s/api/anime/batch-refresh?status=%s&stale_hours=%d",
		j.config.CatalogServiceURL, status, staleHours)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := j.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("batch-refresh returned status %d: %s", resp.StatusCode, string(body))
	}

	var result batchRefreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}
