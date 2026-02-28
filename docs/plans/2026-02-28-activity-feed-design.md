# Activity Feed Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add a public activity feed on the home page showing user anime status changes and score ratings.

**Architecture:** Event sourcing via a new `activity_events` table in the player service. Events are recorded in the service layer when users change watchlist statuses or set scores via reviews. A public API endpoint serves the feed with cursor-based pagination. The frontend displays cards with anime poster, username, action text, and relative time below the existing three columns on the home page.

**Tech Stack:** Go (chi router, GORM), Vue 3 (Composition API), Tailwind CSS, Axios

---

### Task 1: Domain Model — ActivityEvent

**Files:**
- Create: `services/player/internal/domain/activity.go`

**Step 1: Create the domain model file**

```go
package domain

import (
	"time"

	"gorm.io/gorm"
)

type ActivityEvent struct {
	ID        string         `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	UserID    string         `gorm:"type:uuid;index" json:"user_id"`
	Username  string         `gorm:"size:32" json:"username"`
	AnimeID   string         `gorm:"type:uuid;index" json:"anime_id"`
	Anime     *AnimeInfo     `gorm:"foreignKey:AnimeID" json:"anime,omitempty"`
	Type      string         `gorm:"size:20;index" json:"type"`
	OldValue  string         `gorm:"size:50" json:"old_value"`
	NewValue  string         `gorm:"size:50" json:"new_value"`
	CreatedAt time.Time      `gorm:"index" json:"created_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (ActivityEvent) TableName() string { return "activity_events" }
```

**Step 2: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: No errors

**Step 3: Commit**

```bash
git add services/player/internal/domain/activity.go
git commit -m "feat(player): add ActivityEvent domain model"
```

---

### Task 2: Activity Repository

**Files:**
- Create: `services/player/internal/repo/activity.go`
- Create: `services/player/internal/repo/activity_test.go`

**Step 1: Write the repository test**

```go
package repo

import (
	"context"
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupTestDB(t *testing.T) *gorm.DB {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	require.NoError(t, err)
	err = db.AutoMigrate(&domain.ActivityEvent{})
	require.NoError(t, err)
	return db
}

func TestActivityRepository_Create(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityRepository(db)

	event := &domain.ActivityEvent{
		UserID:   "user-1",
		Username: "testuser",
		AnimeID:  "anime-1",
		Type:     "status_change",
		OldValue: "",
		NewValue: "watching",
	}

	err := repo.Create(context.Background(), event)
	require.NoError(t, err)
	assert.NotEmpty(t, event.ID)
}

func TestActivityRepository_GetFeed(t *testing.T) {
	db := setupTestDB(t)
	repo := NewActivityRepository(db)

	// Create 3 events with different timestamps
	for i := 0; i < 3; i++ {
		event := &domain.ActivityEvent{
			UserID:    "user-1",
			Username:  "testuser",
			AnimeID:   "anime-1",
			Type:      "status_change",
			NewValue:  "watching",
			CreatedAt: time.Now().Add(time.Duration(-i) * time.Minute),
		}
		err := repo.Create(context.Background(), event)
		require.NoError(t, err)
	}

	events, hasMore, err := repo.GetFeed(context.Background(), 2, "")
	require.NoError(t, err)
	assert.Len(t, events, 2)
	assert.True(t, hasMore)

	// Second page using cursor
	events2, hasMore2, err := repo.GetFeed(context.Background(), 2, events[1].ID)
	require.NoError(t, err)
	assert.Len(t, events2, 1)
	assert.False(t, hasMore2)
}
```

**Step 2: Run tests — verify they fail**

Run: `cd /data/animeenigma/services/player && go test ./internal/repo/ -run TestActivityRepository -v`
Expected: Compilation error — `NewActivityRepository` not defined

**Step 3: Write the repository**

