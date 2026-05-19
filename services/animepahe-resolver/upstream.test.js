/*
 * services/animepahe-resolver/upstream.test.js
 *
 * Phase 27 / Plan 27-01 — unit tests for the 403-retry + page-recycle logic.
 *
 * Strategy: inject a fake `page` whose `evaluate` is sequenced by a queue of
 * scripted responses. The same fake also stubs `goto` / `setUserAgent` / `close`
 * so browser.refreshChallenge() + recyclePage() run end-to-end against doubles.
 */

const test = require('node:test');
const assert = require('node:assert');

const browser = require('./browser');
const upstream = require('./upstream');
const metrics = require('./metrics');

function makeScriptedPage(responses) {
  const queue = responses.slice();
  return {
    evaluateCalls: 0,
    gotoCalls: 0,
    async evaluate(fn) {
      this.evaluateCalls += 1;
      // Healthz probe path (no args) — we don't drive it from these tests.
      if (fn.length === 0) return 1;
      if (queue.length === 0) {
        throw new Error('test bug: page.evaluate called more times than scripted');
      }
      const next = queue.shift();
      if (typeof next === 'function') return next();
      return next;
    },
    async goto() {
      this.gotoCalls += 1;
    },
    async setUserAgent() {
      /* no-op */
    },
    async close() {
      this.closed = true;
    },
  };
}

function makeBrowserDouble(initialPage) {
  let current = initialPage;
  return {
    pagesCreated: 0,
    async newPage() {
      this.pagesCreated += 1;
      const next = makeScriptedPage([
        // After recycle, the next request lands a 200 by default; tests that
        // exercise post-recycle responses can swap to a richer fake.
        { status: 200, body: '{"ok":1}' },
      ]);
      current = next;
      return next;
    },
    setCurrent(p) {
      current = p;
    },
    getCurrent() {
      return current;
    },
    async close() {
      /* no-op */
    },
  };
}

function installDoubles(scripted) {
  const br = makeBrowserDouble(scripted);
  browser._setTestDoubles({ browser: br, page: scripted });
  upstream.resetForTests();
  return br;
}

function teardown() {
  browser._setTestDoubles(null);
  upstream.resetForTests();
  delete process.env.PAGE_RECYCLE_AT;
}

// ---------------------------------------------------------------------------
// 27-01 — 403-on-first → retry → success increments stealth_challenge_solves_total
// ---------------------------------------------------------------------------

test('403 on first attempt → re-navigate → retry succeeds; counters move correctly',
  async () => {
    const page = makeScriptedPage([
      { status: 403, body: 'challenge' },
      { status: 200, body: '{"ok":1}' },
    ]);
    installDoubles(page);
    try {
      const result = await upstream.fetchWithRetry('https://animepahe.pw/api?m=search&q=Frieren');
      assert.strictEqual(result.status, 200);
      assert.strictEqual(result.body, '{"ok":1}');

      const solves = await metrics.stealthChallengeSolvesTotal.get();
      const failures = await metrics.stealthChallengeFailuresTotal.get();
      const r403 = await metrics.upstream403Total.get();
      assert.strictEqual(solves.values[0].value, 1, 'solves should be 1');
      assert.strictEqual(failures.values[0]?.value || 0, 0, 'failures should be 0');
      const firstStage = r403.values.find((v) => v.labels.stage === 'first');
      assert.strictEqual(firstStage && firstStage.value, 1,
        'upstream_403_total{stage="first"} should be 1');

      // page.goto was called once (refreshChallenge after 403).
      assert.strictEqual(page.gotoCalls, 1);

      // lastChallengeSolveAt was updated.
      assert.ok(upstream.getLastChallengeSolveAt(), 'lastChallengeSolveAt should be set');
    } finally {
      teardown();
    }
  });

// ---------------------------------------------------------------------------
// 27-01 — second 403 → 502 + stealth_challenge_failures_total
// ---------------------------------------------------------------------------

