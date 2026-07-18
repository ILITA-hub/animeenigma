package repo

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode"

	liberrors "github.com/ILITA-hub/animeenigma/libs/errors"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
	"gorm.io/gorm"
)

type AnimeRepository struct {
	db *gorm.DB
}

func NewAnimeRepository(db *gorm.DB) *AnimeRepository {
	return &AnimeRepository{db: db}
}

func (r *AnimeRepository) Create(ctx context.Context, anime *domain.Anime) error {
	if err := r.db.WithContext(ctx).Create(anime).Error; err != nil {
		return fmt.Errorf("create anime: %w", err)
	}
	return nil
}

func (r *AnimeRepository) GetByID(ctx context.Context, id string) (*domain.Anime, error) {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).Preload("Genres").Preload("Studios").First(&anime, "id = ?", id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, liberrors.NotFound("anime")
		}
		return nil, fmt.Errorf("get anime by id: %w", err)
	}
	return &anime, nil
}

func (r *AnimeRepository) GetByShikimoriID(ctx context.Context, shikimoriID string) (*domain.Anime, error) {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).First(&anime, "shikimori_id = ?", shikimoriID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get anime by shikimori id: %w", err)
	}
	return &anime, nil
}

func (r *AnimeRepository) GetByMALID(ctx context.Context, malID string) (*domain.Anime, error) {
	var anime domain.Anime
	if err := r.db.WithContext(ctx).First(&anime, "mal_id = ?", malID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil
		}
		return nil, fmt.Errorf("get anime by mal id: %w", err)
	}
	return &anime, nil
}

// animeMetadataColumns are the Shikimori-sourced metadata columns a refresh
// owns. Update force-writes exactly these (Select includes zero values, so a
// finished anime's next_episode_at is correctly cleared) and never touches the
// lazily-maintained or admin-controlled columns: the provider-availability
// flags (has_dub/has_kodik/has_animelib/has_raw/has_english), the local
// has_video flag, the admin pin (sort_priority), hidden, franchise /
// franchise_checked, or the externally-resolved IDs (mal_id, ani_list_id,
// im_db_id, tmdb_id). Those are maintained by their dedicated Set*/Update*
// methods. Previously Update used Save, a full-row overwrite that silently
// zeroed every one of those columns on each refresh cycle, because the refresh
// paths hand Update a freshly mapped anime with all of them at zero values.
//
// KEEP IN SYNC with AnimeMetadataEqual, which compares exactly these columns to
// let BatchRefreshAnime skip a no-op Update when the fetch was unchanged.
var animeMetadataColumns = []string{
	"name", "name_en", "name_ru", "name_jp", "description",
	"year", "season", "status", "kind", "rating", "material_source",
	"episodes_count", "episodes_aired", "episode_duration",
	"score", "poster_url", "next_episode_at", "next_episode_source", "aired_on",
	"released_on",
}

