# Watch Preference Resolution System — Design Spec

**Date:** 2026-03-18
**Status:** Draft
**Approach:** Hybrid — backend stores + resolves, frontend caches

## Problem

Users lose their player/dub/translation selection when returning to an anime or switching devices. No cross-device persistence exists for watch preferences. The platform also lacks business metrics around viewing behavior.

## Goals

1. Persist per-anime watch preferences (player, language, dub/sub type, translation team) server-side
2. Learn user's global favorite combo from watch history
3. Smart fallback resolution when preferred combo isn't available for a new anime
4. Expose all watch activity as Prometheus metrics for Grafana dashboards
5. Comprehensive test coverage with real translation data

## Non-Goals

- Mobile app support (frontend-only for now)
- Anonymous user preference sync (localStorage only, existing behavior)
- Recommendation engine (this is preference restoration, not discovery)

---

## 1. Data Model

### Watch Combo — Universal Preference Tuple

Every watch session is described by this tuple, normalized across all 4 players:

```
WatchCombo {
    Player:           "kodik" | "animelib" | "hianime" | "consumet"
    Language:          "ru" | "en"
    Type:              "dub" | "sub"
    TranslationID:     string   // provider-specific, always stored as string
                                // (Kodik int "610", HiAnime string "hd-1")
    TranslationTitle:  string   // human-readable ("AniLibria", "HD-1")
}
```

Language mapping is deterministic from player: Kodik/AnimeLib = `ru`, HiAnime/Consumet = `en`.

**Type mapping from provider-specific values:**
- Kodik/AnimeLib `"voice"` → `"dub"`, `"subtitles"` → `"sub"`
- HiAnime `"dub"` → `"dub"`, `"sub"` → `"sub"`, `"raw"` → `"sub"`
- Consumet: determined by `subOrDub` field from search results, passed as prop

**Translation ID normalization:** All provider IDs stored as strings. Kodik/AnimeLib integer IDs are converted via `strconv.Itoa()` / `String()` at the boundary.

### Restructured `watch_history` Table

Currently empty and unused. Restructure to capture full watch context.

**GORM Model:**

```go
type WatchHistory struct {
    ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    UserID           string    `gorm:"type:uuid;index;not null" json:"user_id"`
    AnimeID          string    `gorm:"index;not null" json:"anime_id"`
    EpisodeNumber    int       `gorm:"not null" json:"episode_number"`
    Player           string    `gorm:"size:20;not null" json:"player"`
    Language         string    `gorm:"size:5;not null" json:"language"`
    WatchType        string    `gorm:"size:5;not null" json:"watch_type"`
    TranslationID    string    `gorm:"size:50" json:"translation_id"`
    TranslationTitle string    `gorm:"size:200" json:"translation_title"`
    DurationWatched  int       `gorm:"default:0" json:"duration_watched"`
    WatchedAt        time.Time `gorm:"not null;default:now()" json:"watched_at"`
}
```

**Composite indexes** (created via GORM `AfterAutoMigrate` or `CreateIndex`):
- `idx_wh_user_combo` on `(user_id, language, watch_type, player)` — Tier 2 aggregation
- `idx_wh_anime_combo` on `(anime_id, language, watch_type, player)` — Tier 3 aggregation

**When rows are created:** Only when an episode is marked as watched (auto after 20min threshold or manually). Not on every 30s heartbeat. This keeps the table lean.

**`duration_watched` source:** Populated from the latest `watch_progress.progress` value for this user+anime+episode at the time the episode is marked watched. The `markEpisodeWatched` handler looks up the existing `WatchProgress` record to get the current playback position.

### New `user_anime_preferences` Table

Stores the user's last-used combo per anime.

**GORM Model:**

