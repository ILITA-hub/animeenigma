//go:build integration

// Package e2e contains the capstone GPU-free, model=mock end-to-end integration
// test for the anime upscaler. It wires the REAL server-side pieces in-process
// (orchestrator + leaser + sweeper + hub + finalizer + segment data-plane +
// admin handler + exec relay + enroll store) on an httptest server backed by a
// real Postgres (the Task-3 integration helper, so SELECT … FOR UPDATE SKIP
// LOCKED leasing is genuinely exercised), drives the REAL worker agent against
// it (as a subprocess — see the worker-process rationale below), and asserts the
// full chain:
//
//	job submit → segment → lease → worker upscale (mock) → segment PUT →
//	reassemble → remux → upload (recording MinIO fake)
//
// plus: mid-job spot-resume (kill the worker after ≥1 segment, restart it, and
// assert NO lost / NO duplicate segments), a remote-shell exec round-trip
// (echo ok + the audit log line), and a /metrics scrape for the upscale_*
// series.
//
// WHY THE WORKER RUNS AS A SUBPROCESS (controller-sanctioned fallback):
// The worker lives in a SEPARATE module (github.com/ILITA-hub/animeenigma/worker)
// and its agent lives under .../worker/internal/agent. Go's internal-package
// rule forbids importing .../worker/internal/* from any module outside the
// .../worker/ tree, and the worker module is deliberately NOT part of the root
// go.work (it builds with GOWORK=off). An in-process import from this upscaler
// test module is therefore impossible at the language level — not merely
// awkward. Per the brief, the acceptable fallback is to run the REAL worker
// binary as a subprocess pointed at the httptest server. This still exercises
// the genuine agent code path end-to-end (enroll → WS → lease loop → real mock
// upscale pipeline → segment GET/PUT → exec handler).
package e2e

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	gorillaws "github.com/gorilla/websocket"
	"github.com/minio/minio-go/v7"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/capability"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/controlplane"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/domain"
	upffmpeg "github.com/ILITA-hub/animeenigma/services/upscaler/internal/ffmpeg"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/handler"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/repo"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/service"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/source"
	"github.com/ILITA-hub/animeenigma/services/upscaler/internal/transport"
)

// ── Test-environment plumbing ────────────────────────────────────────────────

const (
	// segmentSeconds is a small override so the 8-second fixture splits into ≥2
	// chunks, giving the resume path multiple segments to span.
	segmentSeconds = 3

	// e2eCapabilitySecret is the shared HMAC secret used for session + segment
	// capability minting/verification. It must be set BEFORE any mint/verify and
	// BEFORE the worker subprocess starts (the worker re-enrolls and gets fresh
	// session triples signed with this secret on the server side).
	e2eCapabilitySecret = "e2e-upscaler-capstone-secret-key"
)

// tinyMKVPath is the deterministically generated fixture, populated by TestMain.
var tinyMKVPath string

// sharedMetricsCollector is a process-wide metrics.Collector. metrics.NewCollector
// registers its series on the DEFAULT Prometheus registry via promauto, which
// PANICS on a second call (duplicate registration). The mock capstone + the two
// T30 pull-on-demand tests each build a serverHarness, so the collector must be
// created exactly ONCE per process and reused across harnesses. (The /metrics
// scrape in the mock test only asserts series PRESENCE, not per-test isolation,
// so a shared collector is fine — and matches production, where the binary has
// one collector for its lifetime.)
var (
	sharedMetricsOnce sync.Once
	sharedMetrics     *metrics.Collector
)

func metricsCollector() *metrics.Collector {
	sharedMetricsOnce.Do(func() {
		sharedMetrics = metrics.NewCollector("upscaler")
	})
	return sharedMetrics
}

// workerBinPath is the prebuilt REAL worker binary, compiled once by TestMain.
// The worker is run as a subprocess (separate module + internal package makes an
// in-process import impossible — see the file header). We compile it ONCE and
// exec the binary directly rather than `go run ./cmd/worker`, so killing the
// process kills the actual worker (no intervening `go run` parent that the kill
// would orphan) — removing the prior spurious "worker maybe still alive" skip
// window. Empty when the worker build was skipped/failed in TestMain.
var workerBinPath string

// ffmpegAvailable reports whether ffmpeg + ffprobe are on PATH.
func binAvailable(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// TestMain generates the fixture once (deterministic, no committed binary) and
// runs the suite. The fixture is an 8-second testsrc + sine clip; at
// SEGMENT_SECONDS=3 the segmenter splits it into ≥2 chunks.
func TestMain(m *testing.M) {
	// Skip-guard env probes are evaluated inside the test (so `go test` still
	// compiles + reports a clean SKIP), but the fixture is cheap and shared, so
	// generate it here when ffmpeg is present.
	if binAvailable("ffmpeg") {
		dir, err := os.MkdirTemp("", "upscaler-e2e-fixture-")
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e: mkdir fixture dir: %v\n", err)
			os.Exit(1)
		}
		out := filepath.Join(dir, "tiny.mkv")
		// Deterministic synthetic clip: 8s @ 24fps 640x360 + 440Hz sine.
		// Force a keyframe every 1s (GOP 24 + forced keyframes) so the
		// stream-copy segmenter (`-c:v copy`, which can only cut at keyframes)
		// actually splits the 8s clip into ≥2 chunks at SEGMENT_SECONDS=3.
		// Without frequent keyframes the whole clip becomes a single segment.
		args := []string{
			"-hide_banner", "-nostats", "-y",
			"-f", "lavfi", "-i", "testsrc=duration=8:size=640x360:rate=24",
			"-f", "lavfi", "-i", "sine=frequency=440:duration=8",
			"-c:v", "libx264",
			"-g", "24", "-keyint_min", "24",
			"-force_key_frames", "expr:gte(t,n_forced*1)",
			"-c:a", "aac", "-t", "8",
			out,
		}
		ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			cancel()
			fmt.Fprintf(os.Stderr, "e2e: generate fixture failed: %v\n%s\n", err, stderr.String())
			os.Exit(1)
		}
		cancel()
		tinyMKVPath = out

		// Build the REAL worker binary ONCE so each subprocess execs the binary
		// directly (process-group-killable, no `go run` parent). Failure here is
		// non-fatal: requireEnv's worker-binary guard SKIPs the test with a precise
		// reproduction command rather than reporting a misleading failure.
		if root, ok := findRepoRoot(); ok {
			binDir, berr := os.MkdirTemp("", "upscaler-e2e-worker-")
			if berr == nil {
				bin := filepath.Join(binDir, "worker")
				bctx, bcancel := context.WithTimeout(context.Background(), 180*time.Second)
				build := exec.CommandContext(bctx, "go", "build", "-o", bin, "./cmd/worker")
				build.Dir = filepath.Join(root, "worker")
				// The worker is a SEPARATE module (GOWORK=off) pinned to go1.25.0.
				build.Env = append(os.Environ(), "GOWORK=off", "GOTOOLCHAIN=go1.25.0")
				var berrBuf bytes.Buffer
				build.Stderr = &berrBuf
				if err := build.Run(); err != nil {
					fmt.Fprintf(os.Stderr, "e2e: build worker binary failed (test will SKIP): %v\n%s\n", err, berrBuf.String())
				} else {
					workerBinPath = bin
				}
				bcancel()
				defer os.RemoveAll(binDir)
			}
		}

		code := m.Run()
		_ = os.RemoveAll(dir)
		os.Exit(code)
	}
	os.Exit(m.Run())
}

// findRepoRoot ascends from the working dir to the repository root (the dir
// containing worker/cmd/worker/main.go). Returns ("", false) if not found.
// Used from TestMain where no *testing.T is available (repoRoot is the T-based
// equivalent used inside tests).
func findRepoRoot() (string, bool) {
	wd, err := os.Getwd()
	if err != nil {
		return "", false
	}
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "worker", "cmd", "worker", "main.go")); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return "", false
}

// requireEnv asserts the externals this E2E needs are reachable; otherwise it
// SKIPs with a precise reproduction command. Never weakens assertions to fake a
// green run.
func requireEnv(t *testing.T) {
	t.Helper()
	if os.Getenv("INTEGRATION") != "1" {
		t.Skip("set INTEGRATION=1 to run the upscaler e2e integration test")
	}
	if !binAvailable("ffmpeg") || !binAvailable("ffprobe") {
		t.Skip("ffmpeg + ffprobe must be on PATH for the upscaler e2e test")
	}
	if tinyMKVPath == "" {
		t.Skip("fixture not generated (ffmpeg unavailable in TestMain)")
	}
	if workerBinPath == "" {
		t.Skip("worker binary not built in TestMain — run from the repo so " +
			"`cd worker && GOWORK=off GOTOOLCHAIN=go1.25.0 go build -o /tmp/worker ./cmd/worker` succeeds")
	}
}

// ── Postgres test-DB helper (mirrors the Task-3 integration helper) ──────────

