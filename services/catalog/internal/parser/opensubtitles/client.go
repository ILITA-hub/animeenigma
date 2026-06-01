// Package opensubtitles is a thin client for OpenSubtitles' v1 REST API.
// Workstream raw-jp, Phase 02 — subtitle aggregation.
//
// Auth: Api-Key header from OPENSUBTITLES_API_KEY env. A configured User-Agent
// is required by OpenSubtitles' usage policy.
//
// The /subtitles endpoint accepts IMDb or TMDB keys plus optional
// season/episode numbers. AnimeEnigma sources IMDb IDs via the Kitsu
// mappings endpoint (see libs/idmapping/kitsu.go) so subtitle search works
// for both new and pre-mapped anime.
package opensubtitles

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

const defaultBaseURL = "https://api.opensubtitles.com/api/v1"

// ErrUnauthorized signals a 401/403 — typically a missing or revoked API key.
var ErrUnauthorized = errors.New("opensubtitles: unauthorized (check api key)")

// ErrRateLimited signals a 429 or "Reached download limit" body.
var ErrRateLimited = errors.New("opensubtitles: rate limited")

// Config controls the client.
type Config struct {
	APIKey    string
	UserAgent string
	Timeout   time.Duration
	BaseURL   string         // override for tests
	Logger    *logger.Logger // optional; when set, Download logs quota usage
}

// Client is the OpenSubtitles v1 REST client.
type Client struct {
	cfg        Config
	httpClient *http.Client
}

