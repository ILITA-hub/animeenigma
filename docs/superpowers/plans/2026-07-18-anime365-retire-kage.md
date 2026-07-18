# anime365 Retirement + Kage Project RU-Sub Adapter

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** anime365 (smotret-anime.org) went fully paywalled (owner-confirmed 2026-07-18; file endpoint 403 "You should login first", anonymous translations API now returns empty). Retire it completely and replace the RU-subtitle slot with a Kage Project (fansubs.ru) adapter.

**Architecture:** Same aggregator seam anime365 occupied: one parser client (`parser/kage/`), one `fetchKage` provider goroutine, one lazy file-resolve route. Kage has no API — three server-rendered cp1251 surfaces: `POST /search.php` (query) → `base.php?id=N` links; `GET /base.php?id=N` → release rows (hidden `srt` ids + label/format + author/team blocks); `POST /base.php` (`srt=N`) → RAR/ZIP archive (or bare subtitle file) with per-episode `.ass`/`.srt` inside. Live-verified 2026-07-18 (Frieren id 7120 → srt 13364 → RAR v4 solid, 28 per-episode ASS files, `rardecode` v1.1.3 extracts fine).

**Tech Stack:** Go stdlib `archive/zip`, `github.com/nwaples/rardecode` (RAR v4/v5, pure Go), `golang.org/x/text/encoding/charmap` (cp1251, already an indirect dep).

## Global Constraints
- Retirement is COMPLETE: delete `parser/anime365/` + all wiring/tests (no compatibility stubs — precedent: `raw` provider deletion, NOT the AniLib keep).
- Fail-soft contract preserved: provider errors → `ProvidersDown`, never abort the aggregate.
- Conservative title matching: exact normalized romaji/EN match only — no fuzzy pick (wrong-show subs are worse than none).
- Never serve a non-subtitle body (HTML error page) as a track.
- Metrics: UXΔ = +1 (Better) · CDI = 0.03 × 8 · MVQ = Griffin 80%/85%.

## Tasks
1. **parser/kage** — `client.go` (Config/NewClient/IsConfigured/SearchSeries/GetReleases/DownloadArchive/Ping, cp1251 decode+encode, regex parse of search results + release forms + author blocks, episode-range parse from label), `archive.go` (magic-sniff rar/zip/bare, subtitle-entry listing, filename episode-number heuristic, cp1251→UTF-8 + BOM strip, subtitle-body validation). Tests: httptest cp1251 fixtures; in-memory zip + bare-file extraction; range/label/filename table tests.
2. **Aggregator swap** — `subs_aggregator.go`: field/ctor/goroutine `anime365`→`kage`; `fetchKage` (series resolve cached `subs:kage:series:<animeID>` 7d/6h, releases filtered by episode range, tracks `/api/anime/{id}/subtitles/kage/file/{srt}?episode=N`, Lang ru, Provider kage); `ResolveKageFile` (result cache 24h + archive cache 1h ≤8MB). Delete anime365 tests, mirror as kage tests.
3. **Handler/router/config/DI** — `GetKageFile` (srtId + episode query), route swap, `KageConfig` (`KAGE_BASE_URL` default `http://www.fansubs.ru`, `KAGE_ENABLED` default true), main.go client + subprobe pinger "kage".
4. **Frontend** — BrowseSubsModal badge map `anime365`→`kage`; OtherSubsPanel providerChip i18n `kage: "Kage"` ×3 locales; subtitleProxy comment.
5. **Docs/memory** — research doc Part 1 rewrite (anime365 paywalled+retired, Kage shipped); memory updates; after-update flow (catalog + web redeploy).
