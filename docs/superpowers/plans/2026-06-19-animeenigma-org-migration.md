# animeenigma.ru → animeenigma.org Migration — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Migrate the canonical public domain from `animeenigma.ru` to `animeenigma.org` with zero forced re-login (cross-domain magic-link SSO), isolate HLS onto `stream.animeenigma.org`, and migrate the MSK RU static edge — delivered in 4 owner-verified phases.

**Architecture:** Four sequential phases, each self-tested then **STOP for owner verification**. Phase 1 proves the HLS-subdomain mechanism on the existing `.ru` domain. Phase 2 brings `.org` up as a parallel mirror. Phase 3 builds + tests the magic-link bridge in isolation. Phase 4 flips the full `.ru`→`.org` 301-bridge.

**Tech Stack:** Go (chi router, Redis cache via `libs/cache`, JWT sessions), Vue 3 + Vite frontend, host nginx + Certbot (DE main host + MSK box), Docker Compose deploy.

**Spec:** `docs/superpowers/specs/2026-06-19-animeenigma-org-migration-design.md`

**Cross-cutting rules (apply to every task):**
- Deploy by building from a **clean `origin/main` worktree** (copy `docker/.env`; compose project stays `docker`), never the shared dirty tree. Commit path-scoped (`git commit <pathspec>`), then `git push`. If push is rejected, cherry-pick onto `origin/main` in a worktree and push from there.
- `git show --stat HEAD` after every commit.
- New i18n keys (none expected here) go in all three locales.
- **Never set `COOKIE_DOMAIN`** — host-only cookies are required for the dual-domain model.
- nginx edits on the MSK box: surgical `nft insert` only, **never `nft -f`**; do not touch `:443` (Xray-Reality).

---

# PHASE 1 — `stream.animeenigma.ru` HLS test bed

**Owner dependency (BLOCKER):** owner adds DNS `stream.animeenigma.ru` → main host IP before Task 1.4.

## Task 1.1: Frontend `hlsProxyUrl()` helper (TDD)

**Files:**
- Create: `frontend/web/src/utils/streaming.ts`
- Test: `frontend/web/src/utils/streaming.spec.ts`

- [ ] **Step 1: Write the failing test**

```typescript
// frontend/web/src/utils/streaming.spec.ts
import { describe, it, expect, afterEach, vi } from 'vitest'
import { hlsProxyUrl } from './streaming'

afterEach(() => {
  vi.unstubAllEnvs()
})

describe('hlsProxyUrl()', () => {
  it('defaults to a same-origin relative path when no base is configured', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', '')
    expect(hlsProxyUrl('url=https%3A%2F%2Fx.test%2Fa.m3u8')).toBe(
      '/api/streaming/hls-proxy?url=https%3A%2F%2Fx.test%2Fa.m3u8',
    )
  })

  it('prepends the configured base (no trailing slash) for an absolute subdomain', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', 'https://stream.animeenigma.ru')
    expect(hlsProxyUrl('url=abc')).toBe(
      'https://stream.animeenigma.ru/api/streaming/hls-proxy?url=abc',
    )
  })

  it('strips a trailing slash on the configured base', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', 'https://stream.animeenigma.ru/')
    expect(hlsProxyUrl('url=abc')).toBe(
      'https://stream.animeenigma.ru/api/streaming/hls-proxy?url=abc',
    )
  })

  it('accepts an empty query string', () => {
    vi.stubEnv('VITE_HLS_PROXY_BASE', '')
    expect(hlsProxyUrl('')).toBe('/api/streaming/hls-proxy?')
  })
})
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd frontend/web && bunx vitest run src/utils/streaming.spec.ts`
Expected: FAIL — `hlsProxyUrl` is not exported / module not found.

- [ ] **Step 3: Write minimal implementation**

```typescript
// frontend/web/src/utils/streaming.ts

/**
 * Builds an HLS-proxy URL. By default returns a same-origin relative path
 * (`/api/streaming/hls-proxy?<query>`). When `VITE_HLS_PROXY_BASE` is set
 * (e.g. `https://stream.animeenigma.org`), the URL is rooted at that host so
 * heavy segment traffic is served from the dedicated HLS subdomain. The proxy
 * already sends `Access-Control-Allow-Origin: *`, so cross-subdomain fetches
 * work without further CORS changes.
 *
 * Scope: HLS video + subtitle-track fetches only. Image proxy stays same-origin.
 *
 * @param query the already-encoded query string WITHOUT the leading `?`
 */
