# Content-Verify Probing System — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build `services/content-verify` (:8101) — a throttled probing service that verifies the REAL audio language (faster-whisper LID) and burned-in subtitles (pixel+OCR) of every anime×provider×unit, stores JSONB verdicts, and feeds aePlayer so unverified sources default to RAW with an explicit marker while DUB/burned-in badges appear only at ≥95% confidence.

**Architecture:** New Go service (queue + throttled worker + stream resolve via catalog/gateway) bundling Python analyzers (faster-whisper tiny int8, tesseract OCR port of `tools/subprobe`) in one debian-slim container. Virtual priority queue computed each tick from Redis signals (visits +15/unique user, ongoing +10, top-100 +5). Catalog proxies a public endpoint + blends summaries into capabilities (Phase-B playability pattern); player fires watching-hints; aePlayer polls dynamically and re-picks the auto-selected combo strictly before playback starts.

**Tech Stack:** Go 1.25 (chi, GORM, go-redis v9), Python 3.11+ (faster-whisper, numpy, Pillow, pytesseract-free direct `tesseract` subprocess), ffmpeg, Vue 3 + TS (vitest).

**Spec:** `docs/superpowers/specs/2026-07-16-content-verify-probing-design.md` (owner-approved 2026-07-16).

## Global Constraints

- **Port 8101**, module `github.com/ILITA-hub/animeenigma/services/content-verify` — confirmed free.
- **NEVER run `go work sync`** (bumps unrelated modules fleet-wide). Never `gofmt -w` / `make fmt` (smartquote landmine in analytics).
- **New go.work module ⇒ `COPY services/content-verify/go.mod services/content-verify/go.sum* ./services/content-verify/` added to ALL 21 Go-service Dockerfiles** (list in Task 10). stealth-scraper (Python) excluded.
- **Service-local plain Prometheus metrics live in `internal/cvmetrics`, NEVER `libs/metrics`** (auto-registration trap: plain promauto in a shared lib exports permanent-0 impostor series from every importer).
- **Verified threshold = confidence ≥ 0.95** (owner spec). Unverified ⇒ FE treats as RAW + explicit "не проверено" marker; DUB facet lists only verified-dub. Combo auto-correction ONLY before playback starts (`hasStarted === false`) and only when `providerAutoSelected === true`.
- Throttle: **1 probe at a time, 60s tick, ~50s unit budget** (budget is a REVISIT-AFTER-TESTS item — Task 17 measures real timings). Governor: skip claiming at degradation level ≥ 1 (`libs/cache` DegradationWatcher).
- i18n: every new key added to **en.json + ru.json + ja.json** with identical ICU placeholders (`locale-parity.spec.ts` gate).
- Frontend: bind semantic tokens only (DS-lint build gate); `bun`, not npm; `bunx`, not npx.
- Commits: pathspec only (`git add <paths>` — NEVER `git add -A`; `.claude/worktrees/`, `tmp/`, `services/scraper/scraper-api` are untracked-not-ignored). Co-authors on every commit:
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Subagents commit but do NOT push; the orchestrator pushes after review.
- Go tests: `go test ./...` from the module dir. FE: `cd frontend/web && bunx vitest run <spec>`.
- All paths relative to the worktree root `/data/animeenigma/.claude/worktrees/content-verify-probing`.

## File Structure (new/modified)

```
services/content-verify/
├── Dockerfile                          # Task 10 (go builder + debian-slim + ffmpeg/tesseract/whisper)
├── go.mod / go.sum                     # Task 1
├── cmd/content-verify-api/main.go      # Task 1 (+wiring in 9)
├── analyzers/
│   ├── requirements.txt                # Task 7
│   ├── lid.py                          # Task 7 (faster-whisper language ID)
│   └── hardsub.py                      # Task 7 (subprobe tier1+tesseract port)
└── internal/
    ├── config/config.go                # Task 1
    ├── cvmetrics/metrics.go            # Task 1
    ├── domain/verify.go                # Task 2 (model + Summarize)
    ├── repo/store.go                   # Task 2
    ├── signals/signals.go              # Task 3 (Redis visits/cooldowns)
    ├── catalogclient/client.go         # Task 5
    ├── queue/enumerate.go              # Task 5 (unit enumeration)
    ├── queue/queue.go                  # Task 6 (score/rank/pending/backoff)
    ├── queue/engine.go                 # Task 6 (claim + snapshot)
    ├── prober/playlist.go              # Task 8 (proxied URL + localize)
    ├── prober/extract.go               # Task 8 (ffmpeg)
    ├── prober/runner.go                # Task 8 (python exec)
    ├── prober/assemble.go              # Task 8 (verdict assembly)
    ├── prober/prober.go                # Task 8
    ├── service/worker.go               # Task 9
    ├── handler/verify.go               # Task 9
    └── transport/router.go             # Task 1 (+routes in 9)
go.work                                 # Task 1 (add ./services/content-verify)
services/*/Dockerfile (21 files)        # Task 10 (COPY sweep)
docker/docker-compose.yml               # Task 10
deploy/kustomize/base/services/content-verify.yaml + kustomization  # Task 10
docs/environment-variables.md, CLAUDE.md (ports)                    # Task 10
services/catalog/internal/repo/anime.go                 # Task 4 (ListVerifyMembership)
services/catalog/internal/handler/internal_verify.go    # Task 4
services/catalog/internal/handler/content_verify.go     # Task 11 (public proxy + visit hint)
services/catalog/internal/service/capability/verify_client.go  # Task 11
services/catalog/internal/service/capability/service.go # Task 11 (blend)
services/catalog/internal/domain/capability.go          # Task 11 (ProviderCap.Verify)
services/catalog/internal/transport/router.go           # Tasks 4+11
services/catalog/cmd/catalog-api/main.go                # Tasks 4+11
services/player/internal/service/verify_hint.go         # Task 12
services/player/internal/service/list.go                # Task 12
services/player/internal/config/config.go               # Task 12
services/player/cmd/player-api/main.go                  # Task 12
frontend/web/src/types/contentVerify.ts                 # Task 13
frontend/web/src/api/client.ts                          # Task 13
frontend/web/src/composables/aePlayer/useContentVerify.ts   # Task 13
frontend/web/src/composables/aePlayer/verifiedCaps.ts   # Task 14
frontend/web/src/composables/aePlayer/useProviderFeed.ts    # Task 14
frontend/web/src/composables/aePlayer/useCapabilityFeed.ts  # Task 14
frontend/web/src/composables/aePlayer/capLabels.ts      # Task 14
frontend/web/src/components/player/aePlayer/ProviderChip.vue  # Task 14
frontend/web/src/components/player/aePlayer/SourcePanel.vue   # Task 14
frontend/web/src/locales/{en,ru,ja}.json                # Task 14
frontend/web/src/composables/aePlayer/useComboBootstrap.ts    # Task 15
frontend/web/src/composables/aePlayer/useDebugTools.ts  # Task 15
frontend/web/src/components/player/aePlayer/AePlayer.vue      # Task 15 (wiring)
frontend/web/src/types/aePlayer.ts                      # Task 14 (ProviderRow.verify)
```

---

### Task 1: content-verify module scaffold (config + metrics + router + main)

**Files:**
- Create: `services/content-verify/go.mod`
- Create: `services/content-verify/internal/config/config.go`
- Create: `services/content-verify/internal/cvmetrics/metrics.go`
- Create: `services/content-verify/internal/transport/router.go`
- Create: `services/content-verify/cmd/content-verify-api/main.go`
- Modify: `go.work` (add `./services/content-verify` to the `use` block, keep alphabetical-ish placement near other services)
- Test: `services/content-verify/internal/config/config_test.go`

**Interfaces:**
- Consumes: `libs/{logger,cache,database,metrics,tracing,httputil}` via replace directives.
- Produces: `config.Config` (fields below) used by every later task; `transport.NewRouter(h *handler.VerifyHandler, log *logger.Logger, collector *metrics.Collector) http.Handler` (handler nil-tolerant until Task 9); `cvmetrics.{QueueDepth, ProbesTotal, ProbeDuration, TicksSkippedTotal, LastProbeTS}`.

- [ ] **Step 1: go.mod + go.work.** Create `services/content-verify/go.mod` (mirror `services/governor/go.mod` — same go version, same replace style):

```
module github.com/ILITA-hub/animeenigma/services/content-verify

go 1.25.0

require (
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
	github.com/go-chi/chi/v5 v5.1.0
	github.com/google/uuid v1.6.0
	github.com/prometheus/client_golang v1.20.5
	github.com/redis/go-redis/v9 v9.6.3
	gorm.io/gorm v1.25.12
)

replace (
	github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache
	github.com/ILITA-hub/animeenigma/libs/database => ../../libs/database
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/metrics => ../../libs/metrics
	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
)
```

Copy exact dependency VERSIONS from `services/governor/go.mod` (chi/prometheus/go-redis/gorm lines) — if governor pins different versions, use governor's. Add `./services/content-verify` to `go.work`'s `use` block. Run `cd services/content-verify && go mod tidy` AFTER the source files of this task exist (Step 6).

- [ ] **Step 2: config.** `internal/config/config.go` — governor's getEnv pattern:

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

type Config struct {
	Server   ServerConfig
	Redis    cache.Config
	Database database.Config

	CatalogURL   string        // internal catalog base (membership, structure, streams)
	GatewayURL   string        // public gateway base — ffmpeg reads hls-proxy through it
	Interval     time.Duration // pause between probes (1 unit per tick)
	UnitBudget   time.Duration // hard per-unit budget; REVISIT AFTER TESTS (spec §2)
	ReprobeTTL   time.Duration // verified/inconclusive re-probe age
	TopLimit     int           // top-N membership
	FFmpegPath   string
	PythonPath   string
	AnalyzersDir string
	WorkDir      string
	WorkerOn     bool
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{Host: getEnv("SERVER_HOST", "0.0.0.0"), Port: getEnvInt("SERVER_PORT", 8101)},
		Redis: cache.Config{
			Host: getEnv("REDIS_HOST", "redis"), Port: getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""), DB: getEnvInt("REDIS_DB", 0),
		},
		Database: database.Config{
			Host: getEnv("DB_HOST", "localhost"), Port: getEnvInt("DB_PORT", 5432),
			User: getEnv("DB_USER", "postgres"), Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"), SSLMode: getEnv("DB_SSLMODE", "disable"),
		},
		CatalogURL:   getEnv("CV_CATALOG_URL", "http://catalog:8081"),
		GatewayURL:   getEnv("CV_GATEWAY_URL", "http://gateway:8000"),
		Interval:     getEnvDuration("CV_INTERVAL", time.Minute),
		UnitBudget:   getEnvDuration("CV_UNIT_BUDGET", 50*time.Second),
		ReprobeTTL:   getEnvDuration("CV_REPROBE_TTL", 720*time.Hour),
		TopLimit:     getEnvInt("CV_TOP_LIMIT", 100),
		FFmpegPath:   getEnv("CV_FFMPEG_PATH", "ffmpeg"),
		PythonPath:   getEnv("CV_PYTHON", "python3"),
		AnalyzersDir: getEnv("CV_ANALYZERS_DIR", "/app/analyzers"),
		WorkDir:      getEnv("CV_WORKDIR", "/tmp/cv"),
		WorkerOn:     getEnv("CV_WORKER_ENABLED", "true") != "false",
	}
	if cfg.Interval < 10*time.Second {
		return nil, fmt.Errorf("CV_INTERVAL too small: %s", cfg.Interval)
	}
	if cfg.UnitBudget >= cfg.Interval {
		return nil, fmt.Errorf("CV_UNIT_BUDGET (%s) must be < CV_INTERVAL (%s)", cfg.UnitBudget, cfg.Interval)
	}
	return cfg, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getEnvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getEnvDuration(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}
```

- [ ] **Step 3: failing config test.** `internal/config/config_test.go`:

```go
package config

import (
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Server.Port != 8101 {
		t.Fatalf("port = %d, want 8101", cfg.Server.Port)
	}
	if cfg.Interval != time.Minute || cfg.UnitBudget != 50*time.Second {
		t.Fatalf("throttle defaults wrong: %s / %s", cfg.Interval, cfg.UnitBudget)
	}
}

func TestLoadRejectsBudgetOverInterval(t *testing.T) {
	t.Setenv("CV_UNIT_BUDGET", "2m")
	if _, err := Load(); err == nil {
		t.Fatal("want error when budget >= interval")
	}
}
```

- [ ] **Step 4: cvmetrics.** `internal/cvmetrics/metrics.go` (deliberately NOT in libs/metrics — auto-registration trap):

```go
// Package cvmetrics holds content-verify's service-local Prometheus metrics.
// Deliberately NOT in libs/metrics: plain promauto metrics auto-register in
// every binary importing the shared package and would export permanent-0
// impostor series from ~20 services.
package cvmetrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	QueueDepth = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_verify_queue_depth", Help: "Candidates with a non-zero score at the last snapshot.",
	})
	ProbesTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_probes_total", Help: "Unit probes by provider and result.",
	}, []string{"provider", "result"}) // result: verified|inconclusive|unreachable|error|synth
	ProbeDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "content_verify_probe_duration_seconds",
		Help:    "Wall time of one unit probe (resolve→extract→analyze).",
		Buckets: []float64{5, 10, 20, 30, 40, 50, 60, 90, 120},
	})
	TicksSkippedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "content_verify_ticks_skipped_total", Help: "Worker ticks that did no probe.",
	}, []string{"reason"}) // reason: degraded|idle|claim_error
	LastProbeTS = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "content_verify_last_probe_timestamp", Help: "Unix time of the last completed probe.",
	})
)
```

- [ ] **Step 5: router + main.** `internal/transport/router.go`:

```go
package transport

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/handler"
)

func NewRouter(h *handler.VerifyHandler, log *logger.Logger, collector *metrics.Collector) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	if collector != nil {
		r.Use(collector.Middleware)
	}
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	r.Handle("/metrics", metrics.Handler())
	if h != nil {
		r.Get("/internal/verify/verdicts", h.Verdicts)
		r.Post("/internal/verify/hint", h.Hint)
		r.Get("/internal/verify/queue", h.Queue)
	}
	return r
}
```

(If governor's router uses a different collector-middleware method name, mirror governor's `internal/transport/router.go` exactly — check it before writing.) Create `internal/handler/verify.go` as a compile-stub for now (fleshed out in Task 9):

```go
package handler

import "net/http"

// VerifyHandler serves the internal verdicts/hint/queue API. Wired in Task 9.
type VerifyHandler struct{}

func (h *VerifyHandler) Verdicts(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusNotImplemented) }
func (h *VerifyHandler) Hint(w http.ResponseWriter, r *http.Request)     { w.WriteHeader(http.StatusNotImplemented) }
func (h *VerifyHandler) Queue(w http.ResponseWriter, r *http.Request)    { w.WriteHeader(http.StatusNotImplemented) }
```

`cmd/content-verify-api/main.go` (governor boot order: logger → config → tracing → redis → db → metrics → signal ctx → router → server → graceful shutdown):

```go
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/tracing"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/config"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("config load failed", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "content-verify")
	if err != nil {
		log.Warnw("tracing init failed", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("redis connect failed", "error", err)
	}
	defer redisCache.Close()

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("db connect failed", "error", err)
	}
	defer db.Close()

	collector := metrics.NewCollector("content-verify")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Worker + full handler wiring lands in Task 9.
	var h *handler.VerifyHandler

	router := transport.NewRouter(h, log, collector)
	srv := &http.Server{Addr: cfg.Server.Address(), Handler: router, ReadHeaderTimeout: 10 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	log.Infow("content-verify listening", "addr", cfg.Server.Address())
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalw("server error", "error", err)
	}
}
```

Mirror `services/governor/cmd/governor-api/main.go` for any signature drift (e.g. `metrics.NewCollector`, `tracing.InitFromEnv`) — governor is the authority.

- [ ] **Step 6: tidy + test + build.**

Run: `cd services/content-verify && go mod tidy && go test ./... && go build ./...`
Expected: config tests PASS, everything compiles.

- [ ] **Step 7: Commit**

```bash
git add services/content-verify go.work
git commit -m "feat(content-verify): service scaffold — config, metrics, router, main (:8101)"
```

---

### Task 2: Domain model + verdict store (JSONB)

**Files:**
- Create: `services/content-verify/internal/domain/verify.go`
- Create: `services/content-verify/internal/repo/store.go`
- Test: `services/content-verify/internal/domain/verify_test.go`, `services/content-verify/internal/repo/store_test.go`

**Interfaces:**
- Consumes: gorm.
- Produces: `domain.UnitKey{Team,Server,Category,Track}` + `.String()`; `domain.UnitVerdict{Key,Episode,Status,Audio,Hardsub,Softsubs,ProbedAt,Sample,Fails}`; `domain.AudioVerdict{Lang,Confidence,Verified}`; `domain.HardsubVerdict{Present,Lang,Confidence,Verified}`; `domain.ContentVerification{ID,AnimeID,Provider,Units,UpdatedAt}`; `domain.Summarize(units) ProviderSummary{Status,Raw,DubLangs,HardsubLangs}`; `repo.Store{Get,ByAnime,UpsertUnit}`; status constants `StatusVerified/StatusInconclusive/StatusUnreachable`.

- [ ] **Step 1: failing domain tests** (`internal/domain/verify_test.go`):

```go
package domain

import "testing"

func TestUnitKeyString(t *testing.T) {
	k := UnitKey{Server: "hd-1", Category: "sub"}
	if got := k.String(); got != "category=sub|server=hd-1" {
		t.Fatalf("key = %q", got)
	}
	if (UnitKey{Team: "610"}).String() != "team=610" {
		t.Fatal("team-only key wrong")
	}
}

func TestSummarize(t *testing.T) {
	units := []UnitVerdict{
		{Status: StatusVerified, Audio: &AudioVerdict{Lang: "ja", Confidence: 0.98, Verified: true}},
		{Status: StatusVerified, Audio: &AudioVerdict{Lang: "en", Confidence: 0.97, Verified: true},
			Hardsub: &HardsubVerdict{Present: true, Lang: "ru", Confidence: 0.96, Verified: true}},
		{Status: StatusInconclusive},
	}
	s := Summarize(units)
	if s.Status != "partial" || !s.Raw {
		t.Fatalf("summary = %+v", s)
	}
	if len(s.DubLangs) != 1 || s.DubLangs[0] != "en" {
		t.Fatalf("dub_langs = %v", s.DubLangs)
	}
	if len(s.HardsubLangs) != 1 || s.HardsubLangs[0] != "ru" {
		t.Fatalf("hardsub_langs = %v", s.HardsubLangs)
	}
	if Summarize(nil).Status != "unverified" {
		t.Fatal("empty must be unverified")
	}
	all := []UnitVerdict{{Status: StatusVerified, Audio: &AudioVerdict{Lang: "en", Verified: true}}}
	if Summarize(all).Status != "verified" {
		t.Fatal("all-verified must be verified")
	}
}
```

- [ ] **Step 2: run to verify FAIL** — `cd services/content-verify && go test ./internal/domain/` → compile error (types undefined).

- [ ] **Step 3: implement** `internal/domain/verify.go`:

```go
// Package domain holds the content-verify verdict model. One row per
// (anime × provider); the provider's internal structure (teams / servers /
// tracks) lives in the Units JSONB column, one UnitVerdict per probe unit.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