```go
package repo

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/services/player/internal/domain"
	"gorm.io/gorm"
)

type ActivityRepository struct {
	db *gorm.DB
}

func NewActivityRepository(db *gorm.DB) *ActivityRepository {
	return &ActivityRepository{db: db}
}

// Create records a new activity event.
func (r *ActivityRepository) Create(ctx context.Context, event *domain.ActivityEvent) error {
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now()
	}
	return r.db.WithContext(ctx).Create(event).Error
}

// GetFeed returns the latest activity events with cursor-based pagination.
// Pass empty `before` for the first page. `before` is the ID of the last event from the previous page.
func (r *ActivityRepository) GetFeed(ctx context.Context, limit int, before string) ([]*domain.ActivityEvent, bool, error) {
	query := r.db.WithContext(ctx).
		Preload("Anime").
		Order("created_at DESC, id DESC")

	if before != "" {
		// Get the created_at of the cursor event
		var cursor domain.ActivityEvent
		if err := r.db.WithContext(ctx).Select("created_at").Where("id = ?", before).First(&cursor).Error; err != nil {
			return nil, false, err
		}
		query = query.Where("created_at < ? OR (created_at = ? AND id < ?)", cursor.CreatedAt, cursor.CreatedAt, before)
	}

	var events []*domain.ActivityEvent
	// Fetch one extra to determine hasMore
	err := query.Limit(limit + 1).Find(&events).Error
	if err != nil {
		return nil, false, err
	}

	hasMore := len(events) > limit
	if hasMore {
		events = events[:limit]
	}

	return events, hasMore, nil
}
```

**Step 4: Run tests — verify they pass**

Run: `cd /data/animeenigma/services/player && go test ./internal/repo/ -run TestActivityRepository -v`
Expected: PASS

Note: If sqlite driver is not available, add `github.com/gorm-io/driver/sqlite` to go.mod or use the existing test patterns in the repo. If the tests can't run with SQLite, skip the test step and verify manually after deployment.

**Step 5: Commit**

```bash
git add services/player/internal/repo/activity.go services/player/internal/repo/activity_test.go
git commit -m "feat(player): add activity repository with cursor pagination"
```

---

### Task 3: Activity Handler — Feed Endpoint

**Files:**
- Create: `services/player/internal/handler/activity.go`

**Step 1: Write the handler**

Follow the exact pattern from `services/player/internal/handler/review.go`.

```go
package handler

import (
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/player/internal/repo"
)

type ActivityHandler struct {
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
}

func NewActivityHandler(activityRepo *repo.ActivityRepository, log *logger.Logger) *ActivityHandler {
	return &ActivityHandler{
		activityRepo: activityRepo,
		log:          log,
	}
}

// GetFeed returns the public activity feed.
func (h *ActivityHandler) GetFeed(w http.ResponseWriter, r *http.Request) {
	limit := 10
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 50 {
			limit = parsed
		}
	}

	before := r.URL.Query().Get("before")

	events, hasMore, err := h.activityRepo.GetFeed(r.Context(), limit, before)
	if err != nil {
		h.log.Errorw("failed to get activity feed", "error", err)
		httputil.Error(w, err)
		return
	}

	httputil.OK(w, map[string]interface{}{
		"events":   events,
		"has_more": hasMore,
	})
}
```

**Step 2: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: No errors

**Step 3: Commit**

```bash
git add services/player/internal/handler/activity.go
git commit -m "feat(player): add activity feed HTTP handler"
```

---

### Task 4: Wire Backend — Router, Main, Gateway

**Files:**
- Modify: `services/player/internal/transport/router.go:15-28,50-117`
- Modify: `services/player/cmd/player-api/main.go:47-93`
- Modify: `services/gateway/internal/transport/router.go:88-123`

**Step 1: Add activityHandler to router.go**

In `services/player/internal/transport/router.go`, add `activityHandler *handler.ActivityHandler` parameter to `NewRouter` function and register the public route.

Add parameter to NewRouter (after `syncHandler`):
```go
activityHandler *handler.ActivityHandler,
```

Add route — OUTSIDE the protected `/users` group, inside the `/api` route, after the public watchlist route (after line 98):
```go
// Public activity feed
r.Get("/activity/feed", activityHandler.GetFeed)
```

