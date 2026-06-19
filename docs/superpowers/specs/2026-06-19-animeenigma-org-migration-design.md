# Spec: animeenigma.ru → animeenigma.org migration

**Date:** 2026-06-19
**Status:** Approved design, pending implementation
**Owner decisions:** full migration to `.org`; `.ru` kept live and `301`-bridged for a transition window (a few months) then dropped; dedicated HLS subdomain; MSK RU edge migrated; **no `admin.` subdomain** (admin stays on the `…/admin` path); **BidBerry untouched** (separate app).

---

## 1. Goal

Move the canonical public domain from `animeenigma.ru` to `animeenigma.org` with **zero forced re-login** for existing users, introduce a dedicated **HLS-only** subdomain to isolate heavy segment traffic, and migrate the RU static edge — all delivered in **four independently-verifiable phases**, each self-tested then owner-verified before the next begins.

**Effort metrics (project CONVENTIONS):**
- **UXΔ = +2 (Better)** — seamless cross-domain SSO (no re-login), HLS isolated from the API rate zone; users keep working through their existing bookmarks.
- **CDI = 0.04 * 21** — touches infra (2 hosts' nginx + certs), env/config, frontend HLS URL construction, and two new auth endpoints; spread is moderate, shift is low (mostly additive + config), Effort_Fib = 21.
- **MVQ = Griffin 85%/80%** — methodical, infrastructure-grade migration with a clean rollback at every phase.

---

## 2. Current topology (as-measured)

**Main host (DE server):**
- Host nginx `sites-available/animeenigma.ru` terminates TLS (Certbot ECDSA cert `animeenigma.ru`) and reverse-proxies: `/` → `127.0.0.1:3003` (web SPA), `/api/` → `:8000` (gateway), plus dedicated locations for `/api/watch-together/ws`, `= /api/streaming/image-proxy` (nginx-cached), `/api/auth/` (tight rate zone), `/admin/`, `/ws/`, `/socket.io/`. Port 80 → 301 to https.
- Admin stack (`admin-nginx` container → Grafana/Prometheus/pgAdmin) is bound **`127.0.0.1:8089` only** (SSH-tunnel access). `admin.animeenigma.ru` is **not** a public vhost here — admin is reached via `animeenigma.ru/admin/` → gateway. **This stays unchanged** (path-based, on `.org` after migration).
- `bidberry.animeenigma.ru` is a **separate application** (WB Analytics, app `:11000` + ws-scrcpy). **Out of scope.**

**MSK box (`Maskanya`, `82.146.35.191`, `super.egor.mamonov.fvds.ru`):**
- nginx `sites-available/ru-cdn.conf` listens `:8443 ssl`, `server_name ru.cdn.animeenigma.ru`, Certbot cert `ru.cdn.animeenigma.ru`. Serves static dist/assets to RU users (cuts the 3×250 ms RTT to DE). `:443` is **Xray-Reality (XHTTP)** and **must not be touched**; nftables is **Ansible-managed + `flush ruleset`** — use surgical `nft insert` only, **never `nft -f`**.

**HLS proxy facts (enable the `stream.` subdomain cheaply):**
- Backend hls-proxy already returns `Access-Control-Allow-Origin: *` (`services/streaming/internal/handler/stream.go:176`, `libs/videoutils/proxy.go:226/611`) → cross-subdomain HLS works with **no backend CORS change**.
- All frontend HLS URLs are built as **relative** `/api/streaming/hls-proxy?…` (`composables/unifiedPlayer/useProviderResolver.ts:376`, `SubtitleOverlay.vue:271`, and 7 `*Player.vue` files) → rewriting them through a base-URL knob is sufficient; rewritten child/segment URLs in the m3u8 follow the host the master was fetched from.

**Auth reuse:** the Telegram deep-link flow (`services/auth/internal/service/auth.go:277` `CreateDeepLinkToken` / `:301` `CheckDeepLinkToken`) is a one-time-token-in-Redis → session-issuance pattern. The magic-link handoff reuses this machinery.

---

## 3. Domain inventory (final)

| Hostname | Role | Where | Action |
|---|---|---|---|
| `animeenigma.org` | App (canonical) | Main host | New vhost + cert |
| `stream.animeenigma.org` | **HLS-only** | Main host | New vhost + cert |
| `ru.cdn.animeenigma.org` | RU static edge | MSK box | New `:8443` vhost + cert |
| `stream.animeenigma.ru` | **HLS-only (test bed, Phase 1)** | Main host | New vhost + cert; proves the mechanism on the known-good domain before `.org` exists |
| `animeenigma.ru` | App (legacy) | Main host | Phase 4: full 301-bridge → `.org` |
| `ru.cdn.animeenigma.ru` | RU edge (legacy) | MSK box | Keep serving during transition; dropped with `.ru` |
| `admin.*` | — | — | **Dropped** (path-based `/admin` only) |
| `bidberry.animeenigma.ru` | Separate app | Main host | **Untouched** |

---

## 4. Phased plan (each phase: implement → self-test → **STOP, owner verifies** → next)

### Phase 1 — `stream.animeenigma.ru` HLS subdomain (test bed)

Prove the dedicated-HLS-subdomain mechanism on the existing, known-good `.ru` domain before introducing `.org`.

**Owner dependency:** add DNS `stream.animeenigma.ru` → main host IP.

**Backend/infra (me):**
1. Host nginx `sites-available/stream.animeenigma.ru`: **only** `location = /api/streaming/hls-proxy` (GET + OPTIONS) → `:8000`; dedicated `limit_req` zone (segment floods never touch the app's API zone); **everything else `return 404`**. Port 80 → 301 https.
2. Certbot cert for `stream.animeenigma.ru` (`--nginx`).

**Frontend (me):**
3. Add `VITE_HLS_PROXY_BASE` env (default `''` = same-origin, fully backward-compatible).
4. New helper `hlsProxyUrl(pathAndQuery)` that prepends the base; refactor the **9** relative `/api/streaming/hls-proxy` call sites to use it. **HLS video only** — subtitle and image proxies stay same-origin.

**Self-test (me) — before flipping any global default:**
- `curl https://stream.animeenigma.ru/api/streaming/hls-proxy?url=<signed test url>` → 200 + playlist body + `Access-Control-Allow-Origin: *`.
- `curl https://stream.animeenigma.ru/anything-else` → 404.
- `curl https://stream.animeenigma.ru/api/anime/...` → 404 (only hls-proxy is exposed).
- Manual browser playback check against a preview build with `VITE_HLS_PROXY_BASE=https://stream.animeenigma.ru` (segments + subtitles load; subtitles still same-origin).

**Risk control:** the global prod `VITE_HLS_PROXY_BASE` is flipped **only after** the above passes (a broken subdomain must not break playback for all users). The flip itself is a one-line `.env` change + `web` redeploy, instantly revertible.

**→ STOP. Owner verifies Phase 1.**

---

### Phase 2 — `animeenigma.org` mirror (parallel, `.ru` untouched)

Bring `.org` up as a **full mirror**: both domains serve the same app in parallel; `.ru` keeps working unchanged.

**Owner dependency:** add DNS `animeenigma.org`, `stream.animeenigma.org` → main host IP; `ru.cdn.animeenigma.org` → MSK box IP.

**Main host nginx + TLS (me):**
1. `sites-available/animeenigma.org` — clone of the `.ru` app block (all locations: `/`→`:3003`, `/api/`→`:8000`, ws/auth/image-proxy/admin/ws/socket.io intact). Canonical.
2. `sites-available/stream.animeenigma.org` — the Phase-1 HLS-only block, retargeted name.
3. One Certbot cert, SANs: `-d animeenigma.org -d stream.animeenigma.org`.

**MSK box (me):**
4. `sites-available/ru-cdn-org.conf` — `:8443 ssl`, `server_name ru.cdn.animeenigma.org`, mirroring `ru-cdn.conf`. LE cert for `ru.cdn.animeenigma.org` issued with the **same method as the existing `ru.cdn.animeenigma.ru` cert** (`:443` is Xray-Reality; do not use HTTP-01 there). `.ru` edge keeps serving. nftables: surgical `nft insert` only.

**Env / config (me):**
5. `docker/.env` + `docker/.env.example`: set/point to `.org` — `SITE_URL`, `WATCH_TOGETHER_PUBLIC_BASE_URL`, `FEEDBACK_BASE_URL`, `ALLOWED_WS_ORIGINS`, `CORS_ORIGINS` (include both `.org` and `.ru` during transition), `VITE_MSK_ASSET_HOST=https://ru.cdn.animeenigma.org:8443`, `VITE_HLS_PROXY_BASE=https://stream.animeenigma.org`.
6. Go fallback defaults `https://animeenigma.ru` → `.org`: `services/gateway/internal/config/config.go`, `services/watch-together/internal/config/config.go`, `services/maintenance/internal/config/config.go`.
7. `frontend/web/src/App.vue` mailto → `info@animeenigma.org`.
8. `deploy/kustomize/base/configmap.yaml` `SITE_URL` → `.org`; drop/repoint the `admin.` subdomain in `deploy/kustomize/base/admin/ingress.yaml` (admin is path-based now); `docker/nginx/admin.conf` server_name; CLAUDE.md admin-URL section.
9. CI: `.github/workflows/player-health.yml`, the two canary workflows, `frontend/web/playwright.health.config.ts` base URLs → `.org`.

**External re-registration (owner) — I supply exact values:**
10. CORS / WS origins already include `.org` (step 5). Telegram + OAuth handled in Phase 4 cutover (kept on `.ru` while `.ru` is still primary-of-record), or now if owner prefers — I'll list `setWebhook`, BotFather `/setdomain`, and any Shikimori/MAL redirect URIs.

**Self-test (me):** `curl` matrix — `.org` 200 (app shell), `.org/api/.../health` green, `stream.org` hls-proxy 200 + 404-elsewhere, `ru.cdn.org:8443` 200, `.ru` still 200 (unchanged). Redeploy `web` + the 3 Go services from a **clean `origin/main` worktree** (never the shared dirty tree).

**→ STOP. Owner verifies Phase 2 (both domains live).**

---

### Phase 3 — Magic-link cross-domain SSO (build + test in isolation)

Two new endpoints, built and tested **before** any `.ru` redirect is flipped, so `.ru` keeps serving normally during development.

**Endpoints (friendly top-level paths, nginx-proxied to gateway → auth):**

- **`GET /magic-link-generate?oldurl=<relative-path>`** — served on **`.ru`** (and harmlessly on `.org` too). Reads the caller's `.ru` session cookie:
  - **authenticated** → mint a one-time magic token (Redis `xdomain:<tok>` → userID; **60 s TTL; single-use** via DEL-on-consume; reuse the deeplink Redis pattern) → `302 https://animeenigma.org/magic-link-login?oldurl=<oldurl>&token=<tok>`.
  - **anonymous** → `302 https://animeenigma.org<oldurl>` (plain, no token).
- **`GET /magic-link-login?oldurl=<relative-path>&token=<tok>`** — served on **`.org`**. Consumes + validates the token, issues a normal `.org` session (sets `.org` cookies exactly like login), `302 <oldurl>`. Invalid/expired/used token → `302 <oldurl>` anonymous (no error page; user simply lands logged-out and can log in).

**Security (TDD, locked):**
- `oldurl` sanitized to a **relative path only**: must start with a single `/`, reject `//`, reject any scheme/`@`/backslash → no open-redirect. Default to `/` if invalid.
- Token: 32-byte CSPRNG, 60 s TTL, **single-use** (atomic GET-then-DEL), HTTPS-only. Tiny URL-leak window.
- Endpoints are **idempotent on failure** (always land somewhere safe on `.org`); never 500 on a bad/missing token.
- Built test-first (`auth` service unit tests for token mint/consume/expiry/single-use + `oldurl` sanitizer table-test).

**Self-test (me):** walk the chain by hand with a real `.ru` session cookie —
`GET https://animeenigma.ru/magic-link-generate?oldurl=/anime/<x>` → 302 to `.org/magic-link-login?...&token=...` → 302 to `.org/anime/<x>`, and confirm a valid `.org` session cookie is set (authenticated landing). Re-using the same token → anonymous landing (single-use proven). Tampered/expired token → safe anonymous landing.

**→ STOP. Owner verifies Phase 3 (the redirect chain logs them in on `.org`).**

---

### Phase 4 — Flip full 301-bridge on `.ru`

Make `.ru` funnel **every** request through the magic-link bridge.

**Main host nginx (me) — `.ru` block becomes:**
1. `location ^~ /magic-link-generate` → **proxy** to `:8000` (gateway → auth). *(Not redirected — this is the bridge entry; excluded from the catch-all to prevent a loop.)*
2. `location ^~ /api/streaming/hls-proxy`, `/api/` (non-bridge) → `301 https://animeenigma.org$request_uri` *(stale SPA XHR just 401s on `.org`; harmless).* HLS already points at `stream.*` via the frontend.
3. `location /` (catch-all) → `302 https://animeenigma.ru/magic-link-generate?oldurl=$request_uri` *(stays on `.ru` so the session cookie is sent, then the bridge bounces to `.org`).*
4. Keep the `.ru` cert alive (needed for the TLS handshake before any redirect).

**External re-registration (owner) — now `.org` is primary-of-record:**
5. Telegram `setWebhook` → `https://animeenigma.org/api/auth/telegram/webhook`; BotFather `/setdomain animeenigma.org`. Any Shikimori/MAL OAuth redirect URIs → `.org` (I provide exact current values + the new ones).

**Self-test (me):** anonymous `curl -I https://animeenigma.ru/anime/<x>` → 302 to `…/magic-link-generate?oldurl=/anime/<x>` → 302 to `.org/anime/<x>`. Authenticated browser bookmark to `.ru/anime/<x>` → lands **logged-in** on `.org/anime/<x>`. No redirect loop; `stream.ru`/`stream.org` unaffected.

**→ STOP. Owner verifies Phase 4 (migration complete; `.ru` fully bridged).**

---

## 5. Sessions / cookies — why the bridge is required

A cookie's `Domain` attribute can only be the setting host or a **parent within the same registrable domain (eTLD+1)**. `animeenigma.ru` and `animeenigma.org` have **different public suffixes** (`.ru` vs `.org`), so there is **no common parent** to scope a shared cookie to (`Domain=animeenigma` is rejected — bare-suffix / non-matching-suffix cookies are forbidden). The browser therefore never transmits a `.ru` cookie to `.org`. The **only** way to carry identity across is an explicit token handoff in the URL — the magic-link bridge (Phase 3/4). `COOKIE_SECURE=true` is unchanged; the existing non-rotating 10-yr cookie persists once set on `.org`.

---

## 6. Rollback (per phase)

- **P1:** unset `VITE_HLS_PROXY_BASE` + `web` redeploy → HLS back to same-origin. Remove the `stream.ru` vhost.
- **P2:** `.org` is purely additive; `.ru` untouched → disable the `.org`/`stream.org`/`ru.cdn.org` vhosts. Revert env (keep `.ru` values).
- **P3:** endpoints are unreferenced until Phase 4 → disable routes; zero user impact.
- **P4:** restore the original `.ru` app vhost (saved verbatim in Phase 2) + reload → `.ru` serves the app again instantly.

The original `.ru` vhost is copied to a `.bak` before Phase 4 edits (a `.bak.YYYYMMDD` convention already exists in `sites-available/`).

---

## 7. Out of scope

- `bidberry.animeenigma.ru` (separate app).
- `admin.` subdomain (admin stays path-based at `…/admin`).
- Dropping `.ru` and `ru.cdn.animeenigma.ru` (a later, separate step "in a few months").
- Any change to MSK `:443` Xray-Reality or a full nftables rewrite.

---

## 8. Manual (owner) steps summary

1. **DNS:** P1 `stream.animeenigma.ru`; P2 `animeenigma.org`, `stream.animeenigma.org`, `ru.cdn.animeenigma.org`.
2. **External:** P4 Telegram `setWebhook` + BotFather `/setdomain`; Shikimori/MAL OAuth redirect URIs (exact values supplied by me).
3. **Verify gate** at the end of each phase.