test('403 on both attempts → ResolverError(502) + stealth_challenge_failures_total',
  async () => {
    const page = makeScriptedPage([
      { status: 403, body: 'challenge' },
      { status: 403, body: 'challenge' },
    ]);
    installDoubles(page);
    try {
      await assert.rejects(
        () => upstream.fetchWithRetry('https://animepahe.pw/api?m=search&q=Frieren'),
        (err) => {
          assert.strictEqual(err.name, 'ResolverError');
          assert.strictEqual(err.status, 502);
          assert.strictEqual(err.code, 'stealth_challenge_failed');
          return true;
        },
      );

      const failures = await metrics.stealthChallengeFailuresTotal.get();
      assert.strictEqual(failures.values[0].value, 1, 'failures should be 1');
      const r403 = await metrics.upstream403Total.get();
      const second = r403.values.find((v) => v.labels.stage === 'second');
      assert.strictEqual(second && second.value, 1,
        'upstream_403_total{stage="second"} should be 1');
    } finally {
      teardown();
    }
  });

// ---------------------------------------------------------------------------
// 27-01 — page-recycle counter ticks after PAGE_RECYCLE_AT successful requests
// ---------------------------------------------------------------------------

test('page-recycle: after PAGE_RECYCLE_AT=3 successful fetches page_recycle_total === 1',
  async () => {
    process.env.PAGE_RECYCLE_AT = '3';
    // Each fetchOnce on the initial page consumes one scripted response.
    const page = makeScriptedPage([
      { status: 200, body: '{"r":1}' },
      { status: 200, body: '{"r":2}' },
      { status: 200, body: '{"r":3}' },
    ]);
    installDoubles(page);
    try {
      assert.strictEqual(upstream.pageRecycleAt(), 3);

      await upstream.fetchWithRetry('https://animepahe.pw/api?m=search&q=a');
      await upstream.fetchWithRetry('https://animepahe.pw/api?m=search&q=b');
      // The 3rd request triggers maybeRecycle() AFTER the response is returned.
      await upstream.fetchWithRetry('https://animepahe.pw/api?m=search&q=c');

      const recycles = await metrics.pageRecycleTotal.get();
      assert.strictEqual(recycles.values[0].value, 1, 'page_recycle_total should be 1 after 3 successful fetches');
    } finally {
      teardown();
    }
  });

// ---------------------------------------------------------------------------
// host-allowlist — direct calls to fetchWithRetry with non-allowlisted hosts
// ---------------------------------------------------------------------------

test('fetchWithRetry rejects non-allowlisted hosts before touching the page',
  async () => {
    const page = makeScriptedPage([]); // empty queue — any evaluate() would throw
    installDoubles(page);
    try {
      await assert.rejects(
        () => upstream.fetchWithRetry('https://malsync.moe/api/...'),
        (err) => err.code === 'host_not_allowed' && err.status === 400,
      );
      assert.strictEqual(page.evaluateCalls, 0,
        'page.evaluate must NOT be called when host-allowlist rejects');
    } finally {
      teardown();
    }
  });

test('fetchWithRetry rejects when browser is not initialized',
  async () => {
    browser._setTestDoubles(null);
    upstream.resetForTests();
    try {
      await assert.rejects(
        () => upstream.fetchWithRetry('https://animepahe.pw/api?m=search&q=x'),
        (err) => err.code === 'browser_down' && err.status === 503,
      );
    } finally {
      teardown();
    }
  });

test('maybeRecycle is a no-op before the first request',
  async () => {
    process.env.PAGE_RECYCLE_AT = '5';
    const page = makeScriptedPage([]);
    installDoubles(page);
    try {
      await upstream.maybeRecycle();
      const recycles = await metrics.pageRecycleTotal.get();
      const v = recycles.values[0]?.value || 0;
      assert.strictEqual(v, 0,
        'maybeRecycle on requestCount=0 must not increment page_recycle_total');
    } finally {
      teardown();
    }
  });
