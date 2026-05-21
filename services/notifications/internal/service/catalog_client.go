package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	apperrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/notifications/internal/domain"
)

// EpisodeChecker is the canonical port the Phase 2 detector depends on.
// Production = HTTPEpisodeChecker hitting catalog's
// /internal/anime/{shikimori_id}/episodes endpoint. Tests = stub returning
// canned latest-episode integers from a map[Combo]int fixture (D-DET-07).
type EpisodeChecker interface {
	LatestEpisode(ctx context.Context, combo domain.Combo) (int, error)
}

// EpisodeCheckerResponse mirrors the wire shape catalog's
// service.EpisodesLookupResult produces.
//
// IMPORTANT: catalog wraps every response in libs/httputil.JSON's
// {"success": bool, "data": {...}} envelope. The detector must unwrap
// `data` before reading the payload — this was caught in SC2 of the
// Phase 2 verification gauntlet where every snapshot persisted as 0
// because the unwrapped LatestAvailableEpisode field was always absent
// from the top-level JSON object.
type EpisodeCheckerResponse struct {
	Success bool                         `json:"success"`
	Data    EpisodeCheckerResponsePayload `json:"data"`
}

// EpisodeCheckerResponsePayload is the inner object catalog returns.
type EpisodeCheckerResponsePayload struct {
	LatestAvailableEpisode int       `json:"latest_available_episode"`
	CheckedAt              time.Time `json:"checked_at"`
}

// HTTPEpisodeChecker is the production EpisodeChecker. Per-call 10s timeout
// (NOTIFICATIONS_PARSER_TIMEOUT). Treats 404 as apperrors.NotFound so the
// detector can distinguish "combo has no current episode" (skip silently)
// from "catalog is broken" (count toward parser_failures metric).
type HTTPEpisodeChecker struct {
	baseURL string
	client  *http.Client
	log     *logger.Logger
}

// NewHTTPEpisodeChecker constructs the HTTP-backed checker.
//
// `baseURL` is the catalog service base URL — typically
// http://catalog:8081 from CATALOG_URL.
func NewHTTPEpisodeChecker(baseURL string, timeout time.Duration, log *logger.Logger) *HTTPEpisodeChecker {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &HTTPEpisodeChecker{
		baseURL: baseURL,
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}
}

// LatestEpisode hits GET /internal/anime/{shikimori_id}/episodes with the
// combo's player/translation_id/watch_type/language params and returns
// the parsed latest_available_episode.
func (c *HTTPEpisodeChecker) LatestEpisode(ctx context.Context, combo domain.Combo) (int, error) {
	if combo.ShikimoriID == "" {
		return 0, apperrors.InvalidInput("combo missing shikimori_id")
	}

	q := url.Values{}
	q.Set("player", combo.Player)
	q.Set("translation_id", combo.TranslationID)
	q.Set("watch_type", combo.WatchType)
	q.Set("language", combo.Language)

	endpoint := fmt.Sprintf("%s/internal/anime/%s/episodes?%s",
		c.baseURL, url.PathEscape(combo.ShikimoriID), q.Encode())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return 0, apperrors.Wrap(err, apperrors.CodeInternal, "build episodes request")
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		// Includes timeouts (context.DeadlineExceeded) and connection
		// failures (DNS, refused, etc).
		return 0, apperrors.Wrap(err, apperrors.CodeUnavailable, "catalog episodes request failed")
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		return 0, apperrors.Wrap(readErr, apperrors.CodeInternal, "read episodes response")
	}

	switch resp.StatusCode {
	case http.StatusOK:
		var parsed EpisodeCheckerResponse
		if err := json.Unmarshal(body, &parsed); err != nil {
			return 0, apperrors.Wrap(err, apperrors.CodeInternal, "decode episodes response")
		}
		return parsed.Data.LatestAvailableEpisode, nil
	case http.StatusNotFound:
		// Combo has no upstream match — treat as not-found, NOT failure.
		// Detector skips silently rather than logging a parser failure.
		return 0, apperrors.NotFound("episode for combo")
	case http.StatusBadRequest:
		return 0, apperrors.InvalidInput(fmt.Sprintf("catalog rejected episodes request: %s", string(body)))
	default:
		return 0, apperrors.New(apperrors.CodeUnavailable,
			fmt.Sprintf("catalog episodes returned %d: %s", resp.StatusCode, string(body)))
	}
}

// IsEpisodeNotFound returns true when the error is the catalog's
// "no upstream episode for this combo" signal. Detector uses this to
// distinguish "skip silently" (not-found) from "count as parser failure"
// (every other error).
func IsEpisodeNotFound(err error) bool {
	if err == nil {
		return false
	}
	var appErr *apperrors.AppError
	if errors.As(err, &appErr) {
		return appErr.Code == apperrors.CodeNotFound
	}
	return false
}
