Verdict: ready — Plan 28-02 may proceed with 0 existing-registry extractors + 1 new extractor (`embeds/vidstream_vip.go`).

# SPIKE-ANIMEFEVER.md — AnimeFever Embed-Extractor Recon

**Phase:** 28-provider-expansion-r2
**Plan:** 28-01 (SCRAPER-HEAL-35)
**Recon date:** 2026-05-20
**Recon target:** Frieren - Beyond Journey's End, MAL 52991, AniList 154587, episode 28
**Upstream slug:** `frieren-beyond-journeys-end.14401`, ep_id `189572`
**Captured fixtures:** `services/scraper/internal/providers/animefever/testdata/` (7 files; see Task 1 commit `0136484`)

---

## 1. Embed Hosts

AnimeFever's watch page exposes a server `<select>` with two options:

| Server | Selected by default | Iframe host | URL shape (URL params, lt=…) | Resolved upstream host |
|--------|---------------------|-------------|------------------------------|------------------------|
| `tserver` | ✓ (default) | `am.vidstream.vip` | `https://am.vidstream.vip?r=&k=<128hex>&li=<ep_id>&tham=<epoch>&lt=ts&qlt=720p&...&key=<32hex>&ua=<128hex>&h=<epoch>` | `static-cdn-ca1.mofl.pro` (HLS m3u8) |
| `hserver` | — | `am.vidstream.vip` | `https://am.vidstream.vip?r=&k=<32hex>&li=<ep_id>&tham=<epoch>&lt=hydrax&qlt=720p&...&key=<32hex>&ua=<128hex>&h=<epoch>` | n/a (JS-evaluated hydrax player; no inline HLS literal) |

**Both servers point at the SAME embed host (`am.vidstream.vip`)** — the difference is the `lt=` query parameter (`ts` vs `hydrax`) which tells the embed which player flavour to render.

Observed iframe URLs (from `ajax_load_ep28.json` and `ajax_load_ep28_hserver.json`):

- tserver: `https://am.vidstream.vip?r=&k=b39f945438bd62fb085d80804071e66a87f2103d75b6640d60792f0098854481b48b4db445ae2d656bc2669380d7784d91052ea83055006a51c7c335ecc77337e592b07ec895c18b4f0842b5925004b2&li=189572&tham=1779241087&lt=ts&qlt=720p&spq=p&prv=&key=b3c8c13eab90bfce60797ecbe7c45a18&ua=<masked>&h=1779241087`
- hserver: `https://am.vidstream.vip?r=&k=6119c63dfb14089c870f3a5683e9c29c&li=189572&tham=1779241098&lt=hydrax&qlt=720p&spq=p&prv=&key=31d5eeffeef5b017d50b8616fd8c06a2&ua=<masked>&h=1779241098`

Downstream HLS CDN observed in `embed_vidstream_vip.html`: `https://static-cdn-ca1.mofl.pro/masters/6849782a3577c84f1d8e0818/master.m3u8` (HTTP 200, Cloudflare-fronted, `content-type: application/vnd.apple.mpegurl`, 40 KiB, year-long cache).

---

## 2. Classification per Host

For each host AnimeFever proxies to, the classification:

| Host | Covered by existing extractor? | Classification | Rationale |
|------|--------------------------------|----------------|-----------|
| `am.vidstream.vip` | ❌ NO. Checked `kwik.go` (kwik.cx, kwik.si), `megacloud.go` (megaup.cc + megacloud.* wildcard), `streamhg.go` (otakuhg.site), `earnvids.go` (otakuvid.online), `vibeplayer.go` (vibeplayer.site, cdn.cimovix.store). None match. | `needs-new-extractor: services/scraper/internal/embeds/vidstream_vip.go` | Plain HTML regex extracts inline `sources: [{"file":"https://...m3u8","type":"mp4","label":"HD"}]` literal. NOT Dean-Edwards-packed → NOT a `packed_common.go` reuse. |
| `static-cdn-ca1.mofl.pro` | n/a — this is the HLS m3u8 CDN, not an iframe host | NOT an extractor target | Only needs `HLSProxyAllowedDomains` entry in `libs/videoutils/proxy.go`. The HLS proxy fetches segments directly; no JS to extract. |

---

## 3. Recommended Extractors for Plan 28-03

Ordered numbered list of new `embeds/<name>.go` files Plan 28-03 must write:

### 1. `services/scraper/internal/embeds/vidstream_vip.go`

- **File path:** `services/scraper/internal/embeds/vidstream_vip.go`
- **Input host(s) it matches:** `am.vidstream.vip` (and any future `*.vidstream.vip` subdomain via `vidstream.vip` suffix-host check — match the `kwik.go` pattern with host-equality + strict-subdomain `HasSuffix`)
- **Upstream Referer to set:** `https://animefever.cc/` (per RESEARCH.md skeleton — the embed accepts requests with the AnimeFever site as referrer; it does NOT require a referrer matching its own host)
- **Extraction approach:** **Plain regex** against the embed HTML body. Pattern (from RESEARCH.md skeleton + verified against `embed_vidstream_vip.html`):
  ```
  sources\s*:\s*\[\s*({[^}]+})
  ```
  Then `json.Unmarshal` the captured `{...}` into `struct { File, Type, Label string }`. Validate `File` is an absolute HTTPS URL with `.m3u8` extension.