```go
type UserAnimePreference struct {
    ID               string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
    UserID           string    `gorm:"type:uuid;not null;uniqueIndex:idx_user_anime_pref" json:"user_id"`
    AnimeID          string    `gorm:"not null;uniqueIndex:idx_user_anime_pref" json:"anime_id"`
    Player           string    `gorm:"size:20;not null" json:"player"`
    Language         string    `gorm:"size:5;not null" json:"language"`
    WatchType        string    `gorm:"size:5;not null" json:"watch_type"`
    TranslationID    string    `gorm:"size:50" json:"translation_id"`
    TranslationTitle string    `gorm:"size:200" json:"translation_title"`
    UpdatedAt        time.Time `gorm:"not null;default:now()" json:"updated_at"`
}
```

Upserted automatically on every progress heartbeat that includes combo fields (upsert on `user_id + anime_id` conflict). Always reflects the last thing the user actually watched for this anime.

### Extended `UpdateProgressRequest`

Add optional combo fields to the existing progress update (backward compatible):

```go
type UpdateProgressRequest struct {
    AnimeID          string `json:"anime_id"`
    EpisodeNumber    int    `json:"episode_number"`
    Progress         int    `json:"progress"`
    Duration         int    `json:"duration"`
    // Optional combo context — omitted for backward compatibility
    Player           string `json:"player,omitempty"`
    Language         string `json:"language,omitempty"`
    WatchType        string `json:"watch_type,omitempty"`
    TranslationID    string `json:"translation_id,omitempty"`
    TranslationTitle string `json:"translation_title,omitempty"`
}
```

**Validation:** When combo fields are present, `Player` must be one of `{kodik, animelib, hianime, consumet}`, `Language` one of `{ru, en}`, `WatchType` one of `{dub, sub}`. Invalid values return `400 Bad Request` via `errors.BadRequest()`.

### Extended `MarkEpisodeWatchedRequest`

```go
type MarkEpisodeWatchedRequest struct {
    Episode          int    `json:"episode"`
    // Optional combo context
    Player           string `json:"player,omitempty"`
    Language         string `json:"language,omitempty"`
    WatchType        string `json:"watch_type,omitempty"`
    TranslationID    string `json:"translation_id,omitempty"`
    TranslationTitle string `json:"translation_title,omitempty"`
}
```

### Database & Migration Strategy

All services share the same `animeenigma` PostgreSQL database. Tables are auto-created via GORM's `AutoMigrate()` on player service startup.

The existing `WatchHistory` struct (currently `id, user_id, anime_id, episode_number, watched_at`) is empty and unused — no data to migrate. Replace the struct definition entirely; GORM AutoMigrate will add the new columns. Since the table is empty, no destructive migration needed.

New `UserAnimePreference` struct: AutoMigrate creates the table on first startup.

---

## 2. Fallback Resolution Engine

### Where Per-Anime Preference Comes From

The `user_anime_preferences` row is upserted automatically whenever a progress heartbeat includes combo data:
- User opens anime, picks HiAnime HD-1 dub, first 30s heartbeat upserts preference
- User switches translation mid-episode, next heartbeat overwrites
- Always reflects the last thing the user actually watched for this anime

### Fallback Hierarchy (5 Tiers)

```
Tier 1: Per-anime saved preference (user_anime_preferences — last-used combo for THIS anime)
   ↓ not available
Tier 2: User's global favorite (#1 only, exact team match)
   ↓ exact team not available
Tier 3: Community popularity for THIS anime (all users' watch_history)
   ↓ not available
Tier 4: Pinned translation (admin-curated, pinned_translations table)
   ↓ not available
Tier 5: Kodik sub (deterministic default)
```

### Strict Rules

1. **Never cross language boundary.** Language locked from the highest tier that has data.
2. **Never cross dub/sub boundary** if any option of the same type exists.
3. **No fuzzy matching at Tier 2.** User's #1 combo is sacred — if that exact translation team isn't available, skip to Tier 3. Do not check #2, #3, etc.

### Resolution Algorithm

