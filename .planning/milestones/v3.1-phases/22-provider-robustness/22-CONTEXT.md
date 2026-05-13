# Phase 22: Provider Robustness — Context

**Gathered:** 2026-05-13
**Status:** Ready for planning (`/gsd-plan-phase --phase 22`)
**Milestone:** v3.1 Scraper Self-Healing
**Spec:** `docs/plans/2026-05-13-scraper-self-healing-spec.md`
**Depends on:** Phase 21 complete (streamprobe gate exists)

<domain>
## Phase Boundary

When a single CDN behind a server fails (signed-URL expired, 403, geo-block), the orchestrator transparently tries that server's secondary URL family before giving up on the server. Catches the failure mode "the regex still works but the URL doesn't" — distinct from Phase 21's "the server is dead".

**Concretely, this phase delivers:**

1. `services/scraper/internal/embeds/streamhg.go` extracts BOTH the `hls2` (signed `.m3u8` at `premilkyway.com`) AND the `hls3` (unsigned `.txt` at `managementadvisory.sbs`) URLs from the unpacked packed-JS dictionary. Returns multi-source `Stream{Sources: [{hls2}, {hls3}]}`.
2. `services/scraper/internal/embeds/earnvids.go` does the same: BOTH `hls2` (`dramiyos-cdn.com`) and `hls3` (`exoplanethunting.space`).
3. `libs/videoutils/proxy.go` `HLSProxyAllowedDomains` adds `managementadvisory.sbs` and `exoplanethunting.space` so the streaming service will actually proxy the `hls3` URLs. Without this, `hls3` 403s at the proxy and `hls2` is the only viable URL.
4. Phase 21's `gogoanime.GetStream` per-server fallback already iterates `Stream.Sources` in order via the playability gate — no orchestrator changes needed in this phase.
5. `docs/issues/README.md` gains an inline `ISS-011: VibePlayer Ad-Decoy Poisoning` entry documenting PoC 2026-05-13 findings: signature, IP-level root cause, fix (Phase 21 server-priority deprioritization), residual risk (still poisoned if WARP not configured later).

**Out of scope (deferred):**
- VibePlayer multi-URL extraction — VibePlayer only exposes one m3u8 URL family (per PoC unpack); nothing to multi-extract.
- WARP egress for VibePlayer — separate phase, separate spec.
- Detecting `hls3` URL rotation (e.g., `managementadvisory.sbs` itself gets blocked) — caught by Phase 23 canary, fixed via maintenance Pattern 7 button-fix.

**Requirements covered:**
- SCRAPER-HEAL-09 (multi-URL streamhg/earnvids)
- SCRAPER-HEAL-10 (HLS proxy allowlist additions)
- SCRAPER-HEAL-11 (ISS-011 inline entry)

</domain>

<decisions>
## Implementation Decisions

### D1 — Both URLs returned as separate `Stream.Sources`, not as a fallback wrapper

Reason: Phase 21's per-server fallback already iterates `[]Sources` and gates each. Reusing that loop is free; introducing a per-source fallback wrapper would duplicate state. Each source carries its own `Headers` (Referer differs between hls2 and hls3 CDNs in some cases — verify in plan).

### D2 — Allowlist addition lives in `libs/videoutils/proxy.go` as plain string literals

Reason: matches existing convention (`vibeplayer.site`, `premilkyway.com`, etc. are inline literals in `HLSProxyAllowedDomains`). The maintenance bot's Pattern 7 fix-path expects to find allowlist additions in this file — keeping the convention means the bot's `Edit` action targets a known location.

### D3 — ISS-011 entry goes inline in `docs/issues/README.md`, not as a separate file

Reason: the project's incident convention (verified by reading `docs/issues/README.md` and CLAUDE.md memory entry "Issues & Incidents Documentation") is inline numbering — ISS-001 through ISS-010 all live inline. Keep the convention.

### D4 — ISS-011 is appended at the bottom of "Active Issues", NOT marked resolved

Reason: the Phase 21 fix mitigates the symptom (users no longer see ads) but the root cause (IP-level poisoning) is unresolved — VibePlayer still serves decoys to our IP. Closing ISS-011 as "Resolved" would misrepresent the state. Status: `Mitigated`. Closed-state moves to "Resolved" only after WARP recovery (future phase).

</decisions>

<open_questions>
None.
</open_questions>

<risks>
## Risks specific to this phase

- **`hls3` CDN host has additional path/query requirements we missed in PoC**: PoC verified `managementadvisory.sbs/UuPRIY08TwydO/hls3/.../master.txt` returns real video, but Referer / Origin / User-Agent requirements may differ from `hls2`. Mitigation: capture per-source `Headers` in the extractor; have the plan-checker verify the streamprobe gate exercises both URL families end-to-end with the captured headers.
- **Packed-JS dictionary structure changes again**: the `hls2` / `hls3` JSON keys may rotate to `streamA` / `streamB` etc. Mitigation: extract ALL `https?://[^"]+\.(m3u8|txt)[^"]*` URLs from the unpacked string and dedupe/order by URL-prefix heuristic. Logs a warning if the count differs from 2 (the canary will catch any silent breakage anyway).
- **Allowlist gets stale**: the new hosts may themselves rotate within weeks. Caught by Phase 23 canary + Pattern 7 fix-path; no in-phase mitigation.
</risks>
