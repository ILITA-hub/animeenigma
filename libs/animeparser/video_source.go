package animeparser

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VideoSource represents a video streaming source
type VideoSource struct {
	Provider     string            `json:"provider"`      // aniboom, kodik, minio
	URL          string            `json:"url"`           // Direct stream URL or embed URL
	Quality      string            `json:"quality"`       // 360p, 480p, 720p, 1080p
	Translation  string            `json:"translation"`   // Dubbing studio or subtitles
	TranslationType string         `json:"translation_type"` // voice, subtitles
	Episode      int               `json:"episode"`
	ExpiresAt    time.Time         `json:"expires_at,omitempty"`
	Headers      map[string]string `json:"headers,omitempty"` // Required headers for streaming
	IsDirect     bool              `json:"is_direct"`    // Can be played directly by frontend
	NeedsProxy   bool              `json:"needs_proxy"`  // Requires backend proxy
}

// VideoSourceProvider interface for different video source providers
type VideoSourceProvider interface {
	Name() string
	SearchByTitle(ctx context.Context, title string, titleJP string) ([]VideoSource, error)
	GetEpisodes(ctx context.Context, animeID string) ([]VideoSource, error)
	GetStreamURL(ctx context.Context, videoID string, quality string) (*VideoSource, error)
}

// ProviderRegistry holds all registered video source providers
type ProviderRegistry struct {
	providers map[string]VideoSourceProvider
}

// NewProviderRegistry creates a new provider registry
func NewProviderRegistry() *ProviderRegistry {
	return &ProviderRegistry{
		providers: make(map[string]VideoSourceProvider),
	}
}

// Register adds a provider to the registry
func (r *ProviderRegistry) Register(provider VideoSourceProvider) {
	r.providers[provider.Name()] = provider
}

// Get returns a provider by name
func (r *ProviderRegistry) Get(name string) (VideoSourceProvider, bool) {
	p, ok := r.providers[name]
	return p, ok
}

// SearchAll searches all providers for video sources
func (r *ProviderRegistry) SearchAll(ctx context.Context, title, titleJP string) ([]VideoSource, error) {
	var allSources []VideoSource

	for _, provider := range r.providers {
		sources, err := provider.SearchByTitle(ctx, title, titleJP)
		if err != nil {
			// Log but continue with other providers
			continue
		}
		allSources = append(allSources, sources...)
	}

	return allSources, nil
}

// =============================================================================
// Kodik Provider
// =============================================================================

// KodikConfig holds Kodik API configuration
type KodikConfig struct {
	APIKey  string
	BaseURL string
}

// KodikProvider implements VideoSourceProvider for Kodik
type KodikProvider struct {
	config KodikConfig
	client *http.Client
}

// NewKodikProvider creates a new Kodik provider
func NewKodikProvider(cfg KodikConfig) *KodikProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://kodikapi.com"
	}
	return &KodikProvider{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (k *KodikProvider) Name() string {
	return "kodik"
}

// KodikSearchResponse represents Kodik API search response
type KodikSearchResponse struct {
	Time    string `json:"time"`
	Total   int    `json:"total"`
	Results []struct {
		ID              string `json:"id"`
		Type            string `json:"type"`
		Link            string `json:"link"`
		Title           string `json:"title"`
		TitleOrig       string `json:"title_orig"`
		OtherTitle      string `json:"other_title"`
		Translation     struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
			Type  string `json:"type"`
		} `json:"translation"`
		Year            int    `json:"year"`
		LastEpisode     int    `json:"last_episode"`
		EpisodesCount   int    `json:"episodes_count"`
		ShikimoriID     string `json:"shikimori_id"`
		Quality         string `json:"quality"`
		Screenshots     []string `json:"screenshots"`
	} `json:"results"`
}

