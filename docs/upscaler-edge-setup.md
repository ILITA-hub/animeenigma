# Upscaler Edge Setup — `ext.animeenigma.org`

## Overview

`ext.animeenigma.org` is the **only internet-facing surface** that untrusted GPU workers
dial to report in, receive work, and push upscaled segments back. The trust chain is:

```
GPU Worker (external)
  → Cloudflare (orange-cloud, WAF, rate-limit)
    → nginx (Authenticated Origin Pulls, infra/nginx/ext.animeenigma.org.conf)
      → gateway:8000 (ExternalAPIKeyMiddleware, /worker/* routes)
        → upscaler:8096 (/worker/enroll, /worker/ws, /worker/segments/*)
```

---

## 1. Cloudflare Configuration

### 1.1 Orange-Cloud (Proxied DNS)
The `ext` A record must be **orange-clouded** (proxied) in the Cloudflare dashboard.
This ensures all traffic flows through Cloudflare before reaching the origin.

### 1.2 SSL/TLS Mode
Set to **Full (Strict)**:
- CF presents a valid cert to the browser/worker.
- CF verifies the origin cert when connecting to nginx.

### 1.3 WAF Managed Rules
Enable the **Cloudflare Managed Ruleset** on `ext.animeenigma.org`:
- Provides bot protection, known-bad-IP blocking, and HTTP anomaly detection.
- Tune sensitivity after observing false positives from legitimate GPU worker traffic.

### 1.4 Rate-Limit Rules
Add a CF Rate Limit rule on `ext.animeenigma.org`:
- Path: `/worker/enroll`
- Threshold: 10 requests / 1 minute per IP (adjust per observed enroll cadence)
- Action: Block (429)

Note: `/worker/segments/*` is intentionally NOT rate-limited at CF — segments
are large and infrequent; the token bucket would false-trip on legitimate uploads.
The gateway's per-IP rate limiter already covers enroll + ws (CD-12).

---

## 2. Authenticated Origin Pulls (MANDATORY)

**Without AOP an attacker who discovers the origin IP can bypass Cloudflare entirely**
(skipping WAF, DDoS protection, and rate limiting).

### Setup steps:
1. Download the Cloudflare Origin CA certificate:
   ```
   https://developers.cloudflare.com/ssl/origin-configuration/authenticated-origin-pull/
   ```
   Direct link: `https://support.cloudflare.com/hc/en-us/article_attachments/360044928032`

2. Copy it to the origin server:
   ```bash
   cp cloudflare-origin-ca.pem /etc/nginx/certs/cloudflare-origin-ca.pem
   ```

3. Uncomment these two lines in `infra/nginx/ext.animeenigma.org.conf`:
   ```nginx
   ssl_client_certificate /etc/nginx/certs/cloudflare-origin-ca.pem;
   ssl_verify_client      on;
   ```

4. Reload nginx:
   ```bash
   nginx -t && systemctl reload nginx
   ```

5. In Cloudflare dashboard → SSL/TLS → Origin Server → **Enable Authenticated Origin Pulls**.

6. **Verify**: from a machine that is NOT Cloudflare, send a direct HTTPS request
   to the origin IP. It must fail with a TLS handshake error or 400.
   ```bash
   curl -k https://<origin-ip>/worker/enroll  # must fail
   ```

---

## 3. EXTERNAL_API_KEY — Coarse Defense-in-Depth Filter

The `EXTERNAL_API_KEY` is a **static shared secret** checked by `ExternalAPIKeyMiddleware`
on every `/worker/*` request (constant-time compare, fail-closed when empty).

**This is NOT the auth boundary.** It is a coarse filter that:
- Stops unauthenticated scanning of the `/worker/*` surface.
- Adds a second factor to the CF + AOP trust chain.

**Real per-worker auth** is the `enroll → session → idx-bound-capability` chain
implemented in Tasks 5 and 10. A compromised `EXTERNAL_API_KEY` alone cannot
authorize any upscaling action — sessions are bound to enrolled worker IDs.

