# Privacy Core: Opaque Stream Tokens (Track A) + Masked Analytics Fallback (Track B5) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Remove the CDN hostname + `proxy?url=` shape from every media URL (opaque AES-GCM path tokens, always-on) and give the FE analytics clients a rotating HMAC-masked fallback path when extensions block `/api/analytics/*`.

**Architecture:** Track A seals `{url, referer, exp, type}` into an AES-256-GCM token placed in the URL path (`/api/streaming/m/<token>/<leaf>`); the m3u8 rewriter emits it for every child URL, catalog stamps a `masked_url` sibling next to today's `exp`/`sig` (dual-accept both directions), and a preauth proxy entry point skips the allowlist gate (the token IS the authorization). Track B5 adds a gateway param route `/api/{hmac-bucket}/{c|e|p}` validated against current+previous hour buckets, advertised to the SPA via an `X-AE-Cfg` response header; a shared FE resolver switches all three analytics clients to it when a probe fetch detects extension blocking (TypeError).

**Tech Stack:** Go (chi, crypto/aes+cipher.NewGCM, crypto/hmac), Vue 3 / TypeScript, vitest, host nginx.

**Spec:** `docs/superpowers/specs/2026-07-10-playback-resilience-constrained-browsers-design.md` §3 (Track A), §4-B5, §8. B1–B4 are explicitly deferred; without B1's probe, B5 detection is a one-shot probe fetch + opportunistic fetch-failure detection inside the shared resolver (the spec's "analytics client endpoint resolver" unit).

## Global Constraints

- **Golden rule:** all work in the worktree; NEVER edit `/data/animeenigma` (base tree) directly. Absolute paths in Edit/Write calls must point INTO the worktree.
- Repo metrics convention: no time units. This plan: **UXΔ = +2 (Better) · CDI = 0.05 * 21 · MVQ = Basilisk 85%/80%**.
- `libs/videoutils` is an existing module — NO Dockerfile or go.mod changes needed (no new deps; stdlib only). NEVER run `go work sync`.
- Secret: reuse `loadProvenanceSecret()` (`STREAM_TOKEN_SECRET` → `JWT_SECRET` fallback, fail-closed). No new required env. Kill-switch: `AE_MASKED_STREAM_DISABLED=1` disables token minting (automatic legacy fallback).
- Dual-accept: legacy `?url=&referer=&exp=&sig=` handler stays untouched and alive. `masked_url` is a SIBLING field — old FE bundles ignore it, new FE prefers it.
- Metrics cardinality: `libs/metrics` `normalizePath(r.URL.Path)` runs AFTER the handler (metrics.go:134→138) — every handler that receives a rotating path MUST rewrite `r.URL.Path` to a stable value before returning/proxying.
- Go tests for libs run only via `cd libs/videoutils && go test ./...` (NOT in `make test`/CI service loop).
- FE: `bun`/`bunx` only. Vitest: `bunx vitest run`. No i18n changes in this scope (no user-visible strings).
- Subagents commit but do NOT push (memory: subagent-commit-not-push). Commit trailers (exact three lines):
  ```
  Co-Authored-By: Claude Code <noreply@anthropic.com>
  Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
  Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>
  ```
- Deploy ORDER (Task 12, main session only): host nginx location → gateway → streaming → catalog → scraper+gacha (videoutils importers) → web.

---

### Task 0: Worktree setup

**Files:** none modified (setup only)

- [ ] **Step 1: Create the worktree from fresh origin/main**

```bash
cd /data/animeenigma
git fetch origin
git worktree add -b feat/privacy-core-tokens /tmp/ae-privacy-core origin/main
cd /tmp/ae-privacy-core/frontend/web && bun install
```

- [ ] **Step 2: Copy this plan into the worktree and commit**

```bash
cp /tmp/claude-0/-data-animeenigma/ef5dbfb3-00d9-46fe-b1a8-e52155d7c9e4/scratchpad/2026-07-11-privacy-core-stream-tokens-masked-analytics.md \
   /tmp/ae-privacy-core/docs/superpowers/plans/
cd /tmp/ae-privacy-core
git add docs/superpowers/plans/2026-07-11-privacy-core-stream-tokens-masked-analytics.md
git commit -m "docs(plan): privacy core — opaque stream tokens + masked analytics"
```

---

### Task 1: `libs/videoutils/streamtoken.go` — AES-GCM stream tokens

**Files:**
- Create: `libs/videoutils/streamtoken.go`
- Test: `libs/videoutils/streamtoken_test.go`

**Interfaces:**
- Consumes: `loadProvenanceSecret()`, `provenanceConfigured`, `provenanceTTL`, `allowLoopbackForTest`, `netguard.ValidatePublicURL` (all existing in package `videoutils`).
- Produces: `type StreamTokenPayload struct { URL, Referer string; Exp int64; Type string }` · `EncodeStreamToken(rawURL, referer, streamType string, now time.Time) string` · `DecodeStreamToken(token string, now time.Time) (*StreamTokenPayload, error)` · `MaskedStreamURL(rawURL, referer, streamType string) string` · `maskedLeaf(rawURL string) string`. Tasks 3, 4, 5 rely on these exact names.

- [ ] **Step 1: Write the failing test**

`libs/videoutils/streamtoken_test.go` (package `videoutils` — TestMain in `videoutils_main_test.go` already sets `STREAM_TOKEN_SECRET` + `allowLoopbackForTest`):

```go
package videoutils

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestStreamToken_RoundTrip(t *testing.T) {
	now := time.Now()
	tok := EncodeStreamToken("https://p12.solodcdn.com/s/m/seg-1.ts", "https://kodikplayer.com/", "", now)
	if tok == "" {
		t.Fatal("expected non-empty token with configured secret")
	}
	if strings.ContainsAny(tok, "/+=") {
		t.Fatalf("token must be a single URL-safe path segment, got %q", tok)
	}
	p, err := DecodeStreamToken(tok, now)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.URL != "https://p12.solodcdn.com/s/m/seg-1.ts" || p.Referer != "https://kodikplayer.com/" || p.Type != "" {
		t.Fatalf("payload mismatch: %+v", p)
	}
}

func TestStreamToken_CarriesType(t *testing.T) {
	tok := EncodeStreamToken("https://video.sibnet.ru/v/ep1.mp4", "https://video.sibnet.ru/", "mp4", time.Now())
	p, err := DecodeStreamToken(tok, time.Now())
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if p.Type != "mp4" {
		t.Fatalf("Type = %q; want mp4", p.Type)
	}
}

func TestStreamToken_RejectsTamper(t *testing.T) {
	tok := EncodeStreamToken("https://cdn.example.com/x.m3u8", "", "", time.Now())
	// Flip a character in the middle of the token.
	b := []byte(tok)
	mid := len(b) / 2
	if b[mid] == 'A' {
		b[mid] = 'B'
	} else {
		b[mid] = 'A'
	}
	if _, err := DecodeStreamToken(string(b), time.Now()); err == nil {
		t.Fatal("tampered token must not decode")
	}
	if _, err := DecodeStreamToken("garbage!!!", time.Now()); err == nil {
		t.Fatal("garbage must not decode")
	}
}

func TestStreamToken_RejectsExpired(t *testing.T) {
	past := time.Now().Add(-2 * provenanceTTL)
	tok := EncodeStreamToken("https://cdn.example.com/x.m3u8", "", "", past)
	if _, err := DecodeStreamToken(tok, time.Now()); err == nil {
		t.Fatal("expired token must not decode")
	}
}

func TestStreamToken_FailsClosedWhenUnconfigured(t *testing.T) {
	savedSecret, savedConfigured := provenanceSecret, provenanceConfigured
	defer func() {
		provenanceSecret, provenanceConfigured = savedSecret, savedConfigured
		streamTokenAEAD = nil
		streamTokenAEADOnce = sync.Once{}
	}()
	provenanceSecret, provenanceConfigured = nil, false
	streamTokenAEAD = nil
	streamTokenAEADOnce = sync.Once{}
	if tok := EncodeStreamToken("https://cdn.example.com/x.m3u8", "", "", time.Now()); tok != "" {
		t.Fatalf("expected empty token when unconfigured, got %q", tok)
	}
	if _, err := DecodeStreamToken("anything", time.Now()); err == nil {
		t.Fatal("expected decode error when unconfigured")
	}
}

func TestMaskedStreamURL_ShapeAndLeaf(t *testing.T) {
	u := MaskedStreamURL("https://p12.solodcdn.com/s/m/720.mp4:hls:seg-225-v1-a1.ts", "https://kodikplayer.com/", "")
	if !strings.HasPrefix(u, "/api/streaming/m/") {
		t.Fatalf("masked URL prefix wrong: %q", u)
	}
	if strings.Contains(u, "url=") || strings.Contains(u, "solodcdn") {
		t.Fatalf("masked URL leaks upstream shape: %q", u)
	}
	if !strings.HasSuffix(u, ".ts") {
		t.Fatalf("leaf extension lost (player heuristics need it): %q", u)
	}
}

func TestMaskedLeaf(t *testing.T) {
	cases := map[string]string{
		"https://cdn.example.com/a/b/manifest.m3u8": "manifest.m3u8",
		"https://cdn.example.com/":                  "media",
		"://bad":                                    "media",
	}
	for in, want := range cases {
		if got := maskedLeaf(in); got != want {
			t.Errorf("maskedLeaf(%q) = %q; want %q", in, got, want)
		}
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /tmp/ae-privacy-core/libs/videoutils && go test ./... -run 'StreamToken|MaskedLeaf|MaskedStreamURL' -v`
Expected: FAIL — `undefined: EncodeStreamToken` etc.

- [ ] **Step 3: Implement `streamtoken.go`**