**Step 2: Add wiring in main.go**

In `services/player/cmd/player-api/main.go`:

Add to AutoMigrate (after `&domain.SyncJob{}`):
```go
&domain.ActivityEvent{},
```

Add after `syncRepo` initialization (after line 62):
```go
activityRepo := repo.NewActivityRepository(db.DB)
```

Add after `syncHandler` initialization (after line 87):
```go
activityHandler := handler.NewActivityHandler(activityRepo, log)
```

Update `transport.NewRouter` call to include `activityHandler` (after `syncHandler`).

**Step 3: Add gateway route**

In `services/gateway/internal/transport/router.go`, add the public activity feed route. This must go BEFORE the protected `/users/*` catch-all (before line 120).

After the public watchlist route (line 117):
```go
// Public activity feed
r.Get("/activity/feed", proxyHandler.ProxyToPlayer)
```

**Step 4: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/... && go build ./services/gateway/...`
Expected: No errors

**Step 5: Commit**

```bash
git add services/player/internal/transport/router.go services/player/cmd/player-api/main.go services/gateway/internal/transport/router.go
git commit -m "feat: wire activity feed endpoint in player and gateway"
```

---

### Task 5: Record Events — ListService Integration

**Files:**
- Modify: `services/player/internal/service/list.go:12-22,38-117`

**Step 1: Inject ActivityRepository into ListService**

Add `activityRepo` field and update constructor:

```go
type ListService struct {
	listRepo     *repo.ListRepository
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
}