func getenv(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// openIntegrationDB provisions a FRESH per-test Postgres database so the
// SELECT … FOR UPDATE SKIP LOCKED leasing path is genuinely exercised. It
// mirrors repo/segment_integration_test.go's openIntegrationDB. SQLite cannot
// stand in here (SKIP LOCKED is a Postgres-only hint), which is exactly why the
// no-lost/no-dup-segment resume assertion needs real Postgres.
func openIntegrationDB(t *testing.T) *gorm.DB {
	t.Helper()

	host := getenv("DB_HOST", getenv("PGHOST", "localhost"))
	port := getenv("DB_PORT", getenv("PGPORT", "5432"))
	user := getenv("DB_USER", getenv("PGUSER", "postgres"))
	pass := getenv("DB_PASSWORD", getenv("PGPASSWORD", "postgres"))

	dbName := fmt.Sprintf("upscaler_e2e_%d_%d", os.Getpid(), time.Now().UnixNano())

	adminDSN := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=postgres sslmode=disable",
		host, port, user, pass,
	)
	adminDB, err := gorm.Open(postgres.Open(adminDSN), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Skipf("e2e: integration postgres unavailable at %s:%s: %v "+
			"(start one, e.g. `docker run -e POSTGRES_PASSWORD=postgres -p 5432:5432 postgres:16-alpine`)",
			host, port, err)
	}
	if err := adminDB.Exec(fmt.Sprintf("CREATE DATABASE %s", dbName)).Error; err != nil {
		t.Fatalf("e2e: create database %s: %v", dbName, err)
	}

	dsn := fmt.Sprintf(
		"host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, pass, dbName,
	)
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	if err != nil {
		t.Fatalf("e2e: connect test db %s: %v", dbName, err)
	}

	// Mirror main.go's AutoMigrate set (incl. the enroll-token table the worker
	// enroll flow consumes). This is the REAL production schema path: Bug 2 (the
	// CurrentJobID `type:uuid` tag that made Postgres reject the "" sentinel and
	// blocked EVERY worker enroll) is now fixed in domain.UpscaleWorker, so
	// AutoMigrate creates current_job_id as text and the enroll path works with
	// NO schema workaround. If the prior `ALTER TABLE … TYPE text` hack were still
	// needed here, that would mean Bug 2 is not actually fixed.
	if err := db.AutoMigrate(
		&domain.UpscaleJob{},
		&domain.UpscaleSegment{},
		&domain.UpscaleWorker{},
		&domain.UpscaleModel{},
		&domain.UpscaleEnrollToken{},
	); err != nil {
		t.Fatalf("e2e: automigrate: %v", err)
	}

	t.Cleanup(func() {
		if sqlDB, _ := db.DB(); sqlDB != nil {
			_ = sqlDB.Close()
		}
		if err := adminDB.Exec(fmt.Sprintf("DROP DATABASE IF EXISTS %s WITH (FORCE)", dbName)).Error; err != nil {
			t.Logf("e2e: drop database %s (cleanup): %v", dbName, err)
		}
		if asqlDB, _ := adminDB.DB(); asqlDB != nil {
			_ = asqlDB.Close()
		}
	})
	return db
}

// ── Recording MinIO writer (mirrors fakeUploader's recording + playlist-last) ─
//
// OrchestratorDeps.Writer is an interface (EnsureBucket + Upload), so we pass a
// recording implementation rather than reach into the unexported
// newWriterWithUploader. It records the exact object key order the orchestrator
// hands it and captures the segments-done count at the moment the playlist is
// written, so we can assert the playlist-last invariant at the orchestrator →
// writer boundary (the byte-level PutObject ordering inside minio.Writer is
// covered by minio/writer_test.go's TestUpload_PlaylistLast_AndContentType).

type recordingWriter struct {
	mu             sync.Mutex
	bucketEnsured  int
	uploadedKeys   []string // object keys in the exact order they were uploaded
	segmentsAtPlay int      // # of .ts uploaded when playlist.m3u8 was uploaded (-1 = never)
	lastPrefix     string
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{segmentsAtPlay: -1}
}

func (w *recordingWriter) EnsureBucket(_ context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.bucketEnsured++
	return nil
}

func (w *recordingWriter) Upload(_ context.Context, prefix string, filePaths []string) (int64, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.lastPrefix = prefix
	var total int64
	segCount := 0
	for _, p := range filePaths {
		fi, err := os.Stat(p)
		if err != nil {
			return total, fmt.Errorf("recordingWriter: stat %q: %w", p, err)
		}
		key := prefix
		if !strings.HasSuffix(key, "/") {
			key += "/"
		}
		key += filepath.Base(p)
		w.uploadedKeys = append(w.uploadedKeys, key)
		total += fi.Size()
		if strings.HasSuffix(p, "playlist.m3u8") {
			w.segmentsAtPlay = segCount
		} else if strings.HasSuffix(p, ".ts") {
			segCount++
		}
	}
	return total, nil
}

func (w *recordingWriter) snapshot() (keys []string, segsAtPlay, bucketEnsured int, prefix string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.uploadedKeys...), w.segmentsAtPlay, w.bucketEnsured, w.lastPrefix
}

// ── In-memory MinIO stand-in for model artifacts (T30 pull-on-demand) ────────
//
// The model admin upload handler (T26) streams the artifact to a modelUploader
// (PutObject) and the worker-facing serve handler (T27) reads it back via a
// modelObjectGetter (GetObject). Rather than spin a real MinIO, we satisfy BOTH
// minimal interfaces with one map[object]→bytes store. This keeps the harness
// dependency-free while exercising the REAL admin-upload + checksum + serve
// streaming code paths: the bytes the worker fetches are the exact bytes the
// admin handler stored, and the X-Model-Checksum the worker verifies is the
// SHA-256 the admin handler computed over those bytes. The method signatures
// match the minio.Uploader surface the handlers depend on (structural
// satisfaction of the unexported modelUploader / modelObjectGetter interfaces).

type memModelStore struct {
	mu      sync.Mutex
	objects map[string][]byte // key: bucket+"/"+object
}

func newMemModelStore() *memModelStore {
	return &memModelStore{objects: make(map[string][]byte)}
}

func (s *memModelStore) key(bucket, object string) string { return bucket + "/" + object }

// PutObject satisfies handler.modelUploader. It reads the full stream into
// memory (model artifacts in this test are tiny tars) and stores it by key.
func (s *memModelStore) PutObject(_ context.Context, bucket, object string, reader interface {
	Read(p []byte) (int, error)
}, _ int64, _ minio.PutObjectOptions) (minio.UploadInfo, error) {
	data, err := io.ReadAll(reader)
	if err != nil {
		return minio.UploadInfo{}, err
	}
	s.mu.Lock()
	s.objects[s.key(bucket, object)] = data
	s.mu.Unlock()
	return minio.UploadInfo{Bucket: bucket, Key: object, Size: int64(len(data))}, nil
}

// GetObject satisfies handler.modelObjectGetter. It returns a ReadCloser over
// the stored bytes, or a not-found error mirroring MinIO's "key does not exist".
func (s *memModelStore) GetObject(_ context.Context, bucket, object string) (io.ReadCloser, error) {
	s.mu.Lock()
	data, ok := s.objects[s.key(bucket, object)]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("memModelStore: object %q does not exist", s.key(bucket, object))
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// ── Model-serve hit recorder (T30) ───────────────────────────────────────────
//
// An OUTER http.Handler that wraps the whole upscaler router and records every
// GET /worker/models/{name} request together with the status the REAL serve
// handler returned. The wrapped router is fully unmodified — the capability
// verify + MinIO stream + 200/404 logic under test is the genuine production
// path; this shim only observes (name, status) so the e2e can assert the worker's
// pull-on-demand GET actually reached the capability-gated endpoint and what it
// answered. It does NOT use chi.URLParam (it sits ABOVE the router, before route
// matching) — it parses the model name from the URL path directly.

const modelServePathPrefix = "/worker/models/"

type modelServeHit struct {
	name   string
	status int
}

type modelServeRecorder struct {
	mu    sync.Mutex
	inner http.Handler
	hits  []modelServeHit
}

func newModelServeRecorder(inner http.Handler) *modelServeRecorder {
	return &modelServeRecorder{inner: inner}
}

// statusSniffer captures the first WriteHeader status (defaulting to 200 if the
// handler writes a body without an explicit WriteHeader).
type statusSniffer struct {
	http.ResponseWriter
	status      int
	wroteHeader bool
}

func (s *statusSniffer) WriteHeader(code int) {
	if !s.wroteHeader {
		s.status = code
		s.wroteHeader = true
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusSniffer) Write(b []byte) (int, error) {
	if !s.wroteHeader {
		s.status = http.StatusOK
		s.wroteHeader = true
	}
	return s.ResponseWriter.Write(b)
}

// Unwrap lets http.NewResponseController (used by ServeModel to clear the write
// deadline before streaming) reach the underlying ResponseWriter through the shim.
func (s *statusSniffer) Unwrap() http.ResponseWriter { return s.ResponseWriter }

func (rec *modelServeRecorder) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only sniff the model-serve data-plane GETs; everything else passes straight
	// through with no wrapping overhead.
	if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, modelServePathPrefix) {
		name := strings.TrimPrefix(r.URL.Path, modelServePathPrefix)
		sniff := &statusSniffer{ResponseWriter: w, status: http.StatusOK}
		rec.inner.ServeHTTP(sniff, r)
		rec.mu.Lock()
		rec.hits = append(rec.hits, modelServeHit{name: name, status: sniff.status})
		rec.mu.Unlock()
		return
	}
	rec.inner.ServeHTTP(w, r)
}

