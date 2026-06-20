# Unified Playback Probe Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the two divergent provider probes (scraper in-process `probe.go` + scheduler playability canary) with ONE engine in the analytics service that resolves through catalog's signed scraper path and validates real playback through the HLS proxy with ffprobe.

**Architecture:** A daily scheduler cron POSTs `/internal/probe/run` on analytics. The analytics `ProbeEngine` picks 4 anime/run (anchor+featured+spotlight_random+random), resolves each via catalog `/api/anime/{uuid}/scraper/...` (signed), validates by downloading a few segments through `/api/streaming/hls-proxy` and running ffprobe, rolls up a per-provider verdict, and emits Prometheus + ClickHouse. The scraper `probe.go` and the scheduler canary job are deleted.

**Tech Stack:** Go 1.25, chi router, go-chi, ClickHouse, Prometheus (`promauto`), `libs/streamprobe`, `libs/logger`, `libs/httputil`, ffprobe (ffmpeg), Grafana JSON dashboards.

## Global Constraints

- **NO scraper provider/extraction edits.** The ONLY authorized `services/scraper/` change is deleting `internal/health/probe.go` + its boot wiring + the `provider_health_up`/`provider_probe_last_tick` metric. Do not touch `services/scraper/internal/providers/` or `embeds/`.
- Anchor anime = Frieren, UUID `f0b40660-6627-4a59-8dcf-7ec8596b3623`.
- Cadence: daily, default cron `0 3 * * *`.
- Validation path MUST be catalog-signed resolve → streaming HLS proxy (never direct CDN fetch).
- Effort metrics per `.planning/CONVENTIONS.md` (no time units).
- Commits: include the three co-authors from `MEMORY.md`. Push after each commit (realtime backup). Use `git commit <pathspec>` (never `git add -A`) — the shared tree is multi-agent.
- New i18n? None expected. If any user-facing string is added, all three locales (en/ru/ja).
- Tests: fakes only, no live APIs.

---

## File Structure

**New (analytics engine) — `services/analytics/internal/probe/`:**
- `types.go` — `ResolvedStream`, `Verdict`, `ProviderVerdict`, `AnimeSlot`, status enum.
- `resolver.go` — `Resolver` (catalog signed scraper client). Independent component.
- `validator.go` — `Validator` (proxy fetch + ffprobe).
- `ffprobe.go` — thin ffprobe exec wrapper.
- `animeset.go` — `AnimeSetResolver` (anchor + spotlight + random).
- `scorer.go` — `Scorer` (per-provider rollup).
- `reporter.go` — `Reporter` (Prometheus + ClickHouse).
- `engine.go` — `Engine` (orchestrates, `RunOnce`).
- `*_test.go` co-located per file.

**New metrics:** `libs/metrics/probe.go`.

**Modified:**
- `services/analytics/internal/handler/probe.go` (+ test) — `/internal/probe/run`.
- `services/analytics/internal/transport/router.go` — register route.
- `services/analytics/cmd/analytics-api/main.go` — DI.
- `services/analytics/internal/config/config.go` — `CatalogURL`, `StreamingURL`, `ProbeAnchorUUID`, `FFprobePath`.
- `services/analytics/internal/repo/clickhouse_schema.go` + `clickhouse_store.go` — probe table + insert.
- `services/analytics/Dockerfile` — add ffmpeg.
- `services/scheduler/internal/jobs/probe_trigger.go` (+ test, replaces `scraper_playability_canary.go`).
- `services/scheduler/internal/service/job.go`, `cmd/scheduler-api/main.go`, `internal/config/config.go` — swap canary→probe-trigger.
- `services/scraper/internal/health/probe.go` (DELETE) + `cmd/scraper-api/main.go` (remove wiring).
- `libs/metrics/provider.go` — remove `ProviderHealthUp`, `ProviderProbeLastTick`.
- `docker/grafana/dashboards/playback-health.json` — table + repoints.
- `services/catalog/internal/service/scraperprovider/seed.go` + a guarded migration — animefever text.
- `CLAUDE.md` — animefever text mirror.

---

## Phase A — Analytics probe engine

### Task A1: Probe domain types + new reason codes

**Files:**
- Create: `services/analytics/internal/probe/types.go`
- Test: `services/analytics/internal/probe/types_test.go`
- Modify: `libs/streamprobe/reason.go` (add 2 reasons)

**Interfaces:**
- Produces: `ResolvedStream`, `Verdict`, `ProviderVerdict`, `Status`, `AnimeSlot`, stage constants; `streamprobe.ReasonDecodeFailed`, `streamprobe.ReasonInvalidVideo`.

- [ ] **Step 1: Add reason codes** to `libs/streamprobe/reason.go` const block and `AllReasons()`:

```go
	ReasonDecodeFailed Reason = "decode_failed"
	ReasonInvalidVideo Reason = "invalid_video"
```
Append both to the slice returned by `AllReasons()`.

- [ ] **Step 2: Write the failing test** `types_test.go`:

```go
package probe

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

func TestProviderVerdict_Status(t *testing.T) {
	if StatusUp.Gauge() != 1.0 || StatusDegraded.Gauge() != 0.5 || StatusDown.Gauge() != 0.0 {
		t.Fatalf("gauge mapping wrong")
	}
}

func TestVerdict_Playable(t *testing.T) {
	v := Verdict{Reason: streamprobe.ReasonPlayable}
	if !v.Playable() {
		t.Fatalf("expected playable")
	}
	if (Verdict{Reason: streamprobe.ReasonStatus403}).Playable() {
		t.Fatalf("403 must not be playable")
	}
}
```

- [ ] **Step 3: Run test, expect FAIL** (`go test ./internal/probe/ -run TestProviderVerdict_Status`): undefined `StatusUp`.

- [ ] **Step 4: Implement `types.go`:**

```go
package probe

import "github.com/ILITA-hub/animeenigma/libs/streamprobe"

// Status is the per-provider rollup verdict shown on the dashboard.
type Status string

const (
	StatusUp       Status = "up"
	StatusDegraded Status = "degraded"
	StatusDown     Status = "down"
)

func (s Status) Gauge() float64 {
	switch s {
	case StatusUp:
		return 1.0
	case StatusDegraded:
		return 0.5
	default:
		return 0.0
	}
}

// Stage is the furthest pipeline step reached.
type Stage string

const (
	StageSearch   Stage = "search"
	StageEpisodes Stage = "episodes"
	StageServers  Stage = "servers"
	StageStream   Stage = "stream"
	StagePlayback Stage = "playback"
)

// AnimeSlot labels which of the 4 per-run anime a probe targeted.
type AnimeSlot string

const (
	SlotAnchor          AnimeSlot = "anchor"
	SlotFeatured        AnimeSlot = "featured"
	SlotSpotlightRandom AnimeSlot = "spotlight_random"
	SlotRandom          AnimeSlot = "random"
)

// ResolvedStream is one server's catalog-signed stream, ready to validate
// through the HLS proxy. Produced by Resolver.
type ResolvedStream struct {
	Provider  string
	AnimeUUID string
	Slot      AnimeSlot
	Server    string // server id/name, e.g. "...type=hd-1..."
	MasterURL string
	Exp       string
	Sig       string
	Referer   string
	Stage     Stage // furthest stage reached when this was produced (StageStream on success)
}

// Verdict is the outcome of validating one ResolvedStream.
type Verdict struct {
	Provider  string
	AnimeUUID string
	Slot      AnimeSlot
	Server    string
	Stage     Stage
	Reason    streamprobe.Reason
}

func (v Verdict) Playable() bool { return v.Reason == streamprobe.ReasonPlayable }

// ProviderVerdict is the per-provider rollup across its anime/servers.
type ProviderVerdict struct {
	Provider string
	Status   Status
	Reason   string // dominant failure classification with locus, "" when up
}
```

