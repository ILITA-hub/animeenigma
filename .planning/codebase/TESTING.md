# Testing Patterns

**Analysis Date:** 2026-04-27

## Go Testing

### Test Framework & Tools

**Standard library:** `testing` package (Go built-in)

**Assertion library:** `github.com/stretchr/testify` (assert, require)

**HTTP testing:** `net/http/httptest` (built-in)

**Test organization:**
- Test file format: `{module}_test.go` in the same package
- Test function format: `func Test{FunctionName}(t *testing.T)`
- Test packages: Tests live in the same package as the code they test (not a separate `_test` package)

### Running Tests

**Run all tests:**
```bash
go test ./...                    # All packages
go test -v ./...                 # Verbose output
go test -cover ./...             # With coverage
go test -race ./...              # Race condition detection
```

**Run for single service:**
```bash
cd services/catalog
go test ./...
```

**Integration tests (marked with build tags):**
```bash
go test -tags=integration ./...
```

**Run via CI/CD:**
```bash
# From .github/workflows/ci-go.yml
for service in services/*/; do
  cd "$service"
  go test ./... -cover -race
  cd ../..
done
```

### Test Structure Pattern

**Basic unit test with httptest:**

```go
// services/player/internal/service/mal_export_test.go
package service

import (
    "context"
    "encoding/json"
    "net/http"
    "net/http/httptest"
    "testing"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestMALExportService_FetchMALPage(t *testing.T) {
    // Create mock HTTP server
    malServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Path == "/animelist/testuser/load.json" {
            w.Header().Set("Content-Type", "application/json")
            w.WriteHeader(http.StatusOK)
            entries := []MALAnimeEntry{
                {AnimeID: 12345, AnimeTitle: "Attack on Titan", Status: 1, Score: 9},
                {AnimeID: 67890, AnimeTitle: "Naruto", Status: 2, Score: 8},
            }
            json.NewEncoder(w).Encode(entries)
            return
        }
        w.WriteHeader(http.StatusNotFound)
    }))
    defer malServer.Close()

    // Create service with mock client
    service := &MALExportService{
        httpClient:   malServer.Client(),
        schedulerURL: "http://localhost:8085",
    }

    // Test assertions
    assert.NotNil(t, service.httpClient)
}

func TestMALExportService_GetExportStatus_NotFound(t *testing.T) {
    schedulerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.WriteHeader(http.StatusNotFound)
    }))
    defer schedulerServer.Close()

    service := &MALExportService{
        httpClient:   http.DefaultClient,
        schedulerURL: schedulerServer.URL,
    }

    _, err := service.GetExportStatus(context.Background(), "nonexistent-id")
    require.Error(t, err)
    assert.Contains(t, err.Error(), "not found")
}
```

**Database tests with testcontainers (setup pattern):**

```go
// services/player/internal/handler/mal_import_test.go
func setupSyncTestDB(t *testing.T) *gorm.DB {
    // Initialize test database (PostgreSQL container via testcontainers)
    // Returns *gorm.DB
}

func TestMALImportHandler_ImportMALList_Unauthorized(t *testing.T) {
    db := setupSyncTestDB(t)
    syncRepo := repo.NewSyncRepository(db)
    log := logger.Default()
    handler := NewMALImportHandler(nil, syncRepo, log)

    // Create request without auth context
    reqBody := map[string]string{"username": "testuser"}
    body, _ := json.Marshal(reqBody)
    req := httptest.NewRequest("POST", "/api/users/import/mal", bytes.NewReader(body))
    w := httptest.NewRecorder()

    handler.ImportMALList(w, req)

    assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestMALImportHandler_ImportMALList_Success(t *testing.T) {
    db := setupSyncTestDB(t)
    syncRepo := repo.NewSyncRepository(db)
    log := logger.Default()
    handler := NewMALImportHandler(nil, syncRepo, log)

    // Create test data
    activeJob := &domain.SyncJob{
        ID:             "existing-job",
        UserID:         "user-1",
        Source:         "mal",
        SourceUsername: "testuser",
        Status:         "processing",
        Total:          200,
        StartedAt:      time.Now(),
    }
    require.NoError(t, syncRepo.Create(context.Background(), activeJob))

    // Create request with auth context
    reqBody := map[string]string{"username": "testuser"}
    body, _ := json.Marshal(reqBody)
    req := httptest.NewRequest("POST", "/api/users/import/mal", bytes.NewReader(body))

    claims := &authz.Claims{UserID: "user-1"}
    ctx := authz.ContextWithClaims(req.Context(), claims)
    req = req.WithContext(ctx)

    w := httptest.NewRecorder()
    handler.ImportMALList(w, req)

    assert.Equal(t, http.StatusOK, w.Code)

    var response map[string]interface{}
    err := json.Unmarshal(w.Body.Bytes(), &response)
    require.NoError(t, err)
    data := response["data"].(map[string]interface{})
    assert.Equal(t, "existing-job", data["job_id"])
}
```