func (rec *modelServeRecorder) snapshot() []modelServeHit {
	rec.mu.Lock()
	defer rec.mu.Unlock()
	return append([]modelServeHit(nil), rec.hits...)
}

// countFor returns how many recorded hits for model `name` had the given status.
func (rec *modelServeRecorder) countFor(name string, status int) int {
	n := 0
	for _, h := range rec.snapshot() {
		if h.name == name && h.status == status {
			n++
		}
	}
	return n
}

// ── Recording exec router (captures the worker's real exec output) ───────────
//
// The worker only RUNS a command carried in exec_open.Data (allowlist mode); the
// production ExecRelay.Open intentionally never sets Data (interactive sessions
// stream stdin, which this worker build does not yet accept inbound). So to
// observe the REAL worker executing `echo ok` and streaming output back over the
// REAL WS, we (a) drive the production ExecRelay for the audit line + a real
// exec_open frame, and (b) temporarily swap the hub's ExecRouter to this
// recorder and send a Data-carrying exec_open via the real hub, capturing the
// worker's genuine exec_data / exec_close frames. This is a recording test
// double (explicitly sanctioned), NOT a reimplementation of relay logic.

type recordingExecRouter struct {
	mu     sync.Mutex
	frames []controlplane.Frame
}

func (r *recordingExecRouter) DeliverFromWorker(f controlplane.Frame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
}

func (r *recordingExecRouter) snapshot() []controlplane.Frame {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]controlplane.Frame(nil), r.frames...)
}

// ── Server harness wiring (REAL constructors, mirrors main.go) ───────────────

type serverHarness struct {
	srv          *httptest.Server
	db           *gorm.DB
	jobs         *repo.JobRepository
	segs         *repo.SegmentRepository
	workers      *repo.WorkerRepository
	models       *repo.ModelRepository
	modelStore   *memModelStore // in-memory MinIO stand-in for model artifacts
	modelGets    *modelServeRecorder
	hub          *controlplane.Hub
	relay        *controlplane.ExecRelay
	auditBuf     *syncBuffer
	writer       *recordingWriter
	orchestrator *service.Orchestrator
	sweeper      *service.Sweeper
	stagingDir   string
	torrentsDir  string
	cancelBg     context.CancelFunc
}

// syncBuffer is a goroutine-safe bytes.Buffer wrapper for the exec audit sink.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.Write(p)
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// newServerHarness wires the real upscaler server stack in-process on an
// httptest server, exactly as cmd/upscaler-api/main.go does, except: the MinIO
// writer is the recording fake (Writer is an interface), and ffmpeg/ffprobe run
// against the local binaries.
func newServerHarness(t *testing.T, db *gorm.DB) *serverHarness {
	t.Helper()
	log := logger.Default()

	// Capability secret must be configured before any mint/verify. capability.Init
	// is sync.Once-gated; if a prior test in the same process already configured
	// it, Init is a no-op — we rely on Enabled() to confirm a usable secret.
	capability.Init(e2eCapabilitySecret)
	if !capability.Enabled() {
		t.Fatal("e2e: capability not enabled after Init — JOB_CAPABILITY_SECRET wiring broken")
	}

	stagingDir := t.TempDir()
	torrentsDir := t.TempDir()

	jobRepo := repo.NewJobRepository(db)
	segmentRepo := repo.NewSegmentRepository(db)
	workerRepo := repo.NewWorkerRepository(db)
	modelRepo := repo.NewModelRepository(db)

	leaser := service.NewLeaserWithLogger(jobRepo, segmentRepo, workerRepo, log)
	hub := controlplane.NewHub(leaser, workerRepo, log)
	enrollStore := controlplane.NewGormEnrollStore(db)
	sweeper := service.NewSweeperWithLogger(segmentRepo, workerRepo, log)

	resolver := source.NewResolver(torrentsDir, stagingDir)
	prober := source.NewProber("") // ffprobe on PATH
	// REAL ffmpeg, no shim. Bug 1 (the subs demux ran `-c:s copy {out}/subs.mks`
	// with no `-f`, so ffmpeg could not infer a muxer from the unregistered `.mks`
	// extension and failed muxer selection → EVERY job failed at demux) is fixed
	// in the Segmenter itself (it now passes `-f matroska` before both the `.mks`
	// and `.mka` outputs). The earlier `writeFfmpegShim` that injected `-f matroska`
	// has been removed: the test now exercises the genuine production ffmpeg argv.
	// If a shim were still required for the suite to pass, that would mean Bug 1
	// is not actually fixed in the code.
	segmenter := upffmpeg.NewSegmenter("ffmpeg")
	finalizer := upffmpeg.NewFinalizer("ffmpeg")
	writer := newRecordingWriter()

	orchestrator := service.NewOrchestrator(service.OrchestratorDeps{
		Jobs:           jobRepo,
		Segments:       segmentRepo,
		Resolver:       resolver,
		Prober:         prober,
		Segmenter:      segmenter,
		Finalizer:      finalizer,
		Writer:         writer,
		StagingDir:     stagingDir,
		SegmentSeconds: segmentSeconds,
		Log:            log,
	})

	segmentHandler := handler.NewSegmentHandler(stagingDir, jobRepo, segmentRepo, log)

	adminHandler := handler.NewAdminHandler(jobRepo, workerRepo, 2, "", log).
		WithCommander(controlplane.NewIssuer(hub))

	// Real exec relay with a recording audit sink.
	auditBuf := &syncBuffer{}
	relay := controlplane.NewExecRelay(hub, controlplane.ExecRelayConfig{Enabled: true}, log, auditBuf)
	hub.SetExecRouter(relay)
	shellHandler := handler.NewExecShellHandler(relay, log)

	// ── T30: dynamic-model data plane ────────────────────────────────────────
	// Wire the REAL admin-upload (T26) + worker-serve (T27) handlers against an
	// in-memory MinIO stand-in (model artifacts are tiny tars) and the real
	// ModelRepository (same Postgres test DB). The serve handler resolves the
	// latest version by name (GetLatest) and verifies the name-bound capability
	// handle the hub minted in the lease grant. The whole router is then wrapped
	// in a recorder so the pull-on-demand scenario can assert the worker's GET
	// reached the capability-gated endpoint with a 200 (fetch+verify) and a 404
	// for the unknown-model scenario.
	const modelBucket = "raw-library"
	modelStore := newMemModelStore()
	modelAdminHandler := handler.NewModelAdminHandler(modelRepo, modelStore, modelBucket, log)
	modelServeHandler := handler.NewModelServeHandler(modelRepo, modelStore, modelBucket, log)

	router := transport.NewRouter(log, metricsCollector(), hub, enrollStore, segmentHandler, adminHandler, shellHandler, modelAdminHandler, modelServeHandler)
	modelGets := newModelServeRecorder(router)
	srv := httptest.NewServer(modelGets)

	bgCtx, cancelBg := context.WithCancel(context.Background())
	go sweeper.Run(bgCtx)
	go orchestrator.Run(bgCtx)

	t.Cleanup(func() {
		cancelBg()
		orchestrator.Stop()
		sweeper.Stop()
		srv.Close()
	})

	return &serverHarness{
		srv:          srv,
		db:           db,
		jobs:         jobRepo,
		segs:         segmentRepo,
		workers:      workerRepo,
		models:       modelRepo,
		modelStore:   modelStore,
		modelGets:    modelGets,
		hub:          hub,
		relay:        relay,
		auditBuf:     auditBuf,
		writer:       writer,
		orchestrator: orchestrator,
		sweeper:      sweeper,
		stagingDir:   stagingDir,
		torrentsDir:  torrentsDir,
		cancelBg:     cancelBg,
	}
}

// stageSource places the fixture under {torrentsDir}/{infohash}/tiny.mkv so the
// real source.Resolver can locate + stage it.
func (h *serverHarness) stageSource(t *testing.T, infohash string) {
	t.Helper()
	dir := filepath.Join(h.torrentsDir, infohash)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("e2e: mkdir torrents/%s: %v", infohash, err)
	}
	dst := filepath.Join(dir, "tiny.mkv")
	in, err := os.Open(tinyMKVPath)
	if err != nil {
		t.Fatalf("e2e: open fixture: %v", err)
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		t.Fatalf("e2e: create staged source: %v", err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		t.Fatalf("e2e: copy fixture: %v", err)
	}
}

