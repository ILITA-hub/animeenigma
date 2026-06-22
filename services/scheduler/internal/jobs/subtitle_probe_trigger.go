// Package jobs — subtitle_probe_trigger.go fires the active subtitle-provider
// health probe. The probe lives in catalog (it owns the Jimaku/OpenSubtitles
// clients + API keys), so this job POSTs catalog's /internal/subtitle-probe/run
// on a 5-min cron. Mirrors probe_trigger.go (same HTTP-trigger pattern), but
// targets catalog instead of analytics.
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

// subtitleProbeReqTimeout caps the POST. The probe is two cheap HTTP pings
// (8s budget each), so 30s is comfortable.
const subtitleProbeReqTimeout = 30 * time.Second

// SubtitleProbeTriggerJob triggers catalog's active subtitle health probe.
type SubtitleProbeTriggerJob struct {
	client *http.Client
	config *config.JobsConfig
	log    *logger.Logger
}

func NewSubtitleProbeTriggerJob(cfg *config.JobsConfig, log *logger.Logger) *SubtitleProbeTriggerJob {
	return &SubtitleProbeTriggerJob{
		client: &http.Client{Timeout: subtitleProbeReqTimeout},
		config: cfg,
		log:    log,
	}
}

// Run POSTs catalog's /internal/subtitle-probe/run. A non-2xx or transport error
// is returned so the JobService metrics wrapper records a failure; a single
// missed run is tolerated (health carries CheckedAt → stale downgrades to unknown).
func (j *SubtitleProbeTriggerJob) Run(ctx context.Context) error {
	if j.log != nil {
		j.log.Info("starting subtitle-health probe trigger")
	}
	url := j.config.CatalogServiceURL + "/internal/subtitle-probe/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build subtitle probe request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("post subtitle probe: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("subtitle probe returned status %d", resp.StatusCode)
	}
	if j.log != nil {
		j.log.Info("subtitle-health probe trigger completed")
	}
	return nil
}