**External API client tests (mock servers):**

```go
// services/catalog/internal/parser/kodik/client_test.go
func TestGetToken(t *testing.T) {
    client, err := NewClient()
    if err != nil {
        t.Fatalf("failed to create client: %v", err)
    }
    if client.token == "" {
        t.Fatal("token is empty")
    }
    t.Logf("Got token: %s", client.token)
}

func TestSearchByTitle(t *testing.T) {
    client, err := NewClient()
    require.NoError(t, err)

    results, err := client.SearchByTitle("Наруто")
    require.NoError(t, err)
    require.NotEmpty(t, results)

    t.Logf("Found %d results", len(results))
}
```

### Assertion Patterns

**Use `github.com/stretchr/testify/assert` and `require`:**

```go
import (
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

// `require.*` — fail the test immediately (Fatal) if assertion fails
require.NoError(t, err)
require.Equal(t, expected, actual)
require.NotNil(t, value)
require.NotEmpty(t, slice)

// `assert.*` — log failure but continue test
assert.Equal(t, http.StatusOK, w.Code)
assert.Contains(t, response.Message, "error")
assert.Nil(t, err)
```

### Mocking External APIs

**Pattern 1: Mock HTTP server for third-party APIs**

```go
mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
    if r.URL.Path == "/api/anime/1" {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusOK)
        json.NewEncoder(w).Encode(map[string]interface{}{
            "id":   1,
            "name": "Test Anime",
        })
        return
    }
    w.WriteHeader(http.StatusNotFound)
}))
defer mockServer.Close()

// Pass mock URL to client
client := NewShikimoriClient(mockServer.URL)
```

**Pattern 2: Pass test doubles as dependencies**

```go
// Mock repository for service tests
type mockAnimeRepo struct{}

func (m *mockAnimeRepo) GetByID(ctx context.Context, id string) (*Anime, error) {
    return &Anime{ID: id, Name: "Test"}, nil
}

func TestSearchAnime(t *testing.T) {
    mockRepo := &mockAnimeRepo{}
    service := NewCatalogService(mockRepo, nil)
    
    anime, err := service.GetAnime(context.Background(), "1")
    require.NoError(t, err)
    assert.Equal(t, "Test", anime.Name)
}
```

### What to Mock

**DO Mock:**
- External APIs (Shikimori, MAL, Jimaku, etc.)
- HTTP endpoints in other services
- Database for unit tests (use in-memory or testcontainers)
- File I/O and system calls
- Time and randomness (use `time.Now()` judiciously in tests)

**DON'T Mock:**
- Application code you're testing
- Standard library functions
- GORM for integration tests (use real test database)
- Context package

### Test Coverage

**No enforced coverage target** in CI/CD, but aim for >70% on critical paths (domain logic, handlers, service layer).

