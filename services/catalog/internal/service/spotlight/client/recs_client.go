// RecsClient is the catalog → recs HTTP fan-out for spotlight resolvers.
//
// FetchUserRecs moved here from PlayerClient on 2026-07-17: the
// /api/users/recs routes migrated player→recs on 2026-06-11 (extraction),
// but the spotlight client kept calling player:8083 — the personalized
// personal_pick path 404'd behind auth and silently fell back to trending.
// Pointing the client at recs:8094 restores personalization.
//
// FetchUpcoming is new (Task 10 — the upcoming_for_you card): GET
// {baseURL}/api/users/recs/upcoming.
//
// T-03-05 carry-over: JWT values MUST NEVER appear in log lines. Error
// paths log a structured event with status/url/error only — no token field.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

// defaultRecsBaseURL is the docker-network DNS name + port of the recs service.
const defaultRecsBaseURL = "http://recs:8094"

// defaultRecsTimeout mirrors defaultPlayerTimeout: tighter than the
// aggregator's 800ms per-card budget so transport failures surface first.
const defaultRecsTimeout = 700 * time.Millisecond

// UserRec is one row from recs' /api/users/recs response. Anime is a
// json.RawMessage so the resolver can forward the payload verbatim into the
// spotlight Card without re-shaping (player/recs and catalog share the same
// AnimeCard frontend renderer).
type UserRec struct {
	Anime json.RawMessage `json:"anime"`
	Score float64         `json:"score,omitempty"`
}

// userRecsEnvelope is recs' wire format for GET /api/users/recs. It wraps
// the RecsEnvelope inside libs/httputil.OK, which produces
// {success: bool, data: {...}}. We only care about data.recs.
type userRecsEnvelope struct {
	Data struct {
		Recs []UserRec `json:"recs"`
	} `json:"data"`
}

// UpcomingWireItem is one announced-title match from
// GET /api/users/recs/upcoming. Anime and Reason are json.RawMessage so the
// resolver forwards recs' payloads verbatim into the spotlight Card without
// re-shaping (same pattern as UserRec.Anime).
type UpcomingWireItem struct {
	Anime      json.RawMessage `json:"anime"`
	MatchScore float64         `json:"match_score"`
	Reason     json.RawMessage `json:"reason"`
}

// upcomingEnvelope is recs' wire format ({success, data:{items}}).
type upcomingEnvelope struct {
	Data struct {
		Items []UpcomingWireItem `json:"items"`
	} `json:"data"`
}

// RecsClient fans out HTTP calls to the recs service.
type RecsClient struct {
	baseURL string
	http    *http.Client
	log     *logger.Logger
}

// NewRecsClient constructs a RecsClient. Empty baseURL → "http://recs:8094".
// Nil hc → an http.Client with the 700ms default Timeout. log may be nil
// — call sites are nil-guarded; production passes catalog's shared logger.
func NewRecsClient(baseURL string, hc *http.Client, log *logger.Logger) *RecsClient {
	if baseURL == "" {
		baseURL = defaultRecsBaseURL
	}
	if hc == nil {
		hc = &http.Client{Timeout: defaultRecsTimeout}
	}
	return &RecsClient{baseURL: baseURL, http: hc, log: log}
}

// BaseURL returns the configured base URL — exported solely for tests.
func (c *RecsClient) BaseURL() string {
	return c.baseURL
}

// FetchUserRecs calls GET {baseURL}/api/users/recs. When jwt is non-empty,
// it is forwarded in Authorization: Bearer <jwt> so recs' OptionalAuth
// picks the personalized recs.upNext row. When jwt is empty, the header is
// omitted entirely and recs serves the shared recs.trending row.
//
// Returns up to the service's row-slice cap on the logged-in path or 20 on
// the anon path — caller is responsible for adapting/slicing further
// (spotlight resolver applies the 1-2-3 rule via AdaptiveSlice).
//
// T-03-05: the jwt value is NEVER written to logs. Error paths log status
// code, URL, and the wrapped error only.
func (c *RecsClient) FetchUserRecs(ctx context.Context, jwt string) ([]UserRec, error) {
	endpoint := c.baseURL + "/api/users/recs"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("recs_client.user_recs: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		// Do NOT log the jwt value — T-03-05.
		if c.log != nil {
			c.log.Warnw("recs_client.user_recs.transport_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("recs_client.user_recs: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		// Read up to 512 bytes for the error message — no body dump in logs.
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if c.log != nil {
			c.log.Warnw("recs_client.user_recs.bad_status", "url", endpoint, "status", resp.StatusCode)
		}
		return nil, fmt.Errorf("recs_client.user_recs: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env userRecsEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		if c.log != nil {
			c.log.Warnw("recs_client.user_recs.decode_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("recs_client.user_recs: decode: %w", err)
	}
	return env.Data.Recs, nil
}

// FetchUpcoming calls GET {baseURL}/api/users/recs/upcoming with the
// caller's JWT (required — the endpoint 401s anonymous callers; resolvers
// must not call this without a JWT).
func (c *RecsClient) FetchUpcoming(ctx context.Context, jwt string) ([]UpcomingWireItem, error) {
	endpoint := c.baseURL + "/api/users/recs/upcoming"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("recs_client.upcoming: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}

	resp, err := c.http.Do(req)
	if err != nil {
		if c.log != nil {
			c.log.Warnw("recs_client.upcoming.transport_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("recs_client.upcoming: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		if c.log != nil {
			c.log.Warnw("recs_client.upcoming.bad_status", "url", endpoint, "status", resp.StatusCode)
		}
		return nil, fmt.Errorf("recs_client.upcoming: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var env upcomingEnvelope
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		if c.log != nil {
			c.log.Warnw("recs_client.upcoming.decode_failed", "url", endpoint, "error", err)
		}
		return nil, fmt.Errorf("recs_client.upcoming: decode: %w", err)
	}
	return env.Data.Items, nil
}