- **Stream shape returned:** `domain.Stream{ Sources: [{ URL: <m3u8>, Type: "hls", Quality: <Label> }], Headers: { "Referer": "https://animefever.cc/" } }`
- **Captured test fixture:** `services/scraper/internal/providers/animefever/testdata/embed_vidstream_vip.html` (7642 bytes — proves the `sources: [{"file":"https://static-cdn-ca1.mofl.pro/masters/6849782a3577c84f1d8e0818/master.m3u8","type":"mp4","label":"HD"}]` literal shape)
- **Approximate line count:** ~120 LOC including doc comment + Matches + Extract + struct + var-tagged `_ EmbedExtractor = (*VidstreamVipExtractor)(nil)` interface assertion. Mirrors the `vibeplayer.go` (regex-only, no goja) template — NOT the heavier `kwik.go`/`streamhg.go` (goja + Dean-Edwards-unpack) template.
- **No goja dependency:** the embed page contains the m3u8 URL in a plain literal, no JS evaluation needed.

**No other new extractors are required.** AnimeFever's `hserver` (hydrax-style) path does NOT need a separate extractor for Phase 28 — tserver carries Frieren ep28 (verified) and is the default-selected server; hserver is fallback-only and the hydrax-shape extraction is out of scope per D4 ("we don't speculatively write extractors"). If a future episode forces hserver-only delivery, a `hydrax.go` extractor can be added in a follow-up phase. Documented as **A2 / Per-Server Coverage** below.

---

## 4. HLS Proxy Allowlist Hosts

Add to `libs/videoutils/proxy.go::HLSProxyAllowedDomains` in Plan 28-02's `cmd/scraper-api/main.go`-adjacent allowlist commit (per D7):

| Host | Why it must be allowlisted | Evidence |
|------|----------------------------|----------|
| `am.vidstream.vip` | Iframe host. The HLS proxy may need to fetch the embed page if the upstream uses redirected m3u8 URLs. (Defensive — current observation has the m3u8 directly on `static-cdn-ca1.mofl.pro`, but the embed-Referer chain is grounded at vidstream.vip.) | `embed_vidstream_vip.html` was served by this host with `Content-Type: text/html` |
| `static-cdn-ca1.mofl.pro` | Actual HLS m3u8 + segment host. Fail-closed allowlist (Phase 25 SCRAPER-HEAL-24) will return 502 without this entry, blocking playback. | `master.m3u8` HTTP 200 from this host, 40 KiB body |

**Optional defensive entry (not required for Frieren but reasonable):**
- `static-cdn-ca2.mofl.pro`, `static-cdn-ca3.mofl.pro`, … — Mofl CDN sibling subdomains. Plan 28-02 may opt for a suffix-allowlist entry for `mofl.pro` to future-proof against CDN-shard rotation. Default recommendation: just `static-cdn-ca1.mofl.pro` for now; widen to suffix only if a daily-canary failure surfaces.

---

## 5. Token Shape (`ctk`)

**Observed value (Frieren ep28, watch_ep28.html):** `21e5bf08107829bf48f33147aba9537e` — 32 lowercase hex characters.

**Regex Plan 28-02 should use** (from RESEARCH.md A8, confirmed against observation):
```go
var ctkRegex = regexp.MustCompile(`var\s+ctk\s*=\s*'([0-9a-fA-F]{32,64})'`)
```

