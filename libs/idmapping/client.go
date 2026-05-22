package idmapping

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

const (
	defaultBaseURL = "https://arm.haglund.dev/api/v2"

	// aniListGraphQL is the public AniList GraphQL endpoint. Used as a
	// fallback for MAL → AniList ID resolution when ARM is unreachable.
	// AniList's Media query supports `idMal: <int>` directly, which is
	// the exact mapping we need for miruro and the catalog's Jimaku
	// integration. AniList does NOT expose AniDB/Kitsu/LiveChart/IMDB,
	// so those fields stay nil when this fallback fires — callers that
	// rely on those fields (catalog's Kitsu mappings) will see nil and
	// handle gracefully.
	aniListGraphQL = "https://graphql.anilist.co"

	// armTimeout is the per-request timeout for ARM. Tightened from the
	// historical 10s after AUTO-139 confirmed the prior failure mode
	// (IPv6 blackhole) was a misdiagnosis — the actual ARM origin
	// hangs at the application layer, dragging callers for the full
	// 10s before fallback. 3s is generous for ARM's normal sub-500ms
	// response time and small enough that the AniList fallback (~200ms)
	// gives a snappy total experience even when ARM is sick.
	armTimeout = 3 * time.Second

	// aniListTimeout is the per-request timeout for the AniList GraphQL
	// fallback. AniList responds in ~150-250ms normally; 5s is the
	// outer bound for transient slowness.
	aniListTimeout = 5 * time.Second
)

// MappingResult holds the ID mapping response from ARM (anime-relations mapping).
//
// Only the AniList and MAL fields are guaranteed to be populated when the
// AniList GraphQL fallback fires (ARM unreachable). AniDB/Kitsu/LiveChart/
// IMDB are ARM-exclusive — callers that depend on them must handle the
// nil case (they already do for the "ARM has no mapping" 404 case, which
// is indistinguishable to the consumer).
type MappingResult struct {
	AniList   *int    `json:"anilist"`
	MAL       *int    `json:"myanimelist"`
	AniDB     *int    `json:"anidb"`
	Kitsu     *int    `json:"kitsu"`
	LiveChart *int    `json:"livechart"`
	IMDB      *string `json:"imdb"`
}

// Client interacts with the ARM anime ID mapping API (arm.haglund.dev).
// On ARM failure or partial-result (AniList ID missing), it falls back to
// the AniList GraphQL API to recover at least the AniList ID — which is
// the field every downstream caller in this codebase actually requires.
type Client struct {
	httpClient     *http.Client
	baseURL        string
	aniListBaseURL string
}

// NewClient creates a new ARM mapping client with the AniList GraphQL
// fallback enabled. The HTTP transport forces IPv4 because Docker
// container egress has no IPv6 route — without this, the default dialer
// prefers IPv6 and blackholes until the timeout (the historical AUTO-139
// issue). The IPv4 fix remains in place even though it did not, on its
// own, address the underlying ARM-origin-hang failure mode that the
// AniList fallback now papers over.
func NewClient() *Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp4", addr)
		},
	}
	return &Client{
		httpClient: &http.Client{
			// Per-request timeouts are enforced via context.WithTimeout
			// in the resolve* helpers; this outer timeout is a defensive
			// upper bound covering the slowest acceptable combined path
			// (ARM timeout + AniList fallback).
			Timeout:   armTimeout + aniListTimeout + 2*time.Second,
			Transport: transport,
		},
		baseURL:        defaultBaseURL,
		aniListBaseURL: aniListGraphQL,
	}
}

// ResolveByShikimoriID resolves anime IDs from a Shikimori ID.
// Shikimori IDs equal MAL IDs, so we query with source=myanimelist.
// Falls back to AniList GraphQL on ARM failure.
func (c *Client) ResolveByShikimoriID(id string) (*MappingResult, error) {
	return c.resolveMAL(id)
}

// ResolveByMALID resolves anime IDs from a MyAnimeList ID.
// Falls back to AniList GraphQL on ARM failure.
func (c *Client) ResolveByMALID(id string) (*MappingResult, error) {
	return c.resolveMAL(id)
}

