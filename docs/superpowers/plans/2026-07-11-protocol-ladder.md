# Protocol Ladder Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** FE-driven h3→h2→h1.1 protocol fallback for HLS segment delivery via the streamX origin group, with QoE measurement, an h3 upgrade probe, the silent-stall watchdog progress fix, and hacker-mode telemetry.

**Architecture:** Browsers negotiate HTTP version per-origin, so the ladder is an *origin* ladder: `stream3` (h3+h2) / `stream2` (h2) / `stream1` (h1.1) all serve the same `/api/streaming/hls-proxy`. A framework-free singleton (`protocolLadder.ts`) owns tier state + measurements; `hlsProxyUrl()` consults it, so playlists, segments, subtitles, storyboards and PWA downloads all follow. Tier switches ride the player's existing position-preserving source swap (`resolveStreamForEpisode(ep, true)`).

**Tech Stack:** Vue 3 + TS, hls.js ~1.5.20 (pinned — do NOT touch its internals beyond public config), vitest, host nginx 1.30.3 (nginx.org build), certbot.

**Spec:** `docs/superpowers/specs/2026-07-11-protocol-ladder-design.md`
**Metrics:** UXΔ = +3 (Better) · CDI = 0.05 * 13 · MVQ = Phoenix 90%/85%

## Global Constraints

- Worktree: `/data/ae-protocol-ladder` — NEVER edit `/data/animeenigma` (base tree) except `docker/.env` appends. Absolute paths in Edit/Write MUST point at the worktree (memory: absolute-path edits hitting base tree).
- Frontend tooling: `bun` / `bunx` only (never npm/npx). Unit tests: `cd /data/ae-protocol-ladder/frontend/web && bunx vitest run <file>`.
- Commits: explicit pathspec (never bare `git commit -a` / `add -A`), trailers on every commit:
  `Co-Authored-By: Claude Code <noreply@anthropic.com>` + `Co-Authored-By: 0neymik0 <0neymik0@gmail.com>` + `Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>`
- Vue named type imports from `.vue` files → TS2614; use `import type` (memory trap).
- DS lint hook runs on every FE edit — bind to semantic tokens; hacker-mode rows reuse the existing `text-[var(--success)]` mono pattern (already lint-clean).
- Do NOT read `docker/.env` contents — existence checks by exit code only; value additions by append.
- Host nginx configs are NOT in git — Task 1 edits live files under `/etc/nginx/` with `nginx -t` before every reload; back up each touched file first.
- `docker/.env` in the worktree: `cp /data/animeenigma/docker/.env /data/ae-protocol-ladder/docker/.env` before any deploy from the worktree (401s otherwise).
- hls.js stays pinned `~1.5.20`; config-level additions only (`xhrSetup`).

---

### Task 1: streamX nginx origins (host-side, not in git)

**Files:**
- Create: `/etc/nginx/sites-available/stream1.animeenigma.org`, `stream2.animeenigma.org`, `stream3.animeenigma.org` (+ symlinks in `sites-enabled/`)
- Modify: `/etc/nginx/sites-available/stream.animeenigma.org` (drop Alt-Svc), possibly `animeenigma.ru` (`listen … http2` flag → `http2 on;` directive)
- No repo files.

**Interfaces:**
- Produces: three live HTTPS origins with protocol ceilings h3 / h2 / h1.1, each serving `/api/streaming/hls-proxy` identically to bare `stream.`; verified by curl per protocol. Later tasks hardcode nothing about them except via `VITE_HLS_PROXY_TIERS`.

- [ ] **Step 1: Sanity-check DNS (owner already provisioned)**

```bash
for h in stream1 stream2 stream3; do echo "$h: $(dig +short $h.animeenigma.org | tail -1)"; done
```
Expected: each prints the server's public IP (same as `dig +short stream.animeenigma.org`). If any is empty, STOP and report to owner.

- [ ] **Step 2: Issue the cert (new 3-SAN lineage, nginx authenticator, no config edits)**

```bash
certbot certonly --nginx -d stream1.animeenigma.org -d stream2.animeenigma.org -d stream3.animeenigma.org --non-interactive
ls /etc/letsencrypt/live/stream1.animeenigma.org/
```
Expected: `fullchain.pem privkey.pem …` exist. Lineage name = `stream1.animeenigma.org`.

- [ ] **Step 3: Audit legacy `http2` listen flags (they are socket-wide; per-origin ceilings need the per-server directive)**

