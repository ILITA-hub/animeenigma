# Phase 19: AnimeKai (gated) — Pattern Map

**Mapped:** 2026-05-12
**Path chosen:** ESCAPE HATCH (per RESEARCH.md §Recommendation, ~3-4 days work)
**Files analyzed:** 8 (5 new, 3 modified)
**Analogs found:** 8 / 8 (100% — every file maps to an existing analog)

The escape hatch ships the **scaffold** of a full AnimeKai provider whose every Provider-interface method returns `domain.ErrProviderDown` so the orchestrator immediately falls through to gogoanime. The intent is: a future v3.1 PR fills in real method bodies and the orchestrator integration test goes from RED-stub to GREEN-real with zero wiring changes elsewhere.

## File Classification

| New/Modified File | Role | Data Flow | Closest Analog | Match Quality |
|-------------------|------|-----------|----------------|---------------|
| `services/scraper/internal/providers/animekai/doc.go` | provider docstring | static | `services/scraper/internal/providers/gogoanime/doc.go` | exact |
| `services/scraper/internal/providers/animekai/client.go` | provider impl (stub returning ErrProviderDown) | request-response | `services/scraper/internal/providers/gogoanime/client.go` | exact (skeleton only — body replaced with stub) |
| `services/scraper/internal/providers/animekai/dto.go` | DTO definitions (stub — minimal/empty) | data-shape | `services/scraper/internal/providers/gogoanime/dto.go` | exact |
| `services/scraper/internal/providers/animekai/client_test.go` | unit tests verifying stub returns ErrProviderDown | test | `services/scraper/internal/providers/gogoanime/client_test.go` | role-match (test must be SHALLOWER than gogo's because there's nothing live to test) |
| `services/scraper/internal/config/config.go` *(modify)* | config struct + env binding | config | existing `GogoanimeConfig` in same file | exact |
| `services/scraper/cmd/scraper-api/main.go` *(modify)* | conditional provider registration | wiring | existing gogoanime registration block (lines 127-154) | exact |
| `docker/megacloud-extractor/server.js` *(modify)* | HTTP 501 stub route `POST /animekai-token` | request-response | existing `/extract` and `/health` routes (lines 210-243) | role-match |
| `docker/docker-compose.yml` + `docker/.env.example` *(modify)* | env var documentation | config | existing `ANIMEPAHE_BASE_URL` + `SCRAPER_GOGOANIME_BASE_URL` blocks | exact |
| `.planning/REQUIREMENTS.md` *(modify)* | annotate SCRAPER-KAI-01..04 as "Pending — carry to v3.1" | docs | existing requirements table (lines 81-87, 171-177) | exact |

## Pattern Assignments

### `services/scraper/internal/providers/animekai/doc.go` (NEW — package docstring)

**Analog:** `services/scraper/internal/providers/gogoanime/doc.go` (lines 1-34, full file)

**Pattern: Package-level docstring with escape-hatch status and traceback to ROADMAP success criteria**

```go
// gogoanime/doc.go (analog — copy this shape exactly, swap names + add escape-hatch note)
// Package gogoanime implements the Gogoanime/Anitaku scraper provider
// (domain.Provider). Phase 18 of the v3.0 milestone.
//
// Naming: backend package slug is "gogoanime" (stable across mirror
// rebrands ...). User-facing display label is "Anitaku" ...
//
// Pivot rationale (2026-05-12): the 9anime mirror chain ... is unreachable;
// anitaku.to survives ... See .planning/phases/18-9anime/18-RESEARCH.md ...
//
// SCRAPER-9ANI-01..06 are the requirement IDs implemented by this package
// (literal names retained per CONTEXT.md S4 ...).
package gogoanime
```

**For animekai/doc.go, the planner should:**
- Replace `gogoanime` → `animekai`, `Anitaku` → `AnimeKai`
- Reference `Phase 19` not Phase 18
- ADD an "ESCAPE HATCH" section stating: "All Provider methods currently return `domain.ErrProviderDown` because Phase 19 R&D did not converge on an in-house token generator. SCRAPER-KAI-01..04 are carried to v3.1; SCRAPER-KAI-05..07 (flag wiring, observability, docs) ship now."
- Reference `SCRAPER-KAI-01..07` as the requirement IDs
- Cross-link to `.planning/phases/19-animekai-gated/19-RESEARCH.md` §Convergence Probability Assessment

---

### `services/scraper/internal/providers/animekai/client.go` (NEW — stub Provider impl)

**Analog:** `services/scraper/internal/providers/gogoanime/client.go` lines 1-225 (skeleton ONLY — methods 232-700 are NOT copied; they are replaced with stub bodies)

**Imports pattern (gogoanime/client.go lines 34-58):**
```go
import (
    "context"
    "errors"
    "fmt"
    "net/url" // not needed for stub but harmless
    "strings"
    "sync"
    "time"

    "github.com/ILITA-hub/animeenigma/libs/cache"
    "github.com/ILITA-hub/animeenigma/libs/logger"
    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
    "github.com/ILITA-hub/animeenigma/services/scraper/internal/health"
)
```
For the stub, the planner can prune unused imports (no goquery, no metrics, no fuzzy needed) but MUST keep `domain`, `health`, `logger`, `cache`, `errors`, `sync`, `time`, `context`.

