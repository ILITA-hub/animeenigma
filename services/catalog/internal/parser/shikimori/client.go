package shikimori

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
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
	mu         sync.Mutex
	tokens     int
	maxTokens  int
	lastRefill time.Time
	interval   time.Duration
}

func newRateLimiter(rps int) *rateLimiter {
	return &rateLimiter{
		tokens:     rps,
		maxTokens:  rps,
		lastRefill: time.Now(),
		interval:   time.Second,
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
	ID          graphql.String `graphql:"id"`
	Name        graphql.String `graphql:"name"`
	English     graphql.String `graphql:"english"`
	Russian     graphql.String `graphql:"russian"`
	Japanese    graphql.String `graphql:"japanese"`
	Description graphql.String `graphql:"description"`
	Score       graphql.Float  `graphql:"score"`
	Status      graphql.String `graphql:"status"`
	// Phase 12 (Decision §A1) — S5 attribute dimensions.
	// Kind (TV/Movie/OVA/...), Rating (G/PG/PG-13/R/R+/Rx — used as the
	// S5 demographic proxy per Decision §A3), and the adaptation source
	// (manga/light_novel/original/...) populate animes.kind, animes.rating,
	// animes.material_source.
	//
	// Shikimori's GraphQL schema names the adaptation-source field "origin"
	// (verified via introspection 2026-05-06). The CONTEXT.md spec called
	// it "source" — the live API rejects that field with "Field 'source'
	// doesn't exist on type 'Anime'", so we query "origin" and surface it
	// as Source on the parser-local struct.
	Kind          graphql.String   `graphql:"kind"`
	Rating        graphql.String   `graphql:"rating"`
	Source        graphql.String   `graphql:"origin"`
	Episodes      graphql.Int      `graphql:"episodes"`
	EpisodesAired graphql.Int      `graphql:"episodesAired"`
	Duration      graphql.Int      `graphql:"duration"`
	MalId         graphql.String   `graphql:"malId"`
	AiredOn       *shikimoriDate   `graphql:"airedOn"`
	NextEpisodeAt graphql.String   `graphql:"nextEpisodeAt"`
	Poster        *shikimoriPoster `graphql:"poster"`
	Genres        []shikimoriGenre `graphql:"genres"`
	// Phase 12 (Decision §A1/A2) — Shikimori does not separate producers,
	// so this single Studios payload feeds both spec dimensions, collapsed
	// in S5 to a 0.25 weight.
	Studios []shikimoriStudio `graphql:"studios"`
	Videos  []shikimoriVideo  `graphql:"videos"`
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

// shikimoriStudio mirrors shikimoriGenre minus the Russian field —
// Shikimori's studios payload has no Russian translations (Phase 12 §A1).
type shikimoriStudio struct {
	ID   graphql.String `graphql:"id"`
	Name graphql.String `graphql:"name"`
}

type shikimoriVideo struct {
	ID        graphql.String `graphql:"id"`
	URL       graphql.String `graphql:"url"`
	Name      graphql.String `graphql:"name"`
	Kind      graphql.String `graphql:"kind"`
	PlayerURL graphql.String `graphql:"playerUrl"`
}

// SearchAnime searches for anime on Shikimori using raw GraphQL
func (c *Client) SearchAnime(ctx context.Context, query string, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	// Don't filter by kind to include TV, ONA, OVA, movies, etc.
	gqlQuery := fmt.Sprintf(`{
		animes(search: "%s", limit: %d, page: %d) {
			id name english russian japanese description score status kind rating origin episodes episodesAired duration
			airedOn { year month day }
			nextEpisodeAt
			malId
			poster { originalUrl }
			genres { id name russian }
			studios { id name }
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
	English     string  `json:"english"`
	Russian     string  `json:"russian"`
	Japanese    string  `json:"japanese"`
	Description string  `json:"description"`
	Score       float64 `json:"score"`
	Status      string  `json:"status"`
	// Phase 12 (Decision §A1) — S5 attribute dimensions. The adaptation
	// source field is "origin" in Shikimori's GraphQL schema; we keep the
	// Go field name `Source` for clarity at the boundary with domain.Anime
	// (column: material_source).
	Kind          string `json:"kind"`
	Rating        string `json:"rating"`
	Source        string `json:"origin"`
	Episodes      int    `json:"episodes"`
	EpisodesAired int    `json:"episodesAired"`
	Duration      int    `json:"duration"`
	NextEpisodeAt string `json:"nextEpisodeAt"`
	MalID         string `json:"malId"`
	AiredOn       *struct {
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
	// Phase 12 (Decision §A1/A2) — studios. Shikimori does not separate
	// producers, so this single payload feeds the collapsed S5 studios
	// dimension at weight 0.25.
	Studios []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"studios"`
}

func (c *Client) mapRawAnimeList(animes []rawAnime) []*domain.Anime {
	result := make([]*domain.Anime, 0, len(animes))
	for _, a := range animes {
		anime := &domain.Anime{
			ShikimoriID:     a.ID,
			Name:            a.Name,
			NameEN:          a.English,
			NameRU:          a.Russian,
			NameJP:          a.Japanese,
			Description:     a.Description,
			Score:           a.Score,
			Status:          mapStatus(a.Status),
			EpisodesCount:   a.Episodes,
			EpisodesAired:   a.EpisodesAired,
			EpisodeDuration: a.Duration,
			MALID:           a.MalID,
		}
		// Phase 12 (Decision §A1) — S5 attribute dimensions.
		anime.Kind = a.Kind
		anime.Rating = a.Rating
		anime.MaterialSource = a.Source
		if a.AiredOn != nil {
			anime.Year = a.AiredOn.Year
			anime.Season = detectSeason(a.AiredOn.Month)
			// Parse full aired date
			if a.AiredOn.Year > 0 && a.AiredOn.Month > 0 && a.AiredOn.Day > 0 {
				airedDate := time.Date(a.AiredOn.Year, time.Month(a.AiredOn.Month), a.AiredOn.Day, 0, 0, 0, 0, time.UTC)
				anime.AiredOn = &airedDate
			}
		}
		if a.NextEpisodeAt != "" {
			if nextEp, err := time.Parse(time.RFC3339, a.NextEpisodeAt); err == nil {
				anime.NextEpisodeAt = &nextEp
			}
		}
		if a.Poster != nil {
			anime.PosterURL = a.Poster.OriginalURL
		}
		for _, g := range a.Genres {
			anime.Genres = append(anime.Genres, domain.Genre{
				ID:     g.ID, // Shikimori genre ID
				Name:   g.Name,
				NameRU: g.Russian,
			})
		}
		// Phase 12 (Decision §A1/A2) — studios m2m hydration.
		for _, st := range a.Studios {
			anime.Studios = append(anime.Studios, domain.Studio{
				ID:   st.ID,
				Name: st.Name,
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

// GetAnimeByIDs fetches multiple anime by Shikimori IDs in a single batch request.
// Caller must chunk to max 50 IDs per call.
func (c *Client) GetAnimeByIDs(ctx context.Context, shikimoriIDs []string) ([]*domain.Anime, error) {
	if len(shikimoriIDs) == 0 {
		return nil, nil
	}
	c.rateLimiter.acquire()

	ids := strings.Join(shikimoriIDs, ",")
	gqlQuery := fmt.Sprintf(`{
		animes(ids: "%s", limit: %d) {
			id name english russian japanese description score status kind rating origin episodes episodesAired duration
			airedOn { year month day }
			nextEpisodeAt
			malId
			poster { originalUrl }
			genres { id name russian }
			studios { id name }
		}
	}`, ids, len(shikimoriIDs))

	return c.executeRawQuery(ctx, gqlQuery)
}

// GetTrendingAnime fetches top-ranked anime (all statuses, ordered by Shikimori ranking)
func (c *Client) GetTrendingAnime(ctx context.Context, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	gqlQuery := fmt.Sprintf(`{
		animes(limit: %d, page: %d, order: ranked) {
			id name english russian japanese description score status kind rating origin episodes episodesAired duration
			airedOn { year month day }
			nextEpisodeAt
			malId
			poster { originalUrl }
			genres { id name russian }
			studios { id name }
		}
	}`, limit, page)

	return c.executeRawQuery(ctx, gqlQuery)
}

// GetPopularAnime fetches popular anime (all time)
func (c *Client) GetPopularAnime(ctx context.Context, page, limit int) ([]*domain.Anime, error) {
	c.rateLimiter.acquire()

	gqlQuery := fmt.Sprintf(`{
		animes(limit: %d, page: %d, order: popularity) {
			id name english russian japanese description score status kind rating origin episodes episodesAired duration
			airedOn { year month day }
			nextEpisodeAt
			malId
			poster { originalUrl }
			genres { id name russian }
			studios { id name }
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
		NameEN:          string(sa.English),
		NameRU:          string(sa.Russian),
		NameJP:          string(sa.Japanese),
		Description:     string(sa.Description),
		Score:           float64(sa.Score),
		Status:          mapStatus(string(sa.Status)),
		EpisodesCount:   int(sa.Episodes),
		EpisodesAired:   int(sa.EpisodesAired),
		EpisodeDuration: int(sa.Duration),
		MALID:           string(sa.MalId),
	}

	// Phase 12 (Decision §A1) — S5 attribute dimensions.
	anime.Kind = string(sa.Kind)
	anime.Rating = string(sa.Rating)
	anime.MaterialSource = string(sa.Source)

	if sa.AiredOn != nil {
		anime.Year = int(sa.AiredOn.Year)
		anime.Season = detectSeason(int(sa.AiredOn.Month))
		// Parse full aired date
		year := int(sa.AiredOn.Year)
		month := int(sa.AiredOn.Month)
		day := int(sa.AiredOn.Day)
		if year > 0 && month > 0 && day > 0 {
			airedDate := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
			anime.AiredOn = &airedDate
		}
	}

	if string(sa.NextEpisodeAt) != "" {
		if nextEp, err := time.Parse(time.RFC3339, string(sa.NextEpisodeAt)); err == nil {
			anime.NextEpisodeAt = &nextEp
		}
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

	// Phase 12 (Decision §A1/A2) — studios m2m hydration.
	for _, st := range sa.Studios {
		anime.Studios = append(anime.Studios, domain.Studio{
			ID:   string(st.ID),
			Name: string(st.Name),
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
	case month >= 10 && month <= 12:
		return "fall"
	default:
		return ""
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

// CalendarEntry represents an entry from the Shikimori /api/calendar endpoint
type CalendarEntry struct {
	NextEpisode   int    `json:"next_episode"`
	NextEpisodeAt string `json:"next_episode_at"`
	Anime         struct {
		ID            int    `json:"id"`
		Name          string `json:"name"`
		Russian       string `json:"russian"`
		Score         string `json:"score"`
		Status        string `json:"status"`
		Episodes      int    `json:"episodes"`
		EpisodesAired int    `json:"episodes_aired"`
		AiredOn       string `json:"aired_on"`
		Image         struct {
			Original string `json:"original"`
			Preview  string `json:"preview"`
		} `json:"image"`
	} `json:"anime"`
}

// GetCalendar fetches the upcoming episode calendar from Shikimori REST API.
// Returns a list of calendar entries with anime ID, next episode number, and air time.
func (c *Client) GetCalendar(ctx context.Context) ([]CalendarEntry, error) {
	c.rateLimiter.acquire()

	url := c.config.BaseURL + "/api/calendar"
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.ExternalAPI("shikimori", fmt.Errorf("calendar returned status %d", resp.StatusCode))
	}

	var entries []CalendarEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, errors.ExternalAPI("shikimori", fmt.Errorf("decode calendar: %w", err))
	}

	return entries, nil
}

// relatedEntry represents a single related anime entry from the Shikimori REST API
type relatedEntry struct {
	Relation        string `json:"relation"`
	RelationRussian string `json:"relation_russian"`
	Anime           *struct {
		ID       int    `json:"id"`
		Name     string `json:"name"`
		Russian  string `json:"russian"`
		Score    string `json:"score"`
		Status   string `json:"status"`
		Episodes int    `json:"episodes"`
		AiredOn  string `json:"aired_on"`
		Image    *struct {
			Original string `json:"original"`
		} `json:"image"`
	} `json:"anime"`
}

// GetRelatedAnime fetches related anime (sequels, prequels, etc.) from the Shikimori REST API.
func (c *Client) GetRelatedAnime(ctx context.Context, shikimoriID string) ([]domain.RelatedAnime, error) {
	c.rateLimiter.acquire()

	url := fmt.Sprintf("%s/api/animes/%s/related", c.config.BaseURL, shikimoriID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, errors.ExternalAPI("shikimori", fmt.Errorf("related returned status %d", resp.StatusCode))
	}

	var entries []relatedEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, errors.ExternalAPI("shikimori", fmt.Errorf("decode related: %w", err))
	}

	var result []domain.RelatedAnime
	for _, e := range entries {
		if e.Anime == nil {
			continue // manga-only relation
		}
		ra := domain.RelatedAnime{
			ShikimoriID: strconv.Itoa(e.Anime.ID),
			Name:        e.Anime.Name,
			NameRU:      e.Anime.Russian,
			RelationRU:  e.RelationRussian,
			RelationEN:  e.Relation,
			Status:      e.Anime.Status,
			Episodes:    e.Anime.Episodes,
		}
		// aired_on is "YYYY-MM-DD" (sometimes null/empty for announcements)
		if len(e.Anime.AiredOn) >= 4 {
			if y, err := strconv.Atoi(e.Anime.AiredOn[:4]); err == nil {
				ra.Year = y
			}
		}
		if score, err := strconv.ParseFloat(e.Anime.Score, 64); err == nil {
			ra.Score = score
		}
		if e.Anime.Image != nil && e.Anime.Image.Original != "" {
			ra.PosterURL = "https://shikimori.io" + e.Anime.Image.Original
		}
		result = append(result, ra)
	}

	return result, nil
}

// similarEntry represents a single similar anime entry from the Shikimori REST
// API. Unlike /related (which wraps each anime in a {relation, anime} object),
// /similar returns a flat array of anime objects.
type similarEntry struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Russian  string `json:"russian"`
	Score    string `json:"score"`
	Episodes int    `json:"episodes"`
	Status   string `json:"status"`
	Image    *struct {
		Original string `json:"original"`
	} `json:"image"`
}

// GetSimilarAnime fetches similar anime from the Shikimori REST API.
// Phase 13 (REC-SIG-06) — feeds the S6 cascade's Shikimori fallback path
// when the local rec_completion_co_occurrence pool yields fewer than 5
// post-S11-filter candidates.
//
// Mirrors GetRelatedAnime in shape (rate-limited, structured logging,
// /api/animes/:id/similar REST endpoint). Returns an empty slice when
// Shikimori has no similar list for this anime.
func (c *Client) GetSimilarAnime(ctx context.Context, shikimoriID string) ([]domain.SimilarAnime, error) {
	c.rateLimiter.acquire()

	url := fmt.Sprintf("%s/api/animes/%s/similar", c.config.BaseURL, shikimoriID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// 404 means "no similar entries" — treat as empty list, not an error.
		return nil, nil
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.ExternalAPI("shikimori", fmt.Errorf("similar returned status %d", resp.StatusCode))
	}

	var entries []similarEntry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, errors.ExternalAPI("shikimori", fmt.Errorf("decode similar: %w", err))
	}

	var result []domain.SimilarAnime
	for _, e := range entries {
		sa := domain.SimilarAnime{
			ShikimoriID: strconv.Itoa(e.ID),
			Name:        e.Name,
			NameRU:      e.Russian,
			Episodes:    e.Episodes,
			Status:      e.Status,
		}
		if score, err := strconv.ParseFloat(e.Score, 64); err == nil {
			sa.Score = score
		}
		if e.Image != nil && e.Image.Original != "" {
			sa.PosterURL = "https://shikimori.io" + e.Image.Original
		}
		result = append(result, sa)
	}

	return result, nil
}

// GetAnimeFranchise fetches the franchise slug for an anime from the Shikimori
// REST API (GET /api/animes/{id}). Shikimori's GraphQL schema does NOT expose
// `franchise`, so this single-anime REST call is the only source. Returns an
// empty string when the anime has no franchise or does not exist (404).
func (c *Client) GetAnimeFranchise(ctx context.Context, shikimoriID string) (string, error) {
	c.rateLimiter.acquire()

	url := fmt.Sprintf("%s/api/animes/%s", c.config.BaseURL, shikimoriID)
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return "", errors.ExternalAPI("shikimori", err)
	}
	req.Header.Set("User-Agent", c.config.UserAgent)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", errors.ExternalAPI("shikimori", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		return "", nil
	}
	if resp.StatusCode != http.StatusOK {
		return "", errors.ExternalAPI("shikimori", fmt.Errorf("anime detail returned status %d", resp.StatusCode))
	}

	var detail struct {
		Franchise string `json:"franchise"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&detail); err != nil {
		return "", errors.ExternalAPI("shikimori", fmt.Errorf("decode anime detail: %w", err))
	}
	return detail.Franchise, nil
}

// shikimoriIDRe pulls the numeric anime ID out of the `/animes/<id>` path
// segment of a Shikimori URL, or out of a bare slugged id. Shikimori prefixes
// some ids with a literal "z" (e.g. z5114) and appends a "-slug" — both are
// optional. Anchored on `/animes/` or start-of-string so it never grabs stray
// digits elsewhere in a URL (ports, query params, etc.).
var shikimoriIDRe = regexp.MustCompile(`(?:/animes/|^)z?(\d+)`)

// ExtractShikimoriID extracts the numeric Shikimori ID from various inputs:
// a bare id ("5114"), a slugged id ("z5114", "5114-cowboy-bebop"), or a full
// URL on any Shikimori domain ("https://shikimori.one/animes/z5114-cowboy-bebop",
// shikimori.me, shikimori.io, …). Returns the input unchanged when no id is found.
func ExtractShikimoriID(input string) string {
	input = strings.TrimSpace(input)

	// Direct numeric ID — fast path.
	if _, err := strconv.Atoi(input); err == nil {
		return input
	}

	if m := shikimoriIDRe.FindStringSubmatch(input); m != nil {
		return m[1]
	}
	return input
}
