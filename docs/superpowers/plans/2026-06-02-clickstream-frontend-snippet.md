# Clickstream Frontend Snippet + Dashboards — Implementation Plan (Plan 2 of 3)

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add a framework-agnostic TypeScript analytics snippet to the Vue 3 frontend that captures pageviews, autocaptured clicks, and heartbeats, batches them, and ships them to Plan 1's `POST /api/analytics/collect` via `navigator.sendBeacon`. Wire anonymous→identified stitching to the auth store. Add a Grafana PostgreSQL datasource + a product-analytics dashboard over `analytics_events_resolved`.

**Architecture:** A small `src/analytics/` module split by responsibility (session, identity, autocapture, transport, public API). The public `analytics` singleton is initialized once at app bootstrap (gated by `VITE_ANALYTICS_ENABLED`, default on). Pageviews fire from `router.afterEach`; `identify`/`reset` are called from the auth store's `login`/`logout`. Delivery mirrors the existing `useWatchSession.ts` beacon pattern. Dashboards are provisioned as code and read the Postgres clickstream directly. Backend is Plan 1 (already shipped); this plan is frontend + Grafana only.

**Tech Stack:** Vue 3, TypeScript, Vite (`import.meta.env.VITE_*`), Vitest + jsdom, Pinia (`stores/auth.ts`), vue-router, Grafana 10.3.3 (PostgreSQL datasource). Reuses `src/utils/anonId.ts` (`getOrCreateAnonId`) and the `src/api/client.ts` base URL. Spec: `docs/superpowers/specs/2026-06-02-analytics-tracing-design.md`. Depends on Plan 1 (`POST /api/analytics/collect` live).

