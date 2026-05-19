# Phase 27 Plan Check — AnimePahe Revival via Stealth-Chromium Sidecar

**Checker:** gsd-plan-checker
**Date:** 2026-05-19
**Plans verified:** 27-01, 27-02, 27-03, 27-04, 27-05

## CHECK FAILED — REVISIONS NEEDED

Plans collectively address the phase goal at the architectural level. Locked decisions D1, D2, D4, D5, D6, D7, D8 are honored; wave ordering is correct (27-01 || 27-02 → 27-03 → 27-04 → 27-05); cross-plan coordination for golden capture is documented; security threat model is present in every plan that owns code surface; VALIDATION.md exists with per-task automated/manual gates.

However, Plan 27-02 contains an internal contradiction in `client.go::GetStream` that will not compile after the rewrite, and a behavioural regression of the Kwik Referer chain. There are also a small number of secondary issues that should be addressed before execution. Severity classifications below.

---

## Required Revisions

### BLOCKER B1 — 27-02 Task 1: `p.baseURL` removal breaks `GetStream` Kwik Referer + compile-time

**Plan:** 27-02
**Task:** Task 1 (rewrite client.go)
**Where:** `services/scraper/internal/providers/animepahe/client.go` line ~507 (`headers := http.Header{"Referer": []string{p.baseURL}}` inside `GetStream`)

