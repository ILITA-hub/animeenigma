# Provider Recovery Log

Daily on-call diary for EN scraper provider recoveries. One entry per run.
Newest entry first. Used by the recovery operator to avoid repeating yesterday's attempt.

---

## 2026-06-24 — miruro

**State before:** `policy=auto, health=down, status=degraded` — reason: `decode_failed on kiwi` (since 2026-06-23T12:00:30)

**Root cause:** Structural probe bug, NOT a miruro failure.
miruro's kiwi inner provider (animepahe-derived) serves AES-128 encrypted HLS
(`#EXT-X-KEY:METHOD=AES-128` in the playlist). The analytics probe validator
(`services/analytics/internal/probe/validator.go`) passes raw segment bytes
to ffprobe to detect a video codec. ffprobe receives encrypted ciphertext and
cannot identify any codec → `no video stream` → `decode_failed`.

The stream itself was fully functional throughout:
- HLS manifest: HTTP 200, 20703 bytes, `application/vnd.apple.mpegurl`
- First segment (vault-16.owocdn.top): HTTP 200, 893760 bytes via streaming proxy
- `scraper resolved stream` log confirms miruro resolved provider_anime_id 180745 successfully at 2026-06-24T02:54:33Z

**Action taken:**
1. Identified root cause: `validator.go` always calls ffprobe on segments, even
   for AES-128 encrypted HLS where ffprobe is guaranteed to fail.
2. Shipped fix `e30badd7` on `main`:
   - Added `hasAES128()` to detect `#EXT-X-KEY:METHOD=AES-128` in any manifest hop
   - When encryption detected, validate via segment reachability (HTTP 200 + bytes)
     instead of video decode
   - Added `TestValidator_AES128SkipsFFprobe` test
3. Redeployed analytics service.
4. Submitted `probe-result pass` → state machine transitioned `down → recovering`.

**Outcome:** ✅ Fix shipped (`e30badd7`), miruro now `health=recovering`.
Next automated probe will pass with the new code and promote miruro toward `up`.

**Next step:** Confirm miruro transitions `recovering → up` at next probe cycle.
Consider auditing other providers (nineanime/okru) for similar probe false-negatives.

---

## Known-hard cases (skip, don't re-attempt unless upstream changes)

- **allanime** — clock.json behind Cloudflare Turnstile (api.allanime.day). Policy=manual. No Go-level fix possible.
- **animefever** — HLS segments 302→ad CDN (sf16-scmcdn-sg.ibytedtos.com) that 403s our egress. Policy=manual.
- **animepahe** — DDoS-Guard→CF managed challenge, sidecar retired 2026-06-24. Policy=disabled.
