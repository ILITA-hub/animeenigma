// CRITICAL: Stream DTO has NO iframe_url field at the Go type level.
// This is enforced by TestStream_HasNoIframeURL in provider_test.go.
//
// Rationale: D-DEC §2.8 — silent cross-tier fallback to a Kodik iframe is
// structurally impossible. ISS-008 and the AnimeLib Kodik fallback (commit
// 9347143) shipped this bug twice; the type system now prevents it.
//
// If you have a real reason to add an iframe-return path, do it on a SEPARATE
// DTO (e.g. KodikEmbed) with its own handler — never overload Stream.
package domain

import (
	"context"
	"time"
)

// Category identifies whether a stream is subbed, dubbed, or raw (no subs/dub).
// Providers fan out across categories so the orchestrator can match the user's
// preference at the scoring layer.
type Category string

const (
	CategorySub Category = "sub"
	CategoryDub Category = "dub"
	CategoryRaw Category = "raw"
)

// AnimeRef is the lookup key passed from catalog to scraper. Catalog UUID is
// the primary key; Shikimori/MAL and AniList IDs are provider-side fallbacks.
type AnimeRef struct {
	AnimeID     string // catalog UUID
	ShikimoriID string // == MAL ID
	AniListID   string
	Title       string
	Year        int
}

// Episode is one episode in a provider's listing for a given anime.
type Episode struct {
	ID       string `json:"id"`
	Number   int    `json:"number"`
	Title    string `json:"title"`
	IsFiller bool   `json:"is_filler"`
}

// Server is one of the streaming servers a provider lists for an episode
// (e.g. "vidstreaming", "megacloud"). The orchestrator picks the first server
// whose GetStream call succeeds.
type Server struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Source is one playable URL for a Stream. Multiple sources per Stream are
// allowed for multi-quality (e.g. 720p + 1080p MP4) or HLS variant-list-only
// providers that publish individual variant URLs.
type Source struct {
	URL     string `json:"url"`
	Type    string `json:"type"` // "hls" / "mp4"
	Quality string `json:"quality,omitempty"`
}

// Track is one subtitle or caption track attached to a Stream.
type Track struct {
	File    string `json:"file"`
	Label   string `json:"label,omitempty"`
	Kind    string `json:"kind"` // "captions" / "subtitles"
	Default bool   `json:"default,omitempty"`
}

// TimeRange marks an intro / outro segment in seconds from the start of the
// episode.
type TimeRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

// Stream is the full playback payload returned to the frontend.
//
// CRITICAL: this struct intentionally has NO iframe_url field. See the
// top-of-file comment in this file and TestStream_HasNoIframeURL.
type Stream struct {
	Sources []Source          `json:"sources"`
	Tracks  []Track           `json:"tracks,omitempty"`
	Intro   *TimeRange        `json:"intro,omitempty"`
	Outro   *TimeRange        `json:"outro,omitempty"`
	Headers map[string]string `json:"headers,omitempty"` // e.g. Referer for HLS proxy
}

// StageHealth captures the last success/failure for one stage of a provider's
// scrape pipeline (search, list, servers, sources). Surfaced in /scraper/health.
type StageHealth struct {
	Up      bool      `json:"up"`
	LastOK  time.Time `json:"last_ok"`
	LastErr string    `json:"last_err,omitempty"`
}

// Health is a per-provider health snapshot. Stages are keyed by stage name
// (e.g. "find_id", "list_episodes", "list_servers", "get_stream").
type Health struct {
	Provider string                 `json:"provider"`
	Stages   map[string]StageHealth `json:"stages"`
}

// Provider is the contract every scraper provider implements. Adding a new
// provider in Phase 16+ is one struct that satisfies this interface plus one
// registry entry — the orchestrator and HTTP handlers DO NOT change.
//
// Method-level contract notes:
//
//   - FindID resolves AnimeRef → provider-internal ID. Returns ErrNotFound if
//     the provider has no record of the anime.
//   - ListEpisodes returns the provider's episode listing. Real-empty (anime
//     exists, no episodes aired yet) is `([]Episode{}, nil)`, not an error.
//   - ListServers returns the streaming servers a provider lists for one episode.
//   - GetStream pulls the actual *Stream DTO for one (provider, episode, server,
//     category) tuple. Returns ErrExtractFailed if the stream URL could not
//     be extracted from the upstream HTML / API response.
//   - HealthCheck never returns an error — it inspects the in-memory stage cache.
//     Callers use it to render /scraper/health.
type Provider interface {
	Name() string
	FindID(ctx context.Context, ref AnimeRef) (string, error)
	ListEpisodes(ctx context.Context, providerID string) ([]Episode, error)
	ListServers(ctx context.Context, providerID, episodeID string) ([]Server, error)
	GetStream(ctx context.Context, providerID, episodeID, serverID string, category Category) (*Stream, error)
	HealthCheck(ctx context.Context) Health
}