- Anchor: `var\s+ctk\s*=\s*'…'` (single quotes — that's what the page emits; no double-quote variant observed).
- Char class: `[0-9a-fA-F]` (hex; lower-case observed but case-insensitive class is safer for future-proofing).
- Length: `{32,64}` (widened from observed 32 — the upper bound accommodates a SHA-256-derived token shape without forcing a Plan-28-02 patch if upstream lengthens).

**Deviation from A8:** None. The observed shape exactly matches the assumption.

**Where the token must propagate:** form-encoded POST body of `/ajax/anime/load_episodes_v2?s=<server>`. Two-step fetch: GET watch page → regex-extract ctk → POST AJAX with `episode_id=<eid>&ctk=<token>`. The cookie-jar-borne PHPSESSID also propagates (verified: same `curl -c/-b` cookie jar across all 4 requests; AJAX returned `status:true`).

---

## 6. Per-Server Coverage (tserver vs hserver)

Per CONTEXT.md A2 and Pitfall 3:

| Server | Carries Frieren ep28? | Iframe response | Embed-page shape | Extractor needed |
|--------|----------------------|-----------------|------------------|------------------|
| `tserver` | ✓ verified (status:true; iframe present; embed returns valid sources literal; m3u8 HTTP 200) | `lt=ts` flavour | Plain `sources: [{...}]` literal | `vidstream_vip.go` (regex) |
| `hserver` | ✓ verified (status:true; iframe present); embed-page extraction NOT verified for Phase 28 | `lt=hydrax` flavour | Hydrax-style JS player; NO inline `sources:` literal; requires per-request API call to hydrax backend with the `slug=` query param | OUT OF SCOPE — Plan 28-02 should treat hserver as fallback-only and surface `ErrNoStream` if tserver fails AND hserver also fails to extract. A daily-canary failure on hserver-only delivery would trigger a follow-up extractor in a future phase. |

**Plan 28-02 ListServers strategy** (per Pitfall 3 + this recon):

- Return BOTH `tserver` and `hserver` from `ListServers` so the UI can surface them.
- In `GetStream`, attempt `tserver` first; if its embed extraction fails (`vidstream_vip.go` returns `ErrExtractFailed`), attempt `hserver`. Since the current vidstream_vip extractor only handles the `lt=ts` shape, the hserver path will fail-soft with `ErrExtractFailed` and the orchestrator will move to the next provider (allanime → … → animefever → …). This is the correct degraded-but-functional posture per D4 ("we don't speculatively write extractors").

**Risk surfaced (A2):** if AnimeFever stops emitting `lt=ts` and only emits `lt=hydrax` for a future Frieren episode, the Frieren E2E gate (D6) would fail. The daily canary (Phase 23) catches within 24h. Mitigation: write the hydrax extractor in a follow-up phase (not Phase 28).

---

## 7. Embed Page Shape (regex anchor for vidstream_vip.go)

**Source page:** `https://am.vidstream.vip/?<query>` (note: redirects to `?` form even without trailing path; cf. `embed_vidstream_vip.html` ETag header proves it's the same page across cache durations).

**Inline JSON literal (verbatim from captured fixture):**
```js
sources: [{"file":"https://static-cdn-ca1.mofl.pro/masters/6849782a3577c84f1d8e0818/master.m3u8","type":"mp4","label":"HD"}]
```

**Anchor regex (matches across `sources\s*:\s*\[\s*` + first `{…}` greedy-but-bounded):**
```go
var vidstreamVipSourcesRegex = regexp.MustCompile(`sources\s*:\s*\[\s*({[^}]+})`)
```

- The `[^}]+` greedy character class is safe because the JSON object value is single-flat (no nested `{…}` per the observed shape — `file`, `type`, `label` are all string-leaf).
- If a future version of vidstream emits nested objects (e.g., `tracks:`), the regex would still match the first flat `{…}` after `sources: [` and JSON-unmarshal it; subsequent objects in the array would be ignored. That's acceptable for "give me the primary stream" semantics.

**Parsed struct shape** (Plan 28-02 / 28-03 internal DTO):
```go
type vidstreamVipSource struct {
    File  string `json:"file"`
    Type  string `json:"type"`
    Label string `json:"label"`
}
```

**`Type` field semantics gotcha:** the observed `"type":"mp4"` is a **lie** — the underlying URL is `.m3u8` (HLS). The extractor MUST classify by URL suffix (`.m3u8` → `domain.SourceTypeHLS`), NOT by the upstream's `type` field. Document this in `vidstream_vip.go`'s doc comment + `dto.go`.

---

## 8. Recon Summary (for cross-reference with 28-RESEARCH.md)

Live recon 2026-05-20 (this spike) confirms 28-RESEARCH.md's predictions exactly:

| RESEARCH.md prediction | Observed (this spike) | Match |
|------------------------|----------------------|-------|
| `am.vidstream.vip` is the embed host | ✓ | ✓ |
| Plain-regex extraction (no Dean-Edwards-packer) | ✓ (`sources: [{"file":"...m3u8"}]` literal) | ✓ |
| `static-cdn-ca1.mofl.pro` is the HLS CDN | ✓ | ✓ |
| `ctk` token is 32-hex | ✓ (`21e5bf08107829bf48f33147aba9537e`) | ✓ |
| 28 episodes for Frieren S1 | ✓ (28 `/watch/?ep=` links) | ✓ |
| Allowlist hosts: `am.vidstream.vip` + `static-cdn-ca1.mofl.pro` | ✓ | ✓ |
| Both `tserver` and `hserver` exist | ✓ (visible in `<select>` on watch page) | ✓ |
| tserver → vidstream.vip embed shape (extractable) | ✓ | ✓ |
| One new extractor (`vidstream_vip.go`) required | ✓ confirmed | ✓ |

**Spike artifact status:** `ready`. Plan 28-02 (AnimeFever provider lift) and Plan 28-03 (new embed extractors) may proceed with zero open questions on the embed-host classification or extractor write-list.

---

*Co-Authored-By: Claude Opus 4.7 <noreply@anthropic.com>*
*Co-Authored-By: 0neymik0 <0neymik0@gmail.com>*
*Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>*