const (
	StatusVerified     = "verified"
	StatusInconclusive = "inconclusive"
	StatusUnreachable  = "unreachable"

	// VerifiedThreshold is the owner-specified confidence gate (spec §3).
	VerifiedThreshold = 0.95
)

// UnitKey identifies one probeable unit inside a provider. Exactly the
// fields that apply are set: Kodik → Team (+Category claim), scraper →
// Server+Category, animejoy legs → Server=provider, ae → Track.
type UnitKey struct {
	Team     string `json:"team,omitempty"`
	Server   string `json:"server,omitempty"`
	Category string `json:"category,omitempty"`
	Track    string `json:"track,omitempty"`
}

// String is the canonical map key: sorted k=v joined by "|".
func (k UnitKey) String() string {
	parts := make([]string, 0, 4)
	if k.Category != "" {
		parts = append(parts, "category="+k.Category)
	}
	if k.Server != "" {
		parts = append(parts, "server="+k.Server)
	}
	if k.Team != "" {
		parts = append(parts, "team="+k.Team)
	}
	if k.Track != "" {
		parts = append(parts, "track="+k.Track)
	}
	sort.Strings(parts)
	return strings.Join(parts, "|")
}

type AudioVerdict struct {
	Lang       string  `json:"lang,omitempty"` // en|ru|ja|other
	Confidence float64 `json:"confidence"`
	Verified   bool    `json:"verified"`
}

type HardsubVerdict struct {
	Present    bool    `json:"present"`
	Lang       string  `json:"lang,omitempty"`
	Confidence float64 `json:"confidence"`
	Verified   bool    `json:"verified"`
}

type SoftTrack struct {
	Lang string `json:"lang,omitempty"`
	Kind string `json:"kind,omitempty"`
}

type SampleInfo struct {
	Fragments     int     `json:"fragments"`
	SpeechSeconds float64 `json:"speech_seconds"`
}

type UnitVerdict struct {
	Key      UnitKey         `json:"key"`
	Episode  int             `json:"episode"`
	Status   string          `json:"status"`
	Audio    *AudioVerdict   `json:"audio,omitempty"`
	Hardsub  *HardsubVerdict `json:"hardsub,omitempty"`
	Softsubs []SoftTrack     `json:"softsubs,omitempty"`
	ProbedAt time.Time       `json:"probed_at"`
	Sample   SampleInfo      `json:"sample"`
	Fails    int             `json:"fails,omitempty"` // consecutive unreachable count → backoff
}

// UnitList serializes as JSON for the jsonb column (works on postgres and
// the pure-Go sqlite driver used in tests).
type UnitList []UnitVerdict

func (u UnitList) Value() (driver.Value, error) { return json.Marshal(u) }

func (u *UnitList) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*u = nil
		return nil
	case []byte:
		return json.Unmarshal(v, u)
	case string:
		return json.Unmarshal([]byte(v), u)
	}
	return fmt.Errorf("unitlist: unsupported scan type %T", src)
}

type ContentVerification struct {
	ID        string    `gorm:"type:uuid;primaryKey;default:gen_random_uuid()" json:"id"`
	AnimeID   string    `gorm:"type:uuid;uniqueIndex:idx_cv_anime_provider" json:"anime_id"`
	Provider  string    `gorm:"size:64;uniqueIndex:idx_cv_anime_provider" json:"provider"`
	Units     UnitList  `gorm:"type:jsonb" json:"units"`
	UpdatedAt time.Time `json:"updated_at"`
}

// BeforeCreate fills the UUID app-side so sqlite tests (no gen_random_uuid)
// behave like postgres.
func (c *ContentVerification) BeforeCreate(*gorm.DB) error {
	if c.ID == "" {
		c.ID = uuid.NewString()
	}
	return nil
}

// ProviderSummary is the wire rollup consumed by catalog + aePlayer.
type ProviderSummary struct {
	Status       string   `json:"status"` // unverified|partial|verified
	Raw          bool     `json:"raw"`
	DubLangs     []string `json:"dub_langs"`
	HardsubLangs []string `json:"hardsub_langs"`
}

func Summarize(units []UnitVerdict) ProviderSummary {
	s := ProviderSummary{Status: "unverified", DubLangs: []string{}, HardsubLangs: []string{}}
	if len(units) == 0 {
		return s
	}
	dub := map[string]bool{}
	hs := map[string]bool{}
	verified := 0
	for _, u := range units {
		if u.Status == StatusVerified {
			verified++
		}
		if u.Audio != nil && u.Audio.Verified {
			if u.Audio.Lang == "ja" {
				s.Raw = true
			} else if u.Audio.Lang == "en" || u.Audio.Lang == "ru" {
				dub[u.Audio.Lang] = true
			}
		}
		if u.Hardsub != nil && u.Hardsub.Verified && u.Hardsub.Present && u.Hardsub.Lang != "" {
			hs[u.Hardsub.Lang] = true
		}
	}
	for l := range dub {
		s.DubLangs = append(s.DubLangs, l)
	}
	for l := range hs {
		s.HardsubLangs = append(s.HardsubLangs, l)
	}
	sort.Strings(s.DubLangs)
	sort.Strings(s.HardsubLangs)
	switch {
	case verified == len(units):
		s.Status = "verified"
	case verified > 0 || s.Raw || len(s.DubLangs) > 0:
		s.Status = "partial"
	}
	return s
}
```

- [ ] **Step 4: run domain tests** — `go test ./internal/domain/` → PASS.

- [ ] **Step 5: failing store test** (`internal/repo/store_test.go`) — pure-Go sqlite (`github.com/glebarez/sqlite`, add to go.mod via tidy):

```go
package repo

import (
	"context"
	"testing"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&domain.ContentVerification{}); err != nil {
		t.Fatal(err)
	}
	return db
}

func TestUpsertUnitAndReadBack(t *testing.T) {
	s := NewStore(testDB(t))
	ctx := context.Background()
	v := domain.UnitVerdict{Key: domain.UnitKey{Server: "hd-1", Category: "sub"}, Episode: 12,
		Status: domain.StatusVerified, Audio: &domain.AudioVerdict{Lang: "en", Confidence: 0.97, Verified: true}}
	if err := s.UpsertUnit(ctx, "a-1", "gogoanime", v); err != nil {
		t.Fatal(err)
	}
	// Same key again → replace, not append.
	v.Audio.Lang = "ru"
	if err := s.UpsertUnit(ctx, "a-1", "gogoanime", v); err != nil {
		t.Fatal(err)
	}
	// Different key → append.
	v2 := domain.UnitVerdict{Key: domain.UnitKey{Server: "hd-2", Category: "dub"}, Status: domain.StatusInconclusive}
	if err := s.UpsertUnit(ctx, "a-1", "gogoanime", v2); err != nil {
		t.Fatal(err)
	}
	rows, err := s.ByAnime(ctx, "a-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || len(rows[0].Units) != 2 {
		t.Fatalf("rows=%d units=%v", len(rows), rows)
	}
	if rows[0].Units[0].Audio.Lang != "ru" {
		t.Fatalf("replace failed: %+v", rows[0].Units[0])
	}
	got, err := s.Get(ctx, "a-1", "gogoanime")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if miss, err := s.Get(ctx, "a-1", "nineanime"); err != nil || miss != nil {
		t.Fatalf("miss must be nil,nil: %v %v", miss, err)
	}
}
```

- [ ] **Step 6: run to verify FAIL**, then implement `internal/repo/store.go`:

```go
// Package repo persists content verifications. Single-writer by design (one
// worker goroutine), so read-modify-write on the Units JSON needs no locking.
package repo

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

type Store struct{ db *gorm.DB }

func NewStore(db *gorm.DB) *Store { return &Store{db: db} }

func (s *Store) Get(ctx context.Context, animeID, provider string) (*domain.ContentVerification, error) {
	var row domain.ContentVerification
	err := s.db.WithContext(ctx).
		Where("anime_id = ? AND provider = ?", animeID, provider).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (s *Store) ByAnime(ctx context.Context, animeID string) ([]domain.ContentVerification, error) {
	var rows []domain.ContentVerification
	err := s.db.WithContext(ctx).Where("anime_id = ?", animeID).Order("provider").Find(&rows).Error
	return rows, err
}

// UpsertUnit merges one unit verdict into the (anime, provider) row,
// replacing an existing unit with the same canonical key.
func (s *Store) UpsertUnit(ctx context.Context, animeID, provider string, v domain.UnitVerdict) error {
	row, err := s.Get(ctx, animeID, provider)
	if err != nil {
		return err
	}
	if row == nil {
		row = &domain.ContentVerification{AnimeID: animeID, Provider: provider}
	}
	key := v.Key.String()
	replaced := false
	for i := range row.Units {
		if row.Units[i].Key.String() == key {
			row.Units[i] = v
			replaced = true
			break
		}
	}
	if !replaced {
		row.Units = append(row.Units, v)
	}
	row.UpdatedAt = time.Now().UTC()
	return s.db.WithContext(ctx).Save(row).Error
}
```

- [ ] **Step 7: tidy + test** — `go mod tidy && go test ./...` → PASS. Register AutoMigrate in `cmd/content-verify-api/main.go` after `database.New`:

```go
	if err := db.AutoMigrate(&domain.ContentVerification{}); err != nil {
		log.Fatalw("automigrate failed", "error", err)
	}
```

(add the `internal/domain` import).

- [ ] **Step 8: Commit** — `git add services/content-verify && git commit -m "feat(content-verify): verdict domain model + JSONB store"`

---

### Task 3: Redis signals (visits, visited-index, cooldowns)

**Files:**
- Create: `services/content-verify/internal/signals/signals.go`
- Test: `services/content-verify/internal/signals/signals_test.go` (miniredis)

**Interfaces:**
- Consumes: `*redis.Client` (`redisCache.Client()` from libs/cache).
- Produces: `signals.Signals` with `RecordVisit(ctx, animeID, visitor string) error`, `UniqueVisitors(ctx, animeID string) int`, `VisitedAnime(ctx) []string`, `InCooldown(ctx, animeID string) bool`, `SetCooldown(ctx, animeID string, ttl time.Duration)`. Keys: `cv:visit:{anime}:{yyyymmdd}` (SET of visitor keys, TTL 8d), `cv:visited:{yyyymmdd}` (SET of animeIDs, TTL 8d), `cv:cooldown:{anime}` (string, TTL = cooldown).

- [ ] **Step 1: failing test** (`internal/signals/signals_test.go`; `go get github.com/alicebob/miniredis/v2` happens via tidy):

```go
package signals

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func testSignals(t *testing.T) (*Signals, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	s := New(rdb)
	s.now = func() time.Time { return time.Date(2026, 7, 16, 12, 0, 0, 0, time.UTC) }
	return s, mr
}

func TestVisitDedupAndWindow(t *testing.T) {
	s, _ := testSignals(t)
	ctx := context.Background()
	_ = s.RecordVisit(ctx, "a-1", "u:alice")
	_ = s.RecordVisit(ctx, "a-1", "u:alice") // same day dedup
	_ = s.RecordVisit(ctx, "a-1", "u:bob")
	if n := s.UniqueVisitors(ctx, "a-1"); n != 2 {
		t.Fatalf("visitors = %d, want 2", n)
	}
	// Same visitor on another day still counts ONCE across the window.
	s.now = func() time.Time { return time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC) }
	_ = s.RecordVisit(ctx, "a-1", "u:alice")
	if n := s.UniqueVisitors(ctx, "a-1"); n != 2 {
		t.Fatalf("cross-day visitors = %d, want 2 (union, not sum)", n)
	}
	visited := s.VisitedAnime(ctx)
	if len(visited) != 1 || visited[0] != "a-1" {
		t.Fatalf("visited = %v", visited)
	}
}

func TestCooldown(t *testing.T) {
	s, mr := testSignals(t)
	ctx := context.Background()
	if s.InCooldown(ctx, "a-1") {
		t.Fatal("fresh anime must not be cooling")
	}
	s.SetCooldown(ctx, "a-1", time.Hour)
	if !s.InCooldown(ctx, "a-1") {
		t.Fatal("cooldown not set")
	}
	mr.FastForward(2 * time.Hour)
	if s.InCooldown(ctx, "a-1") {
		t.Fatal("cooldown must expire")
	}
}
```

- [ ] **Step 2: run to verify FAIL**, implement `internal/signals/signals.go`:

```go
// Package signals stores the dynamic-priority inputs in Redis: per-day
// unique-visitor sets (+15 each), a visited-anime index for queue
// membership, and per-anime claim cooldowns. All reads fail open (errors →
// zero signal) — the queue must keep working through a Redis blip.
package signals

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	dayFormat  = "20060102"
	windowDays = 7
	signalTTL  = 8 * 24 * time.Hour
)

type Signals struct {
	rdb *redis.Client
	now func() time.Time
}

func New(rdb *redis.Client) *Signals { return &Signals{rdb: rdb, now: time.Now} }

func visitKey(animeID, day string) string { return fmt.Sprintf("cv:visit:%s:%s", animeID, day) }
func visitedKey(day string) string        { return "cv:visited:" + day }
func cooldownKey(animeID string) string   { return "cv:cooldown:" + animeID }

func (s *Signals) dayKeys(build func(day string) string) []string {
	keys := make([]string, 0, windowDays)
	for i := 0; i < windowDays; i++ {
		keys = append(keys, build(s.now().UTC().AddDate(0, 0, -i).Format(dayFormat)))
	}
	return keys
}

func (s *Signals) RecordVisit(ctx context.Context, animeID, visitor string) error {
	day := s.now().UTC().Format(dayFormat)
	pipe := s.rdb.Pipeline()
	pipe.SAdd(ctx, visitKey(animeID, day), visitor)
	pipe.Expire(ctx, visitKey(animeID, day), signalTTL)
	pipe.SAdd(ctx, visitedKey(day), animeID)
	pipe.Expire(ctx, visitedKey(day), signalTTL)
	_, err := pipe.Exec(ctx)
	return err
}

// UniqueVisitors counts DISTINCT visitors across the 7-day window (union,
// not sum — a daily returnee is one person, not seven).
func (s *Signals) UniqueVisitors(ctx context.Context, animeID string) int {
	members, err := s.rdb.SUnion(ctx, s.dayKeys(func(d string) string { return visitKey(animeID, d) })...).Result()
	if err != nil {
		return 0
	}
	return len(members)
}

func (s *Signals) VisitedAnime(ctx context.Context) []string {
	members, err := s.rdb.SUnion(ctx, s.dayKeys(visitedKey)...).Result()
	if err != nil {
		return nil
	}
	return members
}

func (s *Signals) InCooldown(ctx context.Context, animeID string) bool {
	n, err := s.rdb.Exists(ctx, cooldownKey(animeID)).Result()
	return err == nil && n > 0
}

