# Distributed Tracing (FE→BE) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make a single user action traceable end-to-end — browser API call → gateway → downstream Go services → DB — as one Tempo trace, with the triggering click event carrying the same `trace_id`, and trace↔log correlation in Grafana.

**Architecture:** Activate the dormant `libs/tracing` OTel SDK by adding three small shared helpers (HTTP server middleware, an otel-instrumented HTTP transport, and a GORM plugin wrapper). The gateway extracts the inbound W3C `traceparent` and propagates it downstream (today it propagates nothing — this is the core FE→BE fix); every backend service continues the span via the server middleware; spans flow over OTLP/gRPC to a new **OTel Collector** (tail-sampling) → **Tempo** (object storage on the existing MinIO). The browser's axios interceptor generates a `traceparent` per API call and stamps that `trace_id` onto the analytics click event that triggered it. The clickstream backend already accepts `trace_id` (built in Plan 1), so no analytics-backend change is needed.

**Tech Stack:** Go 1.24 + OpenTelemetry SDK v1.38 (`go.opentelemetry.io/otel`, `otelhttp` contrib, `gorm.io/plugin/opentelemetry/tracing`), `otel/opentelemetry-collector-contrib`, `grafana/tempo`, existing MinIO + Grafana 10.3.3, Vue 3 frontend (axios, vitest), Docker Compose.

**Build-order coverage:** this plan implements design-spec milestones **4 (tracing pipeline)**, **5 (browser trace_id linking)**, and **6 (trace dashboards / trace↔logs)**. Milestones 1–3 and 7 (clickstream + privacy) shipped in Plans 1 & 2.

**What is already done (do NOT redo):**
- `libs/tracing/tracing.go` — `New()`, `Shutdown()`, sets the global TracerProvider + W3C `TraceContext` propagator. Untouched by this plan except its `go.mod` gains deps.
- `libs/logger.WithContext(ctx)` — already injects `trace_id`/`span_id` into logs when a valid span is present (logger.go:84–93). Becomes useful automatically once spans exist.
- Backend clickstream wire contract already has `trace_id`: `services/analytics/internal/handler/collect.go` (`wireEvent.TraceID`, `json:"trace_id"`), `domain/event.go:61` (`TraceID`), `repo/models.go:42` (`trace_id` column).
- Every backend service Dockerfile already has `COPY libs/ ./libs/` and pre-copies `libs/tracing/go.mod` — **no Dockerfile edits are required** by this plan.

---

## File Structure

**Shared library (one Go module, `libs/tracing`):**
- `libs/tracing/middleware.go` (Create) — `HTTPMiddleware(service)` net/http server middleware (span continuation).
- `libs/tracing/client.go` (Create) — `WrapTransport`/`NewHTTPClient` (outbound propagation + client spans).
- `libs/tracing/setup.go` (Create) — `FromEnv`/`InitFromEnv` env-driven config convenience.
- `libs/tracing/gormtrace/gorm.go` (Create) — **separate sub-package** so Redis-only services don't pull GORM. `InstrumentGORM(db)`.
- `libs/tracing/*_test.go` (Create) — unit tests for the above.
- `libs/tracing/go.mod` / `go.sum` (Modify) — add `otelhttp`, `gorm`, `gorm.io/plugin/opentelemetry`.

**Infra:**
- `infra/tempo/tempo.yaml` (Create) — single-binary Tempo, OTLP receiver, MinIO S3 backend.
- `infra/otel/collector-config.yaml` (Create) — OTLP receivers + tail sampling → Tempo.
- `docker/docker-compose.yml` (Modify) — add `tempo`, `tempo-init`, `otel-collector`; add `tempo_data` volume; add `TRACING_ENABLED` env to traced services.
- `docker/grafana/provisioning/datasources/datasources.yml` (Modify) — Tempo datasource + trace↔logs correlation; Loki `derivedFields` → Tempo.
- `infra/grafana/dashboards/backend-tracing.json` (Create) — backend observability dashboard.

**Gateway (the propagation fix):**
- `services/gateway/cmd/gateway-api/main.go` (Modify) — init tracing, wrap server handler.
- `services/gateway/internal/service/proxy.go` (Modify) — wrap outbound transport with otel.
- `services/gateway/go.mod` (Modify) — add `libs/tracing` require+replace.

**Backend fan-out (Task 8):** `services/{auth,catalog,player,rooms,scheduler,streaming,themes,notifications,watch-together,scraper,library}` — each `go.mod` + entrypoint `main.go`.

**Frontend:**
- `frontend/web/src/analytics/traceparent.ts` (Create) — W3C `traceparent` generator.
- `frontend/web/src/analytics/traceContext.ts` (Create) — short-window click↔trace stamping.
- `frontend/web/src/analytics/types.ts` (Modify) — un-defer `trace_id` on `AnalyticsEvent`.
- `frontend/web/src/analytics/index.ts` (Modify) — register click events for stamping.
- `frontend/web/src/api/client.ts` (Modify) — interceptor sets `traceparent` + stamps clicks.
- `frontend/web/src/analytics/__tests__/traceparent.spec.ts`, `traceContext.spec.ts` (Create).

---

### Task 1: `libs/tracing` HTTP server middleware

**Files:**
- Modify: `libs/tracing/tracing.go` (always install the W3C propagator)
- Modify: `libs/tracing/go.mod` (add otelhttp dep)
- Create: `libs/tracing/middleware.go`
- Create: `libs/tracing/middleware_test.go`

- [ ] **Step 1: Add the otelhttp dependency — PINNED to match otel v1.38.0**

`libs/tracing/go.mod` pins `go.opentelemetry.io/otel` at exactly `v1.38.0`. The contrib instrumentation version that matches core `v1.(N).0` is `v0.(N).0`, i.e. **`v0.63.0`**. Do NOT use `@latest` — it resolves a newer contrib whose `require` silently bumps otel core past v1.38.0 across all 12 modules during `go mod tidy`. Pin directly:
```bash
cd /data/animeenigma/libs/tracing
go get go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp@v0.63.0
go mod tidy
go build ./...
```
Expected: `go.mod` requires `otelhttp v0.63.0`; `go.opentelemetry.io/otel` is still `v1.38.0` (confirm with `grep 'go.opentelemetry.io/otel ' go.mod`); builds with no errors. If v0.63.0 does not exist for this core, run `go list -m -versions go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp` and pick the highest `v0.x` whose module `go.mod` requires `otel <= v1.38.0`; then re-verify the core version is unchanged.

- [ ] **Step 2: Install the W3C propagator unconditionally**

So trace context flows even through a service whose span *export* is disabled (partial rollout / kill-switch), move the propagator registration out of the `Enabled` branch. In `libs/tracing/tracing.go`, at the very top of `New()` (before the `if !cfg.Enabled` early return at line 37), add:
```go
	// Always register the W3C propagator, even when disabled, so inbound
	// trace context is still extracted and re-injected downstream. Only span
	// EXPORT is gated by Enabled — context propagation is free and keeps
	// traces from splitting at a service that has tracing turned off.
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{},
		propagation.Baggage{},
	))
```
The existing identical block inside the enabled path (tracing.go:87–90) is now redundant — delete it (the `otel.SetTracerProvider(provider)` call on line 86 stays). Verify: `go build ./...` → no errors (the `propagation` and `otel` imports are already present).

- [ ] **Step 3: Write the failing test**