```go
// streamtoken.go — opaque AES-GCM stream tokens for the HLS proxy (Track A of
// docs/superpowers/specs/2026-07-10-playback-resilience-constrained-browsers-design.md §3).
//
// Problem: the legacy proxy URL (hls-proxy?url=https%3A%2F%2Fp12.solodcdn…&exp=&sig=)
// embeds the upstream CDN hostname and a `proxy?url=` query shape — a bullseye
// for uBlock-style static network filters, which silently break playback for
// users with hardened browsers (the @gerahertz class of report).
//
// Solution: seal {upstream URL, referer, exp, type} into a single opaque
// AES-256-GCM token carried in the URL PATH (/api/streaming/m/<token>/<leaf>).
// The hostname is unreadable by the client/DOM/filter list; the token is
// unforgeable and tamper-evident (GCM tag); expiry rides inside the sealed
// payload, superseding the separate exp+sig query pair on this path. The key
// derives from the same secret as the provenance HMAC (STREAM_TOKEN_SECRET,
// JWT_SECRET fallback), so catalog-minted tokens open on the streaming service
// exactly like provenance signatures verify today. Fail-closed like
// provenance: no secret → no tokens → callers keep the legacy signed form.
//
// Kill-switch: AE_MASKED_STREAM_DISABLED=1 disables minting (decode keeps
// working so in-flight tokens survive a rollback toggle).
package videoutils

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/ILITA-hub/animeenigma/libs/videoutils/netguard"
)

// StreamTokenPayload is the sealed content of an opaque /m/<token> proxy URL.
type StreamTokenPayload struct {
	URL     string `json:"u"`           // upstream absolute URL (authoritative)
	Referer string `json:"r,omitempty"` // Referer header for the upstream fetch
	Exp     int64  `json:"e"`           // unix-seconds expiry
	Type    string `json:"t,omitempty"` // "mp4"/"webm" content-type override ("" = sniff)
}

var (
	streamTokenAEADOnce sync.Once
	streamTokenAEAD     cipher.AEAD
	maskedMintDisabled  = os.Getenv("AE_MASKED_STREAM_DISABLED") == "1"
)

// streamTokenCipher returns the package AEAD, or nil when no secret is
// configured. provenanceEnabled() is consulted on every call (not inside the
// Once) so tests can toggle provenanceConfigured the same way provenance
// tests do.
func streamTokenCipher() cipher.AEAD {
	if !provenanceEnabled() {
		return nil
	}
	streamTokenAEADOnce.Do(func() {
		key := sha256.Sum256(append([]byte("ae-stream-token-v1\n"), loadProvenanceSecret()...))
		block, err := aes.NewCipher(key[:])
		if err != nil {
			return
		}
		if aead, err := cipher.NewGCM(block); err == nil {
			streamTokenAEAD = aead
		}
	})
	return streamTokenAEAD
}

// EncodeStreamToken seals (rawURL, referer, streamType) into an opaque
// URL-safe token valid for provenanceTTL (12h — must outlive a full VOD watch,
// same rationale as the provenance token). Returns "" when minting is disabled
// (no secret / kill-switch) or the URL fails the SSRF guard — callers then
// fall back to the legacy signed query form.
func EncodeStreamToken(rawURL, referer, streamType string, now time.Time) string {
	if maskedMintDisabled {
		return ""
	}
	aead := streamTokenCipher()
	if aead == nil {
		return ""
	}
	// Mirror signProvenance's SSRF guard: a token authorizes a proxy fetch,
	// so never mint one for a private/loopback/non-http(s) target.
	if !allowLoopbackForTest && netguard.ValidatePublicURL(rawURL) != nil {
		return ""
	}
	payload, err := json.Marshal(StreamTokenPayload{
		URL:     rawURL,
		Referer: referer,
		Exp:     now.Add(provenanceTTL).Unix(),
		Type:    streamType,
	})
	if err != nil {
		return ""
	}
	nonce := make([]byte, aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(aead.Seal(nonce, nonce, payload, nil))
}

// DecodeStreamToken opens a token, validating the GCM tag, expiry, and the
// SSRF guard (defense in depth — mirrors validProvenanceToken).
func DecodeStreamToken(token string, now time.Time) (*StreamTokenPayload, error) {
	aead := streamTokenCipher()
	if aead == nil {
		return nil, errors.New("stream tokens disabled: no secret configured")
	}
	sealed, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(sealed) < aead.NonceSize() {
		return nil, errors.New("malformed stream token")
	}
	plain, err := aead.Open(nil, sealed[:aead.NonceSize()], sealed[aead.NonceSize():], nil)
	if err != nil {
		return nil, errors.New("invalid stream token")
	}
	var p StreamTokenPayload
	if err := json.Unmarshal(plain, &p); err != nil {
		return nil, errors.New("invalid stream token payload")
	}
	if now.Unix() > p.Exp {
		return nil, errors.New("stream token expired")
	}
	if !allowLoopbackForTest && netguard.ValidatePublicURL(p.URL) != nil {
		return nil, errors.New("stream token target not allowed")
	}
	return &p, nil
}

// maskedLeaf returns the last path segment of the upstream URL, kept on the
// masked URL purely so extension-based player heuristics (.m3u8/.ts/.vtt/.key)
// keep working. Cosmetic only — the token is authoritative.
func maskedLeaf(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "media"
	}
	p := u.Path
	if i := strings.LastIndex(p, "/"); i >= 0 {
		p = p[i+1:]
	}
	if p == "" {
		return "media"
	}
	return url.PathEscape(p)
}

// MaskedStreamURL returns the opaque path-token proxy URL for an upstream
// stream/subtitle URL: /api/streaming/m/<token>/<leaf>. Returns "" when the
// token mechanism is disabled — callers keep the legacy signed query form.
func MaskedStreamURL(rawURL, referer, streamType string) string {
	tok := EncodeStreamToken(rawURL, referer, streamType, time.Now())
	if tok == "" {
		return ""
	}
	return "/api/streaming/m/" + tok + "/" + maskedLeaf(rawURL)
}
```

- [ ] **Step 4: Run tests**

Run: `cd /tmp/ae-privacy-core/libs/videoutils && go test ./... -run 'StreamToken|MaskedLeaf|MaskedStreamURL' -v`
Expected: PASS (all 7).

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-privacy-core
git add libs/videoutils/streamtoken.go libs/videoutils/streamtoken_test.go
git commit -m "feat(videoutils): opaque AES-GCM stream tokens (Track A)"
```

---

### Task 2: Preauth proxy entry point

**Files:**
- Modify: `libs/videoutils/proxy.go:799-813` (split `ProxyWithRefererCounted`)
- Test: `libs/videoutils/streamtoken_proxy_test.go` (new)

**Interfaces:**
- Consumes: existing `ProxyWithRefererCounted` body, `isHLSDomainAllowed`, `validProvenanceToken`.
- Produces: `func (p *VideoProxy) ProxyPreauthCounted(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) (uint64, uint64, error)` — Task 4 calls this.

- [ ] **Step 1: Write the failing test**

`libs/videoutils/streamtoken_proxy_test.go` (mirrors `provenance_test.go:120` `TestProxyWithReferer_TokenBypassesAllowlist` — httptest upstream on loopback, allowed by `allowLoopbackForTest`):

```go
package videoutils

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// A preauth call (token already decoded by the handler) must serve a host
// that is NOT in the static allowlist and carries NO exp/sig query params.
func TestProxyPreauthCounted_BypassesAllowlist(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "video/mp2t")
		_, _ = w.Write([]byte("SEGMENTDATA"))
	}))
	defer upstream.Close()

	proxy := NewVideoProxy(DefaultProxyConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/m/sometoken/seg-1.ts", nil)
	rec := httptest.NewRecorder()

	_, _, err := proxy.ProxyPreauthCounted(req.Context(), upstream.URL+"/seg-1.ts", "", rec, req)
	if err != nil {
		t.Fatalf("preauth proxy failed: %v", err)
	}
	if !strings.Contains(rec.Body.String(), "SEGMENTDATA") {
		t.Fatal("upstream body not forwarded")
	}
}

