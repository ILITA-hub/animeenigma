---
phase: 02-be-egress-recorder
reviewed: 2026-06-05T07:26:57Z
depth: standard
files_reviewed: 25
files_reviewed_list:
  - libs/idmapping/client.go
  - libs/kodikextract/extract.go
  - libs/tracing/baggage.go
  - libs/tracing/client.go
  - libs/tracing/effect.go
  - libs/tracing/middleware.go
  - libs/tracing/producer.go
  - libs/videoutils/proxy.go
  - services/analytics/cmd/analytics-api/main.go
  - services/analytics/internal/domain/event.go
  - services/analytics/internal/handler/effects.go
  - services/analytics/internal/repo/clickhouse_store.go
  - services/analytics/internal/transport/router.go
  - services/catalog/cmd/catalog-api/main.go
  - services/catalog/internal/parser/opensubtitles/client.go
  - services/catalog/internal/service/catalog.go
  - services/catalog/internal/transport/router.go
  - services/scraper/cmd/scraper-api/main.go
  - services/scraper/internal/domain/httpclient.go
  - services/scraper/internal/domain/provider_tag.go
  - services/scraper/internal/transport/router.go
  - services/streaming/cmd/streaming-api/main.go
  - services/streaming/internal/handler/stream.go
  - services/streaming/internal/service/hls_sessions.go
  - services/streaming/internal/transport/router.go
findings:
  critical: 1
  warning: 7
  info: 5
  total: 13
status: issues_found
---

# Phase 02: Code Review Report

**Reviewed:** 2026-06-05T07:26:57Z
**Depth:** standard
**Files Reviewed:** 25
**Status:** issues_found

## Summary

Reviewed the BE egress-recorder phase against the four high-risk areas called out in
the brief: the baggage-PII boundary, the async drop-on-full producer, the HLS
session-tally map, and the recording RoundTripper.

The **baggage-PII boundary is sound**. `user_id` and `provider` ride private,
non-propagated `context.WithValue` keys (`libs/tracing/baggage.go`,
`services/scraper/internal/domain/provider_tag.go`); only `origin`/`operation` enter
W3C baggage. The recording transport additionally strips any stray `user_id` baggage
member *before* the otelhttp inner transport runs (`client.go:104-122`), so injection
order is correct. The producer's own analytics POST uses an unwrapped default transport
(`producer.go:79`), so there is no record-the-recorder infinite loop. The
drop-on-full channel is genuinely non-blocking (`producer.go:94-101`), and the HLS
tally mutex is never held across a network copy — byte counts are measured by the
caller and folded in under a few map ops (`hls_sessions.go:123-143`).

The one BLOCKER is a memory-leak / effect-loss class of bug in the recording
RoundTripper: the single Effect is emitted only on `resp.Body.Close()`, and several
hot-path callers in this codebase never close the body on their error paths, so those
egress rows are silently lost AND the underlying connection leaks. Remaining findings
are attribution-fidelity gaps and robustness/quality issues.

## Critical Issues

### CR-01: Recording transport emits the effect ONLY on `Body.Close()` — callers that skip Close on error leak the connection and drop the effect

**File:** `libs/tracing/client.go:156-192`
**Issue:** On the success path `recordingTransport.RoundTrip` replaces `resp.Body`
with a `countingBody` whose `onClose` callback is the *only* place the Effect is
emitted (`client.go:159-164`, `183-192`). If a caller obtains a 2xx response but never
calls `Body.Close()`, the effect is never recorded **and** the HTTP connection is never
returned to the pool (a classic Go response-body leak). This is not hypothetical for
the clients wired in this phase:

- `libs/kodikextract/extract.go` is careful (`resp.Body.Close()` on both hops), but
  the gogoanime/animefever/miruro/nineanime providers route through the
  `BaseHTTPClient` and any provider code path that returns early on a non-2xx-but-
  no-error response (e.g. checks `resp.StatusCode` then `return ... ErrNotFound`
  without `defer resp.Body.Close()`) drops the effect and leaks the socket.
- The contract comment at `client.go:73-78` ("emits the effect on Close") makes the
  emit point load-bearing on caller discipline, which the rest of the codebase does
  not uniformly guarantee.

Because the effect is the entire deliverable of this phase, a missed Close is both a
correctness bug (lost egress row → undercount) and a resource bug (FD/conn leak under
load).
**Fix:** Make the emit independent of caller Close discipline. Either (a) also fire
`onClose` from `Read` when the underlying read returns a terminal error (`io.EOF` or
non-nil err) so a fully-read-but-not-closed body still records, or (b) emit the effect
eagerly in `RoundTrip` for responses with a known short/zero `ContentLength`, or (c)
document and *enforce* via a linter that every `BaseHTTPClient`/wrapped-client call
site `defer`s `Close`. Minimal version of (a):