Create `libs/tracing/middleware_test.go`:
```go
package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.opentelemetry.io/otel/trace"
)

// With tracing enabled, an inbound W3C traceparent must be extracted so the
// handler sees a span whose TraceID equals the header's trace-id (the FE→BE
// continuation guarantee).
func TestHTTPMiddleware_ContinuesInboundTrace(t *testing.T) {
	tr, err := New(context.Background(), Config{ServiceName: "test", Enabled: true, SampleRate: 1.0, OTLPEndpoint: "127.0.0.1:4317"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Shutdown(context.Background()) }()

	const traceID = "0af7651916cd43dd8448eb211c80319c"
	var seen string
	h := HTTPMiddleware("test")(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		seen = trace.SpanContextFromContext(r.Context()).TraceID().String()
	}))

	req := httptest.NewRequest(http.MethodGet, "/anime/123", nil)
	req.Header.Set("traceparent", "00-"+traceID+"-b7ad6b7169203331-01")
	h.ServeHTTP(httptest.NewRecorder(), req)

	if seen != traceID {
		t.Fatalf("expected inbound trace continued (%s), got %q", traceID, seen)
	}
}

// /health, /healthz and /metrics must bypass the span machinery so health
// checks and Prometheus scrapes never create trace spam.
func TestHTTPMiddleware_BypassesOpsEndpoints(t *testing.T) {
	called := false
	h := HTTPMiddleware("test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))
	for _, p := range []string{"/health", "/healthz", "/metrics"} {
		called = false
		h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, p, nil))
		if !called {
			t.Fatalf("bypass path %s did not call next handler", p)
		}
	}
}

// WebSocket upgrades must bypass otelhttp.NewHandler — its ResponseWriter
// wrapper is not guaranteed to implement http.Hijacker, which gorilla/websocket
// (watch-together) and the gateway WS proxy require. Wrapping a WS request
// would 500 the handshake. We assert the next handler runs unwrapped.
func TestHTTPMiddleware_BypassesWebSocketUpgrade(t *testing.T) {
	called := false
	h := HTTPMiddleware("test")(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) { called = true }))
	req := httptest.NewRequest(http.MethodGet, "/api/watch-together/ws", nil)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	h.ServeHTTP(httptest.NewRecorder(), req)
	if !called {
		t.Fatal("websocket upgrade was not passed through unwrapped")
	}
}
```

- [ ] **Step 4: Run the test to verify it fails**

Run: `cd /data/animeenigma/libs/tracing && go test ./... -run TestHTTPMiddleware -v`
Expected: FAIL — `undefined: HTTPMiddleware`.

- [ ] **Step 5: Implement the middleware**

Create `libs/tracing/middleware.go`:
```go
package tracing

import (
	"net/http"
	"strings"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// opsBypass are paths that must never produce trace spans — health checks and
// the Prometheus scrape would otherwise flood Tempo with no diagnostic value.
var opsBypass = map[string]struct{}{
	"/health":  {},
	"/healthz": {},
	"/metrics": {},
}

// isWebSocketUpgrade reports whether r is a WS handshake. Such requests must
// NOT be wrapped by otelhttp.NewHandler: its ResponseWriter wrapper may not
// implement http.Hijacker, which gorilla/websocket (watch-together) and the
// gateway WS proxy require — wrapping would 500 the upgrade.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket")
}

// HTTPMiddleware returns a net/http middleware that continues (or starts) a
// server span per request, using the globally-registered W3C propagator that
// New() installs. Span name is "METHOD /path". When tracing is disabled the
// global provider is a no-op, so wrapping is always safe and effectively free.
//
// Wrap at the http.Server.Handler level so it applies uniformly regardless of
// a service's internal router:
//
//	srv := &http.Server{Handler: tracing.HTTPMiddleware("catalog")(router)}
func HTTPMiddleware(service string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		instrumented := otelhttp.NewHandler(
			next, service,
			otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
				return r.Method + " " + r.URL.Path
			}),
		)
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if _, skip := opsBypass[r.URL.Path]; skip || isWebSocketUpgrade(r) {
				next.ServeHTTP(w, r)
				return
			}
			instrumented.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 6: Run the test to verify it passes**

Run: `cd /data/animeenigma/libs/tracing && go test ./... -run TestHTTPMiddleware -v`
Expected: PASS (all three tests).

- [ ] **Step 7: Commit**

```bash
cd /data/animeenigma
git add libs/tracing/tracing.go libs/tracing/middleware.go libs/tracing/middleware_test.go libs/tracing/go.mod libs/tracing/go.sum
git commit -m "feat(tracing): always-on W3C propagator + HTTPMiddleware (WS-safe) for span continuation"
```

---

### Task 2: `libs/tracing` outbound-propagation transport

**Files:**
- Create: `libs/tracing/client.go`
- Create: `libs/tracing/client_test.go`

- [ ] **Step 1: Write the failing test**

Create `libs/tracing/client_test.go`:
```go
package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// An outbound request made through the wrapped transport, while a valid span
// is active in the context, must carry a W3C traceparent so the downstream
// service can continue the trace. This is the gateway propagation guarantee.
func TestWrapTransport_InjectsTraceparent(t *testing.T) {
	tr, err := New(context.Background(), Config{ServiceName: "client-test", Enabled: true, SampleRate: 1.0, OTLPEndpoint: "127.0.0.1:4317"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer func() { _ = tr.Shutdown(context.Background()) }()

	var got string
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		got = r.Header.Get("traceparent")
	}))
	defer srv.Close()

	// Establish an active span so there is a context to propagate.
	ctx, span := tr.Start(context.Background(), "outbound")
	defer span.End()

	client := NewHTTPClient(nil, 5*time.Second)
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, srv.URL, nil)
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	_ = resp.Body.Close()

	if got == "" {
		t.Fatal("expected traceparent header on the downstream request, got none")
	}
}
```

- [ ] **Step 2: Run the test to verify it fails**

Run: `cd /data/animeenigma/libs/tracing && go test ./... -run TestWrapTransport -v`
Expected: FAIL — `undefined: NewHTTPClient`.

- [ ] **Step 3: Implement the client helpers**

Create `libs/tracing/client.go`:
```go
package tracing

import (
	"net/http"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// WrapTransport wraps an existing RoundTripper so outbound requests get a
// client span and the active trace context is injected (W3C traceparent) using
// the global propagator. Pass nil to wrap http.DefaultTransport. Use this to
// keep a custom transport's dialer/pool settings while adding propagation:
//
//	t.Transport = tracing.WrapTransport(t.Transport)
func WrapTransport(base http.RoundTripper) http.RoundTripper {
	if base == nil {
		base = http.DefaultTransport
	}
	return otelhttp.NewTransport(base)
}

// NewHTTPClient returns an *http.Client whose transport propagates trace
// context. base may be nil (http.DefaultTransport). When tracing is disabled
// the global provider is a no-op and no header is injected.
func NewHTTPClient(base http.RoundTripper, timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: WrapTransport(base)}
}
```

- [ ] **Step 4: Run the test to verify it passes**

Run: `cd /data/animeenigma/libs/tracing && go test ./... -run TestWrapTransport -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add libs/tracing/client.go libs/tracing/client_test.go
git commit -m "feat(tracing): add otel-instrumented HTTP transport for outbound propagation"
```

---

### Task 3: `FromEnv`/`InitFromEnv` + GORM sub-package

**Files:**
- Create: `libs/tracing/setup.go`
- Create: `libs/tracing/setup_test.go`
- Create: `libs/tracing/gormtrace/gorm.go`
- Create: `libs/tracing/gormtrace/gorm_test.go`
- Modify: `libs/tracing/go.mod` (gorm deps, via the gormtrace sub-package)

- [ ] **Step 1: Write the failing test for FromEnv**

Create `libs/tracing/setup_test.go`:
```go
package tracing

import (
	"testing"
)