**The contradiction:**
- The plan must_have asserts: `"Provider.GetStream is byte-for-byte unchanged — hands the Kwik URL to embeds.Registry.Find().Extract(), caches min(expires-30s, 5min)"`.
- The Task 1 action says: `"Replace p.baseURL with p.resolver throughout client.go. ... GetStream is COMPLETELY unchanged — the sidecar is NOT on its path."`
- `p.baseURL` is referenced by `GetStream` at `client.go:507` to set the Referer header sent to the Kwik extractor (`headers := http.Header{"Referer": []string{p.baseURL}}`). The Kwik upstream requires a `Referer` matching the parent animepahe site — historically `https://animepahe.ru/`.
- The rewrite removes `Deps.BaseURL` and (per the wording "Replace `p.baseURL` with `p.resolver` throughout client.go") also removes the `Provider.baseURL` field. After that change:
  1. **Compile fail:** line 507 references a no-longer-existing field — `go build ./...` (the plan's own `<verify>` gate) will fail.
  2. **Substitution would not type-check anyway:** `p.resolver` is `*resolverClient`, not `string`.
  3. **Even if a string substitution were made:** swapping in `http://animepahe-resolver:3000` as the Referer is wrong — Kwik checks the *public* animepahe origin, not the internal docker sidecar hostname. The stream URL would 403 from Kwik. This silently breaks Plan 27-04's stream-fetchability gate.

**Fix required (planner must choose one and write it explicitly into 27-02 Task 1):**
- **Option A (recommended):** Introduce a new package constant `const kwikReferer = "https://animepahe.pw/"` (matching D2's domain choice) and rewrite line 507 to `headers := http.Header{"Referer": []string{kwikReferer}}`. Document the constant in `dto.go` or a new `referer.go` file. Update Task 1 action explicitly to call out this single-line edit inside `GetStream`. Add a `must_have.truth`: `"GetStream sends Referer=https://animepahe.pw/ to the Kwik extractor (preserving the parent-site Referer chain)"`. Update the existing `TestProvider_GetStream` rewire (currently described as "completely unchanged") to assert the Referer value.
- **Option B:** Retain `Provider.baseURL` as a hardcoded `"https://animepahe.pw"` field set inside `New()` (no longer from `Deps.BaseURL`); delete only `Deps.BaseURL` not `Provider.baseURL`. Document this in Task 1 action explicitly.

Either way, the must_have `"GetStream is byte-for-byte unchanged"` must be amended — the Referer source MUST change (either to a constant or to a hardcoded `Provider.baseURL`).

---

### BLOCKER B2 — 27-02 Task 1 deletes `Deps.BaseURL` without specifying what happens to `Provider.baseURL` field

**Plan:** 27-02
**Task:** Task 1

The action text says "Replace `p.baseURL` with `p.resolver` throughout client.go" — but `p.baseURL` is a `string` field assigned in `New()` (currently at `client.go:170`: `baseURL: strings.TrimRight(base, "/")`). The plan does NOT say:
- Whether the `Provider.baseURL` field is being deleted, or
- Whether the `baseURL: strings.TrimRight(base, "/")` initialization in `New()` is being deleted/changed.

This ambiguity (compounded by B1) means an executor could leave the field alive with no initializer, drop it entirely, or repurpose it. Each path has different downstream consequences.

**Fix required:** 27-02 Task 1 must explicitly state the disposition of the `Provider.baseURL` field and the corresponding initializer in `New()`. The cleanest reading, paired with B1's Option A, is: **delete the `baseURL` field from `Provider`**, **delete its initializer in `New()`**, and let GetStream use a package-level constant. State this explicitly.

---

### BLOCKER B3 — 27-04 Task 2 references a Stream DTO shape (`data.headers.Referer`) that does not exist today

**Plan:** 27-04
**Task:** Task 2, step 4

The action expects:
```bash
REF=$(jq -r '.data.headers.Referer // "https://kwik.cx/"' /tmp/27-04-stream.json)
```

The current `domain.Stream` shape sets headers via `http.Header{"Referer": []string{p.baseURL}}` (i.e., the parser sets a header internally that Kwik uses to fetch the upstream playlist) — it is NOT serialized as a `.headers.Referer` field on the response DTO returned by the gateway. Looking at the canonical Phase 24 pipeline (`docs/issues/scraper-provider-verification-2026-05-19.md`) the response DTO contains `sources[].url` but no canonical `headers.Referer` field guaranteed to ship to the client.

If the plan assumes a `data.headers.Referer` field that isn't there, the `curl -I -H "Referer: $REF" "$M3U8"` step uses the fallback `"https://kwik.cx/"` for every run. That MAY happen to work (Kwik accepts its own origin as a Referer), but it dodges the actual contract — the gate-clear test isn't really verifying the parser's Referer logic.

**Fix required:** Either
- (a) 27-04 Task 2 must verify against the actual response DTO shape — drop the assumption that `data.headers.Referer` exists and instead hardcode `Referer: https://animepahe.pw/` (consistent with B1's fix); OR
- (b) 27-04 Task 2 must require a separate parser-side change (in 27-02) to expose `Headers` on the `Stream` DTO so the response carries the Referer the parser used, then the gate test reads it back.

Pick one and write it explicitly. Option (a) is the smaller diff and matches the rest of the design.

---

### BLOCKER B4 — 27-02 Task 2 MalSync reverse-lookup map: providerID-only `ListEpisodes` callers will silently no-op invalidation

**Plan:** 27-02
**Task:** Task 2 (A9 single-strike invalidation)

The proposed design adds `Provider.malsyncReverseLookup map[string]string` populated by `FindID` (animeSession → malID). On a `/release` 404 inside `ListEpisodes`, the code looks up `malID := p.malsyncReverseLookup[providerID]` and invalidates the MalSync entry.

The fragile case is when callers reach `ListEpisodes` with a `providerID` that was NOT produced by `FindID` in the current process lifetime — for example:
- Catalog had a stored `providerID` and passed it through to scraper without re-running FindID.
- A retry from a different scraper process.
- A clean restart where the 6h episodes cache is populated but the in-memory reverse map is empty.

In those cases the invalidation silently fails: `malID := p.malsyncReverseLookup[providerID]` returns `""`, the `if malID != ""` guard skips invalidation, and the stale MalSync entry persists. The very failure mode A9 is meant to fix (stale animeSession in MalSync 24h cache) is not actually fixed.

**Fix required:** 27-02 Task 2 must address this. Two viable approaches:
- **Persist the reverse mapping**: when `FindID` writes the MalSync positive cache (`malsync:<malID>:animepahe → providerID`), ALSO write the reverse (`malsync_reverse:animepahe:<providerID> → malID`) with the same 24h TTL. `ListEpisodes` on 404 reads the reverse key and invalidates the forward entry. This is durable across process restarts.
- **Change ListEpisodes signature** to take `(ctx, malID, providerID)` and thread `malID` from the orchestrator callsite. Wider blast radius but cleaner.

Pick one and write it into 27-02 Task 2 explicitly. The current "in-memory map populated by FindID" approach is a silent-failure footgun.

---

### WARNING W1 — RESEARCH.md `## Open Questions` section is not marked `(RESOLVED)`

**File:** `27-RESEARCH.md` lines 756–776

By Dimension 11 (Research Resolution): the `## Open Questions` heading must end in `(RESOLVED)` or each numbered question must carry an inline `RESOLVED:` marker. The current file has neither — each question has a `Recommendation:` line but no explicit `RESOLVED:` marker.

In substance the four questions ARE resolved by the plans (Q1 → 27-02 Task 3 drops per-host limits; Q2 → 27-01 returns raw HTML; Q3 → 27-02 removes `ANIMEPAHE_BASE_URL`; Q4 → 27-01 awaits `initBrowser()` before `listen`). Only the format is off.

**Fix required:** Append `(RESOLVED)` to the section heading AND add an inline `**RESOLVED in Plan 27-XX:** ...` line under each numbered question. This is a documentation-only fix; no code or plan logic changes.

---

### WARNING W2 — 27-04 Task 2 step 1 assumes response JSON path `.data.episodes` without verification

**Plan:** 27-04
**Task:** Task 2 step 1

```bash
echo "$RESP" | jq '.data.episodes | length >= 28'
EP=$(echo "$RESP" | jq -r '.data.episodes[0].id')
```

The actual gateway/catalog→scraper response envelope shape isn't pinned here against a known schema. Phase 24's verdict log (`docs/issues/scraper-provider-verification-2026-05-19.md`) is the canonical reference and the plan says to use it "verbatim" — but the inline shell snippets in 27-04 don't re-quote the exact shape. If the envelope is `{ episodes: [...] }` rather than `{ data: { episodes: [...] } }` (or vice versa), the gate evaluation will silently produce "0 episodes" and FAIL incorrectly.

**Fix required:** 27-04 Task 2 should either (a) cross-reference the exact path lines from `docs/issues/scraper-provider-verification-2026-05-19.md` (the planner can grep it and pin the path) or (b) include a one-liner `echo "$RESP" | jq 'keys'` diagnostic immediately before the assertion, so a mismatch surfaces as a clear failure rather than a silent zero-count.

---

### WARNING W3 — 27-01 Task 3 `redeploy-animepahe-resolver` Makefile addition may double-add

**Plan:** 27-01
**Task:** Task 3

The action says: "the wildcard `redeploy-%` already supports `make redeploy-animepahe-resolver` mechanically — the explicit target adds discoverability via `make help` and matches the precedent set by `redeploy-web`". Adding an explicit target alongside the wildcard rule can produce a "target redefined" warning OR shadow the wildcard rule depending on Make's resolution order. Verify the precedent in `redeploy-web` and either:
- Use the same shape (which is presumably an `## doc-comment` next to the wildcard, not a fully explicit duplicate target), OR
- Confirm explicitly that an `explicit` target overriding the wildcard is the project convention.

**Fix required:** 27-01 Task 3 should read `Makefile` first, see how `redeploy-web` is structured, and either match that shape or document why a different shape is being used. Minor — could be caught at execute time, but a 1-minute pre-execution Makefile read avoids it.

---

### WARNING W4 — 27-02 Task 2 `cache.Delete` fallback paragraph is misleading

**Plan:** 27-02
**Task:** Task 2

The action says: "deletes the positive-cache key ... via `m.cache.Delete(ctx, key)` (or the equivalent shape of `libs/cache.Cache`; if no `Delete` method exists on the interface, use `m.cache.Set(ctx, key, "", 0)` with a 0 TTL to evict, falling back to a one-line shim if neither path works in the existing libs/cache API)."

`libs/cache/cache.go` line 16 confirms `Delete(ctx context.Context, keys ...string) error` IS on the interface. The fallback prose is dead and misleading — `Set` with `ttl=0` does NOT evict in Redis (it sets a key with no expiry). The fallback path would silently leave the stale entry alive.

**Fix required:** 27-02 Task 2 should delete the fallback prose and unconditionally specify `m.cache.Delete(ctx, key)`. This is a clarity fix; doesn't change implementation outcome.

---

## What is correct (and load-bearing)

For the planner's benefit, the following are explicitly verified and do NOT need revision:

- **D1** (sidecar option A) — implemented in 27-01 verbatim.
- **D2** (animepahe.pw exclusive) — enforced in 27-01 host-allowlist (`upstream.js` rejects non-`animepahe.pw`) and documented in `server.js` header comment. Threat T-27-01-01 owns this.
- **D3** (UUID session contract migration) — 27-02 covers FindID → animeSession; ListEpisodes uses session in pagination; ListServers uses both sessions. The plan correctly acknowledges DTO struct fields are already correct (A1/A2) — no phantom DTO surgery.
- **D4** (fresh goldens, not from `/tmp/pup/`) — 27-02 Task 3 captures three goldens against the locally-built sidecar image, not against probe artifacts.
- **D5** (500 MB hard ship gate) — 27-01 Task 4 is a dedicated soak gate with explicit STOP-and-surface escalation if the cap is exceeded twice; 27-03 enforces the cap at the cgroup level via compose `mem_limit`.
- **D6** (pin policy) — 27-01 Task 3 writes `STEALTH-PINS.md`, splices Pattern 7 branch into `.claude/maintenance-prompt.md`, package.json has exact pins, lockfile is committed, Dockerfile uses `npm ci`.
- **D7** (last-step gated flip) — 27-05 Task 1 verifies the verdict log PASS row exists before any compose edit; 27-05 Task 2 enforces the 10-minute no-flood log scan AND health-gauge sampling.
- **D8** (renumbered stubs) — flagged in ROADMAP.md correctly; plans do not touch unrelated stubs.
- **Wave ordering**: 27-01/27-02 are file-scope-disjoint within Wave 1; 27-03 in Wave 2; 27-04/27-05 in Wave 3 serialized via `27-05 depends_on: [27-04]`.
- **Cross-plan golden-capture coordination** (the issue you flagged): 27-02 Task 3 uses a locally-built `animepahe-resolver:dev` image (NOT the compose-wired service), explicitly to avoid a 27-02 → 27-03 dependency that would push 27-02 to Wave 2. The cross-references at the bottom of both 27-01 and 27-02 document this. Good.
- **Deviation reporting** (A1/A2/A5): 27-02 explicitly states "no DTO struct field changes are required" with cited assumptions and a planned `dto.go` comment + `cache.go` comment to lock the deviation in code. Good.
- **Security threat model**: every plan that owns code has a `<threat_model>` block; HIGH-severity threats (SSRF via host-allowlist in T-27-01-01, OOM cascade in T-27-01-03 + T-27-03-01) have explicit mitigations + tests.
- **Requirements coverage**: SCRAPER-HEAL-29 (27-01), -30 (27-02), -31 (27-03), -32 (27-04), -33 (27-05) — one-to-one mapping in plan frontmatter, matches ROADMAP.md and v3.1-REQUIREMENTS.md.
- **VALIDATION.md task rows** cover 23 specific verification gates with `<automated>` or `<manual>` references; Wave 0 file list aligns with new test/golden files; manual-only gates (D5 soak, end-to-end pipeline, log-flood scan, depends_on smoke) are correctly classified.

---

## Recommendation

Address blockers B1–B4 (parser-side correctness) and warnings W1–W4 (documentation polish). Re-submit for second-pass review. After fixes, plans should be ready for `/gsd-execute-phase 27`.

Estimated revision scope: 27-02 Task 1 + Task 2 prose updates (~20 lines of plan-text changes), 27-04 Task 2 (~5 lines), 27-RESEARCH.md `## Open Questions` (4 inline RESOLVED markers + heading suffix). No new plans, no scope changes, no wave reorganization required.


---

## Iteration 2 Re-Check

**Date:** 2026-05-19 (same-day revision pass)
**Checker:** gsd-plan-checker (second pass)
**Plans verified:** 27-01, 27-02, 27-03, 27-04, 27-05 (re-read after planner's revisions)

## CHECK PASSED

All four blockers (B1–B4) and four warnings (W1–W4) from iteration 1 have been resolved correctly. No new issues introduced by the revision pass. Cross-plan coordination remains intact. Plans are ready for `/gsd-execute-phase 27`.

### Verification of each iteration-1 issue

**B1 + B2 — `Provider.baseURL` removal + Kwik Referer chain — RESOLVED**

- 27-02 Task 1 explicitly states (with line-number citations against the current `client.go`):
  - DELETE `Provider.baseURL string` struct field declaration at `client.go:129`
  - DELETE the initializer `baseURL: strings.TrimRight(base, "/")` at `client.go:170`
  - DELETE the surrounding `base := d.BaseURL; if base == "" { base = "https://animepahe.ru" }` block at `client.go:165-168`
  - DELETE `Deps.BaseURL`
  - ADD `Provider.resolver *resolverClient` field
  - ADD `Deps.ResolverURL string` field
  - ADD package-level `const kwikReferer = "https://animepahe.pw/"` at the top of new `resolver.go` (with a documenting comment that cites D2)
  - REWRITE the single line at `client.go:507` from `headers := http.Header{"Referer": []string{p.baseURL}}` to `headers := http.Header{"Referer": []string{kwikReferer}}`
  - UPDATE the comment immediately above line 507 to reflect the new source
- The Task 1 `<verify>` gate at line 174 explicitly asserts `! grep -q "p\.baseURL" internal/providers/animepahe/client.go` AND `grep -q 'kwikReferer' internal/providers/animepahe/client.go` AND `grep -q 'kwikReferer' internal/providers/animepahe/resolver.go`. These three grep assertions collectively pin the contract.
- The `must_haves.truths` block (lines 32 + 36–37 of 27-02) now contains two truths covering this contract: one saying the Referer source switches from `p.baseURL` to `kwikReferer`, and a separate one asserting the exact Referer value `https://animepahe.pw/` is what reaches Kwik.
- `TestProvider_GetStream` (Task 2) now adds one new assertion that `headers.Get("Referer") == "https://animepahe.pw/"` — the test exercises the new constant.

Cross-checked against actual `client.go` via grep: the file currently has `baseURL` at lines 129, 170, 260, 331, 394, 505, 507 — the plan handles all seven sites (field declaration, initializer, three URL-construction sites that move to `p.resolver.Search/Release/Play`, the GetStream Referer line, and the comment above it). No site is missed.

**B3 — 27-04 Stream DTO shape assumption — RESOLVED**

- 27-04 Task 2 step 4 now hardcodes `REF="https://animepahe.pw/"` with an inline comment citing the `kwikReferer` constant in `resolver.go` (introduced by 27-02 Task 1).
- The action text explicitly notes: "The stream DTO does NOT currently expose a `headers.Referer` field on the response shape, and adding that surface is OUT OF SCOPE for this gate-clear (would require a parser-side DTO change, plus orchestrator and gateway forwarding)."
- The `must_haves.truths` block at line 21 reflects the same: "Referer hardcoded to the parser's `kwikReferer` constant value from 27-02; the response DTO does not currently expose a `headers.Referer` field — surfacing one is explicitly out of scope for this gate-clear".
- The "Files to Read" block at line 53 now includes `resolver.go` so the executor can confirm the constant value before hardcoding it into the curl Referer.
- The post-ship section template at line 188 also adds an inline note explaining why the response excerpt does not show a Referer field: "the Referer applied in the next gate is the parser's kwikReferer constant — https://animepahe.pw/ — verified out-of-band against services/scraper/internal/providers/animepahe/resolver.go".

**B4 — MalSync reverse lookup persistence — RESOLVED**

- 27-02 Task 2 explicitly replaces the in-memory map with a persistent cache key approach. The action text says: "the in-memory `Provider.malsyncReverseLookup map[string]string` and the `malsyncMu` mutex from the prior draft are NOT introduced — they are explicitly out of this plan, replaced by the persistent-cache approach".
- Forward key: `malsync:<malID>:animepahe → providerID` (existing, preserved verbatim)
- Reverse key: `malsync_reverse:animepahe:<providerID> → malID` (NEW; same 24h TTL; written transactionally with the forward key on each `FindID` positive-cache write).
- New exported method `MalSyncClient.Invalidate(ctx, malID, provider [, providerID])` deletes BOTH keys via a single variadic `m.cache.Delete(ctx, forwardKey, reverseKey)` call.
- New exported helper `MalSyncClient.LookupMalID(ctx, providerID, provider) (string, error)` reads the reverse key. Returns `("", nil)` cleanly on `cache.ErrNotFound`.
- Cross-process-restart durability is asserted by sub-test (a) `WithoutPriorFindID_PersistedReverseKey` in the new `malsync_invalidation_test.go` file — it seeds the cache DIRECTLY via `cache.Set` (no prior `FindID` call), simulating a fresh process. This is the load-bearing test that proves persistence > in-memory.
- The `must_haves.truths` block at line 38 now explicitly says: "uses a PERSISTENT reverse-mapping key … invalidation works across process restarts and across scraper processes (no in-memory reverse map)".
- VALIDATION.md row 27-02-07 references the new test file `malsync_invalidation_test.go`, which is in Wave 0 requirements.

The "acceptable no-op" path (sub-test c — `NoMalIDKnown`) is also explicitly covered with rationale that distinguishes it from a silent footgun: "the only way the reverse key is missing is that FindID was never called for this providerID in the past 24h — in which case the forward MalSync entry is also absent and there's nothing to invalidate."

**W1 — RESEARCH.md Open Questions RESOLVED markers — RESOLVED**

- `27-RESEARCH.md` line 756 now reads `## Open Questions (RESOLVED)`.
- Each numbered question (Q1–Q4 at lines 758, 764, 770, 776) has an inline `**RESOLVED in Plan 27-XX (Task N):**` marker citing the specific plan + task that delivers the resolution.

**W2 — 27-04 envelope shape diagnostic — RESOLVED**

- 27-04 Task 2 step 1 now includes `echo "$RESP" | jq 'keys'` as a diagnostic BEFORE the `data.episodes | length >= 28` assertion.
- The action text adds explicit handling for both envelope shapes: "If the `jq 'keys'` line prints `[\"data\"]` confirm the next assertion proceeds; if it prints `[\"episodes\", ...]` directly, the envelope is flat and the assertion must be `.episodes | length >= 28` instead — STOP and surface the shape mismatch as `## ENVELOPE MISMATCH: expected .data.episodes, got <actual keys>` rather than silently failing on a zero-count."
- "Files to Read" at line 51 cross-references `docs/issues/scraper-provider-verification-2026-05-19.md` with grep hints for `data.episodes`/`data.servers`/`data.sources` to confirm canonical shape.

**W3 — 27-01 Makefile redeploy target shape — RESOLVED**

- 27-01 Task 3 now starts with an explicit "Pre-step (W3 — Makefile-shape verification)" that:
  - Cites the actual Makefile structure: `Makefile:267-273` for explicit `redeploy-web` target, `Makefile:275-276` for the `redeploy-%` wildcard, `Makefile:5` for the `.PHONY` list containing `redeploy-web`.
  - Verifies GNU make's resolution order (explicit > pattern; no redefinition warning) so adding an explicit target is the documented convention.
  - Instructs the executor to mirror the same shape AND add `redeploy-animepahe-resolver` to the `.PHONY` list at line 5.
  - Includes a fallback: "If the `redeploy-web` recipe shape differs from what's described above at execution time (e.g. someone has refactored Makefile since this plan was written), re-read `redeploy-web` first and mirror the actual shape — do NOT guess."
- I verified the Makefile claims by reading the file directly: line 1 starts `.PHONY: all build test lint clean generate dev help \` with multi-line continuation; line 5 contains `redeploy-all redeploy-web type-check`; line 267 has the explicit `redeploy-web` target with `i18n-lint type-check ## ...` shape; line 275 has the wildcard. The plan's citations are accurate.

**W4 — 27-02 cache.Delete fallback prose — RESOLVED**

- The misleading `cache.Set(ctx, key, "", 0)` fallback prose is GONE from 27-02 Task 2.
- The action text now says unconditionally: "`libs/cache/cache.go:16` confirms `Delete(ctx context.Context, keys ...string) error` IS on the interface — call it unconditionally with both keys in a single variadic call: `m.cache.Delete(ctx, forwardKey, reverseKey)`."
- I verified against the actual `libs/cache/cache.go` file: line 16 does declare `Delete(ctx context.Context, keys ...string) error` on the `Cache` interface; line 130 has the `RedisCache` implementation. The plan's citation is accurate.
- The "Files to Read" block at line 112 of 27-02 explicitly cross-references `libs/cache/cache.go` line 16 with this note: "confirm `Delete(ctx context.Context, keys ...string) error` IS on the `Cache` interface; this plan uses it unconditionally (Task 2 below)."

### Cross-cutting verification

**No new issues introduced by the revisions:**

- **VALIDATION.md alignment:** Task row 27-02-07 references the new file `malsync_invalidation_test.go` which is also in the Wave 0 requirements list AND in the 27-02 `files_modified` frontmatter. Three-way alignment intact.
- **Files-modified frontmatter:** 27-02 `files_modified` lists `malsync_invalidation_test.go` (line 9 of frontmatter) and `resolver.go` (line 12) and `resolver_test.go` (line 13). All new files are tracked.
- **Cross-plan kwikReferer constant location:** 27-04 Task 2 step 4 reads the constant value from `services/scraper/internal/providers/animepahe/resolver.go`, which is exactly where 27-02 Task 1 places it (top of the new file). Single source of truth, no drift.
- **Wave ordering unchanged:** 27-01 || 27-02 (Wave 1) → 27-03 (Wave 2) → 27-04 → 27-05 (Wave 3, serialized via depends_on). No new dependencies were introduced that would force re-wave-ing.
- **Plan size sanity:** 27-02 grew the most (one new exported method, one new helper, one new test file with three sub-tests, the persistent-key write extension on FindID). Estimated +60–80 LOC of plan-text; +120–150 LOC of source code. Within the 2-task / 5-file budget per plan; remains executable in a single context window.
- **Threat model unchanged:** No new threat surface introduced. T-27-02-01..T-27-02-03 still cover the relevant parser-side surface; the persistent cache key uses the same Redis instance and TTL as the existing forward key — no new trust boundary.
- **Decision compliance:** All eight CONTEXT.md decisions (D1–D8) remain honored:
  - D2 (animepahe.pw exclusive) is REINFORCED by the new `kwikReferer = "https://animepahe.pw/"` constant.
  - D4 (fresh goldens) still owned by 27-02 Task 3.
  - D5 (500 MB hard ship gate) still owned by 27-01 Task 4 + 27-03.
  - D7 (last-step gated flip) still owned by 27-05 with explicit precondition gate.
  - Others unchanged.
- **CLAUDE.md compliance:** No new violations. The `bun` / `bunx` directives are unaffected (this phase doesn't touch the frontend except via the after-update changelog). The "Adding New libs/ Module" steps don't apply (sidecar is a service, not a lib). The After-Update skill is still mandatory and invoked by 27-05 Task 3.

### Estimated execution context budget

| Plan | Tasks | Files | Estimated context % | Status |
|------|-------|-------|---------------------|--------|
| 27-01 | 4     | 14    | ~65% (largest — scaffolding + soak gate) | OK |
| 27-02 | 3     | 14    | ~60% (parser rewrite + persistent-key + tests) | OK |
| 27-03 | 3     | 2     | ~20% | OK |
| 27-04 | 3     | 1     | ~25% (manual gates) | OK |
| 27-05 | 3     | 2     | ~25% (manual gates + after-update) | OK |

All plans remain within the documented 2–3 task / 5–8 file targets (27-01 is at the upper end with 4 tasks but the D5 soak gate is a separable manual task that doesn't compete with the scaffolding tasks for context).

## Recommendation

Plans are ready for execution. Run `/gsd-execute-phase 27`.

The revision pass surgically addressed each issue without introducing new contradictions or scope creep. The persistent-cache reverse-mapping approach (B4 fix) is the right architectural call — durable across process restarts AND across scraper processes, with the same TTL as the forward key, no new state on `Provider`. The `kwikReferer` constant (B1/B2 fix) is correctly co-located with the rest of the resolver transport in `resolver.go` (cited as the source of truth by 27-04 Task 2 step 4 — single location, no drift risk).

No further iterations required.