// The plain path must still enforce the gate (no regression).
func TestProxyWithRefererCounted_GateStillEnforced(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("x"))
	}))
	defer upstream.Close()

	proxy := NewVideoProxy(DefaultProxyConfig())
	req := httptest.NewRequest(http.MethodGet, "/api/v1/hls-proxy?url="+upstream.URL, nil)
	rec := httptest.NewRecorder()

	_, _, err := proxy.ProxyWithRefererCounted(req.Context(), upstream.URL+"/x.ts", "", rec, req)
	if err == nil {
		t.Fatal("unsigned non-allowlisted host must be rejected on the legacy path")
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /tmp/ae-privacy-core/libs/videoutils && go test ./... -run 'Preauth|GateStillEnforced' -v`
Expected: FAIL — `undefined: proxy.ProxyPreauthCounted` (second test compiles against existing API but the file won't build).

- [ ] **Step 3: Implement the split**

In `libs/videoutils/proxy.go`, change the existing method at line 799. Rename the existing `func (p *VideoProxy) ProxyWithRefererCounted(...)` to `func (p *VideoProxy) proxyRefererCounted(ctx context.Context, sourceURL, referer string, preauth bool, w http.ResponseWriter, r *http.Request) (bytesIn, bytesOut uint64, _ error)` (keep the entire body, keep its doc comment on the new public wrapper below). Change ONLY the gate (currently lines 810-813):

```go
	// Check if domain is allowed for HLS proxy. A valid provenance token
	// (minted when the proxy rewrote a playlist from an allowlisted origin)
	// authorizes an otherwise-unlisted host — this is how rotating segment
	// CDNs (megaplay/mewstream) are served without a static per-domain entry.
	// preauth=true skips the gate entirely: the caller already authorized the
	// URL by opening a sealed AES-GCM stream token (streamtoken.go), which can
	// only be minted server-side for SSRF-vetted URLs.
	if !preauth && !isHLSDomainAllowed(parsed.Host) &&
		!validProvenanceToken(sourceURL, r.URL.Query().Get("exp"), r.URL.Query().Get("sig"), time.Now()) {
		return 0, 0, &DomainNotAllowedError{Domain: parsed.Host}
	}
```

Then add the two public wrappers directly above `proxyRefererCounted`:

```go
// ProxyWithRefererCounted is ProxyWithReferer that additionally reports the
// per-call bytes_in (upstream resp.Body) and bytes_out (client sink) so the
// streaming handler can Observe(...) them into the per-session HLS tally
// (AR-EGRESS-04/05). bytes_in is counted via a countReader wrapping resp.Body
// before io.Copy/rateLimitedCopy; bytes_out is the copy's write total. For an
// M3U8 rewrite (the manifest path) the counts reflect the rewritten payload.
func (p *VideoProxy) ProxyWithRefererCounted(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) (uint64, uint64, error) {
	return p.proxyRefererCounted(ctx, sourceURL, referer, false, w, r)
}

// ProxyPreauthCounted is ProxyWithRefererCounted for an upstream URL that was
// already authorized by decoding a sealed stream token (streamtoken.go). The
// static-allowlist / provenance-signature gate is skipped — the AES-GCM token
// WAS the authorization. Everything else (SSRF dial guard, edge failover,
// m3u8 rewriting, byte counting) is identical.
func (p *VideoProxy) ProxyPreauthCounted(ctx context.Context, sourceURL, referer string, w http.ResponseWriter, r *http.Request) (uint64, uint64, error) {
	return p.proxyRefererCounted(ctx, sourceURL, referer, true, w, r)
}
```

(Move the original doc comment onto the public wrapper as shown; give the private method a one-liner: `// proxyRefererCounted is the shared pipeline behind ProxyWithRefererCounted and ProxyPreauthCounted.`)

- [ ] **Step 4: Run the full lib test suite**

Run: `cd /tmp/ae-privacy-core/libs/videoutils && go test ./...`
Expected: PASS (whole package — the rename must not break existing callers of `ProxyWithRefererCounted`/`ProxyWithReferer`).

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-privacy-core
git add libs/videoutils/proxy.go libs/videoutils/streamtoken_proxy_test.go
git commit -m "feat(videoutils): ProxyPreauthCounted — token-authorized proxy entry point"
```

---

### Task 3: Rewriter emits the masked form

**Files:**
- Modify: `libs/videoutils/proxy.go:1127-1180` (`rewriteHLSURL`)
- Test: extend `libs/videoutils/streamtoken_test.go`; update `libs/videoutils/proxy_test.go` (`TestSessTokenInjection`) and any `TestRewriteVTTURLs_*` asserting the legacy URL shape

**Interfaces:**
- Consumes: `EncodeStreamToken`, `maskedLeaf` (Task 1).
- Produces: rewritten manifests whose child URLs are `/api/streaming/m/<token>/<leaf>?sess=<hex>` (or legacy form when minting is disabled). Task 4's handler serves them; Task 8's SW recognizes them.

- [ ] **Step 1: Write the failing tests** (append to `streamtoken_test.go`)

```go
func TestRewriteHLSURL_EmitsMaskedForm(t *testing.T) {
	out := rewriteHLSURL("seg-1.ts", "https://cdn.example.com/ep1/", "https://kodikplayer.com/", "abc123")
	if !strings.HasPrefix(out, "/api/streaming/m/") {
		t.Fatalf("expected masked form, got %q", out)
	}
	if strings.Contains(out, "url=") || strings.Contains(out, "cdn.example.com") || strings.Contains(out, "hls-proxy") {
		t.Fatalf("masked child URL leaks legacy shape: %q", out)
	}
	if !strings.HasSuffix(out, "?sess=abc123") {
		t.Fatalf("sess correlation param missing: %q", out)
	}
	// The token must round-trip to the absolute segment URL + referer.
	tok := strings.TrimPrefix(out, "/api/streaming/m/")
	tok = tok[:strings.Index(tok, "/")]
	p, err := DecodeStreamToken(tok, time.Now())
	if err != nil {
		t.Fatalf("emitted token does not decode: %v", err)
	}
	if p.URL != "https://cdn.example.com/ep1/seg-1.ts" {
		t.Fatalf("token URL = %q", p.URL)
	}
	if p.Referer != "https://kodikplayer.com/" {
		t.Fatalf("token referer = %q", p.Referer)
	}
	// Leaf keeps the extension for player heuristics.
	if !strings.Contains(out, "/seg-1.ts?") {
		t.Fatalf("leaf lost: %q", out)
	}
}

func TestRewriteHLSURL_SkipsAlreadyMasked(t *testing.T) {
	in := "/api/streaming/m/sometoken/seg-1.ts?sess=x"
	if out := rewriteHLSURL(in, "https://cdn.example.com/", "", "y"); out != in {
		t.Fatalf("already-masked URL must pass through, got %q", out)
	}
}

func TestRewriteHLSURL_LegacyFallbackWhenDisabled(t *testing.T) {
	savedSecret, savedConfigured := provenanceSecret, provenanceConfigured
	defer func() {
		provenanceSecret, provenanceConfigured = savedSecret, savedConfigured
		streamTokenAEAD = nil
		streamTokenAEADOnce = sync.Once{}
	}()
	provenanceSecret, provenanceConfigured = nil, false
	streamTokenAEAD = nil
	streamTokenAEADOnce = sync.Once{}

	out := rewriteHLSURL("seg-1.ts", "https://cdn.example.com/ep1/", "", "abc")
	if !strings.HasPrefix(out, "/api/streaming/hls-proxy?url=") {
		t.Fatalf("expected legacy fallback when tokens disabled, got %q", out)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /tmp/ae-privacy-core/libs/videoutils && go test ./... -run 'RewriteHLSURL_Emits|RewriteHLSURL_Skips|RewriteHLSURL_Legacy' -v`
Expected: FAIL — masked prefix assertions fail (legacy form emitted).

- [ ] **Step 3: Implement in `rewriteHLSURL`**

Three edits inside `rewriteHLSURL` (proxy.go:1127):

(a) Extend the top skip-check:

```go
	// Skip if already a proxy URL (check both encoded and decoded, and the
	// Track A masked path form)
	if strings.Contains(urlStr, "/api/streaming/hls-proxy") ||
		strings.Contains(urlStr, "%2Fapi%2Fstreaming%2Fhls-proxy") ||
		strings.Contains(urlStr, "hls-proxy") ||
		strings.Contains(urlStr, "/api/streaming/m/") {
		return urlStr
	}
```

(b) Extend the root-relative skip inside the `strings.HasPrefix(urlStr, "/")` branch:

```go
		// Root-relative URL - but skip if it's our proxy path (legacy or masked)
		if strings.HasPrefix(urlStr, "/api/streaming/hls-proxy") ||
			strings.HasPrefix(urlStr, "/api/streaming/m/") {
			return urlStr
		}
```

(c) Immediately after `absoluteURL` is fully computed (before the `// Build proxy URL` comment), insert:

```go
	// Track A: prefer the opaque path-token form — no hostname, no `url=`
	// query shape for a static filter list to match (spec §3). Falls back to
	// the legacy signed query form below when token minting is disabled
	// (no secret / AE_MASKED_STREAM_DISABLED).
	if tok := EncodeStreamToken(absoluteURL, referer, "", time.Now()); tok != "" {
		masked := "/api/streaming/m/" + tok + "/" + maskedLeaf(absoluteURL)
		if sess != "" {
			// Same per-manifest correlation token as the legacy form
			// (AR-EGRESS-04) — observeEgress reads ?sess= regardless of path.
			masked += "?sess=" + sess
		}
		return masked
	}
```

- [ ] **Step 4: Run the full suite and fix shape-asserting tests**

Run: `cd /tmp/ae-privacy-core/libs/videoutils && go test ./...`
`TestSessTokenInjection` (proxy_test.go:19) and any `TestRewriteVTTURLs_*` case that asserts the literal `hls-proxy?url=` / `&sess=` query shape will now fail. Update each failing assertion to the masked equivalent while preserving its invariant:
- "every child of one manifest carries the same sess" → assert every rewritten line matches `\?sess=<token>$` with one shared token value;
- "URL is proxied and signed" → assert prefix `/api/streaming/m/` AND `DecodeStreamToken` of the token segment returns the expected absolute upstream URL (use the extraction pattern from `TestRewriteHLSURL_EmitsMaskedForm` above);
- "#xywh fragment preserved" (VTT) → the fragment now rides after the query: assert the rewritten cue still contains `#xywh=` suffix.
Do NOT weaken any invariant; only translate its expected URL shape.

Expected after updates: PASS (whole package).

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-privacy-core
git add libs/videoutils/proxy.go libs/videoutils/streamtoken_test.go libs/videoutils/proxy_test.go
git commit -m "feat(videoutils): m3u8 rewriter emits opaque masked child URLs"
```
(Also add any other updated `*_test.go` to the same commit.)

---

### Task 4: Streaming service — `MaskedProxy` handler + route

**Files:**
- Modify: `services/streaming/internal/handler/stream.go:261-395` (extract `serveProxy`, add `MaskedProxy`)
- Modify: `services/streaming/internal/transport/router.go:51-52` (add route)

**Interfaces:**
- Consumes: `videoutils.DecodeStreamToken`, `(*VideoProxy).ProxyPreauthCounted` (Tasks 1-2), `chi.URLParam`.
- Produces: `GET/OPTIONS /api/v1/m/{token}/{leaf}` → `StreamHandler.MaskedProxy`. Task 7's gateway route forwards to it.

- [ ] **Step 1: Extract `serveProxy` from `HLSProxy`**

In `stream.go`, restructure `HLSProxy` (line 261): everything from the semaphore acquire (`if !hlsProxySemaphore.TryAcquire(1)`) through the end of the error mapping becomes a new private method. The ONLY behavioral change is selecting the proxy call by `preauth`:

```go
// serveProxy is the shared body of HLSProxy and MaskedProxy: connection
// semaphore, metrics, byte counting, egress folding, and error mapping around
// one videoProxy call. preauth=true selects ProxyPreauthCounted — the caller
// already authorized sourceURL by opening a sealed stream token, so the
// allowlist/provenance gate is skipped.
func (h *StreamHandler) serveProxy(w http.ResponseWriter, r *http.Request, sourceURL, referer string, preauth bool) {
	// ... [moved body: semaphore, proxyType, counters, log, CountingResponseWriter] ...

	proxyCall := h.videoProxy.ProxyWithRefererCounted
	if preauth {
		proxyCall = h.videoProxy.ProxyPreauthCounted
	}
	bytesIn, bytesOut, err := proxyCall(r.Context(), sourceURL, referer, cw, r)

	// ... [moved body: observeEgress + full error mapping, unchanged] ...
}
```

`HLSProxy` shrinks to:

```go
// HLSProxy proxies HLS streams with proper Referer headers
// This endpoint allows the frontend to play HLS streams that require Referer authentication
func (h *StreamHandler) HLSProxy(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range")
		w.WriteHeader(http.StatusOK)
		return
	}

	sourceURL := r.URL.Query().Get("url")
	referer := r.URL.Query().Get("referer")

	if sourceURL == "" {
		httputil.BadRequest(w, "url parameter is required")
		return
	}

	h.serveProxy(w, r, sourceURL, referer, false)
}
```

- [ ] **Step 2: Add `MaskedProxy`**

```go
// MaskedProxy serves the Track A opaque path-token form
// /api/v1/m/<token>/<leaf> (public: /api/streaming/m/...). The sealed AES-GCM
// token carries {url, referer, exp, type} and IS the authorization — no
// allowlist or exp/sig query pair on this path (spec 2026-07-10 §3).
func (h *StreamHandler) MaskedProxy(w http.ResponseWriter, r *http.Request) {
	// Handle CORS preflight (same policy as HLSProxy)
	if r.Method == http.MethodOptions {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Range")
		w.WriteHeader(http.StatusOK)
		return
	}

	payload, err := videoutils.DecodeStreamToken(chi.URLParam(r, "token"), time.Now())
	if err != nil {
		h.log.Warnw("masked proxy: rejected token", "error", err)
		http.Error(w, "invalid stream token", http.StatusForbidden)
		return
	}

	// Metrics/log hygiene: the raw path embeds a high-cardinality token and
	// libs/metrics normalizePath labels r.URL.Path AFTER the handler runs —
	// collapse it to a stable value.
	r.URL.Path = "/api/v1/m"

	// The mp4/webm content-type override rides inside the token on this path;
	// surface it as the query param the shared pipeline already reads
	// (proxy.go's `type` switch).
	if payload.Type != "" {
		q := r.URL.Query()
		q.Set("type", payload.Type)
		r.URL.RawQuery = q.Encode()
	}

	h.serveProxy(w, r, payload.URL, payload.Referer, true)
}
```

Add `"github.com/go-chi/chi/v5"` and (if absent) `"time"` to stream.go's imports.

- [ ] **Step 3: Register the route** — `services/streaming/internal/transport/router.go`, after line 52:

```go
		// Track A opaque path-token proxy (spec 2026-07-10 §3). Public like
		// hls-proxy; the sealed token is the authorization.
		r.Get("/m/{token}/{leaf}", streamHandler.MaskedProxy)
		r.Options("/m/{token}/{leaf}", streamHandler.MaskedProxy) // CORS preflight
```

- [ ] **Step 4: Build + existing tests**

Run: `cd /tmp/ae-privacy-core/services/streaming && go build ./... && go test ./...`
Expected: PASS. (Handler-level httptest for MaskedProxy is intentionally omitted: `StreamHandler` construction drags storage/session wiring; the decode+gate logic is covered by Task 1/2 lib tests and the E2E curl in Task 12.)

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-privacy-core
git add services/streaming/internal/handler/stream.go services/streaming/internal/transport/router.go
git commit -m "feat(streaming): MaskedProxy — serve opaque /m/<token>/<leaf> stream URLs"
```

---

### Task 5: `streamsign` mints `masked_url`

**Files:**
- Modify: `services/catalog/internal/streamsign/streamsign.go`
- Test: `services/catalog/internal/streamsign/streamsign_test.go`

**Interfaces:**
- Consumes: `videoutils.MaskedStreamURL` (Task 1).
- Produces: `func MaskedURL(u, referer, streamType string) string`; `SignScraperStreamBody` additionally stamps `masked_url` on `sources[]` (with the envelope's `headers.Referer` + per-source `type`) and on external `tracks[]` (referer `""` — today's subtitle proxy fetch sends no Referer; keep parity). Task 6 + Task 8 rely on the `masked_url` JSON key.

- [ ] **Step 1: Write the failing test** — append to `streamsign_test.go`:

```go
func TestSignScraperStreamBody_StampsMaskedURL(t *testing.T) {
	body := []byte(`{"success":true,"data":{"stream":{` +
		`"sources":[{"url":"https://cdn.example.com/master.m3u8","type":"hls"},` +
		`{"url":"https://mp4.example.com/ep.mp4","type":"mp4"}],` +
		`"tracks":[{"file":"https://subs.example.com/en.vtt","kind":"subtitles"}],` +
		`"headers":{"Referer":"https://allmanga.to"}},` +
		`"meta":{"provider":"allanime"}}}`)

	out := SignScraperStreamBody(http.StatusOK, body)

	var env map[string]any
	if err := json.Unmarshal(out, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stream := env["data"].(map[string]any)["stream"].(map[string]any)
	sources := stream["sources"].([]any)

	hls := sources[0].(map[string]any)
	masked, _ := hls["masked_url"].(string)
	if !strings.HasPrefix(masked, "/api/streaming/m/") {
		t.Fatalf("hls source masked_url = %q", masked)
	}
	if strings.Contains(masked, "cdn.example.com") {
		t.Fatal("masked_url leaks upstream hostname")
	}

	mp4 := sources[1].(map[string]any)
	if m, _ := mp4["masked_url"].(string); !strings.HasPrefix(m, "/api/streaming/m/") {
		t.Fatalf("mp4 source masked_url = %q", m)
	}

	track := stream["tracks"].([]any)[0].(map[string]any)
	if m, _ := track["masked_url"].(string); !strings.HasPrefix(m, "/api/streaming/m/") {
		t.Fatalf("track masked_url = %q", m)
	}
	// exp/sig legacy pair still stamped (dual-accept).
	if hls["exp"] == nil || hls["sig"] == nil {
		t.Fatal("legacy exp/sig missing — dual-accept broken")
	}
}
```

Add `"strings"` to the test imports. Note: this package's tests need the provenance secret; check whether `streamsign_test.go` has a `TestMain` setting `STREAM_TOKEN_SECRET` — if not, add one:

```go
func TestMain(m *testing.M) {
	if os.Getenv("STREAM_TOKEN_SECRET") == "" {
		os.Setenv("STREAM_TOKEN_SECRET", "test-streamsign-secret-0123456789")
	}
	os.Exit(m.Run())
}
```

(with `"os"` import).

- [ ] **Step 2: Run to verify failure**

Run: `cd /tmp/ae-privacy-core/services/catalog && go test ./internal/streamsign/... -v`
Expected: FAIL — `masked_url` missing.

- [ ] **Step 3: Implement**

In `streamsign.go`: add after `Sign` (line 32):

```go
// MaskedURL returns the Track A opaque path-token proxy URL
// (/api/streaming/m/<token>/<leaf>) for an external stream/subtitle URL, or
// "" for same-origin URLs or when the token mechanism is unconfigured.
// streamType is "mp4" for progressive MP4 (selects the proxy's
// range-passthrough path), "" for HLS/sniffed content.
func MaskedURL(u, referer, streamType string) string {
	if !IsExternal(u) {
		return ""
	}
	return videoutils.MaskedStreamURL(u, referer, streamType)
}
```

Update `SignScraperStreamBody` (replace lines 60-61):

```go
	// The upstream Referer applies to every source fetch; subtitle tracks are
	// fetched WITHOUT a referer today (buildSubtitleProxyUrl passes none), so
	// their masked tokens keep referer "" for behavior parity.
	referer := ""
	if h, ok := stream["headers"].(map[string]any); ok {
		if v, ok := h["Referer"].(string); ok {
			referer = v
		} else if v, ok := h["referer"].(string); ok {
			referer = v
		}
	}

	changed := signArrayField(stream["sources"], "url", referer, true)
	changed = signArrayField(stream["tracks"], "file", "", false) || changed
```

Update `signArrayField` (replace the whole function):

```go
// signArrayField signs the `urlKey` field of each object in a JSON array,
// stamping "exp"/"sig" siblings (legacy dual-accept) plus the Track A
// "masked_url" opaque path form. withType propagates the item's own
// "type" ("mp4"/"webm") into the token so the proxy picks its
// range-passthrough path. Returns whether anything was signed.
func signArrayField(raw any, urlKey, referer string, withType bool) bool {
	arr, ok := raw.([]any)
	if !ok {
		return false
	}
	changed := false
	for _, item := range arr {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		u, ok := m[urlKey].(string)
		if !ok || !IsExternal(u) {
			continue
		}
		exp, sig := videoutils.SignStreamURL(u)
		m["exp"] = exp
		m["sig"] = sig
		streamType := ""
		if withType {
			if tv, ok := m["type"].(string); ok && (tv == "mp4" || tv == "webm") {
				streamType = tv
			}
		}
		if masked := videoutils.MaskedStreamURL(u, referer, streamType); masked != "" {
			m["masked_url"] = masked
		}
		changed = true
	}
	return changed
}
```

- [ ] **Step 4: Run tests**

Run: `cd /tmp/ae-privacy-core/services/catalog && go test ./internal/streamsign/... -v`
Expected: PASS (new + existing).

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-privacy-core
git add services/catalog/internal/streamsign/
git commit -m "feat(catalog): streamsign mints masked_url opaque tokens (dual-accept)"
```

---

### Task 6: Catalog struct mint sites (raw/ae library, storyboard, animejoy)

**Files:**
- Modify: `services/catalog/internal/service/raw_resolver.go:143-215` (RawStream, RawStoryboard, newLibraryStream)
- Modify: `services/catalog/internal/domain/anime.go:300-309` (AnimejoyStream)
- Modify: `services/catalog/internal/service/catalog.go:1934-1944` (GetAnimejoyStream)

**Interfaces:**
- Consumes: `streamsign.MaskedURL` (Task 5).
- Produces: JSON fields `masked_url` on RawStream, RawStream.storyboard, AnimejoyStream — Task 8's FE types read them.

- [ ] **Step 1: RawStream + RawStoryboard fields** — in `raw_resolver.go`, add after the `Sig` field of each struct (lines 151 and 174):

```go
	// Track A opaque path-token form of URL (spec 2026-07-10 §3); preferred
	// by the FE over URL+exp/sig when present.
	MaskedURL string `json:"masked_url,omitempty"`
```

- [ ] **Step 2: Mint in `newLibraryStream`** (raw_resolver.go:197) — extend both constructions:

```go
	s := &RawStream{
		URL:       minioURL,
		Type:      "hls",
		Quality:   quality,
		Subtitles: nil,
		ExpiresAt: time.Now().Add(time.Hour),
		Source:    "library",
		Exp:       exp,
		Sig:       sig,
		MaskedURL: streamsign.MaskedURL(minioURL, "", ""),
	}
	if storyboardURL != "" {
		sbExp, sbSig := streamsign.Sign(storyboardURL)
		s.Storyboard = &RawStoryboard{
			URL:       storyboardURL,
			Exp:       sbExp,
			Sig:       sbSig,
			MaskedURL: streamsign.MaskedURL(storyboardURL, "", ""),
		}
	}
```

- [ ] **Step 3: AnimejoyStream** — in `domain/anime.go`, after the `Sig` field (line 308):

```go
	// Track A opaque path-token form of URL; preferred by the FE when present.
	MaskedURL string `json:"masked_url,omitempty"`
```

In `catalog.go` `GetAnimejoyStream` (line 1934), extend the returned struct:

```go
	exp, sig := streamsign.Sign(resolved.URL)
	return &domain.AnimejoyStream{
		URL:       resolved.URL,
		Type:      "mp4",
		Quality:   resolved.Quality,
		Referer:   resolved.Referer,
		ExpiresAt: time.Now().Add(5 * time.Minute),
		Source:    "animejoy",
		Exp:       exp,
		Sig:       sig,
		MaskedURL: streamsign.MaskedURL(resolved.URL, resolved.Referer, "mp4"),
	}, nil
```

- [ ] **Step 4: Build + tests**

Run: `cd /tmp/ae-privacy-core/services/catalog && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
cd /tmp/ae-privacy-core
git add services/catalog/internal/service/raw_resolver.go services/catalog/internal/domain/anime.go services/catalog/internal/service/catalog.go
git commit -m "feat(catalog): mint masked_url on library/storyboard/animejoy streams"
```

---

### Task 7: Gateway route for `/api/streaming/m/*`

**Files:**
- Modify: `services/gateway/internal/transport/router.go:840-850` (inside `r.Route("/streaming", ...)`)

**Interfaces:**
- Consumes: existing `proxyHandler.ProxyToStreamingBody` / `ProxyToStreaming`; the `/api/streaming/` → `/api/v1/` prefix rewrite in `service/proxy.go:242-244` already covers the new prefix.
- Produces: public `GET/OPTIONS /api/streaming/m/<token>/<leaf>`.

- [ ] **Step 1: Add routes** — after the `r.Options("/hls-proxy", ...)` line (router.go:847):

```go
			// Track A opaque path tokens: /api/streaming/m/<token>/<leaf> →
			// streaming's /api/v1/m/... masked proxy (no url= query shape for
			// filter lists to match; spec 2026-07-10 §3). Body-streaming
			// client, same as hls-proxy.
			r.Get("/m/*", proxyHandler.ProxyToStreamingBody)
			r.Options("/m/*", proxyHandler.ProxyToStreaming)
```

- [ ] **Step 2: Build**

Run: `cd /tmp/ae-privacy-core/services/gateway && go build ./... && go test ./...`
Expected: PASS.

- [ ] **Step 3: Commit**

```bash
cd /tmp/ae-privacy-core
git add services/gateway/internal/transport/router.go
git commit -m "feat(gateway): route /api/streaming/m/* to the masked stream proxy"
```

---

### Task 8: Frontend Track A — prefer `masked_url`, teach the SW the masked shape

**Files:**
- Modify: `frontend/web/src/utils/streaming.ts` (add `maskedStreamUrl`)
- Modify: `frontend/web/src/utils/subtitleProxy.ts` (4th param)
- Modify: `frontend/web/src/composables/aePlayer/useProviderResolver.ts` (types + `buildProxyUrl` + 5 call sites)
- Modify: `frontend/web/src/pwa/segmentCache.ts` (masked shape)
- Test: `frontend/web/src/pwa/segmentCache.spec.ts` (extend); `frontend/web/src/utils/streaming.spec.ts` (extend if it exists, else skip)

**Interfaces:**
- Consumes: backend `masked_url` JSON fields (Tasks 5-6); masked child URLs in manifests (Task 3).
- Produces: `maskedStreamUrl(path: string): string` in `utils/streaming.ts`; `buildSubtitleProxyUrl(file, exp?, sig?, masked?)`.

- [ ] **Step 1: `utils/streaming.ts`** — append:

```ts
/**
 * Roots a backend-minted masked proxy path (`/api/streaming/m/<token>/<leaf>`,
 * Track A opaque stream tokens) at the same HLS host as hlsProxyUrl, so
 * masked traffic rides the dedicated stream subdomain when
 * VITE_HLS_PROXY_BASE is set.
 */
export function maskedStreamUrl(path: string): string {
  const base = (import.meta.env.VITE_HLS_PROXY_BASE || '').replace(/\/+$/, '')
  return `${base}${path}`
}
```

- [ ] **Step 2: `utils/subtitleProxy.ts`** — replace `buildSubtitleProxyUrl`:

```ts
import { hlsProxyUrl, maskedStreamUrl } from '@/utils/streaming'

/** Wrap a subtitle file URL in the signed HLS proxy (CORS + provenance).
 *  SubtitleOverlay fetches any `/`-prefixed url directly, so a pre-signed
 *  proxy url loads an un-allowlisted scraper subtitle CDN without a 502.
 *  When the backend minted a Track A masked path (opaque token — no url=
 *  shape for filter lists), prefer it outright. */
export function buildSubtitleProxyUrl(file: string, exp?: string, sig?: string, masked?: string): string {
  if (masked) return maskedStreamUrl(masked)
  const params = new URLSearchParams()
  params.set('url', file)
  if (exp && sig) {
    params.set('exp', exp)
    params.set('sig', sig)
  }
  return hlsProxyUrl(params.toString())
}
```

- [ ] **Step 3: `useProviderResolver.ts`** — six edits:

(a) `ScraperSource` (line ~66): add after `sig?: string`:
```ts
  // Track A opaque path-token form; preferred over url+exp/sig when present.
  masked_url?: string
```
(b) `ScraperTrack` (line ~76): add after `sig?: string`: `masked_url?: string`
(c) `LibraryStream` (line ~103): add after `sig?: string`: `masked_url?: string`; and change the storyboard line to:
```ts
  storyboard?: { url: string; exp?: string; sig?: string; masked_url?: string }
```
(d) `AnimejoyStreamResp` (line ~174): add after `sig?: string`: `masked_url?: string`
(e) `buildProxyUrl` (line ~438) — extend the sign parameter and short-circuit:

```ts
function buildProxyUrl(
  url: string,
  referer: string,
  streamType?: 'hls' | 'mp4',
  sign?: { exp?: string; sig?: string; masked?: string },
): string {
  // Track A: a backend-minted masked path already seals url+referer+type
  // inside an opaque token — use it as-is (no url= query shape).
  if (sign?.masked) return maskedStreamUrl(sign.masked)
  const params = new URLSearchParams()
  params.set('url', url)
  if (referer) params.set('referer', referer)
  if (streamType === 'mp4') params.set('type', 'mp4')
  // Provenance signature for self-hosted MinIO (first-party / library) URLs:
  // the minio host is NOT in the proxy allowlist, so the master-playlist
  // request must carry exp/sig; the proxy then mints child segment tokens.
  if (sign?.exp && sign?.sig) {
    params.set('exp', sign.exp)
    params.set('sig', sign.sig)
  }
  return hlsProxyUrl(params.toString())
}
```
Add `maskedStreamUrl` to the `@/utils/streaming` import at the top of the file.
(f) Call sites:
- line ~306 (scraper source): `buildProxyUrl(source.url, referer, type, { exp: source.exp, sig: source.sig, masked: source.masked_url })`
- line ~298 (scraper tracks): `buildSubtitleProxyUrl(t.file, t.exp, t.sig, t.masked_url)`
- line ~339 (ae stream): `buildProxyUrl(stream.url, '', type, { exp: stream.exp, sig: stream.sig, masked: stream.masked_url })`
- line ~349 (ae storyboard): add `masked: stream.storyboard.masked_url` to its sign object
- line ~547 (animejoy): `buildProxyUrl(s.url, s.referer ?? '', 'mp4', { exp: s.exp, sig: s.sig, masked: s.masked_url })`

(Kodik `:502`, hanime `:418`, 18anime `:381` stay on the legacy allowlist path — their entry URLs carry no exp/sig today; their child segments are masked by the rewriter. Full entry-point masking for those catalog handlers is a follow-up.)

- [ ] **Step 4: `pwa/segmentCache.ts`** — teach the masked shape. Add below `PROXY_PATH`:

```ts
const MASKED_PREFIX = '/api/streaming/m/'
```

Replace `segmentCacheKey`:

```ts
/** Cache identity of a proxied segment request: the upstream `url` param for
 *  the legacy form; the full token path for the Track A masked form (the
 *  token is opaque — it IS the identity; it stays stable for the lifetime of
 *  one manifest fetch, which covers a VOD watch session).
 *  Returns null for anything that is not a cacheable HLS segment request. */
export function segmentCacheKey(requestUrl: string): string | null {
  try {
    const u = new URL(requestUrl)
    if (u.pathname.includes(MASKED_PREFIX)) {
      // Masked form: /api/streaming/m/<token>/<leaf> — leaf keeps the ext.
      if (!SEG_EXT.test(u.pathname)) return null
      return '/__segcache/?m=' + encodeURIComponent(u.pathname)
    }
    if (!u.pathname.endsWith(PROXY_PATH)) return null
    if (u.searchParams.get('type') === 'mp4') return null
    const upstream = u.searchParams.get('url')
    if (!upstream) return null
    if (!SEG_EXT.test(new URL(upstream).pathname)) return null
    return '/__segcache/?u=' + encodeURIComponent(upstream)
  } catch {
    return null
  }
}
```

In `markScrubUrl`, replace the path check line:

```ts
    if (!u.pathname.endsWith(PROXY_PATH) && !u.pathname.includes(MASKED_PREFIX)) return url
```

- [ ] **Step 5: Extend `segmentCache.spec.ts`** — append (mirror the file's existing style):

```ts
describe('masked (Track A) proxy form', () => {
  const seg = 'https://x.io/api/streaming/m/AbCd123token/seg-5-v1-a1.ts?sess=ff00'
  const manifest = 'https://x.io/api/streaming/m/AbCd123token/manifest.m3u8'

  it('caches masked segments keyed by token path', () => {
    const key = segmentCacheKey(seg)
    expect(key).toBe('/__segcache/?m=' + encodeURIComponent('/api/streaming/m/AbCd123token/seg-5-v1-a1.ts'))
  })

  it('does not cache masked manifests', () => {
    expect(segmentCacheKey(manifest)).toBeNull()
  })

  it('markScrubUrl tags masked URLs', () => {
    const marked = markScrubUrl(seg)
    expect(marked).toContain('aescrub=1')
  })
})
```

(Import `markScrubUrl` in the spec if not already imported.)

- [ ] **Step 6: Verify FE**

Run from `/tmp/ae-privacy-core`: `bin/ae-fe-verify.sh` (derives touched files; runs DS-lint + eslint + build + touched specs).
Also: `cd frontend/web && bunx vue-tsc --noEmit`.
Expected: all green.

- [ ] **Step 7: Commit**

```bash
cd /tmp/ae-privacy-core
git add frontend/web/src/utils/streaming.ts frontend/web/src/utils/subtitleProxy.ts \
        frontend/web/src/composables/aePlayer/useProviderResolver.ts \
        frontend/web/src/pwa/segmentCache.ts frontend/web/src/pwa/segmentCache.spec.ts
git commit -m "feat(web): prefer masked_url opaque stream paths; SW caches masked segments"
```

---

### Task 9: Gateway Track B5 — masked analytics route + hint header

**Files:**
- Create: `services/gateway/internal/handler/masked_analytics.go`
- Test: `services/gateway/internal/handler/masked_analytics_test.go`
- Modify: `services/gateway/internal/transport/router.go` (middleware + route inside `r.Route("/api", ...)` at line 373)

**Interfaces:**
- Consumes: `ProxyHandler.proxy` (same package), `cfg.JWT.Secret` (`authz.JWTConfig.Secret`), chi URL params.
- Produces: `POST /api/{maskedSeg}/{maskedEp}` → analytics; `X-AE-Cfg: /api/<24-hex>` response header on every /api response. Task 10's FE reads both. Handler names: `NewMaskedAnalyticsHandler(proxy *ProxyHandler, secret []byte) *MaskedAnalyticsHandler` with method `Handle`; middleware `MaskedPathHintMiddleware(secret []byte) func(http.Handler) http.Handler`.

- [ ] **Step 1: Write the failing test** — `masked_analytics_test.go`:

```go
package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
)

var testSecret = []byte("test-masked-analytics-secret")

func TestMaskedAnalyticsSegment_DeterministicAndRotating(t *testing.T) {
	now := time.Now()
	bucket := now.Unix() / maskedBucketSeconds
	a := maskedAnalyticsSegment(testSecret, bucket)
	b := maskedAnalyticsSegment(testSecret, bucket)
	if a != b {
		t.Fatal("segment must be deterministic within a bucket")
	}
	if len(a) != 24 {
		t.Fatalf("segment length = %d; want 24", len(a))
	}
	if a == maskedAnalyticsSegment(testSecret, bucket+1) {
		t.Fatal("segment must rotate across buckets")
	}
}

func TestValidMaskedSegment_CurrentAndPrevious(t *testing.T) {
	now := time.Now()
	bucket := now.Unix() / maskedBucketSeconds
	if !validMaskedSegment(testSecret, maskedAnalyticsSegment(testSecret, bucket), now) {
		t.Fatal("current bucket must validate")
	}
	if !validMaskedSegment(testSecret, maskedAnalyticsSegment(testSecret, bucket-1), now) {
		t.Fatal("previous bucket must validate (clock skew / session straddle)")
	}
	if validMaskedSegment(testSecret, maskedAnalyticsSegment(testSecret, bucket-2), now) {
		t.Fatal("stale bucket must be rejected")
	}
	if validMaskedSegment(testSecret, "deadbeefdeadbeefdeadbeef", now) {
		t.Fatal("forged segment must be rejected")
	}
}

func TestCurrentMaskedAnalyticsBase_Shape(t *testing.T) {
	base := CurrentMaskedAnalyticsBase(testSecret, time.Now())
	if len(base) != len("/api/")+24 || base[:5] != "/api/" {
		t.Fatalf("base = %q", base)
	}
}

// Invalid segment / unknown leaf never reach the proxy (h.proxy would nil-panic).
func TestMaskedAnalyticsHandler_RejectsWithoutProxying(t *testing.T) {
	h := NewMaskedAnalyticsHandler(nil, testSecret)
	r := chi.NewRouter()
	r.Post("/api/{maskedSeg}/{maskedEp}", h.Handle)

	for _, path := range []string{
		"/api/deadbeefdeadbeefdeadbeef/c", // forged segment
		"/api/" + maskedAnalyticsSegment(testSecret, time.Now().Unix()/maskedBucketSeconds) + "/zzz", // bad leaf
	} {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d; want 404", path, rec.Code)
		}
	}
}

