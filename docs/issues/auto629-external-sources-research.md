# AUTO-629 follow-up: external-source research (Yani Neko RU subs · Shoujo Ramune 18+ coverage)

Research date: 2026-07-17 (live HTTP verification unless noted). Origin: AUTO-629 re-triage — see the
feedback pair `2026-07-17T14-04-39_gerahertz_feedback` / `14-08-09`.

## Part 1 — RU subtitles for Yani Neko (ヤニねこ, Shikimori 63403)

**Key correction:** anime365 is NOT dead for this title. `GET smotret-anime.org/api/series?myAnimeListId=63403`
→ series 41893, and `api/translations?episodeId=…` lists the "dead" RU subs as `isActive:1`
(ep2: Sanae `5858218`, Neifur `5851441`; ep1 has three RU subs). What broke: the subtitle **file**
endpoint (`/api/translations/embed/<id>`) now returns `403 {"You should login first."}` —
**anime365 moved file delivery behind auth.** Our proxy surfaces that as the 503s users see.

Ranked:
1. **anime365 (fix the incumbent)** — (a) resolve translation IDs dynamically per `episodeId`
   (filter `typeLang=ru, typeKind=sub`, rank by `priority`/`activeDateTime`) instead of caching
   file IDs; (b) attach an anime365 `access_token` (free registration) so file fetches stop 403-ing.
   Metadata API is public; no Cloudflare. **Owner action needed: create the anime365 account/token.**
2. **Kage Project / fansubs.ru** — no Yani Neko yet, but actively fed with summer-2026 seasonals
   (RSS `?l=rss`, new subs dated 16.07.26). Plain HTTP (https refused), `POST /search.php` in
   **cp1251**, ZIP downloads, no anti-bot. Trivial adapter; good permanent RU fallback.
3. **AniLibria** — has the title as an empty stub (release 10263, `anilibria.top/api/v1`); it is a
   RU **voiceover** group → future RU-DUB video source, not subs. NOTE: legacy `api.anilibria.tv/v3`
   is retired (HTTP 410) — anything still pointing there is dead.

Dead ends: subs.com.ru (now a news blog) · SovetRomantica (403 without browser sidecar) ·
OpenSubtitles web (401; the REST API we already use is the way, RU coverage unlikely for this title) ·
anidub (dub site, no files).

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