```
resolve(userId, animeId, availableTranslations[]) → WatchCombo | null

1. Per-anime preference (Tier 1)
   → Load from user_anime_preferences WHERE user_id + anime_id
   → If exact combo (player+translation_id) in available, return it. Done.
   → If not exact, check translation_title match in available
     (filtered to same language+type as saved preference).
   → If title match found, return it. Done. (Handles same team, different player.)
   → Lock language+type from this preference regardless.
   → Continue to Tier 2.

2. User's global favorite (Tier 2)
   - Query: SELECT player, language, watch_type, translation_title,
            COUNT(*) as count
     FROM watch_history WHERE user_id = ?
     GROUP BY player, language, watch_type, translation_title
     ORDER BY count DESC
     LIMIT 1

   → Take #1 combo. If no Tier 1 lock, lock language+type from it.
   → Look for EXACT translation_title match in available
     (filtered to locked language+type).
   → If found, return it. Done.
   → If NOT found, go to Tier 3. Do NOT try #2, #3 favorites.

3. Community popularity for this anime (Tier 3)
   - Query: SELECT player, language, watch_type, translation_id,
            translation_title, COUNT(DISTINCT user_id) as viewers
     FROM watch_history WHERE anime_id = ?
     GROUP BY player, language, watch_type, translation_id, translation_title
     ORDER BY viewers DESC

   → Filter to locked language+type.
   → If no lock yet (new user), use the most popular combo to set lock.
   → Return top candidate. If found, done.

4. Pinned translation (Tier 4)
   → The pinned_translations table lives in the catalog service DB, but all services
     share the same animeenigma PostgreSQL database, so player service queries it directly.
   → Pinned translations currently exist only for Kodik, so language is always "ru".
   → Map pinned translation_type: "voice" → "dub", "subtitles" → "sub".
   → Match pinned translation_title against available (filtered to locked language+type).
   → Note: pinned_translations.translation_id is int; match by translation_title, not ID.
   → If match, return it. Done.

5. Default (Tier 5)
   → Return Kodik sub (language: "ru", type: "sub", player: "kodik").
   → If Kodik sub not available, return null (no auto-select).
```

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| User always watches RU Kodik AniLibria dub. New anime has RU Kodik Crunchyroll dub but no AniLibria. | Tier 2 top = AniLibria. Exact title not available → skip to Tier 3. Community picks Crunchyroll → return it. |
| User watches RU dub everywhere. New anime has only EN options. | Tiers 1-2 lock `ru`+`dub`. No RU options at any tier → return null. No auto-select. |
| User prefers RU Kodik AniLibria dub. Anime has RU AnimeLib AniLibria dub (same team, different player). | Tier 1 or 2: exact player+id not found, but `translation_title` = "AniLibria" matches → return it. Team loyalty preserved, player can differ. |
| Brand new user, no history. Anime has pinned translation. | Tiers 1-3 empty. Tier 4 returns pinned (if it matches locked language+type, or if no lock, pinned sets the lock). |
| Brand new user, no history, no pinned. | Tier 5 → Kodik sub. |
| User prefers RU dub. Anime has only RU sub. | Lock `ru`+`dub`. Zero dub options → return null. Does NOT fall to sub. |
| Community split 60/40 on two RU dub translations. User has no preference but Tier 1 locked `ru`+`dub`. | Tier 3 picks the 60% one (filtered to ru+dub). |
| User watches 50 eps HiAnime HD-1 EN dub, 2 eps Consumet EN sub. | Tier 2 top = HiAnime EN dub (count=50). Locks `en`+`dub`. |
| Same translation_title across players (e.g., "Crunchyroll" in Kodik and AnimeLib). | Known trade-off: title matching treats them as the same team. This is intentional — team loyalty matters more than player loyalty. Content may differ slightly between players for the same team. |

### What "return null" Means

Frontend does no auto-selection — shows the full translation list and lets the user pick manually. Better to ask than to silently pick something wrong.

---

## 3. API Design

### Modified Endpoints

**`POST /api/users/progress`** — extended payload

Combo fields ride on the existing 30s heartbeat:

```json
{
  "anime_id": "shiki-123",
  "episode_number": 5,
  "progress": 450,
  "duration": 1420,
  "player": "hianime",
  "language": "en",
  "watch_type": "dub",
  "translation_id": "hd-1",
  "translation_title": "HD-1"
}
```