**Constants pattern (gogoanime/client.go lines 64-91):**
```go
// providerName is the stable identifier returned by Name() and used as the
// orchestrator's registry key.
const providerName = "gogoanime"

// stageNames lock the canonical stage keys returned by HealthCheck. Phase 17
// canonical 5-stage strings minus the fifth (stream_segment) which is owned
// by the probe runner, not the provider.
var stageNames = []string{
    health.StageSearch,
    health.StageEpisodes,
    health.StageServers,
    health.StageStream,
}
```
For animekai: `const providerName = "animekai"`. Stage names IDENTICAL.

**Deps + Provider struct pattern (gogoanime/client.go lines 119-148):**
```go
type Deps struct {
    BaseURL string
    HTTP    *domain.BaseHTTPClient
    Embeds  *domain.Registry
    MalSync malSyncClient
    Cache   cache.Cache
    Log     *logger.Logger
}

type Provider struct {
    baseURL string
    http    *domain.BaseHTTPClient
    embeds  *domain.Registry
    malsync malSyncClient
    cache   cache.Cache
    log     *logger.Logger

    stagesMu sync.Mutex
    stages   map[string]domain.StageHealth
}
```
For animekai stub: the planner can keep these fields as-is for forward-compat with the v3.1 fill-in PR, even though the stub bodies don't use most of them. RESEARCH.md §Provider Package Skeleton also lists `MegacloudExtractor` as a Deps field — keep that too.

**New() constructor pattern (gogoanime/client.go lines 156-191):**
```go
func New(d Deps) (*Provider, error) {
    if d.HTTP == nil {
        return nil, errors.New("gogoanime: Deps.HTTP is required")
    }
    if d.Embeds == nil {
        return nil, errors.New("gogoanime: Deps.Embeds is required")
    }
    if d.MalSync == nil {
        return nil, errors.New("gogoanime: Deps.MalSync is required")
    }
    if d.Cache == nil {
        return nil, errors.New("gogoanime: Deps.Cache is required")
    }
    if d.Log == nil {
        d.Log = logger.Default()
    }
    base := d.BaseURL
    if base == "" {
        base = "https://anitaku.to"
    }
    p := &Provider{
        baseURL: strings.TrimRight(base, "/"),
        http:    d.HTTP,
        ...
        stages: make(map[string]domain.StageHealth, len(stageNames)),
    }
    for _, s := range stageNames {
        p.stages[s] = domain.StageHealth{Up: true}
    }
    return p, nil
}
```
For animekai stub: keep the same validation shape. CRITICAL: pre-seed `stages[s] = domain.StageHealth{Up: false, LastErr: "escape-hatch stub: SCRAPER-KAI-01..04 carried to v3.1"}` (NOT `Up: true`) — the probe runner will quickly settle these to `Up=0` anyway, but seeding them as `Up=false` from boot prevents Grafana from showing a green panel for the first ~15min before the first probe tick fires. Default `BaseURL` per RESEARCH.md line 499: `"https://anikai.to"`.

**Provider methods — STUB BODIES (the actual escape-hatch shape):**

Instead of copying gogoanime's real `FindID`/`ListEpisodes`/`ListServers`/`GetStream` (lines 232-700), every method in animekai/client.go returns a wrapped `ErrProviderDown` immediately:

```go
// errAnimeKaiStub is the canonical stub error. Wrapping it with
// domain.WrapProviderDown makes errors.Is(err, domain.ErrProviderDown)
// return true (orchestrator failover semantics) while preserving a clear
// cause string in the log line.
//
// RESEARCH.md Pitfall 4: returning ErrProviderDown (not ErrExtractFailed,
// not ErrNotFound) ensures the orchestrator's failover treats this as a
// SOFT skip — next provider in chain, no alert spam — and the probe
// runner flips provider_health_up{provider="animekai"} to 0 after 3
// consecutive 501s from the sidecar.
var errAnimeKaiStub = errors.New("animekai: escape-hatch stub (SCRAPER-KAI-01..04 carried to v3.1)")

func (p *Provider) Name() string { return providerName }

func (p *Provider) FindID(ctx context.Context, ref domain.AnimeRef) (string, error) {
    err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: FindID not implemented")
    p.markStage(health.StageSearch, err)
    return "", err
}

func (p *Provider) ListEpisodes(ctx context.Context, providerID string) ([]domain.Episode, error) {
    err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: ListEpisodes not implemented")
    p.markStage(health.StageEpisodes, err)
    return nil, err
}

func (p *Provider) ListServers(ctx context.Context, providerID, episodeID string) ([]domain.Server, error) {
    err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: ListServers not implemented")
    p.markStage(health.StageServers, err)
    return nil, err
}

func (p *Provider) GetStream(ctx context.Context, providerID, episodeID, serverID string, category domain.Category) (*domain.Stream, error) {
    err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: GetStream not implemented")
    p.markStage(health.StageStream, err)
    return nil, err
}
```

