# Anidle Plan 2a — Service Skeleton + Pool Client + Comparison Engine

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Stand up a new `anidle` Go microservice (port 8095) that boots with a `/health` endpoint, loads the franchise-collapsed guess pool from catalog's `GET /internal/guessgame/pool` (cached in Redis + indexed in memory), and contains a fully unit-tested pure comparison engine (🟩/🟨/⬜ + ↑/↓). No game endpoints yet — those are Plan 2b.

**Architecture:** Mirror the recently-extracted `recs` service (own Postgres+Redis, JWT validated in-service, gateway reverse-proxy). This plan builds the *scaffolding* + *read-only data layer* + *pure logic*; Plan 2b adds the daily/endless game API + persistence on top. The pool is fetched from catalog over the Docker network and cached, so the engine + game logic operate on local data.

**Tech Stack:** Go 1.25, chi/v5, GORM (Postgres, sqlite for tests), `libs/{database,cache,authz,httputil,logger,errors,metrics,tracing}`, testify.

**Reference spec:** `docs/superpowers/specs/2026-06-15-anidle-anime-guessing-game-design.md` §3, §4.2, §2.2–2.3. **Consumes** the Plan-1 endpoint `GET /internal/guessgame/pool` (live; returns `{success, data:[…]}` with `id,name_ru,name_en,name_jp,poster_url,year,episodes,score,status,rating,genres[],studios[],tags[]`).

---

## File Structure

```
services/anidle/
├── cmd/anidle-api/main.go          # bootstrap (health + pool warm; no game routes yet)
├── internal/
│   ├── config/config.go            # mirror recs config, port 8095
│   ├── domain/anime.go             # PoolAnime + Taxon (decoded pool entry)
│   ├── domain/compare.go           # ColumnResult / GuessComparison types
│   ├── service/poolclient.go       # HTTP client → catalog /internal/guessgame/pool
│   ├── service/poolstore.go        # Redis cache + in-memory index (Get/Lookup/Search)
│   ├── service/engine.go           # pure comparison engine
│   ├── service/engine_test.go
│   ├── service/poolclient_test.go
│   ├── service/poolstore_test.go
│   ├── handler/health.go           # GET /health
│   └── transport/router.go         # chi router (health only for 2a)
├── Dockerfile
└── go.mod / go.sum
```

---

## Task 1: Create the `anidle` Go module + go.work entry

**Files:**
- Create: `services/anidle/go.mod`
- Modify: `go.work`

- [ ] **Step 1: Create `services/anidle/go.mod`**

```
module github.com/ILITA-hub/animeenigma/services/anidle

go 1.25.0

require (
	github.com/ILITA-hub/animeenigma/libs/authz v0.0.0
	github.com/ILITA-hub/animeenigma/libs/cache v0.0.0
	github.com/ILITA-hub/animeenigma/libs/database v0.0.0
	github.com/ILITA-hub/animeenigma/libs/errors v0.0.0
	github.com/ILITA-hub/animeenigma/libs/httputil v0.0.0
	github.com/ILITA-hub/animeenigma/libs/logger v0.0.0
	github.com/ILITA-hub/animeenigma/libs/metrics v0.0.0
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
	github.com/go-chi/chi/v5 v5.2.5
	github.com/prometheus/client_golang v1.23.2
	github.com/redis/go-redis/v9 v9.7.0
	github.com/stretchr/testify v1.11.1
	gorm.io/driver/sqlite v1.6.0
	gorm.io/gorm v1.30.0
)

replace (
	github.com/ILITA-hub/animeenigma/libs/authz => ../../libs/authz
	github.com/ILITA-hub/animeenigma/libs/cache => ../../libs/cache
	github.com/ILITA-hub/animeenigma/libs/database => ../../libs/database
	github.com/ILITA-hub/animeenigma/libs/errors => ../../libs/errors
	github.com/ILITA-hub/animeenigma/libs/httputil => ../../libs/httputil
	github.com/ILITA-hub/animeenigma/libs/logger => ../../libs/logger
	github.com/ILITA-hub/animeenigma/libs/metrics => ../../libs/metrics
	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
)
```

> Note: the `redis/go-redis/v9` version must match what `libs/cache` uses. After Step 2, `go mod tidy` will reconcile exact patch versions and fill indirect requires + `go.sum`.

- [ ] **Step 2: Add `./services/anidle` to `go.work`**

In `go.work`, add `./services/anidle` to the `use (...)` block (keep the list alphabetical — place it right before `./services/auth`... actually after `./services/analytics`):

```
	./services/analytics
	./services/anidle
	./services/auth
```

- [ ] **Step 3: Sync the workspace + tidy the new module**