func TestFromEnv_Defaults(t *testing.T) {
	t.Setenv("TRACING_ENABLED", "")
	t.Setenv("OTLP_ENDPOINT", "")
	t.Setenv("TRACING_SAMPLE_RATE", "")
	cfg := FromEnv("catalog")
	if cfg.Enabled {
		t.Error("expected disabled by default")
	}
	if cfg.OTLPEndpoint != "otel-collector:4317" {
		t.Errorf("default endpoint = %q", cfg.OTLPEndpoint)
	}
	if cfg.SampleRate != 1.0 {
		t.Errorf("default sample rate = %v, want 1.0 (collector tail-samples)", cfg.SampleRate)
	}
	if cfg.ServiceName != "catalog" {
		t.Errorf("service name = %q", cfg.ServiceName)
	}
}

func TestFromEnv_Enabled(t *testing.T) {
	t.Setenv("TRACING_ENABLED", "true")
	t.Setenv("OTLP_ENDPOINT", "host:1234")
	t.Setenv("TRACING_SAMPLE_RATE", "0.5")
	cfg := FromEnv("auth")
	if !cfg.Enabled || cfg.OTLPEndpoint != "host:1234" || cfg.SampleRate != 0.5 {
		t.Errorf("unexpected cfg: %+v", cfg)
	}
}
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/libs/tracing && go test ./... -run TestFromEnv -v`
Expected: FAIL — `undefined: FromEnv`.

- [ ] **Step 3: Implement setup.go**

Create `libs/tracing/setup.go`:
```go
package tracing

import (
	"context"
	"os"
	"strconv"
)

// FromEnv builds a Config from standard env vars. Defaults are chosen so that
// the only var a service needs to set to participate is TRACING_ENABLED=true:
//
//	TRACING_ENABLED      bool    (default false — off until explicitly enabled)
//	OTLP_ENDPOINT        string  (default "otel-collector:4317", gRPC)
//	TRACING_SAMPLE_RATE  float64 (default 1.0 — head-sample everything; the
//	                              OTel Collector does tail sampling centrally)
//	ENV                  string  (default "development")
func FromEnv(service string) Config {
	enabled, _ := strconv.ParseBool(os.Getenv("TRACING_ENABLED"))

	endpoint := os.Getenv("OTLP_ENDPOINT")
	if endpoint == "" {
		endpoint = "otel-collector:4317"
	}

	rate := 1.0
	if v := os.Getenv("TRACING_SAMPLE_RATE"); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil {
			rate = parsed
		}
	}

	env := os.Getenv("ENV")
	if env == "" {
		env = "development"
	}

	return Config{
		ServiceName:    service,
		ServiceVersion: os.Getenv("SERVICE_VERSION"),
		Environment:    env,
		OTLPEndpoint:   endpoint,
		SampleRate:     rate,
		Enabled:        enabled,
	}
}

