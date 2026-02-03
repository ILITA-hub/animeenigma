package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/repo"
)

// MALResolver resolves MAL IDs to Shikimori IDs and loads anime
type MALResolver struct {
	mappingRepo *repo.MappingRepository
	config      *config.JobsConfig
	httpClient  *http.Client
	log         *logger.Logger
}

// ResolutionResult represents the result of resolving a MAL ID
type ResolutionResult struct {
	ShikimoriID string
	AnimeID     string
	Method      domain.ResolutionMethod
	Confidence  float64
	Error       error
}

// ShikimoriSearchResult represents a single search result from Shikimori
type ShikimoriSearchResult struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Japanese string `json:"japanese"`
	Russian  string `json:"russian"`
}

// CatalogAnime represents anime from the catalog service
type CatalogAnime struct {
	ID          string `json:"id"`
	ShikimoriID string `json:"shikimori_id"`
	Name        string `json:"name"`
	NameJP      string `json:"name_jp"`
	MALID       int    `json:"mal_id,omitempty"`
}

// NewMALResolver creates a new MAL resolver
func NewMALResolver(
	mappingRepo *repo.MappingRepository,
	config *config.JobsConfig,
	log *logger.Logger,
) *MALResolver {
	return &MALResolver{
		mappingRepo: mappingRepo,
		config:      config,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		log: log,
	}
}

// Resolve attempts to resolve a MAL ID to a Shikimori ID and local anime ID
func (r *MALResolver) Resolve(ctx context.Context, task *domain.AnimeLoadTask) *ResolutionResult {
	// Step 1: Check cached mapping
	if mapping, err := r.mappingRepo.GetByMALID(ctx, task.MALID); err == nil {
		r.log.Debugw("found cached mapping",
			"mal_id", task.MALID,
			"shikimori_id", mapping.ShikimoriID,
		)
		return &ResolutionResult{
			ShikimoriID: mapping.ShikimoriID,
			AnimeID:     mapping.AnimeID,
			Method:      domain.ResolutionCached,
			Confidence:  mapping.Confidence,
		}
	}

	// Step 2: Check if anime already exists in catalog by MAL ID
	if catalogAnime := r.searchCatalogByMALID(ctx, task.MALID); catalogAnime != nil {
		r.log.Debugw("found anime in catalog by MAL ID",
			"mal_id", task.MALID,
			"anime_id", catalogAnime.ID,
		)
		// Cache the mapping
		if catalogAnime.ShikimoriID != "" {
			_ = r.mappingRepo.Create(ctx, &domain.MALShikimoriMapping{
				MALID:       task.MALID,
				ShikimoriID: catalogAnime.ShikimoriID,
				AnimeID:     catalogAnime.ID,
				Confidence:  1.0,
				Source:      domain.MappingSourceShikimoriAPI,
			})
		}
		return &ResolutionResult{
			ShikimoriID: catalogAnime.ShikimoriID,
			AnimeID:     catalogAnime.ID,
			Method:      domain.ResolutionCached,
			Confidence:  1.0,
		}
	}

	// Step 3: Try exact Japanese title match on Shikimori
	if task.MALTitleJapanese != "" {
		if result := r.searchShikimoriExact(ctx, task.MALTitleJapanese, true); result != nil {
			r.log.Infow("found exact Japanese title match",
				"mal_id", task.MALID,
				"mal_title", task.MALTitle,
				"shikimori_id", result.ShikimoriID,
			)
			return result
		}
	}

	// Step 4: Try exact romanized name match on Shikimori
	if result := r.searchShikimoriExact(ctx, task.MALTitle, false); result != nil {
		r.log.Infow("found exact romanized name match",
			"mal_id", task.MALID,
			"mal_title", task.MALTitle,
			"shikimori_id", result.ShikimoriID,
		)
		return result
	}

	// No exact match found - requires manual resolution
	r.log.Warnw("no exact match found, requires manual resolution",
		"mal_id", task.MALID,
		"mal_title", task.MALTitle,
	)
	return &ResolutionResult{
		Method: domain.ResolutionNotFound,
	}
}