// createJob POSTs a queued job through the REAL admin API. The admin surface is
// gated by requireGatewayInternal, so we inject the X-Gateway-Internal header
// exactly as the gateway proxy does. The model name is what threads through the
// leaser → lease grant → worker model-selection path (T25/T29), so it is a
// parameter: "mock" exercises the built-in no-fetch path, any other name
// exercises pull-on-demand.
func (h *serverHarness) createJob(t *testing.T, shikimoriID string, episode int, infohash, model string) string {
	t.Helper()
	body, _ := json.Marshal(map[string]any{
		"shikimori_id":     shikimoriID,
		"episode":          episode,
		"model":            model,
		"scale":            2,
		"library_infohash": infohash,
	})
	req, err := http.NewRequest(http.MethodPost, h.srv.URL+"/api/upscale/jobs", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("e2e: build create-job request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Gateway-Internal", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("e2e: create-job request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("e2e: create job: status %d, body %s", resp.StatusCode, raw)
	}
	// httputil.Created wraps the payload in {success, data}.
	var env struct {
		Data domain.UpscaleJob `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("e2e: decode create-job response: %v", err)
	}
	if env.Data.ID == "" {
		t.Fatal("e2e: created job has empty ID")
	}
	return env.Data.ID
}

// buildModelTAR builds a valid model artifact tar in memory: a TAR archive
// containing exactly {name}.param and {name}.bin (the layout Manager.Install
// extracts). The contents are arbitrary but deterministic so the SHA-256 the
// admin handler computes is stable. Mirrors the worker package's
// buildModelTARBytes helper (which lives in a separate module and cannot be
// imported here).
func buildModelTAR(t *testing.T, name string) []byte {
	t.Helper()
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	files := map[string]string{
		name + ".param": "param-weights-for-" + name + "\n",
		name + ".bin":   "bin-weights-for-" + name + "\n",
	}
	// Deterministic order for a stable checksum.
	for _, fn := range []string{name + ".param", name + ".bin"} {
		body := files[fn]
		if err := tw.WriteHeader(&tar.Header{
			Name: fn, Mode: 0o644, Size: int64(len(body)), Typeflag: tar.TypeReg,
		}); err != nil {
			t.Fatalf("e2e: tar header %q: %v", fn, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("e2e: tar write %q: %v", fn, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("e2e: tar close: %v", err)
	}
	return buf.Bytes()
}

// uploadModel registers a model server-side via the REAL T26 admin upload
// endpoint (POST /api/upscale/models, X-Gateway-Internal gated, multipart). The
// handler streams the artifact to the in-memory MinIO stand-in, computes the
// SHA-256, and upserts the upscale_models row — exactly the production path an
// operator's `curl -F` would drive. Returns the artifact bytes + the checksum
// the server stored (read back from the model row) so the caller can assert the
// worker fetches and checksum-verifies the very same bytes.
func (h *serverHarness) uploadModel(t *testing.T, name, version string) (artifact []byte, checksum string) {
	t.Helper()
	artifact = buildModelTAR(t, name)

	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	_ = mw.WriteField("name", name)
	_ = mw.WriteField("version", version)
	_ = mw.WriteField("scale", "2")
	fw, err := mw.CreateFormFile("artifact", name+".tar")
	if err != nil {
		t.Fatalf("e2e: create form file: %v", err)
	}
	if _, err := fw.Write(artifact); err != nil {
		t.Fatalf("e2e: write artifact to form: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("e2e: close multipart writer: %v", err)
	}

	req, err := http.NewRequest(http.MethodPost, h.srv.URL+"/api/upscale/models", &body)
	if err != nil {
		t.Fatalf("e2e: build upload-model request: %v", err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Gateway-Internal", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("e2e: upload-model request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		raw, _ := io.ReadAll(resp.Body)
		t.Fatalf("e2e: upload model: status %d, body %s", resp.StatusCode, raw)
	}
	var env struct {
		Data domain.UpscaleModel `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		t.Fatalf("e2e: decode upload-model response: %v", err)
	}
	if env.Data.Checksum == "" {
		t.Fatal("e2e: uploaded model has empty checksum")
	}
	// Confirm the stored checksum matches the SHA-256 of the bytes we sent — the
	// worker will verify the served bytes against this exact value.
	want := sha256Hex(artifact)
	if env.Data.Checksum != want {
		t.Fatalf("e2e: stored model checksum %q != sha256(artifact) %q", env.Data.Checksum, want)
	}
	return artifact, env.Data.Checksum
}

// sha256Hex returns the lowercase hex SHA-256 of b.
func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// seedEnrollToken inserts a fresh single-use enroll token and returns it.
func (h *serverHarness) seedEnrollToken(t *testing.T) string {
	t.Helper()
	tok := "e2e-tok-" + uuid.NewString()
	if err := h.db.Create(&domain.UpscaleEnrollToken{Token: tok, CreatedAt: time.Now().UTC()}).Error; err != nil {
		t.Fatalf("e2e: seed enroll token: %v", err)
	}
	return tok
}

// ── Worker subprocess control ────────────────────────────────────────────────

// workerProc is a handle to a running worker subprocess.
type workerProc struct {
	cmd    *exec.Cmd
	stderr *syncBuffer
	pgid   int // process-group id (== pid; we start it as its own group leader)
}

// repoRoot walks up from the test package dir to the repository root (where the
// worker/ module lives).
func repoRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd() // .../services/upscaler/internal/e2e
	if err != nil {
		t.Fatalf("e2e: getwd: %v", err)
	}
	// Ascend until we find a directory containing worker/cmd/worker/main.go.
	dir := wd
	for i := 0; i < 8; i++ {
		if _, err := os.Stat(filepath.Join(dir, "worker", "cmd", "worker", "main.go")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("e2e: could not locate repo root (worker/cmd/worker/main.go) from %s", wd)
	return ""
}

// startWorker launches the REAL worker agent as a subprocess (the in-process
// import is impossible — separate module + internal package; see file header),
// pointed at the httptest server with model=mock and a fresh enroll token.
//
// It execs the PREBUILT worker binary directly (compiled once in TestMain) — NOT
// `go run ./cmd/worker`. `go run` would fork a child worker process under a
// parent `go` process, so Process.Kill() would only kill the parent and orphan
// the actual worker (the prior flake). The binary is started as its own process-
// group leader (Setpgid) so stop() can SIGKILL the whole group atomically,
// reaping any grandchildren (ffmpeg / nvidia-smi spawned by the pipeline).
func (h *serverHarness) startWorker(t *testing.T, root, enrollToken string) *workerProc {
	t.Helper()
	stderr := &syncBuffer{}
	// Per-worker writable models dir: the extraction target for pull-on-demand
	// Install (the production default "/models" is not writable by the test user).
	// Each worker gets its own dir so a pull installed by one worker doesn't leak
	// into another's manager (each subprocess builds its own Manager from this).
	modelsDir := t.TempDir()
	cmd := exec.Command(workerBinPath)
	cmd.Dir = filepath.Join(root, "worker")
	cmd.Env = append(os.Environ(),
		"SERVER_URL="+h.srv.URL,
		"ENROLL_TOKEN="+enrollToken,
		// MODEL is intentionally NOT set — it was removed in T28; the worker boots
		// with only the built-in mock (no PREINSTALLED_MODELS) and pulls any other
		// model on demand. Setting it would have no effect (config ignores it).
		"MODE=batch",
		"SCALE=2",
		"MODELS_DIR="+modelsDir,
		// Fast telemetry cadence so the metrics/heartbeat assertions observe real
		// frames within the test's polling windows (production defaults are 5s/10s).
		"HEARTBEAT_INTERVAL=300ms",
		"METRICS_INTERVAL=300ms",
		// NOTE: the worker never signs capabilities — it only carries the session
		// triple + per-segment handles minted server-side, so JOB_CAPABILITY_SECRET
		// is intentionally NOT in the worker env (only the server holds it).
	)
	cmd.Stderr = stderr
	cmd.Stdout = io.Discard
	// Start the worker as its own process-group leader so the whole tree is
	// killable atomically (Minor 1 fix).
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("e2e: start worker subprocess: %v", err)
	}
	return &workerProc{cmd: cmd, stderr: stderr, pgid: cmd.Process.Pid}
}

// exited reports whether the worker subprocess has already exited, and if so its
// exit code (best-effort: -1 when the code can't be determined). It is
// NON-BLOCKING and does NOT reap the process (calling Wait here would race the
// stop()/t.Cleanup reaper). It probes liveness with signal 0 — a "process does
// not exist" / ESRCH means the worker is gone. Used by the pull-on-demand tests
// to assert the worker stayed alive (a fetch failure must never crash it).
func (w *workerProc) exited() (bool, int) {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return true, -1
	}
	// Signal 0 performs error checking without actually sending a signal.
	if err := w.cmd.Process.Signal(syscall.Signal(0)); err != nil {
		// ESRCH (no such process) → exited. Any other error (e.g. EPERM, which
		// shouldn't happen for our own child) we conservatively treat as alive.
		if err == os.ErrProcessDone || strings.Contains(err.Error(), "process already finished") || strings.Contains(err.Error(), "no such process") {
			return true, -1
		}
		return false, 0
	}
	return false, 0
}

// stop kills the worker subprocess group (the worker + any ffmpeg/nvidia-smi
// grandchildren) and waits for it to exit. Because the worker is its own
// process-group leader (Setpgid in startWorker), syscall.Kill(-pgid, SIGKILL)
// signals the entire group, so no child survives the kill (Minor 1 fix). Calling
// stop() more than once (explicit + t.Cleanup belt-and-suspenders) is safe.
func (w *workerProc) stop() {
	if w == nil || w.cmd == nil || w.cmd.Process == nil {
		return
	}
	if w.pgid > 0 {
		_ = syscall.Kill(-w.pgid, syscall.SIGKILL)
	}
	// Also signal the leader directly in case Setpgid did not take effect.
	_ = w.cmd.Process.Kill()
	_, _ = w.cmd.Process.Wait()
}

// ── Polling helpers ──────────────────────────────────────────────────────────

func (h *serverHarness) jobStatus(t *testing.T, jobID string) domain.JobStatus {
	t.Helper()
	job, err := h.jobs.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("e2e: get job %s: %v", jobID, err)
	}
	return job.Status
}