Backend side effects when combo fields present:
1. Upsert `watch_progress` (existing)
2. Upsert `user_anime_preferences` for this user+anime (new)
3. Increment Prometheus counters (new)

**`POST /api/users/watchlist/{animeId}/episode`** — extended

Creates `watch_history` row with full combo context:

```json
{
  "episode": 5,
  "player": "hianime",
  "language": "en",
  "watch_type": "dub",
  "translation_id": "hd-1",
  "translation_title": "HD-1"
}
```

The handler also looks up `watch_progress` for this user+anime+episode to populate `duration_watched`.

**Error handling for both endpoints:** If combo fields are present but invalid (e.g., `player: "netflix"`), return `400 Bad Request` via `errors.BadRequest("invalid player")`.

### New Endpoints

**`POST /api/users/preferences/resolve`** — resolve best combo

```json
// Request
{
  "anime_id": "shiki-123",
  "available": [
    { "player": "kodik", "language": "ru", "watch_type": "sub", "translation_id": "610", "translation_title": "Crunchyroll" },
    { "player": "kodik", "language": "ru", "watch_type": "dub", "translation_id": "777", "translation_title": "AniLibria" },
    { "player": "hianime", "language": "en", "watch_type": "dub", "translation_id": "hd-1", "translation_title": "HD-1" }
  ]
}

// Response
{
  "resolved": {
    "player": "kodik",
    "language": "ru",
    "watch_type": "dub",
    "translation_id": "777",
    "translation_title": "AniLibria",
    "tier": "user_global",
    "tier_number": 2
  }
}

// When nothing resolves
{ "resolved": null }
```

**Error responses:**
- `400 Bad Request` if `anime_id` is empty or `available` is empty
- `401 Unauthorized` if no valid JWT

**Backend caching:** Resolve results are cached in Redis via `libs/cache` with key `resolve:{userId}:{animeId}` and TTL of 1 hour (`cache.TTLVideoURL`). Cache is invalidated when `user_anime_preferences` is upserted for this user+anime.

**`GET /api/users/preferences/{animeId}`** — per-anime saved preference

```json
{
  "player": "hianime",
  "language": "en",
  "watch_type": "dub",
  "translation_id": "hd-1",
  "translation_title": "HD-1",
  "updated_at": "2026-03-18T14:30:00Z"
}
```

Returns `404 Not Found` if no preference saved.

**`GET /api/users/preferences/global`** — user's aggregated global preferences

```json
{
  "top_combos": [
    { "player": "kodik", "language": "ru", "watch_type": "dub", "translation_title": "AniLibria", "count": 47 },
    { "player": "hianime", "language": "en", "watch_type": "dub", "translation_title": "HD-1", "count": 12 }
  ]
}
```

### Gateway Routing

All new endpoints are under `/api/users/*` — already routed to player:8083. No gateway changes needed.

### Endpoint Summary

| Method | Path | Auth | Purpose |
|--------|------|------|---------|
| `POST` | `/api/users/progress` | JWT | Extended with combo fields |
| `POST` | `/api/users/watchlist/{animeId}/episode` | JWT | Extended — creates watch_history with combo |
| `POST` | `/api/users/preferences/resolve` | JWT | Resolve best combo from available list |
| `GET` | `/api/users/preferences/{animeId}` | JWT | Get per-anime saved preference |
| `GET` | `/api/users/preferences/global` | JWT | Get user's top combos |

---

## 4. Frontend Integration

### Progress Heartbeat — Adding to All Players

Currently only HiAnimePlayer and KodikPlayer have 30s progress heartbeats. **AnimeLibPlayer and ConsumetPlayer must be extended** to add the same 30s heartbeat pattern (save to localStorage + call `updateProgress()` if authenticated).

This is a prerequisite for combo data to flow from all 4 players.

### How Players Report Combo Context

Each player builds a `currentCombo` object from its selected translation state and merges it into the `updateProgress()` payload. Combo is also sent with `markEpisodeWatched()`.

