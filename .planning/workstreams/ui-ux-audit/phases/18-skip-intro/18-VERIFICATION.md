# Phase 18 Verification — Skip-Intro detection (Griffin)

**Workstream:** ui-ux-audit
**Plan:** 18-PLAN.md
**Closes:** UX-34
**Date:** 2026-05-13

## Gate results

| Gate | Plan spec | Actual command | Result |
|---|---|---|---|
| 1 | `cd services/catalog && go test ./...` clean | `cd services/catalog && go test ./...` | PASSED — all packages `ok`, 0 failures |
| 2 | `cd frontend/web && bunx vue-tsc --noEmit` clean | same | PASSED — exit 0, no output |
| 3 | `bash scripts/i18n-lint.sh` clean | script does not exist; replaced with `python3 -c json.load` validation on all 3 locale files + presence check for `player.skipIntro` / `player.skipOutro` keys | PASSED — all 3 locales valid JSON, all 6 entries present |
| 4 | `make redeploy-{catalog,gateway,web}` succeed | `make redeploy-catalog`, `make redeploy-gateway`, `make redeploy-web` | PASSED — all three reported healthy; `make health` shows all 8 services up (gateway/auth/catalog/streaming/player/rooms/scheduler/scraper) |
| 5 | grep `useSkipTimes` in HiAnimePlayer + ConsumetPlayer (2+ matches) | `grep -l useSkipTimes frontend/web/src/components/player/{HiAnimePlayer,ConsumetPlayer}.vue` | PASSED — both files match |
| 6 | grep `skip-times` in gateway router + catalog router + client.ts | `grep -c skip-times` on each file | PASSED — gateway router: 2 matches; catalog router: 1 match; client.ts: 1 match |
| 7 | curl `/api/skip-times/52614/1` returns JSON (200) | `curl http://localhost:8000/api/skip-times/52614/1` (Frieren — used 52991 instead, which is the actual Frieren MAL ID; 52614 returns `found:false`) | PASSED — see smoke tests below |
| 8 | Manual: load anime with MAL ID through HiAnime/Consumet, advance to OP start, verify button appears, click → seeks past OP | deferred to manual UAT — backend + frontend wiring verified via static checks + smoke tests below | DEFERRED |

## Smoke tests (gate 7)

```text
$ curl -s "http://localhost:8000/api/skip-times/52991/1" | python3 -m json.tool
{
  "success": true,
  "data": {
    "found": true,
    "results": [
      {
        "interval": {
          "startTime": 3.221,
          "endTime": 93.221
        },
        "skipType": "op",
        "skipId": "c2cacbe5-4247-4ed7-bb64-28e780daf975",
        "episodeLength": 1559.949
      },
      {
        "interval": {
          "startTime": 1417.135,
          "endTime": 1507.135
        },
        "skipType": "ed",
        "skipId": "92965d0e-bd0f-4753-a520-c2d6f2dffc47",
        "episodeLength": 1510.102
      }
    ]
  }
}
```

Frieren ep 1 (MAL 52991): OP at 3.2s–93.2s, ED at 1417s–1507s. CTA window math:
- `showSkipIntro = currentTime ∈ [3.2, 92.2)` → button shows in roughly the first 90 seconds.
- `showSkipOutro = currentTime ∈ [1417, 1506)` → button shows in the last 90 seconds before credits.

### Graceful-degradation cases

```text
$ curl -s "http://localhost:8000/api/skip-times/99999999/1"
{"success":true,"data":{"found":false,"results":[]}}
```
Aniskip 404 → uniform empty shape → `useSkipTimes` returns `opening: null, ending: null` → both buttons stay `v-if=false`.

```text
$ curl -s -w "\nHTTP %{http_code}\n" "http://localhost:8000/api/skip-times/abc/1"
{"success":false,"error":{"code":"INVALID_INPUT","message":"malId must be a positive integer"}}
HTTP 400
```
Non-numeric malId → 400 at the handler, cache untouched, no upstream call. Defense against path-injection into the URL.

```text
$ curl -s -w "\nHTTP %{http_code}\n" "http://localhost:8000/api/skip-times/52991/0"
{"success":false,"error":{"code":"INVALID_INPUT","message":"episode must be a positive integer"}}
HTTP 400
```
Episode 0 → 400. Same defense.

## File-presence and grep verification

```text
# Backend
$ ls services/catalog/internal/handler/skip_times.go
services/catalog/internal/handler/skip_times.go

$ grep -c "skip-times" services/catalog/internal/transport/router.go
1

$ grep -c "SkipTimesHandler" services/catalog/cmd/catalog-api/main.go
2  # one for NewSkipTimesHandler, one for the NewRouter argument

$ grep -c "skip-times" services/gateway/internal/transport/router.go
2  # one comment, one HandleFunc

# Frontend
$ ls frontend/web/src/composables/useSkipTimes.ts
frontend/web/src/composables/useSkipTimes.ts

$ grep -c "useSkipTimes" frontend/web/src/components/player/HiAnimePlayer.vue
2  # import + invocation

$ grep -c "useSkipTimes" frontend/web/src/components/player/ConsumetPlayer.vue
2  # import + invocation

$ grep -c "skip-times" frontend/web/src/api/client.ts
1  # path in getSkipTimes

$ grep -c "skipIntro\|skipOutro" frontend/web/src/locales/{en,ru,ja}.json
frontend/web/src/locales/en.json:2
frontend/web/src/locales/ru.json:2
frontend/web/src/locales/ja.json:2
```

## Service health

```text
$ make health
Checking service health...
✓ gateway:8000
✓ auth:8080
✓ catalog:8081
✓ streaming:8082
✓ player:8083
✓ rooms:8084
✓ scheduler:8085
✓ scraper:8088
```

## Conclusion

**All gates 1–7 PASSED.** Gate 8 (manual UAT through the browser to confirm the button visually appears, clicks register, and the seek lands at OP end) is deferred to the user — the full backend + frontend chain is in place, every static check passes, and the smoke tests confirm the data flow. The CTA will render on any anime with a `malId` whose MAL ID has aniskip submissions (Frieren = MAL 52991, Death Note = MAL 1535, both verified).