func (s *Signals) SetCooldown(ctx context.Context, animeID string, ttl time.Duration) {
	if ttl <= 0 {
		return
	}
	_ = s.rdb.Set(ctx, cooldownKey(animeID), "1", ttl).Err()
}
```

- [ ] **Step 3: tidy + test** — `go mod tidy && go test ./internal/signals/` → PASS.

- [ ] **Step 4: Commit** — `git add services/content-verify && git commit -m "feat(content-verify): redis visit/cooldown signals with 7d unique-visitor window"`

---

### Task 4: Catalog — internal membership endpoint

**Files:**
- Modify: `services/catalog/internal/repo/anime.go` (append method; receiver is `*AnimeRepository`)
- Create: `services/catalog/internal/handler/internal_verify.go`
- Modify: `services/catalog/internal/transport/router.go` (param + route beside `/internal/probe/ae-targets`, line ~110)
- Modify: `services/catalog/cmd/catalog-api/main.go` (construct + pass; NewRouter call at ~line 704)
- Test: `services/catalog/internal/handler/internal_verify_test.go`

**Interfaces:**
- Produces: `GET /internal/verify/membership?ongoing_limit=500&top_limit=100` → `{"success":true,"data":{"ongoing":[{"id","name","episodes_aired"}],"top":[{"id","name","episodes_aired"}]}}` (httputil.OK envelope; Docker-network-only — `/internal/*` is never gateway-proxied).
- Consumes: `domain.Anime` fields `ID/Name/EpisodesAired/Status/Hidden/SortPriority/Score`.

- [ ] **Step 1: repo method.** Append to `services/catalog/internal/repo/anime.go`:

```go
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
```

(If `Hidden` is not nullable in the struct, keep the `(hidden = ? OR hidden IS NULL)` form anyway — it matches `GetOngoingAnime` at ~line 460.)

- [ ] **Step 2: failing handler test** (`internal_verify_test.go`, package `handler`):

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

type stubMembership struct{}

func (stubMembership) ListVerifyMembership(_ context.Context, _, _ int) ([]repo.VerifyMembershipRow, []repo.VerifyMembershipRow, error) {
	return []repo.VerifyMembershipRow{{ID: "o1", Name: "Frieren", EpisodesAired: 28}},
		[]repo.VerifyMembershipRow{{ID: "t1", Name: "NANA", EpisodesAired: 47}}, nil
}

func TestVerifyMembership(t *testing.T) {
	h := NewInternalVerifyHandler(stubMembership{}, nil)
	rec := httptest.NewRecorder()
	h.Membership(rec, httptest.NewRequest("GET", "/internal/verify/membership", nil))
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var env struct {
		Data struct {
			Ongoing []repo.VerifyMembershipRow `json:"ongoing"`
			Top     []repo.VerifyMembershipRow `json:"top"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if len(env.Data.Ongoing) != 1 || env.Data.Ongoing[0].EpisodesAired != 28 || len(env.Data.Top) != 1 {
		t.Fatalf("body: %s", rec.Body.String())
	}
}
```

- [ ] **Step 3: run to verify FAIL** — `cd services/catalog && go test ./internal/handler/ -run TestVerifyMembership` → compile error.

- [ ] **Step 4: handler** `internal/handler/internal_verify.go` (mirror `internal_probe.go` shape):

```go
package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/repo"
)

// verifyMembershipSource is the slice of AnimeRepository the handler needs.
type verifyMembershipSource interface {
	ListVerifyMembership(ctx context.Context, ongoingLimit, topLimit int) ([]repo.VerifyMembershipRow, []repo.VerifyMembershipRow, error)
}

// InternalVerifyHandler serves the content-verify queue membership.
// Docker-network-only: /internal/* is never proxied by the gateway.
type InternalVerifyHandler struct {
	src verifyMembershipSource
	log *logger.Logger
}

func NewInternalVerifyHandler(src verifyMembershipSource, log *logger.Logger) *InternalVerifyHandler {
	return &InternalVerifyHandler{src: src, log: log}
}

type verifyMembershipResponse struct {
	Ongoing []repo.VerifyMembershipRow `json:"ongoing"`
	Top     []repo.VerifyMembershipRow `json:"top"`
}

func (h *InternalVerifyHandler) Membership(w http.ResponseWriter, r *http.Request) {
	ongoingLimit := queryInt(r, "ongoing_limit", 500, 1, 2000)
	topLimit := queryInt(r, "top_limit", 100, 1, 500)
	ongoing, top, err := h.src.ListVerifyMembership(r.Context(), ongoingLimit, topLimit)
	if err != nil {
		if h.log != nil {
			h.log.Errorw("verify membership query failed", "error", err)
		}
		http.Error(w, "membership query failed", http.StatusInternalServerError)
		return
	}
	if ongoing == nil {
		ongoing = []repo.VerifyMembershipRow{}
	}
	if top == nil {
		top = []repo.VerifyMembershipRow{}
	}
	httputil.OK(w, verifyMembershipResponse{Ongoing: ongoing, Top: top})
}

func queryInt(r *http.Request, key string, def, min, max int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= min && n <= max {
			return n
		}
	}
	return def
}
```

(If a `queryInt`-like helper already exists in package `handler`, reuse it and drop this one.)

- [ ] **Step 5: wire router + main.** In `transport/router.go`: add `internalVerifyHandler *handler.InternalVerifyHandler` to the `NewRouter` params (next to `internalProbeHandler`, ~line 30) and register beside the other internal routes (~line 110):

```go
	if internalVerifyHandler != nil {
		r.Get("/internal/verify/membership", internalVerifyHandler.Membership)
	}
```

In `cmd/catalog-api/main.go`: `internalVerifyHandler := handler.NewInternalVerifyHandler(animeRepo, log)` near `internalProbeHandler` (~line 579), and add the argument to the `transport.NewRouter(...)` call (~line 704) in the matching position.

- [ ] **Step 6: test + build** — `cd services/catalog && go test ./internal/handler/ -run TestVerifyMembership && go build ./...` → PASS.

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/repo/anime.go services/catalog/internal/handler/internal_verify.go services/catalog/internal/handler/internal_verify_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): internal verify membership endpoint (ongoing + top-100)"
```

---

### Task 5: content-verify catalog client + unit enumeration

**Files:**
- Create: `services/content-verify/internal/catalogclient/client.go`
- Create: `services/content-verify/internal/queue/enumerate.go`
- Test: `services/content-verify/internal/catalogclient/client_test.go`, `services/content-verify/internal/queue/enumerate_test.go`

**Interfaces:**
- Consumes: catalog endpoints — `/internal/verify/membership` (Task 4), `/api/anime/{id}/capabilities`, `/api/anime/{id}/kodik/translations`, `/api/anime/{id}/kodik/stream?episode=&translation=`, `/api/anime/{id}/scraper/episodes|servers|stream?prefer=&exclusive=true`, `/api/anime/{id}/{animejoy-leg}/episodes|stream?episode=`. All responses use the `{"success":true,"data":{...}}` envelope.
- Produces (used by Tasks 6/8/9):

```go
type Client struct{ ... }
func New(catalogURL string, hc *http.Client) *Client                      // nil hc → 20s timeout
func (c *Client) Membership(ctx) (*Membership, error)                     // Membership{Ongoing, Top []MembershipRow{ID,Name,EpisodesAired}}
func (c *Client) Capabilities(ctx, animeID) ([]Cap, error)                // Cap{Provider, State, Group, Lang string; Audios []string}
func (c *Client) KodikTranslations(ctx, animeID) ([]KodikTranslation, error) // {ID int; Title, Type string}
func (c *Client) ScraperEpisodes(ctx, animeID, provider) ([]ScraperEpisode, error) // {ID string; Number int}; 404 → nil, nil
func (c *Client) ScraperServers(ctx, animeID, episodeID, provider) ([]ScraperServer, error) // {ID, Name, Type string}
func (c *Client) ScraperStream(ctx, animeID, episodeID, serverID, category, provider) (*Stream, error)
func (c *Client) KodikStream(ctx, animeID string, episode, translation int) (*Stream, error)
func (c *Client) AnimejoyEpisodes(ctx, animeID, provider) ([]int, error)
func (c *Client) AnimejoyStream(ctx, animeID, provider string, episode int) (*Stream, error)
type Stream struct{ URL, Exp, Sig, Referer, Type string; Intro, Outro *TimeRange; Tracks []TrackInfo }
type TimeRange struct{ Start, End int }
type TrackInfo struct{ File, Label, Kind string }
var ErrNotFound = errors.New(...)                                         // 404/empty translated
```

- `queue.EnumerateUnits(ctx, c *catalogclient.Client, animeID string) ([]Unit, error)`; `queue.Unit{AnimeID, Provider string; Key domain.UnitKey; Episode int; EpisodeID string; AeLang string; StateRank int}`.

- [ ] **Step 1: failing client test** — httptest server returning canned envelopes for each route; assert decode + 404→`nil,nil` for ScraperEpisodes + ErrNotFound for streams:

```go
package catalogclient

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func server(t *testing.T) *httptest.Server {
	mux := http.NewServeMux()
	mux.HandleFunc("/internal/verify/membership", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"ongoing":[{"id":"o1","name":"F","episodes_aired":28}],"top":[{"id":"t1","name":"N","episodes_aired":47}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/capabilities", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"anime_id":"a1","families":[{"family":"others","providers":[
			{"provider":"gogoanime","state":"active","group":"en","audios":["sub","dub"]},
			{"provider":"kodik","state":"active","group":"ru","audios":["sub","dub"]},
			{"provider":"hanime","state":"active","group":"adult","audios":["sub"]}]},
			{"family":"aeProvider","providers":[{"provider":"ae","state":"active","group":"firstparty","audios":["dub"],"lang":"en"}]}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/kodik/translations", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":[{"id":610,"title":"AniLibria","type":"voice","episodes_count":28},{"id":734,"title":"Subs","type":"subtitles","episodes_count":28}]}`))
	})
	mux.HandleFunc("/api/anime/a1/scraper/episodes", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("prefer") == "nineanime" {
			w.WriteHeader(404)
			return
		}
		w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep-1","number":1},{"id":"ep-28","number":28}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/scraper/servers", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"hd-1","name":"HD-1","type":"sub"},{"id":"hd-2","name":"HD-2","type":"dub"}]}}`))
	})
	mux.HandleFunc("/api/anime/a1/scraper/stream", func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"stream":{"headers":{"Referer":"https://x/"},"sources":[{"url":"https://cdn/x.m3u8","exp":"1","sig":"s","type":"hls"}],"tracks":[{"file":"a.vtt","label":"English","kind":"captions"}],"intro":{"start":90,"end":180}}}}`))
	})
	return httptest.NewServer(mux)
}

func TestClientDecodes(t *testing.T) {
	srv := server(t)
	defer srv.Close()
	c := New(srv.URL, srv.Client())
	ctx := context.Background()

	m, err := c.Membership(ctx)
	if err != nil || len(m.Ongoing) != 1 || m.Ongoing[0].EpisodesAired != 28 {
		t.Fatalf("membership: %+v %v", m, err)
	}
	caps, err := c.Capabilities(ctx, "a1")
	if err != nil || len(caps) != 4 {
		t.Fatalf("caps: %+v %v", caps, err)
	}
	tr, err := c.KodikTranslations(ctx, "a1")
	if err != nil || len(tr) != 2 || tr[0].ID != 610 || tr[1].Type != "subtitles" {
		t.Fatalf("translations: %+v %v", tr, err)
	}
	eps, err := c.ScraperEpisodes(ctx, "a1", "gogoanime")
	if err != nil || len(eps) != 2 {
		t.Fatalf("episodes: %+v %v", eps, err)
	}
	if eps, err := c.ScraperEpisodes(ctx, "a1", "nineanime"); err != nil || eps != nil {
		t.Fatalf("404 must be nil,nil: %v %v", eps, err)
	}
	srvs, err := c.ScraperServers(ctx, "a1", "ep-28", "gogoanime")
	if err != nil || len(srvs) != 2 || srvs[1].Type != "dub" {
		t.Fatalf("servers: %+v %v", srvs, err)
	}
	st, err := c.ScraperStream(ctx, "a1", "ep-28", "hd-1", "sub", "gogoanime")
	if err != nil || st.URL != "https://cdn/x.m3u8" || st.Referer != "https://x/" || st.Intro == nil || st.Intro.End != 180 {
		t.Fatalf("stream: %+v %v", st, err)
	}
}
```

- [ ] **Step 2: run to verify FAIL**, implement `internal/catalogclient/client.go`. Key mechanics (full decode types mirror the analytics probe's `envelope` — see `services/analytics/internal/probe/resolver.go`):

```go
// Package catalogclient talks to catalog: queue membership, capability
// structure, and per-provider stream resolution (scraper / kodik / animejoy
// legs). All scraper calls pass prefer=<provider>&exclusive=true so the
// probe result is attributable to exactly one provider.
package catalogclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var ErrNotFound = errors.New("catalogclient: not found")

type Client struct {
	base string
	hc   *http.Client
}

func New(catalogURL string, hc *http.Client) *Client {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{base: strings.TrimRight(catalogURL, "/"), hc: hc}
}

type MembershipRow struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	EpisodesAired int    `json:"episodes_aired"`
}
type Membership struct {
	Ongoing []MembershipRow `json:"ongoing"`
	Top     []MembershipRow `json:"top"`
}

type Cap struct {
	Provider string   `json:"provider"`
	State    string   `json:"state"`
	Group    string   `json:"group"`
	Lang     string   `json:"lang"`
	Audios   []string `json:"audios"`
}

type KodikTranslation struct {
	ID    int    `json:"id"`
	Title string `json:"title"`
	Type  string `json:"type"` // voice | subtitles (claim only — we verify)
}

type ScraperEpisode struct {
	ID     string `json:"id"`
	Number int    `json:"number"`
}
type ScraperServer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
}

type TimeRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}
type TrackInfo struct {
	File  string `json:"file"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}
type Stream struct {
	URL     string
	Exp     string
	Sig     string
	Referer string
	Type    string
	Intro   *TimeRange
	Outro   *TimeRange
	Tracks  []TrackInfo
}

// getJSON fetches u and decodes the {"success","data"} envelope into dst
// (dst receives the "data" value). 404 → ErrNotFound.
func (c *Client) getJSON(ctx context.Context, u string, dst any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return ErrNotFound
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("catalogclient: %s -> %d", u, resp.StatusCode)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return err
	}
	return json.Unmarshal(env.Data, dst)
}

func (c *Client) Membership(ctx context.Context) (*Membership, error) {
	var m Membership
	if err := c.getJSON(ctx, c.base+"/internal/verify/membership", &m); err != nil {
		return nil, err
	}
	return &m, nil
}

func (c *Client) Capabilities(ctx context.Context, animeID string) ([]Cap, error) {
	var data struct {
		Families []struct {
			Providers []Cap `json:"providers"`
		} `json:"families"`
	}
	if err := c.getJSON(ctx, c.base+"/api/anime/"+url.PathEscape(animeID)+"/capabilities", &data); err != nil {
		return nil, err
	}
	var caps []Cap
	for _, f := range data.Families {
		caps = append(caps, f.Providers...)
	}
	return caps, nil
}

func (c *Client) KodikTranslations(ctx context.Context, animeID string) ([]KodikTranslation, error) {
	var tr []KodikTranslation
	if err := c.getJSON(ctx, c.base+"/api/anime/"+url.PathEscape(animeID)+"/kodik/translations", &tr); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return tr, nil
}

func (c *Client) ScraperEpisodes(ctx context.Context, animeID, provider string) ([]ScraperEpisode, error) {
	var data struct {
		Episodes []ScraperEpisode `json:"episodes"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/scraper/episodes?prefer=%s&exclusive=true", c.base, url.PathEscape(animeID), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil // provider has no match — not an error
		}
		return nil, err
	}
	return data.Episodes, nil
}

func (c *Client) ScraperServers(ctx context.Context, animeID, episodeID, provider string) ([]ScraperServer, error) {
	var data struct {
		Servers []ScraperServer `json:"servers"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/scraper/servers?episode=%s&prefer=%s&exclusive=true",
		c.base, url.PathEscape(animeID), url.QueryEscape(episodeID), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
		return nil, err
	}
	return data.Servers, nil
}

func (c *Client) ScraperStream(ctx context.Context, animeID, episodeID, serverID, category, provider string) (*Stream, error) {
	var data struct {
		Stream struct {
			Headers map[string]string `json:"headers"`
			Sources []struct {
				URL  string `json:"url"`
				Exp  string `json:"exp"`
				Sig  string `json:"sig"`
				Type string `json:"type"`
			} `json:"sources"`
			Tracks []TrackInfo `json:"tracks"`
			Intro  *TimeRange  `json:"intro"`
			Outro  *TimeRange  `json:"outro"`
		} `json:"stream"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/scraper/stream?episode=%s&server=%s&category=%s&prefer=%s&exclusive=true",
		c.base, url.PathEscape(animeID), url.QueryEscape(episodeID), url.QueryEscape(serverID), url.QueryEscape(category), url.QueryEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
		return nil, err
	}
	if len(data.Stream.Sources) == 0 {
		return nil, ErrNotFound
	}
	src := data.Stream.Sources[0]
	return &Stream{URL: src.URL, Exp: src.Exp, Sig: src.Sig, Type: src.Type,
		Referer: data.Stream.Headers["Referer"], Tracks: data.Stream.Tracks,
		Intro: data.Stream.Intro, Outro: data.Stream.Outro}, nil
}

func (c *Client) KodikStream(ctx context.Context, animeID string, episode, translation int) (*Stream, error) {
	var data struct {
		StreamURL string `json:"stream_url"`
		Referer   string `json:"referer"`
		Exp       string `json:"exp"`
		Sig       string `json:"sig"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/kodik/stream?episode=%d&translation=%d", c.base, url.PathEscape(animeID), episode, translation)
	if err := c.getJSON(ctx, u, &data); err != nil {
		return nil, err
	}
	if data.StreamURL == "" {
		return nil, ErrNotFound
	}
	return &Stream{URL: data.StreamURL, Exp: data.Exp, Sig: data.Sig, Referer: data.Referer, Type: "hls"}, nil
}

func (c *Client) AnimejoyEpisodes(ctx context.Context, animeID, provider string) ([]int, error) {
	var data struct {
		Episodes []int `json:"episodes"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/%s/episodes", c.base, url.PathEscape(animeID), url.PathEscape(provider))
	if err := c.getJSON(ctx, u, &data); err != nil {
		if errors.Is(err, ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return data.Episodes, nil
}

func (c *Client) AnimejoyStream(ctx context.Context, animeID, provider string, episode int) (*Stream, error) {
	var data struct {
		URL     string `json:"url"`
		Referer string `json:"referer"`
		Exp     string `json:"exp"`
		Sig     string `json:"sig"`
	}
	u := fmt.Sprintf("%s/api/anime/%s/%s/stream?episode=%d", c.base, url.PathEscape(animeID), url.PathEscape(provider), episode)
	if err := c.getJSON(ctx, u, &data); err != nil {
		return nil, err
	}
	if data.URL == "" {
		return nil, ErrNotFound
	}
	return &Stream{URL: data.URL, Exp: data.Exp, Sig: data.Sig, Referer: data.Referer, Type: "mp4"}, nil
}
```

Authoritative wire shapes: `services/analytics/internal/probe/{resolver,kodiknoads,animejoy}.go` and `services/scraper/internal/domain/provider.go:91` (Stream/Track/TimeRange). If a field name differs there, THAT file wins.

- [ ] **Step 3: run client tests** — PASS.

- [ ] **Step 4: failing enumerate test** (`internal/queue/enumerate_test.go`) — reuse the Task-5 httptest server (export the test server builder into a shared `testutil_test.go` inside catalogclient, or duplicate the mux inline here):

```go
package queue

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
)

// buildTestCatalog: same mux as catalogclient tests (copy inline).

func TestEnumerateUnits(t *testing.T) {
	srv := buildTestCatalog(t)
	defer srv.Close()
	c := catalogclient.New(srv.URL, srv.Client())
	units, err := EnumerateUnits(context.Background(), c, "a1")
	if err != nil {
		t.Fatal(err)
	}
	// gogoanime: 2 servers → 2 units (episode = max number 28, EpisodeID ep-28);
	// kodik: 2 translations → 2 units; ae: 1 synth unit; hanime (adult): skipped.
	var gogo, kodik, ae, adult int
	for _, u := range units {
		switch u.Provider {
		case "gogoanime":
			gogo++
			if u.Episode != 28 || u.EpisodeID != "ep-28" {
				t.Fatalf("gogo unit episode: %+v", u)
			}
		case "kodik":
			kodik++
			if u.Key.Team == "" {
				t.Fatalf("kodik unit needs team key: %+v", u)
			}
		case "ae":
			ae++
			if u.AeLang != "en" {
				t.Fatalf("ae synth lang: %+v", u)
			}
		case "hanime":
			adult++
		}
	}
	if gogo != 2 || kodik != 2 || ae != 1 || adult != 0 {
		t.Fatalf("unit counts gogo=%d kodik=%d ae=%d adult=%d", gogo, kodik, ae, adult)
	}
}
```

- [ ] **Step 5: implement** `internal/queue/enumerate.go`:

```go
// Package queue computes the virtual content-verify queue: candidates,
// scores, unit enumeration, and the pending diff.
package queue

import (
	"context"
	"sort"
	"strconv"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// scraperProviders is the EN chain resolved via /scraper/* with prefer=.
var scraperProviders = map[string]bool{
	"gogoanime": true, "animepahe": true, "allanime-okru": true, "miruro": true, "nineanime": true,
}

func isAnimejoyLeg(p string) bool { return p == "animejoy-sibnet" || p == "animejoy-allvideo" }

// Unit is one probeable (anime × provider × internal-structure) tuple.
type Unit struct {
	AnimeID   string
	Provider  string
	Key       domain.UnitKey
	Episode   int    // sample episode (latest available on the provider)
	EpisodeID string // scraper episode id; "" for kodik/animejoy
	AeLang    string // non-empty → synthesize verified verdict, no probe
	StateRank int    // active=0 recovering=1 degraded=2 — probe order
}

func stateRank(s string) int {
	switch s {
	case "active":
		return 0
	case "recovering":
		return 1
	default:
		return 2
	}
}

// EnumerateUnits lists every probeable unit for one anime from live catalog
// structure. Adult providers are skipped in v1 (no membership source ranks
// them; a visited hentai title still gets its non-adult providers probed).
func EnumerateUnits(ctx context.Context, c *catalogclient.Client, animeID string) ([]Unit, error) {
	caps, err := c.Capabilities(ctx, animeID)
	if err != nil {
		return nil, err
	}
	var units []Unit
	for _, cap := range caps {
		if cap.State == "no_content" || cap.Group == "adult" {
			continue
		}
		rank := stateRank(cap.State)
		switch {
		case cap.Group == "firstparty":
			lang := cap.Lang
			if lang == "" {
				lang = "ja"
			}
			units = append(units, Unit{AnimeID: animeID, Provider: cap.Provider,
				Key: domain.UnitKey{Track: "default"}, AeLang: lang, StateRank: rank})

		case cap.Provider == "kodik":
			translations, err := c.KodikTranslations(ctx, animeID)
			if err != nil {
				continue // enumeration is best-effort per provider
			}
			for _, tr := range translations {
				cat := "dub"
				if tr.Type == "subtitles" {
					cat = "sub"
				}
				units = append(units, Unit{AnimeID: animeID, Provider: "kodik",
					Key:     domain.UnitKey{Team: strconv.Itoa(tr.ID), Category: cat},
					Episode: maxInt(tr.EpisodesCountOr(1), 1), StateRank: rank})
			}

		case scraperProviders[cap.Provider]:
			eps, err := c.ScraperEpisodes(ctx, animeID, cap.Provider)
			if err != nil || len(eps) == 0 {
				continue
			}
			latest := eps[0]
			for _, e := range eps {
				if e.Number > latest.Number {
					latest = e
				}
			}
			servers, err := c.ScraperServers(ctx, animeID, latest.ID, cap.Provider)
			if err != nil {
				continue
			}
			for _, s := range servers {
				cat := s.Type
				if cat != "dub" {
					cat = "sub"
				}
				units = append(units, Unit{AnimeID: animeID, Provider: cap.Provider,
					Key:     domain.UnitKey{Server: s.ID, Category: cat},
					Episode: latest.Number, EpisodeID: latest.ID, StateRank: rank})
			}

		case isAnimejoyLeg(cap.Provider):
			eps, err := c.AnimejoyEpisodes(ctx, animeID, cap.Provider)
			if err != nil || len(eps) == 0 {
				continue
			}
			latest := eps[0]
			for _, n := range eps {
				if n > latest {
					latest = n
				}
			}
			units = append(units, Unit{AnimeID: animeID, Provider: cap.Provider,
				Key: domain.UnitKey{Server: cap.Provider}, Episode: latest, StateRank: rank})
		}
	}
	sort.SliceStable(units, func(i, j int) bool { return units[i].StateRank < units[j].StateRank })
	return units, nil
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
```

Note: `KodikTranslation.EpisodesCountOr` does not exist — add `EpisodesCount int json:"episodes_count"` to the client struct and use `maxInt(tr.EpisodesCount, 1)` directly. (Kodik translations carry `episodes_count` — see `services/catalog/internal/domain/anime.go:268`.)

- [ ] **Step 6: run tests** — `go test ./internal/...` → PASS.

- [ ] **Step 7: Commit** — `git add services/content-verify && git commit -m "feat(content-verify): catalog client + per-provider unit enumeration"`

---

### Task 6: Queue engine — score, rank, pending diff, claim

**Files:**
- Create: `services/content-verify/internal/queue/queue.go`
- Create: `services/content-verify/internal/queue/engine.go`
- Test: `services/content-verify/internal/queue/queue_test.go`

**Interfaces:**
- Produces:

```go
type Candidate struct{ AnimeID, Name string; Ongoing, Top bool; Visitors, EpisodesAired int }
func (c Candidate) Score() int                       // 15*Visitors + 10*Ongoing + 5*Top
func BuildCandidates(m *catalogclient.Membership, visited []string, visitors func(string) int) []Candidate
func Rank(cs []Candidate) []Candidate                // score desc; tie: ongoing first, then AnimeID
func Backoff(fails int) time.Duration                // 6h·2^(fails-1), cap 7d
func UnitDue(u Unit, prev *domain.UnitVerdict, now time.Time, reprobeTTL time.Duration) bool
func PendingUnits(units []Unit, rows []domain.ContentVerification, now time.Time, ttl time.Duration) []Unit
func CooldownTTL(ongoing bool) time.Duration         // 6h ongoing / 24h other
type Engine struct{ ... }
func NewEngine(cat *catalogclient.Client, sig *signals.Signals, store *repo.Store, reprobeTTL time.Duration, log *logger.Logger) *Engine
func (e *Engine) Claim(ctx) (*Unit, bool, error)     // next unit to probe; bool = candidate is ongoing
func (e *Engine) Snapshot(ctx, limit int) []QueueEntry  // {AnimeID, Name, Score, Ongoing, Top, Visitors, Cooling}
```

- [ ] **Step 1: failing tests** (`queue_test.go`):

```go
package queue

import (
	"testing"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

func TestScoreAndRank(t *testing.T) {
	m := &catalogclient.Membership{
		Ongoing: []catalogclient.MembershipRow{{ID: "o1", Name: "F", EpisodesAired: 28}},
		Top:     []catalogclient.MembershipRow{{ID: "t1", Name: "N"}, {ID: "o1", Name: "F"}},
	}
	visitors := map[string]int{"v1": 2, "o1": 1}
	cs := BuildCandidates(m, []string{"v1"}, func(id string) int { return visitors[id] })
	ranked := Rank(cs)
	// o1: ongoing(10)+top(5)+15*1=30 ; v1: 15*2=30 ; t1: 5.
	// Tie 30/30 → ongoing first.
	if ranked[0].AnimeID != "o1" || ranked[0].Score() != 30 {
		t.Fatalf("ranked[0]=%+v score=%d", ranked[0], ranked[0].Score())
	}
	if ranked[1].AnimeID != "v1" || ranked[2].AnimeID != "t1" {
		t.Fatalf("order: %+v", ranked)
	}
}

func TestBackoff(t *testing.T) {
	if Backoff(1) != 6*time.Hour || Backoff(2) != 12*time.Hour {
		t.Fatal("backoff base wrong")
	}
	if Backoff(10) != 168*time.Hour {
		t.Fatalf("backoff cap: %s", Backoff(10))
	}
}

func TestUnitDue(t *testing.T) {
	now := time.Now()
	u := Unit{Episode: 28, Key: domain.UnitKey{Server: "hd-1", Category: "sub"}}
	if !UnitDue(u, nil, now, 720*time.Hour) {
		t.Fatal("never-probed must be due")
	}
	fresh := &domain.UnitVerdict{Episode: 28, Status: domain.StatusVerified, ProbedAt: now.Add(-time.Hour)}
	if UnitDue(u, fresh, now, 720*time.Hour) {
		t.Fatal("fresh verified must NOT be due")
	}
	oldEp := &domain.UnitVerdict{Episode: 27, Status: domain.StatusVerified, ProbedAt: now.Add(-time.Hour)}
	if !UnitDue(u, oldEp, now, 720*time.Hour) {
		t.Fatal("new episode must re-probe")
	}
	stale := &domain.UnitVerdict{Episode: 28, Status: domain.StatusVerified, ProbedAt: now.Add(-721 * time.Hour)}
	if !UnitDue(u, stale, now, 720*time.Hour) {
		t.Fatal("stale must re-probe")
	}
	failing := &domain.UnitVerdict{Episode: 28, Status: domain.StatusUnreachable, Fails: 1, ProbedAt: now.Add(-time.Hour)}
	if UnitDue(u, failing, now, 720*time.Hour) {
		t.Fatal("unreachable within backoff must wait")
	}
	failing.ProbedAt = now.Add(-7 * time.Hour)
	if !UnitDue(u, failing, now, 720*time.Hour) {
		t.Fatal("unreachable past backoff must retry")
	}
}
```

- [ ] **Step 2: run to verify FAIL**, implement `internal/queue/queue.go`:

```go
package queue

import (
	"sort"
	"time"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

// Owner-specified priority weights (spec §1).
const (
	weightVisitor = 15
	weightOngoing = 10
	weightTop     = 5

	backoffBase = 6 * time.Hour
	backoffCap  = 168 * time.Hour
)

type Candidate struct {
	AnimeID       string
	Name          string
	Ongoing       bool
	Top           bool
	Visitors      int
	EpisodesAired int
}

func (c Candidate) Score() int {
	s := weightVisitor * c.Visitors
	if c.Ongoing {
		s += weightOngoing
	}
	if c.Top {
		s += weightTop
	}
	return s
}

// BuildCandidates merges membership (ongoing ∪ top ∪ visited) and attaches
// the unique-visitor count to every candidate.
func BuildCandidates(m *catalogclient.Membership, visited []string, visitors func(string) int) []Candidate {
	byID := map[string]*Candidate{}
	add := func(id, name string, aired int) *Candidate {
		if c, ok := byID[id]; ok {
			if c.Name == "" {
				c.Name = name
			}
			if aired > c.EpisodesAired {
				c.EpisodesAired = aired
			}
			return c
		}
		c := &Candidate{AnimeID: id, Name: name, EpisodesAired: aired}
		byID[id] = c
		return c
	}
	if m != nil {
		for _, r := range m.Ongoing {
			add(r.ID, r.Name, r.EpisodesAired).Ongoing = true
		}
		for _, r := range m.Top {
			add(r.ID, r.Name, r.EpisodesAired).Top = true
		}
	}
	for _, id := range visited {
		add(id, "", 0)
	}
	out := make([]Candidate, 0, len(byID))
	for _, c := range byID {
		c.Visitors = visitors(c.AnimeID)
		out = append(out, *c)
	}
	return out
}

func Rank(cs []Candidate) []Candidate {
	sort.SliceStable(cs, func(i, j int) bool {
		si, sj := cs[i].Score(), cs[j].Score()
		if si != sj {
			return si > sj
		}
		if cs[i].Ongoing != cs[j].Ongoing {
			return cs[i].Ongoing
		}
		return cs[i].AnimeID < cs[j].AnimeID
	})
	return cs
}

func Backoff(fails int) time.Duration {
	if fails < 1 {
		fails = 1
	}
	d := backoffBase
	for i := 1; i < fails; i++ {
		d *= 2
		if d >= backoffCap {
			return backoffCap
		}
	}
	return d
}

// UnitDue decides whether a unit needs (re-)probing.
func UnitDue(u Unit, prev *domain.UnitVerdict, now time.Time, reprobeTTL time.Duration) bool {
	if prev == nil {
		return true
	}
	if u.Episode > prev.Episode {
		return true // a newer episode aired — the old sample is stale
	}
	if prev.Status == domain.StatusUnreachable {
		return now.After(prev.ProbedAt.Add(Backoff(prev.Fails)))
	}
	return now.After(prev.ProbedAt.Add(reprobeTTL))
}

// PendingUnits diffs live structure against stored verdicts, keeping probe
// order (StateRank from enumeration).
func PendingUnits(units []Unit, rows []domain.ContentVerification, now time.Time, ttl time.Duration) []Unit {
	prev := map[string]*domain.UnitVerdict{}
	for i := range rows {
		for j := range rows[i].Units {
			u := &rows[i].Units[j]
			prev[rows[i].Provider+"|"+u.Key.String()] = u
		}
	}
	var out []Unit
	for _, u := range units {
		if UnitDue(u, prev[u.Provider+"|"+u.Key.String()], now, ttl) {
			out = append(out, u)
		}
	}
	return out
}

func CooldownTTL(ongoing bool) time.Duration {
	if ongoing {
		return 6 * time.Hour // new episodes must surface same-day
	}
	return 24 * time.Hour
}
```

- [ ] **Step 3: implement engine** (`internal/queue/engine.go`):

```go
package queue

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/signals"
)

const (
	membershipTTL = 10 * time.Minute
	maxScan       = 15 // candidates inspected per claim tick
)

type Engine struct {
	cat        *catalogclient.Client
	sig        *signals.Signals
	store      *repo.Store
	reprobeTTL time.Duration
	log        *logger.Logger

	memb   *catalogclient.Membership
	membAt time.Time
	now    func() time.Time
}

func NewEngine(cat *catalogclient.Client, sig *signals.Signals, store *repo.Store, reprobeTTL time.Duration, log *logger.Logger) *Engine {
	return &Engine{cat: cat, sig: sig, store: store, reprobeTTL: reprobeTTL, log: log, now: time.Now}
}

func (e *Engine) membership(ctx context.Context) *catalogclient.Membership {
	if e.memb != nil && e.now().Sub(e.membAt) < membershipTTL {
		return e.memb
	}
	m, err := e.cat.Membership(ctx)
	if err != nil {
		if e.log != nil {
			e.log.Warnw("membership fetch failed; reusing stale", "error", err)
		}
		return e.memb // possibly nil — BuildCandidates tolerates it
	}
	e.memb, e.membAt = m, e.now()
	return m
}

func (e *Engine) ranked(ctx context.Context) []Candidate {
	m := e.membership(ctx)
	visited := e.sig.VisitedAnime(ctx)
	cs := Rank(BuildCandidates(m, visited, func(id string) int { return e.sig.UniqueVisitors(ctx, id) }))
	cvmetrics.QueueDepth.Set(float64(len(cs)))
	return cs
}

// Claim returns the single highest-priority pending unit, or (nil, false,
// nil) when the queue is idle. One unit per tick — hot titles preempt
// naturally between units of a slower title.
func (e *Engine) Claim(ctx context.Context) (*Unit, bool, error) {
	scanned := 0
	for _, cand := range e.ranked(ctx) {
		if scanned >= maxScan {
			break
		}
		if e.sig.InCooldown(ctx, cand.AnimeID) {
			continue
		}
		scanned++
		units, err := EnumerateUnits(ctx, e.cat, cand.AnimeID)
		if err != nil {
			if e.log != nil {
				e.log.Warnw("enumerate failed", "anime_id", cand.AnimeID, "error", err)
			}
			e.sig.SetCooldown(ctx, cand.AnimeID, time.Hour) // don't hammer a broken title
			continue
		}
		rows, err := e.store.ByAnime(ctx, cand.AnimeID)
		if err != nil {
			return nil, false, err
		}
		pending := PendingUnits(units, rows, e.now(), e.reprobeTTL)
		if len(pending) == 0 {
			e.sig.SetCooldown(ctx, cand.AnimeID, CooldownTTL(cand.Ongoing))
			continue
		}
		u := pending[0]
		return &u, cand.Ongoing, nil
	}
	return nil, false, nil
}

type QueueEntry struct {
	AnimeID  string `json:"anime_id"`
	Name     string `json:"name"`
	Score    int    `json:"score"`
	Ongoing  bool   `json:"ongoing"`
	Top      bool   `json:"top"`
	Visitors int    `json:"visitors"`
	Cooling  bool   `json:"cooling"`
}

// Snapshot renders the computed queue for the admin/debug endpoint.
func (e *Engine) Snapshot(ctx context.Context, limit int) []QueueEntry {
	out := []QueueEntry{}
	for i, c := range e.ranked(ctx) {
		if i >= limit {
			break
		}
		out = append(out, QueueEntry{AnimeID: c.AnimeID, Name: c.Name, Score: c.Score(),
			Ongoing: c.Ongoing, Top: c.Top, Visitors: c.Visitors,
			Cooling: e.sig.InCooldown(ctx, c.AnimeID)})
	}
	return out
}
```

- [ ] **Step 4: run tests** — `go test ./internal/queue/` → PASS (engine itself is covered by Task 9's worker test + live E2E; pure functions covered here).

- [ ] **Step 5: Commit** — `git add services/content-verify && git commit -m "feat(content-verify): virtual priority queue — score/rank/pending/claim"`

---

### Task 7: Python analyzers — lid.py + hardsub.py

**Files:**
- Create: `services/content-verify/analyzers/requirements.txt`
- Create: `services/content-verify/analyzers/lid.py`
- Create: `services/content-verify/analyzers/hardsub.py`

**Interfaces:**
- Produces (consumed by Go runner, Task 8): both scripts print ONE JSON object to stdout, exit 0 on success, non-zero + stderr message on failure.
  - `python3 lid.py <wav1> [wav2 ...]` → `{"fragments":[{"path":"...","lang":"en","prob":0.98,"speech_seconds":24.1,"speech":true}]}`
  - `python3 hardsub.py <frames_dir>` → `{"frames":15,"tier1_hits":6,"ocr_real":3,"script":"latin","text_stroke_p75":0.0071}`
- No CI test (needs whisper model + tesseract); contract is validated by the Go runner tests (fake scripts) in Task 8 and live in Task 17. A manual smoke command is given below.

- [ ] **Step 1: requirements.txt**

```
# faster-whisper: audio language ID (tiny int8 on CPU). Version pinned for
# reproducible ctranslate2 pairing.
faster-whisper==1.1.1
numpy>=1.26
Pillow>=10.0
```

- [ ] **Step 2: lid.py** — language-ID only, VAD-filtered, one JSON out:

```python
#!/usr/bin/env python3
"""Audio language ID for content-verify.

Usage: lid.py <wav1> [wav2 ...]
Prints JSON: {"fragments":[{"path","lang","prob","speech_seconds","speech"}]}

Model: faster-whisper tiny int8 (CPU). We only need detect-language, so we
transcribe with beam_size=1 and read TranscriptionInfo. vad_filter strips
music/silence; a fragment with <5s of speech after VAD is speech=false and
must not be trusted for LID.
"""
import json
import sys

MIN_SPEECH_SECONDS = 5.0


def main() -> int:
    wavs = sys.argv[1:]
    if not wavs:
        print("usage: lid.py <wav1> [wav2 ...]", file=sys.stderr)
        return 2
    from faster_whisper import WhisperModel  # deferred: import cost ~model load

    model = WhisperModel("tiny", device="cpu", compute_type="int8")
    fragments = []
    for path in wavs:
        try:
            _segments, info = model.transcribe(path, beam_size=1, vad_filter=True)
            speech_s = float(getattr(info, "duration_after_vad", 0.0) or 0.0)
            fragments.append({
                "path": path,
                "lang": info.language or "",
                "prob": float(info.language_probability or 0.0),
                "speech_seconds": speech_s,
                "speech": speech_s >= MIN_SPEECH_SECONDS,
            })
        except Exception as exc:  # one broken wav must not kill the batch
            print(f"lid: {path}: {exc}", file=sys.stderr)
            fragments.append({"path": path, "lang": "", "prob": 0.0,
                              "speech_seconds": 0.0, "speech": False})
    print(json.dumps({"fragments": fragments}))
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

- [ ] **Step 3: hardsub.py** — port of `tools/subprobe/analyze.py` tier1/tier2 + `verify_verdict.py` decision inputs (tools/subprobe stays untouched as the standalone diagnostic). Copy `luma/grad_mag/band_box/tier1/script_of/tier2` VERBATIM from `tools/subprobe/analyze.py` (constants `BAND_Y0/Y1=0.72/0.985`, `BAND_X0/X1=0.06/0.94`, `STROKE_T=0.005` from verify_verdict.py), then:

```python
#!/usr/bin/env python3
"""Burned-in subtitle detection for content-verify.

Usage: hardsub.py <frames_dir>
Prints JSON: {"frames","tier1_hits","ocr_real","script","text_stroke_p75"}

Port of tools/subprobe (tier1 pixel band heuristic + tesseract OCR script
ID). Aggregation uses hit counts / p75, not means — subs are intermittent.
"""
import glob
import json
import os
import sys

import numpy as np
from PIL import Image

# --- BEGIN verbatim copies from tools/subprobe/analyze.py ---
# BAND_* constants, luma(), grad_mag(), band_box(), tier1(), script_of(), tier2()
# --- END verbatim copies ---

STROKE_T = 0.005  # tools/subprobe/verify_verdict.py: hardsub >= ~0.006, clean < ~0.003


def main() -> int:
    if len(sys.argv) != 2:
        print("usage: hardsub.py <frames_dir>", file=sys.stderr)
        return 2
    frames_dir = sys.argv[1]
    paths = sorted(glob.glob(os.path.join(frames_dir, "*.png")) + glob.glob(os.path.join(frames_dir, "*.jpg")))
    if not paths:
        print(json.dumps({"frames": 0, "tier1_hits": 0, "ocr_real": 0, "script": "none", "text_stroke_p75": 0.0}))
        return 0
    strokes, hits = [], []
    for p in paths:
        img = Image.open(p).convert("RGB")
        arr = np.asarray(img)
        t1 = tier1(arr)
        strokes.append(t1["text_stroke"])
        if t1["text_stroke"] >= STROKE_T:
            hits.append((p, img))
    ocr_real = 0
    scripts = {}
    for p, img in hits:
        t2 = tier2(img)
        if t2["real_text"]:
            ocr_real += 1
            scripts[t2["script"]] = scripts.get(t2["script"], 0) + 1
    top_script = max(scripts, key=scripts.get) if scripts else "none"
    print(json.dumps({
        "frames": len(paths),
        "tier1_hits": len(hits),
        "ocr_real": ocr_real,
        "script": top_script,
        "text_stroke_p75": float(np.percentile(np.array(strokes), 75)) if strokes else 0.0,
    }))
    return 0


if __name__ == "__main__":
    sys.exit(main())
```

The executor MUST inline the real function bodies from `tools/subprobe/analyze.py` (they are self-contained: numpy+PIL+`subprocess.run(['tesseract', ...])`). Keep the 3× band upscale and `--psm 6` TSV parsing exactly.

- [ ] **Step 4: manual smoke (host has ffmpeg+tesseract+python3; faster-whisper NOT installed on host — lid smoke happens in-container at Task 17).**

Run: `python3 services/content-verify/analyzers/hardsub.py /tmp/nonexistent-dir 2>&1; echo "exit=$?"`
Expected: `usage:`-style behavior is NOT triggered (dir arg present) — prints `{"frames": 0, ...}` and `exit=0`.

- [ ] **Step 5: Commit** — `git add services/content-verify/analyzers && git commit -m "feat(content-verify): python analyzers — whisper LID + subprobe hardsub port"`

---

### Task 8: Prober — resolve → localize → ffmpeg → analyze → verdict

**Files:**
- Create: `services/content-verify/internal/prober/playlist.go`
- Create: `services/content-verify/internal/prober/extract.go`
- Create: `services/content-verify/internal/prober/runner.go`
- Create: `services/content-verify/internal/prober/assemble.go`
- Create: `services/content-verify/internal/prober/prober.go`
- Test: `services/content-verify/internal/prober/{playlist,assemble,prober}_test.go`

**Interfaces:**
- Consumes: `catalogclient.Client` (streams), `queue.Unit`, config paths.
- Produces:

```go
type LIDFragment struct{ Path, Lang string; Prob, SpeechSeconds float64; Speech bool }
type LIDResult struct{ Fragments []LIDFragment }
type HardsubResult struct{ Frames, Tier1Hits, OCRReal int; Script string; TextStrokeP75 float64 }
type AnalyzerRunner interface {
	LID(ctx context.Context, wavs []string) (*LIDResult, error)
	Hardsub(ctx context.Context, framesDir string) (*HardsubResult, error)
}
func NewExecRunner(python, analyzersDir string) AnalyzerRunner
func AssembleAudio(frs []LIDFragment) *domain.AudioVerdict
func AssembleHardsub(h *HardsubResult) *domain.HardsubVerdict
type Prober struct{ ... }
func New(cat *catalogclient.Client, gatewayURL, ffmpegPath, workDir string, runner AnalyzerRunner, log *logger.Logger) *Prober
func (p *Prober) Probe(ctx context.Context, u queue.Unit, prevFails int) domain.UnitVerdict  // never panics; errors → StatusUnreachable w/ Fails=prevFails+1
var ErrResolve = errors.New("prober: stream resolve failed")
```

- [ ] **Step 1: failing assemble tests** (`assemble_test.go`):

```go
package prober

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
)

func TestAssembleAudioVerified(t *testing.T) {
	frs := []LIDFragment{
		{Lang: "en", Prob: 0.99, Speech: true, SpeechSeconds: 24},
		{Lang: "en", Prob: 0.97, Speech: true, SpeechSeconds: 22},
		{Lang: "en", Prob: 0.95, Speech: true, SpeechSeconds: 25},
		{Lang: "ja", Prob: 0.40, Speech: false}, // non-speech ignored
	}
	v := AssembleAudio(frs)
	if v == nil || !v.Verified || v.Lang != "en" {
		t.Fatalf("verdict: %+v", v)
	}
	if v.Confidence < 0.95 {
		t.Fatalf("confidence: %f", v.Confidence)
	}
}

func TestAssembleAudioDisagreementNotVerified(t *testing.T) {
	frs := []LIDFragment{
		{Lang: "en", Prob: 0.99, Speech: true}, {Lang: "ru", Prob: 0.98, Speech: true}, {Lang: "en", Prob: 0.97, Speech: true},
	}
	if v := AssembleAudio(frs); v == nil || v.Verified {
		t.Fatalf("disagreement must not verify: %+v", v)
	}
}

func TestAssembleAudioTooFewSpeech(t *testing.T) {
	frs := []LIDFragment{{Lang: "en", Prob: 0.99, Speech: true}, {Lang: "en", Prob: 0.99, Speech: true}}
	if v := AssembleAudio(frs); v != nil && v.Verified {
		t.Fatal("2 speech fragments must not verify")
	}
}

func TestAssembleHardsub(t *testing.T) {
	h := &HardsubResult{Frames: 15, Tier1Hits: 6, OCRReal: 3, Script: "cyrillic"}
	v := AssembleHardsub(h)
	if !v.Present || !v.Verified || v.Lang != "ru" || v.Confidence < 0.95 {
		t.Fatalf("verdict: %+v", v)
	}
	clean := AssembleHardsub(&HardsubResult{Frames: 15, Tier1Hits: 0, OCRReal: 0, Script: "none"})
	if clean.Present || clean.Verified {
		t.Fatalf("clean must be present=false, verified=false (negative claims are never badged): %+v", clean)
	}
	weak := AssembleHardsub(&HardsubResult{Frames: 15, Tier1Hits: 3, OCRReal: 1, Script: "latin"})
	if !weak.Present || weak.Verified {
		t.Fatalf("1 OCR hit → present but NOT verified: %+v", weak)
	}
}
```

- [ ] **Step 2: implement assemble.go:**

```go
package prober

import "github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"

// LID / Hardsub result types — the JSON contracts of analyzers/lid.py and
// analyzers/hardsub.py.
type LIDFragment struct {
	Path          string  `json:"path"`
	Lang          string  `json:"lang"`
	Prob          float64 `json:"prob"`
	SpeechSeconds float64 `json:"speech_seconds"`
	Speech        bool    `json:"speech"`
}
type LIDResult struct {
	Fragments []LIDFragment `json:"fragments"`
}
type HardsubResult struct {
	Frames        int     `json:"frames"`
	Tier1Hits     int     `json:"tier1_hits"`
	OCRReal       int     `json:"ocr_real"`
	Script        string  `json:"script"`
	TextStrokeP75 float64 `json:"text_stroke_p75"`
}

const minSpeechFragments = 3

// AssembleAudio: all speech fragments agree AND mean prob ≥ threshold →
// verified (spec §3). Whisper returns ISO-639-1 codes (en/ru/ja) directly.
func AssembleAudio(frs []LIDFragment) *domain.AudioVerdict {
	var speech []LIDFragment
	for _, f := range frs {
		if f.Speech && f.Lang != "" {
			speech = append(speech, f)
		}
	}
	if len(speech) == 0 {
		return nil
	}
	lang := speech[0].Lang
	agree := true
	sum := 0.0
	for _, f := range speech {
		if f.Lang != lang {
			agree = false
		}
		sum += f.Prob
	}
	mean := sum / float64(len(speech))
	v := &domain.AudioVerdict{Lang: lang, Confidence: mean}
	if !agree {
		// Disagreement: report the majority-first lang but cap confidence.
		v.Confidence = mean * 0.5
		return v
	}
	v.Verified = len(speech) >= minSpeechFragments && mean >= domain.VerifiedThreshold
	return v
}

// AssembleHardsub applies tools/subprobe's decision rule (verify_verdict.py):
// burned = tier1_hits >= max(2, frames/5) AND ocr_real >= 1. Verified
// (badge-grade) additionally needs ≥2 OCR confirmations + a known script.
func AssembleHardsub(h *HardsubResult) *domain.HardsubVerdict {
	if h == nil || h.Frames == 0 {
		return nil
	}
	minHits := h.Frames / 5
	if minHits < 2 {
		minHits = 2
	}
	present := h.Tier1Hits >= minHits && h.OCRReal >= 1
	v := &domain.HardsubVerdict{Present: present}
	if !present {
		v.Confidence = 0.9 // "looks clean" — informational, never badged
		return v
	}
	switch h.Script {
	case "cyrillic":
		v.Lang = "ru"
	case "latin":
		v.Lang = "en"
	case "cjk":
		v.Lang = "ja"
	}
	if h.OCRReal >= 2 && v.Lang != "" {
		v.Confidence = 0.96
		v.Verified = true
	} else {
		v.Confidence = 0.8
	}
	return v
}
```

- [ ] **Step 3: run assemble tests** — PASS.

- [ ] **Step 4: playlist.go + failing test.** Proxied URL + HLS localization (verify_provider.sh flow, in Go):

```go
package prober

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const publicProxyPath = "/api/streaming/hls-proxy"

// ProxiedURL builds the gateway hls-proxy URL for a resolved stream. A URL
// already pointing at the proxy (masked /m/ or hls-proxy paths) is only
// re-based onto the gateway.
func ProxiedURL(gatewayBase string, rawURL, exp, sig, referer string) string {
	base := strings.TrimRight(gatewayBase, "/")
	if strings.HasPrefix(rawURL, "/api/streaming/") || strings.HasPrefix(rawURL, "/api/v1/") {
		p := strings.Replace(rawURL, "/api/v1/", "/api/streaming/", 1)
		return base + p
	}
	q := url.Values{"url": {rawURL}}
	if exp != "" {
		q.Set("exp", exp)
	}
	if sig != "" {
		q.Set("sig", sig)
	}
	if referer != "" {
		q.Set("referer", referer)
	}
	return base + publicProxyPath + "?" + q.Encode()
}

var extinfRe = regexp.MustCompile(`#EXTINF:([0-9.]+)`)
var uriAttrRe = regexp.MustCompile(`URI="([^"]+)"`)

// LocalizeHLS downloads master → first variant → media playlist through the
// proxy, absolutizes every URI against the gateway (segments AND EXT-X-KEY
// URIs — AES key fetches must also ride the proxy), writes a local .m3u8,
// and returns its path plus the summed EXTINF duration.
func LocalizeHLS(ctx context.Context, hc *http.Client, gatewayBase, masterURL, dir string) (string, float64, error) {
	body, err := fetch(ctx, hc, masterURL)
	if err != nil {
		return "", 0, err
	}
	media := body
	mediaURL := masterURL
	if !strings.Contains(body, "#EXTINF") { // master playlist → hop to first variant
		variant := firstNonComment(body)
		if variant == "" {
			return "", 0, fmt.Errorf("empty master playlist")
		}
		mediaURL = absolutize(gatewayBase, masterURL, variant)
		media, err = fetch(ctx, hc, mediaURL)
		if err != nil {
			return "", 0, err
		}
	}
	var out []string
	var duration float64
	for _, line := range strings.Split(media, "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "#"):
			if m := extinfRe.FindStringSubmatch(trimmed); m != nil {
				if d, err := strconv.ParseFloat(m[1], 64); err == nil {
					duration += d
				}
			}
			out = append(out, uriAttrRe.ReplaceAllStringFunc(trimmed, func(attr string) string {
				u := uriAttrRe.FindStringSubmatch(attr)[1]
				return `URI="` + absolutize(gatewayBase, mediaURL, u) + `"`
			}))
		case trimmed == "":
			out = append(out, trimmed)
		default:
			out = append(out, absolutize(gatewayBase, mediaURL, trimmed))
		}
	}
	local := filepath.Join(dir, "media_local.m3u8")
	if err := os.WriteFile(local, []byte(strings.Join(out, "\n")), 0o644); err != nil {
		return "", 0, err
	}
	return local, duration, nil
}

// absolutize: root-absolute proxy paths (/api/streaming/... or /api/v1/...)
// go onto the gateway; scheme-full URLs pass through; anything else resolves
// relative to the playlist URL.
func absolutize(gatewayBase, baseURL, ref string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if strings.HasPrefix(ref, "/") {
		return strings.TrimRight(gatewayBase, "/") + strings.Replace(ref, "/api/v1/", "/api/streaming/", 1)
	}
	b, err := url.Parse(baseURL)
	if err != nil {
		return ref
	}
	r, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	return b.ResolveReference(r).String()
}

func firstNonComment(manifest string) string {
	for _, line := range strings.Split(manifest, "\n") {
		t := strings.TrimSpace(line)
		if t != "" && !strings.HasPrefix(t, "#") {
			return t
		}
	}
	return ""
}

func fetch(ctx context.Context, hc *http.Client, u string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return "", err
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetch %s -> %d", u, resp.StatusCode)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	return string(b), err
}
```

Test (`playlist_test.go`): httptest server serving a master (`v.m3u8`) + media playlist with `#EXTINF:6.0` lines, one root-absolute `/api/streaming/hls-proxy?url=seg1` segment, one relative `seg2.ts`, and `#EXT-X-KEY:METHOD=AES-128,URI="/api/streaming/hls-proxy?url=key"`. Assert: local file exists, duration==12.0, all lines absolute (segment→gateway-prefixed, relative→server-prefixed, KEY URI rewritten). Also `TestProxiedURL` for raw CDN url (query-encoded) and already-proxied `/api/v1/hls-proxy?...` passthrough (→ `/api/streaming/` on gateway).

- [ ] **Step 5: extract.go** (ffmpeg, library-transcoder conventions: CommandContext + argv slice + 2KB stderr ring):

```go
package prober

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
)

// ExtractFragment pulls one ~30s fragment at seek: mono 16k wav (whisper)
// + 5 frames (1/6 fps) for the hardsub band scan, in a single ffmpeg run.
// input is either a local .m3u8 (HLS) or a proxied http URL (mp4).
func ExtractFragment(ctx context.Context, ffmpegPath, input string, seek float64, durSec, idx int, dir string) (wav string, err error) {
	wav = filepath.Join(dir, fmt.Sprintf("frag_%d.wav", idx))
	frames := filepath.Join(dir, "frames")
	args := []string{
		"-allowed_extensions", "ALL",
		"-protocol_whitelist", "file,http,https,tcp,tls,crypto",
		"-ss", fmt.Sprintf("%.1f", seek),
		"-i", input,
		"-t", fmt.Sprintf("%d", durSec),
		"-vn", "-ac", "1", "-ar", "16000", "-y", wav,
		"-vf", "fps=1/6", "-q:v", "2", "-y", filepath.Join(frames, fmt.Sprintf("f_%d_%%02d.png", idx)),
		"-loglevel", "error",
	}
	cmd := exec.CommandContext(ctx, ffmpegPath, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &limitedWriter{w: &stderr, n: 2048}
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("ffmpeg fragment %d: %w\nstderr tail:\n%s", idx, err, stderr.String())
	}
	return wav, nil
}

type limitedWriter struct {
	w *bytes.Buffer
	n int
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if remaining := l.n - l.w.Len(); remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		l.w.Write(p)
	}
	return len(p), nil
}
```

(Caller creates `dir/frames` beforehand.)

- [ ] **Step 6: runner.go:**

```go
package prober

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"path/filepath"
)

type AnalyzerRunner interface {
	LID(ctx context.Context, wavs []string) (*LIDResult, error)
	Hardsub(ctx context.Context, framesDir string) (*HardsubResult, error)
}

type execRunner struct {
	python string
	dir    string
}

func NewExecRunner(python, analyzersDir string) AnalyzerRunner {
	return &execRunner{python: python, dir: analyzersDir}
}

func (r *execRunner) run(ctx context.Context, script string, args []string, dst any) error {
	argv := append([]string{filepath.Join(r.dir, script)}, args...)
	cmd := exec.CommandContext(ctx, r.python, argv...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &limitedWriter{w: &stderr, n: 2048}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s: %w\nstderr tail:\n%s", script, err, stderr.String())
	}
	return json.Unmarshal(stdout.Bytes(), dst)
}

func (r *execRunner) LID(ctx context.Context, wavs []string) (*LIDResult, error) {
	var out LIDResult
	if err := r.run(ctx, "lid.py", wavs, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

func (r *execRunner) Hardsub(ctx context.Context, framesDir string) (*HardsubResult, error) {
	var out HardsubResult
	if err := r.run(ctx, "hardsub.py", []string{framesDir}, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
```

- [ ] **Step 7: prober.go** — orchestration:

```go
// Package prober turns one queue.Unit into a domain.UnitVerdict: resolve the
// stream via catalog, pull fragments through the streaming proxy with ffmpeg,
// run the python analyzers, assemble confidences.
package prober

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/catalogclient"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

var ErrResolve = errors.New("prober: stream resolve failed")

const (
	fragmentSeconds = 30
	baseFragments   = 3
	maxFragments    = 6
)

type Prober struct {
	cat     *catalogclient.Client
	gateway string
	ffmpeg  string
	workDir string
	runner  AnalyzerRunner
	hc      *http.Client
	log     *logger.Logger
	now     func() time.Time
}

func New(cat *catalogclient.Client, gatewayURL, ffmpegPath, workDir string, runner AnalyzerRunner, log *logger.Logger) *Prober {
	return &Prober{cat: cat, gateway: gatewayURL, ffmpeg: ffmpegPath, workDir: workDir,
		runner: runner, hc: &http.Client{Timeout: 15 * time.Second}, log: log, now: time.Now}
}

// resolveStream fetches the unit's stream, falling back to episode 1 when
// the latest episode is missing on this unit ("ближайший доступный").
func (p *Prober) resolveStream(ctx context.Context, u queue.Unit) (*catalogclient.Stream, int, error) {
	try := func(ep int) (*catalogclient.Stream, error) {
		switch {
		case u.Key.Team != "": // kodik translation
			tid := atoiSafe(u.Key.Team)
			return p.cat.KodikStream(ctx, u.AnimeID, ep, tid)
		case u.EpisodeID != "": // scraper server (episode id fixed at enumeration)
			return p.cat.ScraperStream(ctx, u.AnimeID, u.EpisodeID, u.Key.Server, u.Key.Category, u.Provider)
		default: // animejoy leg
			return p.cat.AnimejoyStream(ctx, u.AnimeID, u.Provider, ep)
		}
	}
	st, err := try(u.Episode)
	if err == nil {
		return st, u.Episode, nil
	}
	if u.Episode > 1 && u.EpisodeID == "" { // ep-numbered providers only
		if st, err2 := try(1); err2 == nil {
			return st, 1, nil
		}
	}
	return nil, 0, fmt.Errorf("%w: %v", ErrResolve, err)
}

// Probe never returns an error — failures become StatusUnreachable verdicts
// with an incremented Fails counter (queue backoff input).
func (p *Prober) Probe(ctx context.Context, u queue.Unit, prevFails int) domain.UnitVerdict {
	v := domain.UnitVerdict{Key: u.Key, Episode: u.Episode, ProbedAt: p.now().UTC()}
	dir, err := os.MkdirTemp(p.workDir, "unit-*")
	if err != nil {
		return p.unreachable(v, prevFails, err)
	}
	defer os.RemoveAll(dir)
	_ = os.MkdirAll(filepath.Join(dir, "frames"), 0o755)

	st, ep, err := p.resolveStream(ctx, u)
	if err != nil {
		return p.unreachable(v, prevFails, err)
	}
	v.Episode = ep
	for _, t := range st.Tracks {
		v.Softsubs = append(v.Softsubs, domain.SoftTrack{Lang: t.Label, Kind: t.Kind})
	}

	input := ProxiedURL(p.gateway, st.URL, st.Exp, st.Sig, st.Referer)
	duration := 0.0
	if st.Type != "mp4" { // HLS: localize + duration from EXTINF sum
		local, dur, err := LocalizeHLS(ctx, p.hc, p.gateway, input, dir)
		if err != nil {
			return p.unreachable(v, prevFails, err)
		}
		input, duration = local, dur
	}
	offsets := sampleOffsets(duration, st.Intro, st.Outro)

	var wavs []string
	for i, seek := range offsets[:baseFragments] {
		wav, err := ExtractFragment(ctx, p.ffmpeg, input, seek, fragmentSeconds, i, dir)
		if err != nil {
			if i == 0 {
				return p.unreachable(v, prevFails, err) // first fragment dead = stream dead
			}
			continue // partial extraction: analyze what we have
		}
		wavs = append(wavs, wav)
	}
	if len(wavs) == 0 {
		return p.unreachable(v, prevFails, errors.New("no fragments extracted"))
	}

	lid, err := p.runner.LID(ctx, wavs)
	if err != nil {
		return p.inconclusive(v, err)
	}
	// Not enough speech? Pull extra fragments (up to maxFragments total).
	if speechCount(lid.Fragments) < baseFragments && len(offsets) > baseFragments {
		for i, seek := range offsets[baseFragments:] {
			idx := baseFragments + i
			if idx >= maxFragments {
				break
			}
			if wav, err := ExtractFragment(ctx, p.ffmpeg, input, seek, fragmentSeconds, idx, dir); err == nil {
				wavs = append(wavs, wav)
			}
		}
		if extra, err := p.runner.LID(ctx, wavs); err == nil {
			lid = extra
		}
	}
	v.Audio = AssembleAudio(lid.Fragments)
	v.Sample = domain.SampleInfo{Fragments: len(wavs), SpeechSeconds: totalSpeech(lid.Fragments)}

	if hs, err := p.runner.Hardsub(ctx, filepath.Join(dir, "frames")); err == nil {
		v.Hardsub = AssembleHardsub(hs)
	} else if p.log != nil {
		p.log.Warnw("hardsub analyzer failed", "provider", u.Provider, "error", err)
	}

	if v.Audio != nil && v.Audio.Verified {
		v.Status = domain.StatusVerified
	} else {
		v.Status = domain.StatusInconclusive
	}
	return v
}

func (p *Prober) unreachable(v domain.UnitVerdict, prevFails int, err error) domain.UnitVerdict {
	if p.log != nil {
		p.log.Warnw("unit unreachable", "key", v.Key.String(), "error", err)
	}
	v.Status = domain.StatusUnreachable
	v.Fails = prevFails + 1
	return v
}

func (p *Prober) inconclusive(v domain.UnitVerdict, err error) domain.UnitVerdict {
	if p.log != nil {
		p.log.Warnw("unit inconclusive", "key", v.Key.String(), "error", err)
	}
	v.Status = domain.StatusInconclusive
	return v
}

// sampleOffsets picks up to maxFragments seek points. Known duration →
// fractions of runtime (skipping intro/outro windows); unknown → fixed
// seeks suited to a ~24min episode.
func sampleOffsets(duration float64, intro, outro *catalogclient.TimeRange) []float64 {
	fracs := []float64{0.25, 0.50, 0.75, 0.35, 0.60, 0.85}
	var out []float64
	if duration < 120 {
		return []float64{60, 240, 480, 300, 600, 720} // duration unknown/tiny: fixed
	}
	for _, f := range fracs {
		s := duration * f
		if intro != nil && s >= float64(intro.Start) && s <= float64(intro.End) {
			s = float64(intro.End) + 10
		}
		if outro != nil && s >= float64(outro.Start) {
			s = float64(outro.Start) - float64(fragmentSeconds) - 10
		}
		if s < 30 {
			s = 30
		}
		if s > duration-float64(fragmentSeconds)-5 {
			s = duration - float64(fragmentSeconds) - 5
		}
		out = append(out, s)
	}
	return out
}

func speechCount(frs []LIDFragment) int {
	n := 0
	for _, f := range frs {
		if f.Speech {
			n++
		}
	}
	return n
}

func totalSpeech(frs []LIDFragment) float64 {
	t := 0.0
	for _, f := range frs {
		t += f.SpeechSeconds
	}
	return t
}

func atoiSafe(s string) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}
```

- [ ] **Step 8: prober test with fake analyzer scripts** (`prober_test.go`): AnalyzerRunner fake struct (returns canned LIDResult/HardsubResult), catalog httptest (Task-5 mux serving an mp4-type animejoy stream so LocalizeHLS is skipped), ffmpeg faked via a shell-script `ffmpeg` stub in t.TempDir that `touch`es the requested wav (`PATH`-independent: pass the stub path as ffmpegPath). Assert: verdict Status==verified when fake LID returns 3 agreeing speech fragments; Status==unreachable with Fails=prevFails+1 when the catalog returns 404. Also `TestSampleOffsets` (intro-skip + unknown-duration fallback).

- [ ] **Step 9: run all** — `go test ./internal/prober/` → PASS.

- [ ] **Step 10: Commit** — `git add services/content-verify && git commit -m "feat(content-verify): prober — resolve, localize, ffmpeg fragments, analyzer verdicts"`

---

### Task 9: Worker loop + HTTP API + main wiring

**Files:**
- Create: `services/content-verify/internal/service/worker.go`
- Modify: `services/content-verify/internal/handler/verify.go` (replace Task-1 stub)
- Modify: `services/content-verify/cmd/content-verify-api/main.go`
- Test: `services/content-verify/internal/service/worker_test.go`, `services/content-verify/internal/handler/verify_test.go`

**Interfaces:**
- Consumes: everything above.
- Produces: `service.Worker{Start(ctx)}`; full `handler.VerifyHandler` (`NewVerifyHandler(store *repo.Store, sig *signals.Signals, engine *queue.Engine, log *logger.Logger)`); wire API:
  - `GET /internal/verify/verdicts?anime_id=` → `{"success":true,"data":{"anime_id":"...","providers":[{"provider":"gogoanime","summary":{...},"units":[...]}]}}`
  - `POST /internal/verify/hint` body `{"anime_id":"...","visitor":"u:<uid>|ip:<hash>","source":"visit|watching"}` → 204
  - `GET /internal/verify/queue` → `{"success":true,"data":{"entries":[QueueEntry...]}}`

- [ ] **Step 1: worker.** `internal/service/worker.go`:

```go
// Package service runs the throttled probe worker: one unit per tick,
// governor-gated, results upserted into the verdict store.
package service

import (
	"context"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/cvmetrics"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
)

// ShedChecker is satisfied by *cache.DegradationWatcher.
type ShedChecker interface {
	ShouldShed(min int) bool
}

type UnitProber interface {
	Probe(ctx context.Context, u queue.Unit, prevFails int) domain.UnitVerdict
}

type VerdictStore interface {
	Get(ctx context.Context, animeID, provider string) (*domain.ContentVerification, error)
	UpsertUnit(ctx context.Context, animeID, provider string, v domain.UnitVerdict) error
}

type Claimer interface {
	Claim(ctx context.Context) (*queue.Unit, bool, error)
}

type Worker struct {
	interval time.Duration
	budget   time.Duration
	shed     ShedChecker
	claimer  Claimer
	prober   UnitProber
	store    VerdictStore
	log      *logger.Logger
}

func NewWorker(interval, budget time.Duration, shed ShedChecker, claimer Claimer, prober UnitProber, store VerdictStore, log *logger.Logger) *Worker {
	return &Worker{interval: interval, budget: budget, shed: shed, claimer: claimer, prober: prober, store: store, log: log}
}

func (w *Worker) Start(ctx context.Context) {
	go func() {
		ticker := time.NewTicker(w.interval)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				w.tick(ctx)
			}
		}
	}()
}

func (w *Worker) tick(ctx context.Context) {
	if w.shed != nil && w.shed.ShouldShed(1) {
		cvmetrics.TicksSkippedTotal.WithLabelValues("degraded").Inc()
		return
	}
	unit, _, err := w.claimer.Claim(ctx)
	if err != nil {
		cvmetrics.TicksSkippedTotal.WithLabelValues("claim_error").Inc()
		if w.log != nil {
			w.log.Errorw("claim failed", "error", err)
		}
		return
	}
	if unit == nil {
		cvmetrics.TicksSkippedTotal.WithLabelValues("idle").Inc()
		return
	}

	// ae first-party: synthesize from library-ingest truth (Phase C), no probe.
	if unit.AeLang != "" {
		v := domain.UnitVerdict{Key: unit.Key, Episode: unit.Episode, Status: domain.StatusVerified,
			Audio:    &domain.AudioVerdict{Lang: unit.AeLang, Confidence: 1.0, Verified: true},
			ProbedAt: time.Now().UTC()}
		w.persist(ctx, *unit, v, "synth")
		return
	}

	prevFails := 0
	if prev, err := w.store.Get(ctx, unit.AnimeID, unit.Provider); err == nil && prev != nil {
		key := unit.Key.String()
		for _, u := range prev.Units {
			if u.Key.String() == key {
				prevFails = u.Fails
				break
			}
		}
	}

	start := time.Now()
	bctx, cancel := context.WithTimeout(ctx, w.budget)
	v := w.prober.Probe(bctx, *unit, prevFails)
	cancel()
	cvmetrics.ProbeDuration.Observe(time.Since(start).Seconds())
	w.persist(ctx, *unit, v, v.Status)
}

func (w *Worker) persist(ctx context.Context, unit queue.Unit, v domain.UnitVerdict, result string) {
	if err := w.store.UpsertUnit(ctx, unit.AnimeID, unit.Provider, v); err != nil {
		cvmetrics.ProbesTotal.WithLabelValues(unit.Provider, "error").Inc()
		if w.log != nil {
			w.log.Errorw("verdict upsert failed", "anime_id", unit.AnimeID, "provider", unit.Provider, "error", err)
		}
		return
	}
	cvmetrics.ProbesTotal.WithLabelValues(unit.Provider, result).Inc()
	cvmetrics.LastProbeTS.Set(float64(time.Now().Unix()))
	if w.log != nil {
		w.log.Infow("unit probed", "anime_id", unit.AnimeID, "provider", unit.Provider,
			"key", v.Key.String(), "status", v.Status)
	}
}
```

- [ ] **Step 2: worker test** (`worker_test.go`): fakes for all four deps. Cases: (a) shed=true → prober never called; (b) claim returns ae-synth unit → store receives verified lang verdict, prober not called; (c) normal unit → prober called with prevFails from store, verdict persisted. Drive `w.tick(ctx)` directly (not Start).

- [ ] **Step 3: handler.** Replace the Task-1 stub `internal/handler/verify.go`:

```go
package handler

import (
	"encoding/json"
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"

	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/domain"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/queue"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/content-verify/internal/signals"
)

type VerifyHandler struct {
	store  *repo.Store
	sig    *signals.Signals
	engine *queue.Engine
	log    *logger.Logger
}

func NewVerifyHandler(store *repo.Store, sig *signals.Signals, engine *queue.Engine, log *logger.Logger) *VerifyHandler {
	return &VerifyHandler{store: store, sig: sig, engine: engine, log: log}
}

type providerVerdicts struct {
	Provider string                 `json:"provider"`
	Summary  domain.ProviderSummary `json:"summary"`
	Units    []domain.UnitVerdict   `json:"units"`
}

type verdictsResponse struct {
	AnimeID   string             `json:"anime_id"`
	Providers []providerVerdicts `json:"providers"`
}

func (h *VerifyHandler) Verdicts(w http.ResponseWriter, r *http.Request) {
	animeID := r.URL.Query().Get("anime_id")
	if animeID == "" {
		http.Error(w, "anime_id required", http.StatusBadRequest)
		return
	}
	rows, err := h.store.ByAnime(r.Context(), animeID)
	if err != nil {
		http.Error(w, "query failed", http.StatusInternalServerError)
		return
	}
	resp := verdictsResponse{AnimeID: animeID, Providers: []providerVerdicts{}}
	for _, row := range rows {
		resp.Providers = append(resp.Providers, providerVerdicts{
			Provider: row.Provider, Summary: domain.Summarize(row.Units), Units: row.Units})
	}
	httputil.OK(w, resp)
}

type hintRequest struct {
	AnimeID string `json:"anime_id"`
	Visitor string `json:"visitor"`
	Source  string `json:"source"`
}

func (h *VerifyHandler) Hint(w http.ResponseWriter, r *http.Request) {
	var req hintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.AnimeID == "" || req.Visitor == "" {
		http.Error(w, "anime_id and visitor required", http.StatusBadRequest)
		return
	}
	if err := h.sig.RecordVisit(r.Context(), req.AnimeID, req.Visitor); err != nil && h.log != nil {
		h.log.Warnw("hint record failed", "anime_id", req.AnimeID, "error", err)
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *VerifyHandler) Queue(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, map[string]any{"entries": h.engine.Snapshot(r.Context(), 50)})
}
```

Handler test (`verify_test.go`): miniredis-backed signals + sqlite store; seed one row via `UpsertUnit`; assert Verdicts JSON carries summary+units; Hint 204 then `UniqueVisitors == 1`; Hint without visitor → 400.

- [ ] **Step 4: main wiring.** In `cmd/content-verify-api/main.go` replace the `var h *handler.VerifyHandler` block:

```go
	store := repo.NewStore(db.DB)
	sig := signals.New(redisCache.Client())
	catClient := catalogclient.New(cfg.CatalogURL, nil)
	engine := queue.NewEngine(catClient, sig, store, cfg.ReprobeTTL, log)
	h := handler.NewVerifyHandler(store, sig, engine, log)

	if cfg.WorkerOn {
		if err := os.MkdirAll(cfg.WorkDir, 0o755); err != nil {
			log.Fatalw("workdir create failed", "error", err)
		}
		shedWatcher := cache.NewDegradationWatcher(redisCache, 5*time.Second)
		shedWatcher.Start(ctx)
		runner := prober.NewExecRunner(cfg.PythonPath, cfg.AnalyzersDir)
		pb := prober.New(catClient, cfg.GatewayURL, cfg.FFmpegPath, cfg.WorkDir, runner, log)
		worker := service.NewWorker(cfg.Interval, cfg.UnitBudget, shedWatcher, engine, pb, store, log)
		worker.Start(ctx)
		log.Infow("content-verify worker started", "interval", cfg.Interval, "budget", cfg.UnitBudget)
	}
```

(`db.DB` is the embedded `*gorm.DB` from libs/database.)

- [ ] **Step 5: full module test + build** — `cd services/content-verify && go test ./... && go build ./...` → PASS.

- [ ] **Step 6: Commit** — `git add services/content-verify && git commit -m "feat(content-verify): worker loop with governor gate + internal verdicts/hint/queue API"`

---

### Task 10: Deployment wiring — Dockerfile, COPY sweep, compose, k8s, docs

**Files:**
- Create: `services/content-verify/Dockerfile`
- Modify: ALL 21 Go-service Dockerfiles (COPY line)
- Modify: `docker/docker-compose.yml`
- Create: `deploy/kustomize/base/services/content-verify.yaml`
- Modify: `deploy/kustomize/base/**/kustomization.yaml` (resources list — find where governor.yaml is registered)
- Modify: `docs/environment-variables.md`, `CLAUDE.md` (Service Ports table)

**Interfaces:** none new — packaging only.

- [ ] **Step 1: content-verify Dockerfile.** Build stage = clone governor's builder verbatim (same full COPY block of ALL module go.mods **including the new `COPY services/content-verify/go.mod services/content-verify/go.sum* ./services/content-verify/`**), building `./cmd/content-verify-api`. Runtime stage = debian (python+tesseract need apt; alpine has no tesseract lang packs):

```dockerfile
# Runtime: debian-slim because tesseract-ocr language packs + faster-whisper
# (ctranslate2 wheels) are apt/pip-clean here; alpine is not.
FROM debian:bookworm-slim
ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    HF_HOME=/opt/models \
    OMP_NUM_THREADS=2
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    tesseract-ocr tesseract-ocr-eng tesseract-ocr-rus tesseract-ocr-jpn \
    python3 python3-pip \
    ca-certificates wget tzdata \
 && rm -rf /var/lib/apt/lists/*
COPY services/content-verify/analyzers/requirements.txt /app/analyzers/requirements.txt
RUN pip3 install --break-system-packages -r /app/analyzers/requirements.txt
# Bake the whisper tiny model into the image (no first-probe download stall).
RUN python3 -c "from faster_whisper import WhisperModel; WhisperModel('tiny', device='cpu', compute_type='int8')"
COPY services/content-verify/analyzers/ /app/analyzers/
COPY --from=builder /content-verify-api /app/
WORKDIR /app
RUN groupadd -r app && useradd -r -g app app && mkdir -p /tmp/cv && chown -R app:app /app /tmp/cv /opt/models
USER app
EXPOSE 8101
CMD ["./content-verify-api"]
```

- [ ] **Step 2: COPY sweep.** Every Dockerfile already has the catalog line — use it as the anchor:

```bash
for d in analytics anidle auth catalog fanfic gacha gateway governor library notifications player policy recs rooms scheduler scraper storage streaming themes upscaler watch-together; do
  f="services/$d/Dockerfile"
  grep -q "services/content-verify/go.mod" "$f" || \
    sed -i '\#COPY services/catalog/go.mod#a COPY services/content-verify/go.mod services/content-verify/go.sum* ./services/content-verify/' "$f"
done
grep -L "services/content-verify/go.mod" services/*/Dockerfile   # expect ONLY services/stealth-scraper/Dockerfile
```

- [ ] **Step 3: compose.** Add to `docker/docker-compose.yml` after the `governor:` block (mirror its shape):

```yaml
  content-verify:
    logging: *default-logging
    build:
      context: ..
      dockerfile: services/content-verify/Dockerfile
    container_name: animeenigma-content-verify
    mem_limit: 1g
    restart: unless-stopped
    environment:
      SERVER_PORT: 8101
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: ${DB_USER}
      DB_PASSWORD: ${DB_PASSWORD}
      DB_NAME: ${DB_NAME}
      REDIS_HOST: redis
      CV_CATALOG_URL: http://catalog:8081
      CV_GATEWAY_URL: http://gateway:8000
      TRACING_ENABLED: "true"
    ports:
      - "127.0.0.1:8101:8101"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "-O", "/dev/null", "http://localhost:8101/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

