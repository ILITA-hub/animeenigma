# HLS Proxy Allow-list — Provenance & Review Process

This document describes the process for managing
`HLSProxyAllowedDomainsWithProvenance` in `libs/videoutils/proxy.go`.

## Why this list matters

The HLS proxy allow-list controls which upstream hosts the streaming service
(`services/streaming`) and the scraper URL validator
(`services/scraper`) will trust. A host appearing in this list means:

- The streaming service will fetch HLS segments / MP4 chunks from that host
  with our internal credentials and stream the bytes back to user clients.
- The scraper handler will accept stream URLs from that host without 403.

Adding a malicious or compromised host gives an attacker the ability to
serve content that user clients treat as same-origin media. A misjudgement
here is a meaningful security regression even though no allow-list entry
gives the attacker code execution by itself.

## Where the list lives

The canonical list is `HLSProxyAllowedDomainsWithProvenance` (a slice of
`AllowedDomain` structs) in `libs/videoutils/proxy.go`. The flat string view
`HLSProxyAllowedDomains []string` is derived from it at package init time so
existing callers keep working unchanged.

```go
type AllowedDomain struct {
    Domain string  // host pattern (exact, eTLD+1, or "prefix-*")
    Reason string  // short note: what is this for?
    Owner  string  // "@github-handle" of the person vouching for the entry
    Added  string  // YYYY-MM-DD
}
```

## Adding a new entry

1. Edit `HLSProxyAllowedDomainsWithProvenance` in `libs/videoutils/proxy.go`.
2. Provide honest `Owner` (your GitHub handle) and `Added` (today's date).
3. Write a `Reason` that future maintainers can verify — name the upstream
   provider and which scraper/proxy path needs the host.
4. Open a PR. Per `.github/CODEOWNERS`, the security owner
   (`@StaticVirtualObserver`) is automatically requested as a reviewer.
5. CI runs `go test ./libs/videoutils/...` which verifies provenance is
   populated and the list-view still matches the struct projection.

Do **not**:

- Add a host without provenance fields (the test
  `TestHLSProxyAllowedDomainsWithProvenance_HasNonEmptyMetadata` will fail).
- Bypass the CODEOWNERS gate (branch protection must be enabled in repo
  settings for this to enforce — see "Limitations" below).
- Add overly broad eTLD+1 patterns when an exact host or wildcard prefix
  would do. The `strings.HasSuffix(host, "."+allowed)` matcher in
  `isHLSDomainAllowed` already accepts every subdomain.

## Quarterly review

Run the audit script and rotate any stale entries:

```bash
scripts/audit-hls-allowlist.sh                # full table
scripts/audit-hls-allowlist.sh -legacy-only   # @legacy-owned (backfill candidates)
scripts/audit-hls-allowlist.sh -format=csv    # for spreadsheets
```

The current owner schedules a quarterly review (Q1/Q2/Q3/Q4) and asks each
entry's `Owner` to confirm:

- The host is still actively used by a scraper provider or streaming flow.
- The host has not been transferred or hijacked since `Added`.
- The `Reason` still describes reality.

Entries that fail confirmation are removed in the same PR. Entries owned by
`@legacy` (assigned during the structured-provenance refactor on 2026-05-20
because per-entry history was not recoverable from git) MUST be backfilled
to a real owner at the next review, or dropped if no one will vouch.

## Audit script details

The script is `scripts/audit-hls-allowlist.sh` — a thin Bash wrapper that
shells to `go run ./libs/videoutils/cmd/audit-allowlist`. The Go program
imports `libs/videoutils` and iterates the struct slice in process, so the
output is robust against:

- Source-file reformatting (`gofmt`)
- Entry reordering or grouping
- Comment changes
- Wildcard syntax (`*.example.com`, `htv-*`)

A naive `git diff origin/main libs/videoutils/proxy.go` is intentionally
**not** the audit mechanism — it produces false positives on every
unrelated edit to `proxy.go`.

Supported output formats: `-format=table` (default, human-readable),
`-format=tsv` (TAB-separated, scriptable), `-format=csv` (RFC 4180 minimal).

## Cross-references

- HLS proxy implementation: `libs/videoutils/proxy.go`
- Streaming caller: `services/streaming/internal/handler/stream.go:38`
- Scraper validator: `services/scraper/internal/handler/scraper_test.go:37`
- CODEOWNERS gate: `.github/CODEOWNERS`
- Audit script: `scripts/audit-hls-allowlist.sh`
- Audit binary: `libs/videoutils/cmd/audit-allowlist/main.go`

## Limitations

- CODEOWNERS only **suggests** reviewers unless branch protection's
  "Require review from Code Owners" rule is enabled on the protected
  branch. This file documents the intent; enforcement is a separate
  repository-setting change.
- The `Owner` field is text — it is not authenticated against the
  committer. A bad actor with commit access can still write any owner
  they want. The actual review gate is the PR review itself, not the
  field value.
- Wildcard entries (`htv-*`) cover an unbounded family of subdomains.
  Treat new wildcards with extra scrutiny — they expand the trust
  surface more than a single host.

## Audit trail (this document)

| Date       | Change                                                                                                                |
|------------|------------------------------------------------------------------------------------------------------------------------|
| 2026-05-20 | Initial structured-provenance refactor (WV3-T2 of `docs/plans/2026-05-20-audit-wave3-security.md`). 41 entries imported with `Owner=@legacy` pending quarterly backfill. |