func (r *AnimeRepository) Update(ctx context.Context, anime *domain.Anime) error {
	if anime.ID == "" {
		return liberrors.NotFound("anime")
	}
	result := r.db.WithContext(ctx).
		Model(&domain.Anime{}).
		Where("id = ?", anime.ID).
		Select(animeMetadataColumns).
		Updates(anime)
	if result.Error != nil {
		return fmt.Errorf("update anime: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

// AnimeMetadataEqual reports whether every Shikimori-sourced metadata column in
// animeMetadataColumns is identical between a and b. BatchRefreshAnime uses it to
// skip a no-op full-row Update (and the secondary-index churn it triggers) when
// Shikimori returned unchanged data for a stale anime.
//
// KEEP IN SYNC with animeMetadataColumns: exactly one comparison here per column
// listed there. Score is compared at the decimal(4,2) storage precision so a
// higher-precision Shikimori value that rounds to the already-stored value is not
// mistaken for a perpetual change (which would defeat the skip on every run).
func AnimeMetadataEqual(a, b *domain.Anime) bool {
	return a.Name == b.Name &&
		a.NameEN == b.NameEN &&
		a.NameRU == b.NameRU &&
		a.NameJP == b.NameJP &&
		a.Description == b.Description &&
		a.Year == b.Year &&
		a.Season == b.Season &&
		a.Status == b.Status &&
		a.Kind == b.Kind &&
		a.Rating == b.Rating &&
		a.MaterialSource == b.MaterialSource &&
		a.EpisodesCount == b.EpisodesCount &&
		a.EpisodesAired == b.EpisodesAired &&
		a.EpisodeDuration == b.EpisodeDuration &&
		scoreEqual(a.Score, b.Score) &&
		a.PosterURL == b.PosterURL &&
		timePtrEqual(a.NextEpisodeAt, b.NextEpisodeAt) &&
		timePtrEqual(a.AiredOn, b.AiredOn) &&
		timePtrEqual(a.ReleasedOn, b.ReleasedOn)
}

// scoreEqual compares two scores at the decimal(4,2) precision the score column
// stores, so e.g. 8.523 (fresh from Shikimori) and 8.52 (already stored) match.
func scoreEqual(a, b float64) bool {
	return math.Round(a*100) == math.Round(b*100)
}

// timePtrEqual treats two *time.Time as equal when both are nil or both point to
// the same instant. Compares by instant (time.Equal), so a DB-round-tripped UTC
// value and a freshly parsed value for the same moment match regardless of
// location or monotonic-clock reading.
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
}

// TouchUpdatedAt advances only the updated_at column for the given anime IDs in a
// single statement, without rewriting any metadata column. BatchRefreshAnime calls
// it for anime whose Shikimori metadata came back unchanged, so the row still
// leaves the stale-refresh window (GetStaleAnime filters on updated_at) on the
// normal cadence without a full-row update and its secondary-index churn. No-op on
// empty input.
func (r *AnimeRepository) TouchUpdatedAt(ctx context.Context, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	result := r.db.WithContext(ctx).
		Model(&domain.Anime{}).
		Where("id IN ?", ids).
		UpdateColumn("updated_at", time.Now())
	if result.Error != nil {
		return fmt.Errorf("touch anime updated_at: %w", result.Error)
	}
	return nil
}

// normalizeSearchQuery lowercases the query and drops every rune that is
// not a letter or digit, mirroring the regexp_replace expression applied to
// the name columns in Search. Returns "" when the query carries no
// alphanumeric content (the caller then falls back to a literal match).
func normalizeSearchQuery(q string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(q) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (r *AnimeRepository) Search(ctx context.Context, filters domain.SearchFilters) ([]*domain.Anime, int64, error) {
	query := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("hidden = ? OR hidden IS NULL", false)

	if filters.Query != "" {
		if norm := normalizeSearchQuery(filters.Query); norm != "" {
			// Punctuation-insensitive match: both sides are lowercased and
			// stripped of everything non-alphanumeric, so "re zero" finds
			// "Re:Zero" and "fate zero" finds "Fate/Zero". [[:alnum:]] is
			// UTF-8-aware in Postgres, covering Cyrillic and Japanese.
			// Stripping also removes LIKE wildcards from the user input.
			const colNorm = "regexp_replace(lower(%s), '[^[:alnum:]]+', '', 'g') LIKE ?"
			pat := "%" + norm + "%"
			query = query.Where(
				fmt.Sprintf(colNorm, "name")+" OR "+fmt.Sprintf(colNorm, "name_ru")+" OR "+fmt.Sprintf(colNorm, "name_jp"),
				pat, pat, pat)
		} else {
			// Query had no letters/digits at all — normalized form would
			// match everything, so keep the literal substring behavior.
			query = query.Where("name ILIKE ? OR name_ru ILIKE ? OR name_jp ILIKE ?",
				"%"+filters.Query+"%", "%"+filters.Query+"%", "%"+filters.Query+"%")
		}
	}
	if filters.Year != nil {
		query = query.Where("year = ?", *filters.Year)
	}
	if filters.YearFrom != nil {
		query = query.Where("year >= ?", *filters.YearFrom)
	}
	if filters.YearTo != nil {
		query = query.Where("year <= ?", *filters.YearTo)
	}
	if filters.Season != "" {
		query = query.Where("season = ?", filters.Season)
	}
	if filters.Status != "" {
		query = query.Where("status = ?", filters.Status)
	}
	// Phase 15 (UX-31) — Kinds is an OR-set over the Shikimori kind enum.
	// A row passes when its kind matches ANY selected value. Whitelisted at
	// the handler so unknown values never reach the SQL.
	if len(filters.Kinds) > 0 {
		query = query.Where("kind IN ?", filters.Kinds)
	}
	// Phase 15 (UX-31) — Providers is an OR-set across the 4 has_{provider}
	// boolean columns. A row passes when ANY of the selected providers is
	// true. Unknown keys are dropped silently; the handler-level whitelist
	// mirrors this, so the inner branch is defence-in-depth.
	if len(filters.Providers) > 0 {
		colsByKey := map[string]string{
			"kodik": "has_kodik",
			"dub":   "has_dub",
			"ae":    "has_video",
		}
		var orParts []string
		for _, p := range filters.Providers {
			if col, ok := colsByKey[p]; ok {
				orParts = append(orParts, col+" = true")
			}
		}
		if len(orParts) > 0 {
			query = query.Where("(" + strings.Join(orParts, " OR ") + ")")
		}
	}
	// Studio filter — OR-set (a row passes if it has ANY selected studio).
	// Unlike genres (AND-set via HAVING COUNT), an anime rarely has >1
	// studio, so AND would near-always return empty. Mirrors the anime_studios
	// m2m join (column studio_id).
	if len(filters.StudioIDs) > 0 {
		query = query.Where("id IN (SELECT anime_id FROM anime_studios WHERE studio_id IN ?)", filters.StudioIDs)
	}
	if len(filters.GenreIDs) > 0 {
		query = query.Where("id IN (SELECT anime_id FROM anime_genres WHERE genre_id IN ? GROUP BY anime_id HAVING COUNT(DISTINCT genre_id) = ?)", filters.GenreIDs, len(filters.GenreIDs))
	}
	if filters.ScoreMin != nil {
		query = query.Where("score >= ?", *filters.ScoreMin)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count anime: %w", err)
	}

	// Phase 11 / UX-21 — sort_priority DESC is the primary pin (CLAUDE.md
	// "Pinning anime to the top" convention). When an explicit sort axis is
	// requested it overrides the SECOND criterion only — never the pin.
	// `sort=title` defaults to ASC (intuitive A→Z); all other axes default
	// to DESC. An explicit filters.Order still wins when provided.
	orderBy := "sort_priority DESC, score DESC"
	if filters.Sort != "" {
		column := mapSortColumn(filters.Sort)
		order := "DESC"
		if filters.Order == "asc" || (filters.Order == "" && filters.Sort == "title") {
			order = "ASC"
		}
		orderBy = fmt.Sprintf("sort_priority DESC, %s %s", column, order)
	}

	offset := (filters.Page - 1) * filters.PageSize
	if offset < 0 {
		offset = 0
	}

	var animes []*domain.Anime
	if err := query.Order(orderBy).Limit(filters.PageSize).Offset(offset).Find(&animes).Error; err != nil {
		return nil, 0, fmt.Errorf("search anime: %w", err)
	}

	return animes, total, nil
}

// ListStudios returns every studio that has at least one anime, ordered by
// anime count DESC then name ASC. The JOIN excludes zero-anime studios.
func (r *AnimeRepository) ListStudios(ctx context.Context) ([]domain.Studio, error) {
	var studios []domain.Studio
	err := r.db.WithContext(ctx).
		Model(&domain.Studio{}).
		Joins("JOIN anime_studios ON anime_studios.studio_id = studios.id").
		Group("studios.id").
		Order("COUNT(anime_studios.anime_id) DESC, studios.name ASC").
		Find(&studios).Error
	if err != nil {
		return nil, fmt.Errorf("list studios: %w", err)
	}
	return studios, nil
}

func (r *AnimeRepository) GetBySeason(ctx context.Context, year int, season string, page, pageSize int) ([]*domain.Anime, int64, error) {
	filters := domain.SearchFilters{
		Year:     &year,
		Season:   season,
		Page:     page,
		PageSize: pageSize,
		Sort:     "score",
		Order:    "desc",
	}
	return r.Search(ctx, filters)
}

func (r *AnimeRepository) SetHasVideo(ctx context.Context, animeID string, hasVideo bool) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("has_video", hasVideo).Error
}

// SetHasDub flips the animes.has_dub column for one anime. Called by
// GetKodikTranslations whenever the catalog touches Kodik for the anime —
// best-effort, the dub badge is decorative. Phase 9 (UX-18).
func (r *AnimeRepository) SetHasDub(ctx context.Context, animeID string, hasDub bool) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("has_dub", hasDub).Error
}

// SetHasKodik flips the animes.has_kodik column for one anime. Called
// lazily by GetKodikTranslations whenever the catalog touches Kodik for
// the anime — best-effort. Phase 15 (UX-31).
func (r *AnimeRepository) SetHasKodik(ctx context.Context, animeID string, has bool) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("has_kodik", has).Error
}