Match the exact `${DB_*}`/postgres service names used by catalog's compose block (copy them — some use hardcoded values).

- [ ] **Step 4: k8s.** `deploy/kustomize/base/services/content-verify.yaml` — clone governor.yaml, substituting name/port/env (`SERVER_PORT: "8101"`, `CV_CATALOG_URL: http://catalog:8081`, `CV_GATEWAY_URL: http://gateway:8000`, image `ghcr.io/ilita-hub/animeenigma/content-verify:latest`, resources requests 250m/512Mi limits 1500m/1Gi — whisper needs headroom). Register in the same kustomization resources list where `services/governor.yaml` appears.

- [ ] **Step 5: docs.** `docs/environment-variables.md` — append the one-paragraph block:

```markdown
**Content-verify** (content probing queue, spec 2026-07-16 — port 8101, internal-only, needs `DB_*` + `REDIS_*`, no `JWT_SECRET`): `CV_CATALOG_URL` (default `http://catalog:8081` — membership/structure/stream resolve), `CV_GATEWAY_URL` (default `http://gateway:8000` — ffmpeg reads hls-proxy through the public path), `CV_INTERVAL` (default `1m` — one probe per tick), `CV_UNIT_BUDGET` (default `50s` — hard per-unit budget, REVISIT after live timings), `CV_REPROBE_TTL` (default `720h`), `CV_TOP_LIMIT` (default `100`), `CV_WORKER_ENABLED` (default `true` — `false` = API only), `CV_FFMPEG_PATH`/`CV_PYTHON`/`CV_ANALYZERS_DIR`/`CV_WORKDIR` (container-baked defaults). Catalog: `CONTENT_VERIFY_URL` (default `http://content-verify:8101`), `CONTENT_VERIFY_ENABLED` (default `true` — kill switch for blend + proxy). Player: `CONTENT_VERIFY_INTERNAL_URL` (default `http://content-verify:8101`), `CONTENT_VERIFY_HINT_ENABLED` (default `true`).
```

`CLAUDE.md` Service Ports table — add row after governor: `| content-verify | 8101 | /metrics | Content probing (audio lang + burned-in subs verdicts) |`.

- [ ] **Step 6: build check** — `docker compose -f docker/docker-compose.yml build content-verify` (slow first time — whisper model bake ~100MB download) and `docker compose -f docker/docker-compose.yml config --quiet`.
Expected: image builds, config valid.

- [ ] **Step 7: Commit**

```bash
git add services/content-verify/Dockerfile services/*/Dockerfile docker/docker-compose.yml deploy/kustomize docs/environment-variables.md CLAUDE.md
git commit -m "feat(content-verify): deployment wiring — image, compose, k8s, docs, COPY sweep"
```

---

### Task 11: Catalog — public proxy endpoint + visit hint + capabilities blend

**Files:**
- Modify: `services/catalog/internal/domain/capability.go` (ProviderCap.Verify + VerifySummary)
- Create: `services/catalog/internal/service/capability/verify_client.go`
- Modify: `services/catalog/internal/service/capability/service.go` (NewService param + blend)
- Create: `services/catalog/internal/handler/content_verify.go`
- Modify: `services/catalog/internal/transport/router.go`, `services/catalog/cmd/catalog-api/main.go`
- Test: `services/catalog/internal/service/capability/verify_client_test.go`, `services/catalog/internal/handler/content_verify_test.go`

**Interfaces:**
- Produces: public `GET /api/anime/{animeId}/content-verify` (30s-cached passthrough of content-verify verdicts; EVERY request fires an async visit hint with the caller's identity); `ProviderCap.Verify *domain.VerifySummary` (`json:"verify,omitempty"`) blended into capabilities.
- Consumes: content-verify `GET /internal/verify/verdicts?anime_id=` + `POST /internal/verify/hint` (Task 9); `scraperUserKey(r)` (same `handler` package, `scraper.go:200` — `"u:"+uid` authed / `"ip:"+sha256(ip|salt|day)` anon).

- [ ] **Step 1: domain.** In `services/catalog/internal/domain/capability.go` add to `ProviderCap`:

```go
	// Verify carries the content-verify probe rollup (nil = never probed).
	Verify *VerifySummary `json:"verify,omitempty"`
```

and the type (JSON mirrors content-verify's `domain.ProviderSummary`):

```go
// VerifySummary is the content-verify rollup for one provider on one anime.
type VerifySummary struct {
	Status       string   `json:"status"` // unverified|partial|verified
	Raw          bool     `json:"raw"`
	DubLangs     []string `json:"dub_langs"`
	HardsubLangs []string `json:"hardsub_langs"`
}
```

- [ ] **Step 2: verify client** (`capability/verify_client.go`, clone of `analytics_client.go` swallow-all shape):

```go
package capability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/ILITA-hub/animeenigma/services/catalog/internal/domain"
)

const verifyFetchTimeout = 3 * time.Second

// VerifySource returns per-provider content-verify rollups; best-effort,
// never errors (nil map on any failure).
type VerifySource interface {
	Summaries(ctx context.Context, animeID string) map[string]domain.VerifySummary
	RawVerdicts(ctx context.Context, animeID string) (json.RawMessage, error)
	Hint(animeID, visitor, source string)
}

type VerifyClient struct {
	base    string
	client  *http.Client
	enabled bool
}

func NewVerifyClient(baseURL string, enabled bool) *VerifyClient {
	return &VerifyClient{base: baseURL, client: &http.Client{Timeout: verifyFetchTimeout}, enabled: enabled}
}

type verifyWire struct {
	AnimeID   string `json:"anime_id"`
	Providers []struct {
		Provider string               `json:"provider"`
		Summary  domain.VerifySummary `json:"summary"`
	} `json:"providers"`
}

// RawVerdicts returns the verbatim data payload for the public passthrough.
func (c *VerifyClient) RawVerdicts(ctx context.Context, animeID string) (json.RawMessage, error) {
	if c == nil || !c.enabled {
		return nil, fmt.Errorf("content-verify disabled")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.base+"/internal/verify/verdicts?anime_id="+url.QueryEscape(animeID), nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("content-verify -> %d", resp.StatusCode)
	}
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return nil, err
	}
	return env.Data, nil
}

func (c *VerifyClient) Summaries(ctx context.Context, animeID string) map[string]domain.VerifySummary {
	raw, err := c.RawVerdicts(ctx, animeID)
	if err != nil {
		return nil
	}
	var wire verifyWire
	if json.Unmarshal(raw, &wire) != nil {
		return nil
	}
	out := make(map[string]domain.VerifySummary, len(wire.Providers))
	for _, p := range wire.Providers {
		out[p.Provider] = p.Summary
	}
	return out
}

// Hint fires a fire-and-forget visit signal (own ctx — the caller's request
// context ends before the POST would finish).
func (c *VerifyClient) Hint(animeID, visitor, source string) {
	if c == nil || !c.enabled || animeID == "" || visitor == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), verifyFetchTimeout)
		defer cancel()
		body, _ := json.Marshal(map[string]string{"anime_id": animeID, "visitor": visitor, "source": source})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.base+"/internal/verify/hint", bytes.NewReader(body))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		if resp, err := c.client.Do(req); err == nil {
			resp.Body.Close()
		}
	}()
}
```

Test: httptest server → Summaries decodes; server 500 → nil; disabled → nil; Hint posts (synchronize with a channel in the test server handler).

- [ ] **Step 3: blend.** `capability/service.go`: add `verify VerifySource` param to `NewService` (append AFTER `playability PlayabilitySource`; update ALL call sites — main.go and any capability tests). In `buildFamilies`, after the families slice is fully built (post `regroupFamilies`), overlay:

```go
	if s.verify != nil {
		if sums := s.verify.Summaries(ctx, animeID); len(sums) > 0 {
			for fi := range families {
				for pi := range families[fi].Providers {
					if sum, ok := sums[families[fi].Providers[pi].Provider]; ok {
						v := sum
						families[fi].Providers[pi].Verify = &v
					}
				}
			}
		}
	}
```

NOTE: the capability report is cached 10 min (`"capabilities:"+animeID`) — blended summaries are the INITIAL state only; the live channel is the public endpoint below. That staleness is by design (spec §4-5).

- [ ] **Step 4: public handler** (`handler/content_verify.go`):

```go
package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/httputil"
	"github.com/ILITA-hub/animeenigma/libs/logger"
)

const (
	contentVerifyTTL        = 30 * time.Second // shorter than the FE poll (45s)
	contentVerifyCtxTimeout = 3 * time.Second
)

// verifyProxySource is the slice of capability.VerifyClient this handler needs.
type verifyProxySource interface {
	RawVerdicts(ctx context.Context, animeID string) (json.RawMessage, error)
	Hint(animeID, visitor, source string)
}

// ContentVerifyHandler serves GET /api/anime/{animeId}/content-verify — the
// aePlayer's dynamic verdict feed. Every request IS the +15 visit signal
// (spec §1): the hint fires before the cache check, deduped downstream.
type ContentVerifyHandler struct {
	src   verifyProxySource
	cache cache.Cache
	log   *logger.Logger
}

func NewContentVerifyHandler(src verifyProxySource, c cache.Cache, log *logger.Logger) *ContentVerifyHandler {
	return &ContentVerifyHandler{src: src, cache: c, log: log}
}

func (h *ContentVerifyHandler) Get(w http.ResponseWriter, r *http.Request) {
	animeID := chi.URLParam(r, "animeId")
	if animeID == "" {
		http.Error(w, "animeId required", http.StatusBadRequest)
		return
	}
	h.src.Hint(animeID, scraperUserKey(r), "visit")

	key := "contentverify:" + animeID
	var cached json.RawMessage
	if h.cache != nil {
		if err := h.cache.Get(r.Context(), key, &cached); err == nil && len(cached) > 0 {
			httputil.OK(w, cached)
			return
		}
	}
	ctx, cancel := context.WithTimeout(r.Context(), contentVerifyCtxTimeout)
	defer cancel()
	raw, err := h.src.RawVerdicts(ctx, animeID)
	if err != nil {
		// Degrade to an empty report — the FE treats it as all-unverified.
		httputil.OK(w, map[string]any{"anime_id": animeID, "providers": []any{}})
		return
	}
	if h.cache != nil {
		_ = h.cache.Set(r.Context(), key, json.RawMessage(raw), contentVerifyTTL)
	}
	httputil.OK(w, json.RawMessage(raw))
}
```

(`scraperUserKey` lives in the same `handler` package — `scraper.go:200`. If `cache.Cache`'s Get signature differs (`Get(ctx, key, &dst)` returning `cache.ErrNotFound`), match the capabilities handler's usage.)

Test: stub src (records Hint calls, returns canned raw), nil cache → 200 with envelope; src error → 200 with empty providers; hint recorded with non-empty visitor (fake request RemoteAddr).

- [ ] **Step 5: wiring.** `router.go`: add param `contentVerifyHandler *handler.ContentVerifyHandler`, register inside the anime route group beside capabilities (~line 196):

```go
	if contentVerifyHandler != nil {
		r.Get("/{animeId}/content-verify", contentVerifyHandler.Get)
	}
```

`main.go` (~line 695, beside the playability wiring):

```go
	verifyEnabled := os.Getenv("CONTENT_VERIFY_ENABLED") != "false" // default true
	verifyURL := os.Getenv("CONTENT_VERIFY_URL")
	if verifyURL == "" {
		verifyURL = "http://content-verify:8101"
	}
	verifyClient := capability.NewVerifyClient(verifyURL, verifyEnabled)
	contentVerifyHandler := handler.NewContentVerifyHandler(verifyClient, redisCache, log)
```

pass `verifyClient` as the new last arg of `capability.NewService(...)` and `contentVerifyHandler` into `transport.NewRouter(...)`. (Match the actual cache variable name used for other handlers in main.go.)

- [ ] **Step 6: test + build** — `cd services/catalog && go test ./internal/... && go build ./...` → PASS. The gateway needs NO change: `/api/anime/*` catch-all already proxies to catalog (gateway router.go:458-467).

- [ ] **Step 7: Commit**

```bash
git add services/catalog/internal/domain/capability.go services/catalog/internal/service/capability services/catalog/internal/handler/content_verify.go services/catalog/internal/handler/content_verify_test.go services/catalog/internal/transport/router.go services/catalog/cmd/catalog-api/main.go
git commit -m "feat(catalog): content-verify proxy endpoint + visit hint + capability blend"
```

---

### Task 12: Player — watching hint

**Files:**
- Create: `services/player/internal/service/verify_hint.go` (clone `recs_hint.go`)
- Modify: `services/player/internal/service/list.go` (~line 440, `MarkEpisodeWatched`)
- Modify: `services/player/internal/config/config.go`, `services/player/cmd/player-api/main.go`
- Test: `services/player/internal/service/verify_hint_test.go`

**Interfaces:**
- Produces: `VerifyHintProducer{Start,Stop,Hint(userID, animeID string)}` — POSTs `{"anime_id","visitor":"u:"+userID,"source":"watching"}` to `CONTENT_VERIFY_INTERNAL_URL + /internal/verify/hint`. Non-blocking, drop-on-full (cap 256), swallow-all.

- [ ] **Step 1: clone the producer.** Copy `services/player/internal/service/recs_hint.go` → `verify_hint.go`; rename type `RecsHintProducer`→`VerifyHintProducer`, message struct fields:

```go
type verifyHintMsg struct {
	AnimeID string `json:"anime_id"`
	Visitor string `json:"visitor"`
	Source  string `json:"source"`
}

func (p *VerifyHintProducer) Hint(userID, animeID string) {
	if p == nil || !p.enabled || userID == "" || animeID == "" {
		return
	}
	msg := verifyHintMsg{AnimeID: animeID, Visitor: "u:" + userID, Source: "watching"}
	select {
	case p.ch <- msg:
	default:
		p.log.Warnw("verify hint channel full; dropping", "anime_id", animeID)
	}
}
```

endpoint: `p.url + "/internal/verify/hint"`. Keep the worker/Start/Stop/send shape byte-for-byte from recs_hint.go.

- [ ] **Step 2: config.** In `config.go` next to `RecsConfig`:

```go
	ContentVerify struct {
		InternalURL string
		HintEnabled bool
	}
```

loaded as `InternalURL: getEnv("CONTENT_VERIFY_INTERNAL_URL", "http://content-verify:8101")`, `HintEnabled: getEnvBool("CONTENT_VERIFY_HINT_ENABLED", true)` (mirror the RecsConfig loading style at ~line 186).

- [ ] **Step 3: wire.** `main.go` (~line 376, beside recsHintProducer):

```go
	verifyHintProducer := service.NewVerifyHintProducer(cfg.ContentVerify.InternalURL, cfg.ContentVerify.HintEnabled, log)
	verifyHintProducer.Start()
	defer verifyHintProducer.Stop()
```

pass into `NewListService(...)` as a new param; in `list.go` `MarkEpisodeWatched` fire beside the recs hint:

```go
	s.recsHint.Hint(userID, animeID)
	s.verifyHint.Hint(userID, animeID)
```

Update `NewListService` signature + struct field + all call sites (tests included — grep `NewListService(`).

- [ ] **Step 4: test** (`verify_hint_test.go`): httptest server capturing the POST; `Hint("u1","a1")`; assert body `{"anime_id":"a1","visitor":"u:u1","source":"watching"}` arrives; disabled producer sends nothing.

- [ ] **Step 5: build + test + commit**

```bash
cd services/player && go test ./... && go build ./...
git add services/player
git commit -m "feat(player): content-verify watching hint on episode watch"
```

---

### Task 13: FE — types, api client, useContentVerify composable

**Files:**
- Create: `frontend/web/src/types/contentVerify.ts`
- Modify: `frontend/web/src/api/client.ts` (beside `capabilitiesApi`, ~line 979)
- Create: `frontend/web/src/composables/aePlayer/useContentVerify.ts`
- Test: `frontend/web/src/composables/aePlayer/useContentVerify.spec.ts`

**Interfaces:**
- Produces:

```ts
// types/contentVerify.ts
export interface VerifyUnit {
  key: { team?: string; server?: string; category?: string; track?: string }
  episode: number
  status: 'verified' | 'inconclusive' | 'unreachable'
  audio?: { lang?: string; confidence: number; verified: boolean }
  hardsub?: { present: boolean; lang?: string; confidence: number; verified: boolean }
  probed_at?: string
}
export interface ProviderVerify {
  status: 'unverified' | 'partial' | 'verified'
  raw: boolean
  dub_langs: string[]
  hardsub_langs: string[]
  units?: VerifyUnit[]
}
export interface VerifyReport {
  animeId: string
  providers: Record<string, ProviderVerify>
}
```

- `contentVerifyApi.get(animeId)` → `GET /anime/{animeId}/content-verify`;
- `useContentVerify(animeId: Ref<string>, active: Ref<boolean>, pollMs = 45000)` → `{ report: Ref<VerifyReport|null>, refresh(): Promise<void> }`. Polls while `active` && page visible; stops (keeps data) when `active` flips false; `normalizeVerify(raw)` maps the wire array (`providers: [{provider, summary, units}]`) into the Record shape.

- [ ] **Step 1: failing spec** (`useContentVerify.spec.ts`) — mock `@/api/client`, fake timers:

```ts
import { describe, expect, it, vi, beforeEach } from 'vitest'
import { nextTick, ref } from 'vue'

const getMock = vi.fn()
vi.mock('@/api/client', async (importOriginal) => {
  const orig = await importOriginal<typeof import('@/api/client')>()
  return { ...orig, contentVerifyApi: { get: (id: string) => getMock(id) } }
})

import { normalizeVerify, useContentVerify } from './useContentVerify'
import { withSetup } from '@/composables/__tests__/withSetup' // reuse the repo's setup helper if present; otherwise call inside a test component via mount

describe('normalizeVerify', () => {
  it('maps the wire array to a provider record', () => {
    const raw = {
      anime_id: 'a1',
      providers: [
        { provider: 'gogoanime', summary: { status: 'partial', raw: true, dub_langs: ['en'], hardsub_langs: [] }, units: [] },
      ],
    }
    const rep = normalizeVerify(raw)!
    expect(rep.animeId).toBe('a1')
    expect(rep.providers.gogoanime.dub_langs).toEqual(['en'])
  })
  it('returns null on garbage', () => {
    expect(normalizeVerify(null)).toBeNull()
    expect(normalizeVerify({})).toBeNull()
  })
})

describe('useContentVerify', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    getMock.mockReset().mockResolvedValue({ data: { data: { anime_id: 'a1', providers: [] } } })
  })
  it('fetches immediately and re-polls while active', async () => {
    const active = ref(true)
    useContentVerify(ref('a1'), active) // inside component-less setup: acceptable if composable guards lifecycle hooks
    await vi.advanceTimersByTimeAsync(0)
    expect(getMock).toHaveBeenCalledTimes(1)
    await vi.advanceTimersByTimeAsync(45000)
    expect(getMock).toHaveBeenCalledTimes(2)
    active.value = false
    await nextTick()
    await vi.advanceTimersByTimeAsync(90000)
    expect(getMock).toHaveBeenCalledTimes(2) // stopped
  })
})
```

Adapt the harness to the repo's existing composable-test pattern (see `useSystemStatus`-style specs; the vue-i18n mock/barrel trap does not apply — no i18n here). Guard `onBeforeUnmount` with `getCurrentInstance()` so the composable is testable without a component.

- [ ] **Step 2: implement** `useContentVerify.ts`:

```ts
import { getCurrentInstance, onBeforeUnmount, ref, watch, type Ref } from 'vue'
import { contentVerifyApi } from '@/api/client'
import type { ProviderVerify, VerifyReport, VerifyUnit } from '@/types/contentVerify'

interface WireProvider {
  provider: string
  summary: Omit<ProviderVerify, 'units'>
  units?: VerifyUnit[]
}

export function normalizeVerify(raw: unknown): VerifyReport | null {
  const r = raw as { anime_id?: string; providers?: WireProvider[] } | null
  if (!r || typeof r.anime_id !== 'string' || !Array.isArray(r.providers)) return null
  const providers: Record<string, ProviderVerify> = {}
  for (const p of r.providers) {
    if (!p?.provider || !p.summary) continue
    providers[p.provider] = { ...p.summary, units: p.units ?? [] }
  }
  return { animeId: r.anime_id, providers }
}

/**
 * Dynamic content-verify feed: fetch on mount, re-poll every pollMs while
 * `active` (player open, playback not started) and the page is visible.
 * When `active` flips false the poll stops but the last report stays —
 * badges keep rendering, only combo correction is off the table.
 */