func NewListService(listRepo *repo.ListRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *ListService {
	return &ListService{
		listRepo:     listRepo,
		activityRepo: activityRepo,
		log:          log,
	}
}
```

**Step 2: Record status_change events in UpdateListEntry**

After the successful `s.listRepo.Upsert(ctx, entry)` call (after current line 112), before the return, add:

```go
// Record activity event if status changed
oldStatus := ""
if existingEntry != nil {
	oldStatus = existingEntry.Status
}
if oldStatus != req.Status {
	activityEvent := &domain.ActivityEvent{
		UserID:   userID,
		AnimeID:  req.AnimeID,
		Type:     "status_change",
		OldValue: oldStatus,
		NewValue: req.Status,
	}
	if err := s.activityRepo.Create(ctx, activityEvent); err != nil {
		s.log.Errorw("failed to record status change activity",
			"user_id", userID,
			"anime_id", req.AnimeID,
			"error", err,
		)
	}
}
```

**Step 3: Update main.go — pass activityRepo to NewListService**

In `services/player/cmd/player-api/main.go`, update the `NewListService` call:

```go
listService := service.NewListService(listRepo, activityRepo, log)
```

**Step 4: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: No errors

**Step 5: Commit**

```bash
git add services/player/internal/service/list.go services/player/cmd/player-api/main.go
git commit -m "feat(player): record status_change events in ListService"
```

---

### Task 6: Record Events — ReviewService Integration

**Files:**
- Modify: `services/player/internal/service/review.go:12-24,27-58`

**Step 1: Inject ActivityRepository into ReviewService**

Add `activityRepo` field and update constructor:

```go
type ReviewService struct {
	reviewRepo   *repo.ReviewRepository
	listRepo     *repo.ListRepository
	activityRepo *repo.ActivityRepository
	log          *logger.Logger
}

func NewReviewService(reviewRepo *repo.ReviewRepository, listRepo *repo.ListRepository, activityRepo *repo.ActivityRepository, log *logger.Logger) *ReviewService {
	return &ReviewService{
		reviewRepo:   reviewRepo,
		listRepo:     listRepo,
		activityRepo: activityRepo,
		log:          log,
	}
}
```

**Step 2: Record score events in CreateOrUpdateReview**

After the successful `s.reviewRepo.Upsert(ctx, review)` call (after current line 40), add:

```go
// Record activity event for score
// Check if there was a previous score
var oldScore string
existingReview, _ := s.reviewRepo.GetByUserAndAnime(ctx, userID, req.AnimeID)
if existingReview != nil && existingReview.ID != review.ID {
	oldScore = strconv.Itoa(existingReview.Score)
}

activityEvent := &domain.ActivityEvent{
	UserID:   userID,
	Username: username,
	AnimeID:  req.AnimeID,
	Type:     "score",
	OldValue: oldScore,
	NewValue: strconv.Itoa(req.Score),
}
if err := s.activityRepo.Create(ctx, activityEvent); err != nil {
	s.log.Errorw("failed to record score activity",
		"user_id", userID,
		"anime_id", req.AnimeID,
		"error", err,
	)
}
```

Note: Need to add `"strconv"` to imports.

Actually, since the Upsert is an insert-or-update, checking for existing review AFTER the upsert won't work properly. Better approach: check for existing review BEFORE the upsert (we already have the data), and only record an event if the score is different.

Revised approach — add before the `s.reviewRepo.Upsert` call:

```go
// Check existing score for activity tracking
existingReview, _ := s.reviewRepo.GetByUserAndAnime(ctx, userID, req.AnimeID)
var oldScore int
if existingReview != nil {
	oldScore = existingReview.Score
}
```

Then after the successful upsert:
```go
// Record score activity event (only if score actually changed)
if oldScore != req.Score {
	activityEvent := &domain.ActivityEvent{
		UserID:   userID,
		Username: username,
		AnimeID:  req.AnimeID,
		Type:     "score",
		OldValue: "",
		NewValue: strconv.Itoa(req.Score),
	}
	if oldScore > 0 {
		activityEvent.OldValue = strconv.Itoa(oldScore)
	}
	if err := s.activityRepo.Create(ctx, activityEvent); err != nil {
		s.log.Errorw("failed to record score activity",
			"user_id", userID,
			"anime_id", req.AnimeID,
			"error", err,
		)
	}
}
```

**Step 3: Update main.go — pass activityRepo to NewReviewService**

```go
reviewService := service.NewReviewService(reviewRepo, listRepo, activityRepo, log)
```

**Step 4: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: No errors

**Step 5: Commit**

```bash
git add services/player/internal/service/review.go services/player/cmd/player-api/main.go
git commit -m "feat(player): record score events in ReviewService"
```

---

### Task 7: Populate Username in Status Events

**Files:**
- Modify: `services/player/internal/handler/list.go:52-81,84-104`

The `ListService.UpdateListEntry` doesn't receive `username` from claims. We need to pass it through so the activity event has the username. Two options:

**Option A (simpler):** Set username in the handler before calling service, by modifying the handler to pass claims info.

**Option B (cleaner):** Have the activity repo look up username. But we don't have access to users table in player service.

**Best approach:** The handler has access to `claims.Username`. Pass it through the service method. Modify `UpdateListEntry` to accept username, and store it on the activity event.

**Step 1: Update ListService.UpdateListEntry signature**

In `services/player/internal/service/list.go`, change:
```go
func (s *ListService) UpdateListEntry(ctx context.Context, userID string, req *domain.UpdateListRequest) (*domain.AnimeListEntry, error) {
```
to:
```go
func (s *ListService) UpdateListEntry(ctx context.Context, userID, username string, req *domain.UpdateListRequest) (*domain.AnimeListEntry, error) {
```

And set `Username` on the activity event:
```go
activityEvent := &domain.ActivityEvent{
	UserID:   userID,
	Username: username,
	AnimeID:  req.AnimeID,
	...
}
```

**Step 2: Update all callers of UpdateListEntry**

In `services/player/internal/handler/list.go`:
- `AddToList` (line 73): `h.listService.UpdateListEntry(r.Context(), claims.UserID, claims.Username, listReq)`
- `UpdateListEntry` (line 97): `h.listService.UpdateListEntry(r.Context(), claims.UserID, claims.Username, &req)`

Search for all other callers in the player service (MAL import, Shikimori import, MigrateListEntry). Those callers may not have a username easily — for imports, pass empty string (import events are noisy anyway, we won't record activity for them).

In `services/player/internal/handler/mal_import.go` and `services/player/internal/handler/shikimori_import.go` — find all calls to `listService.UpdateListEntry` and add `""` as username parameter.

In `services/player/internal/service/list.go` `MigrateListEntry` method — the internal call to `s.listRepo.Upsert` doesn't go through `UpdateListEntry`, so no change needed there.

**Step 3: Suppress activity events for imports**

In the activity event recording code in `UpdateListEntry`, skip if username is empty (import case):
```go
if oldStatus != req.Status && username != "" {
	// record activity event...
}
```

**Step 4: Verify it compiles**

Run: `cd /data/animeenigma && go build ./services/player/...`
Expected: No errors

**Step 5: Commit**

```bash
git add services/player/internal/service/list.go services/player/internal/handler/list.go services/player/internal/handler/mal_import.go services/player/internal/handler/shikimori_import.go
git commit -m "feat(player): pass username to UpdateListEntry for activity events"
```

---

### Task 8: Frontend — API Client

**Files:**
- Modify: `frontend/web/src/api/client.ts`

**Step 1: Add activityApi export**

After the `reviewApi` export block (after line 216), add:

```typescript
export const activityApi = {
  getFeed: (limit: number = 10, before?: string) =>
    apiClient.get('/activity/feed', {
      params: { limit, ...(before && { before }) }
    }),
}
```

**Step 2: Commit**

```bash
git add frontend/web/src/api/client.ts
git commit -m "feat(frontend): add activityApi to API client"
```

---

### Task 9: Frontend — ActivityFeed Component

**Files:**
- Create: `frontend/web/src/components/ActivityFeed.vue`

**Step 1: Create the component**

```vue
<template>
  <div class="glass-card rounded-2xl p-5">
    <div class="flex items-center gap-3 mb-5">
      <div class="w-10 h-10 rounded-xl bg-gradient-to-br from-purple-500 to-pink-500 flex items-center justify-center">
        <svg class="w-5 h-5 text-white" fill="none" stroke="currentColor" viewBox="0 0 24 24">
          <path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z" />
        </svg>
      </div>
      <h2 class="text-xl font-bold text-white">Активность</h2>
    </div>

    <!-- Loading skeleton -->
    <div v-if="loading && events.length === 0" class="space-y-3">
      <div v-for="i in 4" :key="i" class="animate-pulse flex gap-3 p-2">
        <div class="w-12 h-16 bg-white/10 rounded-lg flex-shrink-0"></div>
        <div class="flex-1 space-y-2">
          <div class="h-3 bg-white/10 rounded w-1/4"></div>
          <div class="h-4 bg-white/10 rounded w-3/4"></div>
          <div class="h-3 bg-white/10 rounded w-1/3"></div>
        </div>
      </div>
    </div>

    <!-- Events list -->
    <div v-else class="space-y-2">
      <div
        v-for="event in events"
        :key="event.id"
        class="flex gap-3 p-2 rounded-xl hover:bg-white/5 transition-colors"
      >
        <!-- Anime poster -->
        <router-link
          :to="`/anime/${event.anime_id}`"
          class="flex-shrink-0"
        >
          <img
            :src="event.anime?.poster_url || '/placeholder.svg'"
            :alt="event.anime?.name_ru || event.anime?.name || ''"
            class="w-12 h-16 object-cover rounded-lg"
          />
        </router-link>

        <!-- Event info -->
        <div class="flex-1 min-w-0">
          <p class="text-xs text-gray-400">
            {{ event.username }}
          </p>
          <p class="text-sm text-white mt-0.5">
            <span>{{ actionText(event) }}</span>
            <router-link
              :to="`/anime/${event.anime_id}`"
              class="text-purple-400 hover:text-purple-300 transition-colors"
            >
              {{ animeName(event) }}
            </router-link>
          </p>
          <p class="text-xs text-gray-500 mt-1">
            {{ formatRelativeTime(event.created_at) }}
          </p>
        </div>
      </div>

      <!-- Empty state -->
      <div v-if="events.length === 0 && !loading" class="text-center py-8 text-gray-400">
        Пока нет активности
      </div>

      <!-- Load more button -->
      <button
        v-if="hasMore"
        @click="loadMore"
        :disabled="loading"
        class="w-full mt-3 py-2.5 text-sm text-gray-400 hover:text-white bg-white/5 hover:bg-white/10 rounded-xl transition-colors disabled:opacity-50"
      >
        {{ loading ? 'Загрузка...' : 'Показать ещё' }}
      </button>
    </div>
  </div>
</template>

<script setup lang="ts">
import { ref, onMounted } from 'vue'
import { activityApi } from '@/api/client'

interface ActivityEvent {
  id: string
  user_id: string
  username: string
  anime_id: string
  anime?: {
    id: string
    name: string
    name_ru?: string
    poster_url?: string
  }
  type: string
  old_value: string
  new_value: string
  created_at: string
}

const events = ref<ActivityEvent[]>([])
const hasMore = ref(false)
const loading = ref(true)

const loadFeed = async (before?: string) => {
  loading.value = true
  try {
    const response = await activityApi.getFeed(10, before)
    const data = response.data?.data || response.data
    const newEvents: ActivityEvent[] = data?.events || []
    if (before) {
      events.value.push(...newEvents)
    } else {
      events.value = newEvents
    }
    hasMore.value = data?.has_more || false
  } catch (err) {
    console.error('Failed to load activity feed:', err)
  } finally {
    loading.value = false
  }
}

const loadMore = () => {
  if (events.value.length > 0) {
    const lastEvent = events.value[events.value.length - 1]
    loadFeed(lastEvent.id)
  }
}

const actionText = (event: ActivityEvent): string => {
  if (event.type === 'score') {
    return `поставил ${event.new_value}/10 — `
  }
  const statusTexts: Record<string, string> = {
    watching: 'начал смотреть ',
    completed: 'завершил ',
    dropped: 'дропнул ',
    plan_to_watch: 'добавил в список ',
    on_hold: 'поставил на паузу ',
    rewatching: 'пересматривает ',
  }
  return statusTexts[event.new_value] || `обновил статус — `
}

const animeName = (event: ActivityEvent): string => {
  if (!event.anime) return 'Неизвестное аниме'
  return event.anime.name_ru || event.anime.name || 'Неизвестное аниме'
}

const formatRelativeTime = (dateStr: string): string => {
  const date = new Date(dateStr)
  const now = new Date()
  const diffMs = now.getTime() - date.getTime()
  const diffMinutes = Math.floor(diffMs / (1000 * 60))
  const diffHours = Math.floor(diffMs / (1000 * 60 * 60))
  const diffDays = Math.floor(diffMs / (1000 * 60 * 60 * 24))

  if (diffMinutes < 1) return 'только что'
  if (diffMinutes < 60) return `${diffMinutes} мин. назад`
  if (diffHours < 24) return `${diffHours} ч. назад`
  if (diffDays === 1) return 'вчера'
  if (diffDays < 7) return `${diffDays} дн. назад`
  return date.toLocaleDateString('ru-RU', { day: 'numeric', month: 'short' })
}

onMounted(() => {
  loadFeed()
})
</script>
```

**Step 2: Commit**

```bash
git add frontend/web/src/components/ActivityFeed.vue
git commit -m "feat(frontend): add ActivityFeed component"
```

---

### Task 10: Frontend — Add ActivityFeed to Home.vue

**Files:**
- Modify: `frontend/web/src/views/Home.vue`

**Step 1: Import the component**

In the `<script setup>` section, after the existing imports (line 280):

```typescript
import ActivityFeed from '@/components/ActivityFeed.vue'
```

Remove `reviewApi` from the `import { animeApi, reviewApi } from '@/api/client'` if it's no longer needed elsewhere... actually keep it, it's used for batch ratings.

**Step 2: Add the ActivityFeed block below the grid**

After the closing `</div>` of the grid (after line 272), but still inside the `max-w-7xl` container div, add:

```html
<!-- Activity Feed -->
<div class="mt-6">
  <ActivityFeed />
</div>
```

**Step 3: Verify the build**

Run: `cd /data/animeenigma/frontend/web && bun run build`
Expected: No errors

**Step 4: Commit**

```bash
git add frontend/web/src/views/Home.vue
git commit -m "feat(frontend): add activity feed section to home page"
```

---

### Task 11: Add Locale Strings (en, ja, ru)

**Files:**
- Modify: `frontend/web/src/locales/en.json`
- Modify: `frontend/web/src/locales/ja.json`
- Modify: `frontend/web/src/locales/ru.json`

**Step 1: Add activity section to ru.json**

Add before the closing `}`:
```json
"activity": {
  "title": "Активность",
  "empty": "Пока нет активности",
  "loadMore": "Показать ещё",
  "loading": "Загрузка...",
  "justNow": "только что",
  "minutesAgo": "{n} мин. назад",
  "hoursAgo": "{n} ч. назад",
  "yesterday": "вчера",
  "daysAgo": "{n} дн. назад",
  "score": "поставил {score}/10 —",
  "status": {
    "watching": "начал смотреть",
    "completed": "завершил",
    "dropped": "дропнул",
    "plan_to_watch": "добавил в список",
    "on_hold": "поставил на паузу",
    "rewatching": "пересматривает"
  }
}
```

**Step 2: Add activity section to en.json**

```json
"activity": {
  "title": "Activity",
  "empty": "No activity yet",
  "loadMore": "Show more",
  "loading": "Loading...",
  "justNow": "just now",
  "minutesAgo": "{n} min ago",
  "hoursAgo": "{n}h ago",
  "yesterday": "yesterday",
  "daysAgo": "{n}d ago",
  "score": "rated {score}/10 —",
  "status": {
    "watching": "started watching",
    "completed": "completed",
    "dropped": "dropped",
    "plan_to_watch": "added to list",
    "on_hold": "put on hold",
    "rewatching": "is rewatching"
  }
}
```

**Step 3: Add activity section to ja.json**

```json
"activity": {
  "title": "アクティビティ",
  "empty": "まだアクティビティはありません",
  "loadMore": "もっと見る",
  "loading": "読み込み中...",
  "justNow": "たった今",
  "minutesAgo": "{n}分前",
  "hoursAgo": "{n}時間前",
  "yesterday": "昨日",
  "daysAgo": "{n}日前",
  "score": "{score}/10 と評価 —",
  "status": {
    "watching": "視聴開始",
    "completed": "視聴完了",
    "dropped": "視聴中止",
    "plan_to_watch": "リストに追加",
    "on_hold": "一時停止",
    "rewatching": "再視聴中"
  }
}
```

Note: The ActivityFeed component currently uses hardcoded Russian strings. If the app starts using i18n in the component, these locale keys will be ready. For now, the hardcoded strings are consistent with the rest of Home.vue which also uses hardcoded Russian.

**Step 4: Commit**

```bash
git add frontend/web/src/locales/en.json frontend/web/src/locales/ja.json frontend/web/src/locales/ru.json
git commit -m "feat(i18n): add activity feed locale strings"
```

---

### Task 12: Deploy and Verify

**Step 1: Redeploy player service**

Run: `make redeploy-player`

**Step 2: Redeploy gateway**

Run: `make redeploy-gateway`

**Step 3: Redeploy web frontend**

Run: `make redeploy-web`

**Step 4: Verify the API endpoint works**

Run: `curl -s http://localhost:8000/api/activity/feed | jq .`
Expected: `{"data": {"events": [], "has_more": false}}`

**Step 5: Test by changing a watchlist status or posting a review, then checking the feed again**

**Step 6: Verify Home.vue shows the activity block below the three columns**