**Run coverage:**
```bash
go test ./... -cover
go test ./... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Test Organization by Package

**Services with tests:**

| Service | Test Files | Coverage |
|---------|-----------|----------|
| `services/catalog` | `parser/kodik/client_test.go` | External API mocking |
| `services/player` | `service/mal_export_test.go`, `handler/mal_import_test.go`, `repo/sync_test.go` | DB fixtures, API mocks, handlers |
| `services/scheduler` | `service/mal_resolver_test.go`, `jobs/anime_loader_test.go`, `repo/task_test.go` | Job scheduling, DB |
| `services/gateway` | `transport/router_test.go` | Route registration |

**Test file locations:**
- Unit tests for service logic: `services/{name}/internal/service/{module}_test.go`
- Unit tests for handlers: `services/{name}/internal/handler/{module}_test.go`
- Integration tests: `services/{name}/internal/repo/{module}_test.go` (with testcontainers)
- External API tests: `services/{name}/internal/parser/{provider}/client_test.go`

---

## Frontend Testing (Playwright)

### E2E Test Framework

**Framework:** Playwright (`@playwright/test`)

**Configuration:** `frontend/web/playwright.config.ts`

**Test directory:** `frontend/web/e2e/`

**Test file naming:** `{feature}.spec.ts` (e.g., `auth.spec.ts`, `anime.spec.ts`, `hianime-integration.spec.ts`)

### Configuration

**Key settings in `playwright.config.ts`:**

```typescript
export default defineConfig({
  testDir: './e2e',
  fullyParallel: true,
  forbidOnly: !!process.env.CI,  // Fail if .only left in code
  retries: process.env.CI ? 2 : 1,
  workers: process.env.CI ? 1 : undefined,
  reporter: [
    ['html', { open: 'never' }],
    ['list']
  ],
  timeout: 60000,          // 60s per test
  expect: {
    timeout: 10000         // 10s per assertion
  },
  use: {
    baseURL: process.env.BASE_URL || 'http://localhost:3003',
    trace: 'on-first-retry',
    screenshot: 'only-on-failure',
    video: 'retain-on-failure',
    actionTimeout: 15000,
    navigationTimeout: 30000,
  },
  projects: [
    { name: 'chromium', use: { ...devices['Desktop Chrome'] } },
    { name: 'firefox', use: { ...devices['Desktop Firefox'] } },
    { name: 'Mobile Chrome', use: { ...devices['Pixel 5'] } },
  ],
  webServer: process.env.BASE_URL ? undefined : {
    command: 'npm run dev',
    url: 'http://localhost:5173',
    reuseExistingServer: !process.env.CI,
    timeout: 120000,
  },
})
```

### Running Tests

**Via bun:**
```bash
# Run all e2e tests
bun run test:e2e

# Run with UI
bun run test:e2e:ui

# Run headless with browser window
bun run test:e2e:headed

# Via Playwright CLI directly (bunx)
bunx playwright test
bunx playwright test auth.spec.ts                # Single file
bunx playwright test hianime-integration         # Match pattern
bunx playwright test --reporter=list             # List reporter
```

**From CI/CD:** Tests run in CI on all three projects (Chromium, Firefox, Mobile) with retry logic.

### Test Structure Pattern

**Basic authentication test:**

```typescript
// frontend/web/e2e/auth.spec.ts
import { test, expect } from '@playwright/test'

test.describe('Authentication', () => {
  test.beforeEach(async ({ page }) => {
    // Clear existing auth state
    await page.goto('/')
    await page.evaluate(() => {
      localStorage.removeItem('token')
      localStorage.removeItem('user')
    })
  })

  test.describe('Login Page', () => {
    test('should display login form', async ({ page }) => {
      await page.goto('/auth')

      await expect(page.getByPlaceholder('username')).toBeVisible()
      await expect(page.getByPlaceholder('••••••••').first()).toBeVisible()
      await expect(page.getByRole('button', { name: 'Войти' })).toBeVisible()
    })

    test('should show error for invalid credentials', async ({ page }) => {
      await page.goto('/auth')

      await page.getByPlaceholder('username').fill('invaliduser')
      await page.getByPlaceholder('••••••••').fill('wrongpassword')
      await page.getByRole('button', { name: 'Войти' }).click()

      await expect(page.locator('.text-pink-400')).toBeVisible({ timeout: 5000 })
    })
  })

  test.describe('Registration', () => {
    test('should register new user successfully', async ({ page }) => {
      await page.goto('/auth')
      await page.getByRole('button', { name: 'Регистрация' }).click()

      const uniqueUsername = `testuser_${Date.now()}`
      await page.getByPlaceholder('username (3-32 символа)').fill(uniqueUsername)
      await page.getByPlaceholder('Минимум 6 символов').fill('password123')
      await page.getByPlaceholder('••••••••').nth(1).fill('password123')

      // Assert success
      await expect(page).toHaveURL('/profile')
    })
  })
})
```

### Test Patterns

**Selectors (prefer accessible patterns):**

```typescript
// Role-based (preferred)
page.getByRole('button', { name: 'Sign in' })
page.getByRole('heading', { level: 1 })

