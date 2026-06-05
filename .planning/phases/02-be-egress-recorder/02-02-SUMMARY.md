---
phase: 02-be-egress-recorder
plan: 02
subsystem: infra
tags: [egress, tracing, http-client, retrofit, idmapping, kodik, opensubtitles, scraper, provider-tag]

# Dependency graph
requires:
  - phase: 02-be-egress-recorder
    plan: 01
    provides: "libs/tracing recording transport (WrapRecording/WrapTransport), Producer, SetGlobalSink, PII-safe baggage + private provider/user_id ctx helpers"
provides:
  - "idmapping.WithTransport(rt) Option + exported idmapping.NewIPv4Transport() — transport injection without importing the tracing module (leaf stays zero-dep go 1.22)"
  - "kodikextract.ResolveWithClient(ctx, url, *http.Client) + kodikextract.NewRecordingClient(wrap) — client injection point preserving cookie jar + IPv4 dialer (leaf stays zero-dep go 1.22)"
  - "opensubtitles Config.Transport injection field"
  - "CatalogServiceOptions.EgressTransportWrap — catalog threads tracing.WrapTransport into its internal idmapping client + the Kodik extractor"
  - "scraper domain.ProviderContext/ProviderFromContext (private string ctx key, mirrors tracing's provider value the recorder reads) — D-02/D-09 stream-provider tag"
  - "scraper domain.WithProvider(name) Option on BaseHTTPClient; Get/Do thread the tag onto request ctx; WithTransport is now the production recording seam"
  - "All four uninstrumented clients (Kodik, OpenSubtitles, idmapping ARM/AniList, scraper BaseHTTPClient ×7 providers) now route outbound through the recording transport"
affects: [02-03, 02-04, hls-aggregation, grafana-pivot-reports]

# Tech tracking
tech-stack:
  added: []  # no new external packages (RESEARCH §Package Legitimacy Audit; T-02-SC)
  patterns:
    - "Leaf-module transport injection: dependency-free libs (idmapping, kodikextract) expose a WithTransport/Resolve-with-client seam + an exported base-transport builder; the owning service wraps with tracing.WrapTransport and injects — leaf never imports the tracing module (T-02-LEAF)"
    - "Per-provider stream-provider tag baked at construction (RESEARCH A3): each single-provider BaseHTTPClient carries domain.WithProvider(name); Get/Do tag every request ctx so the recorder pivots target = provider + host (D-02/D-09)"
    - "tracing.WrapTransport (not WrapRecording-with-explicit-sink) used at retrofit sites — composes recording only when a process-global sink is installed; nil-safe pass-through until the general-egress plan calls SetGlobalSink at boot (no coupling to a sink owned by the sibling wave-2 plan)"

key-files:
  created:
    - "services/scraper/internal/domain/provider_tag.go"
    - "services/scraper/internal/domain/provider_tag_test.go"
  modified:
    - "libs/idmapping/client.go"
    - "libs/idmapping/client_test.go"
    - "libs/kodikextract/extract.go"
    - "libs/kodikextract/extract_test.go"
    - "services/catalog/internal/parser/opensubtitles/client.go"
    - "services/catalog/internal/parser/opensubtitles/client_test.go"
    - "services/catalog/internal/service/catalog.go"
    - "services/catalog/cmd/catalog-api/main.go"
    - "services/scraper/internal/domain/httpclient.go"
    - "services/scraper/cmd/scraper-api/main.go"

key-decisions:
  - "Leaf transport injection via an exported base-transport builder. idmapping.NewIPv4Transport() + kodikextract.NewRecordingClient(wrap) let the owning service WRAP the IPv4/cookie-jar transport with tracing.WrapTransport and inject it — preserving the IPv4-forced dialer + per-call cookie jar the DDoS-Guard handoff needs — WITHOUT the leaf importing the tracing module. Leaf go directives stay 1.22, no 14-Dockerfile checklist trigger (T-02-LEAF)."
  - "Retrofit sites use tracing.WrapTransport (composes recording iff a global sink is set), NOT WrapRecording with an explicit sink. This keeps plan 02-02 from depending on a Producer/SetGlobalSink that the parallel wave-2 general-egress plan (02-03) owns at boot — the retrofit is nil-safe today and lights up automatically when the sink is installed."
  - "Stream-provider tag delegates to tracing's provider ctx value. The recording RoundTripper reads tracing.ProviderFromContext; domain.ProviderContext writes BOTH tracing's key (so the recorder sees it) AND a scraper-local private string key (so the domain reads it back independently). Provider baked per-construction since each BaseHTTPClient is single-provider (RESEARCH A3)."
  - "Provider tag rides a PRIVATE ctx value, never W3C wire baggage (T-02-PII) — it cannot leak to 3rd-party hosts on outbound requests."

