// Workstream hero-spotlight v1.0 Phase 3 — Plan 03-03 Task 3 (Part B).
//
// NowWatchingResolver implements spotlight.Resolver for the `now_watching`
// card (HSB-BE-22 + HSB-NF-04). Executes a direct DISTINCT ON SQL query
// against the shared GORM connection — watch_progress JOIN users JOIN
// animes — and projects ONLY the public user fields (username + public_id)
// plus anime metadata + episode_number + updated_at. The user UUID NEVER
// leaves the SQL.
//
// Cache key: spotlight:now_watching (NOT date-keyed — live data). TTL 10s
// rate-limits DB pressure to <= 6 queries/minute regardless of request
// volume (threat T-03-13).

package cards

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/catalog/internal/service/spotlight"
)

// nowWatchingDB is the narrow DB surface this resolver needs. Defined as an
// interface so tests can substitute a handwritten fake without spinning up
// a real *gorm.DB.
type nowWatchingDB interface {
	RawScan(ctx context.Context, dest any, sql string, args ...any) error
}

// gormNowWatchingAdapter wraps *gorm.DB to satisfy nowWatchingDB. Production
// wires this in main.go; tests wire fakeNowWatchingDB.
type gormNowWatchingAdapter struct {
	db *gorm.DB
}

// NewGormNowWatchingAdapter constructs the production adapter. Exposed so
// the catalog main.go can wire it into NewNowWatchingResolver.
func NewGormNowWatchingAdapter(db *gorm.DB) nowWatchingDB {
	return &gormNowWatchingAdapter{db: db}
}

// RawScan executes the raw SQL with ctx and scans rows into dest.
func (a *gormNowWatchingAdapter) RawScan(ctx context.Context, dest any, sql string, args ...any) error {
	return a.db.WithContext(ctx).Raw(sql, args...).Scan(dest).Error
}

// nowWatchingKey is NOT date-keyed because this card surfaces live data.
const nowWatchingKey = "spotlight:now_watching"

// nowWatchingTTL throttles DB pressure to <=6 queries/minute (T-03-13).
const nowWatchingTTL = 10 * time.Second

// nowWatchingSQL projects ONLY public user fields per HSB-NF-04. The
// DISTINCT ON (wp.user_id) guarantees one row per user; the 5-minute
// window keeps the snapshot live without WebSocket complexity.
const nowWatchingSQL = `
SELECT DISTINCT ON (wp.user_id)
       u.username, u.public_id, a.id, a.name, a.name_ru, a.poster_url,
       wp.episode_number,
       to_char(wp.updated_at, 'YYYY-MM-DD"T"HH24:MI:SS"Z"') AS updated_at
FROM watch_progress wp
JOIN users u  ON u.id = wp.user_id
JOIN animes a ON a.id = wp.anime_id
WHERE wp.updated_at > NOW() - INTERVAL '5 minutes'
ORDER BY wp.user_id, wp.updated_at DESC
LIMIT 5
`

// nowWatchingRow is the SQL-row shape. Public fields ONLY (HSB-NF-04). The
// `gorm:"column:..."` tags map case-sensitive Postgres column names from
// the DISTINCT ON projection.
type nowWatchingRow struct {
	Username      string `gorm:"column:username"`
	PublicID      string `gorm:"column:public_id"`
	AnimeID       string `gorm:"column:id"`
	AnimeName     string `gorm:"column:name"`
	AnimeNameRU   string `gorm:"column:name_ru"`
	PosterURL     string `gorm:"column:poster_url"`
	EpisodeNumber int    `gorm:"column:episode_number"`
	UpdatedAt     string `gorm:"column:updated_at"`
}

// NowWatchingResolver implements spotlight.Resolver for `now_watching`.
type NowWatchingResolver struct {
	db    nowWatchingDB
	cache cache.Cache
	rng   *rand.Rand
	log   *logger.Logger
}

// NewNowWatchingResolver constructs the resolver. rng may be nil — a
// time-seeded source is provided.
func NewNowWatchingResolver(db nowWatchingDB, c cache.Cache, rng *rand.Rand, log *logger.Logger) *NowWatchingResolver {
	if rng == nil {
		rng = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	return &NowWatchingResolver{db: db, cache: c, rng: rng, log: log}
}

// Type returns the card discriminator string.
func (r *NowWatchingResolver) Type() string { return "now_watching" }

// Resolve produces the now_watching card. userID is IGNORED — every viewer
// sees the same global snapshot (HSB-NF-04 — the data is public by
// design).
func (r *NowWatchingResolver) Resolve(ctx context.Context, _ *string) (*spotlight.Card, error) {
	// --- Cache GET path -------------------------------------------------
	var cached spotlight.NowWatchingData
	if err := r.cache.Get(ctx, nowWatchingKey, &cached); err == nil {
		// Apply AdaptiveSlice even on cache hit — the cache could have been
		// seeded by another instance with >3 sessions.
		picked := spotlight.AdaptiveSlice(cached.Sessions, r.rng)
		if len(picked) == 0 {
			return nil, nil
		}
		return &spotlight.Card{Type: r.Type(), Data: spotlight.NowWatchingData{
			Sessions: append([]spotlight.NowWatchingSession(nil), picked...),
		}}, nil
	} else if !errors.Is(err, cache.ErrNotFound) {
		r.log.Warnw("spotlight.cache_get_failed", "type", r.Type(), "key", nowWatchingKey, "error", err)
	}

	// --- Cache MISS path: query DB --------------------------------------
	var rows []nowWatchingRow
	if err := r.db.RawScan(ctx, &rows, nowWatchingSQL); err != nil {
		return nil, fmt.Errorf("now_watching: db: %w", err)
	}
	if len(rows) == 0 {
		// Eligibility = false. Do NOT cache empty (Pitfall 5).
		return nil, nil
	}

	sessions := make([]spotlight.NowWatchingSession, 0, len(rows))
	for _, row := range rows {
		sessions = append(sessions, spotlight.NowWatchingSession{
			Username:      row.Username,
			PublicID:      row.PublicID,
			AnimeID:       row.AnimeID,
			AnimeName:     row.AnimeName,
			AnimeNameRU:   row.AnimeNameRU,
			PosterURL:     row.PosterURL,
			EpisodeNumber: row.EpisodeNumber,
			UpdatedAt:     row.UpdatedAt,
		})
	}

	picked := spotlight.AdaptiveSlice(sessions, r.rng)
	if len(picked) == 0 {
		return nil, nil
	}

	data := spotlight.NowWatchingData{
		Sessions: append([]spotlight.NowWatchingSession(nil), picked...),
	}

	// --- Cache SET (best-effort) ----------------------------------------
	if err := r.cache.Set(ctx, nowWatchingKey, data, nowWatchingTTL); err != nil {
		r.log.Warnw("spotlight.cache_set_failed", "type", r.Type(), "key", nowWatchingKey, "error", err)
	}
	return &spotlight.Card{Type: r.Type(), Data: data}, nil
}