// Label
page.getByLabel('Username')

// Placeholder
page.getByPlaceholder('Enter your name')

// Text
page.getByText('Hello, World')

// Test ID (last resort)
page.getByTestId('anime-card')
```

**Waits and assertions:**

```typescript
// Wait for visibility
await expect(page.locator('.loading')).toBeVisible({ timeout: 5000 })
await expect(page.locator('.error')).toBeHidden()

// Wait for navigation
await page.goto('/anime/1')
await page.waitForNavigation()
await page.click('a[href="/anime/2"]')

// Text assertions
await expect(page.locator('h1')).toContainText('My Anime')

// Count assertions
await expect(page.locator('.anime-card')).toHaveCount(20)

// URL assertions
await expect(page).toHaveURL('/profile')
```

**Setup/teardown:**

```typescript
test.beforeEach(async ({ page }) => {
  // Run before each test
  await page.goto('/')
  await loginAsTestUser(page)
})

test.afterEach(async ({ page }) => {
  // Run after each test
  await page.evaluate(() => localStorage.clear())
})

test.describe.skip('Disabled test suite', () => {
  // Skipped — use for wip tests
})

test.skip('Disabled test', async ({ page }) => {
  // Skipped
})
```

### Authentication in Tests

**Login helper:**

```typescript
async function loginAsTestUser(page: Page) {
  await page.goto('/auth')
  await page.getByPlaceholder('username').fill('ui_audit_bot')
  await page.getByPlaceholder('••••••••').fill('audit_bot_test_password_2026')
  await page.getByRole('button', { name: 'Войти' }).click()
  await page.waitForURL('/profile')
}
```

**With API key (backend testing):**

```typescript
// Via REST API in the browser context
const context = await browser.newContext({
  extraHTTPHeaders: {
    'Authorization': 'Bearer ak_' + apiKey,
  },
})
```

### Test Organization

**Existing test files by feature:**

| Test File | Scope | Purpose |
|-----------|-------|---------|
| `auth.spec.ts` | Login/register UI | Authentication forms, error states |
| `anime.spec.ts` | Anime detail/browse | Search, filtering, anime cards |
| `game.spec.ts` | Game room feature | Socket.io, real-time updates |
| `accessibility.spec.ts` | a11y compliance | axe-core scans, ARIA |
| `hianime-integration.spec.ts` | Video streaming | HiAnime player, HLS streams |
| `consumet-integration.spec.ts` | Video streaming | Consumet player, HLS streams |
| `hianime-player.spec.ts` | Video player UI | Playback, subtitle controls, fullscreen |
| `frieren-s1-test.spec.ts` | Known anime | Complete workflow test (known-good anime) |

### UI Audit Framework

**For comprehensive UI/UX audits, use the framework in CLAUDE.md:**

**Methodology:**
1. **Static heuristic review** — Nielsen's 10 heuristics + screenshot review
2. **Automated a11y scan** — axe-core via CDN + Playwright injection
3. **Per-view interaction probe** — Tab×5, Esc, scroll, resize, back-nav
4. **Realistic user scenarios** — 4-6 workflows (search → watch, list management, etc.)
5. **Cross-view consistency** — button styles, modals, focus rings across pages

**Output:** `docs/issues/ui-audit-YYYY-MM-DD.md` with:
- Summary (counts, weighted scores)
- Findings by severity (catastrophic → minor)
- Per-view findings
- Scenario findings
- axe-core results
- Cross-view inconsistencies

**Test user for audits:**
- Username: `ui_audit_bot`
- Password: `audit_bot_test_password_2026` (set 2026-04-07)
- API Key: `docker/.env` as `UI_AUDIT_API_KEY`
- Seeded data: 8 anime_list entries, 3 watch_history, 3 theme_ratings
- **Permanent account** — do not recreate per session; re-seed state via `scripts/seed-ui-audit-user.sh` if needed

**Playwright UI audit tools:**
```typescript
// Import axe-core and run scans
import { injectAxe, checkA11y } from 'axe-playwright'

