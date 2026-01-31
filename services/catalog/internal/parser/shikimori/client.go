package shikimori

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/config"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/hasura/go-graphql-client"
)

// Client for Shikimori API
type Client struct {
	graphqlClient *graphql.Client
	httpClient    *http.Client
	config        config.ShikimoriConfig
	log           *logger.Logger
	rateLimiter   *rateLimiter
}

type rateLimiter struct {
	mu       sync.Mutex
	tokens   int
	maxTokens int
	lastRefill time.Time
	interval time.Duration
}

func newRateLimiter(rps int) *rateLimiter {
	return &rateLimiter{
		tokens:    rps,
		maxTokens: rps,
		lastRefill: time.Now(),
		interval:  time.Second,
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

// NewClient creates a new Shikimori API client
func NewClient(cfg config.ShikimoriConfig, log *logger.Logger) *Client {
	httpClient := &http.Client{
		Timeout: cfg.Timeout,
		Transport: &userAgentTransport{
			userAgent: cfg.UserAgent,
			base:      http.DefaultTransport,
		},
	}

	return &Client{
		graphqlClient: graphql.NewClient(cfg.GraphQLURL, httpClient),
		httpClient:    httpClient,
		config:        cfg,
		log:           log,
		rateLimiter:   newRateLimiter(cfg.RateLimit),
	}
}

type userAgentTransport struct {
	userAgent string
	base      http.RoundTripper
}

func (t *userAgentTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	req.Header.Set("User-Agent", t.userAgent)
	return t.base.RoundTrip(req)
}

// GraphQL query types matching Shikimori's schema
type animeQuery struct {
	Animes []shikimoriAnime `graphql:"animes(search: $search, limit: $limit, page: $page, season: $season, score: $score, kind: $kind)"`
}

type animeByIDQuery struct {
	Animes []shikimoriAnime `graphql:"animes(ids: $ids, limit: 1)"`
}

type shikimoriAnime struct {
	ID          graphql.String  `graphql:"id"`
	Name        graphql.String  `graphql:"name"`
	Russian     graphql.String  `graphql:"russian"`
	Japanese    graphql.String  `graphql:"japanese"`
	Description graphql.String  `graphql:"description"`
	Score       graphql.Float   `graphql:"score"`
	Status      graphql.String  `graphql:"status"`
	Episodes    graphql.Int     `graphql:"episodes"`
	Duration    graphql.Int     `graphql:"duration"`
	AiredOn     *shikimoriDate  `graphql:"airedOn"`
	Poster      *shikimoriPoster `graphql:"poster"`
	Genres      []shikimoriGenre `graphql:"genres"`
	Videos      []shikimoriVideo `graphql:"videos"`
}

type shikimoriDate struct {
	Year  graphql.Int `graphql:"year"`
	Month graphql.Int `graphql:"month"`
	Day   graphql.Int `graphql:"day"`
}

type shikimoriPoster struct {
	OriginalURL graphql.String `graphql:"originalUrl"`
}

type shikimoriGenre struct {
	ID      graphql.String `graphql:"id"`
	Name    graphql.String `graphql:"name"`
	Russian graphql.String `graphql:"russian"`
}

type shikimoriVideo struct {
	ID       graphql.String `graphql:"id"`
	URL      graphql.String `graphql:"url"`
	Name     graphql.String `graphql:"name"`
	Kind     graphql.String `graphql:"kind"`
	PlayerURL graphql.String `graphql:"playerUrl"`
}

// SearchAnime searches for anime on Shikimori using raw GraphQL
func (c *Client) SearchAnime(ctx context.Context, query string, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	// Don't filter by kind to include TV, ONA, OVA, movies, etc.
	gqlQuery := fmt.Sprintf(`{
		animes(search: "%s", limit: %d, page: %d) {
			id name russian japanese description score status episodes duration
			airedOn { year month day }
			poster { originalUrl }
			genres { id name russian }
		}
	}`, query, limit, page)

	return c.executeRawQuery(ctx, gqlQuery)
}

func (c *Client) executeRawQuery(ctx context.Context, query string) ([]*domain.Anime, error) {
	reqBody := map[string]string{"query": query}
	jsonBody, _ := json.Marshal(reqBody)

	req, err := http.NewRequestWithContext(ctx, "POST", c.config.GraphQLURL, bytes.NewBuffer(jsonBody))
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	var result struct {
		Data struct {
			Animes []rawAnime `json:"animes"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}

	if len(result.Errors) > 0 {
		return nil, errors.ExternalAPI("shikimori", fmt.Errorf("%s", result.Errors[0].Message))
	}

	return c.mapRawAnimeList(result.Data.Animes), nil
}

type rawAnime struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Russian     string  `json:"russian"`
	Japanese    string  `json:"japanese"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	Status      string  `json:"status"`
	Episodes    int     `json:"episodes"`
	Duration    int     `json:"duration"`
	AiredOn     *struct {
		Year  int `json:"year"`
		Month int `json:"month"`
		Day   int `json:"day"`
	} `json:"airedOn"`
	Poster *struct {
		OriginalURL string `json:"originalUrl"`
	} `json:"poster"`
	Genres []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Russian string `json:"russian"`
	} `json:"genres"`
}

func (c *Client) mapRawAnimeList(animes []rawAnime) []*domain.Anime {
	result := make([]*domain.Anime, 0, len(animes))
	for _, a := range animes {
		anime := &domain.Anime{
			ShikimoriID:     a.ID,
			Name:            a.Name,
			NameRU:          a.Russian,
			NameJP:          a.Japanese,
			Description:     a.Description,
			Score:           a.Score,
			Status:          mapStatus(a.Status),
			EpisodesCount:   a.Episodes,
			EpisodeDuration: a.Duration,
		}
		if a.AiredOn != nil {
			anime.Year = a.AiredOn.Year
			anime.Season = detectSeason(a.AiredOn.Month)
		}
		if a.Poster != nil {
			anime.PosterURL = a.Poster.OriginalURL
		}
		for _, g := range a.Genres {
			anime.Genres = append(anime.Genres, domain.Genre{
				ID:     g.ID,
				Name:   g.Name,
				NameRU: g.Russian,
			})
		}
		result = append(result, anime)
	}
	return result
}

// GetAnimeByID fetches a specific anime by Shikimori ID
func (c *Client) GetAnimeByID(ctx context.Context, shikimoriID string) (*domain.Anime, error) {
	c.rateLimiter.acquire()

	var q animeByIDQuery
	variables := map[string]interface{}{
		"ids": graphql.String(shikimoriID),
	}

	if err := c.graphqlClient.Query(ctx, &q, variables); err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}

	if len(q.Animes) == 0 {
		return nil, errors.NotFound("anime")
	}

	animes := c.mapAnimeList(q.Animes)
	return animes[0], nil
}

// GetTrendingAnime fetches trending anime (high score, ongoing)
func (c *Client) GetTrendingAnime(ctx context.Context, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	gqlQuery := fmt.Sprintf(`{
		animes(limit: %d, page: %d, status: "ongoing", order: ranked, kind: "tv") {
			id name russian japanese description score status episodes duration
			airedOn { year month day }
			poster { originalUrl }
			genres { id name russian }
		}
	}`, limit, page)

	return c.executeRawQuery(ctx, gqlQuery)
}

// GetPopularAnime fetches popular anime (all time)
func (c *Client) GetPopularAnime(ctx context.Context, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	gqlQuery := fmt.Sprintf(`{
		animes(limit: %d, page: %d, order: popularity, kind: "tv") {
			id name russian japanese description score status episodes duration
			airedOn { year month day }
			poster { originalUrl }
			genres { id name russian }
		}
	}`, limit, page)

	return c.executeRawQuery(ctx, gqlQuery)
}

// GetSeasonalAnime fetches anime for a specific season
func (c *Client) GetSeasonalAnime(ctx context.Context, year int, season string, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	seasonStr := fmt.Sprintf("%s_%d", season, year)

	var q animeQuery
	variables := map[string]interface{}{
		"search": (*graphql.String)(nil),
		"limit":  graphql.Int(limit),
		"page":   graphql.Int(page),
		"season": graphql.String(seasonStr),
		"score":  (*graphql.Int)(nil),
		"kind":   graphql.String("tv"),
	}

	if err := c.graphqlClient.Query(ctx, &q, variables); err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}

	return c.mapAnimeList(q.Animes), nil
}

func (c *Client) mapAnimeList(shikimoriAnimes []shikimoriAnime) []*domain.Anime {
	animes := make([]*domain.Anime, 0, len(shikimoriAnimes))
	for _, sa := range shikimoriAnimes {
		animes = append(animes, c.mapAnime(sa))
	}
	return animes
}

func (c *Client) mapAnime(sa shikimoriAnime) *domain.Anime {
	anime := &domain.Anime{
		ShikimoriID:     string(sa.ID),
		Name:            string(sa.Name),
		NameRU:          string(sa.Russian),
		NameJP:          string(sa.Japanese),
		Description:     string(sa.Description),
		Score:           float64(sa.Score),
		Status:          mapStatus(string(sa.Status)),
		EpisodesCount:   int(sa.Episodes),
		EpisodeDuration: int(sa.Duration),
	}

	if sa.AiredOn != nil {
		anime.Year = int(sa.AiredOn.Year)
		anime.Season = detectSeason(int(sa.AiredOn.Month))
	}

	if sa.Poster != nil {
		anime.PosterURL = string(sa.Poster.OriginalURL)
	}

	// Map genres
	for _, g := range sa.Genres {
		anime.Genres = append(anime.Genres, domain.Genre{
			ID:     string(g.ID),
			Name:   string(g.Name),
			NameRU: string(g.Russian),
		})
	}

	return anime
}

func mapStatus(status string) domain.AnimeStatus {
	switch status {
	case "ongoing":
		return domain.StatusOngoing
	case "released":
		return domain.StatusReleased
	case "anons":
		return domain.StatusAnnounced
	default:
		return domain.StatusReleased
	}
}

func detectSeason(month int) string {
	switch {
	case month >= 1 && month <= 3:
		return "winter"
	case month >= 4 && month <= 6:
		return "spring"
	case month >= 7 && month <= 9:
		return "summer"
	default:
		return "fall"
	}
}

// GetAnimeVideos fetches video links (openings/endings) for an anime
func (c *Client) GetAnimeVideos(ctx context.Context, shikimoriID string) ([]*domain.Video, error) {
	c.rateLimiter.acquire()

	var q animeByIDQuery
	variables := map[string]interface{}{
		"ids": graphql.String(shikimoriID),
	}

	if err := c.graphqlClient.Query(ctx, &q, variables); err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}

	if len(q.Animes) == 0 {
		return nil, errors.NotFound("anime")
	}

	anime := q.Animes[0]
	videos := make([]*domain.Video, 0, len(anime.Videos))

	for _, v := range anime.Videos {
		videoType := mapVideoKind(string(v.Kind))
		if videoType == "" {
			continue
		}

		videos = append(videos, &domain.Video{
			ID:         string(v.ID),
			AnimeID:    shikimoriID,
			Type:       videoType,
			Name:       string(v.Name),
			SourceType: domain.SourceTypeExternal,
			SourceURL:  string(v.URL),
		})
	}

	return videos, nil
}

func mapVideoKind(kind string) domain.VideoType {
	switch kind {
	case "op", "opening":
		return domain.VideoTypeOpening
	case "ed", "ending":
		return domain.VideoTypeEnding
	default:
		return ""
	}
}

// ExtractShikimoriID extracts ID from various Shikimori URL formats
func ExtractShikimoriID(input string) string {
	// Handle direct ID
	if _, err := strconv.Atoi(input); err == nil {
		return input
	}

	// TODO: Parse URL formats like https://shikimori.one/animes/z5114
	return input
}