**KodikPlayer.vue:**
```typescript
// selectedTranslation is a numeric ID ref; look up full object from translations array
const selectedTr = translations.value.find(t => t.id === selectedTranslation.value)
const currentCombo = {
  player: 'kodik',
  language: 'ru',
  watch_type: translationType.value === 'voice' ? 'dub' : 'sub',
  translation_id: String(selectedTranslation.value),
  translation_title: selectedTr?.title ?? ''
}
```

**AnimeLibPlayer.vue:**
```typescript
// selectedTranslation is an AnimeLibTranslation object ref
const currentCombo = {
  player: 'animelib',
  language: 'ru',
  watch_type: selectedTranslation.value?.type === 'voice' ? 'dub' : 'sub',
  translation_id: String(selectedTranslation.value?.id ?? ''),
  translation_title: selectedTranslation.value?.team_name ?? ''
}
```

**HiAnimePlayer.vue:**
```typescript
// selectedServer is a HiAnimeServer object ref, selectedCategory is 'sub'|'dub' ref
const currentCombo = {
  player: 'hianime',
  language: 'en',
  watch_type: selectedCategory.value,
  translation_id: selectedServer.value?.id ?? '',
  translation_title: selectedServer.value?.name ?? ''
}
```

**ConsumetPlayer.vue:**
```typescript
// subOrDub type must be passed as a prop from the parent watch page
// (comes from Consumet search result's subOrDub field)
const currentCombo = {
  player: 'consumet',
  language: 'en',
  watch_type: props.subOrDub === 'dub' ? 'dub' : 'sub',
  translation_id: selectedServer.value?.name ?? '',
  translation_title: selectedServer.value?.name ?? ''
}
```

### Preference Resolution on Page Load

New composable: `useWatchPreferences(animeId)`

```typescript
const { resolvedCombo, isLoading } = useWatchPreferences(animeId)
```

Flow:
1. Page mounts
2. All players fetch their available translations (existing behavior)
3. Collect all available translations into unified WatchCombo[] list
4. Check localStorage cache `pref:{animeId}` — if fresh (< 24h), use it instantly for auto-selection
5. Background: `POST /api/users/preferences/resolve` with available list, update cache if result differs
6. If resolved → auto-switch to resolved player tab, pass resolved combo as prop, player auto-selects matching translation
7. If null → no auto-select, show default view (existing behavior)

### Player Auto-Selection via Prop

Each player receives optional `preferredCombo` prop:

```typescript
const props = defineProps<{
  animeId: string
  preferredCombo?: WatchCombo | null
}>()
```

When set and matching this player, auto-selects that translation instead of first available. This replaces the current "auto-select first" logic.

### Caching

- Frontend caches resolve result in `localStorage` keyed by `pref:{animeId}`
- On page load: use cached combo instantly for auto-selection
- Background: call resolve endpoint, update cache if result differs
- Cache TTL: 24 hours (stale cache is fine — worst case user gets last-known preference)
- Backend caches resolve in Redis for 1 hour (invalidated on preference upsert)

### Anonymous Users

No backend calls. Existing localStorage behavior unchanged.

### What Changes in Each Component

| Component | Changes |
|-----------|---------|
| Watch page (parent) | Collects available translations from all players, calls resolve, passes `preferredCombo` + `subOrDub` props down, auto-switches player tab |
| KodikPlayer | Accepts `preferredCombo` prop, auto-selects matching translation. Merges combo into progress heartbeat + markEpisodeWatched. |
| AnimeLibPlayer | Same + **add 30s progress heartbeat** (currently missing) |
| HiAnimePlayer | Same pattern (heartbeat already exists) |
| ConsumetPlayer | Same + **add 30s progress heartbeat** (currently missing) + accept `subOrDub` prop |
| `useWatchPreferences` | New composable — resolve call + localStorage cache |
| `api/client.ts` | Add `resolvePreference()`, `getAnimePreference()`, `getGlobalPreferences()` |

---

