---
id: RAW-allanime-parser
title: AllAnime parser — raw Japanese audio source for the catalog service
workstream: raw-jp
milestone: v0.1
phase: 01
created_at: 2026-05-18
status: SPEC-ready
ambiguity_score: 0.15
mode: --auto
---

# Phase 01 (workstream `raw-jp`, milestone v0.1): AllAnime Parser — Specification

**Workstream:** `raw-jp`
**Milestone:** v0.1 Raw Provider MVP
**Source design doc:** `/data/animeenigma/docs/superpowers/specs/2026-05-18-raw-jp-provider-design.md`
**Requirements:** RAW-01, RAW-02, RAW-NF-01
**Mode:** `--auto` (Socratic interview skipped per workstream-level decision; key implementation choices auto-selected below)

## Goal

Implement a new catalog parser at `services/catalog/internal/parser/allanime/` that queries the AllAnime GraphQL API with `translationType: raw` and returns episode lists + HLS stream URLs. Expose new catalog endpoints `GET /api/anime/{shikimori_id}/raw/episodes` and `GET /api/anime/{shikimori_id}/raw/stream`.

## Background

**Today, three things are true and need to change:**

1. **No raw JP audio track exists in the catalog.** The catalog has four parsers (`kodik`, `animelib`, `hianime`, `consumet`) and a fifth `hanime` for adult content — none expose original Japanese audio as a first-class track. The `hianime` parser models `Type string // "sub", "dub", "raw"` in `services/catalog/internal/parser/hianime/client.go:79`, but the `raw` branch is unused and the upstream `aniwatch` container has been returning HTTP 500 since the HiAnime takedown in March 2026.

2. **AllAnime is the only verified-live raw source.** Research confirms `https://api.allanime.day/api` returns valid GraphQL responses today (verified with `{__schema{queryType{name}}}`). AllAnime exposes `translationType: raw` explicitly in its episode-source resolution, alongside `sub` and `dub`. Reference implementations: `justfoolingaround/animdl` (Python), `sdaqo/anipy-cli` (Python), `pystardust/ani-cli` (shell). All three use persisted GraphQL queries identified by SHA256 hashes — these hashes rotate every few months.

3. **Rotating domains.** AllAnime has cycled through `allanime.to → allmanga.to → allanime.day` over the past 18 months. A robust parser must iterate a domain list at startup and cache the first responsive domain.

**The implementation:**
- New Go module `services/catalog/internal/parser/allanime/` following the existing parser layout (mirror the structure of `services/catalog/internal/parser/hianime/`).
- Persisted-query SHA hashes in env vars, never hardcoded.
- Catalog DB extended with raw source type in `domain.SourceType`.
- New catalog handler endpoints under `/api/anime/{shikimori_id}/raw/*`.

## Requirements

### RAW-01: AllAnime parser module

- **Current:** No `services/catalog/internal/parser/allanime/` directory. AllAnime is not queried anywhere.
- **Target:** Module exposes the following Go types and methods:
  ```go
  package allanime

  type Client struct { /* http client + config */ }
  func NewClient(cfg Config) *Client

  type Config struct {
      Domains        []string // ordered fallback list
      QuerySearchSHA string
      QueryEpisodesSHA string
      QuerySourcesSHA string
      HTTPTimeout    time.Duration
  }

  type SearchResult struct { ID, Name, JName, Poster string; Episodes int }
  type Episode struct { ID string; Number int; Title string }
  type Stream struct { URL string; Type string; Quality string; Subtitles []Subtitle }
  type Subtitle struct { URL, Lang, Label string }

  func (c *Client) Search(ctx, query string) ([]SearchResult, error)
  func (c *Client) EpisodesByID(ctx, showID string) ([]Episode, error)
  func (c *Client) RawStream(ctx, episodeID string) (Stream, error)
  ```
- **Acceptance:** Unit tests against recorded GraphQL fixtures pass. Integration tests gated on `INTEGRATION=1` hit `api.allanime.day` and return non-empty results for a known title (Bocchi the Rock, MAL 52082).

### RAW-02: Catalog endpoints + raw resolver service

- **Current:** Catalog exposes `/api/anime/{id}/{kodik,animelib,hianime,consumet,hanime}/*` endpoints under `services/catalog/internal/handler/anime.go`. No `/raw/*` route.
- **Target:** Two new endpoints:
  - `GET /api/anime/{shikimori_id}/raw/episodes` — returns `{episodes: [...], available: bool, source: "allanime"}`.
  - `GET /api/anime/{shikimori_id}/raw/stream?episode={n}&quality={q}` — returns `{url: "...", subtitles: [...], quality: "1080p", expires_at: timestamp}`.
  - Backed by a new `services/catalog/internal/service/raw_resolver.go` that wraps the AllAnime client and applies the standard 1-hour cache pattern (`cache.Set(ctx, "raw:stream:"+key, ..., time.Hour)`).