Run:
```bash
cd /data/animeenigma && go work sync
cd /data/animeenigma/services/anidle && go mod tidy
```
Expected: `go.mod` gains indirect requires; `services/anidle/go.sum` is created. No errors. (There are no `.go` files yet, so `go build ./...` will report "no Go files" — that's fine until Task 2.)

- [ ] **Step 4: Commit**

```bash
git add go.work services/anidle/go.mod services/anidle/go.sum
git commit -m "chore(anidle): scaffold go module + go.work entry"
```

---

## Task 2: Minimal bootable service (config, health, router, main, Dockerfile)

**Files:**
- Create: `services/anidle/internal/config/config.go`
- Create: `services/anidle/internal/handler/health.go`
- Create: `services/anidle/internal/transport/router.go`
- Create: `services/anidle/cmd/anidle-api/main.go`
- Create: `services/anidle/Dockerfile`

- [ ] **Step 1: Create `internal/config/config.go`**

```go
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/authz"
	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/database"
)

type Config struct {
	Server      ServerConfig
	Database    database.Config
	Redis       cache.Config
	JWT         authz.JWTConfig
	CatalogURL  string
	PoolTTL     time.Duration
}

type ServerConfig struct {
	Host string
	Port int
}

func (s ServerConfig) Address() string { return fmt.Sprintf("%s:%d", s.Host, s.Port) }

func Load() (*Config, error) {
	if getEnv("JWT_SECRET", "") == "" {
		return nil, fmt.Errorf("JWT_SECRET environment variable is required")
	}
	return &Config{
		Server: ServerConfig{
			Host: getEnv("SERVER_HOST", "0.0.0.0"),
			Port: getEnvInt("SERVER_PORT", 8095),
		},
		Database: database.Config{
			Host:     getEnv("DB_HOST", "localhost"),
			Port:     getEnvInt("DB_PORT", 5432),
			User:     getEnv("DB_USER", "postgres"),
			Password: getEnv("DB_PASSWORD", "postgres"),
			Database: getEnv("DB_NAME", "animeenigma"),
			SSLMode:  getEnv("DB_SSLMODE", "disable"),
		},
		Redis: cache.Config{
			Host:     getEnv("REDIS_HOST", "redis"),
			Port:     getEnvInt("REDIS_PORT", 6379),
			Password: getEnv("REDIS_PASSWORD", ""),
			DB:       getEnvInt("REDIS_DB", 0),
		},
		JWT: authz.JWTConfig{
			Secret:          getEnv("JWT_SECRET", ""),
			Issuer:          getEnv("JWT_ISSUER", "animeenigma"),
			AccessTokenTTL:  getEnvDuration("JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: getEnvDuration("JWT_REFRESH_TTL", 7*24*time.Hour),
		},
		CatalogURL: getEnv("CATALOG_URL", "http://catalog:8081"),
		PoolTTL:    getEnvDuration("ANIDLE_POOL_TTL", 12*time.Hour),
	}, nil
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

- [ ] **Step 2: Create `internal/handler/health.go`**

```go
package handler

import (
	"net/http"

	"github.com/ILITA-hub/animeenigma/libs/httputil"
)

// HealthHandler serves GET /health.
type HealthHandler struct{}

func NewHealthHandler() *HealthHandler { return &HealthHandler{} }

func (h *HealthHandler) Health(w http.ResponseWriter, r *http.Request) {
	httputil.OK(w, map[string]string{"status": "ok", "service": "anidle"})
}
```

- [ ] **Step 3: Create `internal/transport/router.go`**

```go
package transport

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/handler"
)

func NewRouter(healthHandler *handler.HealthHandler, log *logger.Logger, mc *metrics.Collector) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	if mc != nil {
		r.Use(mc.Middleware)
	}

	r.Get("/health", healthHandler.Health)
	r.Handle("/metrics", metrics.Handler())

	return r
}
```

> If `metrics.Collector` has no `Middleware` method or `metrics.Handler()` differs, match the real signatures used in `services/recs/internal/transport/router.go` — read it and mirror exactly. The point of this step is a router that serves `/health` and `/metrics`.

- [ ] **Step 4: Create `cmd/anidle-api/main.go`**

```go
// Package main is the anidle service entrypoint (port 8095).
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
	gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/config"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/transport"
)

