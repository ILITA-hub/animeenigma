package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

// InternalScraperProvidersHandler serves the scraper provider config + capability
// traits to the scraper service over the Docker network.
// Mounted OUTSIDE /api at the root router with NO middleware — same
// gateway-non-routing security model as /internal/cache/invalidate/raw/ and
// /internal/anime/{shikimoriId}/episodes (see internal_cache.go for the precedent).
// The gateway does NOT proxy /internal/*, so the route is reachable only from
// within the Docker network (spec 2026-06-15-scraper-capability-api).
type InternalScraperProvidersHandler struct {
	db *gorm.DB
}

// NewInternalScraperProvidersHandler constructs the handler.
func NewInternalScraperProvidersHandler(db *gorm.DB) *InternalScraperProvidersHandler {
	return &InternalScraperProvidersHandler{db: db}
}

// List handles GET /internal/scraper/providers.
// Returns all domain.ScraperProvider rows ordered by name as
// {"providers":[...]} inside the standard {success,data:{...}} envelope.
func (h *InternalScraperProvidersHandler) List(w http.ResponseWriter, r *http.Request) {
	var rows []domain.ScraperProvider
	if err := h.db.WithContext(r.Context()).Order("name asc").Find(&rows).Error; err != nil {
		httputil.Error(w, errors.Internal("failed to load scraper providers"))
		return
	}
	httputil.OK(w, map[string]any{"providers": rows})
}
