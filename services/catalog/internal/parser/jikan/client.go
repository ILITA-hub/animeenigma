package jikan

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

// Client for the Jikan API (unofficial MAL REST API)
type Client struct {
	httpClient  *http.Client
	baseURL     string
	rateLimiter *rateLimiter
}

// AnimeInfo contains the title information from MAL
type AnimeInfo struct {
	MalID         int    `json:"mal_id"`
	Title         string `json:"title"`
	TitleEnglish  string `json:"title_english"`
	TitleJapanese string `json:"title_japanese"`
}

type rateLimiter struct {
	mu         sync.Mutex
	tokens     int
	maxTokens  int
	lastRefill time.Time
	interval   time.Duration
}

func newRateLimiter(rps int) *rateLimiter {
	return &rateLimiter{
		tokens:     rps,
		maxTokens:  rps,
		lastRefill: time.Now(),
		interval:   time.Second,
	}
}

func (rl *rateLimiter) acquire() {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	if elapsed >= rl.interval {
		rl.tokens = rl.maxTokens
		rl.lastRefill = now
	}

	if rl.tokens <= 0 {
		time.Sleep(rl.interval - elapsed)
		rl.tokens = rl.maxTokens
		rl.lastRefill = time.Now()
	}

	rl.tokens--
}

// NewClient creates a new Jikan API client
func NewClient() *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: 15 * time.Second},
		baseURL:     "https://api.jikan.moe/v4",
		rateLimiter: newRateLimiter(2),
	}
}

// GetAnimeByID fetches anime info from Jikan (MAL proxy)
func (c *Client) GetAnimeByID(ctx context.Context, malID string) (*AnimeInfo, error) {
	c.rateLimiter.acquire()

	url := fmt.Sprintf("%s/anime/%s", c.baseURL, malID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, fmt.Errorf("jikan: create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("jikan: request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == 404 {
		return nil, fmt.Errorf("jikan: MAL anime %s not found", malID)
	}
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("jikan: API returned %d", resp.StatusCode)
	}

	var result struct {
		Data AnimeInfo `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("jikan: decode response: %w", err)
	}

	return &result.Data, nil
}