- [ ] **Step 5: Run tests, expect PASS.** Run: `cd services/analytics && go test ./internal/probe/ -run 'Verdict|Status' -v`

- [ ] **Step 6: Commit** (`git commit libs/streamprobe/reason.go services/analytics/internal/probe/types.go services/analytics/internal/probe/types_test.go -m "feat(analytics): probe domain types + decode reason codes" + co-authors`); push.

---

### Task A2: Resolver — independent catalog signed-scraper client

**Files:**
- Create: `services/analytics/internal/probe/resolver.go`, `resolver_test.go`

**Interfaces:**
- Consumes: `ResolvedStream`, `Stage` (Task A1).
- Produces: `type Resolver interface { Resolve(ctx, animeUUID string, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) }`; `NewHTTPResolver(catalogBaseURL string, hc *http.Client) *HTTPResolver`.

Resolver walks catalog `GET {catalog}/api/anime/{uuid}/scraper/episodes?prefer=<p>` → `servers?episode=<id>&prefer=<p>` → `stream?episode=<id>&server=<srv>&category=sub&prefer=<p>` (episode 1). Returns one `ResolvedStream` per server, the furthest `Stage` reached, and an error only on a hard pre-stream failure.

Catalog envelopes (confirmed live): `{"success":true,"data":{...}}`.
- episodes: `data.episodes[] = {id, number, ...}`
- servers: `data.servers[] = {id, name, type}`
- stream: `data.stream = {headers:{Referer}, sources:[{url, exp, sig, type, quality}]}`

- [ ] **Step 1: Write the failing test** `resolver_test.go` — spin an `httptest.Server` returning the three envelopes for one server; assert one `ResolvedStream` with `MasterURL/Exp/Sig/Referer` populated and `Stage==StageStream`:

```go
package probe

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPResolver_HappyPath(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/scraper/episodes"):
			w.Write([]byte(`{"success":true,"data":{"episodes":[{"id":"ep1","number":1}]}}`))
		case strings.Contains(r.URL.Path, "/scraper/servers"):
			w.Write([]byte(`{"success":true,"data":{"servers":[{"id":"srvA","name":"HD-1","type":"sub"}]}}`))
		case strings.Contains(r.URL.Path, "/scraper/stream"):
			w.Write([]byte(`{"success":true,"data":{"stream":{"headers":{"Referer":"https://ref/"},"sources":[{"url":"https://cdn/m.m3u8","exp":"99","sig":"ab","type":"hls"}]}}}`))
		}
	}))
	defer srv.Close()

	r := NewHTTPResolver(srv.URL, srv.Client())
	streams, stage, err := r.Resolve(context.Background(), "uuid1", SlotAnchor, "gogoanime")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if stage != StageStream || len(streams) != 1 {
		t.Fatalf("stage=%v n=%d", stage, len(streams))
	}
	s := streams[0]
	if s.MasterURL != "https://cdn/m.m3u8" || s.Exp != "99" || s.Sig != "ab" || s.Referer != "https://ref/" {
		t.Fatalf("bad resolved stream: %+v", s)
	}
}

func TestHTTPResolver_NoEpisodes(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"episodes":[]}}`))
	}))
	defer srv.Close()
	r := NewHTTPResolver(srv.URL, srv.Client())
	_, stage, err := r.Resolve(context.Background(), "uuid1", SlotAnchor, "gogoanime")
	if err == nil || stage != StageEpisodes {
		t.Fatalf("want episodes-stage error, got stage=%v err=%v", stage, err)
	}
}
```

- [ ] **Step 2: Run, expect FAIL** (undefined `NewHTTPResolver`).

- [ ] **Step 3: Implement `resolver.go`:**

```go
package probe

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Resolver interface {
	Resolve(ctx context.Context, animeUUID string, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error)
}

type HTTPResolver struct {
	base string
	hc   *http.Client
}

func NewHTTPResolver(catalogBaseURL string, hc *http.Client) *HTTPResolver {
	if hc == nil {
		hc = &http.Client{Timeout: 15 * time.Second}
	}
	return &HTTPResolver{base: strings.TrimRight(catalogBaseURL, "/"), hc: hc}
}

type envelope struct {
	Data struct {
		Episodes []struct {
			ID     string `json:"id"`
			Number int    `json:"number"`
		} `json:"episodes"`
		Servers []struct {
			ID   string `json:"id"`
			Name string `json:"name"`
			Type string `json:"type"`
		} `json:"servers"`
		Stream struct {
			Headers map[string]string `json:"headers"`
			Sources []struct {
				URL  string `json:"url"`
				Exp  string `json:"exp"`
				Sig  string `json:"sig"`
				Type string `json:"type"`
			} `json:"sources"`
		} `json:"stream"`
	} `json:"data"`
}

func (r *HTTPResolver) get(ctx context.Context, path string, q url.Values) (*envelope, error) {
	u := r.base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := r.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s -> %d", path, resp.StatusCode)
	}
	var e envelope
	if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *HTTPResolver) Resolve(ctx context.Context, animeUUID string, slot AnimeSlot, provider string) ([]ResolvedStream, Stage, error) {
	base := "/api/anime/" + animeUUID + "/scraper"
	eps, err := r.get(ctx, base+"/episodes", url.Values{"prefer": {provider}})
	if err != nil {
		return nil, StageEpisodes, err
	}
	if len(eps.Data.Episodes) == 0 {
		return nil, StageEpisodes, fmt.Errorf("no episodes")
	}
	epID := eps.Data.Episodes[0].ID

	sv, err := r.get(ctx, base+"/servers", url.Values{"episode": {epID}, "prefer": {provider}})
	if err != nil {
		return nil, StageServers, err
	}
	if len(sv.Data.Servers) == 0 {
		return nil, StageServers, fmt.Errorf("no servers")
	}

	var out []ResolvedStream
	for _, s := range sv.Data.Servers {
		st, err := r.get(ctx, base+"/stream", url.Values{
			"episode": {epID}, "server": {s.ID}, "category": {"sub"}, "prefer": {provider},
		})
		if err != nil || len(st.Data.Stream.Sources) == 0 {
			continue
		}
		src := st.Data.Stream.Sources[0]
		out = append(out, ResolvedStream{
			Provider: provider, AnimeUUID: animeUUID, Slot: slot, Server: s.ID,
			MasterURL: src.URL, Exp: src.Exp, Sig: src.Sig,
			Referer: st.Data.Stream.Headers["Referer"], Stage: StageStream,
		})
	}
	if len(out) == 0 {
		return nil, StageStream, fmt.Errorf("no resolvable stream")
	}
	return out, StageStream, nil
}
```

- [ ] **Step 4: Run, expect PASS.** `cd services/analytics && go test ./internal/probe/ -run Resolver -v`
- [ ] **Step 5: Commit** (`resolver.go resolver_test.go`); push.

---

### Task A3: ffprobe wrapper + Validator (proxy fetch + decode)

**Files:**
- Create: `services/analytics/internal/probe/ffprobe.go`, `validator.go`, `validator_test.go`

**Interfaces:**
- Consumes: `ResolvedStream`, `Verdict`, `streamprobe` reasons.
- Produces: `type Validator interface { Validate(ctx, rs ResolvedStream) Verdict }`; `NewHTTPValidator(streamingBaseURL string, hc *http.Client, vp VideoProber) *HTTPValidator`; `type VideoProber interface { Probe(ctx, mediaBytes []byte) error }`; `NewFFprobe(path string) *FFprobe`.

The Validator builds the proxy URL `{{streaming}}/api/streaming/hls-proxy?url=<MasterURL>&exp=<Exp>&sig=<Sig>&referer=<Referer>`, GETs the master (classify 403/empty/unreachable), parses for the first variant line (relative proxied URL is returned by the proxy already rewritten — fetch it), then fetches the first segment line and feeds its bytes to `VideoProber`. Caps: ≤8 MiB, ≤10s.

- [ ] **Step 1: Write failing test** `validator_test.go` — fake streaming server returns a rewritten master → variant → a 200-byte "segment"; fake `VideoProber` returns nil → expect `ReasonPlayable`. A 403 master → `ReasonStatus403`. Fake prober returning error → `ReasonDecodeFailed`.

```go
package probe

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type fakeProber struct{ err error }

