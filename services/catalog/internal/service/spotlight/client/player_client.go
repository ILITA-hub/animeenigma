// Workstream hero-spotlight v1.0 Phase 3 — Plan 02 Task 3.
//
// PlayerClient is a thin HTTP wrapper around the catalog → player fan-out
// the spotlight aggregator needs:
//
//	FetchListByStatuses → GET http://player:8083/internal/users/{id}/list?status=...
//	NO JWT — the /internal/* route is docker-network-only (gateway does
//	not proxy /internal/*). Passing a JWT here would be ineffective on
//	the route and a needless secret leak.
//
// Pattern mirror of services/catalog/internal/service/spotlight/client/web_client.go
// (Phase 1's HTTP client to the web container).
//
// FetchUserRecs moved to RecsClient on 2026-07-17 (Task 9) — the
// /api/users/recs routes migrated player→recs on 2026-06-11 and this
// client had kept calling the stale player:8083 route. See recs_client.go.

package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// defaultPlayerBaseURL is the docker-network DNS name + port the catalog
// service uses to reach the player container.
const defaultPlayerBaseURL = "http://player:8083"

// defaultPlayerTimeout is tighter than the spotlight aggregator's 800ms
// per-card budget (HSB-BE-03), so the HTTP transport surfaces a fast
// failure BEFORE the resolver's parent context deadline trips. Pitfall 8
// from 01-RESEARCH.md — never trust the outer ctx to cancel a runaway
// transport; cap the transport itself.
const defaultPlayerTimeout = 700 * time.Millisecond

// InternalListItem mirrors services/player/internal/domain.InternalListItem.
// Kept LOCAL to the catalog client so we do not import player's domain
// package — that would create an awkward cross-service module dependency
// just for one struct's JSON tags. The struct is part of the inter-service
// wire contract; if player extends it, this struct extends in lockstep.
type InternalListItem struct {
	AnimeID            string `json:"anime_id"`
	Name               string `json:"name,omitempty"`
	NameRU             string `json:"name_ru,omitempty"`
	PosterURL          string `json:"poster_url,omitempty"`
	EpisodesAired      int    `json:"episodes_aired,omitempty"`
	EpisodesCount      int    `json:"episodes_count,omitempty"`
	Status             string `json:"status"`
	LastWatchedEpisode int    `json:"last_watched_episode,omitempty"`
	UpdatedAt          string `json:"updated_at,omitempty"`
}

// internalListResponse is the player's wire format for
// GET /internal/users/{id}/list. The internal endpoint writes a bare
// envelope (no httputil.OK wrapping — see player's list_internal.go).
type internalListResponse struct {
	Items []InternalListItem `json:"items"`
}

// PlayerClient fans out HTTP calls to the player service.
type PlayerClient struct {
	baseURL string
	http    *http.Client
	log     *logger.Logger
}

// NewPlayerClient constructs a PlayerClient. Empty baseURL → "http://player:8083".
// Nil hc → an http.Client with the 700ms default Timeout. log MUST be non-nil
// (production wires the same *logger.Logger the rest of catalog uses).
func NewPlayerClient(baseURL string, hc *http.Client, log *logger.Logger) *PlayerClient {
	if baseURL == "" {
		baseURL = defaultPlayerBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultPlayerTimeout}
	}
	return &PlayerClient{baseURL: baseURL, http: hc, log: log}
}

// BaseURL returns the configured base URL — exported solely for tests.
func (c *PlayerClient) BaseURL() string {
	return c.baseURL
}

// FetchListByStatuses calls GET {baseURL}/internal/users/{userID}/list?status={csv}.
// NO Authorization header — the /internal/* route lives on the docker-network
// trust boundary and the gateway does not proxy it.
//
// Empty/nil statuses short-circuits with an empty result and no HTTP call.
// The user_id path component is URL-escaped so callers can safely pass IDs
// with reserved characters (though production UUIDs do not have any).
func (c *PlayerClient) FetchListByStatuses(ctx context.Context, userID string, statuses []string) ([]InternalListItem, error) {
	if len(statuses) == 0 {
		return []InternalListItem{}, nil
	}
	endpoint := fmt.Sprintf("%s/internal/users/%s/list", c.baseURL, url.PathEscape(userID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("player_client.list: build request: %w", err)
	}
	q := req.URL.Query()
	q.Set("status", strings.Join(statuses, ","))
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		if c.log != nil {
			c.log.Warnw("player_client.list.transport_failed", "url", endpoint, "user_id", userID, "error", err)
		}
		return nil, fmt.Errorf("player_client.list: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if c.log != nil {
			c.log.Warnw("player_client.list.bad_status", "url", endpoint, "user_id", userID, "status", resp.StatusCode)
		}
		return nil, fmt.Errorf("player_client.list: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env internalListResponse
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		if c.log != nil {
			c.log.Warnw("player_client.list.decode_failed", "url", endpoint, "user_id", userID, "error", err)
		}
		return nil, fmt.Errorf("player_client.list: decode: %w", err)
	}
	if env.Items == nil {
		return []InternalListItem{}, nil
	}
	return env.Items, nil
}