```bash
grep -rn "listen.*http2\|http2 on\|http2 off" /etc/nginx/sites-enabled/
```
If any vhost uses the legacy `listen 443 ssl http2` FLAG (e.g. `animeenigma.ru`), back it up and convert: change that line to `listen 443 ssl;` and add `http2 on;` on the next line. Also add explicit `http2 on;` to `stream.animeenigma.org` and `animeenigma.org` server blocks (they currently inherit h2 from the socket-wide flag and would silently lose it after conversion).

```bash
cp /etc/nginx/sites-available/animeenigma.ru /etc/nginx/sites-available/animeenigma.ru.pre-ladder.bak
cp /etc/nginx/sites-available/animeenigma.org /etc/nginx/sites-available/animeenigma.org.pre-ladder.bak
cp /etc/nginx/sites-available/stream.animeenigma.org /etc/nginx/sites-available/stream.animeenigma.org.pre-ladder.bak
```

- [ ] **Step 4: Write the three vhosts** (clone of bare `stream.`'s proxy location; differences marked)

`/etc/nginx/sites-available/stream2.animeenigma.org` (h2 tier — no quic, no Alt-Svc):

```nginx
# streamX protocol ladder — h2 tier (spec 2026-07-11-protocol-ladder-design.md)
server {
    server_name stream2.animeenigma.org;
    client_max_body_size 1M;
    limit_req_status 429;

    location = /api/streaming/hls-proxy {
        limit_req zone=hls burst=120 nodelay;
        if ($request_method = OPTIONS) {
            add_header Access-Control-Allow-Origin "*" always;
            add_header Access-Control-Allow-Methods "GET, OPTIONS" always;
            add_header Access-Control-Allow-Headers "Range" always;
            add_header Access-Control-Max-Age 86400 always;
            add_header Content-Length 0;
            return 204;
        }
        proxy_hide_header Access-Control-Allow-Origin;
        add_header Access-Control-Allow-Origin "*" always;
        add_header Access-Control-Allow-Methods "GET, OPTIONS" always;
        add_header Access-Control-Allow-Headers "Range" always;
        add_header Access-Control-Expose-Headers "Content-Length, Content-Range" always;
        proxy_pass http://127.0.0.1:8000;
        include snippets/proxy-params.conf;
    }
    location / { return 404; }

    listen 443 ssl;
    http2 on;
    ssl_certificate /etc/letsencrypt/live/stream1.animeenigma.org/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/stream1.animeenigma.org/privkey.pem;
    include /etc/letsencrypt/options-ssl-nginx.conf;
    ssl_dhparam /etc/letsencrypt/ssl-dhparams.pem;
}
```

`/etc/nginx/sites-available/stream1.animeenigma.org` — identical except:
- `server_name stream1.animeenigma.org;`
- comment header says "h1.1 tier"
- NO `http2 on;` (add `# http/1.1 only — ladder floor` comment where it would be)

`/etc/nginx/sites-available/stream3.animeenigma.org` — identical to stream2 except:
- `server_name stream3.animeenigma.org;`, header says "h3 tier"
- add `add_header Alt-Svc 'h3=":443"; ma=86400' always;` inside BOTH the OPTIONS branch and the main GET branch (same placement as bare `stream.` has today)
- after `http2 on;` add:
```nginx
    listen 443 quic;   # NO reuseport — bare stream. owns the UDP:443 socket
    http3 on;
```

- [ ] **Step 5: Drop Alt-Svc from bare `stream.` (legacy clients quietly return to h2; keep its quic reuseport listener)**

In `/etc/nginx/sites-available/stream.animeenigma.org`, comment out BOTH `add_header Alt-Svc …` lines with suffix `# DISABLED 2026-07-11 — protocol-ladder spec: h3 is opt-in via stream3 probe`. Do NOT touch its `listen 443 quic reuseport;` / `http3 on;` lines.

- [ ] **Step 6: Enable + reload**

```bash
ln -sf /etc/nginx/sites-available/stream1.animeenigma.org /etc/nginx/sites-enabled/
ln -sf /etc/nginx/sites-available/stream2.animeenigma.org /etc/nginx/sites-enabled/
ln -sf /etc/nginx/sites-available/stream3.animeenigma.org /etc/nginx/sites-enabled/
nginx -t && systemctl reload nginx
```
Expected: `syntax is ok / test is successful`. If `nginx -t` fails, fix before reload — never reload a failing config.

- [ ] **Step 7: Verify protocol ceilings per origin**

```bash
Q='url=x&exp=1&sig=x'   # invalid sig — we only care about protocol + a fast 4xx
curl -so /dev/null -w "stream1: %{http_version}\n" "https://stream1.animeenigma.org/api/streaming/hls-proxy?$Q"
curl -so /dev/null -w "stream2: %{http_version}\n" "https://stream2.animeenigma.org/api/streaming/hls-proxy?$Q"
curl -so /dev/null -w "stream3-tcp: %{http_version}\n" "https://stream3.animeenigma.org/api/streaming/hls-proxy?$Q"
curl -sI "https://stream3.animeenigma.org/api/streaming/hls-proxy?$Q" | grep -i alt-svc
curl -sI "https://stream.animeenigma.org/api/streaming/hls-proxy?$Q" | grep -ci alt-svc || echo "bare stream: no Alt-Svc ✓"
docker run --rm --network host ymuski/curl-http3 curl --http3-only -so /dev/null -w "stream3-h3: %{http_version}\n" "https://stream3.animeenigma.org/api/streaming/hls-proxy?$Q"
```
Expected: `stream1: 1.1`, `stream2: 2`, `stream3-tcp: 2`, stream3 shows `alt-svc: h3=":443"`, bare stream shows none, `stream3-h3: 3`. Also re-verify untouched vhosts still negotiate h2: `curl -so /dev/null -w "%{http_version}\n" https://animeenigma.org/` → `2`.

- [ ] **Step 8: Record the host change in the repo docs** (host configs aren't in git — the doc is)

Append a short "streamX origins (2026-07-11)" subsection to `docs/superpowers/specs/2026-07-11-protocol-ladder-design.md` §2 noting: cert lineage `stream1.animeenigma.org`, `.pre-ladder.bak` backups, and the http2-flag conversion if performed. Commit:

```bash
cd /data/ae-protocol-ladder && git add docs/superpowers/specs/2026-07-11-protocol-ladder-design.md && git commit -m "docs(spec): record streamX host provisioning details" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 2: `protocolLadder.ts` core (pure TS, TDD)

**Files:**
- Create: `frontend/web/src/utils/protocolLadder.ts`
- Test: `frontend/web/src/utils/protocolLadder.spec.ts`

**Interfaces:**
- Produces (consumed by Tasks 3–6):

```ts
export type TierId = 'h3' | 'h2' | 'h1'
export interface Tier { id: TierId; base: string }
export interface FragReport { bytes: number; ms: number; mediaDurationS: number; protocol?: string }
export interface InflightState { url: string; receivedBytes: number; totalBytes: number; elapsedMs: number }
export interface LadderDebug {
  tierId: TierId; tierIndex: number; tierCount: number
  protocol: string            // nextHopProtocol of last reported fragment, '?' unknown
  measuredMbps: number; neededMbps: number
  trail: string               // "h3→h2 (first-frag projected 17s)" style, '' if none
  probe: string               // "h3 2.1 Mbps @03:24 — rejected (<1.1× h2)" | 'pending' | ''
}
export function parseTiers(tiersRaw: string | undefined, fallbackBase: string | undefined): Tier[]
export class ProtocolLadder {
  constructor(tiers: Tier[], deps?: { now?: () => number; storage?: Pick<Storage,'getItem'|'setItem'|'removeItem'> })
  currentBase(): string
  isMultiTier(): boolean
  reportFragment(r: FragReport): void
  reportTimeout(): void
  onXhrOpen(url: string): void
  onXhrProgress(url: string, loaded: number, total: number): void
  inflight(): InflightState | null
  onChange(cb: (tier: Tier, reason: string) => void): () => void
  recordProbe(mbps: number, accepted: boolean, note: string): void   // used by Task 5's probe
  switchTo(id: TierId, reason: string): void                          // probe upshift entry
  debugSnapshot(): LadderDebug | null   // null when single-tier
}
/** Watchdog guard (the tNeymik "stale"-loop regression, spec §4): a first
 *  fragment with bytes flowing is SLOW, not dead — the watchdog must defer to
 *  the ladder instead of aborting/re-resolving. */
export function shouldDeferStallToLadder(inflight: InflightState | null): boolean
export const ladder: ProtocolLadder   // singleton from import.meta.env
```

Policy constants (export for tests): `SAFETY_FACTOR = 1.2`, `CONSEC_SLOW_FRAGS = 3`, `TIMEOUTS_TO_DOWNSHIFT = 2`, `FIRSTFRAG_MIN_ELAPSED_MS = 3000`, `FIRSTFRAG_PROJECTED_MS = 8000`, `SWITCH_COOLDOWN_MS = 30_000`, `PROBE_ACCEPT_FACTOR = 1.1`, `PERSIST_TTL_MS = 86_400_000`, `EWMA_ALPHA = 0.3`, `LS_KEY = 'ae:protocolLadder:v1'`.

Behavior notes for the implementer:
- Entry tier: `h2` when present, else first tier. If persisted `{tier, ts}` is `< PERSIST_TTL_MS` old and that tier exists, start there instead.
- `reportFragment`: update `measuredEwmaBps` (bytes*8 / (ms/1000)) and `neededEwmaBps` (bytes*8 / mediaDurationS); clear the inflight slot; increment `consecSlow` when `measuredEwmaBps < neededEwmaBps * SAFETY_FACTOR` (require ≥2 samples first), else reset to 0. At `CONSEC_SLOW_FRAGS` → downshift (`reason: 'ewma <need×1.2 ×3'`).
- `reportTimeout`: counter per current tier; at `TIMEOUTS_TO_DOWNSHIFT` → downshift (`reason: 'frag timeouts ×2'`).
- `onXhrProgress`: track inflight; when NO fragment has completed on this tier yet and `elapsedMs > FIRSTFRAG_MIN_ELAPSED_MS` and `total > 0` and projected `elapsedMs * total/loaded > FIRSTFRAG_PROJECTED_MS` → downshift once (`reason: 'first-frag projected Ns'`).
- Downshift: move one tier down (h1 = floor, no-op below), honor `SWITCH_COOLDOWN_MS` between switches, reset per-tier counters, persist `{tier, ewma, probedH3, ts}` to storage, push trail entry, fire `onChange` callbacks.
- `switchTo('h3', …)` (probe accept): same bookkeeping upward.
- `navigator.connection?.addEventListener('change', …)` → clear persisted state + reset to entry tier (guard `typeof navigator`, try/catch — jsdom).
- Single-tier ladder (`parseTiers` fallback): every report is a no-op; `debugSnapshot()` returns null.
- `parseTiers('h3=https://a,h2=https://b,h1=https://c', …)` → ordered `[h3,h2,h1]` DESCENDING preference but the *entry* rule (h2 first) is ProtocolLadder's job, not parseTiers'. Unknown keys / malformed pairs are skipped; trailing slashes stripped; empty result + fallbackBase → `[{id:'h2', base: fallbackBase}]`; both empty → `[{id:'h2', base: ''}]` (relative same-origin, current behavior).

- [ ] **Step 1: Write failing tests** — `frontend/web/src/utils/protocolLadder.spec.ts` covering, at minimum (use injected `now` + in-memory storage stub; `new ProtocolLadder(parseTiers(RAW, undefined), {now, storage})` per test):
  - parseTiers: full string → 3 tiers; unset + base → single h2 tier with base; both unset → `[{id:'h2',base:''}]`; malformed entries skipped.
  - entry tier is h2; persisted fresh tier wins; persisted stale (>24h) ignored.
  - downshift after 3 consecutive slow fragments (feed `reportFragment({bytes: 4_000_000, ms: 16_000, mediaDurationS: 6})` — measured ≈2 Mbps < needed ≈5.3×1.2) → `currentBase()` becomes h1... **careful:** entry h2 → downshift lands h1. Assert `onChange` fired with reason containing `ewma`.
  - fast fragments (`ms: 2_000`) never downshift; consec counter resets after one fast frag between slow ones.
  - two `reportTimeout()` → downshift; one does not.
  - first-frag projection: `onXhrOpen(u)`, advance `now` by 4000, `onXhrProgress(u, 1_000_000, 4_700_000)` → projected ≈18.8s > 8s → downshift with reason containing `first-frag`; same progress at 2500ms elapsed → no downshift (min-elapsed gate).
  - cooldown: two downshift triggers within 30s → only one switch.
  - persistence roundtrip: after downshift, a NEW ladder with same storage + fresh `now` starts on the persisted tier.
  - `switchTo('h3','probe')` from h2 works and persists; `recordProbe` + `debugSnapshot().probe` renders the measurement.
  - single-tier: reports are no-ops, `debugSnapshot()` null, `isMultiTier()` false.
  - `shouldDeferStallToLadder` (the tNeymik stale-loop regression): `{url:'u', receivedBytes: 800_000, totalBytes: 4_700_000, elapsedMs: 12_000}` → `true` (bytes flowing = slow, not dead); `receivedBytes: 0` → `false`; `null` → `false`.

- [ ] **Step 2: Run to verify failure**
Run: `cd /data/ae-protocol-ladder/frontend/web && bunx vitest run src/utils/protocolLadder.spec.ts`
Expected: FAIL — module not found.

- [ ] **Step 3: Implement `protocolLadder.ts`** per the interface + behavior notes above. Singleton at the bottom:

```ts
export const ladder = new ProtocolLadder(
  parseTiers(import.meta.env.VITE_HLS_PROXY_TIERS, import.meta.env.VITE_HLS_PROXY_BASE),
)
```

- [ ] **Step 4: Run tests to green**
Run: `bunx vitest run src/utils/protocolLadder.spec.ts` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /data/ae-protocol-ladder && git add frontend/web/src/utils/protocolLadder.ts frontend/web/src/utils/protocolLadder.spec.ts && git commit -m "feat(web): protocol ladder core — QoE tier state machine (h3/h2/h1)" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 3: `hlsProxyUrl` → ladder base + env plumbing

**Files:**
- Modify: `frontend/web/src/utils/streaming.ts`
- Modify: `frontend/web/src/vite-env.d.ts` (add `readonly VITE_HLS_PROXY_TIERS?: string` next to `VITE_HLS_PROXY_BASE`)
- Modify: `frontend/web/Dockerfile` (mirror the existing `VITE_HLS_PROXY_BASE` ARG/ENV pair with `VITE_HLS_PROXY_TIERS`, lines ~12/21)
- Modify: `docker/docker-compose.yml` (web build args, ~line 1514: add `VITE_HLS_PROXY_TIERS: "${VITE_HLS_PROXY_TIERS:-}"`)
- Test: `frontend/web/src/utils/streaming.spec.ts` (extend)

**Interfaces:**
- Consumes: `ladder.currentBase()` (Task 2).
- Produces: `hlsProxyUrl(query)` now roots at the ladder's active tier. All existing call sites (subtitleProxy, storyboardVtt, offline downloadEngine/playlistRewrite, player adapters) inherit with zero changes — verify by grep that they all go through `hlsProxyUrl`.

- [ ] **Step 1: Extend `streaming.spec.ts`** — mock the ladder module:

```ts
vi.mock('@/utils/protocolLadder', () => ({ ladder: { currentBase: vi.fn(() => 'https://stream2.test') } }))
```
New cases: base comes from `ladder.currentBase()`; empty base → relative URL (existing cases keep passing — update their expectations to route through the mock, returning `''` where they previously relied on unset env).

- [ ] **Step 2: Run to verify failure** — `bunx vitest run src/utils/streaming.spec.ts` → FAIL.

- [ ] **Step 3: Implement** — `streaming.ts`:

```ts
import { ladder } from '@/utils/protocolLadder'

export function hlsProxyUrl(query: string): string {
  const base = ladder.currentBase().replace(/\/+$/, '')
  return `${base}/api/streaming/hls-proxy?${query}`
}
```
(The env-fallback logic now lives in `parseTiers` — single-tier from `VITE_HLS_PROXY_BASE` preserves current behavior byte-for-byte.)

- [ ] **Step 4: Verify call-site coverage**
Run: `grep -rn "hls-proxy" frontend/web/src --include='*.ts' --include='*.vue' | grep -v "hlsProxyUrl\|spec\|\* " | grep -v "utils/streaming.ts"`
Expected: no raw `/api/streaming/hls-proxy` string-building outside `streaming.ts` (report any stragglers in the task summary; fix them to call `hlsProxyUrl`).

- [ ] **Step 5: Env plumbing** — vite-env.d.ts type, Dockerfile ARG+ENV pair, compose build arg (exact edits per Files list above).

- [ ] **Step 6: Full util tests green** — `bunx vitest run src/utils/` → PASS.

- [ ] **Step 7: Commit**
```bash
cd /data/ae-protocol-ladder && git add frontend/web/src/utils/streaming.ts frontend/web/src/utils/streaming.spec.ts frontend/web/src/vite-env.d.ts frontend/web/Dockerfile docker/docker-compose.yml && git commit -m "feat(web): hlsProxyUrl follows the protocol ladder tier + VITE_HLS_PROXY_TIERS plumbing" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 4: engine wiring — xhr progress, fragment reports, timeouts

**Files:**
- Modify: `frontend/web/src/composables/aePlayer/useVideoEngine.ts`
- Test: `frontend/web/src/composables/aePlayer/useVideoEngine.spec.ts` (extend — it already fakes the Hls class; follow its existing mock pattern)

**Interfaces:**
- Consumes: `ladder.onXhrOpen/onXhrProgress/reportFragment/reportTimeout` (Task 2).
- Produces: `lastFragUrl: Ref<string>` added to the composable's return object (Task 5's probe needs a sample segment URL).

- [ ] **Step 1: Extend the spec** (mock `@/utils/protocolLadder` with vi.fn stubs) — failing cases:
  - `xhrSetup` passed in the Hls config calls `ladder.onXhrOpen(url)` and wires a `progress` listener that forwards `(url, e.loaded, e.total)` to `ladder.onXhrProgress`.
  - FRAG_LOADED always calls `ladder.reportFragment({bytes, ms, mediaDurationS, protocol})` even with `collectStats=false` (the always-on section), with `protocol` read from `performance.getEntriesByName(xhr.responseURL)` (stub `performance.getEntriesByName` to return `[{nextHopProtocol:'h2'}]`).
  - FRAG_LOADED sets `lastFragUrl.value = frag.url`.
  - ERROR with `details === 'fragLoadTimeOut'` calls `ladder.reportTimeout()` (non-fatal too — hook at handler top, before the `data.fatal` gate).

- [ ] **Step 2: Run to verify failure** — `bunx vitest run src/composables/aePlayer/useVideoEngine.spec.ts` → FAIL.

- [ ] **Step 3: Implement in `useVideoEngine.ts`:**
  - `import { ladder } from '@/utils/protocolLadder'`; add `const lastFragUrl = ref('')` (reset in `load()`, add to return).
  - Hls config gains:
```ts
      xhrSetup: (xhr: XMLHttpRequest, url: string) => {
        ladder.onXhrOpen(url)
        xhr.addEventListener('progress', (e: ProgressEvent) => ladder.onXhrProgress(url, e.loaded, e.total))
      },
```
  - In FRAG_LOADED, immediately after `fragLoadedCount.value++` (hoist the existing `loadMs` computation above the collectStats gate so it's computed once):
```ts
      lastFragUrl.value = f.url ?? ''
      const xhrNd = data?.networkDetails
      const rt = xhrNd?.responseURL
        ? (performance.getEntriesByName(xhrNd.responseURL).pop() as PerformanceResourceTiming | undefined)
        : undefined
      ladder.reportFragment({
        bytes: st.total ?? 0,
        ms: loadMs,
        mediaDurationS: f.duration ?? 0,
        protocol: rt?.nextHopProtocol,
      })
```
  - At the very top of the `Hls.Events.ERROR` handler:
```ts
      if (data?.details === Hls.ErrorDetails.FRAG_LOAD_TIMEOUT) ladder.reportTimeout()
```

- [ ] **Step 4: Green** — `bunx vitest run src/composables/aePlayer/useVideoEngine.spec.ts` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /data/ae-protocol-ladder && git add frontend/web/src/composables/aePlayer/useVideoEngine.ts frontend/web/src/composables/aePlayer/useVideoEngine.spec.ts && git commit -m "feat(web): video engine feeds the protocol ladder (xhr progress, frag stats, timeouts)" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 5: AePlayer — watchdog progress guard, tier-switch reload, h3 probe

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (watchdog ~line 908–945; add ladder subscription + probe near the other lifecycle wiring)
- Create: `frontend/web/src/utils/probeH3.ts` + `frontend/web/src/utils/probeH3.spec.ts` (pure, testable probe helper — keeps AePlayer.vue thin)

**Interfaces:**
- Consumes: `ladder` (Task 2), `engine.lastFragUrl` (Task 4), existing `resolveStreamForEpisode(ep, keepPosition)` (AePlayer.vue:2498), `recordDecision(msg)`, `armPlaybackWatchdog`.
- Produces: `probeH3(ladder, sampleUrl, playlistUrl, f?: typeof fetch): Promise<void>` — primes Alt-Svc then measures; calls `ladder.recordProbe(...)` and `ladder.switchTo('h3', 'probe ≥1.1× h2')` on accept.

- [ ] **Step 1: `probeH3.spec.ts` failing tests** (inject a fake `fetch` + fake ladder):
  - Two-fetch sequence: first (prime) hits the h3-tier PLAYLIST URL, then after the prime resolves, the measure fetch hits the h3-tier SEGMENT URL with `{cache:'no-store'}`. (Browsers only speak h3 to an origin after learning Alt-Svc from a prior response — a single fetch would measure h2-over-stream3.)
  - Measure duration + bytes → Mbps; `performance.getEntriesByName` stubbed: `nextHopProtocol==='h3'` and mbps ≥ 1.1× ladder's h2 EWMA → `switchTo` called; slower → `recordProbe(..., false, ...)`, no switch.
  - `nextHopProtocol !== 'h3'` on the measure fetch → recorded as rejected with note `h3-unavailable`, no switch.
  - Any fetch rejection/timeout (20s AbortController) → recorded rejected, never throws.
  - No h3 tier configured / already probed this session → resolves without fetching (fetch never called).

- [ ] **Step 2: Run to verify failure** — `bunx vitest run src/utils/probeH3.spec.ts` → FAIL.

- [ ] **Step 3: Implement `probeH3.ts`.** URL construction: take the current-tier absolute/relative URL and swap its origin prefix for the h3 tier's base (`new URL(u, location.origin)` → replace origin with h3 base, keep path+query — signatures stay valid because the path+query are untouched). Guard everything in try/catch; a probe must never affect playback on failure.

- [ ] **Step 4: Green** — `bunx vitest run src/utils/probeH3.spec.ts` → PASS.

- [ ] **Step 5: Wire AePlayer.vue** (no new spec — logic lives in tested modules; this is subscription glue):
  - Watchdog guard, inside the `playbackWatchdog` timeout callback, directly ABOVE the `if (engine.fragLoadedCount.value > 0) return` line:
```ts
    // A first fragment that is downloading (bytes flowing) is SLOW, not dead —
    // aborting it re-resolves the same source forever (the 2026-07-11 tNeymik
    // "stale" loop: seg0 restarted 3×, video never possible). Let the ladder's
    // projected-too-slow rule downshift the tier instead; just re-arm.
    if (shouldDeferStallToLadder(ladder.inflight())) {
      armPlaybackWatchdog()
      return
    }
```
  - Tier-change subscription (place near the other `onUnmounted` wiring):
```ts
const unsubLadder = ladder.onChange((tier, reason) => {
  recordDecision(`protocol ladder → ${tier.id} (${reason})`)
  const ep = selectedEpisode.value
  if (ep) void resolveStreamForEpisode(ep, true) // position-preserving swap; new base flows via hlsProxyUrl
})
onUnmounted(unsubLadder)
```
  - Probe trigger — 30s after playback starts, once per mount:
```ts
let h3ProbeTimer: ReturnType<typeof setTimeout> | null = null
watch(hasStarted, (started) => {
  if (!started || h3ProbeTimer) return
  h3ProbeTimer = setTimeout(() => {
    if (!state.playing.value) return
    void probeH3(ladder, engine.lastFragUrl.value, currentStream.value?.url ?? '')
  }, 30_000)
})
onUnmounted(() => { if (h3ProbeTimer) clearTimeout(h3ProbeTimer) })
```
  (`import { ladder, shouldDeferStallToLadder } from '@/utils/protocolLadder'`, `import { probeH3 } from '@/utils/probeH3'` — plain imports, they're not types.)

- [ ] **Step 6: Type-check + full player suite**
Run: `cd /data/ae-protocol-ladder/frontend/web && bunx vue-tsc --noEmit 2>&1 | tail -5 && bunx vitest run src/components/player src/composables/aePlayer src/utils`
Expected: no new type errors; all green. (Memory: `vue-tsc --noEmit` alone can false-pass — the vitest run is the real gate.)

- [ ] **Step 7: Commit**
```bash
cd /data/ae-protocol-ladder && git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/utils/probeH3.ts frontend/web/src/utils/probeH3.spec.ts && git commit -m "feat(web): AePlayer rides the ladder — progress-aware watchdog, tier-switch reload, h3 probe" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 6: hacker-mode rows (PROTO / NET / LADDR / PROBE)

**Files:**
- Modify: `frontend/web/src/components/player/aePlayer/AePlayer.vue` (`debugStats` computed, ~line 2375)
- Modify: `frontend/web/src/components/player/aePlayer/PlaybackSettingsMenu.vue` (template rows ~line 154 + props type ~line 182)
- Test: `frontend/web/src/components/player/aePlayer/PlaybackSettingsMenu.spec.ts` (extend, follow its existing mount pattern — beware the vue-i18n mock/barrel trap memory: keep using whatever mock style the file already has)

**Interfaces:**
- Consumes: `ladder.debugSnapshot()` (Task 2).
- Produces: `debugStats` gains optional fields `proto?: string; net?: string; laddr?: string; probe?: string` — rendered only when present (single-tier dev builds get none).

- [ ] **Step 1: Failing spec cases** — with `debugStats` containing `proto:'h2 · tier 2/3'`, `net:'4.1 Mbps ewma / need 5.4 ×1.2'`, `laddr:'h3→h2 (first-frag projected 17s)'`, `probe:'h3 2.1 Mbps @03:24 — rejected (<1.1× h2)'` → four rows render (`data-test="debug-proto"` etc.); with those fields absent → rows absent.

- [ ] **Step 2: Run to verify failure** — `bunx vitest run src/components/player/aePlayer/PlaybackSettingsMenu.spec.ts` → FAIL.

- [ ] **Step 3: Implement**
  - PlaybackSettingsMenu.vue — extend the `debugStats` prop type with the four optional strings; template, after the EDGE block, same mono styling:
```html
          <template v-if="debugStats.proto">
            <div data-test="debug-proto">PROTO {{ debugStats.proto }}</div>
            <div v-if="debugStats.net">NET&nbsp;&nbsp; {{ debugStats.net }}</div>
            <div v-if="debugStats.laddr">LADDR {{ debugStats.laddr }}</div>
            <div v-if="debugStats.probe">PROBE {{ debugStats.probe }}</div>
          </template>
```
  - AePlayer.vue `debugStats` computed — after the `edgeRot` field:
```ts
    // Protocol-ladder telemetry (multi-tier prod only; null snapshot in dev).
    ...(ladderSnap ? {
      proto: `${ladderSnap.protocol} · tier ${ladderSnap.tierIndex}/${ladderSnap.tierCount}`,
      net: `${ladderSnap.measuredMbps.toFixed(1)} Mbps ewma / need ${ladderSnap.neededMbps.toFixed(1)} ×1.2`,
      laddr: ladderSnap.trail,
      probe: ladderSnap.probe,
    } : {}),
```
    with `const ladderSnap = ladder.debugSnapshot()` at the top of the computed.

- [ ] **Step 4: Green** — `bunx vitest run src/components/player/aePlayer/PlaybackSettingsMenu.spec.ts` → PASS.

- [ ] **Step 5: Commit**
```bash
cd /data/ae-protocol-ladder && git add frontend/web/src/components/player/aePlayer/AePlayer.vue frontend/web/src/components/player/aePlayer/PlaybackSettingsMenu.vue frontend/web/src/components/player/aePlayer/PlaybackSettingsMenu.spec.ts && git commit -m "feat(web): hacker-mode PROTO/NET/LADDR/PROBE rows — ladder metrics + logic" -m "Co-Authored-By: Claude Code <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

### Task 7: verify, deploy, live-check, changelog (after-update)

**Files:**
- Modify: `docker/.env` (base tree — allowed exception; append `VITE_HLS_PROXY_TIERS=…`)
- Modify: `frontend/web/changelog.full.json` + regenerate `frontend/web/public/changelog.json`

**Interfaces:** none — ship gate.

- [ ] **Step 1: `/frontend-verify`** — run the skill (DS-lint, i18n parity, real `bun run build`, trap checks) from the worktree. All gates must pass.

- [ ] **Step 2: Set the tiers env (exit-code-only existence check — never read `.env` contents)**
```bash
grep -q '^VITE_HLS_PROXY_TIERS=' /data/animeenigma/docker/.env; echo "exists=$?"
# only if exists=1:
printf 'VITE_HLS_PROXY_TIERS=h3=https://stream3.animeenigma.org,h2=https://stream2.animeenigma.org,h1=https://stream1.animeenigma.org\n' >> /data/animeenigma/docker/.env
```

- [ ] **Step 3: Land + deploy** — per `bin/ae-land.sh` / `bin/ae-deploy.sh` helpers (verify→changelog→land→deploy): pull-rebase-push worktree commits to `main`, `cp /data/animeenigma/docker/.env /data/ae-protocol-ladder/docker/.env`, then `make redeploy-web` (from the base tree AFTER the push lands + autosync, or from the worktree with .env copied — follow the helper).

- [ ] **Step 4: Live verification**
```bash
# built bundle actually carries the tiers:
docker exec animeenigma-web sh -c "grep -rlo 'stream3.animeenigma.org' /usr/share/nginx/html/assets/ | head -1"
# entry tier (h2) serves segments — replay the tNeymik case through stream2:
Q=$(curl -s "http://localhost:8000/api/anime/dbc95dd5-8470-4f83-9632-622431073182/ae/stream?episode=14" \
  | python3 -c "import json,sys,urllib.parse; d=json.load(sys.stdin)['data']; print(f\"url={urllib.parse.quote(d['url'],safe='')}&exp={d['exp']}&sig={d['sig']}\")")
curl -so /dev/null -w "stream2 playlist: %{http_code} over http/%{http_version}\n" "https://stream2.animeenigma.org/api/streaming/hls-proxy?$Q"
# expected: 200 over http/2
```
Then ask the owner to reload the episode-14 page and confirm playback + hacker-mode rows (PROTO shows `h2 · tier 2/3`).

- [ ] **Step 5: Changelog (Trump-mode, Russian)** — one feature entry: the player now auto-picks the fastest protocol per network (умный протокольный лифт h3/h2/h1), fixing "video loads but player calls it stale". Prepend to `changelog.full.json`, regenerate served copy via `node frontend/web/scripts/changelog-trim.mjs`. Commit + push.

- [ ] **Step 6: Memory + report** — save/refresh a memory file on the ladder (tiers, entry=h2, probe semantics, nginx streamX layout, Alt-Svc removal from bare stream.), update MEMORY.md index, and deliver the final summary with metrics (UXΔ = +3 (Better) · CDI = 0.05 * 13 · MVQ = Phoenix 90%/85%).