func (f fakeProber) Probe(_ context.Context, _ []byte) error { return f.err }

func newStreamingStub(t *testing.T, masterStatus int) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		url := r.URL.Query().Get("url")
		switch {
		case masterStatus != 200 && strings.Contains(url, "master"):
			w.WriteHeader(masterStatus)
			w.Write([]byte("blocked"))
		case strings.Contains(url, "master"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=1\n/api/streaming/hls-proxy?url=variant\n"))
		case strings.Contains(url, "variant"):
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXTINF:4,\n/api/streaming/hls-proxy?url=seg0\n"))
		default:
			w.Write([]byte("BINARYSEGMENTDATA"))
		}
	}))
}

func TestValidator_Playable(t *testing.T) {
	s := newStreamingStub(t, 200)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://cdn/master.m3u8", Provider: "p"})
	if got.Reason != streamprobe.ReasonPlayable {
		t.Fatalf("want playable, got %s", got.Reason)
	}
}

func TestValidator_403(t *testing.T) {
	s := newStreamingStub(t, 403)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://cdn/master.m3u8"})
	if got.Reason != streamprobe.ReasonStatus403 {
		t.Fatalf("want status_403, got %s", got.Reason)
	}
}

func TestValidator_DecodeFailed(t *testing.T) {
	s := newStreamingStub(t, 200)
	defer s.Close()
	v := NewHTTPValidator(s.URL, s.Client(), fakeProber{err: errors.New("no video")})
	got := v.Validate(context.Background(), ResolvedStream{MasterURL: "https://cdn/master.m3u8"})
	if got.Reason != streamprobe.ReasonDecodeFailed {
		t.Fatalf("want decode_failed, got %s", got.Reason)
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement `ffprobe.go`:**

```go
package probe

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
)

// VideoProber validates that media bytes contain a decodable video stream.
type VideoProber interface {
	Probe(ctx context.Context, mediaBytes []byte) error
}

// FFprobe shells out to ffprobe, reading the segment from stdin.
type FFprobe struct{ path string }

func NewFFprobe(path string) *FFprobe {
	if path == "" {
		path = "ffprobe"
	}
	return &FFprobe{path: path}
}

type ffprobeOut struct {
	Streams []struct {
		CodecType string `json:"codec_type"`
		CodecName string `json:"codec_name"`
	} `json:"streams"`
}

func (f *FFprobe) Probe(ctx context.Context, media []byte) error {
	if len(media) == 0 {
		return fmt.Errorf("empty media")
	}
	cmd := exec.CommandContext(ctx, f.path,
		"-v", "error", "-print_format", "json", "-show_streams", "-")
	cmd.Stdin = bytes.NewReader(media)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("ffprobe: %w", err)
	}
	var parsed ffprobeOut
	if err := json.Unmarshal(out.Bytes(), &parsed); err != nil {
		return fmt.Errorf("ffprobe decode: %w", err)
	}
	for _, s := range parsed.Streams {
		if s.CodecType == "video" && s.CodecName != "" {
			return nil
		}
	}
	return fmt.Errorf("no video stream")
}
```

- [ ] **Step 4: Implement `validator.go`:**

```go
package probe

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

const (
	validatorBudget = 10 * time.Second
	maxSegmentBytes = 8 << 20
)

type Validator interface {
	Validate(ctx context.Context, rs ResolvedStream) Verdict
}

type HTTPValidator struct {
	streaming string
	hc        *http.Client
	prober    VideoProber
}

func NewHTTPValidator(streamingBaseURL string, hc *http.Client, vp VideoProber) *HTTPValidator {
	if hc == nil {
		hc = &http.Client{Timeout: validatorBudget}
	}
	return &HTTPValidator{streaming: strings.TrimRight(streamingBaseURL, "/"), hc: hc, prober: vp}
}

// proxyURL builds the streaming hls-proxy URL for a raw upstream URL. When the
// upstream is already a proxied (rewritten) path returned in a manifest, it is
// absolute-from-root and used as-is against the streaming base.
func (v *HTTPValidator) proxyURL(rs ResolvedStream, raw string) string {
	if strings.HasPrefix(raw, "/api/streaming/") {
		return v.streaming + raw
	}
	q := url.Values{"url": {raw}}
	if rs.Exp != "" {
		q.Set("exp", rs.Exp)
	}
	if rs.Sig != "" {
		q.Set("sig", rs.Sig)
	}
	if rs.Referer != "" {
		q.Set("referer", rs.Referer)
	}
	return v.streaming + "/api/streaming/hls-proxy?" + q.Encode()
}

func (v *HTTPValidator) fetch(ctx context.Context, u string) ([]byte, int, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	resp, err := v.hc.Do(req)
	if err != nil {
		return nil, 0, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxSegmentBytes))
	return body, resp.StatusCode, nil
}

func firstNonComment(manifest []byte) string {
	for _, ln := range strings.Split(string(manifest), "\n") {
		ln = strings.TrimSpace(ln)
		if ln == "" || strings.HasPrefix(ln, "#") {
			continue
		}
		return ln
	}
	return ""
}

func (v *HTTPValidator) Validate(ctx context.Context, rs ResolvedStream) Verdict {
	ctx, cancel := context.WithTimeout(ctx, validatorBudget)
	defer cancel()
	verdict := Verdict{Provider: rs.Provider, AnimeUUID: rs.AnimeUUID, Slot: rs.Slot, Server: rs.Server, Stage: StagePlayback}

	master, status, err := v.fetch(ctx, v.proxyURL(rs, rs.MasterURL))
	if err != nil {
		verdict.Reason = streamprobe.ReasonCDNUnreachable
		return verdict
	}
	if status == http.StatusForbidden {
		verdict.Reason = streamprobe.ReasonStatus403
		return verdict
	}
	if status != http.StatusOK || len(master) == 0 {
		verdict.Reason = streamprobe.ReasonEmptyResponse
		return verdict
	}

	// master -> first variant (if present) -> first segment.
	cur := master
	for hops := 0; hops < 2; hops++ {
		line := firstNonComment(cur)
		if line == "" {
			verdict.Reason = streamprobe.ReasonEmptyResponse
			return verdict
		}
		body, st, err := v.fetch(ctx, v.proxyURL(rs, line))
		if err != nil || st == http.StatusForbidden {
			verdict.Reason = streamprobe.ReasonStatus403
			return verdict
		}
		if st != http.StatusOK || len(body) == 0 {
			verdict.Reason = streamprobe.ReasonEmptyResponse
			return verdict
		}
		if !strings.Contains(string(body[:min(len(body), 64)]), "#EXTM3U") {
			// reached a media segment
			if perr := v.prober.Probe(ctx, body); perr != nil {
				verdict.Reason = streamprobe.ReasonDecodeFailed
				return verdict
			}
			verdict.Reason = streamprobe.ReasonPlayable
			return verdict
		}
		cur = body
	}
	verdict.Reason = streamprobe.ReasonInvalidVideo
	return verdict
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
```

- [ ] **Step 5: Run, expect PASS.** `cd services/analytics && go test ./internal/probe/ -run Validator -v`
- [ ] **Step 6: Commit** (`ffprobe.go validator.go validator_test.go`); push.

---

### Task A4: Scorer — per-provider rollup

**Files:**
- Create: `services/analytics/internal/probe/scorer.go`, `scorer_test.go`

**Interfaces:**
- Consumes: `Verdict`, `ProviderVerdict`, `Status`.
- Produces: `func Rollup(provider string, verdicts []Verdict) ProviderVerdict`.

Rules: **up** if any `Playable()`. Else **degraded** if any verdict reached `StageStream` or later (i.e. `Stage==StagePlayback`, meaning it resolved a stream but failed validation). Else **down**. Reason = most-common non-playable reason + the server of its first occurrence, formatted `"<reason> on <server-shortlabel>"`. Server short-label: if it contains `type=hd-1`→`HD-1`, `type=hd-2`→`HD-2`, else the host.

- [ ] **Step 1: Write failing test** covering up / degraded / down + dominant-reason formatting (≥4 asserts). Example skeleton:

```go
package probe

import (
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

func TestRollup_Up(t *testing.T) {
	pv := Rollup("p", []Verdict{{Reason: streamprobe.ReasonStatus403, Stage: StagePlayback}, {Reason: streamprobe.ReasonPlayable, Stage: StagePlayback}})
	if pv.Status != StatusUp || pv.Reason != "" {
		t.Fatalf("got %+v", pv)
	}
}

func TestRollup_Degraded(t *testing.T) {
	pv := Rollup("p", []Verdict{{Reason: streamprobe.ReasonStatus403, Stage: StagePlayback, Server: "x?type=hd-1&y"}})
	if pv.Status != StatusDegraded || pv.Reason != "status_403 on HD-1" {
		t.Fatalf("got %+v", pv)
	}
}

func TestRollup_Down(t *testing.T) {
	pv := Rollup("p", []Verdict{{Reason: streamprobe.ReasonCDNUnreachable, Stage: StageServers}})
	if pv.Status != StatusDown {
		t.Fatalf("got %+v", pv)
	}
}

func TestRollup_Empty(t *testing.T) {
	if Rollup("p", nil).Status != StatusDown {
		t.Fatalf("empty must be down")
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement `scorer.go`:**

```go
package probe

import (
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

func serverShortLabel(server string) string {
	switch {
	case strings.Contains(server, "type=hd-1"):
		return "HD-1"
	case strings.Contains(server, "type=hd-2"):
		return "HD-2"
	}
	if i := strings.Index(server, "//"); i >= 0 {
		rest := server[i+2:]
		if j := strings.IndexByte(rest, '/'); j >= 0 {
			return rest[:j]
		}
		return rest
	}
	return server
}

func Rollup(provider string, verdicts []Verdict) ProviderVerdict {
	pv := ProviderVerdict{Provider: provider, Status: StatusDown}
	if len(verdicts) == 0 {
		return pv
	}
	resolved := false
	counts := map[streamprobe.Reason]int{}
	firstServer := map[streamprobe.Reason]string{}
	for _, v := range verdicts {
		if v.Playable() {
			pv.Status = StatusUp
			return pv
		}
		if v.Stage == StagePlayback {
			resolved = true
		}
		counts[v.Reason]++
		if _, ok := firstServer[v.Reason]; !ok {
			firstServer[v.Reason] = v.Server
		}
	}
	if resolved {
		pv.Status = StatusDegraded
	}
	// dominant reason
	var domR streamprobe.Reason
	best := -1
	for r, c := range counts {
		if c > best {
			best, domR = c, r
		}
	}
	pv.Reason = string(domR) + " on " + serverShortLabel(firstServer[domR])
	return pv
}
```

- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit** (`scorer.go scorer_test.go`); push.

---

### Task A5: AnimeSetResolver — 4 slots

**Files:**
- Create: `services/analytics/internal/probe/animeset.go`, `animeset_test.go`

**Interfaces:**
- Produces: `type AnimeRef struct { UUID string; Slot AnimeSlot }`; `type AnimeSetResolver interface { Resolve(ctx) ([]AnimeRef, error) }`; `NewHTTPAnimeSet(catalogBaseURL, anchorUUID string, hc *http.Client, rng *rand.Rand) *HTTPAnimeSet`.

Anchor is always included. `featured` = first card's `anime_id`; `spotlight_random` = a random card's `anime_id` (≠ featured when possible); `random` = a random card too if no dedicated random endpoint (acceptable for now). Source: `GET {catalog}/api/home/spotlight` → `data.cards[] = {anime_id, ...}`. Cards without `anime_id` are skipped. Slots that can't be filled are omitted (anchor always present).

- [ ] **Step 1: Write failing test** — stub returns 3 cards with `anime_id`; assert anchor present + featured = card0 + a spotlight_random from the set; missing-spotlight → only anchor.

```go
package probe

import (
	"context"
	"math/rand"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAnimeSet_AnchorAlways(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(`{"success":true,"data":{"cards":[{"anime_id":"a"},{"anime_id":"b"},{"anime_id":"c"}]}}`))
	}))
	defer srv.Close()
	as := NewHTTPAnimeSet(srv.URL, "ANCHOR", srv.Client(), rand.New(rand.NewSource(1)))
	refs, err := as.Resolve(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	bySlot := map[AnimeSlot]string{}
	for _, r := range refs {
		bySlot[r.Slot] = r.UUID
	}
	if bySlot[SlotAnchor] != "ANCHOR" || bySlot[SlotFeatured] != "a" {
		t.Fatalf("got %+v", bySlot)
	}
	if _, ok := bySlot[SlotSpotlightRandom]; !ok {
		t.Fatalf("expected spotlight_random")
	}
}

func TestAnimeSet_SpotlightDown_AnchorOnly(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(500)
	}))
	defer srv.Close()
	as := NewHTTPAnimeSet(srv.URL, "ANCHOR", srv.Client(), rand.New(rand.NewSource(1)))
	refs, _ := as.Resolve(context.Background())
	if len(refs) != 1 || refs[0].Slot != SlotAnchor {
		t.Fatalf("want anchor-only, got %+v", refs)
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement `animeset.go`:**

```go
package probe

import (
	"context"
	"encoding/json"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

type AnimeRef struct {
	UUID string
	Slot AnimeSlot
}

type AnimeSetResolver interface {
	Resolve(ctx context.Context) ([]AnimeRef, error)
}

type HTTPAnimeSet struct {
	base   string
	anchor string
	hc     *http.Client
	rng    *rand.Rand
}

func NewHTTPAnimeSet(catalogBaseURL, anchorUUID string, hc *http.Client, rng *rand.Rand) *HTTPAnimeSet {
	if hc == nil {
		hc = &http.Client{Timeout: 10 * time.Second}
	}
	return &HTTPAnimeSet{base: strings.TrimRight(catalogBaseURL, "/"), anchor: anchorUUID, hc: hc, rng: rng}
}

func (a *HTTPAnimeSet) Resolve(ctx context.Context) ([]AnimeRef, error) {
	refs := []AnimeRef{{UUID: a.anchor, Slot: SlotAnchor}}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.base+"/api/home/spotlight", nil)
	resp, err := a.hc.Do(req)
	if err != nil {
		return refs, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return refs, nil
	}
	var env struct {
		Data struct {
			Cards []struct {
				AnimeID string `json:"anime_id"`
			} `json:"cards"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&env); err != nil {
		return refs, nil
	}
	var ids []string
	for _, c := range env.Data.Cards {
		if c.AnimeID != "" && c.AnimeID != a.anchor {
			ids = append(ids, c.AnimeID)
		}
	}
	if len(ids) == 0 {
		return refs, nil
	}
	refs = append(refs, AnimeRef{UUID: ids[0], Slot: SlotFeatured})
	if len(ids) > 1 {
		pick := ids[1+a.rng.Intn(len(ids)-1)]
		refs = append(refs, AnimeRef{UUID: pick, Slot: SlotSpotlightRandom})
		other := ids[a.rng.Intn(len(ids))]
		refs = append(refs, AnimeRef{UUID: other, Slot: SlotRandom})
	}
	return refs, nil
}
```

- [ ] **Step 4: Run, expect PASS.**
- [ ] **Step 5: Commit** (`animeset.go animeset_test.go`); push.

---

### Task A6: Probe metrics + Reporter

**Files:**
- Create: `libs/metrics/probe.go`, `services/analytics/internal/probe/reporter.go`, `reporter_test.go`

**Interfaces:**
- Produces: `metrics.ProbeProviderUp` (GaugeVec{provider}), `metrics.ProbeRunsTotal` (CounterVec{provider,slot,server,result,reason}), `metrics.ProbeLastRun` (Gauge); `type Reporter interface { Report(ctx, run RunResult) error }`; `RunResult` struct; `NewPromReporter(chWriter CHWriter) *PromReporter`; `type CHWriter interface { InsertProbeRows(ctx, rows []ProbeRow) error }`; `ProbeRow`.

- [ ] **Step 1: Implement `libs/metrics/probe.go`:**

```go
package metrics

