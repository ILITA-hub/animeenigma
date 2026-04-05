package hanime

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

const (
	authURL   = "https://www.universal-cdn.com/rapi/v4/sessions"
	searchURL = "https://search.htv-services.com/"
	videoURL  = "https://www.universal-cdn.com/rapi/v4/hentai-videos/%s"

	// Signature components (fixed constants used in HMAC-style sig)
	sigPrefix = "994482"
	sigA      = "2"
	sigB      = "8"
	sigC      = "113"

	userAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
)

// Client is the Hanime.tv API client.
type Client struct {
	mu           sync.Mutex
	httpClient   *http.Client
	email        string
	password     string
	sessionToken string
	tokenExpiry  time.Time
}

// NewClient creates a new Hanime client. Email and password are optional;
// if omitted, IsConfigured returns false and authenticated calls will fail.
func NewClient(email, password string) *Client {
	return &Client{
		email:    email,
		password: password,
		httpClient: &http.Client{
			Timeout: 15 * time.Second,
		},
	}
}

// IsConfigured returns true if the client has credentials set.
func (c *Client) IsConfigured() bool {
	return c.email != "" && c.password != ""
}

// ---- Auth ---------------------------------------------------------------

// computeSignature builds the SHA-256 hex signature used by Hanime's API.
// Formula: SHA256(sigPrefix + sigA + timestamp + sigB + timestamp + sigC)
func computeSignature(ts int64) string {
	tsStr := fmt.Sprintf("%d", ts)
	raw := sigPrefix + sigA + tsStr + sigB + tsStr + sigC
	sum := sha256.Sum256([]byte(raw))
	return fmt.Sprintf("%x", sum)
}