## 5. Prometheus Metrics & Grafana Dashboards

### New Prometheus Metrics

All exposed on player service at `/metrics`, registered in `libs/metrics/`.

**Viewing Activity:**
```
watch_episodes_total          counter  [player, language, watch_type]
watch_active_sessions         gauge    []
watch_session_duration_seconds histogram [player, language]
```

Note: `watch_episodes_total` intentionally does NOT include `anime_id` as a label to avoid high-cardinality issues. Per-anime analytics are derived from database queries, not Prometheus.

`watch_active_sessions` is a gauge incremented when a progress heartbeat is received and decremented after no heartbeat for 5 minutes (tracked via in-memory map with TTL).

**Content Preferences:**
```
translation_selections_total  counter  [player, language, watch_type, translation_title]
preference_resolution_total   counter  [tier]
```

**Fallback Analytics:**
```
preference_fallback_total     counter  [tier, language, watch_type]
```

### Grafana Dashboard Panels

**Dashboard 1: Viewing Activity**

| Panel | Type | Query |
|-------|------|-------|
| Active viewers (now) | Stat | `watch_active_sessions` |
| Episodes watched today | Stat | `sum(increase(watch_episodes_total[24h]))` |
| Episodes/hour over time | Time series | `sum(rate(watch_episodes_total[1h]))` |
| Avg session duration | Stat | `histogram_quantile(0.5, watch_session_duration_seconds)` |

**Dashboard 2: Content Preferences**

| Panel | Type | Query |
|-------|------|-------|
| Player split | Pie | `sum by(player)(increase(watch_episodes_total[7d]))` |
| Language split (RU vs EN) | Pie | `sum by(language)(increase(watch_episodes_total[7d]))` |
| Dub vs Sub split | Pie | `sum by(watch_type)(increase(watch_episodes_total[7d]))` |
| Top translations | Table | `topk(20, sum by(translation_title)(increase(translation_selections_total[7d])))` |
| Translation trends | Time series | `sum by(translation_title)(rate(translation_selections_total[1d]))` |

**Dashboard 3: Preference Resolution**

| Panel | Type | Query |
|-------|------|-------|
| Resolution tier distribution | Pie | `sum by(tier)(increase(preference_resolution_total[7d]))` |
| Null resolution rate | Stat | `sum(increase(preference_resolution_total{tier="null"}[24h])) / sum(increase(preference_resolution_total[24h]))` |
| Fallback tier trend | Time series | `sum by(tier)(rate(preference_fallback_total[1h]))` |
| Fallback by language | Bar | `sum by(tier, language)(increase(preference_fallback_total[7d]))` |

---

## 6. Testing Strategy

### Unit Tests — Fallback Resolution Engine

Pure logic tests, no database. Table-driven with real translation names.

**File:** `services/player/internal/service/resolver_test.go`

Real translation data used in tests:
```go
// Kodik (IDs stored as strings)
{"AniLibria", "610", "dub"}, {"AniDUB", "609", "dub"},
{"Crunchyroll", "963", "sub"}, {"SHIZA Project", "616", "dub"}, {"JAM", "971", "dub"}

// HiAnime
{"HD-1", "hd-1", "dub"}, {"HD-2", "hd-2", "sub"}

// AnimeLib
{"AniRise", "1", "dub"}, {"AniLibria", "2", "dub"},
{"Crunchyroll", "3", "sub"}
```

**Group 1: Tier 1 — Per-anime preference**

| Test | Saved Pref | Available | Expected |
|------|-----------|-----------|----------|
| Exact match (player+id) | ru/dub/kodik/610/AniLibria | [kodik/AniLibria, kodik/AniDUB] | kodik/AniLibria |
| Title match cross-player | ru/dub/kodik/610/AniLibria | [animelib/AniLibria, kodik/AniDUB] | animelib/AniLibria (title match) |
| Combo completely gone | ru/dub/kodik/610/AniLibria | [en/dub/hianime/HD-1] | lock ru+dub → Tier 2 |