export function hlsProxyUrl(query: string): string {
  const base = (import.meta.env.VITE_HLS_PROXY_BASE || '').replace(/\/+$/, '')
  return `${base}/api/streaming/hls-proxy?${query}`
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd frontend/web && bunx vitest run src/utils/streaming.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
git add frontend/web/src/utils/streaming.ts frontend/web/src/utils/streaming.spec.ts
git commit frontend/web/src/utils/streaming.ts frontend/web/src/utils/streaming.spec.ts \
  -m "feat(player): hlsProxyUrl() helper for routing HLS through a subdomain"
git show --stat HEAD
```

## Task 1.2: Declare `VITE_HLS_PROXY_BASE` type

**Files:**
- Modify: `frontend/web/src/vite-env.d.ts`

- [ ] **Step 1: Add the type to `ImportMetaEnv`**

Add this line inside the `interface ImportMetaEnv { ... }` block (alongside the existing `readonly VITE_API_URL: string` etc.):

```typescript
  readonly VITE_HLS_PROXY_BASE?: string
```

- [ ] **Step 2: Type-check**

Run: `cd frontend/web && bunx vue-tsc --noEmit`
Expected: no new errors.

- [ ] **Step 3: Commit**

```bash
git commit frontend/web/src/vite-env.d.ts \
  -m "chore(types): declare VITE_HLS_PROXY_BASE"
git show --stat HEAD
```

## Task 1.3: Refactor the 9 HLS call sites to use the helper

Each call site currently ends a proxy-URL builder with a literal `` `/api/streaming/hls-proxy?${params.toString()}` `` (or an inline equivalent). Replace each with the helper. **Image proxy is OUT of scope — do not touch `useImageProxy.ts`.**

**Files (each: add the import + replace the return expression):**

- [ ] **Step 1: `frontend/web/src/composables/unifiedPlayer/useProviderResolver.ts`**
  - Add at top (with other imports): `import { hlsProxyUrl } from '@/utils/streaming'`
  - Replace line ~376 `return \`/api/streaming/hls-proxy?${params.toString()}\`` with:
    `return hlsProxyUrl(params.toString())`

- [ ] **Step 2: `frontend/web/src/components/player/SubtitleOverlay.vue`** (in-scope — subtitle CORS fetch)
  - Add `import { hlsProxyUrl } from '@/utils/streaming'` in the `<script setup>` imports.
  - Replace the ternary tail at line ~271:
    ```typescript
    const fetchUrl = url.startsWith('/')
      ? url
      : hlsProxyUrl(`url=${encodeURIComponent(url)}`)
    ```

- [ ] **Step 3: `frontend/web/src/components/player/HanimePlayer.vue`** — import helper; replace `:204` return with `return hlsProxyUrl(params.toString())`.
- [ ] **Step 4: `frontend/web/src/components/player/KodikAdFreePlayer.vue`** — import helper; replace `:481` return with `return hlsProxyUrl(params.toString())`.
- [ ] **Step 5: `frontend/web/src/components/player/OurEnglishPlayer.vue`** — import helper; replace BOTH `:404` and `:417` returns with `return hlsProxyUrl(params.toString())`.
- [ ] **Step 6: `frontend/web/src/components/player/AnimeLibPlayer.vue`** — import helper; replace `:681` return with `return hlsProxyUrl(params.toString())`.
- [ ] **Step 7: `frontend/web/src/components/player/RawPlayer.vue`** — import helper; replace `:301` return with `return hlsProxyUrl(params.toString())`.
- [ ] **Step 8: `frontend/web/src/components/player/Anime18Player.vue`** — import helper; replace `:203` return with `return hlsProxyUrl(params.toString())`.

- [ ] **Step 9: Verify no stray literals remain**

Run: `cd frontend/web && grep -rn "streaming/hls-proxy" src/ | grep -v "utils/streaming"`
Expected: zero matches (every builder now goes through `hlsProxyUrl`). The only file naming the literal path is `src/utils/streaming.ts`.

- [ ] **Step 10: Type-check + existing player specs**

Run:
```bash
cd frontend/web && bunx vue-tsc --noEmit
bunx vitest run src/components/player/ src/composables/unifiedPlayer/
```
Expected: PASS / no new type errors. (If a player spec asserted the literal proxy string, update it to expect the same value via `hlsProxyUrl` default — same string when `VITE_HLS_PROXY_BASE` is unset.)

- [ ] **Step 11: Commit**

```bash
git commit src/composables/unifiedPlayer/useProviderResolver.ts \
  src/components/player/SubtitleOverlay.vue \
  src/components/player/HanimePlayer.vue \
  src/components/player/KodikAdFreePlayer.vue \
  src/components/player/OurEnglishPlayer.vue \
  src/components/player/AnimeLibPlayer.vue \
  src/components/player/RawPlayer.vue \
  src/components/player/Anime18Player.vue \
  -m "refactor(player): route all HLS-proxy URLs through hlsProxyUrl()"
git show --stat HEAD
```

## Task 1.4: nginx vhost + cert for `stream.animeenigma.ru` (infra)

**Files (host, not in repo):** `/etc/nginx/sites-available/stream.animeenigma.ru`

- [ ] **Step 1: Confirm DNS resolves** (owner dependency)

Run: `dig +short stream.animeenigma.ru`
Expected: the main host's public IP. If empty, STOP — ask owner to add the A record.

- [ ] **Step 2: Write the HLS-only vhost**

Create `/etc/nginx/sites-available/stream.animeenigma.ru`:

```nginx
# HLS-only edge for AnimeEnigma. Exposes ONLY the hls-proxy endpoint so heavy
# segment traffic is isolated from the app's API rate zone. Everything else 404s.
server {
    server_name stream.animeenigma.ru;
    client_max_body_size 1M;
    limit_req_status 429;

    # Dedicated rate zone (declared in nginx.conf http{}: `limit_req_zone ...
    # zone=hls:10m rate=60r/s;` — add it if absent). Segment floods never touch
    # the app's `api` zone.
    location = /api/streaming/hls-proxy {
        limit_req zone=hls burst=120 nodelay;
        proxy_pass http://127.0.0.1:8000;
        include snippets/proxy-params.conf;
    }

    location / { return 404; }

    listen 443 ssl http2;
    # certbot fills ssl_certificate lines in Step 4
}

server {
    server_name stream.animeenigma.ru;
    listen 80;
    return 301 https://$host$request_uri;
}
```

- [ ] **Step 3: Add the `hls` rate zone if missing**

Run: `grep -n "zone=hls" /etc/nginx/nginx.conf`
If absent, add inside the `http { }` block (near the other `limit_req_zone` lines):
```nginx
limit_req_zone $binary_remote_addr zone=hls:10m rate=60r/s;
```

- [ ] **Step 4: Enable + issue cert**

```bash
ln -sf /etc/nginx/sites-available/stream.animeenigma.ru /etc/nginx/sites-enabled/
nginx -t
certbot --nginx -d stream.animeenigma.ru --non-interactive --agree-tos -m info@animeenigma.ru
nginx -t && systemctl reload nginx
```
Expected: `nginx -t` ok; certbot issues an ECDSA cert and rewrites the `listen 443` block.

## Task 1.5: Self-test (no global flip yet)

- [ ] **Step 1: hls-proxy reachable through the subdomain**

Obtain a known-good signed hls-proxy query (copy one from a live playback session's network tab, or use a same-origin one via `https://animeenigma.ru/...` to confirm the upstream works). Then:
```bash
curl -sI "https://stream.animeenigma.ru/api/streaming/hls-proxy?url=<encoded>&exp=<e>&sig=<s>"
```
Expected: `200` (or `302`/playlist), header `access-control-allow-origin: *`.

- [ ] **Step 2: Everything else 404**

```bash
curl -so /dev/null -w "%{http_code}\n" https://stream.animeenigma.ru/
curl -so /dev/null -w "%{http_code}\n" https://stream.animeenigma.ru/api/anime/xyz
```
Expected: `404` for both.

- [ ] **Step 3: Preview-build browser playback check**

Build a local preview with the env set, do NOT deploy globally yet:
```bash
cd frontend/web && VITE_HLS_PROXY_BASE=https://stream.animeenigma.ru bun run build && bun run preview
```
Open the preview, play an episode (any HLS provider), confirm segments load from `stream.animeenigma.ru` (network tab) and subtitles still load. 