export function useContentVerify(animeId: Ref<string>, active: Ref<boolean>, pollMs = 45000) {
  const report = ref<VerifyReport | null>(null)
  let timer: ReturnType<typeof setInterval> | null = null

  async function refresh() {
    if (!animeId.value || (typeof document !== 'undefined' && document.hidden)) return
    try {
      const res = await contentVerifyApi.get(animeId.value)
      const normalized = normalizeVerify(res.data?.data ?? res.data)
      if (normalized) report.value = normalized
    } catch {
      /* best-effort — absent report just means "all unverified" */
    }
  }

  function stop() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  function start() {
    stop()
    if (!active.value || !animeId.value) return
    void refresh()
    timer = setInterval(() => {
      if (active.value) void refresh()
      else stop()
    }, pollMs)
  }

  watch([animeId, active], ([, isActive]) => {
    if (isActive) start()
    else stop()
  }, { immediate: true })

  if (getCurrentInstance()) onBeforeUnmount(stop)

  return { report, refresh }
}
```

`api/client.ts` addition beside `capabilitiesApi`:

```ts
export const contentVerifyApi = {
  get: (animeId: string) => apiClient.get(`/anime/${animeId}/content-verify`),
}
```

- [ ] **Step 3: run** — `cd frontend/web && bunx vitest run src/composables/aePlayer/useContentVerify.spec.ts` → PASS.

- [ ] **Step 4: Commit** — `git add frontend/web/src/types/contentVerify.ts frontend/web/src/api/client.ts frontend/web/src/composables/aePlayer/useContentVerify.ts frontend/web/src/composables/aePlayer/useContentVerify.spec.ts && git commit -m "feat(web): content-verify types, api client, polling composable"`

---

### Task 14: FE — verified gating (RAW default), badges, marker, i18n

**Files:**
- Create: `frontend/web/src/composables/aePlayer/verifiedCaps.ts` (+`verifiedCaps.spec.ts`)
- Modify: `frontend/web/src/composables/aePlayer/useProviderFeed.ts` (+ its spec)
- Modify: `frontend/web/src/composables/aePlayer/useCapabilityFeed.ts`
- Modify: `frontend/web/src/composables/aePlayer/capLabels.ts` (+ its spec)
- Modify: `frontend/web/src/types/aePlayer.ts` (ProviderRow.verify)
- Modify: `frontend/web/src/components/player/aePlayer/ProviderChip.vue`, `SourcePanel.vue`
- Modify: `frontend/web/src/locales/en.json`, `ru.json`, `ja.json`

**Interfaces:**
- Produces (`verifiedCaps.ts`):

```ts
export function verifyFor(report: VerifyReport | null, provider: string): ProviderVerify | null
export function effectiveAudios(cap: Pick<ProviderCap, 'group' | 'audios'>, v: ProviderVerify | null): ('sub' | 'dub')[]
export function verifiedDubLangs(v: ProviderVerify | null): TrackLang[]
export function isUnverified(cap: Pick<ProviderCap, 'group'>, v: ProviderVerify | null): boolean
```

- Gating semantics (owner decision): **unverified ⇒ `['sub']` (RAW-assumed) + marker; DUB only from verified dub_langs. `group==='firstparty'` (ae) trusts `cap.audios`/`cap.lang` even without a verify row** (first-party ingest truth, Phase C). `partial` keeps 'sub' (unverified units stay RAW-assumed) and adds 'dub' when dub_langs non-empty; fully-`verified` rows expose exactly what was proven.
- `ProviderRow` gains `verify?: ProviderVerify | null`; `rowsFromReport(report, filter, verify?)` threads it; `relevant()` uses `effectiveAudios` + `verifiedDubLangs`.
- `deriveCapLabels(cap, verify?)` returns extra fields `{ unverified: boolean; verifiedDub: TrackLang[]; verifiedHardsub: TrackLang[] }`.
- i18n keys (all three locales, ICU-parity):
  - `player.sources.unverified`: en `"unverified"` / ru `"не проверено"` / ja `"未確認"`
  - `player.sources.verifiedDub`: en `"DUB {lang} ✓"` / ru `"DUB {lang} ✓"` / ja `"吹替 {lang} ✓"`
  - `player.sources.verifiedHardsub`: en `"Burned-in ({lang})"` / ru `"Вшитые ({lang})"` / ja `"焼き込み字幕（{lang}）"`

- [ ] **Step 1: failing verifiedCaps spec:**

```ts
import { describe, expect, it } from 'vitest'
import { effectiveAudios, isUnverified, verifiedDubLangs, verifyFor } from './verifiedCaps'
import type { ProviderVerify, VerifyReport } from '@/types/contentVerify'

