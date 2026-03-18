# Watch Preference Resolution System — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Persist per-anime watch preferences server-side with a 5-tier smart fallback resolution engine, Prometheus metrics for Grafana, and cross-device sync.

**Architecture:** Extend the player service with two new tables (`user_anime_preferences`, restructured `watch_history`), a pure-logic resolver, and 3 new API endpoints. Frontend extends all 4 player components to report combo context on the existing progress heartbeat, and adds a `useWatchPreferences` composable for page-load resolution with localStorage caching.

**Tech Stack:** Go (GORM, chi router), Vue 3 (composables, TypeScript), Prometheus (promauto), PostgreSQL.

**Spec:** `docs/superpowers/specs/2026-03-18-watch-preference-resolution-design.md`

---

## File Structure

### Backend — New Files

| File | Responsibility |
|------|---------------|
| `services/player/internal/domain/preference.go` | `UserAnimePreference`, `WatchCombo`, `ResolveRequest`, `ResolveResponse`, `ComboCount`, `CommunityCombo`, `PinnedTranslation` domain types |
| `services/player/internal/service/resolver.go` | Pure-logic 5-tier fallback resolution engine |
| `services/player/internal/service/resolver_test.go` | Table-driven unit tests with real translation data |
| `services/player/internal/service/preference.go` | Preference service (upsert, get, global aggregation — no Redis, DB-only) |
| `services/player/internal/repo/preference.go` | Preference + watch_history DB queries |
| `services/player/internal/handler/preference.go` | HTTP handlers for resolve, get preference, get global |
| `libs/metrics/watch.go` | New Prometheus metrics (episodes, sessions, resolution, translations) |

### Backend — Modified Files

| File | Changes |
|------|---------|
| `services/player/internal/domain/watch.go` | Extend `WatchHistory` struct with combo fields, extend `UpdateProgressRequest` and add `MarkEpisodeWatchedRequest` combo fields |
| `services/player/internal/handler/progress.go` | Pass combo fields to service, increment new metrics |
| `services/player/internal/handler/list.go` | Pass combo fields from `MarkEpisodeWatched` to service |
| `services/player/internal/service/progress.go` | Upsert `UserAnimePreference` when combo fields present |
| `services/player/internal/service/list.go` | Create `WatchHistory` row with combo when marking episode |
| `services/player/internal/repo/progress.go` | No changes (existing upsert works) |
| `services/player/internal/transport/router.go` | Register 3 new preference routes |
| `services/player/cmd/player-api/main.go` | AutoMigrate new `UserAnimePreference` model |
| `libs/cache/ttl.go` | No changes needed (Redis not used — player service has no Redis) |

### Frontend — New Files

| File | Responsibility |
|------|---------------|
| `frontend/web/src/composables/useWatchPreferences.ts` | Resolve composable: API call + localStorage cache |
| `frontend/web/src/types/preference.ts` | `WatchCombo` TypeScript type |

### Frontend — Modified Files

| File | Changes |
|------|---------|
| `frontend/web/src/api/client.ts` | Add `resolvePreference()`, `getAnimePreference()`, `getGlobalPreferences()` |
| `frontend/web/src/views/Anime.vue` | Call `useWatchPreferences`, pass `preferredCombo` prop to players, auto-switch player tab |
| `frontend/web/src/components/player/KodikPlayer.vue` | Accept `preferredCombo` prop, merge combo into heartbeat + markEpisodeWatched |
| `frontend/web/src/components/player/HiAnimePlayer.vue` | Accept `preferredCombo` prop, merge combo into heartbeat + markEpisodeWatched |
| `frontend/web/src/components/player/AnimeLibPlayer.vue` | Add 30s progress heartbeat, accept `preferredCombo` prop, merge combo |
| `frontend/web/src/components/player/ConsumetPlayer.vue` | Add server-side progress save (currently only emits), accept `preferredCombo` + `subOrDub` props, merge combo |

---

## Task 1: Domain Types & WatchHistory Restructure

**Files:**
- Create: `services/player/internal/domain/preference.go`
- Modify: `services/player/internal/domain/watch.go:70-104`
- Modify: `services/player/cmd/player-api/main.go:47-56`

- [ ] **Step 1: Create preference domain types**

Create `services/player/internal/domain/preference.go`:

```go
package domain

import "time"

// WatchCombo describes a normalized player+translation selection
type WatchCombo struct {
	Player           string `json:"player"`            // kodik, animelib, hianime, consumet
	Language         string `json:"language"`           // ru, en
	WatchType        string `json:"watch_type"`         // dub, sub
	TranslationID    string `json:"translation_id"`     // provider-specific, always string
	TranslationTitle string `json:"translation_title"`  // human-readable team name
}

// ValidPlayers is the set of allowed player values
var ValidPlayers = map[string]bool{
	"kodik": true, "animelib": true, "hianime": true, "consumet": true,
}

// ValidLanguages is the set of allowed language values
var ValidLanguages = map[string]bool{"ru": true, "en": true}

// ValidWatchTypes is the set of allowed watch type values
var ValidWatchTypes = map[string]bool{"dub": true, "sub": true}

// ValidateCombo checks if combo fields are valid (when present)
func ValidateCombo(player, language, watchType string) bool {
	if player == "" && language == "" && watchType == "" {
		return true // all empty = no combo, valid
	}
	return ValidPlayers[player] && ValidLanguages[language] && ValidWatchTypes[watchType]
}

// UserAnimePreference stores the user's last-used combo per anime
type UserAnimePreference struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_anime_pref" json:"user_id"`
	AnimeID          string    `gorm:"not null;uniqueIndex:idx_user_anime_pref" json:"anime_id"`
	Player           string    `gorm:"size:20;not null" json:"player"`
	Language         string    `gorm:"size:5;not null" json:"language"`
	WatchType        string    `gorm:"size:5;not null" json:"watch_type"`
	TranslationID    string    `gorm:"size:50" json:"translation_id"`
	TranslationTitle string    `gorm:"size:200" json:"translation_title"`
	UpdatedAt        time.Time `gorm:"not null;autoUpdateTime" json:"updated_at"`
}