**HealthCheck + markStage pattern (gogoanime/client.go lines 201-225) — COPY EXACTLY, unchanged:**
```go
func (p *Provider) markStage(stage string, err error) {
    p.stagesMu.Lock()
    defer p.stagesMu.Unlock()
    sh := p.stages[stage]
    if err == nil {
        sh.Up = true
        sh.LastOK = time.Now()
        sh.LastErr = ""
    } else {
        sh.Up = false
        sh.LastErr = err.Error()
    }
    p.stages[stage] = sh
}

func (p *Provider) HealthCheck(ctx context.Context) domain.Health {
    p.stagesMu.Lock()
    defer p.stagesMu.Unlock()
    snap := make(map[string]domain.StageHealth, len(p.stages))
    for k, v := range p.stages {
        snap[k] = v
    }
    return domain.Health{Provider: providerName, Stages: snap}
}
```

**Error-wrapping pattern source (services/scraper/internal/domain/errors.go lines 22-44) — VERIFIED CONTRACT:**
```go
var ErrProviderDown = errors.New("scraper: provider down")

func WrapProviderDown(cause error, msg string) error {
    return fmt.Errorf("%s: %w (cause: %w)", msg, ErrProviderDown, cause)
}
```
The dual `%w` ensures `errors.Is(err, ErrProviderDown)` AND `errors.Is(err, errAnimeKaiStub)` both return true — the orchestrator matches on the sentinel, logs the cause.

---

### `services/scraper/internal/providers/animekai/dto.go` (NEW — stub DTOs)

**Analog:** `services/scraper/internal/providers/gogoanime/dto.go` (lines 1-57, full file)

**Pattern: Per-provider DTO file (unexported types — Provider interface returns domain types only)**

```go
// gogoanime/dto.go (analog — copy file structure and trim)
package gogoanime

type searchResult struct {
    Slug  string
    Title string
}

type episodeRow struct {
    Number  int
    URLSlug string
    Title   string
}

type serverRow struct {
    Name     string
    EmbedURL string
}

type malSyncEntry struct {
    Identifier any    `json:"identifier"`
    URL        string `json:"url"`
    Title      string `json:"title"`
}

type malSyncResponse struct {
    ID    int                                `json:"id"`
    Title string                             `json:"title"`
    Sites map[string]map[string]malSyncEntry `json:"Sites"`
}
```

**For animekai/dto.go, the planner should:**
- Keep the file SHORT — the stub doesn't actually parse anything. The file's purpose is to satisfy the "Phase 18 gogoanime pattern: doc.go, client.go, dto.go, cache.go, malsync.go" scaffold convention (CONTEXT.md decision section "Provider Package Layout").
- One minimal option: emit a `// dto.go — placeholder for v3.1 — fill in when SCRAPER-KAI-01..04 implementation lands` comment plus the `package animekai` declaration only.
- Alternative: copy gogoanime's full DTO shape verbatim (search/episode/server/malsync rows) so the v3.1 fill-in PR doesn't need to add the file. RECOMMENDED — it's <60 lines and signals intent.

---

### `services/scraper/internal/providers/animekai/client_test.go` (NEW — stub tests)

**Analog:** `services/scraper/internal/providers/gogoanime/client_test.go` (lines 1-50 for table setup; lines 60-120 for a typical sub-test).

Stub tests are SHALLOWER — they only verify the four interface methods return wrapped `ErrProviderDown`. Pattern:

```go
// client_test.go — stub-mode tests for animekai.Provider.
// SCRAPER-KAI-05..07 (Plan 19-stub Task 2). These tests lock in the
// escape-hatch contract: every Provider method returns ErrProviderDown.
// When the v3.1 fill-in PR lands, these tests are replaced wholesale by
// goldens-based tests mirroring gogoanime/client_test.go.

package animekai

import (
    "context"
    "errors"
    "testing"

    "github.com/ILITA-hub/animeenigma/services/scraper/internal/domain"
)

func newStubProvider(t *testing.T) *Provider {
    t.Helper()
    // Construct with mock Deps — escape-hatch stub never makes real HTTP
    // calls, so a no-op transport / fake malsync / in-memory cache are fine.
    // (Planner: lift gogoanime/helpers_test.go newMockProvider for the boilerplate.)
    return mustNew(t)
}

func TestProvider_FindID_StubReturnsErrProviderDown(t *testing.T) {
    p := newStubProvider(t)
    _, err := p.FindID(context.Background(), domain.AnimeRef{Title: "Naruto"})
    if !errors.Is(err, domain.ErrProviderDown) {
        t.Fatalf("expected ErrProviderDown, got %v", err)
    }
}

// Repeat for ListEpisodes, ListServers, GetStream.
```

