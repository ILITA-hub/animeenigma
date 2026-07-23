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

// AniListAiring is the broadcaster airing schedule AniList exposes for a Media.
// Unlike Shikimori's naive "last + 1 week" estimate, AniList's nextAiringEpisode
// reflects the real broadcast schedule and models hiatuses. NextAiringAt is nil
// when AniList has no upcoming episode scheduled (e.g. FINISHED series).
type AniListAiring struct {
	AniListID     int                    // AniList Media.id
	Status        string                 // RELEASING | FINISHED | NOT_YET_RELEASED | CANCELLED | HIATUS
	NextEpisode   int                    // nextAiringEpisode.episode; 0 when none scheduled
	NextAiringAt  *time.Time             // nextAiringEpisode.airingAt (unix seconds → UTC); nil when none
	AiringHistory []AniListEpisodeAiring // Provider schedule; callers decide which past entries to persist.
}

// AniListEpisodeAiring is one episode timestamp from Media.airingSchedule.
type AniListEpisodeAiring struct {
	Episode  int
	AiringAt time.Time
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

// Option configures a Client at construction time.
type Option func(*Client)

// NewIPv4Transport builds the IPv4-forced http.Transport this client uses by
// default. It is exported so the owning service (e.g. catalog) can WRAP it
// with a recording transport and inject the result via WithTransport —
// keeping the IPv4 dialer behavior intact while adding egress recording.
// This avoids importing the tracing module into this dependency-free leaf
// module (RESEARCH §Pitfall 1 / T-02-LEAF).
func NewIPv4Transport() *http.Transport {
	return &http.Transport{
		DialContext: func(ctx context.Context, _, addr string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "tcp4", addr)
		},
	}
}

// WithTransport overrides the http.RoundTripper this client uses. The intended
// production use is to WRAP (not replace) the IPv4 transport — callers build
// NewIPv4Transport(), wrap it with tracing.WrapRecording(ipv4, sink), and pass
// the result here so outbound ARM/AniList requests emit one egress effect each
// while preserving the IPv4-forced dialer. Absent this option, NewClient()
// uses NewIPv4Transport() directly (back-compat, zero-arg).
func WithTransport(rt http.RoundTripper) Option {
	return func(c *Client) {
		if rt != nil {
			c.httpClient.Transport = rt
		}
	}
}