// InitFromEnv is the one-call convenience used in service main.go:
//
//	tr, err := tracing.InitFromEnv(context.Background(), "catalog")
//	if err != nil { log.Fatalw("tracing init", "error", err) }
//	defer func() { _ = tr.Shutdown(context.Background()) }()
func InitFromEnv(ctx context.Context, service string) (*Tracer, error) {
	return New(ctx, FromEnv(service))
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /data/animeenigma/libs/tracing && go test ./... -run TestFromEnv -v`
Expected: PASS.

- [ ] **Step 5: Add the GORM dependency (into the same module) — keep otel core at v1.38.0**

`@latest` on the GORM OTel plugin can transitively bump otel core past v1.38.0 (same trap as Task 1 Step 1). Match the existing `gorm.io/gorm` version the services already use (find it with `grep 'gorm.io/gorm ' /data/animeenigma/services/auth/go.mod`) and pick the plugin release whose `go.mod` requires `otel <= v1.38.0`:
```bash
cd /data/animeenigma/libs/tracing
GORM_VER=$(grep -m1 'gorm.io/gorm ' /data/animeenigma/services/auth/go.mod | awk '{print $2}')
go get gorm.io/gorm@${GORM_VER}
go get gorm.io/plugin/opentelemetry@v0.1.12
go mod tidy
grep 'go.opentelemetry.io/otel ' go.mod   # MUST still be v1.38.0
go build ./...
```
Expected: `go.mod` requires `gorm.io/plugin/opentelemetry` (v0.1.12) and the same `gorm.io/gorm` version as the services; otel core unchanged at v1.38.0; clean build. If v0.1.12 pulls a newer otel, run `go list -m -versions gorm.io/plugin/opentelemetry` and pick the highest version whose `go.mod` requires `otel <= v1.38.0`; re-verify the core version. (Putting GORM in the dedicated `gormtrace` sub-package keeps it out of the binaries of Redis-only services, which import only the root `tracing` package.)

- [ ] **Step 6: Write the failing GORM test**

Create `libs/tracing/gormtrace/gorm_test.go`:
```go
package gormtrace

import (
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestInstrumentGORM_RegistersPlugin(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := InstrumentGORM(db); err != nil {
		t.Fatalf("InstrumentGORM returned error: %v", err)
	}
}
```

Add the sqlite test driver:
```bash
cd /data/animeenigma/libs/tracing
go get gorm.io/driver/sqlite@latest
go mod tidy
```

- [ ] **Step 7: Run to verify it fails**

Run: `cd /data/animeenigma/libs/tracing && go test ./gormtrace/... -v`
Expected: FAIL — `undefined: InstrumentGORM` (package directory may not compile yet).

- [ ] **Step 8: Implement the GORM wrapper**

Create `libs/tracing/gormtrace/gorm.go`:
```go
// Package gormtrace adds OpenTelemetry spans to GORM queries. It lives in its
// own sub-package so services that don't use GORM (Redis-only: gateway,
// watch-together) never pull the GORM dependency into their binaries.
package gormtrace

import (
	"gorm.io/gorm"
	otelgorm "gorm.io/plugin/opentelemetry/tracing"
)

// InstrumentGORM registers the OTel tracing plugin on db. Metrics are disabled
// (Prometheus already covers DB pool stats via libs/metrics). Call once after
// database.New(), before serving traffic:
//
//	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
//	    log.Warnw("gorm tracing disabled", "error", err)
//	}
func InstrumentGORM(db *gorm.DB) error {
	return db.Use(otelgorm.NewPlugin(otelgorm.WithoutMetrics()))
}
```

- [ ] **Step 9: Run to verify it passes**

Run: `cd /data/animeenigma/libs/tracing && go test ./... -v`
Expected: PASS (all tracing + gormtrace tests).

- [ ] **Step 10: Commit**

```bash
cd /data/animeenigma
git add libs/tracing/setup.go libs/tracing/setup_test.go libs/tracing/gormtrace/ libs/tracing/go.mod libs/tracing/go.sum
git commit -m "feat(tracing): add FromEnv/InitFromEnv config + gormtrace sub-package"
```

---

### Task 4: Tempo container + config + MinIO bucket

**Files:**
- Create: `infra/tempo/tempo.yaml`
- Modify: `docker/docker-compose.yml` (add `tempo`, `tempo-init`; add `tempo_data` volume)

- [ ] **Step 1: Write the Tempo config**

Create `infra/tempo/tempo.yaml`:
```yaml
# Single-binary Tempo. Traces are stored in the existing MinIO (S3-compatible);
# only the small write-ahead log lives on a local volume. Retention ~14 days —
# traces are debugging data, not long-term analytics.
server:
  http_listen_port: 3200
  grpc_listen_port: 9095

distributor:
  receivers:
    otlp:
      protocols:
        grpc:
          endpoint: 0.0.0.0:4317
        http:
          endpoint: 0.0.0.0:4318

ingester:
  max_block_duration: 5m

compactor:
  compaction:
    block_retention: 336h # 14 days

storage:
  trace:
    backend: s3
    s3:
      bucket: tempo
      endpoint: minio:9000
      access_key: minioadmin
      secret_key: minioadmin
      insecure: true        # MinIO is plain HTTP on the internal network
      forcepathstyle: true  # required for MinIO
    wal:
      path: /var/tempo/wal
    local:
      path: /var/tempo/blocks

usage_report:
  reporting_enabled: false
```

- [ ] **Step 2: Add the Tempo + bucket-init services to docker-compose**

In `docker/docker-compose.yml`, add these services (place them near the other observability services — the `loki:` block ends ~line 237, `promtail:` is ~238, `grafana:` ~251; insert before `grafana:`). `tempo-init` creates the bucket idempotently before Tempo starts:
```yaml
  tempo-init:
    image: minio/mc:latest
    container_name: animeenigma-tempo-init
    depends_on:
      minio:
        condition: service_healthy
    entrypoint:
      - /bin/sh
      - -c
      - |
        mc alias set local http://minio:9000 minioadmin minioadmin &&
        mc mb --ignore-existing local/tempo &&
        echo "tempo bucket ready"
    restart: "no"

  tempo:
    image: grafana/tempo:2.4.1
    container_name: animeenigma-tempo
    restart: unless-stopped
    command: ["-config.file=/etc/tempo/tempo.yaml"]
    volumes:
      - ../infra/tempo/tempo.yaml:/etc/tempo/tempo.yaml:ro
      - tempo_data:/var/tempo
    ports:
      - "127.0.0.1:3200:3200" # Tempo API (Grafana datasource target)
    depends_on:
      tempo-init:
        condition: service_completed_successfully
    healthcheck:
      test: ["CMD", "wget", "--quiet", "--tries=1", "--spider", "http://localhost:3200/ready"]
      interval: 15s
      timeout: 5s
      retries: 5
```

- [ ] **Step 3: Add the `tempo_data` volume**

In the top-level `volumes:` block of `docker/docker-compose.yml`, add:
```yaml
  tempo_data:
```

- [ ] **Step 4: Validate the compose file + start Tempo**

Run:
```bash
cd /data/animeenigma
docker compose -f docker/docker-compose.yml config >/dev/null && echo "compose OK"
docker compose -f docker/docker-compose.yml up -d tempo-init tempo
```
Expected: `compose OK`; `tempo-init` exits 0 (bucket created); `tempo` becomes healthy. Verify:
```bash
docker compose -f docker/docker-compose.yml logs tempo-init | tail -3   # "tempo bucket ready"
curl -fs http://localhost:3200/ready && echo "  <- tempo ready"
```
Expected: `ready` and exit 0. (If Tempo logs an S3 "bucket does not exist" error, re-run `tempo-init`.)

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add infra/tempo/tempo.yaml docker/docker-compose.yml
git commit -m "feat(tracing): add Tempo container with MinIO-backed trace storage"
```

---

### Task 5: OTel Collector container + config

**Files:**
- Create: `infra/otel/collector-config.yaml`
- Modify: `docker/docker-compose.yml` (add `otel-collector`)

- [ ] **Step 1: Write the collector config**

Create `infra/otel/collector-config.yaml`:
```yaml
# OTLP in (from services) → tail sampling → OTLP out (to Tempo).
# Tail sampling keeps 100% of error and slow (>1s) traces and ~20% of the rest,
# so Tempo stays small while every interesting trace is retained.
receivers:
  otlp:
    protocols:
      grpc:
        endpoint: 0.0.0.0:4317
      http:
        endpoint: 0.0.0.0:4318

processors:
  memory_limiter:
    check_interval: 1s
    limit_percentage: 75
    spike_limit_percentage: 15
  tail_sampling:
    decision_wait: 10s
    num_traces: 50000
    expected_new_traces_per_sec: 100
    policies:
      - name: errors
        type: status_code
        status_code:
          status_codes: [ERROR]
      - name: slow
        type: latency
        latency:
          threshold_ms: 1000
      - name: sample-rest
        type: probabilistic
        probabilistic:
          sampling_percentage: 20
  batch:
    timeout: 5s
    send_batch_size: 1024

exporters:
  otlp/tempo:
    endpoint: tempo:4317
    tls:
      insecure: true

service:
  pipelines:
    traces:
      receivers: [otlp]
      processors: [memory_limiter, tail_sampling, batch]
      exporters: [otlp/tempo]
  telemetry:
    logs:
      level: warn
```

- [ ] **Step 2: Add the collector service to docker-compose**

In `docker/docker-compose.yml`, add (near `tempo:`):
```yaml
  otel-collector:
    image: otel/opentelemetry-collector-contrib:0.103.1
    container_name: animeenigma-otel-collector
    restart: unless-stopped
    command: ["--config=/etc/otel/collector-config.yaml"]
    volumes:
      - ../infra/otel/collector-config.yaml:/etc/otel/collector-config.yaml:ro
    ports:
      - "127.0.0.1:4317:4317" # OTLP gRPC (services export here)
      - "127.0.0.1:4318:4318" # OTLP HTTP
    depends_on:
      tempo:
        condition: service_healthy
```
(The `-contrib` image is required — `tail_sampling` is not in the core collector.)

- [ ] **Step 3: Validate + start the collector**

Run:
```bash
cd /data/animeenigma
docker compose -f docker/docker-compose.yml config >/dev/null && echo "compose OK"
docker compose -f docker/docker-compose.yml up -d otel-collector
sleep 4
docker compose -f docker/docker-compose.yml logs otel-collector | grep -iE "error|started|everything is ready" | tail -5
```
Expected: `compose OK`; collector log shows "Everything is ready" with no ERROR lines (a few `tail_sampling` info lines are fine).

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add infra/otel/collector-config.yaml docker/docker-compose.yml
git commit -m "feat(tracing): add OTel Collector with tail sampling → Tempo"
```

---

### Task 6: Grafana Tempo datasource + trace↔logs correlation

**Files:**
- Modify: `docker/grafana/provisioning/datasources/datasources.yml`

- [ ] **Step 1: Inspect the current datasources file**

Run: `cat /data/animeenigma/docker/grafana/provisioning/datasources/datasources.yml`
The existing **Loki datasource has NO `uid`** (only the Postgres datasource `aenigma-postgres` from Plan 2 has one). Trace↔logs correlation requires both Loki and Tempo to carry explicit, stable uids that reference each other — so the first edit is to give Loki a uid.

- [ ] **Step 2: Give the Loki datasource an explicit `uid` (mandatory)**

In the existing Loki datasource block in `datasources.yml`, add (if not already present):
```yaml
    uid: aenigma-loki
```
This is required — the Tempo `tracesToLogsV2.datasourceUid` (Step 3) and Loki `derivedFields.datasourceUid` (Step 4) form a mutual reference; without a fixed Loki uid the link silently no-ops.

- [ ] **Step 3: Add the Tempo datasource**

Append to the `datasources:` list in `docker/grafana/provisioning/datasources/datasources.yml`:
```yaml
  - name: Tempo
    type: tempo
    uid: aenigma-tempo
    access: proxy
    url: http://tempo:3200
    jsonData:
      tracesToLogsV2:
        datasourceUid: aenigma-loki
        spanStartTimeShift: '-5m'
        spanEndTimeShift: '5m'
        filterByTraceID: true
        filterBySpanID: false
      nodeGraph:
        enabled: true
```

- [ ] **Step 4: Add derivedFields to the Loki datasource (log → trace jump)**

In the existing Loki datasource's `jsonData:` block, add:
```yaml
      derivedFields:
        - name: TraceID
          matcherType: label
          matcherRegex: trace_id
          datasourceUid: aenigma-tempo
          url: '$${__value.raw}'
```
`libs/logger.WithContext` already writes the `trace_id` field on every log line emitted inside a span, so this needs no app change. (The `$$` escapes the `$` so Docker Compose does not interpolate it.)

- [ ] **Step 5: Restart Grafana and verify both datasources load**

Run:
```bash
cd /data/animeenigma
make restart-grafana
sleep 8
docker compose -f docker/docker-compose.yml logs grafana | grep -iE "tempo|datasource|error" | tail -15
```
Expected: provisioning logs show the Tempo datasource registered, no provisioning errors.

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add docker/grafana/provisioning/datasources/datasources.yml
git commit -m "feat(tracing): add Tempo datasource + Loki→Tempo trace correlation"
```

---

### Task 7: Gateway — root span + downstream propagation (the FE→BE fix)

**Files:**
- Modify: `services/gateway/go.mod` (add `libs/tracing` require+replace)
- Modify: `services/gateway/cmd/gateway-api/main.go`
- Modify: `services/gateway/internal/service/proxy.go`

- [ ] **Step 1: Add the libs/tracing dependency to the gateway module**

Edit `services/gateway/go.mod`. In the first `require (` block add:
```
	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
```
In the `replace (` block add:
```
	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
```
Then:
```bash
cd /data/animeenigma/services/gateway && go mod tidy
```
Expected: resolves with no errors.

- [ ] **Step 2: Instrument the outbound proxy transport**

In `services/gateway/internal/service/proxy.go`, add the import:
```go
	"github.com/ILITA-hub/animeenigma/libs/tracing"
```
Then in `NewProxyService`, wrap the custom transport so every forwarded request carries the gateway span's `traceparent`. Change the client construction from:
```go
		client: &http.Client{
			Timeout: 15 * time.Second,
			Transport: &http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   3 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			},
		},
```
to:
```go
		client: &http.Client{
			Timeout: 15 * time.Second,
			// tracing.WrapTransport injects the active span's W3C traceparent
			// into the forwarded request so downstream services continue the
			// same trace. This is the core FE→BE propagation fix — the gateway
			// previously propagated nothing. No-op when tracing is disabled.
			Transport: tracing.WrapTransport(&http.Transport{
				DialContext: (&net.Dialer{
					Timeout:   3 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				MaxIdleConns:        100,
				MaxIdleConnsPerHost: 10,
				IdleConnTimeout:     90 * time.Second,
			}),
		},
```

> **Note (harmless double-write):** `copyForwardHeaders` already copies any client-supplied `traceparent` onto the outbound request, and `WrapTransport` then *overwrites* (Set, not Add) it with the gateway span's context. The gateway therefore becomes the parent span of the trace whose root id the browser minted — exactly the intended FE→BE link. Do NOT "defensively" strip `traceparent` in `copyForwardHeaders`; that would not help and risks breaking propagation.

- [ ] **Step 3: Initialize tracing + wrap the server handler in main.go**

In `services/gateway/cmd/gateway-api/main.go`, add the import:
```go
	"github.com/ILITA-hub/animeenigma/libs/tracing"
```
Immediately after `cfg, err := config.Load()` (and its error check), add:
```go
	// Distributed tracing. No-op unless TRACING_ENABLED=true. Spans export to
	// OTLP_ENDPOINT (default otel-collector:4317); failures are dropped, never
	// fatal, so an unreachable collector cannot take the gateway down.
	tracer, err := tracing.InitFromEnv(context.Background(), "gateway")
	if err != nil {
		log.Warnw("tracing init failed; continuing without tracing", "error", err)
	} else {
		defer func() { _ = tracer.Shutdown(context.Background()) }()
	}
```
Then wrap the router where the server is built. Change:
```go
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      router,
```
to:
```go
	srv := &http.Server{
		Addr:         cfg.Server.Address(),
		Handler:      tracing.HTTPMiddleware("gateway")(router),
```

- [ ] **Step 4: Build the gateway**

Run: `cd /data/animeenigma/services/gateway && go build ./...`
Expected: builds cleanly. (`context` is already imported in main.go.)

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add services/gateway/go.mod services/gateway/go.sum services/gateway/cmd/gateway-api/main.go services/gateway/internal/service/proxy.go
git commit -m "feat(tracing): gateway root span + downstream traceparent propagation"
```

---

### Task 8: Wire tracing into the remaining backend services

This task applies the **same uniform recipe** to each service so every backend continues the trace started at the gateway. The edits are identical except for the service-name string and whether the service uses GORM.

**Services to wire (11):**

| Service | Entrypoint main.go (under `services/<svc>/cmd/`) | metrics name | GORM? |
|---|---|---|---|
| auth | `cmd/auth-api/main.go` | `auth` | yes |
| catalog | `cmd/catalog-api/main.go` | `catalog` | yes |
| player | `cmd/player-api/main.go` | `player` | yes |
| rooms | `cmd/rooms-api/main.go` | `rooms` | **no (Redis-only)** |
| scheduler | `cmd/scheduler-api/main.go` | `scheduler` | yes |
| streaming | `cmd/streaming-api/main.go` | `streaming` | no |
| themes | `cmd/themes-api/main.go` | `themes` | yes |
| notifications | `cmd/notifications-api/main.go` | `notifications` | yes |
| watch-together | `cmd/watch-together-api/main.go` | `watch-together` | no |
| scraper | `cmd/scraper-api/main.go` | `scraper` | no |
| library | `cmd/library-api/main.go` | `library` | yes |

> `analytics` is intentionally excluded: it is a fire-and-forget sink (heartbeat every 15s), not part of user-action call chains, and tracing it would be pure noise. The exact metrics-name string for each service is the literal already passed to `metrics.NewCollector("…")` in that service's main.go — confirm it matches the table while editing.
>
> **GORM split (verified):** only **auth, catalog, player, scheduler, themes, notifications, library** call `database.New` and get GORM instrumentation (Step D). **rooms, streaming, watch-together, scraper** are Redis-only / stateless — they get Steps A–C + E–G but **skip Step D entirely** (rooms uses `cache.New(cfg.Redis)`, not `database.New`, so there is no `db` variable to instrument).

**Per-service recipe (apply to EACH service above):**

- [ ] **Step A: Add the dependency.** In `services/<svc>/go.mod`, add to the first `require (` block:
  ```
  	github.com/ILITA-hub/animeenigma/libs/tracing v0.0.0
  ```
  and to the `replace (` block:
  ```
  	github.com/ILITA-hub/animeenigma/libs/tracing => ../../libs/tracing
  ```

- [ ] **Step B: Init tracing.** In the service's `main.go`, add the import `"github.com/ILITA-hub/animeenigma/libs/tracing"` and, right after config load succeeds, add (replace `<name>` with the metrics name):
  ```go
  	tracer, err := tracing.InitFromEnv(context.Background(), "<name>")
  	if err != nil {
  		log.Warnw("tracing init failed; continuing without tracing", "error", err)
  	} else {
  		defer func() { _ = tracer.Shutdown(context.Background()) }()
  	}
  ```
  If `context` is not already imported in that main.go, add it. If the file already declares `err` via `:=` earlier, reuse `=` (e.g. `tracer, err = ...`) or pick a fresh name to avoid a shadow/redeclare error — the build step will catch it.

- [ ] **Step C: Wrap the server handler.** Find the `&http.Server{...}` literal and change `Handler: <router>,` to:
  ```go
  		Handler: tracing.HTTPMiddleware("<name>")(<router>),
  ```
  (`<router>` is whatever variable was previously assigned, e.g. `router`.)

- [ ] **Step D (GORM services ONLY — auth, catalog, player, scheduler, themes, notifications, library; NOT rooms/streaming/watch-together/scraper):** add the import `gormtrace "github.com/ILITA-hub/animeenigma/libs/tracing/gormtrace"` and, right after `db, err := database.New(...)` succeeds, add:
  ```go
  	if err := gormtrace.InstrumentGORM(db.DB); err != nil {
  		log.Warnw("gorm tracing disabled", "error", err)
  	}
  ```
  Pass **`db.DB`** — `database.DB` embeds `*gorm.DB`, and `db.DB` is the `*gorm.DB` handle the repos already receive (e.g. auth's `repo.NewUserRepository(db.DB)`). Do not change it to bare `db`.

- [ ] **Step E: Enable tracing in docker-compose.** In each service's block in `docker/docker-compose.yml`, add to its `environment:` map:
  ```yaml
      TRACING_ENABLED: "true"
  ```
  (`OTLP_ENDPOINT` defaults to `otel-collector:4317` via `FromEnv`, so it need not be set unless overriding.)

- [ ] **Step F: tidy + build each service.** For each service:
  ```bash
  cd /data/animeenigma/services/<svc> && go mod tidy && go build ./...
  ```
  Expected: clean build. Do this immediately after editing each service so a mistake is localized.

- [ ] **Step G: Workspace sync + full build.** After all 11 services are edited:
  ```bash
  cd /data/animeenigma && go work sync && go build ./services/... ./libs/...
  ```
  Expected: the entire workspace builds with no errors.

- [ ] **Step H: Commit (single commit for the fan-out).**
  ```bash
  cd /data/animeenigma
  git add services/auth services/catalog services/player services/rooms services/scheduler \
          services/streaming services/themes services/notifications services/watch-together \
          services/scraper services/library docker/docker-compose.yml go.work.sum
  git commit -m "feat(tracing): continue gateway trace across all backend services"
  ```

---

### Task 9: Frontend — `traceparent` generator + click↔trace store

**Files:**
- Create: `frontend/web/src/analytics/traceparent.ts`
- Create: `frontend/web/src/analytics/traceContext.ts`
- Create: `frontend/web/src/analytics/__tests__/traceparent.spec.ts`
- Create: `frontend/web/src/analytics/__tests__/traceContext.spec.ts`

- [ ] **Step 1: Write the failing traceparent test**

Create `frontend/web/src/analytics/__tests__/traceparent.spec.ts`:
```ts
import { describe, it, expect } from 'vitest'
import { newTraceparent } from '../traceparent'

describe('newTraceparent', () => {
  it('produces a valid W3C traceparent: 00-<32hex>-<16hex>-01', () => {
    const { header, traceId } = newTraceparent()
    expect(header).toMatch(/^00-[0-9a-f]{32}-[0-9a-f]{16}-01$/)
    expect(traceId).toMatch(/^[0-9a-f]{32}$/)
    expect(header).toContain(traceId)
  })

  it('does not emit an all-zero trace id', () => {
    for (let i = 0; i < 20; i++) {
      expect(newTraceparent().traceId).not.toBe('0'.repeat(32))
    }
  })

  it('is unique across calls', () => {
    const a = newTraceparent().traceId
    const b = newTraceparent().traceId
    expect(a).not.toBe(b)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/traceparent.spec.ts`
Expected: FAIL — cannot resolve `../traceparent`.

- [ ] **Step 3: Implement traceparent.ts**

Create `frontend/web/src/analytics/traceparent.ts`:
```ts
// W3C Trace Context generator. The browser has no real spans (RUM is out of
// scope for v1); it mints a synthetic traceparent per API call so the backend
// trace roots with a trace_id the frontend also knows, which lets us stamp
// that trace_id onto the analytics click event that triggered the call.
function randomHex(bytes: number): string {
  const buf = new Uint8Array(bytes)
  crypto.getRandomValues(buf)
  let out = ''
  for (const b of buf) out += b.toString(16).padStart(2, '0')
  return out
}

export function newTraceparent(): { header: string; traceId: string } {
  const traceId = randomHex(16) // 128-bit
  const spanId = randomHex(8) // 64-bit
  // version 00, trace-flags 01 (sampled — the collector tail-samples centrally)
  return { header: `00-${traceId}-${spanId}-01`, traceId }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/traceparent.spec.ts`
Expected: PASS.

- [ ] **Step 5: Write the failing traceContext test**

Create `frontend/web/src/analytics/__tests__/traceContext.spec.ts`:
```ts
import { describe, it, expect, beforeEach } from 'vitest'
import { registerClickForTrace, stampTrace, _resetForTest } from '../traceContext'
import type { AnalyticsEvent } from '../types'

function clickEvent(): AnalyticsEvent {
  return { event_type: 'click', timestamp: new Date().toISOString(), path: '/x' }
}

describe('traceContext', () => {
  beforeEach(() => _resetForTest())

  it('stamps a registered click event that has no trace_id yet', () => {
    const e = clickEvent()
    registerClickForTrace(e)
    stampTrace('abc123', 1500)
    expect(e.trace_id).toBe('abc123')
  })

  it('does not overwrite a trace_id that is already set', () => {
    const e = clickEvent()
    e.trace_id = 'first'
    registerClickForTrace(e)
    stampTrace('second', 1500)
    expect(e.trace_id).toBe('first')
  })

  it('ignores clicks older than the window', () => {
    const e = clickEvent()
    registerClickForTrace(e, Date.now() - 5000) // 5s ago
    stampTrace('late', 1500, Date.now())
    expect(e.trace_id).toBeUndefined()
  })
})
```

- [ ] **Step 6: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/traceContext.spec.ts`
Expected: FAIL — cannot resolve `../traceContext`.

- [ ] **Step 7: Implement traceContext.ts**

Create `frontend/web/src/analytics/traceContext.ts`:
```ts
// Best-effort click↔trace association (design spec §2, "v1 honesty"). A click
// is enqueued WITHOUT a trace_id; when the API call it triggers fires, the
// axios interceptor calls stampTrace(traceId), which back-fills the trace_id
// onto recent un-stamped click events. Because flush is delayed (≥5s / size
// 20), the in-place mutation lands before the event is shipped.
import type { AnalyticsEvent } from './types'

interface Pending {
  evt: AnalyticsEvent
  ts: number
}

let pending: Pending[] = []

// registerClickForTrace records a click event so the next API call within the
// window can stamp it. `at` is injectable for tests (defaults to now).
export function registerClickForTrace(evt: AnalyticsEvent, at: number = Date.now()): void {
  pending.push({ evt, ts: at })
  // Bound memory: keep only the most recent 50 entries.
  if (pending.length > 50) pending = pending.slice(-50)
}

// stampTrace assigns traceId to every pending click within `withinMs` that has
// no trace_id yet, then prunes entries older than the window. `now` is
// injectable for tests.
export function stampTrace(traceId: string, withinMs = 1500, now: number = Date.now()): void {
  for (const p of pending) {
    if (!p.evt.trace_id && now - p.ts <= withinMs) {
      p.evt.trace_id = traceId
    }
  }
  pending = pending.filter((p) => now - p.ts <= withinMs)
}

// Test-only reset.
export function _resetForTest(): void {
  pending = []
}
```

- [ ] **Step 8: Run to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/traceContext.spec.ts`
Expected: PASS.

- [ ] **Step 9: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/analytics/traceparent.ts frontend/web/src/analytics/traceContext.ts \
        frontend/web/src/analytics/__tests__/traceparent.spec.ts frontend/web/src/analytics/__tests__/traceContext.spec.ts
git commit -m "feat(tracing): frontend W3C traceparent generator + click↔trace store"
```

---

### Task 10: Frontend — wire traceparent into axios + stamp click events

**Files:**
- Modify: `frontend/web/src/analytics/types.ts` (un-defer `trace_id`)
- Modify: `frontend/web/src/analytics/index.ts` (register click for stamping)
- Modify: `frontend/web/src/api/client.ts` (interceptor: set header + stampTrace)

- [ ] **Step 1: Un-defer `trace_id` on the event type**

In `frontend/web/src/analytics/types.ts`, replace the deferral comment line inside `AnalyticsEvent`:
```ts
  // trace_id is intentionally omitted in Plan 2 (added by Plan 3 tracing).
```
with:
```ts
  trace_id?: string // stamped by the axios interceptor (Plan 3), links click → backend trace
```

- [ ] **Step 2: Register click events for trace stamping**

In `frontend/web/src/analytics/index.ts`, add the import at the top (after the existing imports):
```ts
import { registerClickForTrace } from './traceContext'
```
Then, in the `clickListener` inside `init()`, register the event so the next API call can stamp it. Change:
```ts
      const desc = extractClick(target)
      if (!desc) return
      this.enqueue({ event_type: 'click', timestamp: nowISO(), path: location.pathname, ...desc })
```
to:
```ts
      const desc = extractClick(target)
      if (!desc) return
      const evt = { event_type: 'click' as const, timestamp: nowISO(), path: location.pathname, ...desc }
      this.enqueue(evt)
      // Best-effort: the next API call within ~1.5s back-fills evt.trace_id.
      registerClickForTrace(evt)
```
> **Load-bearing (verified):** `stampTrace` mutates `evt.trace_id` in place *after* enqueue. This works only because `Transport.enqueue` stores the event **by reference** (`this.buf.push(e)` in `transport.ts`) and `flush` serializes the buffer lazily (`JSON.stringify` at send time, ≥5s / size-20 later). Do not change `enqueue` to clone/serialize on insert — that would strip the trace_id back-fill. The unit tests check the store in isolation; this cross-module reference contract is not test-covered, so preserve it.

- [ ] **Step 3: Set `traceparent` and stamp clicks in the axios request interceptor**

In `frontend/web/src/api/client.ts`, add the imports (after the existing imports near the top):
```ts
import { newTraceparent } from '@/analytics/traceparent'
import { stampTrace } from '@/analytics/traceContext'

const TRACING_ON = import.meta.env.VITE_ANALYTICS_ENABLED !== 'false'
```
Then, inside the existing request interceptor (`apiClient.interceptors.request.use`), just before `return config`, add:
```ts
    // Distributed tracing: mint a W3C traceparent per call so the backend
    // trace roots with a known trace_id, and stamp that id onto the click
    // event that triggered this request (best-effort, ~1.5s window).
    if (TRACING_ON) {
      const { header, traceId } = newTraceparent()
      config.headers['traceparent'] = header
      stampTrace(traceId)
    }
```
(`config.headers` is already guaranteed non-null by the line `config.headers = config.headers || {}` above the auth block.)

> **Known gap (acceptable):** the interceptor has an early `return config` for `/auth/refresh` and `/auth/login` (client.ts:136–138), so those two calls won't carry a `traceparent` and won't be click-attributed. Login/refresh aren't click-driven flows, so this is fine for v1; note it rather than work around it.

- [ ] **Step 4: Type-check + run the analytics test suite**

Run:
```bash
cd /data/animeenigma/frontend/web
bunx vitest run src/analytics/
bunx tsc --noEmit
```
Expected: all analytics specs PASS; `tsc` reports no errors.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/analytics/types.ts frontend/web/src/analytics/index.ts frontend/web/src/api/client.ts
git commit -m "feat(tracing): mint traceparent per API call + stamp trace_id on clicks"
```

---

### Task 11: Grafana backend-observability dashboard

**Files:**
- Create: `infra/grafana/dashboards/backend-tracing.json`

> Latency/error-rate panels reuse the **existing** Prometheus `http_request_duration_seconds` and `http_requests_total` metrics (already emitted by every service via `libs/metrics`) — no spanmetrics connector needed. Tempo is used via the trace-search panel for slow-trace drill-down, and trace↔logs jump (Task 6) handles correlation. (A spanmetrics connector for true span-level RED metrics is a documented future enhancement, not in scope here.)

- [ ] **Step 1: Give the Prometheus datasource an explicit `uid`**

The Prometheus datasource in `docker/grafana/provisioning/datasources/datasources.yml` currently has **no `uid`** (only `isDefault: true`), so the dashboard cannot reference it by uid. Add to that block:
```yaml
    uid: aenigma-prometheus
```
(Leave `isDefault: true` as-is.) The dashboard JSON below references this literal uid.

- [ ] **Step 2: Create the dashboard JSON**

Create `infra/grafana/dashboards/backend-tracing.json`:
```json
{
  "annotations": { "list": [] },
  "editable": true,
  "title": "Backend Tracing & Latency",
  "uid": "backend-tracing",
  "tags": ["tracing", "backend"],
  "timezone": "",
  "schemaVersion": 39,
  "time": { "from": "now-6h", "to": "now" },
  "templating": { "list": [] },
  "panels": [
    {
      "type": "timeseries",
      "title": "Request latency p95 by service (s)",
      "datasource": { "type": "prometheus", "uid": "aenigma-prometheus" },
      "gridPos": { "h": 9, "w": 12, "x": 0, "y": 0 },
      "targets": [
        {
          "expr": "histogram_quantile(0.95, sum(rate(http_request_duration_seconds_bucket[5m])) by (le, service))",
          "legendFormat": "{{service}}"
        }
      ]
    },
    {
      "type": "timeseries",
      "title": "5xx error rate by service (req/s)",
      "datasource": { "type": "prometheus", "uid": "aenigma-prometheus" },
      "gridPos": { "h": 9, "w": 12, "x": 12, "y": 0 },
      "targets": [
        {
          "expr": "sum(rate(http_requests_total{status=~\"5..\"}[5m])) by (service)",
          "legendFormat": "{{service}}"
        }
      ]
    },
    {
      "type": "traces",
      "title": "Recent slow traces (>1s)",
      "datasource": { "type": "tempo", "uid": "aenigma-tempo" },
      "gridPos": { "h": 12, "w": 24, "x": 0, "y": 9 },
      "targets": [
        { "queryType": "traceqlSearch", "query": "{ duration > 1s }", "limit": 50 }
      ]
    }
  ]
}
```

- [ ] **Step 3: Confirm the dashboards-infra mount picks up the file**

The compose grafana block already mounts `../infra/grafana/dashboards` → `/var/lib/grafana/dashboards-infra` (read-only), and `product-analytics.json` (Plan 2) lives there and provisions. Confirm a dashboard provider points at that directory:
```bash
grep -rn "dashboards-infra\|/var/lib/grafana/dashboards" /data/animeenigma/docker/grafana/provisioning/dashboards/ 2>/dev/null | head
```
Expected: a provider config references the infra dashboards path. (If only `product-analytics.json` is picked up but not subfolders, place `backend-tracing.json` in the same directory — which Step 2 already does.)

- [ ] **Step 4: Restart Grafana and verify the dashboard provisions**

Run:
```bash
cd /data/animeenigma
make restart-grafana
sleep 8
docker compose -f docker/docker-compose.yml logs grafana | grep -iE "backend-tracing|dashboard|error" | tail -10
```
Expected: dashboard provisioned, no JSON/parse errors.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add infra/grafana/dashboards/backend-tracing.json docker/grafana/provisioning/datasources/datasources.yml
git commit -m "feat(tracing): add backend tracing & latency Grafana dashboard"
```

---

### Task 12: End-to-end verification

**Files:** none (runtime verification + redeploy)

- [ ] **Step 1: Redeploy the traced services**

Bring up the new infra and redeploy the edited services:
```bash
cd /data/animeenigma
docker compose -f docker/docker-compose.yml up -d tempo-init tempo otel-collector
make redeploy-gateway
for s in auth catalog player rooms scheduler streaming themes notifications watch-together scraper library; do
  make redeploy-$s
done
make redeploy-web
make health
```
Expected: all services healthy.

- [ ] **Step 2: Generate a trace through the gateway**

Drive a real request that fans out (anime detail hits gateway → catalog → DB). Mint a traceparent so you can find it:
```bash
TP="00-$(openssl rand -hex 16)-$(openssl rand -hex 8)-01"
echo "traceparent: $TP"
curl -s -H "traceparent: $TP" "http://localhost:8000/api/anime/search?q=naruto" -o /dev/null -w "%{http_code}\n"
TRACE_ID=$(echo "$TP" | cut -d- -f2)
echo "trace_id: $TRACE_ID"
```
Expected: HTTP 200.

- [ ] **Step 3: Confirm the trace reached Tempo**

Wait for the collector's `decision_wait` (10s) + batch, then query Tempo:
```bash
sleep 15
curl -s "http://localhost:3200/api/traces/$TRACE_ID" -o /dev/null -w "%{http_code}\n"
```
Expected: `200` (trace found). If `404`, check `docker compose logs otel-collector tempo` for export errors; a sampled-out trace can also 404 — the explicit `-01` sampled flag plus the `errors`/`slow` tail policies should keep targeted curls, but retry with a request that is slow or 5xx if needed.

- [ ] **Step 4: Confirm spans span multiple services**

Run:
```bash
curl -s "http://localhost:3200/api/traces/$TRACE_ID" | grep -oE '"serviceName":"[^"]+"|"service.name"[^,]*' | sort -u
```
Expected: at least `gateway` and `catalog` appear — proving FE→BE propagation across services.

- [ ] **Step 5: Confirm trace↔logs correlation**

In Grafana (`https://animeenigma.ru/admin/grafana` → Explore → Loki), query `{service_name="catalog"} | json | trace_id="<TRACE_ID>"`. Expected: log lines for that request carry the `trace_id` (written automatically by `libs/logger.WithContext`), and the derived-field link jumps to the Tempo trace.

- [ ] **Step 6: Confirm the browser stamps click trace_ids (live smoke)**

This step uses the browser against production as `ui_audit_bot` (the standard automation account) — mirror the Plan 2 smoke. After clicking a nav element that triggers an API call, verify a click event in Postgres carries a non-null `trace_id`:
```bash
docker compose -f docker/docker-compose.yml exec -T postgres psql -U postgres -d animeenigma -c \
  "SELECT event_type, path, left(trace_id, 12) AS trace_id, el_text FROM analytics_events WHERE event_type='click' AND trace_id IS NOT NULL AND trace_id <> '' ORDER BY received_at DESC LIMIT 5;"
```
Expected: at least one recent click row with a populated `trace_id`. Clean up any smoke rows afterward via the internal erase path (same as Plan 2). (If you cannot drive the browser in this session, mark this step as deferred and note it — Steps 2–5 already prove the backend pipeline.)

- [ ] **Step 7: Confirm the kill switch**

Set `TRACING_ENABLED: "false"` on one service block in `docker-compose.yml`, `make redeploy-<that-svc>`, and confirm it still serves traffic with no OTLP export attempts in its logs. Restore `"true"` and redeploy. Expected: clean disable/enable — the service serves normally. Because the W3C propagator is installed unconditionally (Task 1 Step 2), trace **context still flows through** the disabled service to its downstreams; it simply contributes **no spans of its own** to the trace. (So a trace passing through a disabled service shows a gap at that hop, not a split into two traces — an honest, acceptable degradation.)

- [ ] **Step 8: Final verification summary**

Confirm against the design-spec acceptance criteria (§9): single multi-service trace ✓ (Step 4), trace↔logs jump ✓ (Step 5), click carries trace_id ✓ (Step 6), kill switch ✓ (Step 7), nothing fatal when the collector is down ✓ (Step 7 / WARN-not-fatal init). No commit — this task is verification only.

---

## Self-Review

**1. Spec coverage (design-spec §2, §4-trace_id, §6, §8 milestones 4-6, §9):**
- §2 service instrumentation (`tracing.New`, otelhttp, GORM) → Tasks 1, 3, 7, 8. ✓
- §2 gateway propagation (the core FE→BE fix) → Task 7. ✓
- §2 OTel Collector + tail sampling → Task 5. ✓
- §2 Tempo + MinIO storage + retention → Task 4. ✓
- §2 Grafana Tempo datasource + trace→logs by trace_id → Task 6. ✓
- §2/§4.8 browser axios traceparent + stamp on click → Tasks 9, 10. ✓
- §6 backend-observability dashboard → Task 11. ✓
- §9 acceptance (single trace; trace→logs; click trace_id; kill switch; collector-down non-fatal) → Task 12. ✓
- §3 clickstream `trace_id` ingestion → already shipped in Plan 1 (verified: `wireEvent.TraceID`), no task needed. ✓
- §5 privacy / 90-day purge / erasure → shipped in Plans 1 & 2, out of scope here. ✓

**2. Placeholder scan:** No "TBD/TODO/handle errors" placeholders. The only intentional `<...>` tokens are per-service substitutions in Task 8 (service name) and the Loki uid in Task 6 — each with an explicit "read this from the existing file" instruction, not a vague gap. Dependency versions use `@latest` + a `go mod tidy`/build verify (with a pinned fallback for otelhttp) because the exact contrib version that matches otel v1.38.0 must be resolved against the live module graph.

**3. Type consistency:** Helper names are consistent across tasks: `HTTPMiddleware` (T1, used T7/T8), `WrapTransport`/`NewHTTPClient` (T2, used T7), `FromEnv`/`InitFromEnv` (T3, used T7/T8), `gormtrace.InstrumentGORM` (T3, used T8). Frontend: `newTraceparent` (T9, used T10), `registerClickForTrace`/`stampTrace` (T9, used T10), `AnalyticsEvent.trace_id` (T10 type matches T9 store usage and the backend `json:"trace_id"` wire field). Datasource uids `aenigma-tempo`/`aenigma-loki`/`aenigma-prometheus` are consistent across T6 and T11.

**4. Review fixes applied (gsd-code-reviewer pass, 2026-06-03):** C1 — **rooms** moved to the non-GORM set (it is Redis-only; Step D skips it). C3 — `HTTPMiddleware` now bypasses WebSocket upgrades (`Upgrade: websocket`) so the gateway WS proxy and watch-together `/ws` handshakes don't 500 under `otelhttp.NewHandler`. H1/H2 — `aenigma-prometheus` + `aenigma-loki` uids are now added explicitly (the real datasources had none) so dashboard panels and trace↔logs correlation resolve. H3/M1 — otelhttp pinned to `v0.63.0` and the GORM plugin pinned, with an explicit "otel core must stay v1.38.0" check, instead of `@latest`. M2 — the W3C propagator is installed unconditionally (context flows even through a tracing-disabled service). M3/L2/L3 — added load-bearing notes (no defensive traceparent-strip; auth endpoints skip; `enqueue` stores by reference).

## Effort Metrics (per `.planning/CONVENTIONS.md` — no days/hours)

- **UXΔ = +2 (Better)** — no direct end-user UI change, but unlocks FE→BE incident diagnosis and click↔backend attribution. Indirect but real.
- **CDI = 0.05 * 21** — wide spread (shared lib + 12-service wiring + 2 infra containers + frontend + Grafana), low shift (additive; existing systems untouched; default-off gate), Effort_Fib 21.
- **MVQ = Griffin 88%/82%** — composite build bridging known patterns; high slop-resistance because the bulk is activating dormant wiring behind a single uniform recipe and the helpers sit behind unit tests.