patterns-established:
  - "Pattern: instrument a dependency-free leaf HTTP client without a new import edge — inject a wrapped transport from the owning (already-tracing-aware) service."
  - "Pattern: single-provider HTTP client tags its egress provider at construction; the recorder reads it for provider+host attribution."

requirements-completed: [AR-EGRESS-03]

# Metrics
duration: 12min
completed: 2026-06-05
---

# Phase 02 Plan 02: BE Egress Recorder — Four-Client Retrofit Summary

**The four previously-uninstrumented outbound clients (Kodik extractor, OpenSubtitles, idmapping ARM/AniList, and the scraper BaseHTTPClient across all 7 providers) now route through the recording transport — the leaf modules via injected transports that keep them zero-dependency at go 1.22, and the scraper path additionally carrying a private-ctx stream-provider tag the recorder reads for `target = provider + host`.**

## Performance
- **Tasks:** 2 (both TDD)
- **Files created/modified:** 12
- **Completed:** 2026-06-05

## Accomplishments
- **idmapping** (leaf, go 1.22, zero-dep): added `WithTransport(rt) Option` + exported `NewIPv4Transport()`; `NewClient` is now variadic. Catalog (and the scraper miruro path) wrap the IPv4 transport with `tracing.WrapTransport` and inject it — host-only egress per D-08. No tracing import in the leaf.
- **kodikextract** (leaf, go 1.22, zero-dep): added `ResolveWithClient(ctx, url, *http.Client)` + `NewRecordingClient(wrap)` (builds the cookie-jar + IPv4 client and applies a caller wrap to its transport); `Resolve` is now a thin back-compat wrapper. No tracing import in the leaf.
- **opensubtitles**: added `Config.Transport` injection field; catalog passes `tracing.WrapTransport(nil)`.
- **catalog**: `CatalogServiceOptions.EgressTransportWrap` (set to `tracing.WrapTransport`) threads recording into the service's internal idmapping client + the Kodik extractor; main.go wires the OpenSubtitles + the aggregator idmapping client directly.
- **scraper BaseHTTPClient**: `WithTransport` re-documented as the production recording seam (was "tests only"); new `WithProvider(name)` Option bakes the per-provider tag; `Get`/`Do` thread it onto every request context. Wired at all 7 provider construction sites in scraper main with `tracing.WrapTransport`.
- **scraper stream-provider tag** (`domain/provider_tag.go`): `ProviderContext`/`ProviderFromContext` over a private string key, mirroring tracing's provider ctx value so the recording RoundTripper reads `target = provider + host` (D-02/D-09). General egress carries no provider (D-01).

## Task Commits
1. **Task 1: Leaf-client transport injection (idmapping + kodikextract + opensubtitles, wired from catalog)** — `ecf343a0` (feat; impl+tests co-committed — library-API tests cannot compile pre-implementation, same precedent as 02-01 Task 2)
2. **Task 2: Scraper BaseHTTPClient recording + stream-provider context tag** — `32aafb14` (feat; impl+tests co-committed for the same reason)

_Plan metadata commit (this SUMMARY) follows separately._

## Files Created/Modified
- `libs/idmapping/client.go` + `client_test.go` — `WithTransport` Option, exported `NewIPv4Transport`, variadic `NewClient`; `TestIDMappingTransport`
- `libs/kodikextract/extract.go` + `extract_test.go` — `ResolveWithClient`, `NewRecordingClient`, `Resolve` delegates; `TestKodikExtractTransport`
- `services/catalog/internal/parser/opensubtitles/client.go` + `client_test.go` — `Config.Transport`; `TestOpenSubtitlesTransport`
- `services/catalog/internal/service/catalog.go` — `EgressTransportWrap` option, internal idmapping client wrapped, Kodik resolve uses `ResolveWithClient`
- `services/catalog/cmd/catalog-api/main.go` — wires `tracing.WrapTransport` at OpenSubtitles + idmapping + the service option (3 sites)
- `services/scraper/internal/domain/provider_tag.go` + `provider_tag_test.go` — provider tag; `TestProviderTagContext`, `TestBaseHTTPClientTransport`
- `services/scraper/internal/domain/httpclient.go` — `WithProvider` Option, `Get`/`Do` ctx tagging, `WithTransport` production doc
- `services/scraper/cmd/scraper-api/main.go` — `egressTransport` shared var; `WithTransport` + `WithProvider` at 7 provider sites; miruro ARM idmapping client wrapped

