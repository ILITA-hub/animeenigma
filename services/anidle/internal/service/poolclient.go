package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// PoolClient fetches the guess pool from catalog's internal endpoint.
type PoolClient struct {
	baseURL string
	client  *http.Client
	log     *logger.Logger
}

func NewPoolClient(catalogURL string, timeout time.Duration, log *logger.Logger) *PoolClient {
	if timeout <= 0 {
		timeout = 60 * time.Second
	}
	return &PoolClient{
		baseURL: catalogURL,
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}
}

type poolEnvelope struct {
	Success bool               `json:"success"`
	Data    []domain.PoolAnime `json:"data"`
}

// Fetch GETs /internal/guessgame/pool and decodes the {success,data} envelope.
func (c *PoolClient) Fetch(ctx context.Context) ([]domain.PoolAnime, error) {
	endpoint := c.baseURL + "/internal/guessgame/pool"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build pool request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pool request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pool endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var env poolEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode pool envelope: %w", err)
	}
	return env.Data, nil
}