func (h *serverHarness) segCounts(t *testing.T, jobID string) (pending, leased, done int) {
	t.Helper()
	p, l, d, err := h.segs.Counts(context.Background(), jobID)
	if err != nil {
		t.Fatalf("e2e: seg counts %s: %v", jobID, err)
	}
	return p, l, d
}

// waitFor polls fn every 200ms until it returns true or the deadline passes.
func waitFor(t *testing.T, what string, timeout time.Duration, fn func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("e2e: timed out after %s waiting for: %s", timeout, what)
}

// ════════════════════════════════════════════════════════════════════════════
// The capstone test.
// ════════════════════════════════════════════════════════════════════════════

func TestMockE2E_FullChainWithSpotResumeExecAndMetrics(t *testing.T) {
	requireEnv(t)

	db := openIntegrationDB(t)
	h := newServerHarness(t, db)
	root := repoRoot(t)

	const (
		shikimoriID = "57466"
		episode     = 1
		infohash    = "e2ee2ee2ee2ee2ee2ee2ee2ee2ee2ee2ee2ee2ee" // 40-hex-ish
	)
	h.stageSource(t, infohash)

	jobID := h.createJob(t, shikimoriID, episode, infohash, "mock")
	t.Logf("created job %s", jobID)

	// ── Assert the job starts queued (it may already have advanced by the time
	// we read it, since the orchestrator's first tick is immediate; so we only
	// require it has NOT skipped to a terminal/failed state). ────────────────
	if st := h.jobStatus(t, jobID); st == domain.JobFailed || st == domain.JobCancelled {
		t.Fatalf("job entered terminal state %q before processing", st)
	}

	// ── Wait until the orchestrator has segmented (status → upscaling and ≥2
	// segments seeded — proving the 8s/3s split produced multiple chunks). ───
	waitFor(t, "job to reach upscaling with ≥2 segments", 90*time.Second, func() bool {
		st := h.jobStatus(t, jobID)
		if st == domain.JobFailed {
			job, _ := h.jobs.Get(context.Background(), jobID)
			t.Fatalf("job failed during segmenting: %q", job.ErrorText)
		}
		p, l, d := h.segCounts(t, jobID)
		return st == domain.JobUpscaling && (p+l+d) >= 2
	})

	job, _ := h.jobs.Get(context.Background(), jobID)
	totalSegments := job.SegmentCount
	if totalSegments < 2 {
		t.Fatalf("expected ≥2 segments from the 8s fixture at %ds, got %d", segmentSeconds, totalSegments)
	}
	t.Logf("job segmented into %d segments; status=upscaling", totalSegments)

	// ── Start worker #1, wait until ≥1 segment is DONE, then KILL it mid-job. ─
	tok1 := h.seedEnrollToken(t)
	w1 := h.startWorker(t, root, tok1)
	t.Cleanup(w1.stop) // belt-and-suspenders; we stop it explicitly below

	waitFor(t, "worker #1 to complete ≥1 segment", 120*time.Second, func() bool {
		_, _, done := h.segCounts(t, jobID)
		return done >= 1
	})
	_, _, doneBeforeKill := h.segCounts(t, jobID)
	t.Logf("worker #1 completed %d/%d segment(s); killing it mid-job", doneBeforeKill, totalSegments)

	// Guard the spot-resume semantics: the job must NOT already be finished, so
	// the restart genuinely spans the kill (multi-segment resume path).
	if doneBeforeKill >= totalSegments {
		t.Skipf("worker completed all %d segments before kill could land — fixture too small "+
			"to exercise mid-job resume; rerun (timing) or lower SEGMENT_SECONDS", totalSegments)
	}

	// Kill worker #1 mid-job.
	w1.stop()
	t.Log("worker #1 killed")

	// The killed worker may have left one segment leased (in flight). The sweeper
	// re-leases it after the lease TTL — but that TTL is 10 minutes, too long for
	// a test. Instead we assert resume WITHOUT relying on the sweeper: a fresh
	// worker leases the remaining PENDING segments, and the previously-leased
	// in-flight segment (if any) is re-leased by the sweeper. To keep the test
	// bounded, expire the stale lease directly via the REAL repo (this is the
	// exact operation the sweeper performs on its tick — ExpireStale).
	if n, err := h.segs.ExpireStale(context.Background(), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("e2e: expire stale leases (simulating sweeper after kill): %v", err)
	} else {
		t.Logf("re-queued %d in-flight segment(s) from the killed worker (sweeper ExpireStale)", n)
	}

	// ── Restart the worker (fresh ctx == fresh subprocess + fresh enroll token)
	// and assert it RESUMES and the job completes. ───────────────────────────
	tok2 := h.seedEnrollToken(t)
	w2 := h.startWorker(t, root, tok2)
	t.Cleanup(w2.stop)
	t.Log("worker #2 started (resume)")

	// ── Telemetry heartbeat observer (Bug 3a regression) ─────────────────────
	// The worker now starts a per-segment Telemetry loop that emits heartbeat
	// frames; the server's hub applies each to the worker row via
	// WorkerRepository.Heartbeat, setting current_job_id = jobID. Poll FindByJob
	// concurrently DURING the resume window so we capture that a heartbeat
	// genuinely landed while a segment was in flight (after job-done the per-
	// segment context is cancelled and no further heartbeats fire). Before the
	// fix, no heartbeat was ever emitted, so FindByJob would stay empty and this
	// flag never flips.
	hbObserved := make(chan struct{})
	hbStop := make(chan struct{})
	go func() {
		for {
			select {
			case <-hbStop:
				return
			default:
			}
			ws, err := h.workers.FindByJob(context.Background(), jobID)
			if err == nil {
				for _, wk := range ws {
					if wk.CurrentJobID == jobID {
						close(hbObserved)
						return
					}
				}
			}
			time.Sleep(100 * time.Millisecond)
		}
	}()
	t.Cleanup(func() { close(hbStop) })

	// ── Wait for the job to reach done. ──────────────────────────────────────
	waitFor(t, "job to reach done after resume", 180*time.Second, func() bool {
		st := h.jobStatus(t, jobID)
		if st == domain.JobFailed {
			job, _ := h.jobs.Get(context.Background(), jobID)
			t.Fatalf("job failed after resume: %q", job.ErrorText)
		}
		return st == domain.JobDone
	})
	t.Log("job reached done")

	// Assert a heartbeat updated the worker's current_job_id during processing.
	select {
	case <-hbObserved:
		t.Log("telemetry heartbeat observed: worker.current_job_id == jobID during processing (Bug 3a OK)")
	case <-time.After(15 * time.Second):
		// Heartbeats may have only fired during the (now-finished) segment windows.
		// As a fallback, accept a persisted current_job_id on the worker row — the
		// last heartbeat of the final segment leaves it set. An empty result here
		// means NO heartbeat ever landed → telemetry is still dark.
		ws, ferr := h.workers.FindByJob(context.Background(), jobID)
		if ferr != nil || len(ws) == 0 {
			t.Fatalf("no telemetry heartbeat ever set worker.current_job_id to %s "+
				"(FindByJob err=%v, rows=%d) — worker Telemetry not wired", jobID, ferr, len(ws))
		}
		t.Logf("telemetry heartbeat observed via persisted current_job_id on %d worker row(s) (Bug 3a OK)", len(ws))
	}

	// ── Assert NO lost / NO duplicate segments: exactly totalSegments DONE,
	// 0 pending, 0 leased, and exactly one output file per segment idx. ──────
	pending, leased, done := h.segCounts(t, jobID)
	if pending != 0 || leased != 0 || done != totalSegments {
		t.Fatalf("post-resume segment counts = (pending=%d, leased=%d, done=%d), want (0,0,%d)",
			pending, leased, done, totalSegments)
	}
	assertSegmentSetExactlyOnce(t, h, jobID, totalSegments)

	// ── Assert the full lifecycle was traversed (durable evidence): the job is
	// done, has an output prefix, source metadata + segment count persisted. ─
	finalJob, err := h.jobs.Get(context.Background(), jobID)
	if err != nil {
		t.Fatalf("e2e: get final job: %v", err)
	}
	if finalJob.Status != domain.JobDone {
		t.Fatalf("final job status = %q, want done", finalJob.Status)
	}
	if finalJob.OutputPrefix == "" {
		t.Fatal("final job has empty OutputPrefix — finalize/upload did not record it")
	}
	if finalJob.SegmentCount != totalSegments {
		t.Fatalf("final job SegmentCount = %d, want %d", finalJob.SegmentCount, totalSegments)
	}
	t.Logf("final job: status=done prefix=%q segments=%d codec=%q",
		finalJob.OutputPrefix, finalJob.SegmentCount, finalJob.SourceCodec)

	// ── Assert the recording MinIO writer received playlist.m3u8 AFTER the .ts
	// segments (playlist-last invariant) and the bucket was ensured. ─────────
	keys, segsAtPlay, bucketEnsured, prefix := h.writer.snapshot()
	if bucketEnsured < 1 {
		t.Errorf("EnsureBucket was never called; got %d", bucketEnsured)
	}
	assertPlaylistLast(t, keys)
	if segsAtPlay < 1 {
		t.Errorf("playlist.m3u8 was uploaded after %d .ts segments, want ≥1 (playlist-last invariant)", segsAtPlay)
	}
	if !strings.Contains(prefix, shikimoriID) {
		t.Errorf("upload prefix %q does not contain shikimori id %q", prefix, shikimoriID)
	}
	t.Logf("uploaded %d objects under prefix %q; playlist after %d .ts (playlist-last OK)",
		len(keys), prefix, segsAtPlay)

	// ── Remote-shell exec round-trip: open exec, run echo ok, observe output +
	// audit lines. ───────────────────────────────────────────────────────────
	assertExecRoundTrip(t, h)

	// ── Scrape /metrics and assert the expected upscale_* series are present. ─
	assertMetrics(t, h)
}

