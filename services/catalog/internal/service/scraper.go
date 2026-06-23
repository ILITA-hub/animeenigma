package service

import (
	"context"
	"errors"
	"strconv"
	"strings"

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
//
// Phase 26 (SCRAPER-HEAL-25, CONTEXT.md D5): SetHasEnglish added for the
// lazy backfill from the scraper-episodes resolver. Best-effort; failures
// are logged at the caller and never propagated.
type animeFetcher interface {
	GetByID(ctx context.Context, id string) (*domain.Anime, error)
	SetHasEnglish(ctx context.Context, animeID string, has bool) error
}

// scraperForwarder is the minimal interface scraperOps needs from the
// thin HTTP scraper client. Production wiring satisfies it via
// *scraper.Client.
//
// title is forwarded so providers with empty malsync coverage (e.g.
// gogoanime) can run their fuzzy title fallback; without it FindID
// fails with "cannot search without a title".
type scraperForwarder interface {
	GetEpisodes(ctx context.Context, malID int, title string, altTitles []string, prefer string, exclusive bool) (int, []byte, error)
	GetServers(ctx context.Context, malID int, title string, altTitles []string, episodeID, prefer string, exclusive bool) (int, []byte, error)
	GetStream(ctx context.Context, malID int, title string, altTitles []string, episodeID, serverID, category, prefer string, exclusive bool, userKey string) (int, []byte, error)
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

// resolveAnime looks up the catalog Anime by UUID and returns (malID, title)
// to forward to the scraper. Title preference: NameEN (English) → Name
// (Japanese romanization). English wins because the primary EN provider
// (Anitaku/Gogoanime) indexes anime by their English release titles, and
// AnimePahe's malsync path bypasses title anyway via ShikimoriID lookup.
//
// Resolution chain:
//  1. animeRepo.GetByID — if NotFound, surface as liberrors.NotFound("anime").
//  2. Try parsing ShikimoriID as int; Shikimori IDs equal MAL IDs for anime
//     (per `libs/idmapping` ARM mapping).
//  3. If ShikimoriID is empty or unparseable, try MALID.
//  4. Otherwise return ErrMalIDUnavailable so the handler can emit 422.
func (o *scraperOps) resolveAnime(ctx context.Context, animeID string) (int, string, []string, error) {
	if _, err := uuid.Parse(animeID); err != nil {
		return 0, "", nil, liberrors.NotFound("anime")
	}

	anime, err := o.animeRepo.GetByID(ctx, animeID)
	if err != nil {
		return 0, "", nil, err
	}
	if anime == nil {
		return 0, "", nil, liberrors.NotFound("anime")
	}

	title := anime.NameEN
	if title == "" {
		title = anime.Name
	}
	// ISS-017: forward the OTHER title forms as alternates so providers that
	// index an anime under a different language form than our primary Title
	// (e.g. AnimeFever lists "Shingeki no Kyojin" while NameEN is "Attack on
	// Titan") can still fuzzy-match. Order: romaji Name, English NameEN,
	// Japanese NameJP — minus the chosen primary, deduped, blanks dropped.
	altTitles := altTitleForms(title, anime.Name, anime.NameEN, anime.NameJP)

	if anime.ShikimoriID != "" {
		if v, perr := strconv.Atoi(anime.ShikimoriID); perr == nil && v > 0 {
			return v, title, altTitles, nil
		}
	}
	if anime.MALID != "" {
		if v, perr := strconv.Atoi(anime.MALID); perr == nil && v > 0 {
			return v, title, altTitles, nil
		}
	}
	return 0, title, altTitles, ErrMalIDUnavailable
}

// altTitleForms returns the distinct, non-empty title forms in `forms` with
// `primary` excluded (case-insensitive). Preserves input order. ISS-017.
func altTitleForms(primary string, forms ...string) []string {
	seen := map[string]bool{strings.ToLower(strings.TrimSpace(primary)): true}
	out := make([]string, 0, len(forms))
	for _, f := range forms {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		k := strings.ToLower(f)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, f)
	}
	return out
}

// GetScraperEpisodes resolves animeID -> MAL ID and forwards to the
// scraper service. Returns the scraper's status + body verbatim so the
// handler can passthrough 503s (Phase 15 not-yet-implemented contract).
//
// Phase 26 (SCRAPER-HEAL-25, CONTEXT.md D5): on a 200 response with
// non-empty episodes payload, opportunistically flips animes.has_english
// to true via SetHasEnglish. Best-effort — failures are logged but never
// surface to the caller. Mirrors the SetHasKodik / SetHasAnimeLib lazy-
// backfill pattern from Phase 15 (UX-31).
func (o *scraperOps) GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error) {
	malID, title, altTitles, err := o.resolveAnime(ctx, animeID)
	if err != nil {
		return 0, nil, err
	}
	status, body, gErr := o.scraperClient.GetEpisodes(ctx, malID, title, altTitles, prefer, exclusive)
	// Best-effort backfill on confirmed success. Detect non-empty
	// episodes by a simple substring scan — the response envelope is
	// `{"success":true,"data":{"episodes":[...]}}` and an empty
	// episodes array reads `"episodes":[]`. Use `"episodes":[{` as the
	// marker for "at least one element".
	if gErr == nil && status == 200 && len(body) > 0 &&
		strings.Contains(string(body), `"episodes":[{`) {
		if uerr := o.animeRepo.SetHasEnglish(ctx, animeID, true); uerr != nil {
			// Log only — never propagate. The user got their episodes;
			// the column will backfill on the next hit if this one
			// transiently failed.
			_ = uerr
		}
	}
	return status, body, gErr
}

// GetScraperServers resolves animeID -> MAL ID + title and forwards.
func (o *scraperOps) GetScraperServers(ctx context.Context, animeID, episodeID, prefer string, exclusive bool) (int, []byte, error) {
	malID, title, altTitles, err := o.resolveAnime(ctx, animeID)
	if err != nil {
		return 0, nil, err
	}
	return o.scraperClient.GetServers(ctx, malID, title, altTitles, episodeID, prefer, exclusive)
}

// GetScraperStream resolves animeID -> MAL ID + title and forwards.
func (o *scraperOps) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool) (int, []byte, error) {
	malID, title, altTitles, err := o.resolveAnime(ctx, animeID)
	if err != nil {
		return 0, nil, err
	}
	return o.scraperClient.GetStream(ctx, malID, title, altTitles, episodeID, serverID, category, prefer, exclusive, "")
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
func (s *CatalogService) GetScraperEpisodes(ctx context.Context, animeID, prefer string, exclusive bool) (int, []byte, error) {
	return s.scraperOps().GetScraperEpisodes(ctx, animeID, prefer, exclusive)
}

// GetScraperServers — see GetScraperEpisodes.
func (s *CatalogService) GetScraperServers(ctx context.Context, animeID, episodeID, prefer string, exclusive bool) (int, []byte, error) {
	return s.scraperOps().GetScraperServers(ctx, animeID, episodeID, prefer, exclusive)
}

// GetScraperStream — see GetScraperEpisodes.
func (s *CatalogService) GetScraperStream(ctx context.Context, animeID, episodeID, serverID, category, prefer string, exclusive bool) (int, []byte, error) {
	return s.scraperOps().GetScraperStream(ctx, animeID, episodeID, serverID, category, prefer, exclusive)
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