func TestMaskedPathHintMiddleware_SetsHeader(t *testing.T) {
	mw := MaskedPathHintMiddleware(testSecret)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	rec := httptest.NewRecorder()
	mw(inner).ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/system/status", nil))
	got := rec.Header().Get("X-AE-Cfg")
	if got != CurrentMaskedAnalyticsBase(testSecret, time.Now()) {
		t.Fatalf("X-AE-Cfg = %q", got)
	}
}
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /tmp/ae-privacy-core/services/gateway && go test ./internal/handler/ -run 'Masked' -v`
Expected: FAIL — undefined symbols.

- [ ] **Step 3: Implement `masked_analytics.go`**

```go
package handler

// Track B5 (spec 2026-07-10 §4): rotating HMAC-masked ingestion paths for the
// analytics endpoints. Static filter lists (EasyPrivacy-class) carry
// /api/analytics/* verbatim, silently zeroing telemetry for adblock users —
// exactly the population whose playback failures we most need telemetry from.
// A path segment that is HMAC(secret, hour-bucket) rotates hourly and cannot
// be pinned by a static rule. The gateway hands the SPA the current masked
// base via the X-AE-Cfg response header (MaskedPathHintMiddleware) and
// validates inbound masked posts against the current AND previous bucket
// (clock skew / session straddle) before rewriting to the real analytics
// path. Normal users keep hitting /api/analytics/* directly, keeping the
// masked path low-profile.

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

