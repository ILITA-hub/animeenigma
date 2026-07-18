# AUTO-629 follow-up: external-source research (Yani Neko RU subs · Shoujo Ramune 18+ coverage)

Research date: 2026-07-17 (live HTTP verification unless noted). Origin: AUTO-629 re-triage — see the
feedback pair `2026-07-17T14-04-39_gerahertz_feedback` / `14-08-09`.

## Part 1 — RU subtitles for Yani Neko (ヤニねこ, Shikimori 63403)

**RESOLVED 2026-07-18: anime365 went fully PAYWALLED (owner-confirmed) and is RETIRED;
replaced by a Kage Project adapter.** The earlier "auth-gated, free registration fixes it"
reading was too optimistic — access now requires a paid subscription, and even the anonymous
metadata API degraded (`api/translations?episodeId=…` returns `{"data":[]}`; file endpoint
403 `"You should login first."`). The whole integration (parser `anime365/`, aggregator
provider, `/subtitles/anime365/file/` route, `ANIME365_*` config) was deleted — precedent:
`raw`-provider deletion, not the AniLib keep.

**Replacement (shipped): Kage Project / fansubs.ru** — the canonical RU anime fansub archive,
actively fed with summer-2026 seasonals. Plain HTTP (site refuses TLS), `POST /search.php`
in **cp1251**, `GET base.php?id=N` release rows, `POST base.php` (`srt=N`) → RAR/ZIP archive
with per-episode ASS/SRT (live-verified: Frieren id 7120 → srt 13364 → RAR v4 solid,
28 per-episode ASS; `rardecode` extracts fine). Adapter: `services/catalog/internal/parser/kage/`,
provider `kage`, route `/api/anime/{id}/subtitles/kage/file/{srtId}?episode=N`.
Caveats: no MAL/Shikimori ID mapping (exact-normalized-title match only, conservative by
design) and **no Yani Neko release yet** — if a team subs it, it surfaces automatically.

Other candidates (unchanged): **AniLibria** — empty stub for this title (release 10263,
`anilibria.top/api/v1`); RU **voiceover** group → future RU-DUB video source, not subs
(legacy `api.anilibria.tv/v3` is HTTP 410). Dead ends: subs.com.ru (now a news blog) ·
SovetRomantica (403 without browser sidecar) · OpenSubtitles web (401; the REST API we
already use is the way, RU coverage unlikely for this title) · anidub (dub site, no files).

## Part 2 — Shoujo Ramune (小女ラムネ, Shikimori 32587) 18+ episode coverage

| Site | Eps verified | Notes |
|---|---|---|
| **hentaimama.io** | **1–6 all** | DooPlay WordPress; self-hosted MP4s under `wp-content/uploads`; player via `admin-ajax.php` (`doo_player_ajax`); **no Cloudflare**. Light scrape; one browser capture to pin ajax params. |
| **hstream.moe** | **1–6 all** | Laravel Livewire SPA; **EN soft-subs** (only realistic subtitle source for this title); stream token needs the Livewire XHR round-trip → browser sidecar or reproduced XHR. |
| hanime.tv (incumbent) | 5–6 only | Catalog API is open (`guest.freeanimehentai.net/api/v11/search_hvs`); per-video `api/v8/video` is Cloudflare-gated → why we report `no_content`. Eps 1–4 will never come from hanime. |
| oppai.stream | unverified | JS-only search; needs browser sidecar to even check. |
| muchohentai / hentaihaven.xxx | blocked | Cloudflare JS challenge; not worth it given the two above. |

Recommended integration order: **hentaimama.io** (primary, all 6, easy) → **hstream.moe**
(quality + EN subs). Subtitles for hentai OVAs otherwise do not exist (OpenSubtitles/Jimaku empty).

## Related fixes shipped alongside (2026-07-17/18 worktree auto629-fixes)
- Analytics: fetch-first transport → masked/tokenized alias fallback actually engages ($ping blockers).
- aePlayer: bounded media-error recovery + `decode` fatal + **Hi10P wasm compat engine** (libmedia,
  vendored in `frontend/web/public/libmedia/`) — the ae source's High-10 encodes now play in Firefox.
- Library encoder still emits High 10 from 10-bit raws (`transcoder.go` lacks `-pix_fmt yuv420p`) —
  intentional for now (owner chose client-side decode over 8-bit re-encode).
