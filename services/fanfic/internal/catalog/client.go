// Package catalog is a thin, fail-soft client for the catalog service's public
// anime endpoint, used to preload an anime's real synopsis for canon-mode
// generation. Mirrors services/anidle/internal/service/poolclient.go.
package catalog

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
)

type Client struct {
	baseURL string
	client  *http.Client
	log     *logger.Logger
}

func NewClient(baseURL string, timeout time.Duration, log *logger.Logger) *Client {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}
}

// animeEnvelope mirrors the JSON tags of domain.Anime (services/catalog/internal/domain/anime.go)
// as returned by GET /api/anime/{id} — {"success":true,"data":{...}}. Only the
// fields this client needs are decoded. Note the japanese title is tagged
// "name_jp" on the catalog side (NOT "japanese").
type animeEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		NameRU      string `json:"name_ru"`
		NameJP      string `json:"name_jp"`
		Description string `json:"description"`
		PosterURL   string `json:"poster_url"`
	} `json:"data"`
}

// fetchEnvelope resolves the anime endpoint (catalog uuid preferred, falling
// back to the shikimori resolve route) and decodes the response envelope.
// Shared by FetchSynopsis and FetchMeta. Any transport/decoding failure
// returns a non-nil error and a zero-value envelope so callers can degrade
// gracefully.
func (c *Client) fetchEnvelope(ctx context.Context, animeID, shikimoriID string) (animeEnvelope, error) {
	var endpoint string
	switch {
	case strings.TrimSpace(animeID) != "":
		endpoint = c.baseURL + "/api/anime/" + url.PathEscape(animeID)
	case strings.TrimSpace(shikimoriID) != "":
		endpoint = c.baseURL + "/api/anime/shikimori/" + url.PathEscape(shikimoriID)
	default:
		return animeEnvelope{}, fmt.Errorf("no anime id or shikimori_id")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return animeEnvelope{}, fmt.Errorf("build anime request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return animeEnvelope{}, fmt.Errorf("anime request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return animeEnvelope{}, fmt.Errorf("anime endpoint returned %d", resp.StatusCode)
	}

	var env animeEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return animeEnvelope{}, fmt.Errorf("decode anime envelope: %w", err)
	}
	return env, nil
}

// FetchSynopsis returns (canonicalTitle, synopsis). Prefers the catalog uuid;
// falls back to the shikimori resolve route when only a shikimori id is given.
// Any transport/decoding failure returns a non-nil error and empty strings so
// the caller can degrade gracefully (canon gen proceeds without the preload).
func (c *Client) FetchSynopsis(ctx context.Context, animeID, shikimoriID string) (string, string, error) {
	env, err := c.fetchEnvelope(ctx, animeID, shikimoriID)
	if err != nil {
		return "", "", err
	}
	title := env.Data.Name
	if title == "" {
		title = env.Data.NameRU
	}
	return title, env.Data.Description, nil
}

// AnimeMeta is id/title/japanese/poster/synopsis metadata for bot fanfic
// generation (the daily spotlight generator needs the poster, which
// FetchSynopsis doesn't return). ID is the catalog uuid — callers that only
// have a shikimori id (e.g. the daily bot generator) need it back to
// populate domain.Fanfic.AnimeID, which is a uuid-typed column.
type AnimeMeta struct {
	ID, Title, Japanese, Poster, Synopsis string
}

// FetchMeta returns id/title/japanese/poster/synopsis for bot fanfic generation.
// Fail-soft: transport/decode errors return a zero AnimeMeta + error.
func (c *Client) FetchMeta(ctx context.Context, animeID, shikimoriID string) (AnimeMeta, error) {
	env, err := c.fetchEnvelope(ctx, animeID, shikimoriID)
	if err != nil {
		return AnimeMeta{}, err
	}
	title := env.Data.Name
	if title == "" {
		title = env.Data.NameRU
	}
	return AnimeMeta{ID: env.Data.ID, Title: title, Japanese: env.Data.NameJP, Poster: env.Data.PosterURL, Synopsis: env.Data.Description}, nil
}
