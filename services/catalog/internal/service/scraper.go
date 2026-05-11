package service

import (
	"context"
	"errors"
	"strconv"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"github.com/google/uuid"
)

// ErrMalIDUnavailable is returned when the catalog row for the requested
// anime exists but has neither ShikimoriID nor MALID set, so we can't
// forward to the scraper. The handler layer maps this to 422
// Unprocessable Entity with a stable error body so the frontend can
// gracefully degrade ("English sources unavailable for this anime") rather
// than show a generic 5xx.
var ErrMalIDUnavailable = errors.New("mal_id unavailable for this anime")

// animeFetcher is the minimal interface scraperOps needs from the anime
// repository. The production wiring satisfies it via *repo.AnimeRepository.
type animeFetcher interface {
	GetByID(ctx context.Context, id string) (*domain.Anime, error)
}

// scraperForwarder is the minimal interface scraperOps needs from the
// thin HTTP scraper client. Production wiring satisfies it via
// *scraper.Client.
type scraperForwarder interface {
	GetEpisodes(ctx context.Context, malID int, prefer string) (int, []byte, error)
	GetServers(ctx context.Context, malID int, episodeID, prefer string) (int, []byte, error)
	GetStream(ctx context.Context, malID int, episodeID, serverID, category, prefer string) (int, []byte, error)
	GetHealth(ctx context.Context) (int, []byte, error)
}

// scraperOps holds the dependencies the scraper-related service methods
// need. It's an internal seam: CatalogService delegates to it so the
// resolve-MAL-ID + forward logic stays testable without standing up a
// real GORM repo or http server.
type scraperOps struct {
	animeRepo     animeFetcher
	scraperClient scraperForwarder
}

// resolveMALID looks up the catalog Anime by UUID and returns the MAL ID
// to forward to the scraper.
//
// Resolution chain (per plan 15-04 §Task 2 + project memory):
//  1. animeRepo.GetByID — if NotFound, surface as liberrors.NotFound("anime").
//  2. Try parsing ShikimoriID as int; Shikimori IDs equal MAL IDs for anime
//     (per `libs/idmapping` ARM mapping).
//  3. If ShikimoriID is empty or unparseable, try MALID.
//  4. Otherwise return ErrMalIDUnavailable so the handler can emit 422.
func (o *scraperOps) resolveMALID(ctx context.Context, animeID string) (int, error) {
	// Malformed UUID — skip the DB roundtrip entirely (Postgres would
	// otherwise reject with SQLSTATE 22P02, surfaced as a 500). Behave
	// as "anime not found" instead per plan 15-04 must_haves.truths.
	if _, err := uuid.Parse(animeID); err != nil {
		return 0, liberrors.NotFound("anime")
	}

	anime, err := o.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		// liberrors.NotFound from the repo is already the right shape for
		// the handler to render as 404 via httputil.Error.
		return 0, err
	}
	if anime == nil {
		// Defensive — the repo currently never returns (nil, nil), but if a
		// future implementation does we treat it as not-found.
		return 0, liberrors.NotFound("anime")
	}

	if anime.ShikimoriID != "" {
		if v, perr := strconv.Atoi(anime.ShikimoriID); perr == nil && v > 0 {
			return v, nil
		}
	}
	if anime.MALID != "" {
		if v, perr := strconv.Atoi(anime.MALID); perr == nil && v > 0 {
			return v, nil
		}
	}
	return 0, ErrMalIDUnavailable
}

// GetScraperEpisodes resolves animeID -> MAL ID and forwards to the
// scraper service. Returns the scraper's status + body verbatim so the
// handler can passthrough 503s (Phase 15 not-yet-implemented contract).
func (o *scraperOps) GetScraperEpisodes(ctx context.Context, animeID, prefer string) (int, []byte, error) {
	malID, err := o.resolveMALID(ctx, animeID)
	if err != nil {
		return 0, nil, err
	}
	return o.scraperClient.GetEpisodes(ctx, malID, prefer)
}

// GetScraperServers resolves animeID -> MAL ID and forwards.
func (o *scraperOps) GetScraperServers(ctx context.Context, animeID, episodeID, prefer string) (int, []byte, error) {
	malID, err := o.resolveMALID(ctx, animeID)
	if err != nil {
		return 0, nil, err
	}
	return o.scraperClient.GetServers(ctx, malID, episodeID, prefer)
}

// GetScraperStream resolves animeID -> MAL ID and forwards.
func (o *scraperOps) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string) (int, []byte, error) {
	malID, err := o.resolveMALID(ctx, animeID)
	if err != nil {
		return 0, nil, err
	}
	return o.scraperClient.GetStream(ctx, malID, episodeID, serverID, category, prefer)
}

// GetScraperHealth bypasses the animeRepo entirely — the scraper's
// /health is service-wide. The URL-level animeId is structural symmetry
// only (so the gateway routes the same as the business endpoints).
func (o *scraperOps) GetScraperHealth(ctx context.Context) (int, []byte, error) {
	return o.scraperClient.GetHealth(ctx)
}

// --- CatalogService surface ----------------------------------------------

// GetScraperEpisodes is the public CatalogService method the handler
// layer calls. It delegates to the internal scraperOps unit.
func (s *CatalogService) GetScraperEpisodes(ctx context.Context, animeID, prefer string) (int, []byte, error) {
	return s.scraperOps().GetScraperEpisodes(ctx, animeID, prefer)
}

// GetScraperServers — see GetScraperEpisodes.
func (s *CatalogService) GetScraperServers(ctx context.Context, animeID, episodeID, prefer string) (int, []byte, error) {
	return s.scraperOps().GetScraperServers(ctx, animeID, episodeID, prefer)
}

// GetScraperStream — see GetScraperEpisodes.
func (s *CatalogService) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string) (int, []byte, error) {
	return s.scraperOps().GetScraperStream(ctx, animeID, episodeID, serverID, category, prefer)
}

// GetScraperHealth — see GetScraperEpisodes.
func (s *CatalogService) GetScraperHealth(ctx context.Context) (int, []byte, error) {
	return s.scraperOps().GetScraperHealth(ctx)
}

// scraperOps composes the live dependencies into a scraperOps. It is a
// cheap-to-build value type, so allocating one per call keeps the
// CatalogService struct untouched while preserving testability.
func (s *CatalogService) scraperOps() *scraperOps {
	return &scraperOps{
		animeRepo:     s.animeRepo,
		scraperClient: s.scraperClient,
	}
}