// NewClient creates a new ARM mapping client with the AniList GraphQL
// fallback enabled. The HTTP transport forces IPv4 because Docker
// container egress has no IPv6 route — without this, the default dialer
// prefers IPv6 and blackholes until the timeout (the historical AUTO-139
// issue). The IPv4 fix remains in place even though it did not, on its
// own, address the underlying ARM-origin-hang failure mode that the
// AniList fallback now papers over.
//
// Pass WithTransport to inject a recording-wrapped transport from the owning
// service (host-only egress effect per outbound request, D-08).
func NewClient(opts ...Option) *Client {
	c := &Client{
		httpClient: &http.Client{
			// Per-request timeouts are enforced via context.WithTimeout
			// in the resolve* helpers; this outer timeout is a defensive
			// upper bound covering the slowest acceptable combined path
			// (ARM timeout + AniList fallback).
			Timeout:   armTimeout + aniListTimeout + 2*time.Second,
			Transport: NewIPv4Transport(),
		},
		baseURL:        defaultBaseURL,
		aniListBaseURL: aniListGraphQL,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// ResolveByShikimoriID resolves anime IDs from a Shikimori ID.
// Shikimori IDs equal MAL IDs, so we query with source=myanimelist.
// Falls back to AniList GraphQL on ARM failure.
//
// Deprecated for in-process use: prefer ResolveByShikimoriIDContext so the
// caller's request context (trace span + egress attribution) threads into the
// outbound ARM/AniList calls (WR-01). This no-ctx wrapper is kept for
// backward compat and uses context.Background().
func (c *Client) ResolveByShikimoriID(id string) (*MappingResult, error) {
	return c.resolveMAL(context.Background(), id)
}

// ResolveByMALID resolves anime IDs from a MyAnimeList ID.
// Falls back to AniList GraphQL on ARM failure.
//
// Deprecated for in-process use: prefer ResolveByMALIDContext (WR-01). This
// no-ctx wrapper is kept for backward compat and uses context.Background().
func (c *Client) ResolveByMALID(id string) (*MappingResult, error) {
	return c.resolveMAL(context.Background(), id)
}

// ResolveByShikimoriIDContext is the context-aware variant of
// ResolveByShikimoriID (WR-01). The caller's ctx threads into the outbound
// ARM/AniList HTTP requests so the egress effects carry the inbound request's
// trace linkage + attribution; the per-request ARM/AniList timeouts are derived
// from ctx via context.WithTimeout(ctx, …), so a cancelled ctx aborts promptly.
func (c *Client) ResolveByShikimoriIDContext(ctx context.Context, id string) (*MappingResult, error) {
	return c.resolveMAL(ctx, id)
}

// ResolveByMALIDContext is the context-aware variant of ResolveByMALID (WR-01).
// See ResolveByShikimoriIDContext for the contract.
func (c *Client) ResolveByMALIDContext(ctx context.Context, id string) (*MappingResult, error) {
	return c.resolveMAL(ctx, id)
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
func (c *Client) resolveMAL(ctx context.Context, id string) (*MappingResult, error) {
	if id == "" {
		return nil, errors.New("idmapping: empty ID")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	armResult, armErr := c.resolveARM(ctx, "myanimelist", id)
	if armErr == nil && armResult != nil && armResult.AniList != nil {
		return armResult, nil
	}

	// Fallback path: AniList GraphQL.
	fb, fbErr := c.resolveAniList(ctx, id)
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
func (c *Client) resolveARM(ctx context.Context, source, id string) (*MappingResult, error) {
	// Derive the per-request budget from the caller's ctx so the inbound
	// request's trace span + egress attribution thread into the ARM call
	// (WR-01), while still capping the ARM hop at armTimeout.
	ctx, cancel := context.WithTimeout(ctx, armTimeout)
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

// aniListAiringResponse mirrors the JSON shape returned by AniList for the
// airing-schedule Media query.
type aniListAiringResponse struct {
	Data struct {
		Media *struct {
			ID                int    `json:"id"`
			Status            string `json:"status"`
			NextAiringEpisode *struct {
				Episode  int   `json:"episode"`
				AiringAt int64 `json:"airingAt"`
			} `json:"nextAiringEpisode"`
			AiringSchedule struct {
				Nodes []struct {
					Episode  int   `json:"episode"`
					AiringAt int64 `json:"airingAt"`
				} `json:"nodes"`
			} `json:"airingSchedule"`
		} `json:"Media"`
	} `json:"data"`
	Errors []struct {
		Message string `json:"message"`
	} `json:"errors,omitempty"`
}

// postAniListGraphQL sends a GraphQL query body to AniList and returns the raw
// response bytes. It owns the per-request timeout (aniListTimeout), JSON headers,
// body-size limit, and non-200 handling — so resolveAniList and
// AniListAiringByMALID speak to AniList through exactly one code path.
func (c *Client) postAniListGraphQL(ctx context.Context, body string) ([]byte, error) {
	// Per-request budget derived from the caller's ctx (WR-01); see resolveARM.
	ctx, cancel := context.WithTimeout(ctx, aniListTimeout)
	defer cancel()

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
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("AniList HTTP %d: %s", resp.StatusCode, truncate(string(respBytes), 200))
	}
	return respBytes, nil
}

// resolveAniList queries AniList GraphQL for the AniList ID corresponding
// to the given MAL ID. Returns:
//   - (result, nil)       — AniList found a Media with the MAL ID
//   - (nil, nil)          — AniList knows no Media with this MAL ID
//   - (nil, error)        — transport / JSON / GraphQL error
//
// AniList's GraphQL Media query supports `idMal: Int` directly, which is
// what we need. No auth required for public reads.
func (c *Client) resolveAniList(ctx context.Context, malID string) (*MappingResult, error) {
	intID, perr := strconv.Atoi(malID)
	if perr != nil {
		return nil, fmt.Errorf("AniList: invalid MAL id %q: %w", malID, perr)
	}

	body := fmt.Sprintf(
		`{"query":"query($mal:Int){Media(idMal:$mal,type:ANIME){id idMal}}","variables":{"mal":%d}}`,
		intID,
	)

	respBytes, err := c.postAniListGraphQL(ctx, body)
	if err != nil {
		return nil, err
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

// AniListAiringByMALID queries AniList for the broadcaster airing schedule by
// MAL/Shikimori id (Shikimori IDs equal MAL IDs). Returns:
//   - (result, nil) — AniList found a Media (NextAiringAt is nil if nothing is scheduled)
//   - (nil, nil)    — AniList knows no Media with this MAL id
//   - (nil, error)  — transport / JSON / GraphQL error
//
// AniList's Media query supports idMal:Int directly and returns
// nextAiringEpisode in the same call. No auth required for public reads.
func (c *Client) AniListAiringByMALID(ctx context.Context, malID string) (*AniListAiring, error) {
	intID, perr := strconv.Atoi(malID)
	if perr != nil {
		return nil, fmt.Errorf("AniList airing: invalid MAL id %q: %w", malID, perr)
	}

	body := fmt.Sprintf(
		`{"query":"query($mal:Int){Media(idMal:$mal,type:ANIME){id status nextAiringEpisode{episode airingAt} airingSchedule(page:1,perPage:50){nodes{episode airingAt}}}}","variables":{"mal":%d}}`,
		intID,
	)

	respBytes, err := c.postAniListGraphQL(ctx, body)
	if err != nil {
		return nil, err
	}

	var parsed aniListAiringResponse
	if err := json.Unmarshal(respBytes, &parsed); err != nil {
		return nil, fmt.Errorf("AniList airing decode: %w", err)
	}
	if len(parsed.Errors) > 0 {
		return nil, fmt.Errorf("AniList airing GraphQL error: %s", parsed.Errors[0].Message)
	}
	m := parsed.Data.Media
	if m == nil {
		return nil, nil // No Media known to AniList for this id.
	}

	out := &AniListAiring{AniListID: m.ID, Status: m.Status}
	if m.NextAiringEpisode != nil {
		out.NextEpisode = m.NextAiringEpisode.Episode
		t := time.Unix(m.NextAiringEpisode.AiringAt, 0).UTC()
		out.NextAiringAt = &t
	}
	for _, node := range m.AiringSchedule.Nodes {
		if node.Episode <= 0 || node.AiringAt <= 0 {
			continue
		}
		out.AiringHistory = append(out.AiringHistory, AniListEpisodeAiring{
			Episode:  node.Episode,
			AiringAt: time.Unix(node.AiringAt, 0).UTC(),
		})
	}
	return out, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
