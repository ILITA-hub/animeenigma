// Package anilist is a thin GraphQL client for AniList's public read API.
//
// Phase 12 (Decision §A4) — used by the Wave-2 backfill tool to populate
// the new domain.Tag rows + anime_tags join entries that S5 TF-IDF will
// eventually score against. Decision §A4 ignores tag rank in v1 but the
// client surfaces Rank, Category, IsAdult, and IsGeneralSpoiler so the
// backfill (and a future v2.1 rank-weighted TF-IDF) can use them.
//
// Rate limited at ~1 rps (60/min) to stay well under AniList's 90/min
// unauthenticated cap. The Shikimori client's rateLimiter pattern is
// mirrored for structural consistency.
package anilist

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

const (
	defaultEndpoint  = "https://graphql.anilist.co"
	defaultUserAgent = "AnimeEnigma/1.0 (https://animeenigma.ru)"
	// 1 rps = 60 req/min — leaves 33% headroom under AniList's 90/min cap.
	defaultRateLimitRPS = 1
	defaultTimeout      = 10 * time.Second
)

// Tag is the parser-local representation of an AniList tag.
//
// Rank is preserved for v2.1 rank-weighted TF-IDF (Decision §A4 ignores
// rank in v1). Category / IsAdult / IsGeneralSpoiler are exposed so the
// Wave-2 backfill can filter on them (e.g. inherit IsAdult from
// animes.hidden); this client does not filter.
type Tag struct {
	Name             string
	Rank             int
	Category         string
	IsAdult          bool
	IsGeneralSpoiler bool
}

// Client is a thin AniList GraphQL client. Only FetchTags is exposed —
// the catalog service does NOT auto-call AniList during Shikimori fetches;
// only the Wave-2 backfill drives this client.
type Client struct {
	httpClient  *http.Client
	endpoint    string
	userAgent   string
	log         *logger.Logger
	rateLimiter *rateLimiter
}

// rateLimiter mirrors the Shikimori client's token-bucket implementation.
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

// NewClient constructs a production AniList client targeting the public
// graphql.anilist.co endpoint with the Phase-12 default rate limit.
func NewClient(log *logger.Logger) *Client {
	return NewClientWithBaseURL(defaultEndpoint, log)
}

// NewClientWithBaseURL allows tests to point the client at an httptest
// stub server while keeping the production User-Agent and rate limiter.
func NewClientWithBaseURL(endpoint string, log *logger.Logger) *Client {
	return &Client{
		httpClient:  &http.Client{Timeout: defaultTimeout},
		endpoint:    endpoint,
		userAgent:   defaultUserAgent,
		log:         log,
		rateLimiter: newRateLimiter(defaultRateLimitRPS),
	}
}

// fetchTagsQuery is the AniList GraphQL body. Whitespace-stable so the
// query-shape test can assert on substrings.
const fetchTagsQuery = `query ($id: Int) { Media(id: $id, type: ANIME) { tags { name rank category isAdult isGeneralSpoiler } } }`

// FetchTags returns AniList tags for the given anime by its AniList id.
//
// Decision §A4: rank is preserved on the returned Tag.Rank but the v1 S5
// signal ignores it. isAdult / isGeneralSpoiler are returned for caller
// filtering — this method does not filter.
//
// Errors are wrapped with errors.ExternalAPI("anilist", err) so callers
// can route them through the standard external-API error path.
func (c *Client) FetchTags(ctx context.Context, anilistID int) ([]Tag, error) {
	c.rateLimiter.acquire()

	reqBody := map[string]interface{}{
		"query":     fetchTagsQuery,
		"variables": map[string]interface{}{"id": anilistID},
	}
	jsonBody, err := json.Marshal(reqBody)
	if err != nil {
		return nil, errors.ExternalAPI("anilist", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", c.endpoint, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, errors.ExternalAPI("anilist", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI("anilist", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Media *struct {
				Tags []struct {
					Name             string `json:"name"`
					Rank             int    `json:"rank"`
					Category         string `json:"category"`
					IsAdult          bool   `json:"isAdult"`
					IsGeneralSpoiler bool   `json:"isGeneralSpoiler"`
				} `json:"tags"`
			} `json:"Media"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.ExternalAPI("anilist", err)
	}

	if len(result.Errors) > 0 {
		return nil, errors.ExternalAPI("anilist", fmt.Errorf("%s", result.Errors[0].Message))
	}

	if result.Data.Media == nil {
		return []Tag{}, nil
	}

	out := make([]Tag, 0, len(result.Data.Media.Tags))
	for _, t := range result.Data.Media.Tags {
		out = append(out, Tag{
			Name:             t.Name,
			Rank:             t.Rank,
			Category:         t.Category,
			IsAdult:          t.IsAdult,
			IsGeneralSpoiler: t.IsGeneralSpoiler,
		})
	}
	return out, nil
}

// slugifyRegex compiles once. Matches one or more non-alphanumeric ASCII
// characters; replaceAllString collapses each run to a single underscore.
var slugifyRegex = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyTagName converts an AniList tag name into a deterministic,
// lowercase, alphanumeric+underscore primary key. Used by the Wave-2
// backfill to populate domain.Tag.ID.
//
// Multiple consecutive non-alphanumeric characters collapse to a single
// underscore. Leading/trailing underscores are trimmed. Whitespace is
// trimmed before processing. The implementation does NOT strip
// diacritics — non-ASCII letters are treated as non-alphanumeric, so
// "Mahō Shōjo" → "mah_sh_jo". This is dependency-free and deterministic;
// if a future plan needs Unicode-aware normalization it can swap the
// regex without breaking idempotency for ASCII-only names.
//
// Examples:
//
//	"Slice of Life" -> "slice_of_life"
//	"Sci-Fi"        -> "sci_fi"
//	"A & B"         -> "a_b"
//	""              -> ""
//	"  Action  "    -> "action"
func slugifyTagName(name string) string {
	lower := strings.ToLower(strings.TrimSpace(name))
	slug := slugifyRegex.ReplaceAllString(lower, "_")
	slug = strings.Trim(slug, "_")
	return slug
}

// SlugifyTagName is the exported wrapper for slugifyTagName, used by the
// Wave-2 backfill to compute domain.Tag.ID consistently with this
// package's internal logic.
func SlugifyTagName(name string) string { return slugifyTagName(name) }
