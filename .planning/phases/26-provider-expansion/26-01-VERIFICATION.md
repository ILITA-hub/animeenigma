# 26-01 Verification — AllAnime Scraper Provider Lift

**Date:** 2026-05-19
**SCRAPER-HEAL-25** — AllAnime lifted into the scraper failover pool.

## Build + unit tests

```
$ cd services/scraper && go build ./...
(exit 0)

$ cd services/scraper && go vet ./...
(exit 0)

$ cd services/scraper && go test ./internal/providers/allanime/... -race -count=2
ok  	github.com/ILITA-hub/animeenigma/services/scraper/internal/providers/allanime	1.062s
```

Unit tests passing: TestNew_RequiresHTTP, TestNew_RequiresCache, TestNew_Name,
TestFindID_Frieren, TestFindID_NotFound, TestFindID_EmptyTitle, TestListEpisodes_Frieren,
TestListEpisodes_RealEmpty, TestListServers_Frieren_Ep1, TestGetStream_HappyPath,
TestGetStream_NoMatchingServer, TestDoGraphQL_5xxIsProviderDown,
TestDoGraphQL_4xxIsExtractFailed, TestHealthCheck_BootSeedsAllStagesUp,
TestSplitEpisodeID, TestDecodeSourceURL_Passthrough — plus the compile-time
`var _ domain.Provider = (*Provider)(nil)` assertion.

## Redeploy

```
$ make redeploy-scraper
[INFO] Stopping scraper...
[INFO] Removing scraper container...
[INFO] Starting scraper...
[INFO] scraper is running
[INFO] Deployment complete!
[INFO] Checking service health...
```

Scraper boot logs confirm registration:
```
INFO  registered provider {"name": "allanime"}
INFO  scraper.probe: spawned {"provider": "allanime"}
```

## /scraper/health smoke

```
$ curl -sf http://localhost:8088/scraper/health | jq
{
  "success": true,
  "data": {
    "providers": {
      "allanime": {
        "provider": "allanime",
        "stages": {
          "episodes": {"up": true, ...},
          "search":   {"up": true, ...},
          "servers":  {"up": true, ...},
          "stream":   {"up": true, ...},
          "stream_segment": {"up": true, ...}
        }
      }
    }
  }
}
```

All 5 canonical stages present. gogoanime + animepahe currently in
SCRAPER_DEGRADED_PROVIDERS env (pre-Phase 26 state) so they don't show.

## End-to-end Frieren smoke

```
$ curl -s 'http://localhost:8000/api/anime/f0b40660-6627-4a59-8dcf-7ec8596b3623/scraper/episodes?provider=allanime'
{"success":true,"data":{"episodes":[
  {"id":"ReHMC7TQnch3C6z8j:1","number":1,"title":"Episode 1","is_filler":false},
  {"id":"ReHMC7TQnch3C6z8j:2","number":2,"title":"Episode 2","is_filler":false},
  ... (28 episodes total)
]}}
```

LIVE. AllAnime upstream is reachable, FindID resolved Frieren → show ID
`ReHMC7TQnch3C6z8j`, ListEpisodes returned 28 episodes.

## Acceptance criteria

- [x] `services/scraper/internal/providers/allanime/` package exists with
      doc.go, queries.go, decrypt.go, dto.go, cache.go, client.go,
      client_test.go, testdata/ goldens (3 files)
- [x] `var _ domain.Provider = (*Provider)(nil)` compile-time assertion
- [x] All 6 Provider methods (Name, FindID, ListEpisodes, ListServers,
      GetStream, HealthCheck) implemented
- [x] main.go registers allanime between animepahe and the animekai gate
- [x] candidateProviders invariant updated to include "allanime"
- [x] No catalog imports, no anti-bot deps (chromedp/flaresolverr/utls/etc.)
- [x] All upstream HTTP through `domain.BaseHTTPClient`
- [x] Stream returns `Headers["Referer"]="https://allmanga.to"`
- [x] config.AllAnimeConfig + SCRAPER_ALLANIME_BASE_URL env override
- [x] /scraper/health includes allanime with canonical 5 stages
- [x] Live end-to-end smoke: 28 episodes returned for Frieren

## Notes

- Tests reuse a tiny `inMemoryCache` satisfying the full `cache.Cache`
  interface (Get/Set/Delete/Exists/GetOrSet/Invalidate/SetNX/Close).
- `splitEpisodeID` uses `:` separator (not `/` like the catalog parser)
  to avoid collisions with paths in the orchestrator's URL routing.
- Compile-time `var _ domain.Provider = (*Provider)(nil)` lives in
  BOTH client.go (production) and client_test.go (test binary) for
  defence-in-depth.
- The Phase 19 wiring invariant in main.go now requires allanime to be
  registered when not in the degraded list.