**Group 2: Tier 2 — User global (#1 only)**

| Test | Top Combo | Available | Expected |
|------|----------|-----------|----------|
| Exact team found | AniLibria ru/dub (47) | [AniLibria, AniDUB] | AniLibria |
| Team not available | AniLibria ru/dub (47) | [AniDUB, SHIZA, JAM] | → Tier 3 |

**Group 3: Tier 3 — Community popularity**

| Test | Community Data | Lock | Available | Expected |
|------|---------------|------|-----------|----------|
| Clear winner | AniDUB (15 users), SHIZA (3) | ru+dub | [AniDUB, SHIZA] | AniDUB |
| Filtered by lock | AniDUB ru/dub (15), HD-1 en/dub (20) | ru+dub | [AniDUB, HD-1] | AniDUB (HD-1 filtered, wrong lang) |
| No data | empty | ru+dub | [AniDUB] | → Tier 4 |
| New user no lock | AniDUB ru/dub (15), HD-1 en/dub (20) | none | [AniDUB, HD-1] | HD-1 (most popular sets lock) |

**Group 4: Tier 4 — Pinned**

| Test | Pinned | Lock | Available | Expected |
|------|--------|------|-----------|----------|
| Matches lock | AniLibria (voice→dub, ru) | ru+dub | [AniLibria, AniDUB] | AniLibria |
| Wrong type | Crunchyroll (subtitles→sub, ru) | ru+dub | [AniDUB, Crunchyroll] | → Tier 5 |
| No pinned | none | ru+dub | [AniDUB] | → Tier 5 |

Note: Pinned translations are currently Kodik-only, so language is always `ru` and matching is done by `translation_title` (not by integer ID).

**Group 5: Tier 5 — Default Kodik sub**

| Test | Available | Expected |
|------|-----------|----------|
| Kodik sub exists | [kodik/Crunchyroll/sub, hianime/HD-1/dub] | kodik/Crunchyroll/sub |
| No Kodik sub | [hianime/HD-1/dub] | null |

**Group 6: Boundary rules**

| Test | Scenario | Expected |
|------|----------|----------|
| Never cross language | Lock ru+dub, only en available | null |
| Never cross type | Lock ru+dub, only ru+sub available | null |
| Lock carries through tiers | Tier 1 ru+dub (gone), Tier 2 top en+sub | Skip Tier 2, keep ru+dub lock |
| Same title across players | "Crunchyroll" in Kodik and AnimeLib available, Tier 2 top = "Crunchyroll" | Returns first match (either player, both valid) |

**Group 7: Input validation**

| Test | Input | Expected |
|------|-------|----------|
| Empty available list | `available: []` | 400 Bad Request |
| Missing anime_id | `anime_id: ""` | 400 Bad Request |
| Invalid combo in progress | `player: "netflix"` | 400 Bad Request |

### Integration Tests

**File:** `services/player/internal/service/resolver_integration_test.go`

Hit real test database (testcontainers):

| Test | Validates |
|------|-----------|
| Progress with combo → preference created | Heartbeat upserts `user_anime_preferences` |
| Mark episode → watch_history with combo | `watch_history` row has all combo fields + duration_watched from watch_progress |
| Resolve with real DB aggregation | Seed 50 rows, verify Tier 2 picks correct top |
| Community resolution across users | Seed 3 users' history, verify Tier 3 aggregation |
| Concurrent preference updates | Two rapid heartbeats don't create duplicates |
| Pinned translation cross-table query | Verify player service can query catalog's pinned_translations |

### Metrics Tests

| Test | Action | Expected |
|------|--------|---------|
| Tier 1 hit | Resolve with per-anime match | `preference_resolution_total{tier="per_anime"}` +1 |
| Null resolution | Resolve with no matches | `preference_resolution_total{tier="null"}` +1 |
| Episode watched | Mark episode with combo | `watch_episodes_total{player="kodik",language="ru",watch_type="sub"}` +1 |
| Active session gauge | Send heartbeat, wait 5min+ | `watch_active_sessions` increments then decrements |