// resolveMAL is the merged ARM → AniList resolution path used by both
// ResolveBy* entry points. Strategy:
//
//  1. Try ARM first (with a 3s timeout). On success with a non-nil
//     AniList ID, return immediately — ARM gives the richer result
//     (AniDB/Kitsu/LiveChart/IMDB).
//
//  2. If ARM erred OR returned a result with AniList == nil, query
//     AniList GraphQL. On success, merge AniList + MAL into whatever
//     partial result ARM produced and return.
//
//  3. If both fail, return the ARM error (wrapped with the AniList
//     failure message so operators see the full picture). The maintenance
//     bot keys on "ARM" in the error message to dispatch the right
//     pattern (see .claude/maintenance-prompt.md — ARM-down pattern).
func (c *Client) resolveMAL(id string) (*MappingResult, error) {
	if id == "" {
		return nil, errors.New("idmapping: empty ID")
	}

	armResult, armErr := c.resolveARM("myanimelist", id)
	if armErr == nil && armResult != nil && armResult.AniList != nil {
		return armResult, nil
	}

	// Fallback path: AniList GraphQL.
	fb, fbErr := c.resolveAniList(id)
	if fbErr != nil {
		if armErr != nil {
			// Both layers failed — preserve the ARM error for operator
			// triage but mention the fallback also failed.
			return nil, fmt.Errorf("%w (AniList fallback also failed: %s)", armErr, fbErr.Error())
		}
		// ARM succeeded with no AniList ID, AniList fallback also can't
		// help. Return the (incomplete) ARM result so callers that
		// only need AniDB/Kitsu/etc. still get something.
		return armResult, nil
	}

	// Fallback returned (nil, nil) — AniList knows no Media with this
	// MAL ID. Return ARM's partial result (or nil) gracefully; both
	// sources genuinely lack the mapping.
	if fb == nil {
		return armResult, nil
	}

	// Fallback succeeded. Merge the AniList ID (and MAL, in case ARM
	// returned nil) into whatever ARM produced.
	if armResult == nil {
		armResult = &MappingResult{}
	}
	armResult.AniList = fb.AniList
	if armResult.MAL == nil {
		armResult.MAL = fb.MAL
	}
	return armResult, nil
}

// resolveARM is the original ARM HTTP path, unchanged in shape but with
// a tighter timeout and explicit context handling so the per-request
// budget is honored even if the outer client timeout is bumped later.
func (c *Client) resolveARM(source, id string) (*MappingResult, error) {
	ctx, cancel := context.WithTimeout(context.Background(), armTimeout)
	defer cancel()

	endpoint := fmt.Sprintf("%s/ids?source=%s&id=%s",
		c.baseURL, url.QueryEscape(source), url.QueryEscape(id))

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("ARM build request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ARM request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // No mapping found
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ARM returned status %d: %s", resp.StatusCode, string(body))
	}

	var result MappingResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ARM decode response: %w", err)
	}

	return &result, nil
}

// aniListGraphQLResponse mirrors the JSON shape returned by AniList for
// the Media query. `id` is the AniList numeric ID; `idMal` echoes back
// the MAL ID we queried with.
type aniListGraphQLResponse struct {
	Data struct {
		Media *struct {
			ID    int `json:"id"`
			IDMAL int `json:"idMal"`
		} `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// resolveAniList queries AniList GraphQL for the AniList ID corresponding
// to the given MAL ID. Returns:
//   - (result, nil)       — AniList found a Media with the MAL ID
//   - (nil, nil)          — AniList knows no Media with this MAL ID
//   - (nil, error)        — transport / JSON / GraphQL error
//
// AniList's GraphQL Media query supports `idMal: Int` directly, which is
// what we need. No auth required for public reads.
func (c *Client) resolveAniList(malID string) (*MappingResult, error) {
	intID, perr := strconv.Atoi(malID)
	if perr != nil {
		return nil, fmt.Errorf("AniList: invalid MAL id %q: %w", malID, perr)
	}

	ctx, cancel := context.WithTimeout(context.Background(), aniListTimeout)
	defer cancel()

	body := fmt.Sprintf(
		`{"query":"query($mal:Int){Media(idMal:$mal,type:ANIME){id idMal}}","variables":{"mal":%d}}`,
		intID,
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.aniListBaseURL, bytes.NewBufferString(body))
	if err != nil {
		return nil, fmt.Errorf("AniList build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("AniList request failed: %w", err)
	}
	defer resp.Body.Close()

	respBytes, rerr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if rerr != nil {
		return nil, fmt.Errorf("AniList read body: %w", rerr)
	}

	// 404 / 4xx → no mapping (AniList returns 200 even for "no Media"; a
	// non-200 here usually means rate-limiting or upstream blip).
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AniList HTTP %d: %s", resp.StatusCode, truncate(string(respBytes), 200))
	}

	var parsed aniListGraphQLResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("AniList decode response: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("AniList GraphQL error: %s", parsed.Errors[0].Message)
	}
	if parsed.Data.Media == nil || parsed.Data.Media.ID == 0 {
		return nil, nil // No mapping known to AniList.
	}

	aniListID := parsed.Data.Media.ID
	malIDEcho := parsed.Data.Media.IDMAL
	return &MappingResult{
		AniList: &aniListID,
		MAL:     &malIDEcho,
	}, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
