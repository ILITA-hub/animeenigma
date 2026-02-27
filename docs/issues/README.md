# Known Issues & Incidents Log

Track issues discovered during development. Each entry should include root cause analysis and resolution status.

## Active Issues

### ISS-001: Consumet/HiAnime HLS streams blocked by Cloudflare on owocdn.top/uwucdn.top
- **Date:** 2026-02-27
- **Severity:** High (player unusable for affected streams)
- **Affected:** Consumet player (vidcloud server), all browsers
- **Symptom:** Video plays ~0.5s then enters infinite reload loop. Console floods with `bufferAppendError` / `bufferAddCodecError` at ~200ms intervals.
- **Root cause:** Upstream CDN (`vault-*.owocdn.top`) returns Cloudflare 403 HTML challenge page instead of video segments. The HLS proxy was forwarding this HTML with `Content-Type: application/vnd.apple.mpegurl`, causing HLS.js to try parsing HTML as video data, triggering infinite error recovery loop.
- **Contributing factors:**
  - Stream URLs from Consumet API are short-lived and expire quickly
  - Cloudflare may block server IP or require browser challenges the proxy can't solve
  - `uwucdn.top` domain was missing from HLS proxy allowed domains list
- **Fix applied (partial):**
  - Proxy now detects upstream 4xx/5xx errors and returns clean 502 instead of forwarding garbage HTML (commit pending)
  - Added `proxy_upstream_errors_total{status, domain}` Prometheus metric to track CDN failures
  - Added `uwucdn.top` to allowed domains
  - Streaming service logs `upstream CDN error` with domain, status, and whether HTML was returned
- **Remaining work:**
  - Frontend HLS.js error handler should show user-friendly message on 502 instead of generic error
  - Consider auto-switching to alternative server (e.g. vidstreaming) when vidcloud fails
  - Investigate if Consumet API returns stale/expired stream URLs from cache
  - Monitor `proxy_upstream_errors_total` metric in Grafana to track frequency

### ISS-002: uwucdn.top not in HLS proxy allowed domains
- **Date:** 2026-02-27
- **Severity:** Medium (streams from this CDN silently fail)
- **Symptom:** Streaming logs show `domain not allowed for HLS proxy: vault-08.uwucdn.top`
- **Root cause:** Only `owocdn.top` was in the allowed list, but Consumet/Kwik also uses `uwucdn.top` as a mirror domain
- **Fix:** Added `uwucdn.top` to `HLSProxyAllowedDomains` in `libs/videoutils/proxy.go`
- **Status:** Fixed

## Resolved Issues

### ISS-003: Error reports received with empty fields
- **Date:** 2026-02-27
- **Severity:** Medium (reports useless without context)
- **Symptom:** Telegram notifications and server logs showed empty player_type, anime_name, etc.
- **Root cause:** Frontend `diagnostics.ts` sent camelCase JSON keys (`playerType`, `animeId`) but Go struct expected snake_case (`player_type`, `anime_id`). All fields deserialized as zero values.
- **Fix:** Updated `collectDiagnostics()` in `diagnostics.ts` to use snake_case keys matching the Go struct.
- **Status:** Fixed

### ISS-004: Error report data lost on container restart
- **Date:** 2026-02-27
- **Severity:** Medium (can't investigate user reports after deployment)
- **Symptom:** User submitted error report at 06:51 UTC, player container restarted at 08:13 UTC, all report data lost from stdout logs.
- **Root cause:** Reports were only logged to container stdout with no persistent storage.
- **Fix:** Added `player_reports` Docker volume mounted to `/data/reports/`. Each report saved as a JSON file with full diagnostics (console logs, network logs, page HTML). Files persist across container restarts.
- **Status:** Fixed