const maskedBucketSeconds = 3600

// maskedAnalyticsEndpoints maps the short masked leaf to the real analytics
// ingestion path. Single letters — no "collect"/"analytics" bait words.
var maskedAnalyticsEndpoints = map[string]string{
	"c": "/api/analytics/collect",
	"e": "/api/analytics/client-errors",
	"p": "/api/analytics/player-events",
}

// maskedAnalyticsSegment derives the rotating 24-hex path segment for a bucket.
func maskedAnalyticsSegment(secret []byte, bucket int64) string {
	m := hmac.New(sha256.New, secret)
	fmt.Fprintf(m, "ae-analytics-mask\n%d", bucket)
	return hex.EncodeToString(m.Sum(nil))[:24]
}

// CurrentMaskedAnalyticsBase returns "/api/<segment>" for now's bucket.
func CurrentMaskedAnalyticsBase(secret []byte, now time.Time) string {
	return "/api/" + maskedAnalyticsSegment(secret, now.Unix()/maskedBucketSeconds)
}

// validMaskedSegment reports whether seg matches the current or previous
// bucket. Constant-time compares.
func validMaskedSegment(secret []byte, seg string, now time.Time) bool {
	bucket := now.Unix() / maskedBucketSeconds
	for _, b := range []int64{bucket, bucket - 1} {
		want := maskedAnalyticsSegment(secret, b)
		if subtle.ConstantTimeCompare([]byte(want), []byte(seg)) == 1 {
			return true
		}
	}
	return false
}

