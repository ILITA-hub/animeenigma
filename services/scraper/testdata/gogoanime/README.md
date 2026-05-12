# Gogoanime/Anitaku golden fixtures

Phase 18 — captured from `anitaku.to` and the three embed wrapper hosts
(`vibeplayer.site`, `otakuhg.site`, `otakuvid.online`) plus a malsync.moe
response. These goldens drive offline tests for the Gogoanime provider
(`services/scraper/internal/providers/gogoanime/`) and three embed extractors
(`services/scraper/internal/embeds/{vibeplayer,streamhg,earnvids}.go`).

## Fixtures

| File | Source URL | Exercises | Capture date |
|------|-----------|-----------|--------------|
| `search_attack_on_titan.html` | `https://anitaku.to/search.html?keyword=Attack+on+Titan` | Multi-result search page parse → `<a href="/category/...">` link extraction (sub + dub + season variants) | 2026-05-12 |
| `category_one_piece.html` | `https://anitaku.to/category/one-piece` | Single-anime detail page → episode list scrape (sub variant: `/one-piece-episode-N`) | 2026-05-12 |
| `category_one_piece_dub.html` | `https://anitaku.to/category/one-piece-dub` | Single-anime detail page → episode list scrape (dub variant: `/one-piece-dub-episode-N`) — verifies sub/dub merge behaviour | 2026-05-12 |
| `one_piece_episode_1.html` | `https://anitaku.to/one-piece-episode-1` | Episode page with `<ul class="anime_muti_link">` → `<li><a data-video="...">` server-list parse | 2026-05-12 |
| `vibeplayer_embed.html` | the HD-1 `data-video` URL extracted from `one_piece_episode_1.html` (Referer: `https://anitaku.to/`) | `const src = "https://...m3u8"` regex extraction | 2026-05-12 |
| `streamhg_packed.html` | the StreamHG `data-video` URL extracted from `one_piece_episode_1.html` (Referer: `https://otakuhg.site/`) | `eval(function(p,a,c,k,e,d))` Dean-Edwards packer body with `hls2` payload + `&e=<ttl>&s=<unix>&t=<token>` | 2026-05-12 |
| `earnvids_packed.html` | the Earnvids `data-video` URL extracted from `one_piece_episode_1.html` (Referer: `https://otakuvid.online/`) | Same packer shape as StreamHG, different host allowlist (`dramiyos-cdn.com` vs `premilkyway.com`) | 2026-05-12 |
| `malsync_no_gogo.json` | `https://api.malsync.moe/mal/anime/21` (One Piece) | Negative-cache exemplar — response's `Sites` map MUST NOT contain a `Gogoanime` or `Anitaku` key. Verifies that the gogoanime provider's malsync path will return `("", false, nil)` for every MAL ID until malsync.moe ships a Gogoanime entry. | 2026-05-12 |

Rationale for the chosen anime: **One Piece** and **Attack on Titan** are
high-cache-stability EN anime; both have sub + dub variants on Anitaku, both
have episode-1 pages with the full `anime_muti_link` server list (HD-1, HD-2,
StreamHG, Earnvids), and One Piece's MAL ID (21) is a stable malsync probe
key.

## Anonymization invariant

Captured HTML/JSON must NOT contain any of these patterns:

- `Set-Cookie:`
- `__ddg2_`
- `cf_clearance`
- `Bearer ` (with trailing space — matches Authorization headers)

The capture script (`services/scraper/scripts/capture-gogoanime-goldens.sh`)
runs `sed -i -E '/(Set-Cookie|__ddg2_|cf_clearance|Bearer )/d'` on all HTML
fixtures after capture, then re-asserts the invariant with `grep -rE`. The
Makefile target `capture-goldens-gogoanime` re-runs the same gate so a
post-capture leak fails the build before any commit.

Verify the invariant manually:

```bash
grep -rE '(Set-Cookie|__ddg2_|cf_clearance|Bearer )' services/scraper/testdata/gogoanime/
# Expected: no matches; non-zero exit code.
```

## Refresh procedure

```bash
make capture-goldens-gogoanime
```

The Makefile target invokes `services/scraper/scripts/capture-gogoanime-goldens.sh`,
which:

1. Fetches the 4 anitaku.to pages (`search_attack_on_titan.html`,
   `category_one_piece.html`, `category_one_piece_dub.html`,
   `one_piece_episode_1.html`).
2. Greps `data-video` URLs out of `one_piece_episode_1.html` and dispatches
   each by host to the matching wrapper fetch (vibeplayer / streamhg / earnvids).
3. Fetches `https://api.malsync.moe/mal/anime/21` for the negative-cache
   exemplar.
4. Strips Set-Cookie / DDoS / CF / Bearer lines from all `.html` files.
5. Re-asserts the anonymization invariant.

The script uses `set -euo pipefail` and `curl -fsSL`, so any 4xx/5xx from
any upstream halts the whole capture atomically (see "Upstream-death recovery"
below).

## Upstream-death recovery (Issue 12 — pivot protocol)

If `curl -f` returns 4xx/5xx for `anitaku.to`, `vibeplayer.site`,
`otakuhg.site`, or `otakuvid.online` during capture, the script halts
immediately and `make capture-goldens-gogoanime` exits non-zero. **The
recovery protocol is NOT to substitute synthetic fixtures.** Instead:

1. STOP execution of the current Phase 18 plan — do NOT commit partial
   goldens.
2. Document the upstream death in the phase SUMMARY under a new section
   `## Upstream Pivot Required` with: which host returned what status,
   timestamp (UTC), and the exact failing URL.
3. Phase 18 pivots AGAIN per `.planning/phases/18-9anime/18-RESEARCH.md`
   §Mirror Viability D1 rules — the next-best alive EN provider (likely
   AnimeKai or an alternative Gogoanime mirror).
4. The orchestrator (parent agent) detects the SUMMARY-only output (no
   goldens committed) and routes into a fresh research+pivot cycle.

The offline-test contract requires real upstream captures because the parser
tests assert structural invariants (anime_muti_link selector, packer regex,
m3u8 URL shape) that only real upstream bodies exhibit reliably. Synthetic
fixtures would lock the tests to a hand-written grammar and miss real-world
markup drift on the next live capture.

## Threat surface

These goldens may contain CDN URLs, signed m3u8 tokens, and embed IDs
captured from production upstream. They MUST NOT contain user-identifying
auth (Set-Cookie / Bearer / cf_clearance / DDoS-Guard cookies). The
anonymization gate in the capture script + Makefile target is the
enforcement boundary.

Phase 18 STRIDE register (T-18-01): Information Disclosure via captured
HTML — mitigated by `--cookie-jar /dev/null --no-keepalive` curl flags +
post-capture sed strip + CI grep gate.