- [ ] **Step 4: STOP — owner verifies Phase 1.** Do not flip the global `VITE_HLS_PROXY_BASE` until owner confirms. (Global flip is a `docker/.env` one-liner + `make redeploy-web`, done in Phase 2.)

---

# PHASE 2 — `animeenigma.org` mirror (parallel; `.ru` untouched)

**Owner dependency (BLOCKER):** DNS `animeenigma.org`, `stream.animeenigma.org` → main host IP; `ru.cdn.animeenigma.org` → MSK box IP (`82.146.35.191`).

## Task 2.1: Main-host `.org` app vhost + `stream.org` + cert (infra)

**Files (host):** `/etc/nginx/sites-available/animeenigma.org`, `/etc/nginx/sites-available/stream.animeenigma.org`

- [ ] **Step 1: Clone the app vhost**

```bash
cp /etc/nginx/sites-available/animeenigma.ru /etc/nginx/sites-available/animeenigma.org
```
Edit `/etc/nginx/sites-available/animeenigma.org`: change every `server_name animeenigma.ru;` → `server_name animeenigma.org;`. Remove the certbot-managed `ssl_certificate*` lines + the `if ($host = animeenigma.ru)` redirect lines (certbot re-adds them in Step 3). Keep ALL `location` blocks identical (`/`→`:3003`, `/api/`→`:8000`, ws/auth/image-proxy/admin/ws/socket.io).

- [ ] **Step 2: Clone the HLS vhost**

```bash
sed 's/stream.animeenigma.ru/stream.animeenigma.org/' \
  /etc/nginx/sites-available/stream.animeenigma.ru \
  > /etc/nginx/sites-available/stream.animeenigma.org
```
Remove the certbot `ssl_certificate*` lines (re-added in Step 3).

- [ ] **Step 3: Enable + issue one cert with both SANs**

```bash
ln -sf /etc/nginx/sites-available/animeenigma.org /etc/nginx/sites-enabled/
ln -sf /etc/nginx/sites-available/stream.animeenigma.org /etc/nginx/sites-enabled/
nginx -t
certbot --nginx -d animeenigma.org -d stream.animeenigma.org \
  --non-interactive --agree-tos -m info@animeenigma.ru
nginx -t && systemctl reload nginx
```

- [ ] **Step 4: Verify both serve**

```bash
curl -so /dev/null -w "%{http_code}\n" https://animeenigma.org/
curl -so /dev/null -w "%{http_code}\n" https://stream.animeenigma.org/api/streaming/hls-proxy
curl -so /dev/null -w "%{http_code}\n" https://animeenigma.ru/   # still 200, untouched
```
Expected: `.org` 200, `stream.org` 400/200 (reachable, not 404-host), `.ru` 200.

## Task 2.2: MSK box `ru.cdn.animeenigma.org` (infra)

**Files (MSK box, `ssh Maskanya`):** `/etc/nginx/sites-available/ru-cdn-org.conf`

- [ ] **Step 1: Inspect the existing edge + its cert issuance method**

```bash
ssh Maskanya 'cat /etc/nginx/sites-available/ru-cdn.conf; echo ---; ls -la /etc/letsencrypt/renewal/ru.cdn.animeenigma.ru.conf; grep -i "authenticator\|installer" /etc/letsencrypt/renewal/ru.cdn.animeenigma.ru.conf'
```
Note the `authenticator` (likely `webroot` or `standalone` on `:80`, since `:443` is Xray-Reality). Mirror it for the `.org` cert.

- [ ] **Step 2: Clone the vhost for `.org`**

```bash
ssh Maskanya 'sed "s/ru.cdn.animeenigma.ru/ru.cdn.animeenigma.org/g" /etc/nginx/sites-available/ru-cdn.conf > /etc/nginx/sites-available/ru-cdn-org.conf'
```
The `ssl_certificate` paths now point at `…/ru.cdn.animeenigma.org/…` (issued next step).

- [ ] **Step 3: Issue the `.org` cert with the SAME authenticator as the `.ru` cert**

Using whichever method Step 1 revealed (example for webroot):
```bash
ssh Maskanya 'certbot certonly --webroot -w <webroot-from-step1> -d ru.cdn.animeenigma.org --non-interactive --agree-tos -m info@animeenigma.ru'
```
**Do NOT** use `--nginx` if `:80`/`:443` conflicts with Xray; **never** run `nft -f`. If only DNS-01 is viable, follow the same manual DNS-01 path used for the `.ru` cert.

- [ ] **Step 4: Enable + reload (surgical)**

```bash
ssh Maskanya 'ln -sf /etc/nginx/sites-available/ru-cdn-org.conf /etc/nginx/sites-enabled/ && nginx -t && systemctl reload nginx'
curl -sko /dev/null -w "%{http_code}\n" https://ru.cdn.animeenigma.org:8443/
```
Expected: `200`. The `.ru` edge keeps serving (untouched).

## Task 2.3: Env config → `.org` (`docker/.env` + `docker/.env.example`)

**Files:** `docker/.env` (live, gitignored), `docker/.env.example`