```go
func (c *countingBody) Read(p []byte) (int, error) {
	n, err := c.rc.Read(p)
	c.n += n
	if err != nil && err != io.ErrNoProgress { // terminal read (EOF or hard error)
		c.fireOnce()
	}
	return n, err
}

func (c *countingBody) Close() error {
	err := c.rc.Close()
	c.fireOnce()
	return err
}

func (c *countingBody) fireOnce() {
	if !c.closed {
		c.closed = true
		if c.onClose != nil {
			c.onClose(c.n)
		}
	}
}
```
Note this still can't fix the *connection* leak (only Close returns the conn), so pair
it with a call-site audit / `defer Close` enforcement for the BaseHTTPClient consumers.

## Warnings

### WR-01: idmapping discards the inbound request context — egress effects lose origin/operation/user_id AND trace linkage

**File:** `libs/idmapping/client.go:206, 269` (and call sites
`services/catalog/internal/service/catalog.go:2152, 2168`)
**Issue:** `ResolveByShikimoriID`/`ResolveByMALID` take no `context.Context`;
`resolveARM`/`resolveAniList` build their own with `context.WithTimeout(context.
Background(), …)`. When the wrapped recording transport runs on that request, the
context carries no seeded baggage, no private `user_id`, and no parent trace span. The
resulting egress effect rows therefore have empty `origin`/`operation`/`user_id`, and
the otelhttp inner transport injects a *fresh* (orphan) trace context rather than
linking to the inbound catalog/scraper request. The phase doc states idmapping egress
is "host-only (D-08)", which excuses the empty operation, but the **lost trace linkage**
and the inability to attribute the call to the originating request is a fidelity
regression that contradicts the AR-EGRESS-01/02 "attribute egress to the inbound
request that caused it" goal.
**Fix:** Thread `ctx` through the public entry points:
`ResolveByShikimoriID(ctx, id)` → `resolveMAL(ctx, id)` →
`context.WithTimeout(ctx, armTimeout)`. Update catalog call sites to pass the request
context. OpenSubtitles already does this correctly (`opensubtitles/client.go` takes
`ctx` and `subs_aggregator.go:249,417` threads the request ctx) — idmapping is the
outlier.

### WR-02: HLS `Mint` is called on every segment GET, not once per manifest — wasted lock contention and a misleading contract

**File:** `services/streaming/internal/handler/stream.go:294-301`,
`services/streaming/internal/service/hls_sessions.go:85-116`
**Issue:** `observeEgress` calls `h.hlsSessions.Mint(...)` followed by `Observe(...)`
for **every** proxied segment that carries a `?sess=` token. `Mint`'s own doc
(`hls_sessions.go:85-90`) says it captures attribution "at the moment the manifest is
proxied" and is the "only point where the inbound attribution is available" — but the
manifest fetch carries no `?sess=` (the handler explicitly skips it at
`stream.go:285-288`), so `Mint` in fact never sees the manifest request and runs once
per segment instead. Each call takes `s.mu.Lock()` and walks the backfill branch. Under
a 5MB/s HLS stream this is hundreds of redundant locked map lookups per session. It is
not a correctness bug (backfill is idempotent), but the contract comment is wrong and
the per-segment Mint is wasteful.
**Fix:** Fold the backfill into `Observe` (it already creates the tally) and drop the
separate per-segment `Mint` call, or guard `Mint` so it only runs when the tally is
freshly created. Update the `Mint` doc to reflect that segment-context first-touch — not
manifest fetch — is the actual attribution source (the code comment at
`stream.go:276-280` already half-admits this; make the two consistent).

### WR-03: Reaper holds `s.mu` while emitting every idle/all session — a slow/contended sink stalls all HLS accounting

**File:** `services/streaming/internal/service/hls_sessions.go:191-211`
**Issue:** `flushIdle` and `flushAll` hold `s.mu` for the entire map scan while calling
`recordLocked` → `s.sink.Record` inside the loop. The design leans on the contract
"sink.Record is non-blocking" (`hls_sessions.go:164-166`). That holds for the current
`Producer`, but it is a fragile invariant: if a future sink (or a test fake, or a
`Producer` whose channel send is momentarily scheduled out under contention) takes any
time, `Observe`/`Mint` on the request hot path block on `s.mu`, coupling playback
latency to the analytics sink. The brief explicitly flags "mutex must never be held
across a network copy" — Record is not a network copy today, but the lock-across-sink
pattern is one refactor away from violating it.
**Fix:** Collect the to-flush tallies into a local slice under the lock, delete them
from the map, release the lock, then call `sink.Record` on the local slice outside the
critical section:

```go
func (s *HLSSessions) flushIdle(now time.Time) {
	type kv struct{ k sessKey; t *sessionTally }
	var due []kv
	s.mu.Lock()
	for k, t := range s.sessions {
		if now.Sub(t.lastSeen) >= s.idleWindow {
			due = append(due, kv{k, t})
			delete(s.sessions, k)
		}
	}
	s.mu.Unlock()
	for _, d := range due {
		s.record(d.k, d.t) // unlocked
	}
}
```

### WR-04: `int(t.bytesIn)` / `int(t.bytesOut)` truncate uint64 byte tallies to platform int — long sessions silently overflow

**File:** `services/streaming/internal/service/hls_sessions.go:181-182`,
`libs/tracing/client.go:124-127, 145`, `services/analytics/internal/repo/clickhouse_store.go:144-146`
**Issue:** `sessionTally.bytesIn/bytesOut` are `uint64` (correct — a 45s-idle-window
session can accumulate many GB across segments), but `recordLocked` casts them to the
`int`-typed `Effect.BytesIn/BytesOut` (`Effect` measures are `int`, `effect.go:20-23`).
The producer wire struct is also `int` (`producer.go:56-59`). On a 64-bit host this is
fine in practice, but the type narrowing is lossy by contract and the analytics store
then re-widens to `uint64`/`uint32` (`clickhouse_store.go:144-146`) — a round-trip
through `int` that can sign-flip a >9.2EB (theoretical) or, more realistically, a
negative-looking value if any intermediate `int` is 32-bit. `DurationMS` has the same
`int(duration.Milliseconds())` narrowing.
**Fix:** Make `Effect.BytesIn/BytesOut` (and the wire struct) `int64`/`uint64` to match
the upstream `uint64` tallies and the downstream `UInt64` columns, removing the lossy
middle hop. At minimum add a saturating guard so a value exceeding `math.MaxInt32`
clamps rather than wraps when built on a 32-bit target.

### WR-05: `effectsDropped` Prometheus counter is process-global but `Producer.dropped` is per-instance — double-counting / mislabeled drops with >1 producer

**File:** `libs/tracing/producer.go:18-21, 94-104`
**Issue:** `effectsDropped` is a package-level `promauto.NewCounter` with no labels,
incremented by *every* `Producer.Record` drop process-wide, while `Producer.dropped`
(returned by `Dropped()`) is per-instance. Today each service runs exactly one
`Producer`, so they agree. But the HLS aggregator (`streaming`) and the general egress
producer are both `tracing.Producer` instances, and `streaming/cmd/.../main.go` happens
to share one (`effectProducer` is passed to both `SetGlobalSink` and `NewHLSSessions`)
— so it works by luck. If any service ever constructs two producers (e.g. a dedicated
streaming-egress producer plus the HLS one), the global counter conflates their drops
with no way to attribute which buffer overflowed, and the per-instance `Dropped()` no
longer matches `/metrics`. The comment "safe across multiple producers (they share the
counter)" understates the observability loss.
**Fix:** Add a `producer` (or `sink`) label to `effectsDropped`, or document+enforce
the one-producer-per-process invariant. Given the codebase trend toward multiple sinks
(general + HLS), a label is the safer choice.

### WR-06: `evictIfFullLocked` is O(n) on every insert once the map saturates — a distinct-token flood degrades to O(n) per segment under the lock