// NewClient builds a Client. Empty/zero Config fields fall back to safe
// defaults (10s timeout, official base URL, "AnimeEnigma/1.0" agent).
func NewClient(cfg Config) *Client {
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.UserAgent == "" {
		cfg.UserAgent = "AnimeEnigma/1.0"
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return &Client{
		cfg:        cfg,
		httpClient: &http.Client{Timeout: cfg.Timeout},
	}
}

// IsConfigured returns true when an API key is set. The aggregator skips
// OpenSubtitles entirely (no log noise) when this is false.
func (c *Client) IsConfigured() bool {
	return c != nil && c.cfg.APIKey != ""
}

// SearchParams scopes a subtitle search.
type SearchParams struct {
	IMDbID        string   // e.g. "tt15302498" — preferred key
	TMDBID        string   // fallback when IMDb missing
	Query         string   // free-text fallback when no IDs
	Languages     []string // ISO 639-1 codes; empty == all
	SeasonNumber  int      // 0 == omit (movies)
	EpisodeNumber int      // 0 == omit (movies)
}

// SubtitleEntry is one search hit.
type SubtitleEntry struct {
	ID            string // OpenSubtitles file_id as string
	FileID        int    // numeric file id for download
	Language      string // ISO 639-1
	Release       string // release name (e.g. "Bocchi.the.Rock.S01E01.1080p.WEBRip")
	DownloadCount int    // popularity heuristic
	Format        string // "srt", "vtt", "ass"
	DownloadURL   string // direct file URL (when available)
}

// Search hits /subtitles with the given params. On 401/403 returns
// ErrUnauthorized; on 429 returns ErrRateLimited. The aggregator turns both
// into "skip OpenSubtitles, return Jimaku-only" — graceful per RAW-NF-01.
func (c *Client) Search(ctx context.Context, p SearchParams) ([]SubtitleEntry, error) {
	if !c.IsConfigured() {
		return nil, ErrUnauthorized
	}

	q := url.Values{}
	if p.IMDbID != "" {
		// The API expects the imdb_id WITHOUT the "tt" prefix.
		q.Set("imdb_id", strings.TrimPrefix(p.IMDbID, "tt"))
	}
	if p.TMDBID != "" {
		q.Set("tmdb_id", p.TMDBID)
	}
	if p.Query != "" && p.IMDbID == "" && p.TMDBID == "" {
		q.Set("query", p.Query)
	}
	if len(p.Languages) > 0 {
		q.Set("languages", strings.Join(p.Languages, ","))
	}
	if p.SeasonNumber > 0 {
		q.Set("season_number", strconv.Itoa(p.SeasonNumber))
	}
	if p.EpisodeNumber > 0 {
		q.Set("episode_number", strconv.Itoa(p.EpisodeNumber))
	}

	endpoint := fmt.Sprintf("%s/subtitles?%s", c.cfg.BaseURL, q.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: build request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Api-Key", c.cfg.APIKey)
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("opensubtitles: request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrRateLimited
	case strings.Contains(string(body), "Reached download limit"):
		return nil, ErrRateLimited
	case resp.StatusCode >= 500:
		return nil, fmt.Errorf("opensubtitles: upstream %d", resp.StatusCode)
	case resp.StatusCode >= 400:
		return nil, fmt.Errorf("opensubtitles: %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var doc struct {
		Data []struct {
			Type       string `json:"type"`
			ID         string `json:"id"`
			Attributes struct {
				Language       string `json:"language"`
				Release        string `json:"release"`
				DownloadCount  int    `json:"download_count"`
				FileExtension  string `json:"file_extension"`
				Files          []struct {
					FileID   int    `json:"file_id"`
					FileName string `json:"file_name"`
				} `json:"files"`
				URL string `json:"url"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("opensubtitles: parse: %w", err)
	}

	out := make([]SubtitleEntry, 0, len(doc.Data))
	for _, d := range doc.Data {
		entry := SubtitleEntry{
			ID:            d.ID,
			Language:      normalizeLang(d.Attributes.Language),
			Release:       d.Attributes.Release,
			DownloadCount: d.Attributes.DownloadCount,
			Format:        d.Attributes.FileExtension,
			DownloadURL:   d.Attributes.URL,
		}
		if len(d.Attributes.Files) > 0 {
			entry.FileID = d.Attributes.Files[0].FileID
			if entry.Format == "" {
				entry.Format = formatFromFilename(d.Attributes.Files[0].FileName)
			}
		}
		out = append(out, entry)
	}
	return out, nil
}

// Download resolves a subtitle file_id to its actual content. It spends one
// unit of the OpenSubtitles daily download quota per call (per RAW-NF-01 the
// caller is expected to cache the result). Returns the raw bytes plus the
// server-provided file name (used for format detection).
//
// On quota exhaustion returns ErrRateLimited; on 401/403 ErrUnauthorized.
func (c *Client) Download(ctx context.Context, fileID int) ([]byte, string, error) {
	if !c.IsConfigured() {
		return nil, "", ErrUnauthorized
	}

	reqBody, _ := json.Marshal(map[string]int{"file_id": fileID})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.BaseURL+"/download", strings.NewReader(string(reqBody)))
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: build download request: %w", err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Api-Key", c.cfg.APIKey)
	req.Header.Set("User-Agent", c.cfg.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: download request: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, "", ErrUnauthorized
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, "", ErrRateLimited
	case strings.Contains(string(body), "download limit"),
		strings.Contains(string(body), "Reached download limit"),
		strings.Contains(string(body), "maximum number"):
		return nil, "", ErrRateLimited
	case resp.StatusCode >= 400:
		return nil, "", fmt.Errorf("opensubtitles: download %d: %s", resp.StatusCode, truncate(string(body), 200))
	}

	var doc struct {
		Link      string `json:"link"`
		FileName  string `json:"file_name"`
		Requests  int    `json:"requests"`
		Remaining int    `json:"remaining"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, "", fmt.Errorf("opensubtitles: parse download: %w", err)
	}
	// Spec rule: exhausted quota presents as remaining<=0 and/or no usable link
	// (OpenSubtitles returns 200 with a message in that case, not a 4xx).
	if doc.Link == "" {
		return nil, "", ErrRateLimited
	}
	if c.cfg.Logger != nil {
		c.cfg.Logger.Infow("opensubtitles download spent",
			"file_id", fileID,
			"requests_today", doc.Requests,
			"remaining", doc.Remaining)
	}

	fileReq, err := http.NewRequestWithContext(ctx, http.MethodGet, doc.Link, nil)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: build file request: %w", err)
	}
	fileResp, err := c.httpClient.Do(fileReq)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: fetch file: %w", err)
	}
	defer fileResp.Body.Close()
	if fileResp.StatusCode >= 400 {
		return nil, "", fmt.Errorf("opensubtitles: fetch file %d", fileResp.StatusCode)
	}
	content, err := io.ReadAll(fileResp.Body)
	if err != nil {
		return nil, "", fmt.Errorf("opensubtitles: read file: %w", err)
	}
	return content, doc.FileName, nil
}

// normalizeLang collapses common 3-letter and full-word codes to ISO 639-1.
func normalizeLang(code string) string {
	code = strings.ToLower(strings.TrimSpace(code))
	switch code {
	case "ja", "jpn", "japanese":
		return "ja"
	case "en", "eng", "english":
		return "en"
	case "ru", "rus", "russian":
		return "ru"
	case "zh", "chi", "zho", "chinese":
		return "zh"
	case "ko", "kor", "korean":
		return "ko"
	case "es", "spa", "spanish":
		return "es"
	case "fr", "fra", "fre", "french":
		return "fr"
	case "de", "deu", "ger", "german":
		return "de"
	case "pt", "por", "portuguese":
		return "pt"
	case "it", "ita", "italian":
		return "it"
	case "ar", "ara", "arabic":
		return "ar"
	}
	if len(code) == 2 {
		return code
	}
	return code
}

// formatFromFilename guesses the subtitle format from the file extension when
// OpenSubtitles' explicit `file_extension` field is empty.
func formatFromFilename(name string) string {
	low := strings.ToLower(name)
	switch {
	case strings.HasSuffix(low, ".srt"):
		return "srt"
	case strings.HasSuffix(low, ".ass"):
		return "ass"
	case strings.HasSuffix(low, ".vtt"):
		return "vtt"
	default:
		return ""
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