- [ ] **Step 1: Edit `docker/.env`** (set/add these; create lines that don't exist):

```ini
SITE_URL=https://animeenigma.org
WATCH_TOGETHER_PUBLIC_BASE_URL=https://animeenigma.org
FEEDBACK_BASE_URL=https://animeenigma.org
ALLOWED_WS_ORIGINS=https://animeenigma.org,https://animeenigma.ru
CORS_ORIGINS=https://animeenigma.org,https://animeenigma.ru
VITE_MSK_ASSET_HOST=https://ru.cdn.animeenigma.org:8443
VITE_HLS_PROXY_BASE=https://stream.animeenigma.org
TELEGRAM_WEBHOOK_URL=https://animeenigma.org/api/auth/telegram/webhook
MAGIC_LINK_TARGET_BASE=https://animeenigma.org
```
**Leave `COOKIE_SECURE=true`. Do NOT add `COOKIE_DOMAIN`** (host-only cookies required).

- [ ] **Step 2: Mirror the documented defaults in `docker/.env.example`**

Update the existing `.ru` example/comment lines (`# Production default: https://animeenigma.ru`, `WATCH_TOGETHER_PUBLIC_BASE_URL`, `ALLOWED_WS_ORIGINS` example, `PGADMIN_EMAIL`) to `.org`, and add commented examples for `VITE_HLS_PROXY_BASE`, `MAGIC_LINK_TARGET_BASE`, `SITE_URL`.

- [ ] **Step 3: Commit** (`.env.example` only — `.env` is gitignored)

```bash
git commit docker/.env.example -m "chore(env): document .org defaults + VITE_HLS_PROXY_BASE/MAGIC_LINK_TARGET_BASE"
git show --stat HEAD
```

## Task 2.4: Go fallback defaults `.ru` → `.org`

**Files:**
- `services/gateway/internal/config/config.go:22` (SiteURL doc comment only — default stays env-driven)
- `services/watch-together/internal/config/config.go:111` (and the `:10/:42` doc comments)
- `services/maintenance/internal/config/config.go:117`

- [ ] **Step 1: watch-together default**

In `services/watch-together/internal/config/config.go`, change:
```go
PublicBaseURL: strings.TrimRight(getEnv("WATCH_TOGETHER_PUBLIC_BASE_URL", "https://animeenigma.org"), "/"),
```
and update the two doc comments (`default https://animeenigma.ru` → `.org`).

- [ ] **Step 2: maintenance default**

In `services/maintenance/internal/config/config.go:117`:
```go
FeedbackBaseURL: getEnv("FEEDBACK_BASE_URL", "https://animeenigma.org"),
```

- [ ] **Step 3: gateway doc comment** — update the `// e.g. "https://animeenigma.ru"` comment at `config.go:22` to `.org` (no behavior change; SiteURL is env-driven).

- [ ] **Step 4: Build the three services**

```bash
cd services/watch-together && go build ./... && cd ../..
cd services/maintenance && go build ./... && cd ../..
cd services/gateway && go build ./... && cd ../..
```
Expected: clean builds.

- [ ] **Step 5: Commit**

```bash
git commit services/watch-together/internal/config/config.go \
  services/maintenance/internal/config/config.go \
  services/gateway/internal/config/config.go \
  -m "chore(config): default public base URLs to animeenigma.org"
git show --stat HEAD
```

## Task 2.5: Frontend mailto + kustomize + admin.conf + CI

**Files:**
- `frontend/web/src/App.vue:98-99`
- `deploy/kustomize/base/configmap.yaml:28`
- `deploy/kustomize/base/admin/ingress.yaml:18-22`
- `docker/nginx/admin.conf:3`
- `.github/workflows/player-health.yml`, the two canary workflows, `frontend/web/playwright.health.config.ts`
- CLAUDE.md "Admin URLs (Kubernetes)" section

- [ ] **Step 1: App.vue mailto** — replace both `info@animeenigma.ru` (href + text) with `info@animeenigma.org`.
- [ ] **Step 2: kustomize configmap** — `SITE_URL: "https://animeenigma.org"`.
- [ ] **Step 3: Drop the admin subdomain** — in `deploy/kustomize/base/admin/ingress.yaml`, change `admin.animeenigma.ru` → `admin.animeenigma.org` is NOT wanted; instead the owner uses path-based `/admin`. Replace the host with `animeenigma.org` and the path with `/admin` (mirror the main ingress), OR delete the admin ingress if the main ingress already routes `/admin`. Add a comment: `# admin is path-based (animeenigma.org/admin); no admin.* subdomain`.
- [ ] **Step 4: admin.conf** — `docker/nginx/admin.conf:3` change `server_name admin.animeenigma.ru localhost;` → `server_name localhost;` (container is localhost-bound; the public subdomain is dropped).
- [ ] **Step 5: CI + playwright** — replace `animeenigma.ru` base URLs with `animeenigma.org` in `.github/workflows/player-health.yml`, `.github/workflows/ourenglish-playability-canary.yml`, `.github/workflows/watch-together-kodik-canary.yml`, and `frontend/web/playwright.health.config.ts`. (Leave external `.ru` provider hosts like anilist/kodik untouched — only AnimeEnigma's own domain.)
- [ ] **Step 6: CLAUDE.md** — update the "Admin URLs (Kubernetes)" block (`admin.animeenigma.ru/...`) to note admin is path-based at `animeenigma.org/admin` (Grafana/Prometheus/pgAdmin reached via the gateway `/admin/*` route).

- [ ] **Step 7: Lint + type-check frontend**

```bash
cd frontend/web && bunx vue-tsc --noEmit && bash scripts/design-system-lint.sh
```
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git commit frontend/web/src/App.vue deploy/kustomize/base/configmap.yaml \
  deploy/kustomize/base/admin/ingress.yaml docker/nginx/admin.conf \
  .github/workflows/player-health.yml \
  .github/workflows/ourenglish-playability-canary.yml \
  .github/workflows/watch-together-kodik-canary.yml \
  frontend/web/playwright.health.config.ts CLAUDE.md \
  -m "chore(migration): point app domain refs at animeenigma.org; drop admin subdomain"
git show --stat HEAD
git push
```

## Task 2.6: Deploy + self-test

- [ ] **Step 1: Deploy from a clean worktree** (commit+push first, done above)

```bash
git fetch origin
WT=$(mktemp -d /tmp/ae-deploy.XXXXXX); git worktree add "$WT" origin/main
cp docker/.env "$WT/docker/.env"
cd "$WT" && make redeploy-web && make redeploy-gateway && make redeploy-watch-together && make redeploy-maintenance
cd - && git worktree remove "$WT" --force
```

- [ ] **Step 2: Curl matrix**

```bash
for u in https://animeenigma.org/ "https://animeenigma.org/status/health" \
  "https://stream.animeenigma.org/api/streaming/hls-proxy" \
  "https://ru.cdn.animeenigma.org:8443/" https://animeenigma.ru/ ; do
  echo "$u -> $(curl -sko /dev/null -w '%{http_code}' "$u")"; done
make health
```
Expected: `.org` 200, health green, `.ru` still 200, `ru.cdn.org:8443` 200.

- [ ] **Step 3: Browser smoke** — load `https://animeenigma.org`, log in, play an episode; confirm HLS now loads from `stream.animeenigma.org` (the global `VITE_HLS_PROXY_BASE` is live).

- [ ] **Step 4: STOP — owner verifies Phase 2** (both domains live, HLS on the subdomain).

---

# PHASE 3 — Magic-link cross-domain SSO (build + test in isolation)

`.ru` still serves the app normally; the two endpoints exist but are only exercised by hand until Phase 4.

## Task 3.1: Redis key + TTL for magic tokens

**Files:**
- `libs/cache/keyclass.go`
- `libs/cache/ttl.go`

- [ ] **Step 1: Add the prefix + key builder** in `libs/cache/keyclass.go`

Add to the prefix const block: `PrefixXDomainMagic = "xdomain:"`. Add the builder:
```go
// KeyXDomainMagic is the Redis key for a one-time cross-domain SSO handoff token.
func KeyXDomainMagic(token string) string {
	return PrefixXDomainMagic + token
}
```
If there is a `KeyClass()` switch, add `case PrefixXDomainMagic: return "xdomain"`.

- [ ] **Step 2: Add the TTL** in `libs/cache/ttl.go`

```go
	// One-time cross-domain SSO handoff token. Deliberately tiny — the token
	// rides in a URL during a single redirect chain.
	TTLXDomainMagic = 60 * time.Second
```

- [ ] **Step 3: Build + commit**

```bash
cd libs/cache && go build ./... && cd ../..
git commit libs/cache/keyclass.go libs/cache/ttl.go \
  -m "feat(cache): xdomain magic-token key + 60s TTL"
git show --stat HEAD
```

## Task 3.2: `oldurl` sanitizer (TDD, pure func)

**Files:**
- Create: `services/auth/internal/service/magiclink.go`
- Test: `services/auth/internal/service/magiclink_test.go`

- [ ] **Step 1: Write the failing test**

```go
// services/auth/internal/service/magiclink_test.go
package service

import "testing"

func TestSanitizeOldURL(t *testing.T) {
	cases := []struct{ in, want string }{
		{"/anime/abc", "/anime/abc"},
		{"/anime/abc?x=1&y=2", "/anime/abc?x=1&y=2"},
		{"", "/"},
		{"//evil.com", "/"},
		{"/\\evil.com", "/"},
		{"https://evil.com", "/"},
		{"http://evil.com/x", "/"},
		{"javascript:alert(1)", "/"},
		{"/path with space", "/path with space"},
		{"relative/no/leading/slash", "/"},
		{"/\t/control", "/"},
	}
	for _, c := range cases {
		if got := SanitizeOldURL(c.in); got != c.want {
			t.Errorf("SanitizeOldURL(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
```

- [ ] **Step 2: Run — verify it fails**

Run: `cd services/auth && go test ./internal/service/ -run TestSanitizeOldURL`
Expected: FAIL — `SanitizeOldURL` undefined.

- [ ] **Step 3: Implement**

```go
// services/auth/internal/service/magiclink.go
package service

import "strings"

// SanitizeOldURL constrains a caller-supplied return path to a safe SAME-ORIGIN
// relative path, defeating open-redirect. It must start with a single '/', must
// not start with '//' or '/\' (protocol-relative), must contain no scheme and no
// ASCII control chars. Anything else collapses to "/".
func SanitizeOldURL(raw string) string {
	if raw == "" || raw[0] != '/' {
		return "/"
	}
	if strings.HasPrefix(raw, "//") || strings.HasPrefix(raw, "/\\") {
		return "/"
	}
	if strings.Contains(raw, "://") {
		return "/"
	}
	for _, r := range raw {
		if r < 0x20 || r == 0x7f {
			return "/"
		}
	}
	return raw
}
```

- [ ] **Step 4: Run — verify it passes**

Run: `cd services/auth && go test ./internal/service/ -run TestSanitizeOldURL -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git commit services/auth/internal/service/magiclink.go services/auth/internal/service/magiclink_test.go \
  -m "feat(auth): SanitizeOldURL open-redirect guard for magic-link"
git show --stat HEAD
```

## Task 3.3: `MintMagicToken` service method (TDD)

Resolves the caller's `.ru` session (from the refresh-token cookie value) to a user ID and stores a one-time token. Returns `""` (no error) when the caller is anonymous/invalid.

**Files:**
- Modify: `services/auth/internal/service/magiclink.go`
- Modify: `services/auth/internal/service/magiclink_test.go`

- [ ] **Step 1: Add the domain type + token generator** to `magiclink.go`

```go
import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/ILITA-hub/animeenigma/libs/cache"
)

// xdomainMagicSession is the Redis value behind a magic token.
type xdomainMagicSession struct {
	UserID string `json:"user_id"`
}

const magicTokenPrefix = "ml_"

func generateMagicToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("rand: %w", err)
	}
	return magicTokenPrefix + hex.EncodeToString(b), nil
}

// MintMagicToken resolves a refresh-token cookie value to its user and stores a
// one-time cross-domain handoff token. Returns ("", nil) when the refresh token
// is missing/invalid/revoked (caller is anonymous — no token minted).
func (s *AuthService) MintMagicToken(ctx context.Context, refreshToken string) (string, error) {
	refreshToken = strings.TrimSpace(refreshToken)
	if refreshToken == "" {
		return "", nil
	}
	session, err := s.sessionRepo.FindAliveByHash(ctx, hashRefreshToken(refreshToken))
	if err != nil {
		return "", nil // anonymous / revoked — not an error
	}
	token, err := generateMagicToken()
	if err != nil {
		return "", err
	}
	val := &xdomainMagicSession{UserID: session.UserID}
	if err := s.cache.Set(ctx, cache.KeyXDomainMagic(token), val, cache.TTLXDomainMagic); err != nil {
		return "", fmt.Errorf("store magic token: %w", err)
	}
	return token, nil
}
```

- [ ] **Step 2: Write the failing test** (append to `magiclink_test.go`)

Use the existing in-package test fakes for `sessionRepo` + `cache`. Inspect `services/auth/internal/service/auth_session_test.go` for the fake `sessionRepo`/`cache` constructors already used, and mirror them. Test:
```go
func TestMintMagicToken_AnonymousReturnsEmpty(t *testing.T) {
	s := newTestAuthService(t) // mirror the helper used in auth_session_test.go
	tok, err := s.MintMagicToken(context.Background(), "")
	if err != nil || tok != "" {
		t.Fatalf("want empty token,nil err; got %q,%v", tok, err)
	}
}

func TestMintMagicToken_ValidSessionMintsToken(t *testing.T) {
	s, rt := newTestAuthServiceWithSession(t) // seed an alive session, return its raw refresh token
	tok, err := s.MintMagicToken(context.Background(), rt)
	if err != nil || !strings.HasPrefix(tok, "ml_") {
		t.Fatalf("want ml_ token; got %q,%v", tok, err)
	}
}
```
> If no reusable fake exists, add minimal in-package fakes for `sessionRepo` (`FindAliveByHash`) and `cache` (`Set`/`Get`/`Delete`) in the test file — do NOT use testify/mock (house style is handwritten fakes).

- [ ] **Step 3: Run — fail, then implement (Step 1 already has impl), then pass**

Run: `cd services/auth && go test ./internal/service/ -run TestMintMagicToken -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git commit services/auth/internal/service/magiclink.go services/auth/internal/service/magiclink_test.go \
  -m "feat(auth): MintMagicToken (refresh-cookie -> one-time xdomain token)"
git show --stat HEAD
```

## Task 3.4: `ConsumeMagicToken` service method (TDD)

Validates + single-use-consumes a token, issues a brand-new session for the user.

**Files:** `services/auth/internal/service/magiclink.go` (+ test)

- [ ] **Step 1: Implement** (append to `magiclink.go`)

```go
import "github.com/ILITA-hub/animeenigma/services/auth/internal/domain"

// ConsumeMagicToken validates a one-time magic token, deletes it (single-use),
// and issues a fresh session for the bound user — exactly like a login. Returns
// an error for unknown/expired/already-used tokens.
func (s *AuthService) ConsumeMagicToken(ctx context.Context, token string, sc SessionContext) (*domain.AuthResponse, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, fmt.Errorf("empty magic token")
	}
	var val xdomainMagicSession
	if err := s.cache.Get(ctx, cache.KeyXDomainMagic(token), &val); err != nil {
		return nil, fmt.Errorf("magic token not found or expired")
	}
	// Single-use: delete immediately so a replay finds nothing.
	_ = s.cache.Delete(ctx, cache.KeyXDomainMagic(token))

	user, err := s.userRepo.GetByID(ctx, val.UserID)
	if err != nil {
		return nil, fmt.Errorf("magic token user: %w", err)
	}
	return s.createSessionAndAuthResponse(ctx, user, sc)
}
```

- [ ] **Step 2: Test** (append to `magiclink_test.go`)

```go
func TestConsumeMagicToken_SingleUse(t *testing.T) {
	s, rt := newTestAuthServiceWithSession(t)
	tok, _ := s.MintMagicToken(context.Background(), rt)
	resp, err := s.ConsumeMagicToken(context.Background(), tok, SessionContext{})
	if err != nil || resp == nil || resp.AccessToken == "" {
		t.Fatalf("first consume should succeed; got %v,%v", resp, err)
	}
	if _, err := s.ConsumeMagicToken(context.Background(), tok, SessionContext{}); err == nil {
		t.Fatalf("second consume must fail (single-use)")
	}
}

func TestConsumeMagicToken_Unknown(t *testing.T) {
	s := newTestAuthService(t)
	if _, err := s.ConsumeMagicToken(context.Background(), "ml_deadbeef", SessionContext{}); err == nil {
		t.Fatalf("unknown token must error")
	}
}
```

- [ ] **Step 3: Run — verify pass**

Run: `cd services/auth && go test ./internal/service/ -run TestConsumeMagicToken -v`
Expected: PASS.

- [ ] **Step 4: Commit**

```bash
git commit services/auth/internal/service/magiclink.go services/auth/internal/service/magiclink_test.go \
  -m "feat(auth): ConsumeMagicToken single-use -> fresh session"
git show --stat HEAD
```

## Task 3.5: Magic-link HTTP handlers (302 + cookies)

**Files:** Create `services/auth/internal/handler/magiclink.go`

- [ ] **Step 1: Implement the two handlers**

```go
// services/auth/internal/handler/magiclink.go
package handler

import (
	"net/http"
	"net/url"

	"github.com/ILITA-hub/animeenigma/libs/logger"
	"github.com/ILITA-hub/animeenigma/libs/metrics"
	"github.com/ILITA-hub/animeenigma/services/auth/internal/service"
)

// MagicLinkHandler serves the cross-domain SSO bridge endpoints. targetBase is
// the canonical .org base (e.g. https://animeenigma.org) that generate redirects to.
type MagicLinkHandler struct {
	authService  *service.AuthService
	cookie       cookieSetter
	targetBase   string
	log          *logger.Logger
}

// cookieSetter is satisfied by *AuthHandler (reuses setRefreshTokenCookie /
// setAccessTokenCookie). Defined to avoid duplicating cookie logic.
type cookieSetter interface {
	setRefreshTokenCookie(w http.ResponseWriter, token string)
	setAccessTokenCookie(w http.ResponseWriter, token string, expiresAt time.Time)
}

func NewMagicLinkHandler(authService *service.AuthService, cookie cookieSetter, targetBase string, log *logger.Logger) *MagicLinkHandler {
	return &MagicLinkHandler{authService: authService, cookie: cookie, targetBase: targetBase, log: log}
}

// Generate (served on .ru): reads the refresh_token cookie, mints a one-time
// token, and 302s to <targetBase>/magic-link-login?oldurl=&token=. Anonymous
// callers are redirected straight to <targetBase><oldurl> (no token).
func (h *MagicLinkHandler) Generate(w http.ResponseWriter, r *http.Request) {
	oldurl := service.SanitizeOldURL(r.URL.Query().Get("oldurl"))
	var token string
	if c, err := r.Cookie(refreshTokenCookieName); err == nil {
		token, _ = h.authService.MintMagicToken(r.Context(), c.Value)
	}
	dest := h.targetBase + oldurl
	if token != "" {
		dest = h.targetBase + "/magic-link-login?oldurl=" + url.QueryEscape(oldurl) + "&token=" + url.QueryEscape(token)
	}
	http.Redirect(w, r, dest, http.StatusFound)
}

// Login (served on .org): consumes the token, sets .org session cookies, 302s to
// oldurl. Any failure lands the user anonymously on oldurl (never an error page).
func (h *MagicLinkHandler) Login(w http.ResponseWriter, r *http.Request) {
	oldurl := service.SanitizeOldURL(r.URL.Query().Get("oldurl"))
	token := r.URL.Query().Get("token")
	if token != "" {
		if resp, err := h.authService.ConsumeMagicToken(r.Context(), token, sessionContextFromReq(r)); err == nil && resp != nil {
			h.cookie.setRefreshTokenCookie(w, resp.RefreshToken)
			h.cookie.setAccessTokenCookie(w, resp.AccessToken, resp.ExpiresAt)
			metrics.AuthEventsTotal.WithLabelValues("magic_link", "success").Inc()
		} else {
			metrics.AuthEventsTotal.WithLabelValues("magic_link", "error").Inc()
		}
	}
	http.Redirect(w, r, oldurl, http.StatusFound)
}
```
> Add `"time"` to imports. The `cookieSetter` interface lets `MagicLinkHandler` reuse `*AuthHandler`'s existing `setRefreshTokenCookie`/`setAccessTokenCookie` (they are methods on `*AuthHandler` in the same package, so the interface is satisfied without exporting). If Go complains the methods are unexported across the interface (same package — it is fine), pass the `*AuthHandler` directly typed.

- [ ] **Step 2: Build**

Run: `cd services/auth && go build ./...`
Expected: clean. Fix any import/interface issues (same-package unexported methods satisfy the interface).

- [ ] **Step 3: Commit**

```bash
git commit services/auth/internal/handler/magiclink.go \
  -m "feat(auth): magic-link Generate/Login handlers (302 + .org cookies)"
git show --stat HEAD
```

## Task 3.6: Auth router + DI wiring

**Files:**
- `services/auth/internal/transport/router.go`
- `services/auth/cmd/auth-api/main.go`
- `services/auth/internal/config/config.go` (add `MagicLinkTargetBase`)

- [ ] **Step 1: Config** — add to the auth config struct + loader:
```go
// in the Config struct:
MagicLinkTargetBase string
// in the loader:
MagicLinkTargetBase: strings.TrimRight(getEnv("MAGIC_LINK_TARGET_BASE", "https://animeenigma.org"), "/"),
```

- [ ] **Step 2: Router** — register the two routes at the ROOT (not under `/api`), mirroring `/health`. Pass a `*MagicLinkHandler` into `NewRouter`:
```go
r.Get("/magic-link-generate", magicLinkHandler.Generate)
r.Get("/magic-link-login", magicLinkHandler.Login)
```
Update `NewRouter`'s signature to accept `magicLinkHandler *handler.MagicLinkHandler`.

- [ ] **Step 3: main.go DI** — after `authHandler := handler.NewAuthHandler(...)`:
```go
magicLinkHandler := handler.NewMagicLinkHandler(authService, authHandler, cfg.MagicLinkTargetBase, log)
```
and pass `magicLinkHandler` into `transport.NewRouter(...)`.

- [ ] **Step 4: Build + existing auth tests**

```bash
cd services/auth && go build ./... && go test ./...
```
Expected: clean build, tests pass.

- [ ] **Step 5: Commit**

```bash
git commit services/auth/internal/transport/router.go services/auth/cmd/auth-api/main.go \
  services/auth/internal/config/config.go \
  -m "feat(auth): wire magic-link routes + MAGIC_LINK_TARGET_BASE"
git show --stat HEAD
```

## Task 3.7: Gateway non-following proxy for the two routes

The shared proxy client follows redirects; magic-link must pass the 302 to the browser. Add a second client + handler.

**Files:**
- `services/gateway/internal/service/proxy.go`
- `services/gateway/internal/handler/proxy.go`
- `services/gateway/internal/transport/router.go`

- [ ] **Step 1: Add a non-following client + Forward variant** in `proxy.go`

In `ProxyService`, add a field `noRedirectClient *http.Client`; in `NewProxyService` initialize it identically to `client` but with:
```go
CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
```
Add a method `ForwardNoRedirect(r *http.Request, service string) (*http.Response, error)` that is a copy of `Forward` but uses `s.noRedirectClient`. (Extract the shared body into a helper `forwardWith(client, r, service)` to avoid duplication — DRY.)

- [ ] **Step 2: Add the gateway handler** in `handler/proxy.go`

```go
// ProxyToAuthNoRedirect forwards to auth WITHOUT following upstream redirects, so
// a 302 (e.g. magic-link bridge) reaches the browser. Cookies + Set-Cookie are
// handled exactly as in proxy().
func (h *ProxyHandler) ProxyToAuthNoRedirect(w http.ResponseWriter, r *http.Request) {
	resp, err := h.proxyService.ForwardNoRedirect(r, "auth")
	if err != nil {
		metrics.ProxyUpstreamErrors.WithLabelValues("forward_error", "auth").Inc()
		httputil.Error(w, err)
		return
	}
	defer resp.Body.Close()
	for key, values := range resp.Header {
		if isCORSHeader(key) {
			continue
		}
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}
```

- [ ] **Step 3: Register the routes** at the gateway root (before `r.Route("/api", ...)`, near `/status/health`):
```go
r.Get("/magic-link-generate", proxyHandler.ProxyToAuthNoRedirect)
r.Get("/magic-link-login", proxyHandler.ProxyToAuthNoRedirect)
```

- [ ] **Step 4: Build + test**

```bash
cd services/gateway && go build ./... && go test ./...
```
Expected: clean.

- [ ] **Step 5: Commit**

```bash
git commit services/gateway/internal/service/proxy.go \
  services/gateway/internal/handler/proxy.go \
  services/gateway/internal/transport/router.go \
  -m "feat(gateway): non-following proxy for magic-link bridge routes"
git show --stat HEAD
git push
```

## Task 3.8: nginx carve-outs on both vhosts

The magic-link paths are top-level (not `/api/`), and `location /` serves the SPA — so they need explicit exact-match locations → `:8000` on BOTH the `.org` and `.ru` app vhosts.

**Files (host):** `/etc/nginx/sites-available/animeenigma.org`, `/etc/nginx/sites-available/animeenigma.ru`

- [ ] **Step 1: Add to both vhosts** (place ABOVE the `location /` block):

```nginx
    location = /magic-link-generate {
        proxy_pass http://127.0.0.1:8000;
        include snippets/proxy-params.conf;
    }
    location = /magic-link-login {
        proxy_pass http://127.0.0.1:8000;
        include snippets/proxy-params.conf;
    }
```

- [ ] **Step 2: Reload**

Run: `nginx -t && systemctl reload nginx`
Expected: ok.

## Task 3.9: Deploy + self-test the full chain

- [ ] **Step 1: Deploy auth + gateway from a clean worktree**

```bash
git fetch origin
WT=$(mktemp -d /tmp/ae-deploy.XXXXXX); git worktree add "$WT" origin/main
cp docker/.env "$WT/docker/.env"
cd "$WT" && make redeploy-auth && make redeploy-gateway
cd - && git worktree remove "$WT" --force
```

- [ ] **Step 2: Walk the chain with a real `.ru` session**

Grab a logged-in `refresh_token` cookie for `.ru` (from your browser). Then:
```bash
# generate on .ru → expect 302 to .org/magic-link-login?...&token=ml_...
curl -sI --cookie "refresh_token=<RT>" \
  "https://animeenigma.ru/magic-link-generate?oldurl=/anime/test" | grep -i "^location"

# login on .org with that token → expect 302 to /anime/test + Set-Cookie
curl -sI "https://animeenigma.org/magic-link-login?oldurl=%2Fanime%2Ftest&token=<ml_token>" \
  | grep -iE "^location|^set-cookie"
```
Expected: generate → `Location: https://animeenigma.org/magic-link-login?...&token=ml_...`; login → `Location: /anime/test` + `Set-Cookie: access_token=...` + `Set-Cookie: refresh_token=...`.

- [ ] **Step 3: Single-use proof** — repeat the login curl with the SAME token → expect `Location: /anime/test` but **no `Set-Cookie`** (token already consumed; anonymous landing).

- [ ] **Step 4: Anonymous proof** — generate without a cookie:
```bash
curl -sI "https://animeenigma.ru/magic-link-generate?oldurl=/anime/test" | grep -i "^location"
```
Expected: `Location: https://animeenigma.org/anime/test` (no token).

- [ ] **Step 5: Open-redirect proof**
```bash
curl -sI "https://animeenigma.org/magic-link-login?oldurl=//evil.com&token=x" | grep -i "^location"
```
Expected: `Location: /` (sanitized).

- [ ] **Step 6: STOP — owner verifies Phase 3** (the redirect chain logs in on `.org`).

---

# PHASE 4 — Flip the full `.ru` 301-bridge

## Task 4.1: External re-registration (owner) — values supplied by me

- [ ] **Step 1: Enumerate OAuth/redirect URIs to change**

```bash
grep -rn "redirect_uri\|RedirectURL\|callback\|webhook\|animeenigma.ru" \
  services/auth/internal/ services/catalog/internal/parser/ docker/.env | grep -i "redirect\|callback\|webhook\|oauth"
```
Produce a list of exact current `.ru` values + their `.org` replacements; hand to owner.

- [ ] **Step 2: Owner performs** (provide exact commands/values):
  - Telegram `setWebhook` → `https://animeenigma.org/api/auth/telegram/webhook` (the bot's `TELEGRAM_WEBHOOK_URL` is already `.org` from Task 2.3 — confirm the live webhook with `getWebhookInfo`).
  - BotFather `/setdomain` → `animeenigma.org` (Telegram Login Widget).
  - Shikimori / MAL OAuth redirect URIs (if any) → `.org`.
  - **Do this BEFORE Step 2 of Task 4.2** (so Telegram POSTs don't hit a 301).

## Task 4.2: Rewrite the `.ru` vhost into the bridge

**Files (host):** `/etc/nginx/sites-available/animeenigma.ru`

- [ ] **Step 1: Back up the current app vhost**

```bash
cp /etc/nginx/sites-available/animeenigma.ru \
   /etc/nginx/sites-available/animeenigma.ru.bak.$(date +%Y%m%d-%H%M%S)
```

- [ ] **Step 2: Replace the `.ru` server block's locations** with the bridge (keep the `listen 443 ssl` + certbot `ssl_certificate*` lines and `server_name animeenigma.ru;`):

```nginx
    # Cross-domain SSO bridge entry — MUST be proxied (not redirected) and
    # excluded from the catch-all so it can read the .ru session cookie.
    location = /magic-link-generate {
        proxy_pass http://127.0.0.1:8000;
        include snippets/proxy-params.conf;
    }

    # API + HLS hit on .ru by stale SPA tabs: send straight to .org (a stale
    # XHR just 401s on .org — harmless).
    location /api/ {
        return 301 https://animeenigma.org$request_uri;
    }

    # Everything else: bounce through the bridge (stays on .ru first so the
    # session cookie is sent), which 302s to .org logged-in.
    location / {
        return 302 https://animeenigma.ru/magic-link-generate?oldurl=$request_uri;
    }
```
Keep the port-80 server block redirecting to https (so even http bookmarks enter the bridge over TLS).

- [ ] **Step 3: Reload + self-test**

```bash
nginx -t && systemctl reload nginx
# anonymous: full chain
curl -sIL "https://animeenigma.ru/anime/test" | grep -iE "^HTTP|^location"
```
Expected chain: `302 →/magic-link-generate?oldurl=/anime/test` → `302 → https://animeenigma.org/anime/test` → `200`.

- [ ] **Step 4: Authenticated browser test** — open a `.ru/anime/<x>` bookmark while logged in on `.ru`; land **logged-in** on `.org/anime/<x>`. Confirm no redirect loop, `stream.*` unaffected, Telegram login still works on `.org`.

- [ ] **Step 5: STOP — owner verifies Phase 4** (migration complete; `.ru` fully bridged).

---

# Post-migration (after all phases verified)

- [ ] Run `/animeenigma-after-update` to update the changelog (Russian Trump-mode) + push. Note the user-facing change: «AnimeEnigma переехал на animeenigma.org — старые ссылки работают и логинят вас автоматически.»
- [ ] Keep the `.ru` cert + bridge alive for the transition window. Dropping `.ru` and `ru.cdn.animeenigma.ru` is a separate, later step (out of this plan).

---

## Self-review notes (author)

- **Spec coverage:** §4 P1→Task 1.1–1.5; P2→2.1–2.6; P3→3.1–3.9; P4→4.1–4.2. §5 cookies→host-only enforced (2.3 Step 1). §6 rollback→each phase's vhost is additive/backed-up. §7 out-of-scope honored (no admin subdomain, BidBerry untouched). §I magic-link→3.2–3.9.
- **Redirect-follow hazard** handled in Task 3.7 (non-following gateway client) — the one non-obvious correctness item.
- **No placeholders**: all new Go/TS functions have full bodies; the only "inspect existing fakes" note (Task 3.3 Step 2) is because the in-package test helper names must match what's already in `auth_session_test.go` — the worker reads that file and mirrors it.
- **Type consistency:** `hlsProxyUrl(query)`, `SanitizeOldURL`, `MintMagicToken`, `ConsumeMagicToken`, `xdomainMagicSession`, `KeyXDomainMagic`, `TTLXDomainMagic`, `MagicLinkTargetBase`, `ProxyToAuthNoRedirect`/`ForwardNoRedirect` used consistently across tasks.
