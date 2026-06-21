// Package allanime is a parser for the AllAnime GraphQL API. It exposes the
// catalog's "raw JP" video provider — original Japanese audio with no dub,
// served as HLS streams resolved via persisted-query GraphQL calls against
// AllAnime's rotating domain list.
//
// The API is undocumented and the persisted-query SHA256 hashes rotate every
// few months; hashes are read from config (env vars) so they can be updated
// without a code change. Reference implementations: pystardust/ani-cli,
// justfoolingaround/animdl, sdaqo/anipy-cli.
package allanime

import (
	"net/http"
	"sync"
	"time"
)

// Config controls AllAnime client behavior. All fields are populated from env
// via services/catalog/internal/config/config.go so the operator can rotate
// SHA hashes or domains without a redeploy beyond an env-var change.
type Config struct {
	// Domains is an ordered fallback list; the client iterates in order on
	// startup and caches the first responsive domain in memory for the
	// process lifetime (subject to Cooldown re-checks on failure).
	Domains []string

	// Persisted-query SHA256 hashes. Mirror the AllAnime web client's
	// Apollo persisted-query extension; rotate every few months. Source
	// from a live browser network capture or an upstream reference.
	QuerySearchSHA   string
	QueryEpisodesSHA string
	QuerySourcesSHA  string

	// HTTPTimeout per request. Default 10s.
	HTTPTimeout time.Duration

	// Headers expected by AllAnime's WAF.
	Referer   string // default "https://allmanga.to/"
	UserAgent string // default "AnimeEnigma/1.0"
}

// Client is the AllAnime GraphQL client.
type Client struct {
	cfg        Config
	httpClient *http.Client

	// Domain rotation cache. mu guards activeDomain + failedAt.
	mu             sync.RWMutex
	activeDomain   string    // current first-success domain
	failedAt       time.Time // last time the active domain failed
	domainCooldown time.Duration
}

// SearchResult is a search hit.
type SearchResult struct {
	ID       string // AllAnime show ID, used as input to EpisodesByID + RawStream
	Name     string // English / romanized title
	JName    string // Native Japanese title
	Poster   string // Poster URL (may be relative)
	Episodes int    // Total raw-translation-type episode count
}

// Episode is a single episode entry.
type Episode struct {
	ID     string // composite "showID/translationType/episodeString" used by RawStream
	Number int
	Title  string
}

// Stream is a resolved HLS stream URL.
type Stream struct {
	URL       string
	Type      string // "hls" or "mp4"
	Quality   string
	Subtitles []Subtitle
	Headers   map[string]string
}

// Subtitle is an embedded subtitle track returned alongside a stream.
type Subtitle struct {
	URL   string
	Lang  string
	Label string
}

// DefaultDomains is the fallback ordered list when the env var is unset.
var DefaultDomains = []string{"allanime.day", "allmanga.to", "allanime.to"}

// NewClient builds a Client from a Config. Empty/zero fields fall back to
// safe defaults (DefaultDomains, 10s timeout, project User-Agent, default
// Referer). The client makes no network calls at construction time —
// the rotating-domain cache is populated lazily on the first request.
func NewClient(cfg Config) *Client {
	if len(cfg.Domains) == 0 {
		cfg.Domains = DefaultDomains
	}
	if cfg.HTTPTimeout == 0 {
		cfg.HTTPTimeout = 10 * time.Second
	}
	if cfg.Referer == "" {
		cfg.Referer = "https://allmanga.to/"
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "AnimeEnigma/1.0"
	}

	return &Client{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.HTTPTimeout,
		},
		domainCooldown: 5 * time.Minute,
	}
}