// login authenticates with Hanime and stores the session token.
func (c *Client) login() error {
	ts := time.Now().Unix()
	sig := computeSignature(ts)

	body, err := json.Marshal(map[string]string{
		"burger": c.email,
		"fries":  c.password,
	})
	if err != nil {
		return fmt.Errorf("hanime: marshal login body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, authURL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("hanime: create login request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("x-claim", fmt.Sprintf("%d", ts))
	req.Header.Set("x-signature-version", "app2")
	req.Header.Set("x-signature", sig)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("hanime: login request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("hanime: read login response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("hanime: login returned status %d: %s", resp.StatusCode, string(raw))
	}

	var result struct {
		SessionToken string `json:"session_token"`
		ExpiresAt    int64  `json:"session_token_expire_time_unix"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		return fmt.Errorf("hanime: parse login response: %w", err)
	}
	if result.SessionToken == "" {
		return fmt.Errorf("hanime: login succeeded but session_token is empty")
	}

	c.sessionToken = result.SessionToken
	if result.ExpiresAt > 0 {
		c.tokenExpiry = time.Unix(result.ExpiresAt, 0)
	} else {
		c.tokenExpiry = time.Now().Add(24 * time.Hour)
	}

	return nil
}

// ensureAuth refreshes the session token if it is absent or within 5 minutes of expiry.
func (c *Client) ensureAuth() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.sessionToken != "" && time.Now().Before(c.tokenExpiry.Add(-5*time.Minute)) {
		return nil
	}
	return c.login()
}

// authHeaders sets the required authentication headers on a request.
func (c *Client) authHeaders(req *http.Request) {
	ts := time.Now().Unix()
	sig := computeSignature(ts)

	req.Header.Set("x-claim", fmt.Sprintf("%d", ts))
	req.Header.Set("x-signature-version", "app2")
	req.Header.Set("x-signature", sig)
	req.Header.Set("x-session-token", c.sessionToken)
}

// ---- Search (no auth) ---------------------------------------------------

// SearchHit represents a single result from the Hanime search API.
type SearchHit struct {
	Name  string   `json:"name"`
	Slug  string   `json:"slug"`
	Brand string   `json:"brand"`
	Tags  []string `json:"tags"`
}

// searchRequest mirrors the body expected by search.htv-services.com.
type searchRequest struct {
	SearchText string   `json:"search_text"`
	Tags       []string `json:"tags"`
	Blacklist  []string `json:"blacklist"`
	Brands     []string `json:"brands"`
	OrderBy    string   `json:"order_by"`
	Ordering   string   `json:"ordering"`
	Page       int      `json:"page"`
	TagsMode   string   `json:"tags_mode"`
}

// searchHitRaw is used to handle the hits field which can be either a JSON
// array or a JSON-encoded string containing an array.
type searchHitRaw struct {
	Name  string          `json:"name"`
	Slug  string          `json:"slug"`
	Brand string          `json:"brand"`
	Tags  json.RawMessage `json:"tags"`
}

// Search queries the Hanime search API for the given title.
// This endpoint does not require authentication.
func (c *Client) Search(title string) ([]SearchHit, error) {
	payload := searchRequest{
		SearchText: title,
		Tags:       []string{},
		Blacklist:  []string{},
		Brands:     []string{},
		OrderBy:    "title_sortable",
		Ordering:   "asc",
		Page:       0,
		TagsMode:   "AND",
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("hanime: marshal search body: %w", err)
	}

	req, err := http.NewRequest(http.MethodPost, searchURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("hanime: create search request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", userAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hanime: search request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("hanime: read search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hanime: search returned status %d: %s", resp.StatusCode, string(raw))
	}

	// The `hits` field can be either a JSON array or a JSON-encoded string.
	var envelope struct {
		Hits json.RawMessage `json:"hits"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("hanime: parse search envelope: %w", err)
	}

	return parseHits(envelope.Hits)
}

// parseHits handles the dual hits format: array or JSON-string-of-array.
func parseHits(raw json.RawMessage) ([]SearchHit, error) {
	if len(raw) == 0 {
		return nil, nil
	}

	// Try direct array first.
	var rawHits []searchHitRaw
	if err := json.Unmarshal(raw, &rawHits); err != nil {
		// Maybe it's a JSON string wrapping an array.
		var encoded string
		if err2 := json.Unmarshal(raw, &encoded); err2 != nil {
			return nil, fmt.Errorf("hanime: parse hits: %w", err)
		}
		if err := json.Unmarshal([]byte(encoded), &rawHits); err != nil {
			return nil, fmt.Errorf("hanime: parse encoded hits: %w", err)
		}
	}

	hits := make([]SearchHit, 0, len(rawHits))
	for _, h := range rawHits {
		hit := SearchHit{
			Name:  h.Name,
			Slug:  h.Slug,
			Brand: h.Brand,
		}
		// Tags can be []string or []map with a name key — handle both gracefully.
		if len(h.Tags) > 0 {
			var tags []string
			if err := json.Unmarshal(h.Tags, &tags); err == nil {
				hit.Tags = tags
			} else {
				var tagObjs []struct {
					Text string `json:"text"`
					Name string `json:"name"`
				}
				if err2 := json.Unmarshal(h.Tags, &tagObjs); err2 == nil {
					for _, t := range tagObjs {
						if t.Text != "" {
							hit.Tags = append(hit.Tags, t.Text)
						} else if t.Name != "" {
							hit.Tags = append(hit.Tags, t.Name)
						}
					}
				}
			}
		}
		hits = append(hits, hit)
	}

	return hits, nil
}

// ---- GetVideo (auth required) -------------------------------------------

// Stream represents a single video stream entry from the manifest.
type Stream struct {
	URL              string  `json:"url"`
	Height           string  `json:"height"`
	Width            int     `json:"width"`
	FilesizeMbs      float64 `json:"filesize_mbs"`
	Extension        string  `json:"extension"`
	MimeType         string  `json:"mime_type"`
	IsGuestAllowed   bool    `json:"is_guest_allowed"`
	IsMemberAllowed  bool    `json:"is_member_allowed"`
	IsPremiumAllowed bool    `json:"is_premium_allowed"`
}

// Server represents a CDN server in the videos manifest.
type Server struct {
	Streams []Stream `json:"streams"`
}

// VideoMeta holds core metadata about a hentai video.
type VideoMeta struct {
	Name        string `json:"name"`
	Slug        string `json:"slug"`
	Brand       string `json:"brand"`
	PosterURL   string `json:"poster_url"`
	CoverURL    string `json:"cover_url"`
	Description string `json:"description"`
}

// FranchiseEntry is a related video in the same franchise.
type FranchiseEntry struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// Franchise holds the franchise metadata.
type Franchise struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
}

// VideoResponse is the full parsed response from the GetVideo endpoint.
type VideoResponse struct {
	Video              VideoMeta        `json:"hentai_video"`
	Servers            []Server         `json:"servers"`
	Franchise          Franchise        `json:"hentai_franchise"`
	FranchiseVideos    []FranchiseEntry `json:"hentai_franchise_hentai_videos"`
}

// GetVideo retrieves video metadata and stream URLs for the given slug.
// Authentication is performed (and refreshed) automatically.
func (c *Client) GetVideo(slug string) (*VideoResponse, error) {
	if err := c.ensureAuth(); err != nil {
		return nil, fmt.Errorf("hanime: authenticate before GetVideo: %w", err)
	}

	endpoint := fmt.Sprintf(videoURL, slug)

	req, err := http.NewRequest(http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("hanime: create video request: %w", err)
	}

	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json")
	c.authHeaders(req)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("hanime: video request: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("hanime: read video response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hanime: video returned status %d: %s", resp.StatusCode, string(raw))
	}

	// The API returns a top-level object with hentai_video, videos_manifest, etc.
	var envelope struct {
		HentaiVideo  VideoMeta `json:"hentai_video"`
		VideosManifest struct {
			Servers []Server `json:"servers"`
		} `json:"videos_manifest"`
		HentaiFranchise            Franchise        `json:"hentai_franchise"`
		HentaiFranchiseHentaiVideos []FranchiseEntry `json:"hentai_franchise_hentai_videos"`
	}

	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, fmt.Errorf("hanime: parse video response: %w", err)
	}

	result := &VideoResponse{
		Video:           envelope.HentaiVideo,
		Servers:         envelope.VideosManifest.Servers,
		Franchise:       envelope.HentaiFranchise,
		FranchiseVideos: envelope.HentaiFranchiseHentaiVideos,
	}

	return result, nil
}
