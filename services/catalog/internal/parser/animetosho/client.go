// Package animetosho is the AnimeTosho client — an indexer that extracts
// softsub tracks (and fonts) out of every anime release it mirrors and
// serves them as individual downloadable files. For Crunchyroll simulcasts
// the Erai-raws Multi-Sub releases carry the OFFICIAL CR subtitles in up to
// ~10 languages (including EN and RU), available within hours of airing —
// which makes this the primary subtitle source for ongoing seasonals.
//
// Three read-only surfaces (no auth, no key):
//
//	search: GET {feed}/json?qx=1&aid=<anidb id>     → release list (newest first)
//	detail: GET {feed}/json?show=torrent&id=<id>    → files + per-file attachments
//	file:   GET {storage}/storage/attach/%08x/s.ass.xz → xz-compressed subtitle
//
// The attachment download path embeds the attachment id as 8-digit
// zero-padded hex; the trailing filename is cosmetic (verified live
// 2026-07-18: attachment 2905425 → /storage/attach/002c5551/rus.ass.xz).
package animetosho

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/ulikunitz/xz"
)

const (
	defaultFeedBaseURL    = "https://feed.animetosho.org"
	defaultStorageBaseURL = "https://animetosho.org"

	// maxResponseBytes caps any single upstream body read. Feed JSON for a
	// long-running series runs a few hundred KB; attachment .xz files are
	// tens of KB. 16MB bounds a hostile body far above both.
	maxResponseBytes = 16 << 20

	// maxDecompressedBytes caps the xz-decompressed subtitle size. Real ASS
	// files are 10-200KB; 8MB stops a decompression bomb.
	maxDecompressedBytes = 8 << 20
)

// Config configures the AnimeTosho client. Empty fields fall back to defaults.
type Config struct {
	FeedBaseURL    string
	StorageBaseURL string
	Enabled        bool
	UserAgent      string
	Timeout        time.Duration
	Transport      http.RoundTripper // egress-recording transport (AR-EGRESS-03)
}

// Client is the AnimeTosho read-only HTTP client.
type Client struct {
	feedBaseURL    string
	storageBaseURL string
	enabled        bool
	userAgent      string
	httpClient     *http.Client
}

// Release is one entry in the per-series release list. The feed carries more
// fields (timestamp, num_files, ...); only what the aggregator reads is kept.
type Release struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
}

// AttachmentInfo is the codec/language metadata of one extracted attachment.
type AttachmentInfo struct {
	Codec string `json:"codec"` // "ASS", "SRT", ... (mkv codec naming: SRT may appear as "UTF-8")
	Lang  string `json:"lang"`  // ISO 639-2 ("eng", "rus", ...); empty when untagged
	Name  string `json:"name"`  // track title, e.g. "CR", "Latin_America_CR"
}

// Attachment is one extracted file (subtitle track or embedded font).
type Attachment struct {
	ID   int            `json:"id"`
	Type string         `json:"type"` // "subtitle" | "font" | ...
	Info AttachmentInfo `json:"info"`
}

// TorrentFile is one file inside a release, with its extracted attachments.
type TorrentFile struct {
	Filename    string       `json:"filename"`
	Attachments []Attachment `json:"attachments"`
}

// NewClient builds a Client, applying safe defaults.
func NewClient(cfg Config) *Client {
	if cfg.FeedBaseURL == "" {
		cfg.FeedBaseURL = defaultFeedBaseURL
	}
	if cfg.StorageBaseURL == "" {
		cfg.StorageBaseURL = defaultStorageBaseURL
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 12 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "AnimeEnigma/1.0 (+https://animeenigma.org)"
	}
	return &Client{
		feedBaseURL:    strings.TrimRight(cfg.FeedBaseURL, "/"),
		storageBaseURL: strings.TrimRight(cfg.StorageBaseURL, "/"),
		enabled:        cfg.Enabled,
		userAgent:      cfg.UserAgent,
		httpClient:     &http.Client{Timeout: cfg.Timeout, Transport: cfg.Transport},
	}
}

// IsConfigured reports whether the provider is enabled. AnimeTosho needs no
// key, so this is just the enable flag; the aggregator skips it when false.
func (c *Client) IsConfigured() bool { return c != nil && c.enabled }

