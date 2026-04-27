# Coding Conventions

**Analysis Date:** 2026-04-27

## Go Conventions

### File Naming

- **snake_case** for all Go files
- Examples: `anime_parser.go`, `mal_export.go`, `list_repository.go`, `http_util.go`
- Test files: `{module}_test.go` (e.g., `mal_export_test.go`)
- No underscores between words and file suffixes (not `mal_export_handler.go`, but split into separate concerns)

### Type Naming

- **Exported types:** PascalCase (e.g., `CatalogService`, `AnimeRepository`, `SearchFilter`)
- **Unexported types:** camelCase (e.g., `hianimeInflight`)
- **Interfaces:** PascalCase with `-er` suffix or concrete name (e.g., `AnimeParser`, `Repository`)
- **Constants:** PascalCase (e.g., `StatusOngoing`, `TTLAnimeDetails`) or ALL_CAPS for env vars (e.g., `JWT_SECRET`)

### Variable & Function Naming

- **Exported functions:** PascalCase (e.g., `NewCatalogService()`, `SearchAnime()`)
- **Unexported functions:** camelCase (e.g., `sanitizedOrderClause()`, `isTitleSort()`)
- **Variables:** camelCase (e.g., `animeID`, `videoURL`, `catalogService`, `refreshPromise`)
- **Receivers:** single letter if no ambiguity; explicit `r` for repositories, `s` for services, `h` for handlers

**Example:**
```go
func (r *ListRepository) GetByUser(ctx context.Context, userID string) ([]*domain.AnimeListEntry, error) {
    var entries []*domain.AnimeListEntry
    // Implementation
}

func (s *CatalogService) SearchAnime(r *http.Request) {
    // Implementation
}
```

### Package Layout

**Standard service directory structure:**
```
services/{name}/
├── cmd/{name}-api/
│   └── main.go              # Entry point, initialization
├── internal/
│   ├── config/              # Environment configuration
│   │   └── config.go        # Config struct and loader
│   ├── domain/              # Domain models & interfaces
│   │   └── {model}.go       # Entity definitions (Anime, User, etc.)
│   ├── handler/             # HTTP request handlers
│   │   ├── {feature}.go     # Handler structs and methods
│   │   └── {feature}_test.go
│   ├── service/             # Business logic
│   │   ├── {feature}.go     # Service structs and methods
│   │   └── {feature}_test.go
│   ├── repo/                # Database repositories (GORM)
│   │   ├── {entity}.go      # Repository structs and methods
│   │   └── {entity}_test.go
│   ├── parser/              # External API clients (Shikimori, Kodik, etc.)
│   │   └── {provider}/
│   │       ├── client.go    # API client implementation
│   │       └── client_test.go
│   └── transport/           # Router & HTTP middleware setup
│       └── router.go        # Chi router initialization
├── migrations/              # SQL schema migrations (if used)
├── Dockerfile
└── go.mod
```

**Example locations:**
- Catalog anime search logic: `services/catalog/internal/service/catalog.go` (`SearchAnime()` method)
- Anime domain model: `services/catalog/internal/domain/anime.go`
- Database queries: `services/catalog/internal/repo/` (not yet found but follows GORM convention)
- Video stream clients: `services/catalog/internal/parser/{kodik,hianime,consumet}/`
- HTTP routes: `services/catalog/internal/transport/router.go`

### Error Handling

**Use the shared `libs/errors` package:**

```go
import "github.com/ILITA-hub/animeenigma/libs/errors"
```

**Return domain errors with context:**

```go
// NotFound error for missing resources
if anime == nil {
    return nil, errors.NotFound("anime not found on kodik")
}

// Wrap external/system errors with code and message
if err != nil {
    return nil, errors.Wrap(err, errors.CodeInternal, "failed to fetch jimaku subtitles")
}

// Specific error codes
return nil, errors.Wrap(err, errors.CodeExternalAPI, "fetch MAL list")
return nil, errors.Wrap(err, errors.CodeInternal, "decode response")
```

**Error codes used in codebase:**
- `errors.CodeInternal` — application/database errors
- `errors.CodeExternalAPI` — third-party API failures
- `errors.CodeBadRequest` — client input validation (implicit, via httputil.BadRequest)

