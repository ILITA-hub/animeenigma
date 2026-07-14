// Package jobs — fanfic_daily.go fires the daily fanfic spotlight generation.
// The fanfic service (services/fanfic, port 8097) owns the Groq client + the
// idempotency check for "has today's spotlight already been generated", so
// this job just POSTs its /internal/fanfic/ensure-daily endpoint. Mirrors
// subtitle_probe_trigger.go (same HTTP-trigger pattern), but targets the
// fanfic service instead of catalog.
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

// fanficDailyReqTimeout caps the POST. ensure-daily runs a synchronous Groq
// generation that can take tens of seconds; the fanfic service's own Groq
// timeout is 120s, so 3 minutes gives comfortable margin.
const fanficDailyReqTimeout = 3 * time.Minute

// FanficDailyJob triggers the fanfic service's daily spotlight ensure-daily
// endpoint.
type FanficDailyJob struct {
	client *http.Client
	config *config.JobsConfig
	log    *logger.Logger
}

func NewFanficDailyJob(cfg *config.JobsConfig, log *logger.Logger) *FanficDailyJob {
	return &FanficDailyJob{
		client: &http.Client{Timeout: fanficDailyReqTimeout},
		config: cfg,
		log:    log,
	}
}

// Run POSTs the fanfic service's /internal/fanfic/ensure-daily. A non-2xx or
// transport error is returned so the JobService metrics wrapper records a
// failure; ensure-daily's own idempotency check lives on the fanfic side (the
// scheduler holds no cross-instance lock), so a retried or missed run is safe.
func (j *FanficDailyJob) Run(ctx context.Context) error {
	if j.log != nil {
		j.log.Info("starting fanfic daily ensure-daily trigger")
	}
	url := j.config.FanficServiceURL + "/internal/fanfic/ensure-daily"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build fanfic ensure-daily request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("post fanfic ensure-daily: %w", err)
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("fanfic ensure-daily returned status %d", resp.StatusCode)
	}
	if j.log != nil {
		j.log.Info("fanfic daily ensure-daily trigger completed")
	}
	return nil
}