**Cross-reference helper pattern:** `services/scraper/internal/providers/gogoanime/helpers_test.go` already builds a `newMockProvider` helper that wires a fake malsync, fake HTTP transport via `domain.WithTransport`, and an in-memory cache. For animekai, the planner can lift the same helper shape with even less wiring (the stub doesn't exercise any of them).

---

### `services/scraper/internal/config/config.go` (MODIFY — add AnimeKaiConfig)

**Analog:** `GogoanimeConfig` block in the SAME file (lines 65-71, 100-102, 122-130)

**Struct addition pattern (lines 17-23 + 65-71):**
```go
type Config struct {
    Server             ServerConfig
    MegacloudExtractor MegacloudExtractorConfig
    Redis              RedisConfig
    AnimePahe          AnimePaheConfig
    Gogoanime          GogoanimeConfig
    // NEW: append AnimeKai (keep ordering: animepahe → gogoanime → animekai
    // matches the orchestrator's failover order).
}

type GogoanimeConfig struct {
    BaseURL string
}
```

**For animekai, ADD (mirror RESEARCH.md lines 486-499):**
```go
// AnimeKaiConfig is the per-provider override surface for animekai.Provider
// (Phase 19 — gated). Enabled defaults to FALSE in production. Toggle via
// SCRAPER_ANIMEKAI_ENABLED=true (boolean string). BaseURL defaults to
// https://anikai.to (the canonical AnimeKai mirror as of 2026-05-12;
// animekai.to 301s here). Override via SCRAPER_ANIMEKAI_BASE_URL when the
// mirror rotates. Invalid URL fails service boot.
//
// SCRAPER-KAI-05: flag is read at orchestrator startup; restart-not-rebuild
// is achieved via `docker compose restart scraper`.
type AnimeKaiConfig struct {
    Enabled bool
    BaseURL string
}
```

**Load() addition pattern (lines 97-102 + 122-130):**
```go
// existing:
Gogoanime: GogoanimeConfig{
    BaseURL: getEnv("SCRAPER_GOGOANIME_BASE_URL", "https://anitaku.to"),
},
```

**For animekai, ADD:**
```go
AnimeKai: AnimeKaiConfig{
    Enabled: getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false), // default FALSE in prod
    BaseURL: getEnv("SCRAPER_ANIMEKAI_BASE_URL", "https://anikai.to"),
},
```

**Validation block pattern (lines 122-130):**
```go
if u := cfg.Gogoanime.BaseURL; u != "" {
    parsed, err := url.Parse(u)
    if err != nil {
        return nil, fmt.Errorf("invalid SCRAPER_GOGOANIME_BASE_URL %q: %w", u, err)
    }
    if parsed.Scheme == "" || parsed.Host == "" {
        return nil, fmt.Errorf("invalid SCRAPER_GOGOANIME_BASE_URL %q: missing scheme or host", u)
    }
}
```

**For animekai, ADD identical block** swapping `Gogoanime` → `AnimeKai` and the env-var name.

**NEW helper `getEnvBool`:** The file currently has `getEnv`, `getEnvInt`, `getEnvDuration` (lines 134-157) but NOT `getEnvBool`. The planner must add one — pattern:
```go
func getEnvBool(key string, defaultVal bool) bool {
    if val := os.Getenv(key); val != "" {
        if b, err := strconv.ParseBool(val); err == nil {
            return b
        }
    }
    return defaultVal
}
```
`strconv.ParseBool` accepts `"1"`, `"t"`, `"T"`, `"true"`, `"TRUE"`, `"True"`, `"0"`, `"f"`, `"false"`, etc — RESEARCH.md mentions the env-var value as the literal string `"true"` / `"false"`; both work.

**config_test.go (`services/scraper/internal/config/config_test.go`) MUST also gain a test case** verifying `getEnvBool` and the new AnimeKai defaults — pattern lifted from existing `TestLoad_Defaults` / `TestLoad_InvalidGogoanimeBaseURL`.

---

### `services/scraper/cmd/scraper-api/main.go` (MODIFY — conditional registration)

**Analog:** The existing gogoanime registration block (lines 127-154)

**Existing gogoanime registration (verbatim, lines 127-154):**
```go
// Gogoanime/Anitaku — second EN provider (Phase 18).
// Pivoted from "9anime" since the entire 9anime mirror chain is dead per
// 2026-05-12 research (.planning/phases/18-9anime/18-RESEARCH.md).
// Backend slug is "gogoanime"; the user-facing display label is "Anitaku".
// Registration ORDER is the failover ORDER (CONTEXT.md D5) — animepahe
// is tried first; gogoanime is the second-chance provider when animepahe
// is unhealthy or returns ErrNotFound.
gogoanimeBaseHTTP := domain.NewBaseHTTPClient(log,
    domain.WithPerHostRPS("anitaku.to", 1.0, 2),
    domain.WithPerHostRPS("vibeplayer.site", 1.0, 2),
    domain.WithPerHostRPS("otakuhg.site", 1.0, 2),
    domain.WithPerHostRPS("otakuvid.online", 1.0, 2),
    domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
)
gogoanimeMalsync := gogoanime.NewMalSyncClient(redisCache)
gogoanimeProvider, err := gogoanime.New(gogoanime.Deps{
    BaseURL: cfg.Gogoanime.BaseURL,
    HTTP:    gogoanimeBaseHTTP,
    Embeds:  registry,
    MalSync: gogoanimeMalsync,
    Cache:   redisCache,
    Log:     log,
})
if err != nil {
    log.Fatalw("failed to construct Gogoanime provider", "error", err)
}
orchestrator.Register(gogoanimeProvider)
log.Infow("registered provider", "name", gogoanimeProvider.Name())
```

**For animekai, ADD immediately after the gogoanime block (RESEARCH.md lines 445-481):**
```go
// Phase 19 — AnimeKai (gated, ESCAPE-HATCH path). Default FALSE in prod.
// SCRAPER-KAI-05: env-flag toggle; SCRAPER-KAI-06: stub provider returns
// ErrProviderDown so failover lands on the previous two providers.
if cfg.AnimeKai.Enabled {
    animeKaiBaseHTTP := domain.NewBaseHTTPClient(log,
        domain.WithPerHostRPS("anikai.to", 1.0, 2),
        domain.WithPerHostRPS("megaup.cc", 1.0, 2),
        domain.WithPerHostRPS("api.malsync.moe", 2.0, 4),
    )
    animeKaiMalsync := animekai.NewMalSyncClient(redisCache) // forward-compat stub or omit if dto.go is minimal
    animeKaiProvider, err := animekai.New(animekai.Deps{
        BaseURL: cfg.AnimeKai.BaseURL,
        HTTP:    animeKaiBaseHTTP,
        Embeds:  registry,
        MalSync: animeKaiMalsync,
        Cache:   redisCache,
        Log:     log,
    })
    if err != nil {
        log.Fatalw("failed to construct AnimeKai provider", "error", err)
    }
    orchestrator.Register(animeKaiProvider)
    log.Infow("registered provider", "name", animeKaiProvider.Name(),
        "flag", "SCRAPER_ANIMEKAI_ENABLED=true",
        "mode", "escape-hatch-stub")
} else {
    log.Infow("AnimeKai provider SKIPPED (flag off)",
        "flag", "SCRAPER_ANIMEKAI_ENABLED=false")
}
```

**Wiring invariant pattern (existing lines 228-231 — the Phase 18 invariant):**
```go
// Phase 18 wiring invariant — fatal if the provider chain didn't grow
// to 2 (animepahe + gogoanime). A future maintainer dropping one of the
// orchestrator.Register calls would silently degrade the failover chain
// to a single provider; this guard surfaces the regression at boot.
if got, want := len(orchestrator.RegisteredProviders()), 2; got != want {
    log.Fatalw("Phase 18 wiring invariant broken: expected 2 providers (animepahe + gogoanime)",
        "got", got, "want", want)
}
```

**REPLACE that block with the Phase 19 adaptation (RESEARCH.md lines 472-480):**
```go
// Phase 19 wiring invariant — adapts the Phase 18 invariant to the
// flag-conditional shape. With flag off (prod default): 2 providers.
// With flag on (R&D / staging): 3 providers — animepahe, gogoanime,
// animekai-stub.
expectedProviders := 2
if cfg.AnimeKai.Enabled {
    expectedProviders = 3
}
if got := len(orchestrator.RegisteredProviders()); got != expectedProviders {
    log.Fatalw("Phase 19 wiring invariant broken",
        "got", got, "want", expectedProviders,
        "flag", cfg.AnimeKai.Enabled)
}
```

**Logging additions (existing lines 233-242 — the boot summary log):**
```go
log.Infow("scraper service ready",
    "port", cfg.Server.Port,
    ...
    "animepahe_base_url", cfg.AnimePahe.BaseURL,
    "gogoanime_base_url", cfg.Gogoanime.BaseURL,
)
```
Add: `"animekai_enabled", cfg.AnimeKai.Enabled`, `"animekai_base_url", cfg.AnimeKai.BaseURL`.

**Import addition (existing lines 19-20):**
```go
"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animepahe"
"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/gogoanime"
```
Add: `"github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/animekai"`.

---

### `docker/megacloud-extractor/server.js` (MODIFY — add `/animekai-token` stub route)

**Analog:** Existing `/extract` and `/health` route blocks (lines 210-243)

**Existing route pattern (lines 210-243):**
```javascript
const server = http.createServer(async (req, res) => {
  const parsed = url.parse(req.url, true);

  if (parsed.pathname === "/health") {
    res.writeHead(200, { "Content-Type": "application/json" });
    res.end(JSON.stringify({ status: "ok" }));
    return;
  }

  if (parsed.pathname === "/extract") {
    const embedUrl = parsed.query.url;
    if (!embedUrl) {
      res.writeHead(400, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: "url parameter required" }));
      return;
    }

    try {
      console.log(`\nExtracting: ${embedUrl}`);
      const start = Date.now();
      const result = await extractSources(embedUrl);
      const elapsed = Date.now() - start;
      console.log(...);
      res.writeHead(200, { "Content-Type": "application/json" });
      res.end(JSON.stringify(result));
    } catch (err) {
      console.error(`Extraction failed: ${err.message}`);
      res.writeHead(500, { "Content-Type": "application/json" });
      res.end(JSON.stringify({ error: err.message }));
    }
    return;
  }

  res.writeHead(404, { "Content-Type": "application/json" });
  res.end(JSON.stringify({ error: "not found" }));
});
```

**For animekai, INSERT before the catch-all 404 (RESEARCH.md lines 787-791 + Pitfall 4 lines 898-903):**
```javascript
// Phase 19 — AnimeKai escape-hatch stub. Returns HTTP 501 so the Go
// scraper's animekai.Provider can map status==501 → domain.ErrProviderDown
// once with a clear log warning, then the in-memory healthCache flips
// to 0 after 3 consecutive 501s and the probe deregisters. This avoids
// the 500-status retry-storm pitfall (RESEARCH.md Pitfall 4).
//
// SCRAPER-KAI-04 is explicitly carried to v3.1 — when this stub becomes
// a real implementation, only this handler changes; the Go side is
// already wired.
if (parsed.pathname === "/animekai-token" && req.method === "POST") {
  console.warn(`/animekai-token called — escape-hatch stub returning 501`);
  res.writeHead(501, { "Content-Type": "application/json" });
  res.end(JSON.stringify({
    error: "AnimeKai sidecar not yet converged — carry to v3.1"
  }));
  return;
}
```

**CRITICAL — RESEARCH.md Pitfall 4 (line 898-903):** Use HTTP **501** (Not Implemented), NOT 500. The Go provider's logic distinguishes:
- 5xx → ErrProviderDown → soft retry then skip (would loop)
- 501 specifically → ErrProviderDown once, then probe drops the provider after 3 consecutive 501s → no retry storm

The planner MUST make the test scaffolding assert the response is 501, not just `>= 500`.

**Cross-reference:** `services/scraper/internal/embeds/megacloud.go` (Phase 16) is the analog for how the Go side calls the sidecar over HTTP. The future `services/scraper/internal/providers/animekai/megacloud_extractor.go` (CARRIED TO v3.1) would map sidecar HTTP 501 → `domain.ErrProviderDown`.

---

### `docker/docker-compose.yml` (MODIFY — env var documentation)

**Analog:** Existing scraper service block (lines 147-177), specifically the `environment:` map (lines 153-164)

**Existing env-map pattern (lines 153-164):**
```yaml
    environment:
      SERVER_PORT: 8088
      SERVER_HOST: 0.0.0.0
      MEGACLOUD_EXTRACTOR_URL: http://megacloud-extractor:3200
      # Phase 16 plan 05 additions — Redis cache for malsync/episodes/stream
      # and the AnimePahe base URL (env-controlled so domain rotation is
      # restart-not-rebuild per the Wave 1 16-01 connectivity finding).
      REDIS_HOST: redis
      REDIS_PORT: 6379
      REDIS_PASSWORD: ""
      REDIS_DB: 0
      ANIMEPAHE_BASE_URL: https://animepahe.ru
```

**For animekai, APPEND:**
```yaml
      # Phase 19 — AnimeKai (gated). Flag is default-off in production.
      # See docker/.env.example and .planning/REQUIREMENTS.md SCRAPER-KAI-05.
      # Toggle live with: SCRAPER_ANIMEKAI_ENABLED=true docker compose up -d scraper
      SCRAPER_ANIMEKAI_ENABLED: ${SCRAPER_ANIMEKAI_ENABLED:-false}
      SCRAPER_ANIMEKAI_BASE_URL: ${SCRAPER_ANIMEKAI_BASE_URL:-https://anikai.to}
```

Note: pattern uses `${VAR:-default}` so the docker-compose value SOURCES from the `.env` file but provides an explicit fallback. The existing `ANIMEPAHE_BASE_URL: https://animepahe.ru` hardcodes; the new flag follows the `${VAR:-default}` convention because the README docs assume you toggle via .env, not by editing docker-compose.yml.

---

### `docker/.env.example` (MODIFY — document the flag)

**Analog:** Existing Phase 18 block (lines 80-86)

**Existing pattern (lines 80-86):**
```bash
# =============================================================================
# Phase 18 — Gogoanime/Anitaku scraper provider
# =============================================================================
# Optional override for the Anitaku mirror. Defaults to https://anitaku.to.
# Gogoanime has historically rotated mirrors; if anitaku.to becomes
# unreachable, set this to the alive alternative (e.g. https://anitaku.io).
# SCRAPER_GOGOANIME_BASE_URL=https://anitaku.to
```

**For animekai, APPEND:**
```bash
# =============================================================================
# Phase 19 — AnimeKai scraper provider (gated, escape-hatch)
# =============================================================================
# Default: OFF in production. AnimeKai shipped in Phase 19 as a flag-only
# stub because the in-house MegaUp token generator did not converge during
# R&D. The provider's Go methods return ErrProviderDown immediately, the
# sidecar /animekai-token route returns HTTP 501, and the orchestrator
# falls back to AnimePahe → Gogoanime without users seeing the third option.
#
# To enable (R&D / staging only — does NOT actually serve streams until
# SCRAPER-KAI-01..04 ship in v3.1):
#   SCRAPER_ANIMEKAI_ENABLED=true
# SCRAPER_ANIMEKAI_ENABLED=false
#
# Mirror override — defaults to https://anikai.to. animekai.to 301s here;
# both work. Override only if anikai.to becomes unreachable.
# SCRAPER_ANIMEKAI_BASE_URL=https://anikai.to
```

---

### `.planning/REQUIREMENTS.md` (MODIFY — annotate SCRAPER-KAI-01..04 carry status)

**Analog:** Existing SCRAPER-9ANI implementation note (line 70) — established precedent for adding a "Implementation note" block above a numbered list.

**Existing precedent (line 70):**
```markdown
> **Implementation note (2026-05-12):** SCRAPER-9ANI-01..06 are implemented by the Gogoanime/Anitaku provider (display label "Anitaku", backend slug "gogoanime"). The 9anime mirror chain ... is unreachable as of 2026-05-12; only anitaku.to survives ... Requirement IDs keep their literal SCRAPER-9ANI-* prefix.
```

**For SCRAPER-KAI-01..04, INSERT above line 81 (the start of the SCRAPER-KAI numbered list):**
```markdown
> **Implementation note (2026-05-12 — Phase 19 escape hatch):** SCRAPER-KAI-01..04 are **carried to v3.1**. Phase 19 shipped only the gate (SCRAPER-KAI-05 ✓), the failover ordering (SCRAPER-KAI-06 partial — flag default-off is locked, in-cluster sidecar stub returns HTTP 501), and the v3.1 carryover annotation in this document. The provider package `services/scraper/internal/providers/animekai/` exists as a stub whose every Provider method returns `domain.ErrProviderDown`; the sidecar route `POST /animekai-token` returns HTTP 501. See `.planning/phases/19-animekai-gated/19-RESEARCH.md` §Convergence Probability Assessment for the rationale. SCRAPER-KAI-07 (end-to-end stream-from-AnimeKai verification) is **blocked on SCRAPER-KAI-01..04** and also carries to v3.1.
```

**Status table updates (lines 171-177):**
```markdown
| SCRAPER-KAI-01 | Phase 19 | Pending |
| SCRAPER-KAI-02 | Phase 19 | Pending |
| SCRAPER-KAI-03 | Phase 19 | Pending |
| SCRAPER-KAI-04 | Phase 19 | Pending |
| SCRAPER-KAI-05 | Phase 19 | Pending |
| SCRAPER-KAI-06 | Phase 19 | Pending |
| SCRAPER-KAI-07 | Phase 19 | Pending |
```

**Update to:**
```markdown
| SCRAPER-KAI-01 | Phase 19 → v3.1 | Carry — escape hatch |
| SCRAPER-KAI-02 | Phase 19 → v3.1 | Carry — escape hatch |
| SCRAPER-KAI-03 | Phase 19 → v3.1 | Carry — escape hatch |
| SCRAPER-KAI-04 | Phase 19 → v3.1 | Carry — escape hatch |
| SCRAPER-KAI-05 | Phase 19 | Done (flag wired, default off) |
| SCRAPER-KAI-06 | Phase 19 | Done (escape hatch taken; flag default-off documented) |
| SCRAPER-KAI-07 | Phase 19 → v3.1 | Carry — blocked on KAI-01..04 |
```

The planner can also add a `v3.1` entry to the ROADMAP if one doesn't already exist (RESEARCH.md line 111 mentions a v3.1 milestone marker; check `.planning/MILESTONES.md` before deciding).

---

## Shared Patterns

### Pattern A: ErrProviderDown wrapping (applies to all 4 stub provider methods)

**Source:** `services/scraper/internal/domain/errors.go` lines 22-44
**Apply to:** Every method in `services/scraper/internal/providers/animekai/client.go`

```go
// Sentinel — keep this verbatim form, the orchestrator uses errors.Is.
var ErrProviderDown = errors.New("scraper: provider down")

// Dual-wrap: errors.Is(err, ErrProviderDown) AND errors.Is(err, cause)
// both return true. Orchestrator matches on ErrProviderDown; log readers
// see the original cause.
func WrapProviderDown(cause error, msg string) error {
    return fmt.Errorf("%s: %w (cause: %w)", msg, ErrProviderDown, cause)
}
```

**Used as:**
```go
err := domain.WrapProviderDown(errAnimeKaiStub, "animekai: <METHOD> not implemented")
p.markStage(health.<STAGE>, err)
return <zero-value>, err
```

### Pattern B: Stage health bookkeeping (applies to all 4 stub methods + HealthCheck)

**Source:** `services/scraper/internal/providers/gogoanime/client.go` lines 201-225
**Apply to:** Every method in `services/scraper/internal/providers/animekai/client.go`

Every method calls `p.markStage(<stage>, <err>)` before returning. `HealthCheck()` snapshots the in-memory map. The Phase 17 probe runner reads `HealthCheck()` and pushes `provider_health_up{provider="animekai",stage=...}` to Prometheus.

### Pattern C: Env-var binding with default + URL validation (applies to config.go additions)

**Source:** `services/scraper/internal/config/config.go` lines 100-130 (GogoanimeConfig block)
**Apply to:** The new AnimeKaiConfig + the new `getEnvBool` helper

```go
// Bind default
AnimeKai: AnimeKaiConfig{
    Enabled: getEnvBool("SCRAPER_ANIMEKAI_ENABLED", false),
    BaseURL: getEnv("SCRAPER_ANIMEKAI_BASE_URL", "https://anikai.to"),
},

// Validate URL — fail boot on bad input
if u := cfg.AnimeKai.BaseURL; u != "" {
    parsed, err := url.Parse(u)
    if err != nil || parsed.Scheme == "" || parsed.Host == "" {
        return nil, fmt.Errorf("invalid SCRAPER_ANIMEKAI_BASE_URL %q", u)
    }
}
```

### Pattern D: Boot-time wiring invariant (applies to main.go)

**Source:** `services/scraper/cmd/scraper-api/main.go` lines 228-231
**Apply to:** Replace existing Phase 18 invariant with Phase 19 conditional invariant

```go
expectedProviders := 2 // animepahe + gogoanime
if cfg.AnimeKai.Enabled {
    expectedProviders = 3 // + animekai stub
}
if got := len(orchestrator.RegisteredProviders()); got != expectedProviders {
    log.Fatalw("Phase 19 wiring invariant broken",
        "got", got, "want", expectedProviders, "flag", cfg.AnimeKai.Enabled)
}
```

This is critical because the escape-hatch stub provider registers identically to a real provider — the only way to catch "a maintainer commented out the Register() call by accident" is the count check. The flag-conditional form is necessary because the count varies.

### Pattern E: Sidecar HTTP 501 stub (sidecar route handler)

**Source:** RESEARCH.md §Pitfall 4 (lines 898-903) + RESEARCH.md §Code Examples lines 787-791
**Apply to:** `docker/megacloud-extractor/server.js` new route block

```javascript
// Use 501 (Not Implemented), NOT 500 (Internal Server Error) —
// the Go side's status-code mapping treats them differently:
//   500 → ErrProviderDown with retry storm (orchestrator hammers it)
//   501 → ErrProviderDown once + probe deregisters after 3 → silent
if (parsed.pathname === "/animekai-token" && req.method === "POST") {
  console.warn(`/animekai-token: escape-hatch stub`);
  res.writeHead(501, { "Content-Type": "application/json" });
  res.end(JSON.stringify({
    error: "AnimeKai sidecar not yet converged — carry to v3.1"
  }));
  return;
}
```

---

## No Analog Found

None. Every file maps cleanly to an existing analog — this is the entire reason the escape-hatch path is so cheap.

Files explicitly DEFERRED to v3.1 (no analog needed yet because they aren't part of the escape-hatch ship):

| File | Role | Status |
|------|------|--------|
| `services/scraper/internal/providers/animekai/cache.go` | Stream URL TTL parser | Carry to v3.1 (no real stream URLs to cache from a stub) |
| `services/scraper/internal/providers/animekai/malsync.go` | MAL → AnimeKai resolver | OPTIONAL for escape hatch — see note below |
| `services/scraper/internal/providers/animekai/megacloud_extractor.go` | HTTP wrapper around sidecar `/animekai-token` | Carry to v3.1 |
| `services/scraper/internal/providers/animekai/dto_test.go` / `cache_test.go` / `malsync_test.go` / `megacloud_extractor_test.go` | Goldens-based unit tests | Carry to v3.1 |

**Note on malsync.go:** If the planner chooses to include a `malsync.go` and `NewMalSyncClient` constructor in the stub (matching the RESEARCH.md "Provider Package Skeleton"), the analog is `services/scraper/internal/providers/gogoanime/malsync.go` lines 1-80 (constants + struct + constructor) — copy the cache-TTL constants, the option-pattern struct, and the constructor; the actual `Lookup` method body can be a stub returning `("", false, nil)` (false = miss; orchestrator falls through to the next provider's FindID logic, which there isn't one of, so it lands on ErrProviderDown anyway). This adds ~60 LOC for forward-compat.

**Recommendation:** The planner should consider whether to ship `malsync.go` and `cache.go` as forward-compat scaffolds (~100 LOC total, frictionless v3.1 fill-in) OR omit them and let v3.1 add them along with method bodies. RESEARCH.md "Provider Package Skeleton" (lines 429-441) suggests shipping the full file list; the cost is small.

---

## Metadata

**Analog search scope:**
- `services/scraper/internal/providers/{gogoanime,animepahe}/` (full directory)
- `services/scraper/internal/domain/{provider.go,errors.go}` (interface contracts)
- `services/scraper/internal/config/config.go` (existing env-binding patterns)
- `services/scraper/cmd/scraper-api/main.go` (wiring pattern, invariant pattern)
- `docker/megacloud-extractor/server.js` (sidecar route handler pattern)
- `docker/docker-compose.yml` + `docker/.env.example` (env var documentation pattern)
- `.planning/REQUIREMENTS.md` (carryover annotation precedent — SCRAPER-9ANI line 70)

**Files scanned:** 13 source files + 3 docs/config files
**Pattern extraction date:** 2026-05-12

**Key insight:** Every line of the escape-hatch ship is either (a) verbatim from gogoanime with one identifier swap, or (b) a stub body that returns `domain.WrapProviderDown(errAnimeKaiStub, "animekai: X not implemented")`. The Go LOC budget is ~150 lines; the JS LOC is ~10. Total work is wiring + tests + docs, not new logic — confirming RESEARCH.md's 3-4 day estimate.