// SetHasAnimeLib flips the animes.has_animelib column for one anime.
// Called lazily by GetAnimeLibTranslations whenever AnimeLib's hapi
// returns at least one non-Kodik translation — best-effort. The
// Kodik-iframe-fallback path inside AnimeLib does NOT count (per
// feedback_animelib_no_kodik_fallback.md). Phase 15 (UX-31).
func (r *AnimeRepository) SetHasAnimeLib(ctx context.Context, animeID string, has bool) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("has_animelib", has).Error
}

// SetHasRaw flips the animes.has_raw column for one anime. Called
// lazily by the raw resolver when an AllAnime show ID resolves to a
// playable raw stream — best-effort. Workstream raw-jp, Phase 01.
func (r *AnimeRepository) SetHasRaw(ctx context.Context, animeID string, has bool) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("has_raw", has).Error
}

// SetHasEnglish flips the animes.has_english column for one anime. Called
// lazily by the catalog's scraper-episodes resolver whenever any scraper
// provider (gogoanime, animepahe, allanime-okru, miruro, nineanime) returns >= 1 episode
// for the anime — best-effort. Failures are logged at the caller, never
// propagated. Phase 26 (SCRAPER-HEAL-25, CONTEXT.md D5).
func (r *AnimeRepository) SetHasEnglish(ctx context.Context, animeID string, has bool) error {
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("has_english", has).Error
}