- **Acceptance:** `curl http://localhost:8081/api/anime/52082/raw/episodes` returns ≥10 episodes. `curl ...?episode=1` returns an HLS URL. Second call within 1h is a cache hit (log line or metric confirms).

### RAW-NF-01: Error-handling convention

- **Current:** Existing parsers return raw errors or wrap with `fmt.Errorf`; catalog handlers map to 500.
- **Target:** All AllAnime parser failures wrap with `libs/errors.Wrap`. Specifically:
  - All-domains-timeout → `errors.Unavailable("allanime: all domains unreachable", err)`.
  - GraphQL 4xx response → `errors.Unavailable("allanime: query rejected (likely stale SHA)", err)`.
  - Empty `data.shows.edges` for a search → `errors.NotFound("allanime: no match for query")`.
  - HTTP 5xx upstream → `errors.Unavailable` (not propagated as 500).
- **Acceptance:** Handler returns 503 (not 500) on all upstream failures. Frontend can show "raw provider unavailable" state cleanly.

## Acceptance Criteria

1. `services/catalog/internal/parser/allanime/` exists with `client.go`, `queries.go`, `episodes.go`, `domains.go`.
2. `docker/.env.example` documents the three new env vars (`ALLANIME_QUERY_SEARCH_SHA`, `ALLANIME_QUERY_EPISODES_SHA`, `ALLANIME_QUERY_SOURCES_SHA`).
3. `docker/.env` (not committed) has values populated by the implementer from a live network capture against `allmanga.to`.
4. `services/catalog/internal/domain/anime.go` includes `SourceTypeRaw = "raw"` constant.
5. `GET /api/anime/52082/raw/episodes` returns 200 with non-empty episodes array.
6. `GET /api/anime/52082/raw/stream?episode=1` returns 200 with valid HLS URL.
7. With `ALLANIME_QUERY_SEARCH_SHA=invalid`, the same endpoint returns 503 with body `{"error": "raw provider unavailable"}` (not 500).
8. Stream URL cache hit on second call within 1h verified by Prometheus counter or log line.
9. Unit tests against fixtures pass. Integration tests gated on `INTEGRATION=1` pass against live API.

## Auto-selected implementation decisions

(Logged for review before plan-phase. Override by editing this file or via `/gsd-discuss-phase 1 --ws raw-jp`.)

- **Domain rotation:** First-success caching in memory (process lifetime), no persistence. Re-check fails after 5-minute cooldown.
- **HTTP client:** Standard `net/http` with `Timeout: 10s`. No third-party GraphQL library — direct POST with hand-crafted body, matching the existing `hianime` parser's style.
- **Headers:** `Referer: https://allmanga.to/`, `User-Agent: AnimeEnigma/1.0`. Both env-overridable.
- **Subtitle extraction:** Best-effort — if AllAnime returns subtitle URLs in the stream response, surface them; do not block stream resolution on subtitle parsing failures.
- **Quality selection:** Return all qualities; let the frontend pick. No server-side quality preference.

## Touches

- **New:** `services/catalog/internal/parser/allanime/{client.go,queries.go,episodes.go,domains.go}`
- **New:** `services/catalog/internal/service/raw_resolver.go`
- **New:** `services/catalog/internal/handler/raw.go`
- **Extend:** `services/catalog/internal/transport/router.go` (register routes)
- **Extend:** `services/catalog/internal/domain/anime.go` (`SourceTypeRaw`)
- **Extend:** `docker/.env.example` + `docker/.env`
- **Extend:** `services/catalog/internal/config/config.go` (load AllAnime env vars)

## Out of Scope (for this phase)

- Frontend integration — Phase 3 + 4.
- Subtitle aggregation — Phase 2.
- Self-hosted MinIO fallback — v0.2.
- Replacing the dead HiAnime parser — separate workstream.

## Citations to design doc

- Tech-choices table → "Raw streaming source: AllAnime GraphQL with `translationType: raw`"
- Tech-choices table → "Persisted-query SHA: env config (`ALLANIME_QUERY_*_SHA`)"
- Tech-choices table → "Rotating domains: Static list with first-success caching"
- Configuration section → `ALLANIME_QUERY_SEARCH_SHA` example
- Error-handling table → AllAnime GraphQL 4xx + all-domains-timeout rows