// ResolveRequest is the payload for POST /api/users/preferences/resolve
type ResolveRequest struct {
	AnimeID   string       `json:"anime_id"`
	Available []WatchCombo `json:"available"`
}

// ResolveResponse is the response for the resolve endpoint
type ResolveResponse struct {
	Resolved *ResolvedCombo `json:"resolved"`
}

// ResolvedCombo extends WatchCombo with resolution metadata
type ResolvedCombo struct {
	WatchCombo
	Tier       string `json:"tier"`        // per_anime, user_global, community, pinned, default
	TierNumber int    `json:"tier_number"` // 1-5
}

// ComboCount is a user's watch count for a specific combo (used by Tier 2)
type ComboCount struct {
	Player           string `json:"player"`
	Language         string `json:"language"`
	WatchType        string `json:"watch_type"`
	TranslationTitle string `json:"translation_title"`
	Count            int    `json:"count"`
}

// CommunityCombo is a community popularity entry for an anime (used by Tier 3)
type CommunityCombo struct {
	Player           string `json:"player"`
	Language         string `json:"language"`
	WatchType        string `json:"watch_type"`
	TranslationID    string `json:"translation_id"`
	TranslationTitle string `json:"translation_title"`
	Viewers          int    `json:"viewers"`
}

// PinnedTranslation maps to catalog's pinned_translations table (shared DB, used by Tier 4)
type PinnedTranslation struct {
	AnimeID          string `gorm:"column:anime_id"`
	TranslationID    int    `gorm:"column:translation_id"`
	TranslationTitle string `gorm:"column:translation_title"`
	TranslationType  string `gorm:"column:translation_type"` // "voice" or "subtitles"
}

func (PinnedTranslation) TableName() string { return "pinned_translations" }
```

- [ ] **Step 2: Extend WatchHistory struct with combo fields**

In `services/player/internal/domain/watch.go`, replace the `WatchHistory` struct (lines 70-80) with:

```go
// WatchHistory records a watched episode with full combo context
type WatchHistory struct {
	ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID           string    `gorm:"type:uuid;not null;index:idx_wh_user_combo" json:"user_id"`
	AnimeID          string    `gorm:"not null;index;index:idx_wh_anime_combo" json:"anime_id"`
	EpisodeNumber    int       `gorm:"not null" json:"episode_number"`
	Player           string    `gorm:"size:20;not null;index:idx_wh_user_combo;index:idx_wh_anime_combo" json:"player"`
	Language         string    `gorm:"size:5;not null;index:idx_wh_user_combo;index:idx_wh_anime_combo" json:"language"`
	WatchType        string    `gorm:"size:5;not null;index:idx_wh_user_combo;index:idx_wh_anime_combo" json:"watch_type"`
	TranslationID    string    `gorm:"size:50" json:"translation_id"`
	TranslationTitle string    `gorm:"size:200" json:"translation_title"`
	DurationWatched  int       `gorm:"default:0" json:"duration_watched"`
	WatchedAt        time.Time `gorm:"not null;default:now()" json:"watched_at"`
}
```

This creates two composite indexes via GORM tags:
- `idx_wh_user_combo` on `(user_id, player, language, watch_type)` — Tier 2 aggregation
- `idx_wh_anime_combo` on `(anime_id, player, language, watch_type)` — Tier 3 aggregation

- [ ] **Step 3: Extend UpdateProgressRequest and add MarkEpisodeWatchedRequest**

In `services/player/internal/domain/watch.go`, replace `UpdateProgressRequest` (lines 99-104) with:

```go
type UpdateProgressRequest struct {
	AnimeID          string `json:"anime_id"`
	EpisodeNumber    int    `json:"episode_number"`
	Progress         int    `json:"progress"`
	Duration         int    `json:"duration"`
	Player           string `json:"player,omitempty"`
	Language         string `json:"language,omitempty"`
	WatchType        string `json:"watch_type,omitempty"`
	TranslationID    string `json:"translation_id,omitempty"`
	TranslationTitle string `json:"translation_title,omitempty"`
}

// MarkEpisodeWatchedRequest extends the episode-watched payload with combo context
type MarkEpisodeWatchedRequest struct {
	Episode          int    `json:"episode"`
	Player           string `json:"player,omitempty"`
	Language         string `json:"language,omitempty"`
	WatchType        string `json:"watch_type,omitempty"`
	TranslationID    string `json:"translation_id,omitempty"`
	TranslationTitle string `json:"translation_title,omitempty"`
}
```

- [ ] **Step 4: Add AutoMigrate for UserAnimePreference**

In `services/player/cmd/player-api/main.go`, add `&domain.UserAnimePreference{}` to the AutoMigrate call (after `&domain.WatchHistory{}`).

- [ ] **Step 5: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add services/player/internal/domain/preference.go services/player/internal/domain/watch.go services/player/cmd/player-api/main.go
git commit -m "feat(player): add preference domain types, extend WatchHistory with combo fields"
```

---

## Task 2: Prometheus Metrics

**Files:**
- Create: `libs/metrics/watch.go`

- [ ] **Step 1: Create watch metrics file**

Create `libs/metrics/watch.go`:

```go
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	WatchEpisodesTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "watch_episodes_total",
			Help: "Total episodes marked as watched",
		},
		[]string{"player", "language", "watch_type"},
	)

	WatchActiveSessions = promauto.NewGauge(
		prometheus.GaugeOpts{
			Name: "watch_active_sessions",
			Help: "Number of currently active watch sessions",
		},
	)

	WatchSessionDuration = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "watch_session_duration_seconds",
			Help:    "Duration of watch sessions in seconds",
			Buckets: prometheus.ExponentialBuckets(60, 2, 10), // 1min to ~17hrs
		},
		[]string{"player", "language"},
	)

	TranslationSelectionsTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "translation_selections_total",
			Help: "Total translation selections by users",
		},
		[]string{"player", "language", "watch_type", "translation_title"},
	)

	PreferenceResolutionTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "preference_resolution_total",
			Help: "Total preference resolution outcomes by tier",
		},
		[]string{"tier"},
	)

	PreferenceFallbackTotal = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "preference_fallback_total",
			Help: "Total preference fallback triggers by tier and context",
		},
		[]string{"tier", "language", "watch_type"},
	)
)
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma && go build ./libs/metrics/...`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add libs/metrics/watch.go
git commit -m "feat(metrics): add Prometheus metrics for watch episodes, sessions, preferences"
```

---

## Task 3: (Removed — No Redis)

Player service does not use Redis. Caching is frontend-only (localStorage). This task is intentionally skipped.

---

## Task 4: Preference Repository

**Files:**
- Create: `services/player/internal/repo/preference.go`

- [ ] **Step 1: Write the preference repository**

Create `services/player/internal/repo/preference.go`:

```go
package repo

import (
	"context"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type PreferenceRepository struct {
	db *gorm.DB
}

func NewPreferenceRepository(db *gorm.DB) *PreferenceRepository {
	return &PreferenceRepository{db: db}
}

// UpsertAnimePreference creates or updates the user's per-anime preference
func (r *PreferenceRepository) UpsertAnimePreference(ctx context.Context, pref *domain.UserAnimePreference) error {
	return r.db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
			DoUpdates: clause.AssignmentColumns([]string{"player", "language", "watch_type", "translation_id", "translation_title", "updated_at"}),
		}).
		Create(pref).Error
}

// GetAnimePreference returns the user's saved preference for a specific anime
func (r *PreferenceRepository) GetAnimePreference(ctx context.Context, userID, animeID string) (*domain.UserAnimePreference, error) {
	var pref domain.UserAnimePreference
	err := r.db.WithContext(ctx).
		Where("user_id = ? AND anime_id = ?", userID, animeID).
		First(&pref).Error
	if err != nil {
		return nil, err
	}
	return &pref, nil
}

// GetUserGlobalFavorite returns the user's #1 most-watched combo from watch_history
// Returns domain.ComboCount (defined in domain/preference.go)
func (r *PreferenceRepository) GetUserGlobalFavorite(ctx context.Context, userID string) (*domain.ComboCount, error) {
	var result domain.ComboCount
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_title, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("player, language, watch_type, translation_title").
		Order("count DESC").
		Limit(1).
		Scan(&result).Error
	if err != nil || result.Count == 0 {
		return nil, err
	}
	return &result, nil
}

// GetUserTopCombos returns the user's top combos ranked by watch count
func (r *PreferenceRepository) GetUserTopCombos(ctx context.Context, userID string, limit int) ([]domain.ComboCount, error) {
	var results []domain.ComboCount
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_title, COUNT(*) as count").
		Where("user_id = ?", userID).
		Group("player, language, watch_type, translation_title").
		Order("count DESC").
		Limit(limit).
		Scan(&results).Error
	return results, err
}

// GetCommunityPopularity returns the most popular combos for a specific anime
// Returns domain.CommunityCombo (defined in domain/preference.go)
func (r *PreferenceRepository) GetCommunityPopularity(ctx context.Context, animeID string) ([]domain.CommunityCombo, error) {
	var results []domain.CommunityCombo
	err := r.db.WithContext(ctx).
		Model(&domain.WatchHistory{}).
		Select("player, language, watch_type, translation_id, translation_title, COUNT(DISTINCT user_id) as viewers").
		Where("anime_id = ?", animeID).
		Group("player, language, watch_type, translation_id, translation_title").
		Order("viewers DESC").
		Scan(&results).Error
	return results, err
}

// GetPinnedTranslations queries catalog's pinned_translations table (shared DB)
// Returns domain.PinnedTranslation (defined in domain/preference.go)
func (r *PreferenceRepository) GetPinnedTranslations(ctx context.Context, animeID string) ([]domain.PinnedTranslation, error) {
	var results []domain.PinnedTranslation
	err := r.db.WithContext(ctx).
		Where("anime_id = ?", animeID).
		Find(&results).Error
	return results, err
}