### Rotation
1. Generate a new key: `openssl rand -hex 32`
2. Set it in `docker/.env`: `EXTERNAL_API_KEY=<new-value>`
3. Update all registered worker deployments with the new key.
4. Restart the gateway: `make restart-gateway` (no rebuild needed)
5. Verify worker connectivity after restart.

### Why a single shared key is weak
A single key across all operators means:
- One compromised operator exposes all others.
- No per-operator revocation (revoke = change the key for everyone).

**Phase 2 recommendation**: issue per-operator keys stored in the database,
verified by the `/worker/enroll` handler (Task 5). Combined with CF mTLS
(CD-9) for per-device revocation.

---

## 4. nginx Between CF and Gateway

nginx sits between Cloudflare and the gateway container for two reasons:

1. **X-Real-IP chain**: nginx sets `proxy_set_header X-Real-IP $remote_addr;`
   which is the CF IP. The gateway's `RealClientIP` middleware then reads
   `X-Real-IP` as the true client IP for per-IP rate limiting and logging.
   (The gateway is hardened to trust ONLY `X-Real-IP`, not `X-Forwarded-For`
   which is client-spoofable — see `services/gateway/internal/transport/realip.go`.)

2. **WebSocket upgrade**: nginx passes `Upgrade` and `Connection` headers via
   `proxy_set_header Upgrade $http_upgrade; proxy_set_header Connection $connection_upgrade;`
   so the gateway's dedicated WS reverse proxy (`/worker/ws`) receives the
   upgrade handshake intact.

---

## 5. Phase 2 Hardening: CF mTLS (CD-9)

For Phase 1, the API-key gate is the per-worker auth fallback (owner decision).
Phase 2 should add **Cloudflare mTLS**:

- CF issues per-operator client certificates.
- Workers present their cert on every connection.
- CF enforces cert validity before the request even reaches nginx.
- Revoke a specific operator by revoking their cert in the CF dashboard — no
  need to rotate the shared API key for all operators.

Implementation guide: https://developers.cloudflare.com/api-shield/security/mtls/

---

## 6. Backend Defense-in-Depth (upscaler side)

The upscaler's `/api/upscale/*` admin group is gated by `requireGatewayInternal`
(see `services/upscaler/internal/transport/router.go`). This middleware checks for
the `X-Gateway-Internal` header, which the gateway injects on `/api/upscale/*`
proxied requests (JWT + AdminRole path) but NOT on the ext edge `/worker/*` path.

A direct dial to `upscaler:8096/api/upscale/*` without the header returns 404
(deliberately — not 401 — to avoid revealing the gate to unauthorized callers).

**Phase 2 follow-up**: sign the `X-Gateway-Internal` header with HMAC-SHA256
(rotated per-deploy) so a leaked Docker network access cannot impersonate the
gateway by setting a known static header value. Task 6 establishes the separation
contract; the signing is a separate hardening step.

---

## 7. Operational Verification Checklist

After deploying:
- [ ] CF orange-cloud on `ext.animeenigma.org` DNS record
- [ ] CF SSL mode = Full (Strict)
- [ ] CF WAF Managed Ruleset enabled
- [ ] CF Rate Limit on `/worker/enroll` configured
- [ ] AOP enabled in CF dashboard + `ssl_verify_client on` in nginx
- [ ] Direct-dial to origin IP fails (AOP verification)
- [ ] `EXTERNAL_API_KEY` set in `docker/.env` and confirmed non-empty
- [ ] `make restart-gateway` confirms gateway picks up new key
- [ ] Worker enrollment succeeds end-to-end
- [ ] `/worker/ws` WebSocket connects and stays open
- [ ] `/worker/segments/*` returns 200 for a test segment upload
- [ ] `/worker/enroll` returns 401 without `X-API-Key`
- [ ] Request to any non-`/worker/` path on `ext.animeenigma.org` returns 404