const v = (p: Partial<ProviderVerify>): ProviderVerify => ({
  status: 'unverified', raw: false, dub_langs: [], hardsub_langs: [], ...p,
})

describe('effectiveAudios', () => {
  const cap = { group: 'en' as const, audios: ['sub', 'dub'] as ('sub' | 'dub')[] }
  it('unverified ⇒ RAW-assumed only', () => {
    expect(effectiveAudios(cap, null)).toEqual(['sub'])
    expect(effectiveAudios(cap, v({}))).toEqual(['sub'])
  })
  it('verified dub only ⇒ dub only', () => {
    expect(effectiveAudios(cap, v({ status: 'verified', dub_langs: ['en'] }))).toEqual(['dub'])
  })
  it('partial with dub keeps RAW for unverified units', () => {
    expect(effectiveAudios(cap, v({ status: 'partial', dub_langs: ['en'] }))).toEqual(expect.arrayContaining(['sub', 'dub']))
  })
  it('verified raw ⇒ sub', () => {
    expect(effectiveAudios(cap, v({ status: 'verified', raw: true }))).toEqual(['sub'])
  })
  it('firstparty trusts cap.audios without a row', () => {
    expect(effectiveAudios({ group: 'firstparty', audios: ['dub'] }, null)).toEqual(['dub'])
  })
})

