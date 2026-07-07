package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	chdriver "github.com/ClickHouse/clickhouse-go/v2/lib/driver"

	"github.com/ILITA-hub/animeenigma/services/analytics/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/analytics/internal/service"
)

// PlayabilityHandler serves GET /internal/playability?anime_id=<id> with
// per-provider decayed watch/probe weights. Docker-network-only.
type PlayabilityHandler struct {
	conn chdriver.Conn
}

// NewPlayabilityHandler builds the handler over the shared CH connection.
func NewPlayabilityHandler(conn chdriver.Conn) *PlayabilityHandler {
	return &PlayabilityHandler{conn: conn}
}

type playabilityResponse struct {
	Providers service.PlayabilityScores `json:"providers"`
}

func (h *PlayabilityHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	animeID := r.URL.Query().Get("anime_id")

	ctx, cancel := context.WithTimeout(r.Context(), 60*time.Second)
	defer cancel()

	watch, err := repo.QueryPlayabilityWatch(ctx, h.conn, animeID)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	probe, err := repo.QueryPlayabilityProbeUp(ctx, h.conn)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}

	scores := service.BuildPlayabilityScores(watch, probe, func(p string) bool {
		_, ok := knownProviders[p]
		return ok
	})

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(playabilityResponse{Providers: scores})
}
