# Environment Variables

Required for all services: `DB_HOST, DB_PORT, DB_USER, DB_PASSWORD, DB_NAME`, `REDIS_HOST, REDIS_PORT`, `JWT_SECRET`.

Secrets live in `docker/.env` (host-only, git-ignored). Non-secret service-discovery URLs carry sensible in-cluster defaults (below).

**BE egress recorder** (catalog/scraper/streaming — Activity Register v4.0 Phase 2): `ANALYTICS_INTERNAL_URL` (default `http://analytics:8092`) — ship recorded outbound egress effects (host/provider/bytes, one aggregated row per HLS watch session) to analytics `POST /internal/effects` over the Docker network. Non-secret service-discovery URL; producer is non-blocking + drop-on-full (analytics outage never affects requests). `/internal/effects` NOT gateway-proxied (Docker-network-only).

**Catalog:** `SHIKIMORI_CLIENT_ID`, `SHIKIMORI_CLIENT_SECRET`, `KODIK_API_KEY` (if using), `JIMAKU_API_KEY` (if using JP subtitles).

**Streaming:** `MINIO_ENDPOINT`, `MINIO_ACCESS_KEY`, `MINIO_SECRET_KEY`, `MINIO_BUCKET`.

**Gateway** (WV3-T3 per-user rate limit): `RATE_LIMIT_RPS` (per-IP, default 100), `RATE_LIMIT_BURST` (per-IP, default 200), `USER_RATE_LIMIT_PER_MINUTE` (per-auth-user GCRA rate, default 240 — was 60; resized 2026-06-12, profile-page tab prefetch tripped it), `USER_RATE_LIMIT_BURST` (per-auth-user GCRA burst, default 40 — was 10), `REDIS_ADDR` (default `redis:6379`, per-user limiter), `NOTIFICATIONS_SERVICE_URL` (default `http://notifications:8090`, proxies `/api/notifications/*`). The per-user limit layers on top of per-IP and applies AFTER auth (anonymous stays per-IP). A Redis outage fails open (logs WARN, lets the request through) so a Redis blip can't 500 every authed request. Blocked count at `/metrics`: `gateway_rate_limit_user_blocked_total` (no labels).

**Notifications** (workstream notifications, v1.0 Phase 1): `CATALOG_URL` (default `http://catalog:8081` — Phase 2 detector calls catalog's `/internal/anime/{id}/episodes`). Standard `DB_*` + `JWT_SECRET` + `REDIS_HOST`. Internal producer `POST /internal/notifications` is Docker-network-only (gateway doesn't proxy `/internal/*`).

**Recs** (extracted from player, spec 2026-06-11): `CATALOG_URL` (default `http://catalog:8081` — S6 combo-pin Shikimori `/similar` fallback). Standard `DB_*` + `JWT_SECRET` + `REDIS_HOST`. Internal `POST /internal/recs/recompute-hint` (Docker-network-only) gets fire-and-forget watch-activity hints from player. Player config: `RECS_INTERNAL_URL` (default `http://recs:8094`), `RECS_HINT_ENABLED` (default true). Gateway: `RECS_SERVICE_URL` (default `http://recs:8094`).

**Scheduler:** `SUBTITLE_PROBE_CRON` (default `*/5 * * * *` — active subtitle-provider health probe; POSTs catalog's `/internal/subtitle-probe/run`; catalog pings Jimaku + OpenSubtitles cheap non-quota endpoints, records up/degraded/down + latency → `probe_subtitle_*` gauges + `provider_health` overlay on `/subtitles/all`). Standard `DB_*` + `REDIS_HOST` + `JWT_SECRET`. Also runs `SHIKIMORI_SYNC_CRON`, `SCRAPER_PLAYABILITY_CANARY_CRON`, `SUBTITLE_PROBE_CRON`.