**Scope notes / deferrals (honest about Plan 1 reality):**
- **`trace_id` deferred to Plan 3** — the snippet leaves it absent; it needs the backend OTel collector. The event type allows an optional field so Plan 3 only adds population.
- **`device_type` left empty** — Plan 1's collect handler does not map `device_type` from the envelope (it has no such field), so sending it would be dropped. It is SQL-derivable from `user_agent`; the dashboard does not depend on it.
- Beacon body uses `Content-Type: text/plain` (a CORS "simple" request — no preflight; Plan 1's handler reads the raw body regardless of content-type).

---

## File Structure

**New module `frontend/web/src/analytics/`:**
- `types.ts` — `AnalyticsEvent`, `AnalyticsEnvelope`, `AnalyticsContext`, `EventType`
- `session.ts` — `getSessionId()` (30-min idle / new-day rotation)
- `identity.ts` — anon id (reuses `utils/anonId`) + `user_id` persistence + `clearUserId`
- `autocapture.ts` — `buildSelector(el)`, `stripPII(text)`, `extractClick(el)`
- `transport.ts` — buffer + `enqueue`, `flush`, `startAutoFlush`, `stopAutoFlush` (sendBeacon + fetch-keepalive fallback, 64 KB split)
- `index.ts` — the `analytics` singleton: `init / page / track / identify / reset`, heartbeat, autocapture wiring, lifecycle flush triggers
- `__tests__/{session,identity,autocapture,transport,index}.spec.ts`

**Modified:**
- `frontend/web/src/utils/anonId.ts` — add `resetAnonId()` (rotate on logout)
- `frontend/web/src/main.ts` — `analytics.init(...)` in the deferred-init block, gated by flag
- `frontend/web/src/router/index.ts` — `router.afterEach` → `analytics.page()`
- `frontend/web/src/stores/auth.ts` — `analytics.identify(...)` in `login` / `setUser`; `analytics.reset()` in `logout`
- `frontend/web/.env.example` — `VITE_ANALYTICS_ENABLED=true`

**New Grafana files:**
- `docker/grafana/provisioning/datasources/datasources.yml` — add PostgreSQL datasource (uid `aenigma-postgres`)
- `infra/grafana/dashboards/product-analytics.json` — new dashboard

---

## Task 1: Module scaffold + types + env flag

**Files:**
- Create: `frontend/web/src/analytics/types.ts`
- Modify: `frontend/web/.env.example`

- [ ] **Step 1: Create the types**

Create `frontend/web/src/analytics/types.ts`:

```ts
// Analytics clickstream types. The envelope/event shapes mirror Plan 1's
// backend wire contract (services/analytics handler/collect.go wireEnvelope).
export type EventType = 'pageview' | 'click' | 'heartbeat' | 'identify' | 'custom'

export interface AnalyticsEvent {
  event_type: EventType
  event_name?: string
  timestamp: string // ISO 8601
  url?: string
  path?: string
  referrer?: string
  title?: string
  el_selector?: string
  el_text?: string
  el_tag?: string
  el_attrs?: Record<string, string>
  active_ms?: number
  // trace_id is intentionally omitted in Plan 2 (added by Plan 3 tracing).
  properties?: Record<string, unknown>
}

export interface AnalyticsContext {
  user_agent: string
  screen_w: number
  screen_h: number
}

export interface AnalyticsEnvelope {
  anonymous_id: string
  user_id: string | null
  session_id: string
  events: AnalyticsEvent[]
  ctx: AnalyticsContext
}

export interface AnalyticsConfig {
  endpoint: string // full URL of POST /api/analytics/collect
  heartbeatMs?: number // default 15000
  flushMs?: number // default 5000
  maxBatch?: number // default 20
}
```

- [ ] **Step 2: Add the feature flag to .env.example**

Append to `frontend/web/.env.example` (under the feature-flags section):

```
VITE_ANALYTICS_ENABLED=true              # Clickstream analytics snippet (Plan 2)
```

- [ ] **Step 3: Verify it type-checks**

Run: `cd /data/animeenigma/frontend/web && bunx tsc --noEmit 2>&1 | tail -5`
Expected: no new errors referencing `src/analytics/types.ts`.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/analytics/types.ts frontend/web/.env.example
git commit -m "feat(analytics-fe): clickstream snippet types + VITE_ANALYTICS_ENABLED flag

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 2: Session management

**Files:**
- Create: `frontend/web/src/analytics/session.ts`
- Test: `frontend/web/src/analytics/__tests__/session.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/analytics/__tests__/session.spec.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('getSessionId', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('creates and persists a session id', async () => {
    const { getSessionId } = await import('../session')
    const now = Date.parse('2026-06-02T10:00:00Z')
    const id = getSessionId(now)
    expect(id).toBeTruthy()
    expect(getSessionId(now)).toBe(id) // stable within the window
  })

  it('rotates after 30 minutes of inactivity', async () => {
    const { getSessionId } = await import('../session')
    const t0 = Date.parse('2026-06-02T10:00:00Z')
    const id1 = getSessionId(t0)
    const id2 = getSessionId(t0 + 31 * 60 * 1000)
    expect(id2).not.toBe(id1)
  })

  it('keeps the session within the idle window', async () => {
    const { getSessionId } = await import('../session')
    const t0 = Date.parse('2026-06-02T10:00:00Z')
    const id1 = getSessionId(t0)
    const id2 = getSessionId(t0 + 20 * 60 * 1000)
    expect(id2).toBe(id1)
  })

  it('rotates on a new UTC day', async () => {
    const { getSessionId } = await import('../session')
    const id1 = getSessionId(Date.parse('2026-06-02T23:59:00Z'))
    const id2 = getSessionId(Date.parse('2026-06-03T00:05:00Z'))
    expect(id2).not.toBe(id1)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/session.spec.ts`
Expected: FAIL (cannot resolve `../session`).

- [ ] **Step 3: Implement**

Create `frontend/web/src/analytics/session.ts`:

```ts
// Session id management. A session rotates after 30 min of inactivity or on
// a new UTC day. Stored in localStorage alongside its last-activity stamp.
const SID_KEY = 'aenig_analytics_sid'
const SID_TS_KEY = 'aenig_analytics_sid_ts'
const SID_DAY_KEY = 'aenig_analytics_sid_day'
const IDLE_MS = 30 * 60 * 1000

function dayOf(now: number): string {
  return new Date(now).toISOString().slice(0, 10)
}

// getSessionId returns the current session id, rotating it when the idle
// window has elapsed or the UTC day changed. Each call refreshes the
// last-activity stamp. `now` is injectable for testing.
export function getSessionId(now: number = Date.now()): string {
  try {
    const sid = localStorage.getItem(SID_KEY)
    const ts = Number(localStorage.getItem(SID_TS_KEY) || '0')
    const day = localStorage.getItem(SID_DAY_KEY)
    const expired = !sid || now - ts > IDLE_MS || day !== dayOf(now)
    if (expired) {
      const fresh = crypto.randomUUID()
      localStorage.setItem(SID_KEY, fresh)
      localStorage.setItem(SID_DAY_KEY, dayOf(now))
      localStorage.setItem(SID_TS_KEY, String(now))
      return fresh
    }
    localStorage.setItem(SID_TS_KEY, String(now))
    return sid as string
  } catch {
    // localStorage unavailable — ephemeral per-call id.
    return crypto.randomUUID()
  }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/session.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/analytics/session.ts frontend/web/src/analytics/__tests__/session.spec.ts
git commit -m "feat(analytics-fe): session id with 30min-idle / new-day rotation

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 3: Identity (reuse anon id + user_id) and anon-id reset

**Files:**
- Modify: `frontend/web/src/utils/anonId.ts` (add `resetAnonId`)
- Create: `frontend/web/src/analytics/identity.ts`
- Test: `frontend/web/src/analytics/__tests__/identity.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/analytics/__tests__/identity.spec.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'

describe('analytics identity', () => {
  beforeEach(() => {
    localStorage.clear()
    vi.resetModules()
  })

  it('anon id is stable then rotates on reset', async () => {
    const { getAnonId, resetAnon } = await import('../identity')
    const a = getAnonId()
    expect(getAnonId()).toBe(a)
    resetAnon()
    expect(getAnonId()).not.toBe(a)
  })

  it('user id persists and clears', async () => {
    const { getUserId, setUserId, clearUserId } = await import('../identity')
    expect(getUserId()).toBeNull()
    setUserId('u1')
    expect(getUserId()).toBe('u1')
    clearUserId()
    expect(getUserId()).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/identity.spec.ts`
Expected: FAIL (cannot resolve `../identity`).

- [ ] **Step 3: Add `resetAnonId` to the shared util**

In `frontend/web/src/utils/anonId.ts`, after the existing `getOrCreateAnonId` function, add:

```ts
// resetAnonId clears the cached + persisted anonymous id and generates a
// fresh one. Called on logout so a logged-out visitor becomes a new
// anonymous identity (also rotates the X-Anon-ID header). Returns the new id.
export function resetAnonId(): string {
  try {
    localStorage.removeItem(STORAGE_KEY)
  } catch {
    // ignore
  }
  cached = null
  return getOrCreateAnonId()
}
```

(The existing file already declares `const STORAGE_KEY` and `let cached`; this function reuses them. If `cached`/`STORAGE_KEY` are not module-scoped `let`/`const` as the research showed, adjust to match — `cached` must be reassignable.)

- [ ] **Step 4: Implement identity.ts**

Create `frontend/web/src/analytics/identity.ts`:

```ts
// Analytics identity: anonymous id (reuses the app-wide anon id used for the
// X-Anon-ID header) + an optional resolved user id persisted across reloads.
import { getOrCreateAnonId, resetAnonId } from '@/utils/anonId'

const UID_KEY = 'aenig_analytics_uid'

export function getAnonId(): string {
  return getOrCreateAnonId()
}

export function resetAnon(): string {
  return resetAnonId()
}

export function getUserId(): string | null {
  try {
    return localStorage.getItem(UID_KEY)
  } catch {
    return null
  }
}

export function setUserId(id: string): void {
  try {
    localStorage.setItem(UID_KEY, id)
  } catch {
    // ignore
  }
}

export function clearUserId(): void {
  try {
    localStorage.removeItem(UID_KEY)
  } catch {
    // ignore
  }
}
```

- [ ] **Step 5: Run to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/identity.spec.ts`
Expected: PASS (2 tests).

- [ ] **Step 6: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/utils/anonId.ts frontend/web/src/analytics/identity.ts frontend/web/src/analytics/__tests__/identity.spec.ts
git commit -m "feat(analytics-fe): identity module (reuse anon id, persist user id, reset on logout)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 4: Autocapture (selector, PII strip, extract)

**Files:**
- Create: `frontend/web/src/analytics/autocapture.ts`
- Test: `frontend/web/src/analytics/__tests__/autocapture.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/analytics/__tests__/autocapture.spec.ts`:

```ts
import { describe, it, expect } from 'vitest'
import { buildSelector, stripPII, extractClick, isTrackable } from '../autocapture'

describe('buildSelector', () => {
  it('uses id when present', () => {
    const el = document.createElement('button')
    el.id = 'buy'
    expect(buildSelector(el)).toContain('button#buy')
  })

  it('includes a couple of classes', () => {
    const el = document.createElement('a')
    el.className = 'cta primary'
    expect(buildSelector(el)).toContain('a.cta.primary')
  })
})

describe('stripPII', () => {
  it('redacts emails', () => {
    expect(stripPII('mail me at john@example.com now')).not.toContain('john@example.com')
  })
  it('redacts long digit runs (phones/cards)', () => {
    expect(stripPII('call 5551234567')).not.toContain('5551234567')
  })
  it('caps length to 200 chars', () => {
    expect(stripPII('x'.repeat(500)).length).toBeLessThanOrEqual(200)
  })
})

describe('isTrackable', () => {
  it('false when the element opts out via data-no-track', () => {
    const el = document.createElement('button')
    el.setAttribute('data-no-track', '')
    expect(isTrackable(el)).toBe(false)
  })
  it('false when an ancestor opts out', () => {
    const parent = document.createElement('div')
    parent.setAttribute('data-no-track', '')
    const child = document.createElement('button')
    parent.appendChild(child)
    expect(isTrackable(child)).toBe(false)
  })
  it('true for a normal element', () => {
    expect(isTrackable(document.createElement('button'))).toBe(true)
  })
})

describe('extractClick', () => {
  it('captures tag, selector, trimmed text, and data-* attrs', () => {
    const el = document.createElement('button')
    el.id = 'buy'
    el.textContent = '  Buy now  '
    el.setAttribute('data-plan', 'pro')
    el.setAttribute('aria-label', 'ignored')
    const c = extractClick(el)
    expect(c).not.toBeNull()
    expect(c!.el_tag).toBe('button')
    expect(c!.el_selector).toContain('button#buy')
    expect(c!.el_text).toBe('Buy now')
    expect(c!.el_attrs).toEqual({ 'data-plan': 'pro' })
  })

  it('returns null for opted-out elements', () => {
    const el = document.createElement('button')
    el.setAttribute('data-no-track', '')
    expect(extractClick(el)).toBeNull()
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/autocapture.spec.ts`
Expected: FAIL (cannot resolve `../autocapture`).

- [ ] **Step 3: Implement**

Create `frontend/web/src/analytics/autocapture.ts`:

```ts
// Autocapture helpers: build a stable-ish CSS selector, strip PII from text,
// and extract a click descriptor. Respects `data-no-track` opt-out on the
// element or any ancestor.
const EMAIL_RE = /[\w.+-]+@[\w-]+\.[\w.-]+/g
const DIGIT_RUN_RE = /\d{6,}/g // phone / card-like runs
const MAX_TEXT = 200
const MAX_DEPTH = 3

export function isTrackable(el: Element | null): boolean {
  let cur: Element | null = el
  while (cur) {
    if (cur.hasAttribute && cur.hasAttribute('data-no-track')) return false
    cur = cur.parentElement
  }
  return true
}

function selectorPart(el: Element): string {
  let part = el.tagName.toLowerCase()
  if (el.id) {
    part += `#${el.id}`
    return part
  }
  const cls = (el.getAttribute('class') || '')
    .trim()
    .split(/\s+/)
    .filter(Boolean)
    .slice(0, 2)
  if (cls.length) part += '.' + cls.join('.')
  return part
}

// buildSelector walks up to MAX_DEPTH ancestors, building a descendant path.
// Stops early at an element with an id (ids are unique enough).
export function buildSelector(el: Element): string {
  const parts: string[] = []
  let cur: Element | null = el
  let depth = 0
  while (cur && depth < MAX_DEPTH) {
    const part = selectorPart(cur)
    parts.unshift(part)
    if (part.includes('#')) break
    cur = cur.parentElement
    depth++
  }
  return parts.join(' > ')
}

export function stripPII(text: string): string {
  return text
    .replace(EMAIL_RE, '[email]')
    .replace(DIGIT_RUN_RE, '[num]')
    .slice(0, MAX_TEXT)
}

export interface ClickDescriptor {
  el_tag: string
  el_selector: string
  el_text: string
  el_attrs: Record<string, string>
}

export function extractClick(el: Element): ClickDescriptor | null {
  if (!isTrackable(el)) return null
  const attrs: Record<string, string> = {}
  for (const a of Array.from(el.attributes)) {
    if (a.name.startsWith('data-') && a.name !== 'data-no-track') {
      attrs[a.name] = a.value
    }
  }
  const raw = (el.textContent || '').trim().replace(/\s+/g, ' ')
  return {
    el_tag: el.tagName.toLowerCase(),
    el_selector: buildSelector(el),
    el_text: stripPII(raw),
    el_attrs: attrs,
  }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/autocapture.spec.ts`
Expected: PASS (all groups).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/analytics/autocapture.ts frontend/web/src/analytics/__tests__/autocapture.spec.ts
git commit -m "feat(analytics-fe): click autocapture (selector, PII strip, data-no-track)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 5: Transport (batch buffer + sendBeacon)

**Files:**
- Create: `frontend/web/src/analytics/transport.ts`
- Test: `frontend/web/src/analytics/__tests__/transport.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/analytics/__tests__/transport.spec.ts`:

```ts
import { describe, it, expect, beforeEach, vi } from 'vitest'
import type { AnalyticsEvent } from '../types'

function evt(t: AnalyticsEvent['event_type']): AnalyticsEvent {
  return { event_type: t, timestamp: new Date().toISOString() }
}

describe('transport', () => {
  let beacon: ReturnType<typeof vi.fn>

  beforeEach(async () => {
    vi.resetModules()
    beacon = vi.fn().mockReturnValue(true)
    // @ts-expect-error jsdom has no sendBeacon by default
    navigator.sendBeacon = beacon
    localStorage.clear()
  })

  it('flush sends a single envelope with buffered events', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/api/analytics/collect', maxBatch: 100, flushMs: 999999 })
    t.enqueue(evt('pageview'))
    t.enqueue(evt('click'))
    t.flush('manual')
    expect(beacon).toHaveBeenCalledTimes(1)
    const [url, blob] = beacon.mock.calls[0]
    expect(url).toContain('/api/analytics/collect')
    const text = await (blob as Blob).text()
    const env = JSON.parse(text)
    expect(env.events).toHaveLength(2)
    expect(env.anonymous_id).toBeTruthy()
    expect(env.session_id).toBeTruthy()
  })

  it('flush is a no-op when the buffer is empty', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/x', maxBatch: 100, flushMs: 999999 })
    t.flush('manual')
    expect(beacon).not.toHaveBeenCalled()
  })

  it('auto-flushes when the buffer reaches maxBatch', async () => {
    const { Transport } = await import('../transport')
    const t = new Transport({ endpoint: '/x', maxBatch: 2, flushMs: 999999 })
    t.enqueue(evt('click'))
    t.enqueue(evt('click')) // hits maxBatch -> flush
    expect(beacon).toHaveBeenCalledTimes(1)
  })

  it('falls back to fetch keepalive when sendBeacon returns false', async () => {
    const { Transport } = await import('../transport')
    beacon.mockReturnValue(false)
    const fetchMock = vi.fn().mockResolvedValue(undefined)
    vi.stubGlobal('fetch', fetchMock)
    const t = new Transport({ endpoint: '/x', maxBatch: 100, flushMs: 999999 })
    t.enqueue(evt('pageview'))
    t.flush('manual')
    expect(fetchMock).toHaveBeenCalledTimes(1)
    const opts = fetchMock.mock.calls[0][1]
    expect(opts.keepalive).toBe(true)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/transport.spec.ts`
Expected: FAIL (cannot resolve `../transport`).

- [ ] **Step 3: Implement**

Create `frontend/web/src/analytics/transport.ts`:

```ts
// Transport batches events and ships them as one envelope via sendBeacon
// (fetch+keepalive fallback). Mirrors the useWatchSession.ts beacon pattern.
import type { AnalyticsConfig, AnalyticsEnvelope, AnalyticsEvent } from './types'
import { getAnonId, getUserId } from './identity'
import { getSessionId } from './session'

const BEACON_LIMIT = 60 * 1024 // stay under the ~64 KB sendBeacon cap

export class Transport {
  private buf: AnalyticsEvent[] = []
  private timer: ReturnType<typeof setInterval> | null = null
  private readonly endpoint: string
  private readonly maxBatch: number
  private readonly flushMs: number

  constructor(cfg: AnalyticsConfig) {
    this.endpoint = cfg.endpoint
    this.maxBatch = cfg.maxBatch ?? 20
    this.flushMs = cfg.flushMs ?? 5000
  }

  enqueue(e: AnalyticsEvent): void {
    this.buf.push(e)
    if (this.buf.length >= this.maxBatch) this.flush('size')
  }

  startAutoFlush(): void {
    if (this.timer) return
    this.timer = setInterval(() => this.flush('interval'), this.flushMs)
  }

  stopAutoFlush(): void {
    if (this.timer) {
      clearInterval(this.timer)
      this.timer = null
    }
  }

  flush(_reason: string): void {
    if (this.buf.length === 0) return
    const events = this.buf
    this.buf = []
    const envelope: AnalyticsEnvelope = {
      anonymous_id: getAnonId(),
      user_id: getUserId(),
      session_id: getSessionId(),
      events,
      ctx: {
        user_agent: navigator.userAgent,
        screen_w: window.screen?.width ?? 0,
        screen_h: window.screen?.height ?? 0,
      },
    }
    this.send(envelope)
  }

  private send(envelope: AnalyticsEnvelope): void {
    const payload = JSON.stringify(envelope)
    // Oversized batch: split events in half and recurse.
    if (payload.length > BEACON_LIMIT && envelope.events.length > 1) {
      const mid = Math.ceil(envelope.events.length / 2)
      this.send({ ...envelope, events: envelope.events.slice(0, mid) })
      this.send({ ...envelope, events: envelope.events.slice(mid) })
      return
    }
    // text/plain keeps it a CORS "simple" request (no preflight); the backend
    // reads the raw body regardless of content-type.
    try {
      const blob = new Blob([payload], { type: 'text/plain' })
      const ok = navigator.sendBeacon(this.endpoint, blob)
      if (ok) return
    } catch {
      // fall through to fetch
    }
    void fetch(this.endpoint, {
      method: 'POST',
      headers: { 'Content-Type': 'text/plain' },
      body: payload,
      keepalive: true,
      credentials: 'include',
    }).catch(() => undefined)
  }
}
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/transport.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/analytics/transport.ts frontend/web/src/analytics/__tests__/transport.spec.ts
git commit -m "feat(analytics-fe): batching transport (sendBeacon + fetch fallback, 64KB split)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 6: Public `analytics` singleton (init/page/track/identify/reset + heartbeat + autocapture)

**Files:**
- Create: `frontend/web/src/analytics/index.ts`
- Test: `frontend/web/src/analytics/__tests__/index.spec.ts`

- [ ] **Step 1: Write the failing test**

Create `frontend/web/src/analytics/__tests__/index.spec.ts`:

```ts
import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'

describe('analytics singleton', () => {
  let beacon: ReturnType<typeof vi.fn>

  beforeEach(() => {
    vi.resetModules()
    localStorage.clear()
    beacon = vi.fn().mockReturnValue(true)
    // @ts-expect-error jsdom lacks sendBeacon
    navigator.sendBeacon = beacon
  })

  afterEach(() => {
    vi.restoreAllMocks()
    vi.useRealTimers()
  })

  it('does not send before init', async () => {
    const { analytics } = await import('../index')
    analytics.page()
    analytics.track('foo')
    analytics.flushNow()
    expect(beacon).not.toHaveBeenCalled()
  })

  it('page() enqueues a pageview that flushes', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/api/analytics/collect', flushMs: 999999 })
    analytics.page()
    analytics.flushNow()
    expect(beacon).toHaveBeenCalledTimes(1)
    const env = JSON.parse(await (beacon.mock.calls[0][1] as Blob).text())
    expect(env.events[0].event_type).toBe('pageview')
  })

  it('identify sets user_id on subsequent events; reset clears it', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    analytics.identify('u1')
    analytics.track('after_login')
    analytics.flushNow()
    let env = JSON.parse(await (beacon.mock.calls.at(-1)![1] as Blob).text())
    expect(env.user_id).toBe('u1')

    analytics.reset()
    analytics.track('after_logout')
    analytics.flushNow()
    env = JSON.parse(await (beacon.mock.calls.at(-1)![1] as Blob).text())
    expect(env.user_id).toBeNull()
  })

  it('a click on the document is autocaptured after init', async () => {
    const { analytics } = await import('../index')
    analytics.init({ endpoint: '/x', flushMs: 999999 })
    const btn = document.createElement('button')
    btn.id = 'buy'
    document.body.appendChild(btn)
    btn.dispatchEvent(new MouseEvent('click', { bubbles: true }))
    analytics.flushNow()
    const env = JSON.parse(await (beacon.mock.calls.at(-1)![1] as Blob).text())
    const click = env.events.find((e: { event_type: string }) => e.event_type === 'click')
    expect(click).toBeTruthy()
    expect(click.el_selector).toContain('button#buy')
    document.body.removeChild(btn)
  })
})
```

- [ ] **Step 2: Run to verify it fails**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/index.spec.ts`
Expected: FAIL (cannot resolve `../index`).

- [ ] **Step 3: Implement**

Create `frontend/web/src/analytics/index.ts`:

```ts
// Public analytics API. A single module-level instance is initialized once at
// app bootstrap. Before init(), all methods are no-ops (consent/flag gate is
// applied by the caller in main.ts).
import type { AnalyticsConfig, AnalyticsEvent, EventType } from './types'
import { Transport } from './transport'
import { extractClick } from './autocapture'
import { setUserId, clearUserId, resetAnon } from './identity'

class Analytics {
  private transport: Transport | null = null
  private heartbeatMs = 15000
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null
  private lastBeat = 0
  private clickListener: ((e: MouseEvent) => void) | null = null
  private visibilityListener: (() => void) | null = null
  private pagehideListener: (() => void) | null = null

  init(cfg: AnalyticsConfig): void {
    if (this.transport) return // idempotent
    this.transport = new Transport(cfg)
    this.heartbeatMs = cfg.heartbeatMs ?? 15000
    this.transport.startAutoFlush()

    // Autocapture clicks via one delegated listener.
    this.clickListener = (e: MouseEvent) => {
      const target = e.target as Element | null
      if (!target) return
      const desc = extractClick(target)
      if (!desc) return
      this.enqueue({ event_type: 'click', timestamp: nowISO(), path: location.pathname, ...desc })
    }
    document.addEventListener('click', this.clickListener, { capture: true })

    // Heartbeat while foregrounded.
    this.lastBeat = Date.now()
    this.startHeartbeat()
    this.visibilityListener = () => {
      if (document.visibilityState === 'hidden') {
        this.stopHeartbeat()
        this.flushNow()
      } else {
        this.lastBeat = Date.now()
        this.startHeartbeat()
      }
    }
    document.addEventListener('visibilitychange', this.visibilityListener)

    this.pagehideListener = () => this.flushNow()
    window.addEventListener('pagehide', this.pagehideListener)

    // Initial pageview.
    this.page()
  }

  page(props?: Record<string, unknown>): void {
    this.enqueue({
      event_type: 'pageview',
      timestamp: nowISO(),
      url: location.href,
      path: location.pathname,
      referrer: document.referrer,
      title: document.title,
      properties: props,
    })
  }

  track(name: string, props?: Record<string, unknown>): void {
    this.enqueue({ event_type: 'custom', event_name: name, timestamp: nowISO(), path: location.pathname, properties: props })
  }

  identify(userId: string): void {
    if (!userId) return
    setUserId(userId)
    this.enqueue({ event_type: 'identify', timestamp: nowISO(), path: location.pathname })
  }

  reset(): void {
    clearUserId()
    resetAnon()
  }

  flushNow(): void {
    this.transport?.flush('manual')
  }

  private enqueue(e: AnalyticsEvent): void {
    if (!this.transport) return
    this.transport.enqueue(e)
  }

  private startHeartbeat(): void {
    if (this.heartbeatTimer || !this.transport) return
    this.heartbeatTimer = setInterval(() => {
      const now = Date.now()
      const active = now - this.lastBeat
      this.lastBeat = now
      this.enqueue({ event_type: 'heartbeat', timestamp: nowISO(), path: location.pathname, active_ms: active })
    }, this.heartbeatMs)
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer)
      this.heartbeatTimer = null
    }
  }
}

function nowISO(): string {
  return new Date().toISOString()
}

// type re-export for callers
export type { EventType }

export const analytics = new Analytics()
```

- [ ] **Step 4: Run to verify it passes**

Run: `cd /data/animeenigma/frontend/web && bunx vitest run src/analytics/__tests__/index.spec.ts`
Expected: PASS (4 tests).

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/analytics/index.ts frontend/web/src/analytics/__tests__/index.spec.ts
git commit -m "feat(analytics-fe): analytics singleton (page/track/identify/reset + heartbeat + autocapture)

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 7: Integration — bootstrap, router, auth store

**Files:**
- Modify: `frontend/web/src/main.ts`
- Modify: `frontend/web/src/router/index.ts`
- Modify: `frontend/web/src/stores/auth.ts`

- [ ] **Step 1: Initialize the snippet at bootstrap (flag-gated)**

In `frontend/web/src/main.ts`, inside the existing deferred-init block (next to the diagnostics import), add the analytics init. Use the same `deferInit` callback pattern already present:

```ts
deferInit(() => {
  if (import.meta.env.VITE_ANALYTICS_ENABLED !== 'false') {
    import('./analytics').then(({ analytics }) => {
      const base = import.meta.env.VITE_API_URL || '/api'
      analytics.init({ endpoint: `${base}/analytics/collect` })
    })
  }
})
```

(If `deferInit` wraps a single callback already used for diagnostics, add a second `deferInit(...)` call or extend the existing callback body — match the file's actual structure. Default-on: only `'false'` disables.)

- [ ] **Step 2: Fire a pageview on every route change**

In `frontend/web/src/router/index.ts`, after the router is created and the existing guards are registered (before `export default router`), add:

```ts
// Clickstream pageview on every successful navigation (Plan 2). Lazy import
// keeps analytics out of the router's critical path and SSR-safe.
router.afterEach((to) => {
  if (import.meta.env.VITE_ANALYTICS_ENABLED === 'false') return
  import('@/analytics').then(({ analytics }) => {
    analytics.page({ route: typeof to.name === 'string' ? to.name : undefined })
  })
})
```

- [ ] **Step 3: Wire identify/reset to auth**

In `frontend/web/src/stores/auth.ts`:

In the `setUser` helper, after it persists a non-null user, identify them:
```ts
const setUser = (userData: User | null) => {
  user.value = userData
  if (userData) {
    localStorage.setItem('user', JSON.stringify(userData))
    import('@/analytics').then(({ analytics }) => analytics.identify(userData.id)).catch(() => undefined)
  } else {
    localStorage.removeItem('user')
  }
}
```

In the `logout` action, after the token is cleared, reset analytics:
```ts
  // after localStorage.removeItem('token') / clearPreferenceCache()
  import('@/analytics').then(({ analytics }) => analytics.reset()).catch(() => undefined)
```

(Match the exact surrounding code from the file. `setUser(null)` during logout must NOT call identify — the `if (userData)` guard already handles that; `reset()` is called explicitly in `logout`.)

- [ ] **Step 4: Verify type-check, lint, and the full analytics test suite**

Run:
```bash
cd /data/animeenigma/frontend/web && bunx tsc --noEmit 2>&1 | tail -8
bunx vitest run src/analytics/ 2>&1 | tail -12
bun lint 2>&1 | tail -8
```
Expected: tsc clean (no new errors), all analytics specs pass, lint 0 errors. Fix any issues.

- [ ] **Step 5: Commit**

```bash
cd /data/animeenigma
git add frontend/web/src/main.ts frontend/web/src/router/index.ts frontend/web/src/stores/auth.ts
git commit -m "feat(analytics-fe): wire snippet — bootstrap init, router pageviews, auth identify/reset

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Task 8: Grafana — PostgreSQL datasource + product-analytics dashboard

**Files:**
- Modify: `docker/grafana/provisioning/datasources/datasources.yml`
- Create: `infra/grafana/dashboards/product-analytics.json`

- [ ] **Step 1: Add the PostgreSQL datasource**

Append to `docker/grafana/provisioning/datasources/datasources.yml` (under `datasources:`):

```yaml
  - name: PostgreSQL
    uid: aenigma-postgres
    type: postgres
    access: proxy
    url: postgres:5432
    user: postgres
    jsonData:
      database: animeenigma
      sslmode: disable
      postgresVersion: 1600
      timescaledb: false
    secureJsonData:
      password: postgres
    editable: false
```

(Mirrors the existing Prometheus/Loki entries. The fixed `uid: aenigma-postgres` lets the dashboard reference it deterministically. Creds match the postgres service in `docker/docker-compose.yml`.)

- [ ] **Step 2: Create the dashboard**

Create `infra/grafana/dashboards/product-analytics.json`:

```json
{
  "annotations": { "list": [] },
  "editable": true,
  "graphTooltip": 0,
  "id": null,
  "uid": "product-analytics",
  "title": "Product Analytics (Clickstream)",
  "description": "Clickstream over analytics_events_resolved (Plan 1/2). person_id = identified user if known, else anonymous.",
  "tags": ["analytics", "clickstream"],
  "time": { "from": "now-7d", "to": "now" },
  "timezone": "",
  "schemaVersion": 39,
  "version": 1,
  "panels": [
    {
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "title": "Unique visitors",
      "type": "stat",
      "gridPos": { "h": 4, "w": 6, "x": 0, "y": 0 },
      "id": 1,
      "fieldConfig": { "defaults": { "unit": "short" }, "overrides": [] },
      "options": { "reduceOptions": { "calcs": ["lastNotNull"] } },
      "targets": [
        {
          "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
          "rawQuery": true,
          "format": "table",
          "refId": "A",
          "rawSql": "SELECT count(DISTINCT person_id) AS visitors FROM analytics_events_resolved WHERE $__timeFilter(timestamp)"
        }
      ]
    },
    {
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "title": "Sessions",
      "type": "stat",
      "gridPos": { "h": 4, "w": 6, "x": 6, "y": 0 },
      "id": 2,
      "fieldConfig": { "defaults": { "unit": "short" }, "overrides": [] },
      "options": { "reduceOptions": { "calcs": ["lastNotNull"] } },
      "targets": [
        {
          "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
          "rawQuery": true,
          "format": "table",
          "refId": "A",
          "rawSql": "SELECT count(DISTINCT session_id) AS sessions FROM analytics_events_resolved WHERE $__timeFilter(timestamp)"
        }
      ]
    },
    {
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "title": "Anonymous vs identified visitors",
      "type": "piechart",
      "gridPos": { "h": 4, "w": 12, "x": 12, "y": 0 },
      "id": 3,
      "fieldConfig": { "defaults": {}, "overrides": [] },
      "options": { "legend": { "displayMode": "list", "placement": "right" } },
      "targets": [
        {
          "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
          "rawQuery": true,
          "format": "table",
          "refId": "A",
          "rawSql": "SELECT CASE WHEN resolved_user_id IS NULL THEN 'anonymous' ELSE 'identified' END AS kind, count(DISTINCT person_id) AS visitors FROM analytics_events_resolved WHERE $__timeFilter(timestamp) GROUP BY 1"
        }
      ]
    },
    {
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "title": "Pageviews over time",
      "type": "timeseries",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 4 },
      "id": 4,
      "fieldConfig": { "defaults": { "custom": { "drawStyle": "line", "fillOpacity": 10 } }, "overrides": [] },
      "options": { "legend": { "displayMode": "list", "placement": "bottom" } },
      "targets": [
        {
          "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
          "rawQuery": true,
          "format": "time_series",
          "refId": "A",
          "rawSql": "SELECT $__timeGroupAlias(timestamp,'1h') AS time, count(*) AS pageviews FROM analytics_events_resolved WHERE event_type='pageview' AND $__timeFilter(timestamp) GROUP BY 1 ORDER BY 1"
        }
      ]
    },
    {
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "title": "Top clicked elements",
      "type": "table",
      "gridPos": { "h": 8, "w": 12, "x": 12, "y": 4 },
      "id": 5,
      "fieldConfig": { "defaults": {}, "overrides": [] },
      "options": {},
      "targets": [
        {
          "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
          "rawQuery": true,
          "format": "table",
          "refId": "A",
          "rawSql": "SELECT el_selector, count(*) AS clicks FROM analytics_events_resolved WHERE event_type='click' AND coalesce(el_selector,'') <> '' AND $__timeFilter(timestamp) GROUP BY el_selector ORDER BY clicks DESC LIMIT 25"
        }
      ]
    },
    {
      "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
      "title": "Time on page (active seconds, top paths)",
      "type": "table",
      "gridPos": { "h": 8, "w": 12, "x": 0, "y": 12 },
      "id": 6,
      "fieldConfig": { "defaults": {}, "overrides": [] },
      "options": {},
      "targets": [
        {
          "datasource": { "type": "postgres", "uid": "aenigma-postgres" },
          "rawQuery": true,
          "format": "table",
          "refId": "A",
          "rawSql": "SELECT path, round(sum(coalesce(active_ms,0))/1000.0,1) AS active_seconds, count(DISTINCT session_id) AS sessions FROM analytics_events_resolved WHERE event_type='heartbeat' AND $__timeFilter(timestamp) GROUP BY path ORDER BY active_seconds DESC LIMIT 25"
        }
      ]
    }
  ]
}
```

- [ ] **Step 3: Validate the dashboard JSON**

Run: `cd /data/animeenigma && python3 -c "import json;json.load(open('infra/grafana/dashboards/product-analytics.json'));print('JSON OK')"`
Expected: `JSON OK`.

- [ ] **Step 4: Commit**

```bash
cd /data/animeenigma
git add docker/grafana/provisioning/datasources/datasources.yml infra/grafana/dashboards/product-analytics.json
git commit -m "feat(analytics-fe): Grafana Postgres datasource + product-analytics dashboard

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
Co-Authored-By: 0neymik0 <0neymik0@gmail.com>
Co-Authored-By: NANDIorg <super.egor.mamonov@yandex.ru>"
```

---

## Deploy & verify (via /animeenigma-after-update, on explicit user OK)

Not part of task execution. When deploying:
1. `make redeploy-web` (rebuilds the frontend with the snippet; runs i18n-lint + type-check).
2. `make restart-grafana` (loads the new datasource + dashboard).
3. Browser smoke: open the site, navigate between 2-3 routes, click a button → confirm rows appear in `analytics_events` (`event_type` in pageview/click/heartbeat) and the "Product Analytics" Grafana dashboard populates. Log in → confirm an `identify` row and that prior anonymous events resolve to the user via `person_id`.

## Self-Review (spec coverage)

- Pageviews → Task 6 `page()` + Task 7 router hook. ✅
- Autocapture clicks (selector, PII strip, data-no-track) → Task 4 + Task 6 listener. ✅
- Heartbeat / time-on-page → Task 6 heartbeat + Task 8 dashboard panel. ✅
- Batching + sendBeacon + fallback + 64 KB split → Task 5. ✅
- anonymous_id (reused) + session + identify/reset → Tasks 2, 3, 6, 7. ✅
- `VITE_ANALYTICS_ENABLED` gate → Tasks 1, 7. ✅
- Anonymous vs identified, top clicks, retention-adjacent panels → Task 8. ✅
- Privacy (PII strip, no raw input values, hashed IP done server-side) → Task 4 (text); raw IP never leaves the browser anyway. ✅
- Deferred (documented): `trace_id` (Plan 3), `device_type` (backend doesn't map it), full retention() cohort panel (Postgres hand-rolled — add later if needed).

## Follow-on
- **Plan 3 — Distributed tracing**: wire `libs/tracing`, OTel Collector + Tempo, gateway `traceparent` propagation, and the browser axios `traceparent` + `trace_id` stamping that lets a click join its backend trace (the `AnalyticsEvent.trace_id` field is reserved for this).
