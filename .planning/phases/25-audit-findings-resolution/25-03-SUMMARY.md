---
phase: 25-audit-findings-resolution
plan: 03
status: completed
requirement: SCRAPER-HEAL-24
audit_finding: W-INT-03
date: 2026-05-19
---

# Plan 25-03 Summary — Silent-200 → typed 502 on HLS proxy domain-not-allowed

## What changed

### `libs/videoutils/proxy.go`

Added typed sentinel error and switched the allowlist gate:

```go
type DomainNotAllowedError struct {
    Domain string
}

func (e *DomainNotAllowedError) Error() string {
    return fmt.Sprintf("domain not allowed for HLS proxy: %s", e.Domain)
}
```

`ProxyWithReferer` now returns `&DomainNotAllowedError{Domain: parsed.Host}`
instead of `fmt.Errorf(...)` — preserves the same error string (so any
log-grep monitoring keeps matching) while letting downstream handlers
use `errors.As` for typed dispatch.

### `services/streaming/internal/handler/stream.go::HLSProxy`

Added `errors.As` branch for `*videoutils.DomainNotAllowedError`:

- Emits `metrics.ProxyUpstreamErrors.WithLabelValues("403", domain).Inc()`
  (so Pattern 7 dispatcher routes it like a real 403_upstream signal).
- Logs `domain` + `url` at Warn level.
- `http.Error(w, "domain not allowed for HLS proxy", http.StatusBadGateway)`.

Also added a belt-and-braces 502 fallback at the end of the generic
error path — replaces the misleading "Don't send error response if
headers already sent" comment and ensures no error path silently
commits a 200/0 response.

### `services/streaming/internal/handler/stream_test.go` (new)

Created `TestHLSProxy_DomainNotAllowed_Returns502` — constructs
`StreamHandler` via struct literal (HLSProxy doesn't use
`streamingService`), points it at the real `HLSProxyAllowedDomains`
list, fires an `httptest.NewRecorder` with a deliberately
non-allowlisted URL, asserts:

- `rec.Code == http.StatusBadGateway` (502)
- body contains `"domain not allowed"` (case-insensitive)
- body is non-empty (locks the Content-Length:0 regression)

## Verification logs

### `libs/videoutils` build + test:

```
$ cd libs/videoutils && go build ./... && go test ./... -count=1
ok  	github.com/ILITA-hub/animeenigma/libs/videoutils	0.006s
```

### `services/streaming` build + test:

```
$ cd services/streaming && go test ./... -count=1
ok  	github.com/ILITA-hub/animeenigma/services/streaming/internal/handler	0.010s
```

Including the new unit test:

```
=== RUN   TestHLSProxy_DomainNotAllowed_Returns502
--- PASS: TestHLSProxy_DomainNotAllowed_Returns502 (0.00s)
PASS
```

### `make redeploy-streaming` (2026-05-19 07:25 UTC):

```
[INFO] Stopping streaming...
[INFO] Removing streaming container...
[INFO] Starting streaming...
[INFO] streaming is running
[INFO] Deployment complete!
[INFO] Checking service health...
[INFO] streaming:8082 - healthy
```

### Live curl smoke — non-allowlisted host (the bug we fixed):

```
$ curl -s -o /tmp/hls_smoke_body.txt -w "HTTP_CODE=%{http_code}\nCONTENT_LENGTH=%{size_download}\n" \
    "http://localhost:8000/api/streaming/hls-proxy?url=https%3A%2F%2Fdefinitely-not-allowed-host.invalid%2Fmaster.m3u8"
HTTP_CODE=502
CONTENT_LENGTH=33
$ cat /tmp/hls_smoke_body.txt
domain not allowed for HLS proxy
```

**Expected 502 + non-empty body — bug is fixed.** Previously this
returned HTTP 200 / Content-Length:0.

### Live curl smoke — allowlisted host happy path (no regression):

```
$ curl -s -o /tmp/hls_smoke_allow.txt -w "HTTP_CODE=%{http_code}\nCONTENT_LENGTH=%{size_download}\n" \
    "http://localhost:8000/api/streaming/hls-proxy?url=https%3A%2F%2Fkwik.cx%2Fno-such-master.m3u8"
HTTP_CODE=502
CONTENT_LENGTH=28
$ head -c 200 /tmp/hls_smoke_allow.txt
upstream stream unavailable
```

Allowlisted host → 502 with `"upstream stream unavailable"` (the
existing `UpstreamError` path, not our new path). No regression.

## Diff snapshot

```
libs/videoutils/proxy.go                          | 15 +++++++++++++--
services/streaming/internal/handler/stream.go     | 20 +++++++++++++++++++-
services/streaming/internal/handler/stream_test.go | 56 +++++++++++++++++++++++
3 files changed, 88 insertions(+), 3 deletions(-)
```

## Anchor

W-INT-03 (Phase 25 milestone audit, 2026-05-13): HLSProxy returned
HTTP 200 / Content-Length:0 on allowlist-rejected URLs, hiding the
failure from FE error boundaries, Prometheus dashboards, and the
BLK-INT-01 canary self-heal loop. Now emits 502 with a descriptive
body. SCRAPER-HEAL-24 closed.