// MaskedAnalyticsHandler validates and forwards masked ingestion posts.
type MaskedAnalyticsHandler struct {
	proxy  *ProxyHandler
	secret []byte
}

func NewMaskedAnalyticsHandler(proxy *ProxyHandler, secret []byte) *MaskedAnalyticsHandler {
	return &MaskedAnalyticsHandler{proxy: proxy, secret: secret}
}

// Handle validates {maskedSeg}/{maskedEp} and forwards to the analytics
// service under the real path. Rejections rewrite r.URL.Path to a stable
// value: libs/metrics normalizePath labels the RAW first two path segments
// after the handler runs, so an attacker-chosen path would otherwise mint
// unbounded label values.
func (h *MaskedAnalyticsHandler) Handle(w http.ResponseWriter, r *http.Request) {
	seg := chi.URLParam(r, "maskedSeg")
	target, ok := maskedAnalyticsEndpoints[chi.URLParam(r, "maskedEp")]
	if !ok || !validMaskedSegment(h.secret, seg, time.Now()) {
		r.URL.Path = "/api/masked-rejected"
		http.NotFound(w, r)
		return
	}
	// Forward() reads r.URL.Path verbatim for the analytics service (no
	// rewrite case in service/proxy.go) — hand it the real path.
	r.URL.Path = target
	h.proxy.proxy(w, r, "analytics")
}

