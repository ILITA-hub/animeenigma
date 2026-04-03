package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/animelib"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/parser/kodik"
)

const (
	playerKodik    = "kodik"
	playerAnimeLib = "animelib"
	playerHiAnime  = "hianime"
	playerConsumet = "consumet"
)

// PlayerHealthChecker periodically tests each player/parser to verify availability
// and exposes the results as Prometheus metrics.
type PlayerHealthChecker struct {
	kodikClient    *kodik.Client
	animelibClient *animelib.Client
	aniwatchURL    string
	consumetURL    string
	httpClient     *http.Client
	interval       time.Duration
	log            *logger.Logger

	// track previous status for transition logging
	prevStatus map[string]bool
}

// NewPlayerHealthChecker creates a new health checker.
func NewPlayerHealthChecker(
	kodikClient *kodik.Client,
	animelibClient *animelib.Client,
	aniwatchURL string,
	consumetURL string,
	interval time.Duration,
	log *logger.Logger,
) *PlayerHealthChecker {
	if interval <= 0 {
		interval = 5 * time.Minute
	}
	return &PlayerHealthChecker{
		kodikClient:    kodikClient,
		animelibClient: animelibClient,
		aniwatchURL:    aniwatchURL,
		consumetURL:    consumetURL,
		httpClient:     &http.Client{Timeout: 15 * time.Second},
		interval:       interval,
		log:            log,
		prevStatus:     make(map[string]bool),
	}
}

// Start runs the health checker loop until ctx is cancelled.
func (h *PlayerHealthChecker) Start(ctx context.Context) {
	h.log.Infow("player health checker started", "interval", h.interval.String())

	// Run immediately on start
	h.checkAll()

	ticker := time.NewTicker(h.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			h.log.Info("player health checker stopped")
			return
		case <-ticker.C:
			h.checkAll()
		}
	}
}

func (h *PlayerHealthChecker) checkAll() {
	h.checkPlayer(playerKodik, h.checkKodik)
	h.checkPlayer(playerAnimeLib, h.checkAnimeLib)
	h.checkPlayer(playerHiAnime, h.checkHiAnime)
	h.checkPlayer(playerConsumet, h.checkConsumet)
}

func (h *PlayerHealthChecker) checkPlayer(name string, check func() error) {
	start := time.Now()
	err := check()
	duration := time.Since(start).Seconds()

	metrics.PlayerHealthCheckDuration.WithLabelValues(name).Observe(duration)
	metrics.PlayerHealthLastCheck.WithLabelValues(name).SetToCurrentTime()

	up := err == nil
	if up {
		metrics.PlayerHealthUp.WithLabelValues(name).Set(1)
	} else {
		metrics.PlayerHealthUp.WithLabelValues(name).Set(0)
	}

	// Log status transitions
	prev, hasPrev := h.prevStatus[name]
	if !hasPrev || prev != up {
		if up {
			h.log.Infow("player is UP", "player", name, "duration_s", fmt.Sprintf("%.2f", duration))
		} else {
			h.log.Warnw("player is DOWN", "player", name, "error", err, "duration_s", fmt.Sprintf("%.2f", duration))
		}
	}
	h.prevStatus[name] = up
}

// checkKodik verifies Kodik API is reachable and returns results with embed links.
func (h *PlayerHealthChecker) checkKodik() error {
	if h.kodikClient == nil {
		return fmt.Errorf("kodik client not initialized")
	}
	results, err := h.kodikClient.SearchByShikimoriID("20") // Naruto
	if err != nil {
		return err
	}
	if len(results) == 0 {
		return fmt.Errorf("kodik returned 0 results for test query")
	}
	// Verify at least one result has an embed link
	for _, r := range results {
		if r.Link != "" {
			return nil
		}
	}
	return fmt.Errorf("kodik results have no embed links")
}

// checkAnimeLib verifies the full AnimeLib playback chain:
// search → get episodes → get episode streams → verify video sources exist.
func (h *PlayerHealthChecker) checkAnimeLib() error {
	if h.animelibClient == nil {
		return fmt.Errorf("animelib client not initialized")
	}

	// Step 1: Search
	results, err := h.animelibClient.Search("naruto")
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}
	if len(results) == 0 {
		return fmt.Errorf("search returned 0 results")
	}

	// Step 2: Get episodes
	episodes, err := h.animelibClient.GetEpisodes(results[0].ID)
	if err != nil {
		return fmt.Errorf("get episodes failed (anime %d): %w", results[0].ID, err)
	}
	if len(episodes) == 0 {
		return fmt.Errorf("anime %d has 0 episodes", results[0].ID)
	}

	// Step 3: Get streams for first episode
	detail, err := h.animelibClient.GetEpisodeStreams(episodes[0].ID)
	if err != nil {
		return fmt.Errorf("get streams failed (episode %d): %w", episodes[0].ID, err)
	}
	if len(detail.Players) == 0 {
		return fmt.Errorf("episode %d has 0 players", episodes[0].ID)
	}

	// Step 4: Verify at least one player has a video source
	for _, p := range detail.Players {
		if p.Video != nil && len(p.Video.Quality) > 0 {
			return nil // Has direct MP4
		}
		if p.Src != "" {
			return nil // Has Kodik iframe
		}
	}
	return fmt.Errorf("episode %d players have no video sources", episodes[0].ID)
}