// searchShikimoriExact searches Shikimori for an exact title match
func (r *MALResolver) searchShikimoriExact(ctx context.Context, title string, isJapanese bool) *ResolutionResult {
	results, err := r.searchShikimori(ctx, title)
	if err != nil {
		r.log.Warnw("Shikimori search failed",
			"title", title,
			"error", err,
		)
		return nil
	}

	// Look for exact match
	normalizedTitle := normalizeTitle(title)
	for _, result := range results {
		var compareTitle string
		if isJapanese {
			compareTitle = result.Japanese
		} else {
			compareTitle = result.Name
		}

		if normalizeTitle(compareTitle) == normalizedTitle {
			method := domain.ResolutionExactRomanized
			if isJapanese {
				method = domain.ResolutionExactJapanese
			}
			return &ResolutionResult{
				ShikimoriID: result.ID,
				Method:      method,
				Confidence:  1.0,
			}
		}
	}

	return nil
}

// searchShikimori searches for anime on Shikimori
func (r *MALResolver) searchShikimori(ctx context.Context, query string) ([]ShikimoriSearchResult, error) {
	url := fmt.Sprintf("%s/api/anime?search=%s&limit=10", r.config.ShikimoriAPIURL, neturl.QueryEscape(query))

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", r.config.ShikimoriAppName)

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("shikimori returned status %d", resp.StatusCode)
	}

	var results []ShikimoriSearchResult
	if err := json.NewDecoder(resp.Body).Decode(&results); err != nil {
		return nil, err
	}

	return results, nil
}

// searchCatalogByMALID searches the catalog service for anime by MAL ID
func (r *MALResolver) searchCatalogByMALID(ctx context.Context, malID int) *CatalogAnime {
	url := fmt.Sprintf("%s/api/anime/mal/%d", r.config.CatalogServiceURL, malID)

	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return nil
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil
	}

	var result struct {
		Data CatalogAnime `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil
	}

	if result.Data.ID != "" {
		return &result.Data
	}

	return nil
}

// LoadAnimeFromShikimori loads an anime from Shikimori into the catalog
func (r *MALResolver) LoadAnimeFromShikimori(ctx context.Context, shikimoriID string, malID int) (string, error) {
	// Call catalog service to load anime by Shikimori ID
	url := fmt.Sprintf("%s/api/anime/shikimori/%s", r.config.CatalogServiceURL, shikimoriID)

	req, err := http.NewRequestWithContext(ctx, "POST", url, nil)
	if err != nil {
		return "", err
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("catalog service returned status %d", resp.StatusCode)
	}

	var result struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", err
	}

	animeID := result.Data.ID

	// Update mapping with anime ID
	if animeID != "" {
		_ = r.mappingRepo.Create(ctx, &domain.MALShikimoriMapping{
			MALID:       malID,
			ShikimoriID: shikimoriID,
			AnimeID:     animeID,
			Confidence:  1.0,
			Source:      domain.MappingSourceShikimoriAPI,
		})
	}

	return animeID, nil
}

// SaveMapping saves a MAL to Shikimori mapping
func (r *MALResolver) SaveMapping(ctx context.Context, malID int, shikimoriID, animeID string, method domain.ResolutionMethod) error {
	confidence := 1.0
	if method == domain.ResolutionUserSelected {
		confidence = 0.9 // Slightly lower confidence for user-selected mappings
	}

	source := domain.MappingSourceTitleSearch
	if method == domain.ResolutionUserSelected {
		source = domain.MappingSourceManual
	}

	return r.mappingRepo.Create(ctx, &domain.MALShikimoriMapping{
		MALID:       malID,
		ShikimoriID: shikimoriID,
		AnimeID:     animeID,
		Confidence:  confidence,
		Source:      source,
	})
}

// normalizeTitle normalizes a title for comparison
func normalizeTitle(title string) string {
	// Convert to lowercase and trim whitespace
	normalized := strings.ToLower(strings.TrimSpace(title))

	// Remove common variations
	normalized = strings.ReplaceAll(normalized, ":", "")
	normalized = strings.ReplaceAll(normalized, "-", " ")
	normalized = strings.ReplaceAll(normalized, "  ", " ")

	return normalized
}
