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

type animeEnvelope struct {
	Success bool `json:"success"`
	Data    struct {
		Name        string `json:"name"`
		NameRU      string `json:"name_ru"`
		Description string `json:"description"`
	} `json:"data"`
}

// FetchSynopsis returns (canonicalTitle, synopsis). Prefers the catalog uuid;
// falls back to the shikimori resolve route when only a shikimori id is given.
// Any transport/decoding failure returns a non-nil error and empty strings so
// the caller can degrade gracefully (canon gen proceeds without the preload).
func (c *Client) FetchSynopsis(ctx context.Context, animeID, shikimoriID string) (string, string, error) {
	var endpoint string
	switch {
	case strings.TrimSpace(animeID) != "":
		endpoint = c.baseURL + "/api/anime/" + url.PathEscape(animeID)
	case strings.TrimSpace(shikimoriID) != "":
		endpoint = c.baseURL + "/api/anime/shikimori/" + url.PathEscape(shikimoriID)
	default:
		return "", "", fmt.Errorf("no anime id or shikimori_id")
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", "", fmt.Errorf("build anime request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return "", "", fmt.Errorf("anime request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", "", fmt.Errorf("anime endpoint returned %d", resp.StatusCode)
	}

	var env animeEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return "", "", fmt.Errorf("decode anime envelope: %w", err)
	}
	title := env.Data.Name
	if title == "" {
		title = env.Data.NameRU
	}
	return title, env.Data.Description, nil
}