func main() {
	log := logger.Default()
	defer func() { _ = log.Sync() }()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalw("failed to load config", "error", err)
	}

	tracer, err := tracing.InitFromEnv(context.Background(), "anidle")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_ = tracer.Shutdown(ctx)
		}()
	}

	db, err := database.New(cfg.Database)
	if err != nil {
		log.Fatalw("failed to connect to database", "error", err)
	}
	defer db.Close()
	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
		log.Warnw("gorm tracing disabled", "error", err)
	}
	if sqlDB, derr := db.DB.DB(); derr == nil {
		metrics.StartDBPoolCollector(sqlDB, 15*time.Second)
	}
	// Plan 2b adds db.AutoMigrate(...) for game tables.

	redisCache, err := cache.New(cfg.Redis)
	if err != nil {
		log.Fatalw("failed to connect to redis", "host", cfg.Redis.Host, "error", err)
	}
	defer redisCache.Close()

	healthHandler := handler.NewHealthHandler()
	mc := metrics.NewCollector("anidle")
	router := transport.NewRouter(healthHandler, log, mc)

	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("anidle")(router),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Infow("starting anidle service", "address", cfg.Server.Address())
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalw("failed to start server", "error", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info("shutting down anidle service...")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalw("server forced to shutdown", "error", err)
	}
	log.Info("anidle service stopped")
}
```

> `redisCache` is unused until Task 7; if the compiler complains about an unused variable, keep it — it's used by `defer redisCache.Close()`, so it is referenced. The `db` warm path is referenced via `defer db.Close()`. Both are fine.

- [ ] **Step 5: Create `services/anidle/Dockerfile`** — clone of recs Dockerfile with anidle names. Copy `services/recs/Dockerfile` verbatim, then change: every `recs` → `anidle`, `/recs-api` → `/anidle-api`, `EXPOSE 8094` → `EXPOSE 8095`, and ADD a line `COPY services/anidle/go.mod services/anidle/go.sum* ./services/anidle/` in the module-copy block (right after the recs COPY line). The full module-copy block must list every workspace module (read the current `services/recs/Dockerfile` and copy it exactly, then add the anidle line). Final form:

```dockerfile
FROM golang:1.25-alpine AS builder
WORKDIR /app
RUN apk add --no-cache git
COPY go.work go.work.sum ./
# ... (all the existing COPY libs/*/go.mod and services/*/go.mod lines from recs Dockerfile, verbatim) ...
COPY services/recs/go.mod services/recs/go.sum* ./services/recs/
COPY services/anidle/go.mod services/anidle/go.sum* ./services/anidle/
RUN cd services/anidle && go mod download
COPY libs/ ./libs/
COPY services/anidle/ ./services/anidle/
RUN cd services/anidle && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /anidle-api ./cmd/anidle-api
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata wget
WORKDIR /app
COPY --from=builder /anidle-api .
EXPOSE 8095
CMD ["./anidle-api"]
```

- [ ] **Step 6: Build locally**

Run: `cd /data/animeenigma/services/anidle && go build ./...`
Expected: success (no output).

- [ ] **Step 7: Commit**

```bash
git add services/anidle/internal services/anidle/cmd services/anidle/Dockerfile services/anidle/go.mod services/anidle/go.sum
git commit -m "feat(anidle): bootable service skeleton (config, health, router, main, Dockerfile)"
```

---

## Task 3: Peer-Dockerfile fan-out (go.work validation fix)

Every service Dockerfile copies the root `go.work` and runs `go mod download`, which validates that EVERY module in `go.work` has its `go.mod` present in the build context. Since Task 1 added `./services/anidle` to `go.work`, **every peer service Dockerfile** must now also `COPY services/anidle/go.mod`. Missing this makes that service's build fail with "cannot load module … listed in go.work file". (Sidecars without go.work — `animepahe-resolver`, `megacloud-extractor` — are exempt.)

**Files:** every `services/*/Dockerfile` that contains a `COPY services/recs/go.mod` line.

- [ ] **Step 1: Add the COPY line to all peer Dockerfiles**

Run this from repo root — it inserts the anidle COPY line immediately after the recs COPY line in every service Dockerfile that has one (idempotent: skips files already patched):

```bash
cd /data/animeenigma
for df in services/*/Dockerfile; do
  if grep -q 'COPY services/recs/go.mod' "$df" && ! grep -q 'COPY services/anidle/go.mod' "$df"; then
    sed -i 's#\(COPY services/recs/go.mod services/recs/go.sum\* ./services/recs/\)#\1\nCOPY services/anidle/go.mod services/anidle/go.sum* ./services/anidle/#' "$df"
    echo "patched $df"
  fi
done
```

Expected: prints `patched services/<name>/Dockerfile` for each of auth, catalog, streaming, player, rooms, scraper, scheduler, gateway, themes, notifications, watch-together, analytics, library, gacha, maintenance, recs (recs' own Dockerfile gets it too — harmless, anidle is a peer). The anidle Dockerfile already has it (Task 2) so it's skipped.

- [ ] **Step 2: Verify each patched Dockerfile has exactly one anidle COPY line**

Run: `grep -c 'COPY services/anidle/go.mod' services/*/Dockerfile`
Expected: each listed service shows `1` (none show `2`).

- [ ] **Step 3: Commit**

```bash
git add services/*/Dockerfile
git commit -m "chore(anidle): COPY anidle go.mod in all peer service Dockerfiles (go.work validation)"
```

---

## Task 4: docker-compose `anidle` service + gateway env

**Files:**
- Modify: `docker/docker-compose.yml`

- [ ] **Step 1: Add the `anidle` service block**

Copy the `recs:` block (find it in `docker/docker-compose.yml`) and add an adapted `anidle:` block next to it:

```yaml
  anidle:
    build:
      context: ..
      dockerfile: services/anidle/Dockerfile
    container_name: animeenigma-anidle
    restart: unless-stopped
    environment:
      SERVER_PORT: 8095
      DB_HOST: postgres
      DB_PORT: 5432
      DB_USER: postgres
      DB_PASSWORD: postgres
      DB_NAME: animeenigma
      JWT_SECRET: ${JWT_SECRET:-dev-secret-change-in-production}
      REDIS_HOST: redis
      CATALOG_URL: http://catalog:8081
      TRACING_ENABLED: "true"
    ports:
      - "127.0.0.1:8095:8095"
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy
      catalog:
        condition: service_started
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "-O", "/dev/null", "http://localhost:8095/health"]
      interval: 30s
      timeout: 5s
      retries: 3
```

- [ ] **Step 2: Add anidle to the gateway service**

In the `gateway:` service block, add to `environment:`:
```yaml
      ANIDLE_SERVICE_URL: http://anidle:8095
```
and add to gateway's `depends_on:`:
```yaml
      anidle:
        condition: service_started