// CreateWatchHistory inserts a watch_history row with full combo context
func (r *PreferenceRepository) CreateWatchHistory(ctx context.Context, history *domain.WatchHistory) error {
	return r.db.WithContext(ctx).Create(history).Error
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/repo/preference.go
git commit -m "feat(player): add preference repository with combo queries"
```

---

## Task 5: Resolver — Tests First (TDD)

**Files:**
- Create: `services/player/internal/service/resolver_test.go`
- Create: `services/player/internal/service/resolver.go`

- [ ] **Step 1: Write resolver test file with all test groups**

Create `services/player/internal/service/resolver_test.go` with table-driven tests covering all 7 groups from the spec:

- Group 1: Tier 1 per-anime preference (exact match, title match cross-player, combo gone)
- Group 2: Tier 2 user global #1 only (team found, team not available)
- Group 3: Tier 3 community popularity (clear winner, filtered by lock, no data, new user no lock)
- Group 4: Tier 4 pinned (matches lock, wrong type, no pinned)
- Group 5: Tier 5 default kodik sub (exists, doesn't exist)
- Group 6: Boundary rules (never cross language, never cross type, lock carries through)
- Group 7: Input validation (empty available, missing anime_id)

Use real translation names: AniLibria/610, AniDUB/609, Crunchyroll/963, SHIZA/616, JAM/971, HD-1/hd-1, HD-2/hd-2, AniRise/1.

The resolver is a pure function using only domain types:

```go
func Resolve(
    userPref *domain.UserAnimePreference,
    globalFav *domain.ComboCount,
    community []domain.CommunityCombo,
    pinned []domain.PinnedTranslation,
    available []domain.WatchCombo,
) *domain.ResolvedCombo
```

All parameter types are defined in `domain/preference.go`. No repo imports needed. This makes it fully testable without mocking — just pass in the data.

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /data/animeenigma && go test ./services/player/internal/service/ -run TestResolve -v`
Expected: FAIL — `resolver.go` doesn't exist yet.

- [ ] **Step 3: Implement the resolver**

Create `services/player/internal/service/resolver.go` with the 5-tier resolution algorithm:

```go
package service

import "github.com/ILITA-hub/animeenigma/services/player/internal/domain"

func Resolve(
	userPref *domain.UserAnimePreference,
	globalFav *domain.ComboCount,
	community []domain.CommunityCombo,
	pinned []domain.PinnedTranslation,
	available []domain.WatchCombo,
) *domain.ResolvedCombo {
	// ... 5-tier algorithm per spec
}
```

Key implementation details:
- Tier 1: Check exact (player+translationID), then title match filtered to same lang+type
- Tier 2: Check only #1 favorite's translation_title, no scoring, no #2/#3
- Tier 3: Filter community results by locked lang+type, take top
- Tier 4: Map pinned `voice→dub`, `subtitles→sub`, always `ru` language, match by title
- Tier 5: Find first kodik+sub combo in available, or return nil
- Lock language+type from first tier that has data

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /data/animeenigma && go test ./services/player/internal/service/ -run TestResolve -v`
Expected: All tests PASS.

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/service/resolver.go services/player/internal/service/resolver_test.go
git commit -m "feat(player): implement 5-tier preference resolver with tests"
```

---

## Task 6: Preference Service

**Files:**
- Create: `services/player/internal/service/preference.go`

- [ ] **Step 1: Write the preference service**

Create `services/player/internal/service/preference.go`:

```go
package service

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type PreferenceService struct {
	prefRepo *repo.PreferenceRepository
	log      *logger.Logger
}

func NewPreferenceService(prefRepo *repo.PreferenceRepository, log *logger.Logger) *PreferenceService {
	return &PreferenceService{prefRepo: prefRepo, log: log}
}
```

No Redis — player service has no Redis connection. DB queries are fast enough for a self-hosted instance with few users.

Methods to implement:
- `UpsertAnimePreference(ctx, userID string, req *domain.UpdateProgressRequest)` — builds `UserAnimePreference` from request combo fields, upserts via repo
- `Resolve(ctx, userID string, req *domain.ResolveRequest) (*domain.ResolveResponse, error)` — loads all 4 data sources from DB (pref, global, community, pinned), calls `Resolve()` pure function, increments metrics
- `GetAnimePreference(ctx, userID, animeID string) (*domain.UserAnimePreference, error)` — simple repo passthrough
- `GetGlobalPreferences(ctx, userID string) ([]domain.ComboCount, error)` — repo passthrough with limit 10

- [ ] **Step 2: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add services/player/internal/service/preference.go
git commit -m "feat(player): add preference service with resolve and upsert"
```

---

## Task 7: Extend Progress Handler & Service

**Files:**
- Modify: `services/player/internal/handler/progress.go:28-49`
- Modify: `services/player/internal/service/progress.go:25-41`

- [ ] **Step 1: Add combo validation to progress handler**

In `services/player/internal/handler/progress.go`, after binding the request (around line 40), add validation:

```go
if req.Player != "" && !domain.ValidateCombo(req.Player, req.Language, req.WatchType) {
    httputil.Error(w, errors.BadRequest("invalid combo fields: player, language, or watch_type"))
    return
}
```

- [ ] **Step 2: Add preference upsert to progress service**

In `services/player/internal/service/progress.go`, after the existing `UpdateProgress` logic, add:

```go
// Upsert anime preference when combo fields are present
if req.Player != "" {
    s.prefService.UpsertAnimePreference(ctx, userID, req)
}
```

This requires injecting `PreferenceService` into `ProgressService`. Update the struct and constructor:

```go
// In services/player/internal/service/progress.go
type ProgressService struct {
	progressRepo *repo.ProgressRepository
	prefService  *PreferenceService  // NEW
	log          *logger.Logger
}

func NewProgressService(progressRepo *repo.ProgressRepository, prefService *PreferenceService, log *logger.Logger) *ProgressService {
	return &ProgressService{progressRepo: progressRepo, prefService: prefService, log: log}
}
```

Update the call in `main.go` (line 72) accordingly — `prefService` must be created before `progressService`:

```go
prefRepo := repo.NewPreferenceRepository(db.DB)
prefService := service.NewPreferenceService(prefRepo, log)
progressService := service.NewProgressService(progressRepo, prefService, log)
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add services/player/internal/handler/progress.go services/player/internal/service/progress.go
git commit -m "feat(player): extend progress handler to validate and upsert combo preferences"
```

---

## Task 8: Extend MarkEpisodeWatched with WatchHistory

**Files:**
- Modify: `services/player/internal/handler/list.go:179-203`
- Modify: `services/player/internal/service/list.go:170-213`

- [ ] **Step 1: Extend the MarkEpisodeWatched request binding**

In `services/player/internal/handler/list.go` (line 179-203), the handler currently binds an inline `struct{ Episode int }`. Replace with:

```go
var req domain.MarkEpisodeWatchedRequest
if err := httputil.Bind(r, &req); err != nil {
    httputil.Error(w, errors.BadRequest("invalid request body"))
    return
}
if req.Player != "" && !domain.ValidateCombo(req.Player, req.Language, req.WatchType) {
    httputil.Error(w, errors.BadRequest("invalid combo fields"))
    return
}
```

Then pass `&req` to the service instead of just `req.Episode`.

- [ ] **Step 2: Create WatchHistory row in service**

In `services/player/internal/service/list.go`, after the existing `IncrementEpisodes` call, add:

```go
// Create watch_history row with full combo context
if req.Player != "" {
    // Look up existing watch_progress for duration_watched
    progress, _ := s.progressRepo.GetByUserAnimeEpisode(ctx, userID, animeID, req.Episode)
    durationWatched := 0
    if progress != nil {
        durationWatched = progress.Progress
    }

    history := &domain.WatchHistory{
        UserID:           userID,
        AnimeID:          animeID,
        EpisodeNumber:    req.Episode,
        Player:           req.Player,
        Language:         req.Language,
        WatchType:        req.WatchType,
        TranslationID:    req.TranslationID,
        TranslationTitle: req.TranslationTitle,
        DurationWatched:  durationWatched,
        WatchedAt:        time.Now(),
    }
    s.prefRepo.CreateWatchHistory(ctx, history)

    // Increment episode watched metric
    metrics.WatchEpisodesTotal.WithLabelValues(req.Player, req.Language, req.WatchType).Inc()
}
```

This requires injecting `PreferenceRepository` and `ProgressRepository` into `ListService`. Update the struct and constructor:

```go
// In services/player/internal/service/list.go
type ListService struct {
	listRepo     *repo.ListRepository
	activityRepo *repo.ActivityRepository
	prefRepo     *repo.PreferenceRepository   // NEW
	progressRepo *repo.ProgressRepository     // NEW
	log          *logger.Logger
}

func NewListService(listRepo *repo.ListRepository, activityRepo *repo.ActivityRepository,
	prefRepo *repo.PreferenceRepository, progressRepo *repo.ProgressRepository, log *logger.Logger) *ListService {
	return &ListService{
		listRepo: listRepo, activityRepo: activityRepo,
		prefRepo: prefRepo, progressRepo: progressRepo, log: log,
	}
}
```

The `MarkEpisodeWatched` method signature changes to accept the extended request:

```go
// Old: func (s *ListService) MarkEpisodeWatched(ctx context.Context, userID, animeID string, episode int) (...)
// New:
func (s *ListService) MarkEpisodeWatched(ctx context.Context, userID, animeID string, req *domain.MarkEpisodeWatchedRequest) (*domain.AnimeListEntry, error) {
	// Existing IncrementEpisodes logic uses req.Episode
	// Then creates WatchHistory if combo fields present
}
```

Update `main.go` (line 73) accordingly:

```go
listService := service.NewListService(listRepo, activityRepo, prefRepo, progressRepo, log)
```

- [ ] **Step 3: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: Build succeeds.

- [ ] **Step 4: Commit**

```bash
git add services/player/internal/handler/list.go services/player/internal/service/list.go
git commit -m "feat(player): create watch_history with combo on markEpisodeWatched"
```

---

## Task 9: Preference HTTP Handlers & Routes

**Files:**
- Create: `services/player/internal/handler/preference.go`
- Modify: `services/player/internal/transport/router.go`

- [ ] **Step 1: Write preference handlers**

Create `services/player/internal/handler/preference.go` with 3 handlers:

- `ResolvePreference(w, r)` — binds `ResolveRequest`, validates anime_id + available not empty, calls `prefService.Resolve()`, returns `ResolveResponse`
- `GetAnimePreference(w, r)` — extracts `animeId` from URL path, calls `prefService.GetAnimePreference()`, returns 404 if not found
- `GetGlobalPreferences(w, r)` — calls `prefService.GetGlobalPreferences()`, wraps in `{ top_combos: [...] }`

- [ ] **Step 2: Register routes**

In `services/player/internal/transport/router.go`, add within the existing `r.Route("/api", func(r chi.Router) { r.Route("/users", func(r chi.Router) { ... }) })` block (after progress routes around line 69):

**IMPORTANT:** Routes are nested under `/api/users` already. Do NOT include the `/api/users` prefix — it would double-prefix and 404.

```go
// Preference routes (inside the /api/users route group)
r.Post("/preferences/resolve", prefHandler.ResolvePreference)
r.Get("/preferences/global", prefHandler.GetGlobalPreferences)
r.Get("/preferences/{animeId}", prefHandler.GetAnimePreference)
```

Note: `/global` must come BEFORE `/{animeId}` to avoid `global` matching as an animeId.

- [ ] **Step 3: Update NewRouter signature**

The `NewRouter` function (line 15-30) currently takes 11 handler parameters. Add `preferenceHandler`:

```go
func NewRouter(
	progressHandler *handler.ProgressHandler,
	listHandler *handler.ListHandler,
	historyHandler *handler.HistoryHandler,
	reviewHandler *handler.ReviewHandler,
	malImportHandler *handler.MALImportHandler,
	malExportHandler *handler.MALExportHandler,
	shikimoriImportHandler *handler.ShikimoriImportHandler,
	reportHandler *handler.ReportHandler,
	syncHandler *handler.SyncHandler,
	activityHandler *handler.ActivityHandler,
	exportHandler *handler.ExportHandler,
	preferenceHandler *handler.PreferenceHandler,  // NEW
	jwtConfig authz.JWTConfig,
	log *logger.Logger,
	metricsCollector *metrics.Collector,
) http.Handler {
```

- [ ] **Step 4: Wire up dependency injection in main.go**

In `services/player/cmd/player-api/main.go`, the full wiring order is:

```go
// After existing repo initialization (line 59-64):
prefRepo := repo.NewPreferenceRepository(db.DB)

// Update service initialization (line 71-75):
prefService := service.NewPreferenceService(prefRepo, log)
progressService := service.NewProgressService(progressRepo, prefService, log)
listService := service.NewListService(listRepo, activityRepo, prefRepo, progressRepo, log)

// After existing handler initialization (line 80-91):
prefHandler := handler.NewPreferenceHandler(prefService, log)

// Update router call (line 97) — add prefHandler before jwtConfig:
router := transport.NewRouter(progressHandler, listHandler, historyHandler,
	reviewHandler, malImportHandler, malExportHandler, shikimoriImportHandler,
	reportHandler, syncHandler, activityHandler, exportHandler,
	prefHandler, cfg.JWT, log, metricsCollector)
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add services/player/internal/handler/preference.go services/player/internal/transport/router.go services/player/cmd/player-api/main.go
git commit -m "feat(player): add preference HTTP handlers and routes"
```

---

## Task 10: Frontend — TypeScript Types & API Client

**Files:**
- Create: `frontend/web/src/types/preference.ts`
- Modify: `frontend/web/src/api/client.ts`

- [ ] **Step 1: Create WatchCombo type**

Create `frontend/web/src/types/preference.ts`:

```typescript
export interface WatchCombo {
  player: 'kodik' | 'animelib' | 'hianime' | 'consumet'
  language: 'ru' | 'en'
  watch_type: 'dub' | 'sub'
  translation_id: string
  translation_title: string
}

export interface ResolvedCombo extends WatchCombo {
  tier: string
  tier_number: number
}

export interface ResolveResponse {
  resolved: ResolvedCombo | null
}
```

- [ ] **Step 2: Add API client methods and update markEpisodeWatched**

In `frontend/web/src/api/client.ts`, add to the `userApi` object (after existing methods around line 202):

```typescript
resolvePreference: (animeId: string, available: WatchCombo[]) =>
  apiClient.post<ResolveResponse>('/users/preferences/resolve', { anime_id: animeId, available }),
getAnimePreference: (animeId: string) =>
  apiClient.get<WatchCombo & { updated_at: string }>(`/users/preferences/${animeId}`),
getGlobalPreferences: () =>
  apiClient.get<{ top_combos: (WatchCombo & { count: number })[] }>('/users/preferences/global'),
```

Also update `markEpisodeWatched` (line 199-200) to accept optional combo:

```typescript
// Old: markEpisodeWatched: (animeId: string, episode: number) =>
//        apiClient.post(`/users/watchlist/${animeId}/episode`, { episode }),
// New:
markEpisodeWatched: (animeId: string, episode: number, combo?: Partial<WatchCombo>) =>
  apiClient.post(`/users/watchlist/${animeId}/episode`, { episode, ...combo }),
```

And update `updateProgress` (line 202) — the existing signature already accepts a Record, so no change needed. Frontend callers will just spread combo into the object.

Import `WatchCombo` and `ResolveResponse` from `@/types/preference`.

- [ ] **Step 3: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`
Expected: Build succeeds (unused imports are OK since components will use them next).

- [ ] **Step 4: Commit**

```bash
git add frontend/web/src/types/preference.ts frontend/web/src/api/client.ts
git commit -m "feat(frontend): add WatchCombo types and preference API client methods"
```

---

## Task 11: Frontend — useWatchPreferences Composable

**Files:**
- Create: `frontend/web/src/composables/useWatchPreferences.ts`

- [ ] **Step 1: Write the composable**

Create `frontend/web/src/composables/useWatchPreferences.ts`:

```typescript
import { ref } from 'vue'
import { userApi } from '@/api/client'
import { useAuthStore } from '@/stores/auth'
import type { WatchCombo, ResolvedCombo } from '@/types/preference'

const CACHE_TTL = 24 * 60 * 60 * 1000 // 24 hours

export function useWatchPreferences(animeId: string) {
  const resolvedCombo = ref<ResolvedCombo | null>(null)
  const isLoading = ref(false)
  const authStore = useAuthStore()

  // Try cached result first
  const cacheKey = `pref:${animeId}`
  const cached = localStorage.getItem(cacheKey)
  if (cached) {
    try {
      const { data, timestamp } = JSON.parse(cached)
      if (Date.now() - timestamp < CACHE_TTL) {
        resolvedCombo.value = data
      }
    } catch { /* ignore corrupt cache */ }
  }

  async function resolve(available: WatchCombo[]) {
    if (!authStore.isAuthenticated || available.length === 0) return

    isLoading.value = true
    try {
      const { data } = await userApi.resolvePreference(animeId, available)
      resolvedCombo.value = data.resolved
      // Cache the result
      localStorage.setItem(cacheKey, JSON.stringify({
        data: data.resolved,
        timestamp: Date.now()
      }))
    } catch (err) {
      console.error('Failed to resolve preference:', err)
    } finally {
      isLoading.value = false
    }
  }

  return { resolvedCombo, isLoading, resolve }
}
```

- [ ] **Step 2: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add frontend/web/src/composables/useWatchPreferences.ts
git commit -m "feat(frontend): add useWatchPreferences composable with localStorage cache"
```

---

## Task 12: Frontend — Extend KodikPlayer with Combo Reporting

**Files:**
- Modify: `frontend/web/src/components/player/KodikPlayer.vue`

- [ ] **Step 1: Add preferredCombo prop**

Add to props (the component uses `defineProps` — add `preferredCombo`):

```typescript
const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  preferredCombo?: WatchCombo | null
}>()
```

Import `WatchCombo` from `@/types/preference`.

- [ ] **Step 2: Build currentCombo reactive**

Add a computed that builds the current combo from selected translation state:

```typescript
const currentCombo = computed((): WatchCombo | null => {
  if (!selectedTranslation.value) return null
  const tr = translations.value.find(t => t.id === selectedTranslation.value)
  if (!tr) return null
  return {
    player: 'kodik',
    language: 'ru',
    watch_type: translationType.value === 'voice' ? 'dub' : 'sub',
    translation_id: String(tr.id),
    translation_title: tr.title
  }
})
```

- [ ] **Step 3: Merge combo into saveProgressServer**

In the `saveProgressServer` method (around line 268), spread `currentCombo.value` into the `updateProgress` payload:

```typescript
await userApi.updateProgress({
  anime_id: props.animeId,
  episode_number: currentEpisode,
  progress: Math.floor(currentTime),
  duration: Math.floor(duration),
  ...currentCombo.value
})
```

- [ ] **Step 4: Merge combo into markCurrentEpisodeWatched**

Update the `markEpisodeWatched` call to include combo fields (client signature already updated in Task 10):

```typescript
await userApi.markEpisodeWatched(props.animeId, epNum, currentCombo.value ?? undefined)
```

- [ ] **Step 5: Auto-select from preferredCombo prop**

After fetching translations and pinned translations (around line 422-445), add logic to check `props.preferredCombo`:

```typescript
// If preferredCombo matches this player, auto-select that translation
if (props.preferredCombo?.player === 'kodik') {
  const match = translations.value.find(t =>
    String(t.id) === props.preferredCombo!.translation_id ||
    t.title === props.preferredCombo!.translation_title
  )
  if (match) {
    selectedTranslation.value = match.id
    translationType.value = match.type
    // skip normal pinned/default selection
    return
  }
}
```

- [ ] **Step 6: Expose available translations for resolve**

Emit available translations so parent page can collect them:

```typescript
const emit = defineEmits<{
  (e: 'availableTranslations', translations: WatchCombo[]): void
}>()
```

After fetching translations, emit them mapped to WatchCombo format.

- [ ] **Step 7: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`
Expected: Build succeeds.

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/components/player/KodikPlayer.vue frontend/web/src/api/client.ts
git commit -m "feat(frontend): extend KodikPlayer with combo reporting and preferredCombo"
```

---

## Task 13: Frontend — Extend HiAnimePlayer with Combo Reporting

**Files:**
- Modify: `frontend/web/src/components/player/HiAnimePlayer.vue`

Same pattern as Task 12:

- [ ] **Step 1: Add `preferredCombo` prop and `WatchCombo` import**

- [ ] **Step 2: Build `currentCombo` computed from `selectedServer` + `selectedCategory`**

```typescript
const currentCombo = computed((): WatchCombo | null => {
  if (!selectedServer.value) return null
  return {
    player: 'hianime',
    language: 'en',
    watch_type: selectedCategory.value,
    translation_id: selectedServer.value.id,
    translation_title: selectedServer.value.name
  }
})
```

- [ ] **Step 3: Merge combo into existing `saveProgress` method (line ~957-962)**

Spread `currentCombo.value` into `userApi.updateProgress()` payload.

- [ ] **Step 4: Merge combo into `markCurrentEpisodeWatched` (line ~986-996)**

Pass combo to `userApi.markEpisodeWatched()`.

- [ ] **Step 5: Auto-select from `preferredCombo` prop after fetching servers**

- [ ] **Step 6: Emit available translations as WatchCombo[]**

- [ ] **Step 7: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/components/player/HiAnimePlayer.vue
git commit -m "feat(frontend): extend HiAnimePlayer with combo reporting and preferredCombo"
```

---

## Task 14: Frontend — Add Heartbeat to AnimeLibPlayer + Combo Reporting

**Files:**
- Modify: `frontend/web/src/components/player/AnimeLibPlayer.vue`

- [ ] **Step 1: Add `preferredCombo` prop**

- [ ] **Step 2: Add 30s progress heartbeat**

AnimeLibPlayer currently has NO `saveProgress` / `updateProgress` calls. Add the same pattern used by KodikPlayer/HiAnimePlayer:

```typescript
const SAVE_INTERVAL = 30
const lastSaveTime = ref(0)

// In handleTimeUpdate (line ~549):
if (currentTime.value - lastSaveTime.value >= SAVE_INTERVAL) {
  lastSaveTime.value = currentTime.value
  saveProgress()
}

// In handlePause (line ~559 — currently empty):
const handlePause = () => {
  if (!selectedEpisode.value) return
  saveProgress()
}

const saveProgress = () => {
  if (!selectedEpisode.value) return
  // localStorage
  const key = `watch_progress:${props.animeId}`
  const existing = JSON.parse(localStorage.getItem(key) || '{}')
  existing[selectedEpisode.value.number] = {
    time: currentTime.value,
    maxTime: maxTime.value,
    updatedAt: Date.now()
  }
  localStorage.setItem(key, JSON.stringify(existing))
  // Server
  if (authStore.isAuthenticated && currentCombo.value) {
    userApi.updateProgress({
      anime_id: props.animeId,
      episode_number: parseInt(selectedEpisode.value.number),
      progress: Math.floor(currentTime.value),
      duration: Math.floor(maxTime.value),
      ...currentCombo.value
    }).catch(() => {})
  }
}
```

- [ ] **Step 3: Build `currentCombo` computed**

```typescript
const currentCombo = computed((): WatchCombo | null => {
  if (!selectedTranslation.value) return null
  return {
    player: 'animelib',
    language: 'ru',
    watch_type: selectedTranslation.value.type === 'voice' ? 'dub' : 'sub',
    translation_id: String(selectedTranslation.value.id),
    translation_title: selectedTranslation.value.team_name
  }
})
```

- [ ] **Step 4: Merge combo into markCurrentEpisodeWatched (line ~581-589)**

- [ ] **Step 5: Auto-select from preferredCombo, emit available translations**

- [ ] **Step 6: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/player/AnimeLibPlayer.vue
git commit -m "feat(frontend): add progress heartbeat to AnimeLibPlayer, combo reporting"
```

---

## Task 15: Frontend — Add Server-Side Progress to ConsumetPlayer + Combo Reporting

**Files:**
- Modify: `frontend/web/src/components/player/ConsumetPlayer.vue`

- [ ] **Step 1: Add `preferredCombo` and `subOrDub` props**

```typescript
const props = defineProps<{
  animeId: string
  animeName?: string
  totalEpisodes?: number
  preferredCombo?: WatchCombo | null
  subOrDub?: 'sub' | 'dub'
}>()
```

- [ ] **Step 2: Replace emit-based progress with direct server save**

ConsumetPlayer currently emits `progress` events (line 837-841) that nobody listens to. Replace with direct save:

```typescript
// Replace: emit('progress', { episode, time, maxTime })
// With:
saveProgress()
```

Add `saveProgress` method (same pattern as AnimeLibPlayer Task 14 step 2) with localStorage + server call using `currentCombo.value`.

- [ ] **Step 3: Build `currentCombo` computed**

```typescript
const currentCombo = computed((): WatchCombo | null => {
  if (!selectedServer.value) return null
  return {
    player: 'consumet',
    language: 'en',
    watch_type: props.subOrDub === 'dub' ? 'dub' : 'sub',
    translation_id: selectedServer.value.name,
    translation_title: selectedServer.value.name
  }
})
```

- [ ] **Step 4: Merge combo into markCurrentEpisodeWatched (line ~869-874)**

- [ ] **Step 5: Auto-select from preferredCombo, emit available translations**

- [ ] **Step 6: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`

- [ ] **Step 7: Commit**

```bash
git add frontend/web/src/components/player/ConsumetPlayer.vue
git commit -m "feat(frontend): add server progress save to ConsumetPlayer, combo reporting"
```

---

## Task 16: Frontend — Wire Up Anime.vue with Preference Resolution

**Files:**
- Modify: `frontend/web/src/views/Anime.vue`

- [ ] **Step 1: Import and use `useWatchPreferences`**

```typescript
import { useWatchPreferences } from '@/composables/useWatchPreferences'
import type { WatchCombo } from '@/types/preference'

// After anime is loaded:
const { resolvedCombo, resolve } = useWatchPreferences(anime.value?.id ?? '')
```

- [ ] **Step 2: Collect available translations from player components**

Add state to collect emissions from all 4 players:

```typescript
const availableTranslations = ref<WatchCombo[]>([])

const handleAvailableTranslations = (combos: WatchCombo[]) => {
  availableTranslations.value.push(...combos)
}
```

Listen to `@availableTranslations` on each player component.

- [ ] **Step 3: Trigger resolve after translations are collected**

Use a watcher or `nextTick` to call `resolve(availableTranslations.value)` once translations are populated. Only the active player emits, so resolve when the active player's translations arrive.

- [ ] **Step 4: Auto-switch player tab based on resolved combo**

When `resolvedCombo.value` is set, switch `videoProvider.value` to `resolvedCombo.value.player` and `videoLanguage.value` to `resolvedCombo.value.language`.

- [ ] **Step 5: Pass preferredCombo prop to all players**

Update template (lines 346-372):

```html
<KodikPlayer
  v-if="videoProvider === 'kodik'"
  :anime-id="anime.id"
  :anime-name="anime.title"
  :total-episodes="anime.totalEpisodes"
  :preferred-combo="resolvedCombo"
  @available-translations="handleAvailableTranslations"
/>
<!-- Same for AnimeLib, HiAnime, Consumet -->
<!-- ConsumetPlayer also gets :sub-or-dub="'sub'" (or pass from search results) -->
```

- [ ] **Step 6: Verify frontend builds**

Run: `cd /data/animeenigma/frontend/web && bun run build`

- [ ] **Step 7: Test manually**

1. Open an anime page, select a translation, watch for 30+ seconds
2. Refresh — should auto-select the same translation
3. Open on another browser/incognito while logged in — should resolve same combo

- [ ] **Step 8: Commit**

```bash
git add frontend/web/src/views/Anime.vue
git commit -m "feat(frontend): wire Anime.vue with preference resolution and auto-player switching"
```

---

## Task 17: Grafana Dashboard Provisioning

**Files:**
- Create: Grafana dashboard JSON (location depends on deployment setup — check existing dashboards)

- [ ] **Step 1: Check where existing Grafana dashboards are provisioned**

Look in `docker/grafana/dashboards/` or `deploy/kustomize/monitoring/` for existing dashboard JSON files.

- [ ] **Step 2: Create watch preferences dashboard JSON**

Create 3 dashboard JSON files per spec Section 5:
1. Viewing Activity — active sessions gauge, episodes watched, episodes/hour
2. Content Preferences — player/language/type pie charts, top translations table
3. Preference Resolution — tier distribution pie, null rate stat, fallback trends

Use the exact PromQL queries from the spec.

- [ ] **Step 3: Commit**

```bash
git add docker/grafana/dashboards/ # or deploy path
git commit -m "feat(grafana): add watch activity, preferences, and resolution dashboards"
```

---

## Task 18: Integration Test & Deploy

- [ ] **Step 1: Run full backend build**

Run: `cd /data/animeenigma && go build ./...`
Expected: All services build.

- [ ] **Step 2: Run full frontend build**

Run: `cd /data/animeenigma/frontend/web && bun run build`
Expected: Build succeeds.

- [ ] **Step 3: Redeploy player service**

Run: `make redeploy-player`
Expected: Player service restarts, AutoMigrate creates new tables.

- [ ] **Step 4: Redeploy frontend**

Run: `make redeploy-web`
Expected: Frontend rebuilds with new composable and player changes.

- [ ] **Step 5: Verify health**

Run: `make health`
Expected: All services healthy.

- [ ] **Step 6: Verify new tables exist**

```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma \
  -c "\dt user_anime_preferences" -c "\dt watch_histories"
```
Expected: Both tables listed.

- [ ] **Step 7: Verify new metrics are exposed**

```bash
curl -s http://localhost:8083/metrics | grep watch_episodes_total
curl -s http://localhost:8083/metrics | grep preference_resolution_total
```
Expected: Metric names appear (with 0 values initially).

- [ ] **Step 8: Manual smoke test**

1. Log in, open an anime, select HiAnime HD-1 dub
2. Watch for 30+ seconds (progress heartbeat fires)
3. Check DB: `SELECT * FROM user_anime_preferences LIMIT 5;`
4. Refresh page — should auto-select HiAnime HD-1 dub
5. Mark episode as watched
6. Check DB: `SELECT * FROM watch_histories LIMIT 5;` — should have combo fields

- [ ] **Step 9: Final commit**

```bash
git add -A
git commit -m "feat(player): watch preference resolution system - complete"
```