test('audit accessibility', async ({ page }) => {
  await page.goto('/anime/1')
  await injectAxe(page)
  await checkA11y(page)
})

// Or manually load axe and run
test('manual axe scan', async ({ page }) => {
  await page.goto('/anime/1')
  await page.addScriptTag({
    url: 'https://cdnjs.cloudflare.com/ajax/libs/axe-core/4.10.2/axe.min.js'
  })
  const results = await page.evaluate(() => (window as any).axe.run())
  console.log(results)
})
```

### Integration Test Examples

**HiAnime streaming integration:**

```typescript
// frontend/web/e2e/hianime-integration.spec.ts
test('should load HiAnime player with HLS stream', async ({ page }) => {
  await loginAsTestUser(page)
  await page.goto('/anime/57466')  // Known anime with HiAnime source

  // Click watch button
  await page.click('[data-testid="watch-button"]')
  await page.waitForURL(/\/watch/)

  // Wait for player to load
  await expect(page.locator('video')).toBeVisible({ timeout: 10000 })

  // Verify HLS.js or Video.js initialized
  const playerType = await page.locator('[data-player-type]').getAttribute('data-player-type')
  expect(['hls', 'video.js']).toContain(playerType)
})

test('should load subtitles from Jimaku.cc', async ({ page }) => {
  await loginAsTestUser(page)
  await page.goto('/watch/57466/1')  // Episode 1

  // Subtitle menu visible
  await expect(page.locator('[aria-label="Subtitles"]')).toBeVisible()

  // Click subtitle button
  await page.click('[aria-label="Subtitles"]')

  // Verify Japanese subtitles option
  await expect(page.locator('text=Japanese')).toBeVisible()
})
```

---

## CI/CD Testing

### GitHub Actions Workflows

**Go CI (.github/workflows/ci-go.yml):**

```yaml
jobs:
  lint:
    # golangci-lint on services/ and libs/
  
  test:
    # Run go test ./... on each service with PostgreSQL + Redis
    # Environment variables set for test DB
  
  build:
    # Build binaries for all services
    # Matrix strategy across services
```

**Frontend CI (.github/workflows/ci-frontend.yml):**

```yaml
jobs:
  lint:
    # eslint + type-check (bun lint, bun run type-check)
  
  test:
    # bun run test:unit (not yet implemented)
  
  build:
    # bun run build
```

**Playwright runs on deployment/manual trigger (setup needed if expanded).**

### Test Database Setup

**In CI, PostgreSQL and Redis are provisioned as services:**

```yaml
services:
  postgres:
    image: postgres:16-alpine
    env:
      POSTGRES_USER: postgres
      POSTGRES_PASSWORD: postgres
      POSTGRES_DB: test
    ports: ['5432:5432']
    options: >-
      --health-cmd pg_isready
      --health-interval 10s
      --health-timeout 5s
      --health-retries 5

  redis:
    image: redis:7-alpine
    ports: ['6379:6379']
    options: >-
      --health-cmd "redis-cli ping"
      --health-interval 10s
      --health-timeout 5s
      --health-retries 5
```

**Tests set env vars to point to services:**

```bash
DB_HOST=localhost DB_PORT=5432 DB_USER=postgres DB_PASSWORD=postgres \
REDIS_HOST=localhost REDIS_PORT=6379 \
go test ./...
```

---

## Test Maintenance Checklist

- [ ] All new logic is covered by unit tests
- [ ] External API calls are mocked in unit tests
- [ ] Integration tests use testcontainers or real test services
- [ ] Test names describe what is being tested (`Test{Function}_{Scenario}_{Expected}`)
- [ ] Tests clean up resources (defer mock server Close, etc.)
- [ ] No hardcoded timeouts; use `require.Eventually` or Playwright waits
- [ ] Assertions use `require.*` for critical assertions, `assert.*` for secondary ones
- [ ] Frontend tests use accessible selectors (getByRole, getByLabel) before test IDs
- [ ] Auth context is properly mocked for handler tests
- [ ] Coverage is tracked (aim >70% on critical paths)

---

*Testing analysis: 2026-04-27*