import "github.com/prometheus/client_golang/prometheus/promauto"
import "github.com/prometheus/client_golang/prometheus"

var (
	ProbeProviderUp = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "probe_provider_up",
		Help: "Per-provider playability verdict: 1 up, 0.5 degraded, 0 down.",
	}, []string{"provider"})

	ProbeRunsTotal = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "probe_runs_total",
		Help: "Playability probe results per (provider, slot, server, result, reason).",
	}, []string{"provider", "slot", "server", "result", "reason"})

	ProbeLastRun = promauto.NewGauge(prometheus.GaugeOpts{
		Name: "probe_last_run_timestamp",
		Help: "Unix timestamp of the last completed probe run.",
	})
)
```

- [ ] **Step 2: Write failing test** `reporter_test.go` — fake `CHWriter` captures rows; `Report` sets gauges (assert via `testutil.ToFloat64`) and inserts one row per verdict.

```go
package probe

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

type fakeCH struct{ rows []ProbeRow }

func (f *fakeCH) InsertProbeRows(_ context.Context, rows []ProbeRow) error {
	f.rows = append(f.rows, rows...)
	return nil
}

func TestReporter_SetsGaugeAndRows(t *testing.T) {
	ch := &fakeCH{}
	rep := NewPromReporter(ch)
	run := RunResult{
		ProviderVerdicts: []ProviderVerdict{{Provider: "gogoanime", Status: StatusUp}},
		Verdicts: []Verdict{{Provider: "gogoanime", Slot: SlotAnchor, Server: "s", Reason: streamprobe.ReasonPlayable}},
		At: 1000,
	}
	if err := rep.Report(context.Background(), run); err != nil {
		t.Fatal(err)
	}
	if got := testutil.ToFloat64(metrics.ProbeProviderUp.WithLabelValues("gogoanime")); got != 1.0 {
		t.Fatalf("gauge=%v", got)
	}
	if len(ch.rows) != 1 {
		t.Fatalf("rows=%d", len(ch.rows))
	}
}
```

- [ ] **Step 3: Run, expect FAIL.**

- [ ] **Step 4: Implement `reporter.go`:**

```go
package probe

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/metrics"
)