describe('isUnverified / verifiedDubLangs / verifyFor', () => {
  it('marks non-firstparty rows without verdicts', () => {
    expect(isUnverified({ group: 'en' }, null)).toBe(true)
    expect(isUnverified({ group: 'firstparty' }, null)).toBe(false)
    expect(isUnverified({ group: 'en' }, v({ status: 'partial' }))).toBe(false)
  })
  it('extracts langs and reads the report', () => {
    expect(verifiedDubLangs(v({ dub_langs: ['en', 'ru'] }))).toEqual(['en', 'ru'])
    const rep: VerifyReport = { animeId: 'a', providers: { kodik: v({ raw: true }) } }
    expect(verifyFor(rep, 'kodik')?.raw).toBe(true)
    expect(verifyFor(rep, 'gogoanime')).toBeNull()
  })
})
```

- [ ] **Step 2: implement verifiedCaps.ts:**

```ts
import type { ProviderVerify, VerifyReport } from '@/types/contentVerify'
import type { TrackLang } from '@/types/aePlayer'

/**
 * Owner-approved hard gate (spec §5): a source with no probe verdict is
 * assumed RAW and marked "unverified"; the DUB facet lists only providers
 * with a ≥95%-verified dub language. First-party (ae) is exempt — its
 * audio/lang comes from our own library ingest and is trusted as-is.
 */