**HTTP handler error pattern:**
```go
animes, total, err := h.catalogService.SearchAnime(r.Context(), filters)
if err != nil {
    httputil.Error(w, err)  // Automatically sets status code based on error type
    return
}
```

### Logging

**Use structured logging via `libs/logger`:**

```go
import "github.com/ILITA-hub/animeenigma/libs/logger"

type CatalogService struct {
    log *logger.Logger
}

// Log with key-value pairs (structured)
s.log.Infow("fetching anime from shikimori",
    "query", query,
    "page", page,
)

s.log.Errorw("failed to proxy video stream",
    "anime_id", animeID,
    "source", "kodik",
    "error", err,
)

// Initialization warnings/info
log.Infow("jimaku client initialized")
log.Warnw("failed to initialize kodik client, kodik features will be unavailable", "error", err)
```

**Do not use `fmt.Printf`, `log.Println`, or bare `fmt.Errorf`.** All service logging goes through `libs/logger`.

### Caching

**Use the shared `libs/cache` with appropriate TTL constants:**

```go
import "github.com/ILITA-hub/animeenigma/libs/cache"

// Cache anime details for 6 hours
if err := s.cache.Get(ctx, cacheKey, &anime); err == nil {
    return anime, nil  // Cache hit
}

_ = s.cache.Set(ctx, cacheKey, dbAnime, cache.TTLAnimeDetails)

// Search results cache 15 minutes
_ = s.cache.Set(ctx, searchCacheKey, struct {
    Animes []*Anime
    Total  int64
}{Animes: animes, Total: total}, cache.TTLSearchResults)

// Top anime cache
_ = s.cache.Set(ctx, cache.KeyTopAnime(), shikimoriAnimes, cache.TTLTopAnime)

// External video URLs expire quickly (1 hour)
_ = s.cache.Set(ctx, "video:"+animeID, videoURL, time.Hour)
```

**Common TTL constants:**
- `cache.TTLAnimeDetails` — 6 hours (anime metadata)
- `cache.TTLSearchResults` — 15 minutes (search queries)
- `cache.TTLTopAnime` — (varies, check libs/cache)
- `cache.TTLGenreList` — (varies, check libs/cache)
- `time.Hour` — 1 hour (video URLs that expire)

### Database & GORM

**Primary Keys:**

