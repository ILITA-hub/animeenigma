package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

const verifyFetchTimeout = 3 * time.Second

// VerifySource returns per-provider content-verify rollups; best-effort,
// never errors (nil map on any failure).
type VerifySource interface {
	Summaries(ctx context.Context, animeID string) map[string]domain.VerifySummary
	RawVerdicts(ctx context.Context, animeID string) (json.RawMessage, error)
	Hint(animeID, visitor, source string)
}

// VerifyClient talks to the content-verify service (:8101). enabled=false
// (kill switch) makes every method a no-op.
type VerifyClient struct {
	base    string
	client  *http.Client
	enabled bool
}

// NewVerifyClient builds the client. baseURL is the content-verify service
// root (e.g. "http://content-verify:8101"), no trailing slash.
func NewVerifyClient(baseURL string, enabled bool) *VerifyClient {
	return &VerifyClient{base: baseURL, client: &http.Client{Timeout: verifyFetchTimeout}, enabled: enabled}
}

type verifyWire struct {
	AnimeID   string `json:"anime_id"`
	Providers []struct {
		Provider string               `json:"provider"`
		Summary  domain.VerifySummary `json:"summary"`
	} `json:"providers"`
}

// RawVerdicts returns the verbatim data payload for the public passthrough.
func (c *VerifyClient) RawVerdicts(ctx context.Context, animeID string) (json.RawMessage, error) {
	if c == nil || !c.enabled {
		return nil, fmt.Errorf("content-verify disabled")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/internal/verify/verdicts?anime_id="+url.QueryEscape(animeID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("content-verify -> %d", resp.StatusCode)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

// Summaries fetches and decodes the per-provider rollup map. Any failure
// (disabled, transport error, non-200, bad JSON) degrades to a nil map so
// capability assembly is never blocked on content-verify availability.
func (c *VerifyClient) Summaries(ctx context.Context, animeID string) map[string]domain.VerifySummary {
	raw, err := c.RawVerdicts(ctx, animeID)
	if err != nil {
		return nil
	}
	var wire verifyWire
	if json.Unmarshal(raw, &wire) != nil {
		return nil
	}
	out := make(map[string]domain.VerifySummary, len(wire.Providers))
	for _, p := range wire.Providers {
		out[p.Provider] = p.Summary
	}
	return out
}

// Hint fires a fire-and-forget visit signal (own ctx — the caller's request
// context ends before the POST would finish).
func (c *VerifyClient) Hint(animeID, visitor, source string) {
	if c == nil || !c.enabled || animeID == "" || visitor == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), verifyFetchTimeout)
		defer cancel()
		body, _ := json.Marshal(map[string]string{"anime_id": animeID, "visitor": visitor, "source": source})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/internal/verify/hint", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := c.client.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
}