// ProbeRow is one ClickHouse history row.
type ProbeRow struct {
	RunTS     int64
	Provider  string
	AnimeUUID string
	Slot      string
	Server    string
	Stage     string
	Reason    string
	Playable  bool
}

type CHWriter interface {
	InsertProbeRows(ctx context.Context, rows []ProbeRow) error
}

type RunResult struct {
	ProviderVerdicts []ProviderVerdict
	Verdicts         []Verdict
	At               int64 // unix seconds
}

type Reporter interface {
	Report(ctx context.Context, run RunResult) error
}

type PromReporter struct{ ch CHWriter }

func NewPromReporter(ch CHWriter) *PromReporter { return &PromReporter{ch: ch} }

func (r *PromReporter) Report(ctx context.Context, run RunResult) error {
	for _, pv := range run.ProviderVerdicts {
		metrics.ProbeProviderUp.WithLabelValues(pv.Provider).Set(pv.Status.Gauge())
	}
	rows := make([]ProbeRow, 0, len(run.Verdicts))
	for _, v := range run.Verdicts {
		result := "fail"
		if v.Playable() {
			result = "pass"
		}
		metrics.ProbeRunsTotal.WithLabelValues(v.Provider, string(v.Slot), v.Server, result, string(v.Reason)).Inc()
		rows = append(rows, ProbeRow{
			RunTS: run.At, Provider: v.Provider, AnimeUUID: v.AnimeUUID, Slot: string(v.Slot),
			Server: v.Server, Stage: string(v.Stage), Reason: string(v.Reason), Playable: v.Playable(),
		})
	}
	metrics.ProbeLastRun.Set(float64(run.At))
	if r.ch != nil {
		return r.ch.InsertProbeRows(ctx, rows)
	}
	return nil
}
```

- [ ] **Step 5: Run, expect PASS.**
- [ ] **Step 6: Commit** (`libs/metrics/probe.go reporter.go reporter_test.go`); push.

---

### Task A7: Engine — orchestrate RunOnce

**Files:**
- Create: `services/analytics/internal/probe/engine.go`, `engine_test.go`

**Interfaces:**
- Consumes: `Resolver`, `Validator`, `AnimeSetResolver`, `Reporter`, `Rollup`.
- Produces: `type Engine struct{...}`; `NewEngine(providers []string, as AnimeSetResolver, res Resolver, val Validator, rep Reporter, now func() int64, log *logger.Logger) *Engine`; `func (e *Engine) RunOnce(ctx) error`.

For each provider: gather verdicts across all anime refs (resolve → validate each server; if resolve fails before stream, synthesize one `Verdict{Stage: <failed stage>, Reason: cdn_unreachable|...}`). `Rollup` per provider. Build `RunResult`, call `Reporter.Report`. Bounded concurrency (sequential is fine for 5 providers × 4 anime daily). Per-provider isolation: a panic/err in one provider never aborts the run.

- [ ] **Step 1: Write failing test** `engine_test.go` — fakes for AnimeSet (1 anchor), Resolver (returns 1 stream), Validator (returns playable), Reporter (captures RunResult). Assert provider verdict Up and Reporter called once.

```go
package probe

