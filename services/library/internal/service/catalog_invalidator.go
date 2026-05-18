package service

// Phase 06 (workstream raw-jp / v0.2). Best-effort webhook fired by
// the library encoder worker after every successful job so the
// catalog's hybrid raw resolver re-fetches the source decision
// instead of waiting out the 1h TTL.
//
// Failure handling is asymmetric: the webhook is fire-and-forget by
// design — encoding succeeded, the catalog will pick up the new
// row within the 1h TTL even if invalidation fails. We log + count
// the failure but never propagate it to the caller.

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// CatalogInvalidator is the interface the encoder worker depends on.
// Tests inject a fake that records the (shikimoriID) arguments; the
// production *HTTPCatalogInvalidator (or *noopInvalidator) satisfies
// the interface.
type CatalogInvalidator interface {
	Invalidate(ctx context.Context, shikimoriID string)
}

// InvalidatorConfig drives the HTTPCatalogInvalidator. An empty
// CatalogInternalAPIURL switches the constructor to a no-op
// implementation — the encoder remains correct, only the cache-bust
// fast-path is skipped.
type InvalidatorConfig struct {
	CatalogInternalAPIURL string
	Timeout               time.Duration
}

// InvalidationMetrics is the slice of *metrics.LibraryMetrics that
// the invalidator depends on. Declared locally so the service
// package does not import the metrics package directly — keeps the
// dependency arrow service → metrics implicit, mirrors how
// EncodeMetrics is structured.
type InvalidationMetrics interface {
	IncCacheInvalidation(result string)
}

// HTTPCatalogInvalidator is the real implementation. POSTs to
// {base}/internal/cache/invalidate/raw/{shikimoriID} with an empty
// body. 2xx → "ok" metric; everything else (non-2xx, transport
// error, timeout) → "fail" metric. Never returns an error to the
// caller and never blocks longer than the configured Timeout.
type HTTPCatalogInvalidator struct {
	cfg        InvalidatorConfig
	httpClient *http.Client
	metrics    InvalidationMetrics
	log        *logger.Logger
}

// noopInvalidator is returned when CatalogInternalAPIURL is empty —
// the encoder worker still runs, but the webhook fast-path is
// skipped. The 1h TTL covers correctness.
type noopInvalidator struct{}

func (noopInvalidator) Invalidate(_ context.Context, _ string) {}

// NewCatalogInvalidator constructs a CatalogInvalidator. When
// cfg.CatalogInternalAPIURL is empty, returns a no-op invalidator
// and logs a single info line so the operator sees the configuration
// gap at startup.
func NewCatalogInvalidator(cfg InvalidatorConfig, m InvalidationMetrics, log *logger.Logger) CatalogInvalidator {
	if cfg.CatalogInternalAPIURL == "" {
		if log != nil {
			log.Infow("catalog invalidator disabled (CATALOG_INTERNAL_API_URL empty); 1h TTL covers correctness")
		}
		return noopInvalidator{}
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Second
	}
	cfg.CatalogInternalAPIURL = strings.TrimRight(cfg.CatalogInternalAPIURL, "/")
	return &HTTPCatalogInvalidator{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		metrics: m,
		log:     log,
	}
}

// Invalidate fires the webhook. Best-effort: errors are logged + counted
// via library_cache_invalidation_total{result="fail"}, never returned.
// The caller's ctx + the invalidator's own Timeout both bound the
// request — caller cancellation propagates correctly.
func (i *HTTPCatalogInvalidator) Invalidate(ctx context.Context, shikimoriID string) {
	if shikimoriID == "" {
		// Defensive — encoder worker already gates on this.
		return
	}

	reqCtx, cancel := context.WithTimeout(ctx, i.cfg.Timeout)
	defer cancel()

	u := fmt.Sprintf("%s/internal/cache/invalidate/raw/%s",
		i.cfg.CatalogInternalAPIURL,
		url.PathEscape(shikimoriID),
	)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, u, nil)
	if err != nil {
		i.recordFail("build request", shikimoriID, err)
		return
	}

	resp, err := i.httpClient.Do(req)
	if err != nil {
		i.recordFail("do request", shikimoriID, err)
		return
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		i.recordFail(fmt.Sprintf("non-2xx %d", resp.StatusCode), shikimoriID, nil)
		return
	}

	if i.metrics != nil {
		i.metrics.IncCacheInvalidation("ok")
	}
	if i.log != nil {
		i.log.Infow("cache invalidation ok",
			"shikimori_id", shikimoriID,
			"status", resp.StatusCode,
		)
	}
}

func (i *HTTPCatalogInvalidator) recordFail(stage, shikimoriID string, err error) {
	if i.metrics != nil {
		i.metrics.IncCacheInvalidation("fail")
	}
	if i.log != nil {
		i.log.Warnw("cache invalidation failed",
			"shikimori_id", shikimoriID,
			"stage", stage,
			"error", err,
		)
	}
}