## Decisions Made
- **Leaf transport injection (T-02-LEAF).** Both leaf modules stay zero-require at `go 1.22` (verified) — they expose injection seams + an exported base-transport builder so the owning service wraps with the tracing module and injects. No tracing import in the leaf → no go-directive bump → no 14-Dockerfile checklist trigger (RESEARCH §Pitfall 1).
- **`tracing.WrapTransport` over an explicit sink at retrofit sites.** Composes recording only when `SetGlobalSink` is installed (the general-egress wave-2 plan's job at boot); nil-safe today. This deliberately avoids coupling plan 02-02 to a Producer/sink owned by the parallel plan 02-03, preventing a merge collision.
- **Provider tag delegates to tracing's ctx value.** `domain.ProviderContext` writes tracing's private provider key (read by the recording RoundTripper) AND a scraper-local private string key (read back by `domain.ProviderFromContext`). Tag baked per-construction since each `BaseHTTPClient` is single-provider (RESEARCH A3). Private ctx value, never wire baggage (T-02-PII).
- **Scoped completeness add (Rule 2).** The scraper's miruro provider builds its own `idmapping.NewClient()` for ARM/AniList resolution — wrapped it with the recording transport too, since the plan's must-have truth covers idmapping (ARM/AniList) egress recording. Trivial, same one-line pattern as the catalog path.

## Deviations from Plan
- **TDD commit shape (both tasks):** impl + tests co-committed per task rather than split RED→GREEN. The transport-injection tests exercise new library APIs (`WithTransport`, `ResolveWithClient`/`NewRecordingClient`, `Config.Transport`, `WithProvider`, `ProviderContext`) that cannot compile before the implementation exists — identical to plan 02-01 Task 2's accepted shape for pure helper APIs. RED was observed locally (tests written against the new API, run after impl, green). Not a functional deviation.
- **[Rule 2 - Missing critical functionality] miruro ARM idmapping client recording-wrapped** in scraper main.go — the plan's files list named `scraper-api/main.go` and the must-have truth covers idmapping egress recording; the scraper's own ARM client was an in-scope gap. One-line wrap matching the catalog path.

No other deviations — the four-client retrofit, leaf injection seams, provider tag, and per-provider wiring match the plan's design.

## TDD Gate Compliance
- Task 1: feat with co-committed tests (`ecf343a0`) — three transport-injection tests (`TestIDMappingTransport`, `TestKodikExtractTransport`, `TestOpenSubtitlesTransport`) all green.
- Task 2: feat with co-committed tests (`32aafb14`) — `TestProviderTagContext` + `TestBaseHTTPClientTransport` green.
- Library-API tests cannot compile pre-implementation; co-commit is the accepted shape for this phase (precedent: 02-01 Task 2).

## Known Stubs
None. All four clients route through real injection seams. `tracing.WrapTransport` is intentionally a nil-safe pass-through until the general-egress plan installs the process-global sink at BE boot — by design, not a stub (the recording lights up automatically once `SetGlobalSink` is called).

## User Setup Required
None — no external service configuration. Egress recording activates when the BE services install the process-global sink (general-egress wave-2 plan).

## Next Phase Readiness
- AR-EGRESS-03 closed: all four previously-uninstrumented clients are on the recording transport.
- 02-03 (general-egress wiring) installs `SetGlobalSink(producer)` at catalog/scraper boot, at which point these retrofitted clients begin emitting effect rows with zero further per-client edits.
- 02-04 (PII strip + end-to-end `TestNoUserIDOnOutboundWire`) can assert the provider tag stays on private ctx (never baggage) — the foundation is the `tracing.WithProvider` private-key delegation used here.

---
*Phase: 02-be-egress-recorder*
*Completed: 2026-06-05*

## Self-Check: PASSED
- Created files present: `provider_tag.go`, `provider_tag_test.go`, `02-02-SUMMARY.md`.
- Task commits present: `ecf343a0` (Task 1), `32aafb14` (Task 2).
- All four clients build + transport-injection tests green; leaf libs verified zero tracing import at go 1.22 (`grep -c 'libs/tracing' == 0`); catalog + scraper `go build ./...` clean; `go vet` clean.