// assertSegmentSetExactlyOnce verifies every idx in [0,n) is DONE exactly once
// AND that exactly one upscaled output file exists per idx on disk (no lost, no
// duplicate). This is the teeth of the resume assertion.
func assertSegmentSetExactlyOnce(t *testing.T, h *serverHarness, jobID string, n int) {
	t.Helper()
	segs, err := h.segs.ListByJob(context.Background(), jobID)
	if err != nil {
		t.Fatalf("e2e: list segments: %v", err)
	}
	if len(segs) != n {
		t.Fatalf("segment rows = %d, want %d", len(segs), n)
	}
	seen := make(map[int]int, n)
	for _, s := range segs {
		seen[s.Idx]++
		if s.Status != domain.SegDone {
			t.Errorf("segment idx=%d status=%q, want done", s.Idx, s.Status)
		}
	}
	for i := 0; i < n; i++ {
		if seen[i] != 1 {
			t.Errorf("segment idx=%d appears %d time(s) in DB, want exactly 1 (lost/dup)", i, seen[i])
		}
	}

	// On-disk: exactly one upscaled output file per idx in {staging}/{jobID}/upscaled.
	upscaledDir := filepath.Join(h.stagingDir, jobID, "upscaled")
	entries, err := os.ReadDir(upscaledDir)
	if err != nil {
		t.Fatalf("e2e: read upscaled dir %s: %v", upscaledDir, err)
	}
	gotFiles := make(map[string]int)
	for _, e := range entries {
		if !e.IsDir() {
			gotFiles[e.Name()]++
		}
	}
	for i := 0; i < n; i++ {
		name := fmt.Sprintf("seg_%05d.mkv", i)
		if gotFiles[name] != 1 {
			t.Errorf("upscaled output %q present %d time(s) on disk, want exactly 1", name, gotFiles[name])
		}
	}
	if len(gotFiles) != n {
		extra := make([]string, 0)
		for name := range gotFiles {
			extra = append(extra, name)
		}
		sort.Strings(extra)
		t.Errorf("upscaled dir has %d files, want exactly %d: %v", len(gotFiles), n, extra)
	}
}

// assertPlaylistLast verifies the playlist.m3u8 key appears AFTER every .ts key.
func assertPlaylistLast(t *testing.T, keys []string) {
	t.Helper()
	playlistIdx := -1
	lastTsIdx := -1
	tsCount := 0
	for i, k := range keys {
		switch {
		case strings.HasSuffix(k, "playlist.m3u8"):
			playlistIdx = i
		case strings.HasSuffix(k, ".ts"):
			lastTsIdx = i
			tsCount++
		}
	}
	if tsCount < 1 {
		t.Fatalf("no .ts segments were uploaded; keys=%v", keys)
	}
	if playlistIdx < 0 {
		t.Fatalf("playlist.m3u8 was never uploaded; keys=%v", keys)
	}
	if playlistIdx < lastTsIdx {
		t.Fatalf("playlist.m3u8 uploaded at position %d BEFORE the last .ts at %d (playlist-last invariant violated); keys=%v",
			playlistIdx, lastTsIdx, keys)
	}
}

// assertExecRoundTrip exercises the remote-shell exec relay end-to-end against
// the REAL connected worker subprocess. It asserts:
//  1. The production ExecRelay.Open writes an EXEC_OPEN audit line and delivers a
//     real exec_open frame to the worker (via the admin shell WS path).
//  2. The REAL worker executes an allowlisted `echo ok` and streams genuine
//     exec_data ("ok") + exec_close(exit 0) frames back over the real WS.
//  3. CloseSession writes an EXEC_CLOSE audit line.
func assertExecRoundTrip(t *testing.T, h *serverHarness) {
	t.Helper()

	hdr := http.Header{}
	hdr.Set("X-Gateway-Internal", "1") // admin gate

	// Identify a worker that is connected to the HUB (not merely present in the
	// DB). With per-segment telemetry now ON, the KILLED worker #1 also has a
	// recent last_heartbeat_at, so WorkerRepository.ListConnected (a DB query)
	// returns BOTH workers — and the dead one may sort first. ExecRelay.Open
	// routes through the hub's in-memory WS connection map, so opening against the
	// dead worker yields 503 (worker not connected). We therefore probe each DB-
	// connected candidate by actually dialing the admin shell WS and keep the one
	// whose upgrade succeeds (101) — i.e. the worker the relay can genuinely reach
	// (Minor 2 ordering-flake fix). The successful dial IS Part A's admin
	// connection, so no redundant open.
	var workerID string
	var adminConn *gorillaws.Conn
	waitFor(t, "a hub-connected worker reachable via the admin shell WS", 60*time.Second, func() bool {
		ws, err := h.workers.ListConnected(context.Background(), time.Now().Add(-5*time.Minute))
		if err != nil || len(ws) == 0 {
			return false
		}
		for _, wk := range ws {
			adminWSURL := toWS(h.srv.URL) + "/api/upscale/workers/" + wk.WorkerID + "/shell?pty=true"
			conn, resp, derr := gorillaws.DefaultDialer.Dial(adminWSURL, hdr)
			if derr == nil {
				workerID = wk.WorkerID
				adminConn = conn
				return true
			}
			// 503 = that worker isn't in the hub (e.g. the killed worker #1); try
			// the next candidate. Any other status is also non-fatal here — we
			// retry until a live worker accepts the upgrade or the deadline passes.
			if resp != nil {
				_ = resp.Body.Close()
			}
		}
		return false
	})
	t.Logf("exec target worker (hub-reachable): %s", workerID)

	// ── Part A: production relay path — audit line + real exec_open frame. The
	// admin shell WebSocket upgrade above already drove the REAL
	// handler.ExecShellHandler → ExecRelay.Open, so the full admin surface is
	// exercised and the EXEC_OPEN audit line is in flight. ────────────────────
	// Give the relay a moment to deliver exec_open + write the audit line.
	waitFor(t, "EXEC_OPEN audit line", 10*time.Second, func() bool {
		return strings.Contains(h.auditBuf.String(), "EXEC_OPEN")
	})
	auditOpen := h.auditBuf.String()
	if !strings.Contains(auditOpen, "EXEC_OPEN") || !strings.Contains(auditOpen, workerID) {
		t.Fatalf("audit log missing EXEC_OPEN for worker %s; got:\n%s", workerID, auditOpen)
	}
	// Close the admin session → EXEC_CLOSE audit line.
	_ = adminConn.WriteMessage(gorillaws.TextMessage, mustFrame(t, "exec_close", controlplane.ExecPayload{}))
	_ = adminConn.Close()
	waitFor(t, "EXEC_CLOSE audit line", 10*time.Second, func() bool {
		return strings.Contains(h.auditBuf.String(), "EXEC_CLOSE")
	})
	t.Log("relay audit round-trip OK (EXEC_OPEN + EXEC_CLOSE)")

	// ── Part B: capture the REAL worker executing `echo ok` end-to-end. The
	// worker only runs commands carried in exec_open.Data (allowlist mode); the
	// production relay's Open never sets Data (the worker build does not accept
	// inbound exec_data stdin yet). So we swap the hub's ExecRouter to a recorder
	// and send a Data-carrying exec_open through the REAL hub, capturing the
	// worker's genuine exec_data / exec_close frames. We restore the relay after.
	rec := &recordingExecRouter{}
	h.hub.SetExecRouter(rec)
	defer h.hub.SetExecRouter(h.relay)

	sessionID := "e2e-echo-" + uuid.NewString()
	openFrame, err := controlplane.NewFrame("exec_open", 0, controlplane.ExecPayload{
		SessionID: sessionID,
		Data:      []byte("echo ok"),
		Pty:       false, // allowlist mode: echo is allowlisted
	})
	if err != nil {
		t.Fatalf("e2e: build exec_open frame: %v", err)
	}
	if err := h.hub.Send(workerID, openFrame); err != nil {
		t.Fatalf("e2e: send exec_open to worker %s: %v", workerID, err)
	}

	// Collect frames until we see exec_close for our session.
	var output bytes.Buffer
	var sawClose bool
	var exitCode = -999
	waitFor(t, "worker exec_close for echo session", 30*time.Second, func() bool {
		for _, f := range rec.snapshot() {
			var p controlplane.ExecPayload
			if err := f.Decode(&p); err != nil || p.SessionID != sessionID {
				continue
			}
			switch f.Type {
			case "exec_data":
				output.Write(p.Data)
			case "exec_close":
				sawClose = true
				if p.ExitCode != nil {
					exitCode = *p.ExitCode
				}
			}
		}
		return sawClose
	})

	if !strings.Contains(output.String(), "ok") {
		t.Fatalf("worker exec output = %q, want it to contain %q", output.String(), "ok")
	}
	if exitCode != 0 {
		t.Fatalf("worker exec exit code = %d, want 0", exitCode)
	}
	t.Logf("worker exec round-trip OK: output=%q exit=%d", strings.TrimSpace(output.String()), exitCode)
}