func (k *KodikProvider) SearchByTitle(ctx context.Context, title, titleJP string) ([]VideoSource, error) {
	// Try Japanese title first, then original
	searchTitle := titleJP
	if searchTitle == "" {
		searchTitle = title
	}

	params := url.Values{}
	params.Set("token", k.config.APIKey)
	params.Set("title", searchTitle)
	params.Set("with_episodes", "true")
	params.Set("with_material_data", "true")

	reqURL := fmt.Sprintf("%s/search?%s", k.config.BaseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var searchResp KodikSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	var sources []VideoSource
	for _, result := range searchResp.Results {
		translationType := "voice"
		if strings.Contains(strings.ToLower(result.Translation.Type), "sub") {
			translationType = "subtitles"
		}

		source := VideoSource{
			Provider:        "kodik",
			URL:             result.Link,
			Quality:         result.Quality,
			Translation:     result.Translation.Title,
			TranslationType: translationType,
			Episode:         result.LastEpisode,
			IsDirect:        false, // Kodik uses iframe embed
			NeedsProxy:      false, // Frontend can use iframe
		}
		sources = append(sources, source)
	}

	return sources, nil
}

func (k *KodikProvider) GetEpisodes(ctx context.Context, animeID string) ([]VideoSource, error) {
	params := url.Values{}
	params.Set("token", k.config.APIKey)
	params.Set("shikimori_id", animeID)
	params.Set("with_episodes", "true")

	reqURL := fmt.Sprintf("%s/search?%s", k.config.BaseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := k.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var searchResp KodikSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	var sources []VideoSource
	for _, result := range searchResp.Results {
		for ep := 1; ep <= result.EpisodesCount; ep++ {
			source := VideoSource{
				Provider:        "kodik",
				URL:             fmt.Sprintf("%s?episode=%d", result.Link, ep),
				Quality:         result.Quality,
				Translation:     result.Translation.Title,
				TranslationType: result.Translation.Type,
				Episode:         ep,
				IsDirect:        false,
				NeedsProxy:      false,
			}
			sources = append(sources, source)
		}
	}

	return sources, nil
}

func (k *KodikProvider) GetStreamURL(ctx context.Context, videoID, quality string) (*VideoSource, error) {
	// Kodik uses iframe embeds, return the embed URL
	return &VideoSource{
		Provider:   "kodik",
		URL:        videoID, // The link is already the embed URL
		Quality:    quality,
		IsDirect:   false,
		NeedsProxy: false,
	}, nil
}

// =============================================================================
// Aniboom Provider
// =============================================================================

// AniboomConfig holds Aniboom API configuration
type AniboomConfig struct {
	BaseURL string
}

// AniboomProvider implements VideoSourceProvider for Aniboom
type AniboomProvider struct {
	config AniboomConfig
	client *http.Client
}

// NewAniboomProvider creates a new Aniboom provider
func NewAniboomProvider(cfg AniboomConfig) *AniboomProvider {
	if cfg.BaseURL == "" {
		cfg.BaseURL = "https://api.aniboom.one"
	}
	return &AniboomProvider{
		config: cfg,
		client: &http.Client{Timeout: 10 * time.Second},
	}
}

func (a *AniboomProvider) Name() string {
	return "aniboom"
}

// AniboomSearchResponse represents Aniboom API response
type AniboomSearchResponse struct {
	Success bool `json:"success"`
	Data    []struct {
		ID          int    `json:"id"`
		Title       string `json:"title"`
		TitleOrig   string `json:"title_orig"`
		ShikimoriID int    `json:"shikimori_id"`
		Episodes    []struct {
			Number int    `json:"number"`
			Title  string `json:"title"`
			Videos []struct {
				Quality string `json:"quality"`
				URL     string `json:"url"`
				Studio  string `json:"studio"`
			} `json:"videos"`
		} `json:"episodes"`
	} `json:"data"`
}

func (a *AniboomProvider) SearchByTitle(ctx context.Context, title, titleJP string) ([]VideoSource, error) {
	searchTitle := title
	if titleJP != "" {
		searchTitle = titleJP
	}

	params := url.Values{}
	params.Set("q", searchTitle)

	reqURL := fmt.Sprintf("%s/anime/search?%s", a.config.BaseURL, params.Encode())

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var searchResp AniboomSearchResponse
	if err := json.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, err
	}

	var sources []VideoSource
	for _, anime := range searchResp.Data {
		for _, ep := range anime.Episodes {
			for _, video := range ep.Videos {
				source := VideoSource{
					Provider:        "aniboom",
					URL:             video.URL,
					Quality:         video.Quality,
					Translation:     video.Studio,
					TranslationType: "voice",
					Episode:         ep.Number,
					IsDirect:        true,  // Aniboom provides direct HLS streams
					NeedsProxy:      true,  // But may need proxy for CORS
					ExpiresAt:       time.Now().Add(time.Hour), // URLs typically expire in 1 hour
				}
				sources = append(sources, source)
			}
		}
	}

	return sources, nil
}

func (a *AniboomProvider) GetEpisodes(ctx context.Context, animeID string) ([]VideoSource, error) {
	reqURL := fmt.Sprintf("%s/anime/%s/episodes", a.config.BaseURL, animeID)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var episodesResp struct {
		Episodes []struct {
			Number int `json:"number"`
			Videos []struct {
				Quality string `json:"quality"`
				URL     string `json:"url"`
				Studio  string `json:"studio"`
			} `json:"videos"`
		} `json:"episodes"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&episodesResp); err != nil {
		return nil, err
	}

	var sources []VideoSource
	for _, ep := range episodesResp.Episodes {
		for _, video := range ep.Videos {
			source := VideoSource{
				Provider:        "aniboom",
				URL:             video.URL,
				Quality:         video.Quality,
				Translation:     video.Studio,
				TranslationType: "voice",
				Episode:         ep.Number,
				IsDirect:        true,
				NeedsProxy:      true,
				ExpiresAt:       time.Now().Add(time.Hour),
			}
			sources = append(sources, source)
		}
	}

	return sources, nil
}

func (a *AniboomProvider) GetStreamURL(ctx context.Context, videoID, quality string) (*VideoSource, error) {
	reqURL := fmt.Sprintf("%s/video/%s/stream?quality=%s", a.config.BaseURL, videoID, quality)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return nil, err
	}

	resp, err := a.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var streamResp struct {
		URL     string `json:"url"`
		Quality string `json:"quality"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&streamResp); err != nil {
		return nil, err
	}

	return &VideoSource{
		Provider:   "aniboom",
		URL:        streamResp.URL,
		Quality:    streamResp.Quality,
		IsDirect:   true,
		NeedsProxy: true,
		ExpiresAt:  time.Now().Add(time.Hour),
	}, nil
}