```go
type Anime struct {
    ID string `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
}
```

**Timestamps and soft deletes:**

```go
type Anime struct {
    // ... fields ...
    CreatedAt time.Time      `json:"created_at"`
    UpdatedAt time.Time      `json:"updated_at"`
    DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`  // Soft delete
}
```

**Database initialization in main.go:**

```go
// Automatically create DB if not exists
db, err := database.New(cfg.Database)

// Auto-migrate tables
db.AutoMigrate(&domain.Anime{})
db.AutoMigrate(&domain.AnimeListEntry{})
```

**Query patterns:**

```go
// Use GORM query methods for most operations
return r.db.WithContext(ctx).Where("user_id = ?", userID).Find(&entries).Error

// For complex queries or performance-critical operations, use raw SQL
return r.db.WithContext(ctx).Raw(`
    SELECT * FROM anime_list
    WHERE user_id = ? AND status = ?
    ORDER BY ? ?
`, userID, status, sort, order).Scan(&entries).Error

// Upserting with conflict resolution
return r.db.WithContext(ctx).Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "user_id"}, {Name: "anime_id"}},
    DoUpdates: clause.Assignments(map[string]interface{}{
        "status":     entry.Status,
        "score":      entry.Score,
        "episodes":   entry.Episodes,
        "updated_at": entry.UpdatedAt,
    }),
}).Create(entry).Error
```

### Configuration

**Load from environment variables:**

```go
package config

type Config struct {
    Database struct {
        Host     string `env:"DB_HOST,required"`
        Port     int    `env:"DB_PORT,required"`
        User     string `env:"DB_USER,required"`
        Password string `env:"DB_PASSWORD,required"`
        Name     string `env:"DB_NAME,required"`
    }
    Redis struct {
        Host string `env:"REDIS_HOST,required"`
        Port int    `env:"REDIS_PORT,required"`
    }
    JWT struct {
        Secret string `env:"JWT_SECRET,required"`
    }
}

func Load() (*Config, error) {
    // Implementation uses env package to load from environment
}
```

**In services (example from catalog):**

```go
opts := CatalogServiceOptions{
    AniwatchAPIURL:   opts[0].AniwatchAPIURL,
    ConsumetAPIKey:   opts[0].ConsumetAPIKey,
    JimakuAPIKey:     opts[0].JimakuAPIKey,
}
```

### HTTP Handler Pattern

**Handlers are methods on structs (receivers):**

```go
type CatalogHandler struct {
    catalogService *service.CatalogService
    log            *logger.Logger
}

func NewCatalogHandler(catalogService *service.CatalogService, log *logger.Logger) *CatalogHandler {
    return &CatalogHandler{
        catalogService: catalogService,
        log:            log,
    }
}

// Handler method signature
func (h *CatalogHandler) SearchAnime(w http.ResponseWriter, r *http.Request) {
    query := r.URL.Query().Get("q")
    if query == "" || len(query) < 2 {
        httputil.BadRequest(w, "search query must be at least 2 characters")
        return
    }

    animes, total, err := h.catalogService.SearchAnime(r.Context(), filters)
    if err != nil {
        httputil.Error(w, err)
        return
    }

    meta := httputil.Meta{
        Page:       filters.Page,
        PageSize:   filters.PageSize,
        TotalCount: total,
        TotalPages: int((total + int64(filters.PageSize) - 1) / int64(filters.PageSize)),
    }

    httputil.JSONWithMeta(w, http.StatusOK, animes, meta)
}
```

**Router setup in transport/router.go:**

```go
func NewRouter(
    catalogHandler *handler.CatalogHandler,
    adminHandler *handler.AdminHandler,
    cfg *config.Config,
    log *logger.Logger,
    metricsCollector *metrics.Collector,
) http.Handler {
    r := chi.NewRouter()

    // Middleware
    r.Use(middleware.RequestID)
    r.Use(metricsCollector.Middleware)
    r.Use(httputil.RequestLogger(log))
    r.Use(httputil.Recoverer(log))

    // Routes
    r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
        httputil.OK(w, map[string]string{"status": "ok"})
    })

    r.Route("/api/anime", func(r chi.Router) {
        r.Get("/", catalogHandler.BrowseAnime)
        r.Get("/search", catalogHandler.SearchAnime)
        r.Get("/{animeId}", catalogHandler.GetAnime)
    })

    return r
}
```

### Linting

**Configuration: `.golangci.yml`**

- **Enabled linters:** errcheck, gosimple, govet, ineffassign, staticcheck, unused
- **Timeout:** 5 minutes
- **Disabled linters:** typecheck (handled by `go build`)
- **Error checks in tests:** Relaxed (errcheck disabled for `*_test.go` files)

**Run linting:**
```bash
golangci-lint run ./...
```

---

## Frontend (Vue 3) Conventions

### Component Structure

**Use `<script setup>` with TypeScript:**

```vue
<template>
  <div class="glass-card rounded-2xl p-5">
    <h2 class="text-xl font-bold">{{ title }}</h2>
    <button @click="handleClick" class="px-4 py-2 rounded">
      {{ label }}
    </button>
  </div>
</template>

<script setup lang="ts">
import { ref, computed } from 'vue'

interface Props {
  title: string
  label?: string
}

const props = withDefaults(defineProps<Props>(), {
  label: 'Click me'
})

const emit = defineEmits<{
  click: [value: string]
}>()

const isActive = ref(false)

const computed_value = computed(() => {
  return isActive.value ? 'active' : 'inactive'
})

const handleClick = () => {
  emit('click', computed_value.value)
}
</script>

<style scoped>
/* Scoped styles */
</style>
```

### File Naming

- **Components:** PascalCase (e.g., `AnimeCardNew.vue`, `LastUpdates.vue`, `HiAnimePlayer.vue`)
- **Utility files:** camelCase (e.g., `subtitle-parser.ts`, `diagnostics.ts`)
- **Stores:** camelCase (e.g., `auth.ts`, `watchlist.ts`)

**File organization:**
```
src/
├── components/          # Vue components (PascalCase)
│   ├── anime/
│   │   └── AnimeCardNew.vue
│   ├── player/
│   │   ├── HiAnimePlayer.vue
│   │   ├── ConsumetPlayer.vue
│   │   └── SubtitleOverlay.vue
│   ├── layout/
│   │   └── Navbar.vue
│   └── ui/
│       ├── Modal.vue
│       └── Badge.vue
├── views/               # Page components (PascalCase)
│   ├── Home.vue
│   ├── Watch.vue
│   └── Profile.vue
├── stores/              # Pinia stores (camelCase)
│   ├── auth.ts
│   ├── home.ts
│   └── watchlist.ts
├── api/                 # API clients (camelCase)
│   └── client.ts
└── utils/               # Utilities (camelCase)
    ├── diagnostics.ts
    └── subtitle-parser.ts
```

### Pinia Store Pattern

**Define stores with composition API:**

```typescript
import { defineStore } from 'pinia'
import { ref, computed } from 'vue'
import { apiClient } from '@/api/client'
import i18n from '@/i18n'

export interface User {
  id: string
  username: string
  email: string
  role: string
}

export const useAuthStore = defineStore('auth', () => {
  const user = ref<User | null>(loadUserFromStorage())
  const token = ref<string | null>(localStorage.getItem('token'))
  const loading = ref(false)
  const error = ref<string | null>(null)

  const isAuthenticated = computed(() => !!token.value)
  const isAdmin = computed(() => user.value?.role === 'admin')

  const setUser = (userData: User | null) => {
    user.value = userData
    if (userData) {
      localStorage.setItem('user', JSON.stringify(userData))
    } else {
      localStorage.removeItem('user')
    }
  }

  const setToken = (accessToken: string) => {
    token.value = accessToken
    localStorage.setItem('token', accessToken)
  }

  const login = async (credentials: LoginCredentials) => {
    loading.value = true
    error.value = null

    try {
      const response = await apiClient.post('/auth/login', credentials)
      const data = response.data?.data || response.data
      setToken(data.access_token)
      setUser(data.user)
      return true
    } catch (err: unknown) {
      const e = err as AxiosError
      error.value = e.response?.data?.error?.message || i18n.global.t('auth.loginError')
      return false
    } finally {
      loading.value = false
    }
  }

  return {
    user,
    token,
    loading,
    error,
    isAuthenticated,
    isAdmin,
    setUser,
    setToken,
    login,
  }
})
```

### API Client Pattern

**Centralized axios instance with interceptors:**

```typescript
// src/api/client.ts
import axios, { AxiosInstance, InternalAxiosRequestConfig } from 'axios'

const BASE_URL = import.meta.env.VITE_API_URL || '/api'

export const apiClient: AxiosInstance = axios.create({
  baseURL: BASE_URL,
  timeout: 30000,
  headers: { 'Content-Type': 'application/json' },
  withCredentials: true, // Send cookies (refresh token)
})

// Request interceptor — auto-refresh expired tokens
apiClient.interceptors.request.use(async (config: InternalAxiosRequestConfig) => {
  if (config.url?.includes('/auth/refresh') || config.url?.includes('/auth/login')) {
    return config
  }

  let token = localStorage.getItem('token')
  if (token && isTokenExpired(token)) {
    const newToken = await doTokenRefresh()
    token = newToken
  }
  if (token && config.headers) {
    config.headers.Authorization = `Bearer ${token}`
  }
  return config
})

// Response interceptor — handle 401/token expiry
apiClient.interceptors.response.use(
  async (response) => {
    // Handle token refresh on optional-auth endpoints
    if (response.headers['x-token-expired']) {
      // Refresh logic
    }
    return response
  },
  (error) => {
    if (error.response?.status === 401) {
      // Redirect to login
    }
    return Promise.reject(error)
  }
)
```

### Vue Template Rules

**CRITICAL: v-if/v-else-if chains**

Never place non-conditional elements between `v-if` and `v-else-if` branches:

```vue
<!-- ❌ WRONG — breaks the chain -->
<template>
  <div v-if="loading">Loading...</div>
  <p>This breaks the chain!</p>
  <div v-else-if="error">Error occurred</div>
</template>

<!-- ✅ CORRECT — keep chain intact, place independent elements after -->
<template>
  <div v-if="loading">Loading...</div>
  <div v-else-if="error">Error occurred</div>
  <div v-else>Content</div>
  <!-- Independent elements here -->
  <p>This is outside the chain</p>
</template>
```

**Conditional rendering patterns:**

```vue
<!-- Avoid nested conditions when possible -->
<div v-if="anime">
  <div v-if="anime.score > 7">High rated</div>
</div>

<!-- Better: use computed properties -->
<div v-if="isHighRated">High rated</div>
```

### ESLint & TypeScript

**Configuration: `frontend/web/.eslintrc.cjs`**

- **Parser:** vue-eslint-parser with @typescript-eslint/parser
- **Extends:** vue/vue3-essential, eslint:recommended, @typescript-eslint/recommended
- **Disabled rules:**
  - `vue/multi-word-component-names` (off — single-word components allowed like `Badge`)
- **Enforced rules:**
  - `@typescript-eslint/no-explicit-any` (warn)
  - `@typescript-eslint/no-unused-vars` (error, ignores names starting with `_`)

**Run linting:**
```bash
# Via bun (NOT npm/pnpm)
bun run lint              # Check
bun run lint:fix          # Auto-fix
bun run type-check        # TypeScript check
```

**No Prettier config detected.** Formatting is controlled by ESLint rules.

### TypeScript Type Conventions

```typescript
// Interface for component props
export interface AnimeCardProps {
  anime: Anime
  isHighlighted?: boolean
}

// Type for API responses
interface APIResponse<T> {
  data: T
  error?: ErrorInfo
  meta?: Metadata
}

// Enum for status values
enum AnimeStatus {
  Ongoing = 'ongoing',
  Released = 'released',
  Announced = 'announced',
}

// Use `unknown` before narrowing, avoid `any`
const handleError = (err: unknown) => {
  if (err instanceof Error) {
    console.error(err.message)
  }
}
```

---

## Commit Message Style

**Format:** `<type>(<scope>): <description>`

**Types:**
- `feat` — new feature
- `fix` — bug fix
- `refactor` — code restructuring
- `chore` — maintenance, dependencies, config
- `docs` — documentation changes
- `style` — formatting, no logic change
- `perf` — performance improvement
- `test` — test additions/fixes

**Scope:** affected area (e.g., `ui`, `auth`, `catalog`, `player`, `home`)

**Description:** concise, lowercase, no period; use imperative mood ("add" not "adds")

**Examples from codebase:**
```
feat(ui): revamp anime card context menu — kebab, keyboard nav, hero cards
fix(ui): opaque dark bg on navbar + Home/Browse search dropdowns
refactor(search): extract shared SearchAutocomplete + fix Home overlap
chore(ops): maintenance-service state checkpoint
feat(player): JP subtitle timing offset with per-team persistence
docs(ui-audit): mobile re-audit 2026-04-20 (UA-042..056)
```

**Multi-line body:** Include details about WHY and IMPACT if needed.

---

## Frontend Tooling: Bun, NOT npm/pnpm

**All frontend package management uses `bun`:**

```bash
# Install dependencies
bun install

# Run dev server
bun run dev

# Build
bun run build

# Lint
bun run lint
bun run lint:fix

# Type-check
bun run type-check

# Run tests
bun run test:unit        # Unit tests (not yet implemented)
bun run test:e2e         # Playwright e2e tests

# For Playwright CLI
bunx playwright test
bunx playwright test --ui
bunx playwright test --headed

# For other CLI tools
bunx eslint src/
bunx tsc --noEmit
```

**Never use `npm run`, `pnpm run`, or `npx` — always use `bun` and `bunx`.**

---

## Cross-Language Patterns

### Async/Await & Promises (Frontend)

```typescript
// Handle promise chains with proper error boundaries
try {
  const response = await apiClient.get('/api/anime/search')
  const anime = response.data?.data || response.data
  return anime
} catch (err: unknown) {
  const error = err as AxiosError
  console.error('Failed to search anime:', error.message)
  throw error
}

// For concurrent requests, use Promise.all
const [animes, genres] = await Promise.all([
  apiClient.get('/api/anime'),
  apiClient.get('/api/genres'),
])
```

### Context Usage (Go)

- **Always pass `ctx` as first parameter** to functions that interact with external services or databases
- **Use `context.WithValue` sparingly** — prefer explicit parameters
- **Always check context cancellation** in loops and long-running operations

```go
func (s *CatalogService) SearchAnime(ctx context.Context, query string) ([]*Anime, error) {
    // Check if context is already cancelled
    select {
    case <-ctx.Done():
        return nil, ctx.Err()
    default:
    }

    // Pass context to DB queries
    return s.repo.SearchByQuery(ctx, query)
}
```

---

*Conventions analysis: 2026-04-27*