// MaskedPathHintMiddleware stamps the current masked analytics base onto
// every /api response, so the SPA learns it from any bootstrap call it
// already makes (e.g. /api/policy/features/mine fires unconditionally at app
// start). Recomputed once per bucket, cached under a mutex.
func MaskedPathHintMiddleware(secret []byte) func(http.Handler) http.Handler {
	var mu sync.Mutex
	var cachedBucket int64 = -1
	var cachedBase string
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			bucket := now.Unix() / maskedBucketSeconds
			mu.Lock()
			if bucket != cachedBucket {
				cachedBucket = bucket
				cachedBase = CurrentMaskedAnalyticsBase(secret, now)
			}
			base := cachedBase
			mu.Unlock()
			w.Header().Set("X-AE-Cfg", base)
			next.ServeHTTP(w, r)
		})
	}
}
```

- [ ] **Step 4: Register in `router.go`** — two edits inside `r.Route("/api", func(r chi.Router) {` (line 373):

(a) FIRST line inside the closure (chi requires Use before routes):

```go
		// Track B5: advertise the rotating masked analytics base on every
		// /api response (see handler/masked_analytics.go).
		r.Use(handler.MaskedPathHintMiddleware([]byte(cfg.JWT.Secret)))
```

(b) Directly after the three `r.Post("/analytics/...")` lines (~line 393):

```go
		// Track B5: rotating masked ingestion alias. Param route — chi
		// prefers static siblings, so every existing /api/<service> route
		// wins; only otherwise-unmatched two-segment POSTs land here, and the
		// handler 404s anything without a valid HMAC bucket segment.
		maskedAnalytics := handler.NewMaskedAnalyticsHandler(proxyHandler, []byte(cfg.JWT.Secret))
		r.Post("/{maskedSeg}/{maskedEp}", maskedAnalytics.Handle)
```

(`handler` is already imported; `cfg` is in scope — `sysStatusHandler := handler.NewSystemStatusHandler(cfg)` at line 381 proves both.)

- [ ] **Step 5: Run tests + build (chi route-conflict check happens at router construction)**

Run: `cd /tmp/ae-privacy-core/services/gateway && go test ./... && go build ./...`
Expected: PASS. If any router test constructs the full router it will surface a chi panic on conflict — there must be none (no existing first-level `{param}` route under /api).

- [ ] **Step 6: Commit**

```bash
cd /tmp/ae-privacy-core
git add services/gateway/internal/handler/masked_analytics.go services/gateway/internal/handler/masked_analytics_test.go services/gateway/internal/transport/router.go
git commit -m "feat(gateway): rotating HMAC-masked analytics ingestion path + X-AE-Cfg hint"
```

---

### Task 10: Frontend Track B5 — shared endpoint resolver + wire 3 clients

**Files:**
- Create: `frontend/web/src/utils/analyticsTransport.ts`
- Test: `frontend/web/src/utils/__tests__/analyticsTransport.spec.ts`
- Modify: `frontend/web/src/utils/playerTelemetry.ts` (endpoint + catch)
- Modify: `frontend/web/src/utils/feErrorLog.ts` (endpoint + catch + isOwnTraffic)
- Modify: `frontend/web/src/analytics/transport.ts` (endpoint override + catch)
- Modify: `frontend/web/src/api/client.ts:250-252` (header capture + probe trigger)

**Interfaces:**
- Consumes: `X-AE-Cfg` header (Task 9).
- Produces: `noteMaskedAnalyticsPath(v)`, `maskedOverrideFor(leaf): string | null`, `analyticsEndpoint(leaf): string`, `markBlockedFromError(err): boolean`, `isMaskedAnalyticsUrl(url): boolean`, `probeAnalyticsReachability(): void`, `__resetAnalyticsTransportForTest()`.

- [ ] **Step 1: Write the failing spec** — `frontend/web/src/utils/__tests__/analyticsTransport.spec.ts`:

```ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import {
  noteMaskedAnalyticsPath,
  analyticsEndpoint,
  maskedOverrideFor,
  markBlockedFromError,
  isMaskedAnalyticsUrl,
  probeAnalyticsReachability,
  __resetAnalyticsTransportForTest,
} from '../analyticsTransport'

const MASKED = '/api/0123456789abcdef01234567'

describe('analyticsTransport', () => {
  beforeEach(() => {
    __resetAnalyticsTransportForTest()
    vi.stubGlobal('fetch', vi.fn(() => Promise.resolve(new Response(null, { status: 200 }))))
  })
  afterEach(() => {
    vi.unstubAllGlobals()
  })

  it('defaults to the primary endpoints', () => {
    expect(analyticsEndpoint('collect')).toBe('/api/analytics/collect')
    expect(analyticsEndpoint('player-events')).toBe('/api/analytics/player-events')
    expect(maskedOverrideFor('collect')).toBeNull()
  })

  it('accepts only well-formed masked bases', () => {
    noteMaskedAnalyticsPath('https://evil.example/steal')
    noteMaskedAnalyticsPath('/api/short')
    expect(isMaskedAnalyticsUrl(MASKED + '/c')).toBe(false)
    noteMaskedAnalyticsPath(MASKED)
    expect(isMaskedAnalyticsUrl(MASKED + '/c')).toBe(true)
  })

  it('markBlockedFromError flips only on TypeError with a known masked base', () => {
    noteMaskedAnalyticsPath(MASKED)
    expect(markBlockedFromError(new Error('boom'))).toBe(false)
    expect(analyticsEndpoint('collect')).toBe('/api/analytics/collect')
    expect(markBlockedFromError(new TypeError('Failed to fetch'))).toBe(true)
    expect(analyticsEndpoint('collect')).toBe(`${MASKED}/c`)
    expect(analyticsEndpoint('client-errors')).toBe(`${MASKED}/e`)
    expect(analyticsEndpoint('player-events')).toBe(`${MASKED}/p`)
  })

  it('does not flip without a masked base (fail-open)', () => {
    expect(markBlockedFromError(new TypeError('Failed to fetch'))).toBe(false)
    expect(analyticsEndpoint('collect')).toBe('/api/analytics/collect')
  })

  it('probe fires once, only after a masked base is known, and flips on TypeError', async () => {
    probeAnalyticsReachability() // no masked base yet → no-op
    expect(fetch).not.toHaveBeenCalled()

    noteMaskedAnalyticsPath(MASKED)
    vi.stubGlobal('fetch', vi.fn(() => Promise.reject(new TypeError('Failed to fetch'))))
    probeAnalyticsReachability()
    probeAnalyticsReachability() // second call must be a no-op
    expect(fetch).toHaveBeenCalledTimes(1)
    await Promise.resolve()
    await Promise.resolve()
    expect(analyticsEndpoint('collect')).toBe(`${MASKED}/c`)
  })
})
```

- [ ] **Step 2: Run to verify failure**

Run: `cd /tmp/ae-privacy-core/frontend/web && bunx vitest run src/utils/__tests__/analyticsTransport.spec.ts`
Expected: FAIL — module missing.

- [ ] **Step 3: Implement `analyticsTransport.ts`**

```ts
// Shared analytics endpoint resolver with adblock fallback (Track B5,
// spec docs/superpowers/specs/2026-07-10-playback-resilience-constrained-browsers-design.md §4).
//
// Static filter lists (EasyPrivacy-class) block /api/analytics/* by URL
// shape, silently zeroing telemetry for exactly the users whose playback
// failures we most need to see (the @gerahertz report class: every analytics
// request status 0). The gateway exposes a rotating HMAC-masked alias
// (/api/<hmac-hour-bucket>/{c|e|p}) and advertises the current base via the
// X-AE-Cfg response header on every /api response; the axios client captures
// it here. Detection is a one-shot probe fetch per session — a TypeError
// rejection means the request was blocked client-side before the network —
// plus opportunistic detection on any later fetch-fallback failure. Once
// blocked, all three analytics clients (playerTelemetry, feErrorLog, the
// clickstream Transport) resolve to the masked base for the session.
// Fail-open: with no learned masked base, behavior is identical to before
// this module existed.

export type AnalyticsLeaf = 'collect' | 'client-errors' | 'player-events'

const LEAF_CODE: Record<AnalyticsLeaf, string> = {
  collect: 'c',
  'client-errors': 'e',
  'player-events': 'p',
}

let maskedBase: string | null = null
let blocked = false
let probeFired = false

/** Store the masked base learned from an X-AE-Cfg response header.
 *  Strictly validated — never trust an arbitrary header value as a URL. */
export function noteMaskedAnalyticsPath(value: string | undefined | null): void {
  if (typeof value === 'string' && /^\/api\/[0-9a-f]{24}$/.test(value)) {
    maskedBase = value
  }
}

/** True when url targets the masked alias (feErrorLog self-traffic guard). */
export function isMaskedAnalyticsUrl(url: string): boolean {
  return maskedBase !== null && url.includes(maskedBase)
}

/** Masked override when this session is blocked, else null (callers keep
 *  their primary endpoint). */
export function maskedOverrideFor(leaf: AnalyticsLeaf): string | null {
  if (!blocked || !maskedBase) return null
  const base = (import.meta.env.VITE_API_URL || '/api') as string
  // maskedBase is an absolute /api/<seg> path; keep any non-default origin.
  return `${base.replace(/\/api$/, '')}${maskedBase}/${LEAF_CODE[leaf]}`
}

/** Resolve the endpoint for a leaf: masked when blocked, primary otherwise. */
export function analyticsEndpoint(leaf: AnalyticsLeaf): string {
  const override = maskedOverrideFor(leaf)
  if (override) return override
  const base = (import.meta.env.VITE_API_URL || '/api') as string
  return `${base}/analytics/${leaf}`
}

/** Mark the session blocked from a fetch rejection. TypeError = the request
 *  was blocked client-side before reaching the network (the adblock
 *  signature — an HTTP error status resolves, it never rejects). Returns
 *  whether the session just flipped (caller then retries once, masked). */
export function markBlockedFromError(err: unknown): boolean {
  if (!blocked && err instanceof TypeError && maskedBase !== null) {
    blocked = true
    return true
  }
  return false
}

/** One-shot reachability probe (fired by the axios client once the masked
 *  base is known). An empty batch reaches the collect handler and enqueues
 *  nothing; ANY HTTP response — even a 4xx — proves the URL is not
 *  extension-blocked, so only a TypeError flips the session. */
export function probeAnalyticsReachability(): void {
  if (probeFired || maskedBase === null || typeof fetch === 'undefined') return
  probeFired = true
  const base = (import.meta.env.VITE_API_URL || '/api') as string
  try {
    void fetch(`${base}/analytics/collect`, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: '{"events":[]}',
      keepalive: true,
      credentials: 'include',
    }).catch((err) => {
      markBlockedFromError(err)
    })
  } catch {
    // never throw into callers
  }
}

/** Test-only reset. */
export function __resetAnalyticsTransportForTest(): void {
  maskedBase = null
  blocked = false
  probeFired = false
}
```

- [ ] **Step 4: Run the new spec**

Run: `cd /tmp/ae-privacy-core/frontend/web && bunx vitest run src/utils/__tests__/analyticsTransport.spec.ts`
Expected: PASS.

- [ ] **Step 5: Wire the three clients** (behavior identical while unblocked, so their existing specs stay green):

(a) `utils/playerTelemetry.ts`: add import `import { analyticsEndpoint, markBlockedFromError } from './analyticsTransport'`. Delete the `const ENDPOINT = ...` line (line 29). In `flushPlayerTelemetry`, at the top of the try blocks add `const endpoint = analyticsEndpoint('player-events')` and replace both `ENDPOINT` references with `endpoint`; replace the fetch `.catch(() => undefined)` with:

```ts
      .catch((err) => {
        // Adblock signature → flip this session to the masked alias and
        // retry this batch once there (Track B5).
        if (markBlockedFromError(err)) {
          try {
            void fetch(analyticsEndpoint('player-events'), {
              method: 'POST',
              headers: { 'Content-Type': 'text/plain' },
              body: payload,
              keepalive: true,
              credentials: 'include',
            }).catch(() => undefined)
          } catch {
            // give up silently
          }
        }
      })
```

(b) `utils/feErrorLog.ts`: same pattern — import `{ analyticsEndpoint, markBlockedFromError, isMaskedAnalyticsUrl } from './analyticsTransport'`; delete the `const ENDPOINT` line (line 41); in `flushFeErrors` resolve `const endpoint = analyticsEndpoint('client-errors')`, replace both uses, and upgrade the fetch catch identically (leaf `'client-errors'`). Extend `isOwnTraffic`:

```ts
function isOwnTraffic(url?: string): boolean {
  if (!url) return false
  return (
    url.includes('/analytics/client-errors') ||
    url.includes('/analytics/collect') ||
    isMaskedAnalyticsUrl(url)
  )
}
```

(c) `analytics/transport.ts`: import `{ maskedOverrideFor, markBlockedFromError } from '../utils/analyticsTransport'`. In `send()`, resolve the target first: `const endpoint = maskedOverrideFor('collect') ?? this.endpoint` and use `endpoint` in both the beacon and fetch calls; upgrade the fetch catch:

```ts
    void fetch(endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: payload,
      keepalive: true,
      credentials: 'include',
    }).catch((err) => {
      if (markBlockedFromError(err)) {
        const retry = maskedOverrideFor('collect')
        if (retry) {
          void fetch(retry, {
            method: 'POST',
            headers: { 'Content-Type': 'text/plain' },
            body: payload,
            keepalive: true,
            credentials: 'include',
          }).catch(() => undefined)
        }
      }
    })