import (
	"context"
	"testing"

	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type fakeAS struct{}
func (fakeAS) Resolve(_ context.Context) ([]AnimeRef, error) { return []AnimeRef{{UUID: "u", Slot: SlotAnchor}}, nil }
type fakeRes struct{}
func (fakeRes) Resolve(_ context.Context, u string, s AnimeSlot, p string) ([]ResolvedStream, Stage, error) {
	return []ResolvedStream{{Provider: p, AnimeUUID: u, Slot: s, Server: "srv", Stage: StageStream}}, StageStream, nil
}
type fakeVal struct{}
func (fakeVal) Validate(_ context.Context, rs ResolvedStream) Verdict {
	return Verdict{Provider: rs.Provider, AnimeUUID: rs.AnimeUUID, Slot: rs.Slot, Server: rs.Server, Stage: StagePlayback, Reason: streamprobe.ReasonPlayable}
}
type capRep struct{ run RunResult; n int }
func (c *capRep) Report(_ context.Context, run RunResult) error { c.run = run; c.n++; return nil }

func TestEngine_RunOnce(t *testing.T) {
	rep := &capRep{}
	e := NewEngine([]string{"gogoanime"}, fakeAS{}, fakeRes{}, fakeVal{}, rep, func() int64 { return 42 }, nil)
	if err := e.RunOnce(context.Background()); err != nil {
		t.Fatal(err)
	}
	if rep.n != 1 || len(rep.run.ProviderVerdicts) != 1 || rep.run.ProviderVerdicts[0].Status != StatusUp {
		t.Fatalf("got %+v (n=%d)", rep.run, rep.n)
	}
	if rep.run.At != 42 {
		t.Fatalf("At=%d", rep.run.At)
	}
}
```

- [ ] **Step 2: Run, expect FAIL.**

- [ ] **Step 3: Implement `engine.go`:**

```go
package probe

import (
	"context"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/streamprobe"
)

type Engine struct {
	providers []string
	as        AnimeSetResolver
	res       Resolver
	val       Validator
	rep       Reporter
	now       func() int64
	log       *logger.Logger
}

func NewEngine(providers []string, as AnimeSetResolver, res Resolver, val Validator, rep Reporter, now func() int64, log *logger.Logger) *Engine {
	return &Engine{providers: providers, as: as, res: res, val: val, rep: rep, now: now, log: log}
}

func (e *Engine) RunOnce(ctx context.Context) error {
	refs, err := e.as.Resolve(ctx)
	if err != nil && len(refs) == 0 {
		return err
	}
	var allVerdicts []Verdict
	var provVerdicts []ProviderVerdict

	for _, p := range e.providers {
		var verdicts []Verdict
		for _, ref := range refs {
			streams, stage, rerr := e.res.Resolve(ctx, ref.UUID, ref.Slot, p)
			if rerr != nil {
				verdicts = append(verdicts, Verdict{
					Provider: p, AnimeUUID: ref.UUID, Slot: ref.Slot, Stage: stage,
					Reason: streamprobe.ReasonCDNUnreachable,
				})
				continue
			}
			for _, s := range streams {
				verdicts = append(verdicts, e.val.Validate(ctx, s))
			}
		}
		allVerdicts = append(allVerdicts, verdicts...)
		provVerdicts = append(provVerdicts, Rollup(p, verdicts))
	}

	return e.rep.Report(ctx, RunResult{ProviderVerdicts: provVerdicts, Verdicts: allVerdicts, At: e.now()})
}
```

- [ ] **Step 4: Run, expect PASS.** `cd services/analytics && go test ./internal/probe/ -v`
- [ ] **Step 5: Commit** (`engine.go engine_test.go`); push.

---

### Task A8: ClickHouse probe table + InsertProbeRows

**Files:**
- Modify: `services/analytics/internal/repo/clickhouse_schema.go` (add table to `EnsureSchema`)
- Modify: `services/analytics/internal/repo/clickhouse_store.go` (add `InsertProbeRows`)
- Test: `services/analytics/internal/repo/clickhouse_store_test.go` (add a shape/SQL-builder test if one exists; else a thin unit test of the row→args mapping)

**Interfaces:**
- Produces: `func (s *ClickHouseStore) InsertProbeRows(ctx context.Context, rows []probe.ProbeRow) error` — satisfies `probe.CHWriter`.

- [ ] **Step 1: Read** `clickhouse_schema.go` + `clickhouse_store.go` to mirror the existing `CREATE TABLE IF NOT EXISTS` + batch-insert idiom (driver `clickhouse-go/v2`, `conn.PrepareBatch`).

- [ ] **Step 2: Add table** in `EnsureSchema` (engine `MergeTree`, TTL ~90d, mirror existing tables' column style):

```sql
CREATE TABLE IF NOT EXISTS probe_runs (
  run_ts      DateTime,
  provider    LowCardinality(String),
  anime_uuid  String,
  slot        LowCardinality(String),
  server      String,
  stage       LowCardinality(String),
  reason      LowCardinality(String),
  playable    UInt8
) ENGINE = MergeTree ORDER BY (run_ts, provider) TTL run_ts + INTERVAL 90 DAY
```

- [ ] **Step 3: Implement `InsertProbeRows`** mirroring the existing batch-insert method (import the `probe` package for `ProbeRow`):

```go
func (s *ClickHouseStore) InsertProbeRows(ctx context.Context, rows []probe.ProbeRow) error {
	if len(rows) == 0 {
		return nil
	}
	batch, err := s.conn.PrepareBatch(ctx, "INSERT INTO probe_runs")
	if err != nil {
		return err
	}
	for _, r := range rows {
		var pl uint8
		if r.Playable {
			pl = 1
		}
		if err := batch.Append(time.Unix(r.RunTS, 0), r.Provider, r.AnimeUUID, r.Slot, r.Server, r.Stage, r.Reason, pl); err != nil {
			return err
		}
	}
	return batch.Send()
}
```

(Adjust receiver/field names to the actual store. If `probe` importing `repo` would cycle, define `CHWriter` against an interface only — `repo` imports `probe` for the row type, never the reverse; confirm no cycle.)

- [ ] **Step 4: Build** `cd services/analytics && go build ./...` — expect success.
- [ ] **Step 5: Commit**; push.

---

### Task A9: Config + handler + router + DI + Dockerfile (ffmpeg)

**Files:**
- Modify: `services/analytics/internal/config/config.go` (+ test)
- Create: `services/analytics/internal/handler/probe.go`, `probe_test.go`
- Modify: `services/analytics/internal/transport/router.go`
- Modify: `services/analytics/cmd/analytics-api/main.go`
- Modify: `services/analytics/Dockerfile`

**Interfaces:**
- Consumes: `probe.Engine` (Task A7), config values.
- Produces: `ProbeHandler` at `POST /internal/probe/run`.

- [ ] **Step 1: Add config** fields (mirror existing `getEnv` pattern): `CatalogURL` (`CATALOG_URL`, default `http://catalog:8081`), `StreamingURL` (`STREAMING_URL`, default `http://streaming:8082`), `ProbeAnchorUUID` (`PROBE_ANCHOR_UUID`, default `f0b40660-6627-4a59-8dcf-7ec8596b3623`), `FFprobePath` (`FFPROBE_PATH`, default `ffprobe`), `ProbeProviders` (`PROBE_PROVIDERS`, default `gogoanime,miruro,allanime,nineanime,animefever`). Add a config test asserting the defaults.

- [ ] **Step 2: Write failing handler test** `probe_test.go` (mirror `player_ranking_test.go`): fake runner; POST → 204; runner error → 500.

```go
package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

type fakeRunner struct{ err error }
func (f fakeRunner) RunOnce(_ context.Context) error { return f.err }

func TestProbeHandler_OK(t *testing.T) {
	h := NewProbeHandler(fakeRunner{})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/internal/probe/run", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("code=%d", rr.Code)
	}
}

func TestProbeHandler_Err(t *testing.T) {
	h := NewProbeHandler(fakeRunner{err: errors.New("x")})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPost, "/internal/probe/run", nil))
	if rr.Code != http.StatusInternalServerError {
		t.Fatalf("code=%d", rr.Code)
	}
}
```

- [ ] **Step 3: Implement `handler/probe.go`:**

```go
package handler

import (
	"context"
	"net/http"
	"time"
)

type probeRunner interface {
	RunOnce(ctx context.Context) error
}

type ProbeHandler struct{ runner probeRunner }

func NewProbeHandler(r probeRunner) *ProbeHandler { return &ProbeHandler{runner: r} }

func (h *ProbeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if err := h.runner.RunOnce(ctx); err != nil {
		http.Error(w, "probe run failed", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
```

- [ ] **Step 4: Register route** in `router.go` — add `probe *handler.ProbeHandler` param and `r.Post("/internal/probe/run", probe.ServeHTTP)`; update the doc-comment route list. Update the `NewRouter(...)` call in `main.go`.

- [ ] **Step 5: Wire DI** in `main.go` (only when ClickHouse `chConn` is available, mirroring the player-ranking guard): build `probe.NewEngine(strings.Split(cfg.ProbeProviders, ","), animeSet, resolver, validator, reporter, func() int64 { return time.Now().Unix() }, log)` where `resolver = probe.NewHTTPResolver(cfg.CatalogURL, nil)`, `validator = probe.NewHTTPValidator(cfg.StreamingURL, nil, probe.NewFFprobe(cfg.FFprobePath))`, `animeSet = probe.NewHTTPAnimeSet(cfg.CatalogURL, cfg.ProbeAnchorUUID, nil, rand.New(rand.NewSource(time.Now().UnixNano())))`, `reporter = probe.NewPromReporter(chStore)`. Pass the engine into `NewProbeHandler`.

- [ ] **Step 6: Dockerfile** — add ffmpeg to the runtime stage:

```dockerfile
RUN apk add --no-cache ca-certificates tzdata wget ffmpeg
```

- [ ] **Step 7: Build + test** `cd services/analytics && go build ./... && go test ./...` — expect success.
- [ ] **Step 8: Commit** (`config.go handler/probe.go handler/probe_test.go transport/router.go cmd/analytics-api/main.go Dockerfile` + config test); push.

---

## Phase B — Scheduler trigger (replace canary)

### Task B1: probe_trigger job + config

**Files:**
- Create: `services/scheduler/internal/jobs/probe_trigger.go`, `probe_trigger_test.go`
- Modify: `services/scheduler/internal/config/config.go`
- Delete: `services/scheduler/internal/jobs/scraper_playability_canary.go` (+ `_test.go`, fixtures)

**Interfaces:**
- Produces: `NewProbeTriggerJob(cfg *config.JobsConfig, log *logger.Logger) *ProbeTriggerJob` with `Run(ctx) error` POSTing `{AnalyticsInternalURL}/internal/probe/run`.

- [ ] **Step 1: Write failing test** (mirror `provider_ranking_test.go`): httptest captures path `/internal/probe/run`, returns 204; assert `Run` nil-err and path hit.

- [ ] **Step 2: Implement `probe_trigger.go`** — copy `provider_ranking.go` structure verbatim, swapping the URL to `/internal/probe/run`, type name `ProbeTriggerJob`, timeout `5*time.Minute` (probe is slower than a recompute):

```go
package jobs

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/services/scheduler/internal/config"
)

const probeTriggerReqTimeout = 5 * time.Minute

type ProbeTriggerJob struct {
	client *http.Client
	config *config.JobsConfig
	log    *logger.Logger
}

func NewProbeTriggerJob(cfg *config.JobsConfig, log *logger.Logger) *ProbeTriggerJob {
	return &ProbeTriggerJob{client: &http.Client{Timeout: probeTriggerReqTimeout}, config: cfg, log: log}
}

func (j *ProbeTriggerJob) Run(ctx context.Context) error {
	url := j.config.AnalyticsInternalURL + "/internal/probe/run"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	if err != nil {
		return fmt.Errorf("build probe request: %w", err)
	}
	resp, err := j.client.Do(req)
	if err != nil {
		return fmt.Errorf("post probe: %w", err)
	}
	defer func() { _, _ = io.Copy(io.Discard, resp.Body); _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("probe returned status %d", resp.StatusCode)
	}
	return nil
}
```

- [ ] **Step 3: Config** — in `config.go` rename `ScraperPlayabilityCanaryCron` → `PlaybackProbeCron` (env `PLAYBACK_PROBE_CRON`, default `0 3 * * *`); remove now-unused canary-specific config (`CanaryReportDir`, anchor MALs, etc. — grep them out). Keep `AnalyticsInternalURL`.

- [ ] **Step 4: Delete** `scraper_playability_canary.go` + `scraper_playability_canary_test.go` + any fixture files under the jobs dir referencing it (`grep -rl scraper_playability_canary services/scheduler`).

- [ ] **Step 5: Build** `cd services/scheduler && go build ./...` — will FAIL until Task B2 rewires `main.go`/`job.go`. Proceed to B2 in the same task group.

- [ ] **Step 6: Commit** with B2 (single reviewable unit).

---

### Task B2: Wire probe_trigger into JobService + main

**Files:**
- Modify: `services/scheduler/internal/service/job.go`
- Modify: `services/scheduler/cmd/scheduler-api/main.go`

- [ ] **Step 1: Read** `job.go` `NewJobService(...)` + `Start(...)` signatures.
- [ ] **Step 2: Swap** the `scraperPlayabilityCanaryJob` param/field for `probeTriggerJob *jobs.ProbeTriggerJob` throughout `NewJobService` and its `Start` cron list; replace the canary cron arg with `cfg.Jobs.PlaybackProbeCron`.
- [ ] **Step 3: main.go** — replace `jobs.NewScraperPlayabilityCanaryJob(db.DB, &cfg.Jobs, log)` with `jobs.NewProbeTriggerJob(&cfg.Jobs, log)` (no `db.DB` dependency now); update the `NewJobService(...)` call + the `Start(...)` cron list (line ~149 `ScraperPlayabilityCanaryCron` → `PlaybackProbeCron`).
- [ ] **Step 4: Build + test** `cd services/scheduler && go build ./... && go test ./...` — expect success.
- [ ] **Step 5: Commit** (B1+B2 files together); push.

---

## Phase C — Delete scraper probe

### Task C1: Remove scraper in-process probe + its metrics

**Files:**
- Delete: `services/scraper/internal/health/probe.go` (+ `probe_test.go` + probe-only helpers in `health/` that nothing else imports — verify with grep)
- Modify: `services/scraper/cmd/scraper-api/main.go` (remove probe-spawn loop + gauge-seed loop)
- Modify: `libs/metrics/provider.go` (remove `ProviderHealthUp`, `ProviderProbeLastTick`)

**Constraint:** Keep `health.InMemoryHealthCache` / `IsHealthy` / `AllStages` if the orchestrator still imports them — only remove the probe runner + metric emission. The cache becomes unfed (defaults healthy); that is acceptable (observability moved to analytics).

- [ ] **Step 1: Map usage** — `grep -rn "ProviderHealthUp\|ProviderProbeLastTick\|NewProbeRunner\|ProbeRunner\|RunOnce\|markRemainingStale" services/scraper libs/metrics`. Identify every reference.
- [ ] **Step 2: Delete** `health/probe.go` + `health/probe_test.go`.
- [ ] **Step 3: Edit `cmd/scraper-api/main.go`** — remove the per-provider probe-spawn goroutine block (around lines ~536–560) and the gauge-seed loop that calls `metrics.ProviderHealthUp` / `metrics.ProviderProbeLastTick` (around ~518–527). Remove now-unused imports (`health` only if nothing else there is used; otherwise keep). Keep the orchestrator + health cache construction (line ~212) if `IsHealthy` is still referenced.
- [ ] **Step 4: Remove** `ProviderHealthUp` + `ProviderProbeLastTick` var blocks from `libs/metrics/provider.go` (and their doc comments). Confirm no other service references them (`grep -rn` across `services/`).
- [ ] **Step 5: Build all affected** `cd services/scraper && go build ./...` and `cd libs/metrics && go build ./...` — expect success.
- [ ] **Step 6: Commit** (`git commit services/scraper/internal/health/probe.go services/scraper/internal/health/probe_test.go services/scraper/cmd/scraper-api/main.go libs/metrics/provider.go -m "chore(scraper): remove in-process probe — unified into analytics" + co-authors`); push. Run `git show --stat HEAD` to confirm only these paths.

---

## Phase D — Dashboard + animefever comment

### Task D1: Rebuild dashboard panel as a table + repoints

**Files:**
- Modify: `docker/grafana/dashboards/playback-health.json`

- [ ] **Step 1: Read** the `"Provider × Stage Up"` state-timeline panel object and note its `gridPos`, `id`, `datasource`.
- [ ] **Step 2: Replace** that panel with a `type: "table"` panel titled `"Provider Playability"` keeping the same `gridPos`/`id`. Targets:
  - A: `probe_provider_up{provider=~"$provider"}` (instant, format `table`) → provides Provider + numeric status.
  - B: `probe_runs_total{provider=~"$provider", result="fail"}` topk by reason → for the Reason column (or use a `label_join`/transform to surface the latest reason).
  Add field transforms: map `probe_provider_up` value `1→up / 0.5→degraded / 0→down`, and value-mappings + thresholds coloring green/amber/red. Columns shown: **Provider | Stage Status | Reason**. (If a single Prometheus query can't carry the reason text, drive Reason from the `probe_runs_total` label via a "Labels to fields" + "Group by provider" transform.)
- [ ] **Step 3: Repoint** the `"Canary Last Run (age)"` stat expr → `time() - probe_last_run_timestamp`.
- [ ] **Step 4: Repoint** `"Top Failing (provider, server, reason, slot) Tuples"` expr → `topk(15, sum by (provider, server, reason, slot) (probe_runs_total{result="fail", provider=~"$provider"}))`.
- [ ] **Step 5: Validate JSON** `python3 -c "import json; json.load(open('docker/grafana/dashboards/playback-health.json'))"` — expect no error.
- [ ] **Step 6: Commit**; push.

---

### Task D2: animefever description — drop "Region-walled"

**Files:**
- Modify: `services/catalog/internal/service/scraperprovider/seed.go`
- Create: a guarded migration (mirror the existing scraper-provider description-update migration pattern under catalog)
- Modify: `CLAUDE.md`

- [ ] **Step 1: Read** `seed.go` animefever entry + locate the existing guarded-update migration pattern (`grep -rn "animefever\|UPDATE scraper_providers\|description" services/catalog/internal/service/scraperprovider services/catalog/migrations 2>/dev/null`).
- [ ] **Step 2: Edit** the animefever `description` in `seed.go` to remove region claims. New text:

> `animefever.cc → am.vidstream.vip returns a valid manifest, but HLS segments 302-redirect to an ad CDN (ibytedtos / ad-site-i18n-sg) that 403s for us, so playback fails. Degraded: kept manually selectable (hacker mode) but out of the auto-failover chain.`

- [ ] **Step 3: Guarded migration** — since the seed is insert-if-absent, add a one-shot `UPDATE scraper_providers SET description = '<new text>' WHERE provider='animefever'` migration (follow the catalog migration mechanism; if catalog uses GORM AutoMigrate + a `migrations/` SQL dir, add the SQL file; otherwise add to the seed's update path).
- [ ] **Step 4: Update `CLAUDE.md`** — replace the animefever bullet text (in the OurEnglish/proxy sections) to match, dropping the "Region-walled"/egress-IP-class wording.
- [ ] **Step 5: Build** `cd services/catalog && go build ./...` — expect success.
- [ ] **Step 6: Commit** (`seed.go`, migration file, `CLAUDE.md`); push.

---

## Phase E — Deploy & verify (via /animeenigma-after-update)

### Task E1: Deploy, verify, changelog

- [ ] **Step 1:** Lint/build affected: analytics, scheduler, scraper, catalog, plus `bash frontend/web/scripts/i18n-lint.sh` (no FE change expected). Grafana JSON validated in D1.
- [ ] **Step 2: Deploy** from a CLEAN `origin/main` worktree (copy `docker/.env`, compose project stays `docker`): `make redeploy-analytics redeploy-scheduler redeploy-scraper redeploy-catalog` then `make restart-grafana` (dashboard JSON is provisioned).
- [ ] **Step 3: Verify** — `make health`; trigger one probe run manually: `docker exec animeenigma-scheduler wget -qO- --post-data='' http://analytics:8092/internal/probe/run` (or curl from the analytics network); then `docker exec animeenigma-analytics wget -qO- http://localhost:8092/metrics | grep -E "probe_provider_up|probe_runs_total|probe_last_run"`. Confirm `probe_provider_up{provider="gogoanime"}` reflects reality (expect `0.5`/`0` given the live megaplay 403), and Frieren anchor appears in `probe_runs_total{result="fail"}`.
- [ ] **Step 4: Confirm** the scraper no longer exposes `provider_health_up` (`docker exec animeenigma-scraper wget -qO- http://localhost:8088/metrics | grep -c provider_health_up` → `0`).
- [ ] **Step 5: Changelog + commit + push** via the `/animeenigma-after-update` skill (Russian Trump-mode entry: "ЕДИНЫЙ честный пробер playability — теперь зелёное значит РАБОТАЕТ").

---

## Self-Review notes (author)

- Spec §3 D1–D9 each map to a task: D1→A*, D2→C1, D3→B1/B2, D4→B1/A9, D5→A2/A3, D6→A3/A9, D7→A5, D8→D1, D9→A2. ✓
- animefever (#2)→D2; dashboard (#1)→D1; daily (#3-cron)→B1; anime set (#3)→A5; video check (#4)→A3. ✓
- Type consistency: `Resolver.Resolve` signature identical in A2/A7; `CHWriter.InsertProbeRows`/`ProbeRow` identical in A6/A8; `probeRunner.RunOnce`/`Engine.RunOnce` match in A7/A9. ✓
- Open risk to validate at execution: catalog scraper endpoints reachable unauthenticated from analytics over the Docker network; `repo`↔`probe` import direction (repo imports probe for `ProbeRow`, never reverse) — confirm no cycle in A8.
