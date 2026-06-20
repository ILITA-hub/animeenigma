// Package jobs — probe_trigger.go is the Phase A daily trigger for the
// playback-health analytics probe. The scheduler has no ClickHouse connection,
// so this job POSTs analytics' /internal/probe/run endpoint; analytics runs
// the full catalog-signed resolve → HLS proxy validation chain and records
// the results. Mirrors ProviderRankingJob exactly (same HTTP-trigger pattern).
package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

// probeTriggerReqTimeout caps the probe POST. The analytics handler runs
// catalog-signed resolve → HLS proxy for each provider, which is slower than
// a pure ClickHouse aggregate; 5 minutes is comfortable for a full provider
// sweep.
const probeTriggerReqTimeout = 5 * time.Minute

// ProbeTriggerJob triggers the analytics daily playback-health probe.
type ProbeTriggerJob struct {
	client *http.Client
	config *config.JobsConfig
	log    *logger.Logger
}

func NewProbeTriggerJob(cfg *config.JobsConfig, log *logger.Logger) *ProbeTriggerJob {
	return &ProbeTriggerJob{
		client: &http.Client{Timeout: probeTriggerReqTimeout},
		config: cfg,
		log:    log,
	}
}

// Run POSTs analytics' /internal/probe/run. A non-2xx or transport error is
// returned so the JobService metrics wrapper records a failure; a single
// missed run is tolerated (the probe results carry their own TTL in analytics).
func (j *ProbeTriggerJob) Run(ctx context.Context) error {
	if j.log != nil {
		j.log.Info("starting playback-health probe trigger")
	}
	url := j.config.AnalyticsInternalURL + "/internal/probe/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("post probe: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("probe returned status %d", resp.StatusCode)
	}
	if j.log != nil {
		j.log.Info("playback-health probe trigger completed")
	}
	return nil
}