// assertMetrics scrapes /metrics and asserts the expected upscale_* series are
// present, INCLUDING the worker-telemetry + enroll series that Bug 3 turned on.
//
// Prometheus subtlety that drives WHICH series we assert: a *Vec metric
// (CounterVec/GaugeVec/HistogramVec) emits NO text-format lines until a child
// with concrete label values is created — promhttp does not even emit the
// # HELP/# TYPE header for an unobserved vec. So the LABEL-LESS upscale_*
// metrics (plain Counter/Gauge) are present after package init, while the
// LABELLED ones only appear once their labels are observed.
//
// Bug 3 wired BOTH halves of that observability into the live flow:
//   - 3a: the worker now starts agent.Telemetry per segment → it emits `metrics`
//     frames → hub.RecordWorkerTelemetry sets the labelled upscale_decode_fps /
//     upscale_inference_fps / upscale_encode_fps / upscale_worker_* gauges, so
//     those vecs now appear in the scrape (proving a real `metrics` frame was
//     received end-to-end).
//   - 3b: EnrollTx now increments upscale_enroll_total{result="ok"} on every
//     successful production enroll (≥2 happened in this run: worker #1 + #2).
//
// We therefore now assert these series ARE present. Because the worker emits at
// the configured cadence and the scrape races the last metrics frame, we poll
// the endpoint briefly rather than scrape once.
func assertMetrics(t *testing.T, h *serverHarness) {
	t.Helper()

	scrape := func() string {
		resp, err := http.Get(h.srv.URL + "/metrics")
		if err != nil {
			t.Fatalf("e2e: scrape /metrics: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("e2e: /metrics status %d", resp.StatusCode)
		}
		raw, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("e2e: read /metrics body: %v", err)
		}
		return string(raw)
	}

	body := scrape()

	// (a) upscaler HTTP collector — populated by every request through the
	// router middleware (we POSTed a job, scraped /metrics, etc.).
	for _, s := range []string{"http_requests_total", "http_request_duration_seconds"} {
		if !strings.Contains(body, s) {
			t.Errorf("/metrics missing HTTP collector series %q", s)
		}
	}

	// (b) label-less upscale_* series — registered at package init and emitted
	// even at zero, so they MUST be present in any upscaler /metrics scrape.
	wantUpscaleSeries := []string{
		"upscale_lease_expired_total",
		"upscale_job_progress_ratio",
		"upscale_job_eta_seconds",
	}
	for _, s := range wantUpscaleSeries {
		if !strings.Contains(body, s) {
			t.Errorf("/metrics missing expected upscale_* series %q", s)
		}
	}

	// (c) Bug 3b: the PRODUCTION enroll path now counts enrolls. Two workers
	// enrolled in this run, so upscale_enroll_total{result="ok"} must be ≥1.
	if !containsEnrollOK(body) {
		t.Errorf(`/metrics missing upscale_enroll_total{result="ok"} ≥1 — `+
			`EnrollTx is not incrementing the enroll counter (Bug 3b).`+"\nenroll lines:\n%s",
			grepLines(body, "upscale_enroll_total"))
	}

	// (d) Bug 3a: the worker now emits `metrics` frames, so the labelled
	// worker-telemetry gauges become non-empty once a metrics frame is applied
	// server-side. These vecs would be ABSENT entirely if telemetry were still
	// dark. Poll briefly so the assertion does not race the worker's cadence.
	telemetrySeries := []string{
		"upscale_decode_fps",
		"upscale_inference_fps",
		"upscale_encode_fps",
		"upscale_worker_gpu_util",
		"upscale_worker_vram_used_bytes",
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		missing := false
		for _, s := range telemetrySeries {
			if !strings.Contains(body, s) {
				missing = true
				break
			}
		}
		if !missing {
			break
		}
		if time.Now().After(deadline) {
			for _, s := range telemetrySeries {
				if !strings.Contains(body, s) {
					t.Errorf("/metrics missing worker-telemetry series %q — no `metrics` "+
						"frame was received from the worker (Bug 3a: Telemetry not wired)", s)
				}
			}
			break
		}
		time.Sleep(300 * time.Millisecond)
		body = scrape()
	}

	// (e) sanity: at least one upscale_* family is present (guards against a
	// total registration regression).
	if !strings.Contains(body, "upscale_") {
		t.Error("/metrics contains no upscale_* series at all — registration regression")
	}
	t.Log("/metrics scrape OK — http_* collector + label-less upscale_* + " +
		"worker-telemetry fps gauges + upscale_enroll_total{ok} present")
}

// containsEnrollOK reports whether the scrape has a non-zero
// upscale_enroll_total sample with result="ok". It scans for a line like:
//
//	upscale_enroll_total{result="ok"} 2
//
// and checks the trailing value is > 0.
func containsEnrollOK(body string) bool {
	for _, ln := range strings.Split(body, "\n") {
		if !strings.HasPrefix(ln, "upscale_enroll_total") {
			continue
		}
		if !strings.Contains(ln, `result="ok"`) {
			continue
		}
		fields := strings.Fields(ln)
		if len(fields) < 2 {
			continue
		}
		if v, err := strconv.ParseFloat(fields[len(fields)-1], 64); err == nil && v > 0 {
			return true
		}
	}
	return false
}

// grepLines returns the lines of body that contain sub, for error diagnostics.
func grepLines(body, sub string) string {
	var b strings.Builder
	for _, ln := range strings.Split(body, "\n") {
		if strings.Contains(ln, sub) {
			b.WriteString(ln)
			b.WriteString("\n")
		}
	}
	return b.String()
}

// ── small helpers ────────────────────────────────────────────────────────────

func toWS(httpURL string) string {
	u, _ := url.Parse(httpURL)
	if u.Scheme == "https" {
		u.Scheme = "wss"
	} else {
		u.Scheme = "ws"
	}
	return u.String()
}

func mustFrame(t *testing.T, typ string, payload controlplane.ExecPayload) []byte {
	t.Helper()
	f, err := controlplane.NewFrame(typ, 0, payload)
	if err != nil {
		t.Fatalf("e2e: build %s frame: %v", typ, err)
	}
	raw, err := json.Marshal(f)
	if err != nil {
		t.Fatalf("e2e: marshal %s frame: %v", typ, err)
	}
	return raw
}

// ════════════════════════════════════════════════════════════════════════════
// T30: pull-on-demand model provisioning.
//
// These tests prove the dynamic-models chain end-to-end through the REAL server
// stack and the REAL worker subprocess. They are the companions to the mock
// capstone above: the mock job proves full-chain COMPLETION with no fetch; these
// prove the server can PROVISION a model the worker did not boot with.
//
// GPU-free design (option (b) per the T30 brief):
// A pulled model installs as a realesrgan model (Manager.Install always registers
// a realesrgan backend), which requires realesrgan-ncnn-vulkan + a GPU — absent
// in this CI host, so the post-install Process() step cannot actually run a
// segment to completion here. There is NO clean GPU-free seam in the production
// install path (Install does not special-case any name), and faking one would
// weaken the very code under test. So we prove the pull-on-demand chain THROUGH
// fetch → capability-verify → install → per-job selection, and rely on the mock
// capstone above for full-chain completion. The teeth of the proof:
//   1. The worker's GET hit the capability-gated /worker/models/{name} endpoint
//      and the REAL serve handler returned 200 (the name-bound HMAC handle the
//      hub minted in the lease grant verified) — i.e. the fetch is genuinely
//      authenticated, not stubbed.
//   2. The worker logged a successful install of that exact model name (the
//      served bytes checksum-matched and Manager.Install extracted+registered
//      them) — observed via the worker subprocess's stderr.
//   3. The worker stays alive throughout (the post-install GPU-free Process()
//      failure is a clean per-segment fail, re-leased, NOT a crash).
// ════════════════════════════════════════════════════════════════════════════