export function verifyFor(report: VerifyReport | null, provider: string): ProviderVerify | null {
  return report?.providers[provider] ?? null
}

export function isUnverified(cap: { group: string }, v: ProviderVerify | null): boolean {
  if (cap.group === 'firstparty') return false
  return !v || v.status === 'unverified'
}

export function verifiedDubLangs(v: ProviderVerify | null): TrackLang[] {
  return (v?.dub_langs ?? []).filter((l): l is TrackLang => l === 'en' || l === 'ru' || l === 'ja')
}

export function effectiveAudios(
  cap: { group: string; audios: ('sub' | 'dub')[] },
  v: ProviderVerify | null,
): ('sub' | 'dub')[] {
  if (cap.group === 'firstparty') {
    return [...new Set(cap.audios.map((a) => (a === 'dub' ? 'dub' : 'sub')))]
  }
  if (!v || v.status === 'unverified') return ['sub']
  const out = new Set<'sub' | 'dub'>()
  if (v.raw) out.add('sub')
  if (verifiedDubLangs(v).length) out.add('dub')
  if (v.status === 'partial') out.add('sub') // unverified units stay RAW-assumed
  return out.size ? [...out] : ['sub']
}
```

- [ ] **Step 3: thread through the feed.** `types/aePlayer.ts`: `ProviderRow` gains `verify?: ProviderVerify | null` (import type). `useProviderFeed.ts`:
  - `rowsFromReport(report, filter, verify: VerifyReport | null = null)`;
  - `relevant(cap, f, v)`: replace the audio checks —

```ts
  const audios = effectiveAudios(cap, v)
  if (f.audio === 'dub') {
    const langs = cap.group === 'firstparty' ? langsForCap(cap) : verifiedDubLangs(v)
    return audios.includes('dub') && langs.includes(f.lang)
  }
  return audios.includes('sub')
```

  - `toRow(cap, v)`: `audios: effectiveAudios(cap, v), verify: v` (keep the rest).
  - Update `useProviderFeed`'s exported spec accordingly (existing tests updated: pass `null` verify → previous dub rows now expect RAW-only for non-firstparty; adjust assertions to the new model).
- `useCapabilityFeed.ts`: deps gain `getVerify: () => VerifyReport | null`; `rows = computed(() => rowsFromReport(report.value, filter.value, deps.getVerify()))`.

- [ ] **Step 4: labels + chips.** `capLabels.ts`: `deriveCapLabels(cap, verify?: ProviderVerify | null)` adds to its return: `unverified: isUnverified(cap, verify ?? null)`, `verifiedDub: verifiedDubLangs(verify ?? null)`, `verifiedHardsub: (verify?.hardsub_langs ?? []) as TrackLang[]`; when `verify` has verified data, derive `categories` from it instead of variants (dub if verifiedDub non-empty, sub if raw/partial). Extend its spec with: unverified cap → `unverified: true`; verified summary → categories/badges from verdicts.

`ProviderChip.vue` (badge block at lines 32-51/63-78; ALL styling via semantic tokens — no raw colors, DS-lint gate):
  - after the category chips: verified badges `v-for="lang in labels.verifiedDub"` → `{{ t('player.sources.verifiedDub', { lang: lang.toUpperCase() }) }}` and same for `verifiedHardsub`;
  - when `labels.unverified` → muted chip `{{ t('player.sources.unverified') }}` (e.g. `border border-border text-muted-foreground` chip classes consistent with the existing state badges);
  - when unverified, suppress the old asserted SUB/DUB category chips (the whole point: no claims without verification) — render the marker instead.
`SourcePanel.vue`: pass `:verify="row.verify ?? null"` down to ProviderChip (new prop), which forwards it into `deriveCapLabels(props.cap, props.verify)`.

- [ ] **Step 5: i18n.** Add the three keys to `en.json`, `ru.json`, `ja.json` under `player.sources` (values above).

- [ ] **Step 6: run FE tests** — `bunx vitest run src/composables/aePlayer/ src/locales/__tests__/locale-parity.spec.ts` → PASS (including updated useProviderFeed/capLabels specs).

- [ ] **Step 7: Commit** — `git add frontend/web/src && git commit -m "feat(web): verified gating — unverified=RAW+marker, DUB only >=95%, verified badges"`

---

### Task 15: FE — pre-playback combo correction + debugStats + wiring

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/useComboBootstrap.ts` (+ spec)
- Modify: `frontend/web/src/composables/aePlayer/useDebugTools.ts`
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue`

**Interfaces:**
- `useComboBootstrap` deps gain `verifyReport: Ref<VerifyReport | null>` and `getHasStarted: () => boolean`. New watcher re-runs the smart default when verdicts arrive, guarded by: playback not started AND provider was auto-selected AND no room combo AND preference settled. Manual picks stay untouchable (`providerAutoSelected === false`).
- `useDebugTools` deps gain `getVerify: () => VerifyReport | null`; `debugStats` gains a `verify` line for the current provider.

- [ ] **Step 1: bootstrap watcher.** In `useComboBootstrap.ts` (after Watcher B, ~line 158):

```ts
  // Content-verify verdicts landed while the user is still reading the
  // description (spec §5): silently re-run the smart default. NEVER after
  // the first frame, NEVER over a manual pick.
  watch(deps.verifyReport, (rep) => {
    if (!rep) return
    if (deps.getHasStarted()) return
    if (roomHasCombo.value) return
    if (!preferenceSettled.value) return
    if (state.combo.value.provider && !providerAutoSelected.value) return
    const pick = pickFacetDefault()
    if (pick && pick.id !== state.combo.value.provider) {
      providerAutoSelected.value = true
      state.setProvider(pick.id, '')
      deps.recordDecision('content-verify update — re-picked best source')
    }
  })
```

(`rows` already reflect verdicts reactively via Task 14, so `pickFacetDefault()` sees the corrected pool.)

- [ ] **Step 2: bootstrap spec.** Extend the existing `useComboBootstrap` spec (or add `useComboBootstrap.verify.spec.ts`): fake deps; feed a verifyReport ref; assert (a) re-pick fires with `getHasStarted:()=>false` + auto-selected provider; (b) no re-pick when `getHasStarted:()=>true`; (c) no re-pick when `providerAutoSelected=false` (manual pin).

- [ ] **Step 3: debugStats.** `useDebugTools.ts`: deps `getVerify`; inside the `debugStats` computed add:

```ts
    const rep = deps.getVerify?.()
    const prov = deps.state.combo.value.provider
    let verify = 'unverified'
    if (rep && prov && rep.providers[prov]) {
      const v = rep.providers[prov]
      verify = `${v.status} raw:${v.raw ? '✓' : '–'} dub:[${v.dub_langs.join(',')}] hs:[${v.hardsub_langs.join(',')}] units:${v.units?.length ?? 0}`
    }
```

and include `verify` in the returned object (hacker-mode-only surface — the "new datum → debugStats" rule).

- [ ] **Step 4: AePlayer.vue wiring.**

```ts
// after usePlaybackClock (hasStarted, ~line 715) and before useCapabilityFeed (~line 746):
const verifyActive = computed(() => !hasStarted.value)
const contentVerify = useContentVerify(animeIdRef, verifyActive)
```

- `useCapabilityFeed({ ..., getVerify: () => contentVerify.report.value })`
- `useComboBootstrap({ ..., verifyReport: contentVerify.report, getHasStarted: () => hasStarted.value })`
- `useDebugTools({ ..., getVerify: () => contentVerify.report.value })`

- [ ] **Step 5: run FE suite** — `bunx vitest run src/composables/aePlayer/` → PASS; `bunx vue-tsc --noEmit` → 0 errors.

- [ ] **Step 6: Commit** — `git add frontend/web/src && git commit -m "feat(web): dynamic verify polling — pre-playback combo re-pick + hacker verify datum"`

---

### Task 16: FE gates — /frontend-verify

- [ ] **Step 1:** Run the `/frontend-verify` skill (DS-lint, i18n en/ru/ja parity, `bun run build`, lucide/TS2614/Tailwind-cascade traps). Fix anything it flags. NOTE: the `ds-lint-postedit` hook already ran per-edit; this is the full gate.
- [ ] **Step 2:** Full FE test suite: `cd frontend/web && bunx vitest run` → green.
- [ ] **Step 3:** Commit any fixes — `git add frontend/web && git commit -m "chore(web): frontend-verify gate fixes for content-verify"`

---

### Task 17: Live E2E + timing measurement (budget revisit)

No new files (spec update only). Requires the deployed stack (orchestrator runs this AFTER push + redeploy — coordinate with Task 18's after-update flow, or run redeploys from this worktree per redeploy.sh worktree support).

- [ ] **Step 1: deploy** — `make redeploy-content-verify redeploy-catalog redeploy-player redeploy-web` (content-verify first build is slow: whisper model bake). `make health` → all green.
- [ ] **Step 2: seed a visit** — open a real ongoing title (or curl the public endpoint to fire the hint):

```bash
curl -s "http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/content-verify" | jq .
docker exec animeenigma-content-verify wget -qO- "http://127.0.0.1:8101/internal/verify/queue" | jq '.data.entries[:5]'
```

Expected: queue shows the anchor near the top (score ≥ 15 + ongoing/top bonuses).
- [ ] **Step 3: watch the first probes** — `make logs-content-verify` until 3-5 "unit probed" lines; then re-curl the verdicts endpoint: units with `audio.lang` + confidence appear. Verify in the aePlayer UI (hacker mode → verify datum; Source panel → marker on unverified providers, verified badges where probed; DUB slider only lists verified dubs).
- [ ] **Step 4: MEASURE the unit budget (spec §2 revisit item).** Pull the histogram + eyeball per-provider costs:

```bash
curl -s http://localhost:8101/metrics | grep content_verify_probe_duration
```

Record: p50/p95 per run, which providers time out at 50s (browser-engine resolves are the suspects). **Update the spec's §2 "ПЕРЕСМОТРЕТЬ ПОСЛЕ ТЕСТОВ" block with the measured numbers and the decision** (keep 50s / raise budget+interval / per-provider budgets). If a change is warranted, adjust `CV_UNIT_BUDGET`/`CV_INTERVAL` defaults in config + compose + docs in the same commit.
- [ ] **Step 5: governor drill** — `bin/degradation-override.sh set 1`, confirm `content_verify_ticks_skipped_total{reason="degraded"}` increments and logs show no probing; `bin/degradation-override.sh clear`.
- [ ] **Step 6: feedback status** — `bin/feedback-status 2026-07-16T08-50-00_tNeymik_manual ai_done` (pre-authorized; `resolved` stays human-only).
- [ ] **Step 7: Commit** spec/doc updates — `git add docs/superpowers/specs/2026-07-16-content-verify-probing-design.md docs/environment-variables.md docker/docker-compose.yml services/content-verify && git commit -m "docs(content-verify): live timing measurements + budget decision"`

---

### Task 18: After-update (MANDATORY)

- [ ] **Step 1:** Invoke `/animeenigma-after-update`: /simplify over the changed code, lint+build, redeploys (whatever Task 17 didn't already cover), health checks, **Russian Trump-mode changelog** entry (prepend to `frontend/web/changelog.full.json`, regenerate `public/changelog.json` via `frontend/web/scripts/changelog-trim.mjs`), commit + push with co-authors.
- [ ] **Step 2:** Verify the pushed main passes the base-tree autosync (no divergence), then `git worktree remove` + `prune` once green.

---

## Self-Review Notes (performed at plan-writing time)

- **Spec coverage:** queue+throttle (T6/T9), governor (T9), priorities 15/10/5 (T6), membership ongoing/top/visits (T4/T3), JSONB table anime×provider×units (T2), per-unit probing kodik-teams/scraper-servers/tracks (T5), whisper LID (T7/T8), hardsub OCR (T7/T8), 95% gate (T2/T8), ae synth from library truth (T9), aePlayer dynamic load + RAW-default + marker + DUB gate + badges (T13/T14), pre-playback combo correction (T15), hacker debugStats (T15), deploy+env docs+ports (T10), budget-revisit TODO (T17), OP/ED-timings future extension — no task needed (spec §8 records that it extends THIS service; sampleOffsets already consumes intro/outro markers where providers ship them).
- **Type consistency:** `domain.UnitVerdict`/`ProviderSummary` JSON in T2 == wire consumed in T11 (`VerifySummary`) and T13 (`ProviderVerify`); `queue.Unit` produced in T5 == consumed by T8/T9; `LIDResult`/`HardsubResult` JSON in T7 == structs in T8.
- **Known uncertainty flagged inline:** exact field names of scraper `Track`, kodik stream response, `cache.Cache` method shapes, and collector-middleware naming — each task names the authoritative file to mirror; executors must check those files, not guess.