// SearchByAniDB lists all releases indexed under an AniDB series id,
// newest first. AniDB ids are per-season, so every release returned is the
// right season by construction — episode numbers in titles are the only
// remaining disambiguation.
func (c *Client) SearchByAniDB(ctx context.Context, aniDBID int) ([]Release, error) {
	body, err := c.fetch(ctx, c.feedBaseURL+"/json?qx=1&aid="+strconv.Itoa(aniDBID))
	if err != nil {
		return nil, err
	}
	var out []Release
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("animetosho: search response: %w", err)
	}
	return out, nil
}

// TorrentFiles fetches one release's file list with extracted attachments.
func (c *Client) TorrentFiles(ctx context.Context, torrentID int) ([]TorrentFile, error) {
	body, err := c.fetch(ctx, c.feedBaseURL+"/json?show=torrent&id="+strconv.Itoa(torrentID))
	if err != nil {
		return nil, err
	}
	var detail struct {
		Files []TorrentFile `json:"files"`
	}
	if err := json.Unmarshal(body, &detail); err != nil {
		return nil, fmt.Errorf("animetosho: torrent response: %w", err)
	}
	return detail.Files, nil
}

// DownloadAttachment fetches one extracted subtitle attachment and returns
// the decompressed (xz) subtitle bytes.
func (c *Client) DownloadAttachment(ctx context.Context, attachID int) ([]byte, error) {
	url := fmt.Sprintf("%s/storage/attach/%08x/sub.ass.xz", c.storageBaseURL, attachID)
	body, err := c.fetch(ctx, url)
	if err != nil {
		return nil, err
	}
	r, err := xz.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("animetosho: xz open attachment %d: %w", attachID, err)
	}
	out, err := io.ReadAll(io.LimitReader(r, maxDecompressedBytes))
	if err != nil {
		return nil, fmt.Errorf("animetosho: xz read attachment %d: %w", attachID, err)
	}
	return out, nil
}

// Ping checks feed reachability. Returns latency. aid=1 is an ancient AniDB
// entry with a tiny, stable result set — cheap on both sides.
func (c *Client) Ping(ctx context.Context) (time.Duration, error) {
	start := time.Now()
	body, err := c.fetch(ctx, c.feedBaseURL+"/json?qx=1&aid=1")
	if err != nil {
		return time.Since(start), err
	}
	var probe []json.RawMessage
	if err := json.Unmarshal(body, &probe); err != nil {
		return time.Since(start), fmt.Errorf("animetosho: feed did not return a JSON list")
	}
	return time.Since(start), nil
}

func (c *Client) fetch(ctx context.Context, url string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("animetosho: GET %s: status %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
}

// Episode-number extraction from release/file titles. Two conventions cover
// the groups that matter:
//
//	"[Erai-raws] Title - 05 [1080p ...]"  → dash convention
//	"[DKB] Title - S04E05 [1080p ...]"    → SxxEyy convention
//
// Batch ranges ("Title - 01-12", "Title - 01~12") deliberately do NOT match
// the dash convention (the digits must be followed by a separator, not by a
// range mark), so batches are skipped rather than mis-picked. A version
// suffix ("05v2") is tolerated.
var (
	reEpSxxEyy = regexp.MustCompile(`(?i)\bS\d{1,3}E(\d{1,4})\b`)
	// After the digits: a separator, end of string, or a file-extension dot.
	// `\.\D` (not a bare `\.`) keeps half-episode specials ("Title - 05.5")
	// from matching as episode 5.
	reEpDash = regexp.MustCompile(`\s-\s(\d{1,4})(?:v\d+)?(?:[\s\[(]|\.\D|\.$|$)`)
)

// EpisodeFromTitle parses the episode number out of a release or file title.
// Returns (0, false) when no single-episode marker is present (movies,
// batches, specials with nonstandard naming).
func EpisodeFromTitle(title string) (int, bool) {
	if m := reEpSxxEyy.FindStringSubmatch(title); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n, true
		}
	}
	if m := reEpDash.FindStringSubmatch(title); m != nil {
		if n, err := strconv.Atoi(m[1]); err == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}

// ReleaseGroup extracts the leading "[Group]" tag from a release title,
// e.g. "[Erai-raws] Title - 05" → "Erai-raws". Empty when absent.
func ReleaseGroup(title string) string {
	t := strings.TrimSpace(title)
	if !strings.HasPrefix(t, "[") {
		return ""
	}
	end := strings.IndexByte(t, ']')
	if end <= 1 {
		return ""
	}
	return t[1:end]
}