// checkHiAnime verifies the full HiAnime playback chain via aniwatch API:
// search → get episodes → get stream sources.
func (h *PlayerHealthChecker) checkHiAnime() error {
	if h.aniwatchURL == "" {
		return fmt.Errorf("aniwatch API URL not configured")
	}

	// Step 1: Search
	body, err := h.httpGet(h.aniwatchURL + "/api/v2/hianime/search?q=naruto&page=1")
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	var searchResp struct {
		Data struct {
			Animes []struct {
				ID string `json:"id"`
			} `json:"animes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return fmt.Errorf("failed to parse search response: %w", err)
	}
	if len(searchResp.Data.Animes) == 0 {
		return fmt.Errorf("search returned 0 results")
	}
	animeID := searchResp.Data.Animes[0].ID

	// Step 2: Get episodes
	body, err = h.httpGet(fmt.Sprintf("%s/api/v2/hianime/anime/%s/episodes", h.aniwatchURL, url.PathEscape(animeID)))
	if err != nil {
		return fmt.Errorf("episodes failed: %w", err)
	}

	var epsResp struct {
		Data struct {
			Episodes []struct {
				EpisodeID string `json:"episodeId"`
			} `json:"episodes"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &epsResp); err != nil {
		return fmt.Errorf("failed to parse episodes response: %w", err)
	}
	if len(epsResp.Data.Episodes) == 0 {
		return fmt.Errorf("anime %s has 0 episodes", animeID)
	}
	episodeID := epsResp.Data.Episodes[0].EpisodeID

	// Step 3: Get stream sources
	body, err = h.httpGet(fmt.Sprintf("%s/api/v2/hianime/episode/sources?animeEpisodeId=%s&server=hd-2&category=sub",
		h.aniwatchURL, url.QueryEscape(episodeID)))
	if err != nil {
		return fmt.Errorf("sources failed: %w", err)
	}

	var srcResp struct {
		Data struct {
			Sources []struct {
				URL string `json:"url"`
			} `json:"sources"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &srcResp); err != nil {
		return fmt.Errorf("failed to parse sources response: %w", err)
	}
	if len(srcResp.Data.Sources) == 0 || srcResp.Data.Sources[0].URL == "" {
		return fmt.Errorf("no HLS sources for episode %s", episodeID)
	}

	return nil
}

// checkConsumet verifies the full Consumet playback chain:
// search → get info with episodes → get stream sources.
func (h *PlayerHealthChecker) checkConsumet() error {
	if h.consumetURL == "" {
		return fmt.Errorf("consumet API URL not configured")
	}

	// Step 1: Search
	body, err := h.httpGet(h.consumetURL + "/anime/animekai/naruto")
	if err != nil {
		return fmt.Errorf("search failed: %w", err)
	}

	var searchResp struct {
		Results []struct {
			ID string `json:"id"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return fmt.Errorf("failed to parse search response: %w", err)
	}
	if len(searchResp.Results) == 0 {
		return fmt.Errorf("search returned 0 results")
	}
	animeID := searchResp.Results[0].ID

	// Step 2: Get info with episodes
	body, err = h.httpGet(fmt.Sprintf("%s/anime/animekai/info?id=%s", h.consumetURL, url.QueryEscape(animeID)))
	if err != nil {
		return fmt.Errorf("info failed: %w", err)
	}

	var infoResp struct {
		Episodes []struct {
			ID string `json:"id"`
		} `json:"episodes"`
	}
	if err := json.Unmarshal(body, &infoResp); err != nil {
		return fmt.Errorf("failed to parse info response: %w", err)
	}
	if len(infoResp.Episodes) == 0 {
		return fmt.Errorf("anime %s has 0 episodes", animeID)
	}
	episodeID := infoResp.Episodes[0].ID

	// Step 3: Get stream
	body, err = h.httpGet(fmt.Sprintf("%s/anime/animekai/watch?episodeId=%s", h.consumetURL, url.QueryEscape(episodeID)))
	if err != nil {
		return fmt.Errorf("stream failed: %w", err)
	}

	var streamResp struct {
		Sources []struct {
			URL string `json:"url"`
		} `json:"sources"`
	}
	if err := json.Unmarshal(body, &streamResp); err != nil {
		return fmt.Errorf("failed to parse stream response: %w", err)
	}
	if len(streamResp.Sources) == 0 || streamResp.Sources[0].URL == "" {
		return fmt.Errorf("no stream sources for episode %s", episodeID)
	}

	return nil
}

// httpGet performs a GET request and returns the body. Returns error on non-200 status.
func (h *PlayerHealthChecker) httpGet(reqURL string) ([]byte, error) {
	resp, err := h.httpClient.Get(reqURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	return body, nil
}