```

- [ ] **Step 3: Validate compose**

Run: `cd /data/animeenigma && docker compose -f docker/docker-compose.yml config >/dev/null && echo "compose OK"`
Expected: `compose OK` (no YAML errors).

- [ ] **Step 4: Commit**

```bash
git add docker/docker-compose.yml
git commit -m "feat(anidle): docker-compose service (8095) + gateway env"
```

---

## Task 5: Gateway routing for `/api/anidle/*`

**Files:**
- Modify: `services/gateway/internal/config/config.go`
- Modify: `services/gateway/internal/handler/proxy.go`
- Modify: `services/gateway/internal/service/proxy.go`
- Modify: `services/gateway/internal/transport/router.go`

- [ ] **Step 1: Config — add the upstream URL**

In `services/gateway/internal/config/config.go`, in the `ServiceURLs` struct add (next to `RecsService`):
```go
	AnidleService string
```
and in `Load()` (next to the `RecsService:` line):
```go
		AnidleService: getEnv("ANIDLE_SERVICE_URL", "http://anidle:8095"),
```

- [ ] **Step 2: Proxy switch — map the service name to its URL**

In `services/gateway/internal/service/proxy.go`, in the `getServiceURL` switch (next to `case "recs"`):
```go
	case "anidle":
		return s.serviceURLs.AnidleService, nil
```

- [ ] **Step 3: Proxy handler — add the shim**

In `services/gateway/internal/handler/proxy.go` (next to `ProxyToRecs`):
```go
func (h *ProxyHandler) ProxyToAnidle(w http.ResponseWriter, r *http.Request) {
	h.proxy(w, r, "anidle")
}
```

- [ ] **Step 4: Router — register the route group (optional JWT)**

In `services/gateway/internal/transport/router.go`, add a route group near the recs registration. Anidle is guest-friendly, so the whole family is OPTIONAL-JWT; Plan 2b's handlers gate logged-in features themselves:
```go
	// Anidle guessing game (spec 2026-06-15) — guest-friendly, JWT optional.
	r.Group(func(r chi.Router) {
		r.Use(OptionalJWTValidationMiddleware(cfg.JWT, cfg.Services.AuthService))
		r.Use(userRateLimit)
		r.HandleFunc("/anidle/*", proxyHandler.ProxyToAnidle)
	})
```

> Match the exact field name the router uses for the auth service URL (`cfg.Services.AuthService` in the recs example) and the exact `userRateLimit` middleware reference by reading the surrounding recs registration.

- [ ] **Step 5: Build the gateway**

Run: `cd /data/animeenigma/services/gateway && go build ./...`
Expected: success.

- [ ] **Step 6: Commit**

```bash
git add services/gateway/internal/config/config.go services/gateway/internal/handler/proxy.go services/gateway/internal/service/proxy.go services/gateway/internal/transport/router.go
git commit -m "feat(gateway): route /api/anidle/* to anidle:8095 (optional JWT)"
```

---

## Task 6: Pool client (catalog `/internal/guessgame/pool` → typed entries)

**Files:**
- Create: `services/anidle/internal/domain/anime.go`
- Create: `services/anidle/internal/service/poolclient.go`
- Test: `services/anidle/internal/service/poolclient_test.go`

- [ ] **Step 1: Define the domain types**

Create `services/anidle/internal/domain/anime.go`:
```go
package domain

// Taxon is an id+name pair (genre/studio/tag). anidle compares by ID.
type Taxon struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PoolAnime is one guessable anime, decoded from catalog's
// GET /internal/guessgame/pool. Field shape MUST match the catalog DTO
// (services/catalog/internal/service/guesspool.go GuessPoolEntry).
type PoolAnime struct {
	ID        string  `json:"id"`
	NameRU    string  `json:"name_ru"`
	NameEN    string  `json:"name_en"`
	NameJP    string  `json:"name_jp"`
	PosterURL string  `json:"poster_url"`
	Year      int     `json:"year"`
	Episodes  int     `json:"episodes"`
	Score     float64 `json:"score"`
	Status    string  `json:"status"`
	Rating    string  `json:"rating"`
	Genres    []Taxon `json:"genres"`
	Studios   []Taxon `json:"studios"`
	Tags      []Taxon `json:"tags"`
}
```

- [ ] **Step 2: Write the failing test**

Create `services/anidle/internal/service/poolclient_test.go`:
```go
package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolClient_Fetch_DecodesEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "/internal/guessgame/pool", r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":[
			{"id":"frieren","name_ru":"Фрирен","poster_url":"p","year":2023,"episodes":28,
			 "score":9.3,"status":"released","rating":"pg_13",
			 "genres":[{"id":"1","name":"Драма"}],"studios":[{"id":"s","name":"Madhouse"}],"tags":[]}
		]}`))
	}))
	defer srv.Close()

	c := NewPoolClient(srv.URL, 5*time.Second, nil)
	pool, err := c.Fetch(context.Background())
	require.NoError(t, err)
	require.Len(t, pool, 1)
	assert.Equal(t, "frieren", pool[0].ID)
	assert.Equal(t, 2023, pool[0].Year)
	assert.Equal(t, "Madhouse", pool[0].Studios[0].Name)
	assert.Empty(t, pool[0].Tags)
}

func TestPoolClient_Fetch_ErrorOnNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := NewPoolClient(srv.URL, 5*time.Second, nil).Fetch(context.Background())
	require.Error(t, err)
}
```

- [ ] **Step 2b: Run it (fails)**

Run: `cd /data/animeenigma/services/anidle && go test ./internal/service/ -run TestPoolClient -v`
Expected: FAIL — `NewPoolClient` undefined.

- [ ] **Step 3: Implement the client**

Create `services/anidle/internal/service/poolclient.go`:
```go
package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// PoolClient fetches the guess pool from catalog's internal endpoint.
type PoolClient struct {
	baseURL string
	client  *http.Client
	log     *logger.Logger
}

func NewPoolClient(catalogURL string, timeout time.Duration, log *logger.Logger) *PoolClient {
	if timeout <= 0 {
		timeout = 60 * time.Second // first build backfills franchises; allow time
	}
	return &PoolClient{
		baseURL: catalogURL,
		client:  &http.Client{Timeout: timeout},
		log:     log,
	}
}

type poolEnvelope struct {
	Success bool               `json:"success"`
	Data    []domain.PoolAnime `json:"data"`
}

// Fetch GETs /internal/guessgame/pool and decodes the {success,data} envelope.
func (c *PoolClient) Fetch(ctx context.Context) ([]domain.PoolAnime, error) {
	endpoint := c.baseURL + "/internal/guessgame/pool"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, fmt.Errorf("build pool request: %w", err)
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("pool request: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("pool endpoint returned %d: %s", resp.StatusCode, string(body))
	}

	var env poolEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return nil, fmt.Errorf("decode pool envelope: %w", err)
	}
	return env.Data, nil // read .Data, never the top-level object
}
```

- [ ] **Step 4: Run tests (pass)**

Run: `cd /data/animeenigma/services/anidle && go test ./internal/service/ -run TestPoolClient -v`
Expected: PASS (both).

- [ ] **Step 5: Commit**

```bash
git add services/anidle/internal/domain/anime.go services/anidle/internal/service/poolclient.go services/anidle/internal/service/poolclient_test.go
git commit -m "feat(anidle): pool HTTP client with envelope decode"
```

---

## Task 7: Pool store (Redis cache + in-memory index)

**Files:**
- Create: `services/anidle/internal/service/poolstore.go`
- Test: `services/anidle/internal/service/poolstore_test.go`

The store caches the pool JSON in Redis (`anidle:pool`, TTL from config) and builds an in-memory map (id → PoolAnime) + a lowercase name index for autocomplete. On a miss it calls the PoolClient and repopulates.

- [ ] **Step 1: Write the failing test**

Create `services/anidle/internal/service/poolstore_test.go`:
```go
package service

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

// fakeCache implements the subset of libs/cache.Cache that PoolStore uses.
type fakeCache struct {
	mu    sync.Mutex
	store map[string][]byte
}

func newFakeCache() *fakeCache { return &fakeCache{store: map[string][]byte{}} }

func (f *fakeCache) Get(_ context.Context, key string, dest interface{}) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	b, ok := f.store[key]
	if !ok {
		return errCacheMiss
	}
	return json.Unmarshal(b, dest)
}

func (f *fakeCache) Set(_ context.Context, key string, value interface{}, _ time.Duration) error {
	b, _ := json.Marshal(value)
	f.mu.Lock()
	f.store[key] = b
	f.mu.Unlock()
	return nil
}

// fakeFetcher implements poolFetcher.
type fakeFetcher struct {
	pool  []domain.PoolAnime
	calls int
}

func (f *fakeFetcher) Fetch(_ context.Context) ([]domain.PoolAnime, error) {
	f.calls++
	return f.pool, nil
}

func samplePool() []domain.PoolAnime {
	return []domain.PoolAnime{
		{ID: "frieren", NameRU: "Фрирен", NameEN: "Frieren"},
		{ID: "jjk", NameRU: "Магическая битва", NameEN: "Jujutsu Kaisen"},
	}
}

func TestPoolStore_FetchesOnceThenServesFromCache(t *testing.T) {
	fetch := &fakeFetcher{pool: samplePool()}
	store := NewPoolStore(newFakeCache(), fetch, time.Hour, nil)

	p1, err := store.All(context.Background())
	require.NoError(t, err)
	require.Len(t, p1, 2)

	_, err = store.All(context.Background())
	require.NoError(t, err)
	assert.Equal(t, 1, fetch.calls, "second All() must hit the in-memory/Redis cache, not refetch")
}

func TestPoolStore_LookupAndSearch(t *testing.T) {
	store := NewPoolStore(newFakeCache(), &fakeFetcher{pool: samplePool()}, time.Hour, nil)
	_, err := store.All(context.Background())
	require.NoError(t, err)

	a, ok := store.Lookup("jjk")
	require.True(t, ok)
	assert.Equal(t, "Магическая битва", a.NameRU)

	_, ok = store.Lookup("missing")
	assert.False(t, ok)

	res := store.Search(context.Background(), "маг", 10)
	require.Len(t, res, 1)
	assert.Equal(t, "jjk", res[0].ID)

	res = store.Search(context.Background(), "frie", 10)
	require.Len(t, res, 1)
	assert.Equal(t, "frieren", res[0].ID)
}
```

- [ ] **Step 1b: Run it (fails)**

Run: `cd /data/animeenigma/services/anidle && go test ./internal/service/ -run TestPoolStore -v`
Expected: FAIL — `NewPoolStore`/`errCacheMiss`/`poolFetcher` undefined.

- [ ] **Step 2: Implement the store**

Create `services/anidle/internal/service/poolstore.go`:
```go
package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/cache"
	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

const poolCacheKey = "anidle:pool"

// errCacheMiss lets tests assert a miss without importing libs/cache; the real
// cache returns cache.ErrNotFound which we treat identically.
var errCacheMiss = errors.New("cache miss")

type poolCache interface {
	Get(ctx context.Context, key string, dest interface{}) error
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
}

type poolFetcher interface {
	Fetch(ctx context.Context) ([]domain.PoolAnime, error)
}

// PoolStore caches the guess pool (Redis + in-memory index).
type PoolStore struct {
	cache   poolCache
	fetcher poolFetcher
	ttl     time.Duration
	log     *logger.Logger

	mu     sync.RWMutex
	byID   map[string]domain.PoolAnime
	all    []domain.PoolAnime
	loaded bool
}

func NewPoolStore(c poolCache, f poolFetcher, ttl time.Duration, log *logger.Logger) *PoolStore {
	return &PoolStore{cache: c, fetcher: f, ttl: ttl, log: log}
}

// All returns the full pool, loading it (Redis → catalog) on first use.
func (s *PoolStore) All(ctx context.Context) ([]domain.PoolAnime, error) {
	s.mu.RLock()
	if s.loaded {
		all := s.all
		s.mu.RUnlock()
		return all, nil
	}
	s.mu.RUnlock()

	pool, err := s.load(ctx)
	if err != nil {
		return nil, err
	}
	return pool, nil
}

func (s *PoolStore) load(ctx context.Context) ([]domain.PoolAnime, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loaded { // another goroutine won the race
		return s.all, nil
	}

	var pool []domain.PoolAnime
	if err := s.cache.Get(ctx, poolCacheKey, &pool); err != nil {
		if !errors.Is(err, cache.ErrNotFound) && !errors.Is(err, errCacheMiss) {
			if s.log != nil {
				s.log.Warnw("pool cache get failed; refetching", "error", err)
			}
		}
		fetched, ferr := s.fetcher.Fetch(ctx)
		if ferr != nil {
			return nil, ferr
		}
		pool = fetched
		if serr := s.cache.Set(ctx, poolCacheKey, pool, s.ttl); serr != nil && s.log != nil {
			s.log.Warnw("pool cache set failed", "error", serr)
		}
	}

	s.byID = make(map[string]domain.PoolAnime, len(pool))
	for _, a := range pool {
		s.byID[a.ID] = a
	}
	s.all = pool
	s.loaded = true
	return pool, nil
}

// Lookup returns the pool anime by id (after All has loaded the pool).
func (s *PoolStore) Lookup(id string) (domain.PoolAnime, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	a, ok := s.byID[id]
	return a, ok
}

// Search returns up to limit entries whose RU/EN/JP name contains q (case-insensitive).
func (s *PoolStore) Search(ctx context.Context, q string, limit int) []domain.PoolAnime {
	if _, err := s.All(ctx); err != nil {
		return nil
	}
	q = strings.ToLower(strings.TrimSpace(q))
	s.mu.RLock()
	defer s.mu.RUnlock()
	if q == "" {
		return nil
	}
	out := make([]domain.PoolAnime, 0, limit)
	for _, a := range s.all {
		if strings.Contains(strings.ToLower(a.NameRU), q) ||
			strings.Contains(strings.ToLower(a.NameEN), q) ||
			strings.Contains(strings.ToLower(a.NameJP), q) {
			out = append(out, a)
			if len(out) >= limit {
				break
			}
		}
	}
	return out
}
```

- [ ] **Step 3: Run tests (pass)**

Run: `cd /data/animeenigma/services/anidle && go test ./internal/service/ -run TestPoolStore -v`
Expected: PASS (both).

- [ ] **Step 4: Commit**

```bash
git add services/anidle/internal/service/poolstore.go services/anidle/internal/service/poolstore_test.go
git commit -m "feat(anidle): pool store with Redis cache + in-memory index"
```

---

## Task 8: Comparison engine (pure)

**Files:**
- Create: `services/anidle/internal/domain/compare.go`
- Create: `services/anidle/internal/service/engine.go`
- Test: `services/anidle/internal/service/engine_test.go`

Implements spec §2.3: set columns (genres/studios/tags) → correct/partial/wrong; numeric (year/episodes/score) → correct or wrong+hint; enum (status/rating) → correct/wrong.

- [ ] **Step 1: Define result types**

Create `services/anidle/internal/domain/compare.go`:
```go
package domain

// MatchStatus is the per-column verdict.
type MatchStatus string

const (
	MatchCorrect MatchStatus = "correct" // 🟩
	MatchPartial MatchStatus = "partial" // 🟨
	MatchWrong   MatchStatus = "wrong"   // ⬜
)

// Hint is the numeric direction (secret relative to the guess).
type Hint string

const (
	HintHigher Hint = "higher" // ↑
	HintLower  Hint = "lower"  // ↓
	HintNone   Hint = ""
)

// ColumnResult is one cell of a guess row.
type ColumnResult struct {
	Status MatchStatus `json:"status"`
	Hint   Hint        `json:"hint,omitempty"`
}

// GuessComparison is the full per-column result for one guess.
type GuessComparison struct {
	Genres   ColumnResult `json:"genres"`
	Studios  ColumnResult `json:"studios"`
	Year     ColumnResult `json:"year"`
	Episodes ColumnResult `json:"episodes"`
	Score    ColumnResult `json:"score"`
	Status   ColumnResult `json:"status"`
	Rating   ColumnResult `json:"rating"`
	Tags     ColumnResult `json:"tags"`
}
```

- [ ] **Step 2: Write the failing test**

Create `services/anidle/internal/service/engine_test.go`:
```go
package service

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"
)

func tx(ids ...string) []domain.Taxon {
	out := make([]domain.Taxon, 0, len(ids))
	for _, id := range ids {
		out = append(out, domain.Taxon{ID: id, Name: id})
	}
	return out
}

func TestCompare_SetColumns(t *testing.T) {
	secret := domain.PoolAnime{Genres: tx("a", "b")}
	// equal set -> correct
	assert.Equal(t, domain.MatchCorrect, Compare(secret, domain.PoolAnime{Genres: tx("a", "b")}).Genres.Status)
	// overlap -> partial
	assert.Equal(t, domain.MatchPartial, Compare(secret, domain.PoolAnime{Genres: tx("a", "c")}).Genres.Status)
	// disjoint -> wrong
	assert.Equal(t, domain.MatchWrong, Compare(secret, domain.PoolAnime{Genres: tx("x", "y")}).Genres.Status)
	// both empty -> correct (equal empty sets)
	assert.Equal(t, domain.MatchCorrect, Compare(domain.PoolAnime{}, domain.PoolAnime{}).Genres.Status)
}

func TestCompare_Numeric(t *testing.T) {
	secret := domain.PoolAnime{Year: 2023, Episodes: 28, Score: 9.3}
	r := Compare(secret, domain.PoolAnime{Year: 2020, Episodes: 24, Score: 8.6})
	assert.Equal(t, domain.MatchWrong, r.Year.Status)
	assert.Equal(t, domain.HintHigher, r.Year.Hint) // secret 2023 > guess 2020
	assert.Equal(t, domain.HintHigher, r.Episodes.Hint)
	assert.Equal(t, domain.HintHigher, r.Score.Hint)

	r2 := Compare(secret, domain.PoolAnime{Year: 2025, Episodes: 28, Score: 9.3})
	assert.Equal(t, domain.HintLower, r2.Year.Hint) // secret 2023 < guess 2025
	assert.Equal(t, domain.MatchCorrect, r2.Episodes.Status)
	assert.Equal(t, domain.HintNone, r2.Episodes.Hint)
	assert.Equal(t, domain.MatchCorrect, r2.Score.Status)
}

func TestCompare_Enum(t *testing.T) {
	secret := domain.PoolAnime{Status: "released", Rating: "pg_13"}
	r := Compare(secret, domain.PoolAnime{Status: "released", Rating: "r"})
	assert.Equal(t, domain.MatchCorrect, r.Status.Status)
	assert.Equal(t, domain.MatchWrong, r.Rating.Status)
}

func TestCompare_FullMatchAllGreen(t *testing.T) {
	a := domain.PoolAnime{
		Year: 2023, Episodes: 28, Score: 9.3, Status: "released", Rating: "pg_13",
		Genres: tx("a"), Studios: tx("s"), Tags: tx("t"),
	}
	r := Compare(a, a)
	for _, c := range []domain.ColumnResult{r.Genres, r.Studios, r.Year, r.Episodes, r.Score, r.Status, r.Rating, r.Tags} {
		assert.Equal(t, domain.MatchCorrect, c.Status)
		assert.Equal(t, domain.HintNone, c.Hint)
	}
}
```

- [ ] **Step 2b: Run it (fails)**

Run: `cd /data/animeenigma/services/anidle && go test ./internal/service/ -run TestCompare -v`
Expected: FAIL — `Compare` undefined.

- [ ] **Step 3: Implement the engine**

Create `services/anidle/internal/service/engine.go`:
```go
package service

import "github.com/ILITA-hub/animeenigma/services/anidle/internal/domain"

// Compare scores a guess against the secret, per spec §2.3. Pure function.
func Compare(secret, guess domain.PoolAnime) domain.GuessComparison {
	return domain.GuessComparison{
		Genres:   compareSet(taxonIDs(secret.Genres), taxonIDs(guess.Genres)),
		Studios:  compareSet(taxonIDs(secret.Studios), taxonIDs(guess.Studios)),
		Tags:     compareSet(taxonIDs(secret.Tags), taxonIDs(guess.Tags)),
		Year:     compareInt(secret.Year, guess.Year),
		Episodes: compareInt(secret.Episodes, guess.Episodes),
		Score:    compareFloat(secret.Score, guess.Score),
		Status:   compareEnum(secret.Status, guess.Status),
		Rating:   compareEnum(secret.Rating, guess.Rating),
	}
}

func taxonIDs(ts []domain.Taxon) map[string]struct{} {
	m := make(map[string]struct{}, len(ts))
	for _, t := range ts {
		m[t.ID] = struct{}{}
	}
	return m
}

func compareSet(secret, guess map[string]struct{}) domain.ColumnResult {
	if len(secret) == len(guess) {
		equal := true
		for id := range secret {
			if _, ok := guess[id]; !ok {
				equal = false
				break
			}
		}
		if equal {
			return domain.ColumnResult{Status: domain.MatchCorrect}
		}
	}
	for id := range guess {
		if _, ok := secret[id]; ok {
			return domain.ColumnResult{Status: domain.MatchPartial}
		}
	}
	return domain.ColumnResult{Status: domain.MatchWrong}
}

func compareInt(secret, guess int) domain.ColumnResult {
	switch {
	case secret == guess:
		return domain.ColumnResult{Status: domain.MatchCorrect}
	case secret > guess:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintHigher}
	default:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintLower}
	}
}

func compareFloat(secret, guess float64) domain.ColumnResult {
	switch {
	case secret == guess:
		return domain.ColumnResult{Status: domain.MatchCorrect}
	case secret > guess:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintHigher}
	default:
		return domain.ColumnResult{Status: domain.MatchWrong, Hint: domain.HintLower}
	}
}

func compareEnum(secret, guess string) domain.ColumnResult {
	if secret == guess {
		return domain.ColumnResult{Status: domain.MatchCorrect}
	}
	return domain.ColumnResult{Status: domain.MatchWrong}
}
```

- [ ] **Step 4: Run tests (pass)**

Run: `cd /data/animeenigma/services/anidle && go test ./internal/service/ -run TestCompare -v`
Expected: PASS (all four).

- [ ] **Step 5: Run the whole service test suite + build**

Run: `cd /data/animeenigma/services/anidle && go build ./... && go test ./... -count=1`
Expected: build clean; all tests PASS.

- [ ] **Step 6: Commit**

```bash
git add services/anidle/internal/domain/compare.go services/anidle/internal/service/engine.go services/anidle/internal/service/engine_test.go
git commit -m "feat(anidle): pure comparison engine (set/numeric/enum columns)"
```

---

## Task 9: Deploy + smoke

- [ ] **Step 1: Build the new service image + bring it up**

Run: `make redeploy-anidle`
(If no such Make target exists, the convention generates per-service targets from compose; if it errors, run `cd /data/animeenigma && docker compose -f docker/docker-compose.yml up -d --build anidle`.)
Expected: image builds, container starts, becomes healthy.

- [ ] **Step 2: Rebuild gateway (picks up the new route)**

Run: `make redeploy-gateway`
Expected: gateway healthy.

- [ ] **Step 3: Smoke /health (host port)**

Run: `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8095/health`
Expected: `200`.

- [ ] **Step 4: Confirm peer builds still work (go.work fan-out sanity)**

Run: `make redeploy-recs` (any one peer service) — confirms the Dockerfile fan-out (Task 3) didn't break a peer build.
Expected: recs builds + healthy. (If a peer build fails with "cannot load module … listed in go.work", a Dockerfile is missing the anidle COPY line — fix Task 3.)

- [ ] **Step 5: No commit** (deploy only). Leave any redeploy state files for the after-update flow.

---

## Self-Review

**Spec coverage (this plan's slice):**
- ✅ New `anidle:8095` service, own DB+Redis, JWT-in-service (spec §3) — scaffolded (Tasks 1–5), game tables deferred to 2b.
- ✅ Pool consumed from catalog `/internal/guessgame/pool`, cached `anidle:pool` (spec §4.2) — Tasks 6–7.
- ✅ Comparison engine 🟩/🟨/⬜ + ↑/↓ per §2.3 — Task 8.
- ✅ Gateway routes `/api/anidle/*` optional-JWT (spec §5.1) — Task 5.
- ⏭️ Deferred to **Plan 2b**: Postgres game tables (daily_puzzle/user_game_result/user_stats), daily determinism, guess/endless/stats/leaderboard endpoints, no-cheat secret handling, JWT middleware on handlers.

**Placeholder scan:** No "TBD"/vague steps. Two "match the real signature" notes (router metrics middleware in Task 2 Step 3; gateway field names in Task 5 Step 4) are explicit verification instructions against named reference files, not placeholders — the implementer confirms the exact symbol from `services/recs`/gateway. Acceptable per the integration-point pattern used in Plan 1.

**Type consistency:** `PoolAnime`/`Taxon` (domain) are used identically by `PoolClient.Fetch`, `PoolStore`, and `Compare`. `GuessComparison`/`ColumnResult`/`MatchStatus`/`Hint` are defined in `domain/compare.go` and consumed by the engine + (future) handlers. `NewPoolClient`/`Fetch`, `NewPoolStore`/`All`/`Lookup`/`Search`, `Compare` signatures match across tasks. The `poolFetcher` interface (poolstore) is satisfied by `*PoolClient` (`Fetch(ctx) ([]PoolAnime, error)`).

**Known integration assumptions (verify during execution):** exact `metrics.Collector` middleware/handler symbols; gateway `cfg.Services.AuthService` + `userRateLimit` names; `redis/go-redis/v9` version alignment via `go mod tidy`; the `make redeploy-<svc>` target generation. Each is checked by a build/run step in the plan.