// UpdateExternalIDs sets animes.imdb_id and/or animes.tmdb_id when present.
// Nil values are not written (existing values preserved). Workstream raw-jp,
// Phase 02 — populated lazily on the first OpenSubtitles query via the
// Kitsu mappings endpoint.
func (r *AnimeRepository) UpdateExternalIDs(ctx context.Context, animeID string, imdb, tmdb *string) error {
	updates := map[string]any{}
	if imdb != nil {
		updates["imdb_id"] = *imdb
	}
	if tmdb != nil {
		updates["tmdb_id"] = *tmdb
	}
	if len(updates) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Updates(updates).Error
}

func (r *AnimeRepository) SetHidden(ctx context.Context, animeID string, hidden bool) error {
	result := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("hidden", hidden)
	if result.Error != nil {
		return fmt.Errorf("set hidden: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) GetHiddenAnime(ctx context.Context) ([]*domain.Anime, error) {
	var animes []*domain.Anime
	if err := r.db.WithContext(ctx).Where("hidden = ?", true).Order("updated_at DESC").Find(&animes).Error; err != nil {
		return nil, fmt.Errorf("get hidden anime: %w", err)
	}
	return animes, nil
}

func (r *AnimeRepository) UpdateMALID(ctx context.Context, animeID string, malID string) error {
	result := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("mal_id", malID)
	if result.Error != nil {
		return fmt.Errorf("update mal_id: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) UpdateAniListID(ctx context.Context, animeID string, anilistID string) error {
	result := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("ani_list_id", anilistID)
	if result.Error != nil {
		return fmt.Errorf("update ani_list_id: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) UpdateShikimoriID(ctx context.Context, animeID string, shikimoriID string) error {
	result := r.db.WithContext(ctx).Model(&domain.Anime{}).Where("id = ?", animeID).
		Update("shikimori_id", shikimoriID)
	if result.Error != nil {
		return fmt.Errorf("update shikimori_id: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("anime")
	}
	return nil
}

func (r *AnimeRepository) GetSchedule(ctx context.Context) ([]*domain.Anime, error) {
	var animes []*domain.Anime
	err := r.db.WithContext(ctx).
		Where("status = ? AND next_episode_at IS NOT NULL AND next_episode_at > NOW() AND (hidden = ? OR hidden IS NULL)",
			"ongoing", false).
		Order("next_episode_at ASC").
		Find(&animes).Error
	if err != nil {
		return nil, fmt.Errorf("get schedule: %w", err)
	}
	return animes, nil
}

func (r *AnimeRepository) GetOngoingAnime(ctx context.Context, page, pageSize int, sort, order string, recentOnly bool) ([]*domain.Anime, int64, error) {
	var total int64
	query := r.db.WithContext(ctx).Model(&domain.Anime{}).
		Where("status = ? AND (hidden = ? OR hidden IS NULL)", "ongoing", false)

	if recentOnly {
		cutoff := time.Now().AddDate(-1, 0, 0)
		query = query.Where("aired_on IS NULL OR aired_on >= ?", cutoff)
	}

	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("count ongoing anime: %w", err)
	}

	offset := (page - 1) * pageSize
	if offset < 0 {
		offset = 0
	}

	orderClause := "COALESCE(next_episode_at, '9999-12-31') ASC, score DESC"
	if sort != "" && sort != "next_episode_at" {
		dir := "DESC"
		if strings.ToLower(order) == "asc" {
			dir = "ASC"
		}
		orderClause = fmt.Sprintf("%s %s NULLS LAST", mapSortColumn(sort), dir)
	}

	var animes []*domain.Anime
	err := query.Order(orderClause).
		Limit(pageSize).Offset(offset).Find(&animes).Error
	if err != nil {
		return nil, 0, fmt.Errorf("get ongoing anime: %w", err)
	}

	return animes, total, nil
}

// GetStaleAnime returns anime of a given status that haven't been updated since staleBefore.
func (r *AnimeRepository) GetStaleAnime(ctx context.Context, status domain.AnimeStatus, staleBefore time.Time) ([]*domain.Anime, error) {
	var animes []*domain.Anime
	err := r.db.WithContext(ctx).
		Where("status = ? AND updated_at < ? AND shikimori_id != '' AND shikimori_id IS NOT NULL AND (hidden = ? OR hidden IS NULL)",
			string(status), staleBefore, false).
		Order("updated_at ASC").
		Find(&animes).Error
	if err != nil {
		return nil, fmt.Errorf("get stale %s anime: %w", status, err)
	}
	return animes, nil
}

// mapSortColumn whitelists frontend sort axes to backend SQL columns.
// Phase 11 / UX-21 added `updated -> updated_at` so the Browse view's
// 5-axis sort dropdown (popularity / rating / year / updated / title)
// maps cleanly here. Unknown values fall through to `score` (the
// existing default ordering for the catalog).
func mapSortColumn(sort string) string {
	switch sort {
	case "popularity", "rating":
		return "score"
	case "year", "score", "name", "created_at", "updated_at":
		return sort
	case "updated":
		return "updated_at"
	case "title":
		return "name"
	default:
		return "score"
	}
}

func (r *AnimeRepository) GetPinnedTranslations(ctx context.Context, animeID string) ([]domain.PinnedTranslation, error) {
	var pinned []domain.PinnedTranslation
	err := r.db.WithContext(ctx).Where("anime_id = ?", animeID).Order("pinned_at ASC").Find(&pinned).Error
	if err != nil {
		return nil, fmt.Errorf("get pinned translations: %w", err)
	}
	return pinned, nil
}

func (r *AnimeRepository) PinTranslation(ctx context.Context, pin *domain.PinnedTranslation) error {
	pin.PinnedAt = time.Now()
	result := r.db.WithContext(ctx).Save(pin)
	if result.Error != nil {
		return fmt.Errorf("pin translation: %w", result.Error)
	}
	return nil
}

func (r *AnimeRepository) UnpinTranslation(ctx context.Context, animeID string, translationID int) error {
	result := r.db.WithContext(ctx).Where("anime_id = ? AND translation_id = ?", animeID, translationID).
		Delete(&domain.PinnedTranslation{})
	if result.Error != nil {
		return fmt.Errorf("unpin translation: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return liberrors.NotFound("pinned translation not found")
	}
	return nil
}

// ListGuessPoolCandidates returns non-hidden anime with score strictly greater
// than minScore, ordered earliest-aired first (NULLs last) so a franchise's
// first member is encountered first during collapse. Genres/Studios/Tags are
// preloaded for attribute comparison.
func (r *AnimeRepository) ListGuessPoolCandidates(ctx context.Context, minScore float64) ([]*domain.Anime, error) {
	var animes []*domain.Anime
	err := r.db.WithContext(ctx).
		Where("score > ? AND (hidden = ? OR hidden IS NULL)", minScore, false).
		Preload("Genres").
		Preload("Studios").
		Preload("Tags").
		Order("aired_on ASC NULLS LAST").
		Find(&animes).Error
	if err != nil {
		return nil, fmt.Errorf("list guess pool candidates: %w", err)
	}
	return animes, nil
}

// SetFranchise persists a backfilled franchise slug onto an anime row and marks
// it checked, so standalone anime (empty franchise) are not re-fetched on every
// guess-pool build. Uses a map so franchise_checked is written even when the
// franchise itself is the empty string (GORM struct updates skip zero values).
func (r *AnimeRepository) SetFranchise(ctx context.Context, id, franchise string) error {
	return r.db.WithContext(ctx).
		Model(&domain.Anime{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"franchise":         franchise,
			"franchise_checked": true,
		}).Error
}

// SetMALPopularity persists the Jikan-sourced MAL anticipation counts for one
// anime (targeted update — leaves every other column untouched). Feeds the recs
// relative-MAL-popularity signal for announced titles.
func (r *AnimeRepository) SetMALPopularity(ctx context.Context, id string, members, favorites int) error {
	return r.db.WithContext(ctx).
		Model(&domain.Anime{}).
		Where("id = ?", id).
		Updates(map[string]interface{}{
			"mal_members":   members,
			"mal_favorites": favorites,
		}).Error
}

// ListFranchiseUncheckedListed returns anime that appear in at least one
// user's anime_list but were never franchise-checked — the S8 seed-side
// franchise backfill pool (spec 2026-07-17). Bounded by limit; oldest rows
// first so the backfill converges deterministically across daily runs.
func (r *AnimeRepository) ListFranchiseUncheckedListed(ctx context.Context, limit int) ([]*domain.Anime, error) {
	var out []*domain.Anime
	err := r.db.WithContext(ctx).
		Where("franchise_checked = ? AND shikimori_id <> ''", false).
		Where("EXISTS (SELECT 1 FROM anime_list al WHERE al.anime_id = animes.id)").
		Order("created_at ASC").
		Limit(limit).
		Find(&out).Error
	return out, err
}

// VerifyMembershipRow is the minimal projection the content-verify queue
// needs: identity + the latest-aired counter for sample-episode selection.
type VerifyMembershipRow struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EpisodesAired int    `json:"episodes_aired"`
}

// ListVerifyMembership returns the content-verify queue membership: all
// visible ongoings plus the browse-order top (sort_priority DESC, score DESC).
func (r *AnimeRepository) ListVerifyMembership(ctx context.Context, ongoingLimit, topLimit int) (ongoing, top []VerifyMembershipRow, err error) {
	err = r.db.WithContext(ctx).Model(&domain.Anime{}).
		Select("id, name, episodes_aired").
		Where("status = ? AND (hidden = ? OR hidden IS NULL)", "ongoing", false).
		Order("score DESC").Limit(ongoingLimit).
		Scan(&ongoing).Error
	if err != nil {
		return nil, nil, err
	}
	err = r.db.WithContext(ctx).Model(&domain.Anime{}).
		Select("id, name, episodes_aired").
		Where("hidden = ? OR hidden IS NULL", false).
		Order("sort_priority DESC, score DESC").Limit(topLimit).
		Scan(&top).Error
	return ongoing, top, err
}