// TestPullOnDemandE2E_FetchInstallSelection proves the server can provision a
// model the worker did not boot with: an admin uploads it, a job names it, and a
// mock-only worker fetches + installs it on first lease.
func TestPullOnDemandE2E_FetchInstallSelection(t *testing.T) {
	requireEnv(t)

	db := openIntegrationDB(t)
	h := newServerHarness(t, db)
	root := repoRoot(t)

	const (
		shikimoriID = "57466"
		episode     = 2
		infohash    = "f30f30f30f30f30f30f30f30f30f30f30f30f30f"
		modelName   = "e2emodel" // NOT mock, NOT preinstalled → must be pulled
		modelVer    = "1"
	)
	h.stageSource(t, infohash)

	// ── Register the model SERVER-SIDE via the real T26 admin upload endpoint. ─
	_, checksum := h.uploadModel(t, modelName, modelVer)
	t.Logf("uploaded model %q v%s (checksum %s…)", modelName, modelVer, checksum[:12])

	// Sanity: the model is now listed by the admin registry (real GET path).
	if got := h.modelGetLatestName(t, modelName); got != modelName {
		t.Fatalf("model registry GetLatest(%q) returned %q", modelName, got)
	}

	// ── Submit a job NAMING that model. The leaser threads job.Model into the
	// lease grant and the hub mints a name-bound model-fetch capability handle. ─
	jobID := h.createJob(t, shikimoriID, episode, infohash, modelName)
	t.Logf("created pull-on-demand job %s (model=%s)", jobID, modelName)

	// Wait until the orchestrator has segmented the fixture into ≥1 segment so a
	// lease is actually grantable (the worker can only fetch once it is granted a
	// segment that names the model).
	waitFor(t, "pull-on-demand job to reach upscaling with ≥1 segment", 90*time.Second, func() bool {
		st := h.jobStatus(t, jobID)
		if st == domain.JobFailed {
			job, _ := h.jobs.Get(context.Background(), jobID)
			t.Fatalf("job failed during segmenting: %q", job.ErrorText)
		}
		p, l, d := h.segCounts(t, jobID)
		return st == domain.JobUpscaling && (p+l+d) >= 1
	})

	// ── Boot a worker with ONLY the built-in mock (no PREINSTALLED_MODELS). It
	// must FETCH e2emodel from the server on first lease. ────────────────────
	tok := h.seedEnrollToken(t)
	w := h.startWorker(t, root, tok)
	t.Cleanup(w.stop)

	// ── Teeth #1: the worker's pull-on-demand GET reached the capability-gated
	// serve endpoint and the REAL handler returned 200 (handle verified). ─────
	waitFor(t, "worker GET /worker/models/"+modelName+" → 200 (capability verified)", 90*time.Second, func() bool {
		return h.modelGets.countFor(modelName, http.StatusOK) >= 1
	})
	t.Logf("model serve endpoint hit: %d×200 for %q", h.modelGets.countFor(modelName, http.StatusOK), modelName)

	// ── Teeth #2: the worker installed that exact model (served bytes checksum-
	// matched + extracted + registered). Observed via the worker's stderr marker
	// from fetchAndInstallModel's success path. ───────────────────────────────
	installLine := fmt.Sprintf("model %q fetched and installed", modelName)
	waitFor(t, "worker stderr to report a successful install of "+modelName, 60*time.Second, func() bool {
		return strings.Contains(w.stderr.String(), installLine)
	})
	t.Logf("worker reported install: %q", installLine)

	// ── Teeth #3: the worker is still alive after install (the post-install
	// GPU-free realesrgan Process() failure is a clean per-segment fail, not a
	// crash). A live worker keeps retrying the lease, so the serve endpoint sees
	// repeated 200s; we assert the process has NOT exited. ────────────────────
	if exited, code := w.exited(); exited {
		t.Fatalf("worker exited (code=%d) after pull-on-demand install — must stay alive; stderr:\n%s",
			code, w.stderr.String())
	}
	t.Log("worker still alive after pull-on-demand fetch+install (no crash)")

	// Defensive: no 401/403 was ever returned for this model (the capability is
	// genuinely verified, not bypassed). A single non-200, non-404 status here
	// would mean the handle minted by the hub did not verify.
	for _, hit := range h.modelGets.snapshot() {
		if hit.name == modelName && hit.status != http.StatusOK {
			t.Errorf("unexpected status %d for model %q serve (capability handle should verify → 200)", hit.status, modelName)
		}
	}
}

// TestPullOnDemandE2E_UnknownModel404 proves the negative path: a job naming a
// model that was NEVER registered on the server → the worker's fetch gets 404 →
// the segment cleanly fails (re-leased) and the worker stays alive (no crash).
func TestPullOnDemandE2E_UnknownModel404(t *testing.T) {
	requireEnv(t)

	db := openIntegrationDB(t)
	h := newServerHarness(t, db)
	root := repoRoot(t)

	const (
		shikimoriID = "57466"
		episode     = 3
		infohash    = "404404404404404404404404404404404404404a"
		modelName   = "ghostmodel" // never uploaded → server has no row → 404
	)
	h.stageSource(t, infohash)

	// NOTE: deliberately do NOT upload the model. The hub still mints a model
	// handle (model != "mock"), the worker still attempts the fetch, but the
	// serve handler's GetLatest finds no row → 404.

	jobID := h.createJob(t, shikimoriID, episode, infohash, modelName)
	t.Logf("created unknown-model job %s (model=%s, NOT registered)", jobID, modelName)

	waitFor(t, "unknown-model job to reach upscaling with ≥1 segment", 90*time.Second, func() bool {
		st := h.jobStatus(t, jobID)
		if st == domain.JobFailed {
			job, _ := h.jobs.Get(context.Background(), jobID)
			t.Fatalf("job failed during segmenting: %q", job.ErrorText)
		}
		p, l, d := h.segCounts(t, jobID)
		return st == domain.JobUpscaling && (p+l+d) >= 1
	})

	tok := h.seedEnrollToken(t)
	w := h.startWorker(t, root, tok)
	t.Cleanup(w.stop)

	// ── Teeth #1: the worker's fetch got a 404 from the REAL serve handler
	// (capability verified, but GetLatest found no row). ──────────────────────
	waitFor(t, "worker GET /worker/models/"+modelName+" → 404 (no such model)", 90*time.Second, func() bool {
		return h.modelGets.countFor(modelName, http.StatusNotFound) >= 1
	})
	t.Logf("model serve endpoint hit: %d×404 for unregistered %q", h.modelGets.countFor(modelName, http.StatusNotFound), modelName)

	// ── Teeth #2: the segment cleanly fails + is re-leased — NOT marked done,
	// and NOT left permanently leased. After a 404, processSegment returns a
	// clean error so the segment goes back to pending/leased for the next lease;
	// it must NEVER reach done (nothing was upscaled). We observe ≥2 fetch 404s
	// (the worker retries the re-leased segment), proving re-lease, and assert no
	// segment for this job is done. ───────────────────────────────────────────
	waitFor(t, "worker to re-attempt the fetch (re-lease after clean-fail)", 60*time.Second, func() bool {
		return h.modelGets.countFor(modelName, http.StatusNotFound) >= 2
	})
	_, _, done := h.segCounts(t, jobID)
	if done != 0 {
		t.Fatalf("unknown-model job has %d done segment(s); expected 0 (nothing can be upscaled without the model)", done)
	}
	t.Logf("segment cleanly re-leased after 404 (≥2 fetch attempts, 0 done)")

	// ── Teeth #3: the worker stays alive throughout (a fetch 404 must be a clean
	// per-segment fail, never a worker crash). ────────────────────────────────
	if exited, code := w.exited(); exited {
		t.Fatalf("worker exited (code=%d) after a 404 model fetch — must stay alive; stderr:\n%s",
			code, w.stderr.String())
	}
	t.Log("worker still alive after repeated 404 model fetches (no crash)")
}

// modelGetLatestName resolves the latest model row for name via the REAL repo
// and returns its Name (or "" if absent). Used as a registry sanity check after
// the admin upload.
func (h *serverHarness) modelGetLatestName(t *testing.T, name string) string {
	t.Helper()
	m, err := h.models.GetLatest(context.Background(), name)
	if err != nil {
		t.Fatalf("e2e: GetLatest(%q): %v", name, err)
	}
	if m == nil {
		return ""
	}
	return m.Name
}