**File:** `services/streaming/internal/service/hls_sessions.go:147-162`
**Issue:** When `len(sessions) >= maxEntries` (10k default), every new
`Mint`/`Observe` for an unseen `(sess, host)` triggers a full linear scan to find the
oldest entry — under `s.mu`. The brief flags the map as a DoS surface ("bounded against
unbounded growth"). It *is* bounded (good — no OOM), but an attacker who can mint
distinct `?sess=`/host pairs (the token is `crypto/rand`, not user-controlled, but the
*host* portion of `sessKey` comes from the proxied `url` query param, which the client
controls within the allowlist + provenance gate) can hold the map at capacity and force
an O(10k) scan inside the request-path lock on every segment. That converts the intended
DoS *bound* into a CPU-amplification under contention.
**Fix:** This is partly out of v1 perf scope, but the lock-held O(n) eviction is a
correctness-adjacent robustness gap. Track approximate LRU with a heap or a
`container/list` so eviction is O(log n)/O(1), or evict in batches outside the
hot-path lock. At minimum, document the host-portion-of-key amplification and confirm
the allowlist/provenance gate caps distinct hosts in practice.

### WR-07: `nineanime` provider constructs a second `MegaplayExtractor` that bypasses the shared egress-recording transport

**File:** `services/scraper/cmd/scraper-api/main.go:99-101, 413-419`,
`services/scraper/internal/embeds/megaplay.go:93-98`
**Issue:** `NewMegaplayExtractor()` builds its own `&http.Client{Timeout: …}` with the
**default** (unwrapped) transport (`megaplay.go:94-97`). The shared instance registered
at `main.go:99` and the per-nineanime instance at `main.go:418` therefore make upstream
HTTP calls (the 1anime.site wrapper fetch, the megaplay.buzz player fetch, and the
`getSources` XHR — all real third-party egress) that are **never recorded** as egress
effects, unlike every `BaseHTTPClient`-routed provider call which goes through
`egressTransport`. This is an egress-accounting blind spot introduced precisely in the
phase whose goal is "one effect per outbound request". The Kodik extractor solved the
identical leaf-module problem with `NewRecordingClient(wrap)` — Megaplay did not get the
same treatment.
**Fix:** Give `MegaplayExtractor` a transport-wrap injection seam mirroring
`kodikextract.NewRecordingClient` (a `func(base http.RoundTripper) http.RoundTripper`
option), and have `scraper-api/main.go` pass `tracing.WrapTransport` so its three
outbound hops emit effects. Keep the leaf module tracing-free by injecting the wrapper
from main, as kodikextract does.

## Info

### IN-01: `wireProducerEffect` JSON contract is a strict subset of `wireEffect` — `anime_id`/`row_count` silently unsendable

**File:** `libs/tracing/producer.go:48-60` vs
`services/analytics/internal/handler/effects.go:38-52`
**Issue:** The producer's wire struct omits `anime_id` and `row_count` that the
analytics handler accepts. For egress effects these are legitimately always empty/zero,
so nothing breaks today, but the two structs are a hand-maintained contract with no
shared definition or round-trip test guarding drift. A future db/cache effect kind that
needs `row_count` would require editing both sides in lockstep with nothing to catch a
miss.
**Fix:** Extract the wire effect shape into a shared package (or add a contract test in
`effects_test.go` that marshals the producer struct and asserts the handler decodes
every field it cares about).

### IN-02: `omitempty` on `Status`/`BytesIn`/etc. is harmless but conflates "zero" with "absent" on the wire

**File:** `libs/tracing/producer.go:48-60`
**Issue:** `Status int json:"status,omitempty"` drops `status:0` from the JSON for the
error/transport-failure path (where status is deliberately 0). The handler defaults the
missing field back to 0, so the value survives — but `omitempty` on measures means an
error effect and a "field genuinely absent" effect are indistinguishable on the wire.
Low impact; flagged for clarity since status 0 is a meaningful sentinel here.
**Fix:** Drop `omitempty` on the numeric measure/dimension fields, or document that
absent == 0 is intentional.

### IN-03: `getCorrectHLSContentType` keyword match on "key"/"enc" can misclassify legitimate segment paths

**File:** `libs/videoutils/proxy.go:853-856`
**Issue:** Any path containing the substring `key` or `enc` is forced to
`application/octet-stream` (intended for AES key files). A segment path like
`/encoded/seg001.ts` or `/monkey/…` would be mislabeled. Pre-existing behavior, not
introduced this phase, but adjacent to the touched bytes-counting code.
**Fix:** Anchor on the actual key-file extension/name (`.key`, `/key?`) rather than a
loose `Contains`.

### IN-04: `rateLimitedCopy` dead branch `nw < 0 || nr < nw`

**File:** `libs/videoutils/proxy.go:906-911`
**Issue:** `io.Writer.Write` is contractually forbidden from returning `n < 0` or
`n > len(p)`, so the defensive `nw < 0 || nr < nw` branch is effectively unreachable
with a conforming writer. Harmless, but dead-code-adjacent and copied from the stdlib
`io.Copy` internals. Pre-existing.
**Fix:** Leave as defensive copy or trim; no action required.

### IN-05: `host` used as both `Host` and `Target` on every egress effect

**File:** `libs/tracing/client.go:140-141`
**Issue:** The recording transport sets `Host: host` and `Target: host` identically for
egress, and the producer then re-derives `target = e.Target | e.Host`
(`producer.go:155-158`). The `Provider`-pivot (`target = provider + host`) described in
`provider_tag.go:18-20` and `httpclient.go:92-96` is **not** actually applied in
`client.go` — `Target` is plain host, and `Provider` is carried as a separate field.
Whether downstream analytics composes `provider + host` for the pivot is outside the
reviewed files; flagging that the "target = provider + host" contract is asserted in
comments/tests but the RoundTripper itself stores them separately.
**Fix:** Confirm the analytics query layer composes the pivot from the separate
`provider`+`target` columns; if a pre-composed `target` was intended, build it in
`recordingTransport.build`.

---

_Reviewed: 2026-06-05T07:26:57Z_
_Reviewer: Claude (gsd-code-reviewer)_
_Depth: standard_