```

(d) `api/client.ts` response interceptor (line 250, inside the success arm, next to `maybeBustPrefsCache`):

```ts
    // Track B5: learn the rotating masked analytics base + probe once.
    noteMaskedAnalyticsPath(response.headers['x-ae-cfg'] as string | undefined)
    probeAnalyticsReachability()
```

with import `import { noteMaskedAnalyticsPath, probeAnalyticsReachability } from '@/utils/analyticsTransport'`.

- [ ] **Step 6: Full FE verify**

Run from `/tmp/ae-privacy-core`: `bin/ae-fe-verify.sh` then `cd frontend/web && bunx vitest run && bunx vue-tsc --noEmit`.
Expected: all green — including the three existing client specs unchanged (unblocked behavior is byte-identical).

- [ ] **Step 7: Commit**

```bash
cd /tmp/ae-privacy-core
git add frontend/web/src/utils/analyticsTransport.ts frontend/web/src/utils/__tests__/analyticsTransport.spec.ts \
        frontend/web/src/utils/playerTelemetry.ts frontend/web/src/utils/feErrorLog.ts \
        frontend/web/src/analytics/transport.ts frontend/web/src/api/client.ts
git commit -m "feat(web): masked analytics fallback — shared resolver wired into all 3 clients"
```

---

### Task 11: Full-tree verification in the worktree

- [ ] **Step 1: All Go suites**

```bash
cd /tmp/ae-privacy-core/libs/videoutils && go test ./... -race
cd /tmp/ae-privacy-core/services/catalog && go test ./... && go build ./...
cd /tmp/ae-privacy-core/services/streaming && go test ./... && go build ./...
cd /tmp/ae-privacy-core/services/gateway && go test ./... && go build ./...
cd /tmp/ae-privacy-core/services/scraper && go build ./...
cd /tmp/ae-privacy-core/services/gacha && go build ./...
```
Expected: all PASS (scraper/gacha build-only — they import videoutils).

- [ ] **Step 2: FE gates** — `cd /tmp/ae-privacy-core && bin/ae-fe-verify.sh` → green.

- [ ] **Step 3: /simplify pass** (per after-update convention; 1 agent default) over the branch diff; fold behavior-preserving cleanups; re-run affected tests.

---

### Task 12: Land, host nginx, deploy, E2E, after-update — MAIN SESSION ONLY (not a subagent)

- [ ] **Step 1: Land** — from `/tmp/ae-privacy-core`, use `bin/ae-land.sh` (rebase onto origin/main, push HEAD:main). Commit message for any final squash-batch follows repo style.

- [ ] **Step 2: Host nginx location (BEFORE deploying services)** — edit `/etc/nginx/sites-available/stream.animeenigma.org`, insert before `location / { return 404; }`:

```nginx
    # Track A opaque path tokens (2026-07-11): masked HLS proxy form
    # /api/streaming/m/<token>/<leaf> — same edge treatment as hls-proxy.
    location /api/streaming/m/ {
        limit_req zone=hls burst=120 nodelay;

        if ($request_method = OPTIONS) {
            add_header Access-Control-Allow-Origin "*" always;
            add_header Access-Control-Allow-Methods "GET, OPTIONS" always;
            add_header Access-Control-Allow-Headers "Range" always;
            add_header Access-Control-Max-Age 86400 always;
            add_header Alt-Svc 'h3=":443"; ma=86400' always;
            add_header Content-Length 0;
            return 204;
        }

        proxy_hide_header Access-Control-Allow-Origin;
        add_header Access-Control-Allow-Origin "*" always;
        add_header Access-Control-Allow-Methods "GET, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Range" always;
        add_header Access-Control-Expose-Headers "Content-Length, Content-Range" always;
        add_header Alt-Svc 'h3=":443"; ma=86400' always;

        proxy_pass http://127.0.0.1:8000;
        include snippets/proxy-params.conf;
    }
```

Then: `nginx -t && systemctl reload nginx`. (Host config is NOT in git — this is the documented exception; reloads are graceful.)

- [ ] **Step 3: Deploy in order** — `bin/ae-deploy.sh gateway streaming catalog scraper gacha web` (order matters: gateway route must exist before streaming starts emitting masked manifests; analytics service unchanged — no redeploy).

- [ ] **Step 4: E2E verify with a REAL anime** (memory: test actual anime, not just health):
  1. Resolve a Kodik stream via the site API, fetch its manifest through `/api/streaming/hls-proxy?...` — assert child URLs now match `^/api/streaming/m/[A-Za-z0-9_-]+/.*\.ts\?sess=`.
  2. `curl` one masked child segment (through stream.animeenigma.org if `VITE_HLS_PROXY_BASE` is set — check `docker/.env` / web build args) → HTTP 200 + video bytes; also OPTIONS → 204.
  3. Tamper one char of the token → 403.
  4. `curl -sI https://animeenigma.org/api/system/status | grep -i x-ae-cfg` → `/api/<24hex>`.
  5. `SEG=$(curl -sI https://animeenigma.org/api/system/status | tr -d '\r' | awk -F': ' 'tolower($1)=="x-ae-cfg"{print $2}')` then `curl -s -o /dev/null -w '%{http_code}' -X POST "https://animeenigma.org$SEG/p" -H 'Content-Type: text/plain' -d '{"events":[]}'` → 2xx (NOT 404); a forged segment → 404.
  6. Play an episode in-browser (owner opt-in only, per Chrome-smoke policy) OR verify `library`/`raw` + `animejoy` streams respond with `masked_url` fields via the API.
  7. Watch `make logs-streaming` for `masked proxy: rejected token` noise (should be zero during normal playback).

- [ ] **Step 5: `/animeenigma-after-update`** — changelog entry (user-facing: adblock/privacy-hardened browsers now stream reliably; Trump-mode RU), final commit/push. Update the spec's §3/§4 status lines (Track A + B5 → shipped 2026-07-11; B1-B4 still pending).

- [ ] **Step 6: Memory** — new memory file `project_privacy_core_stream_tokens_masked_analytics.md` (Track A+B5 live; kill-switch `AE_MASKED_STREAM_DISABLED`; leaf cosmetic/token authoritative; masked analytics = `/api/<hmac-hour>/{c,e,p}`, hint header `X-AE-Cfg`; nginx stream vhost got a second location block; B1-B4 deferred; kodik/hanime/18anime ENTRY urls still legacy — follow-up). Update `MEMORY.md` index + the QUIC memory's vhost note.

- [ ] **Step 7: Worktree teardown** (only after after-update is green): `git worktree remove /tmp/ae-privacy-core && git worktree prune && git branch -d feat/privacy-core-tokens`.

---

## Self-review notes (done at plan time)

- **Spec coverage:** §3 token/AEAD/URL-shape/dual-accept → Tasks 1-8; §3 "redeploy all importers" → Task 12; B5 masked path + bucket validation + hint hand-off + shared resolver → Tasks 9-10; §8 Go+FE test strategy → per-task tests. Deliberately out: B1-B4 (user decision), legacy-handler removal (spec defers it), kodik/hanime/18anime/AnimeLib entry-point masking (documented follow-up — their segment volume IS masked via the rewriter).
- **Types:** `EncodeStreamToken(rawURL, referer, streamType string, now time.Time) string` consistent across Tasks 1/3; `MaskedStreamURL(rawURL, referer, streamType string) string` across Tasks 1/5/6; `masked_url` JSON key across Tasks 5/6/8; `analyticsEndpoint/maskedOverrideFor/markBlockedFromError` across Tasks 9/10 (`X-AE-Cfg`, `/api/<24hex>`, leaves c/e/p).
- **Known risks:** (1) chi param-route conflict — none found at /api first level; gateway tests + startup verify. (2) `TestSessTokenInjection`/VTT tests assert legacy shape — Task 3 Step 4 handles explicitly. (3) SW masked cache key rotates per manifest re-fetch — acceptable (VOD manifests fetch once/session; documented in code). (4) Old cached manifests keep legacy child URLs ≤12h — legacy handler untouched, dual-accept.
