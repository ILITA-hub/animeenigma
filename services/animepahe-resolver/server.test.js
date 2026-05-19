/*
 * services/animepahe-resolver/server.test.js
 *
 * Phase 27 / Plan 27-01 — unit tests for the Fastify server's routes + schema
 * validation. Run with `npm test` (alias for `node --test`).
 *
 * Covers VALIDATION rows:
 *   27-01-01 (validation)       — Fastify schema rejects missing/malformed query params
 *   27-01-02 (host-allowlist)   — upstream.fetchWithRetry rejects non-`animepahe.pw` hosts
 *   27-01-03 (healthz)          — two-layer probe returns 200 on success / 503 on failure
 *   27-01-04..06 (search/release/play) — handlers passthrough the stubbed upstream body
 *
 * Strategy: inject a fake `page` object + monkey-patch upstream.fetchWithRetry
 * via the exported module surface. No real Chromium spawned.
 */

const test = require('node:test');
const assert = require('node:assert');
const fs = require('node:fs');
const path = require('node:path');

const browser = require('./browser');
const upstream = require('./upstream');
const metrics = require('./metrics');
const { buildApp } = require('./server');

const fixturePath = path.join(__dirname, '__fixtures__', 'intercepted-frieren.json');
const fixture = JSON.parse(fs.readFileSync(fixturePath, 'utf8'));

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

function makeFakePage(opts) {
  opts = opts || {};
  return {
    async evaluate(fn) {
      // /healthz probe path: page.evaluate(() => 1)
      if (fn.length === 0) {
        if (opts.probeThrows) throw new Error(opts.probeThrows);
        if (opts.probeHangs) {
          return new Promise((res) => setTimeout(() => res(opts.probeValue ?? 1), 5000));
        }
        return opts.probeValue ?? 1;
      }
      // fetchOnce path: page.evaluate(async (u) => fetch(u))
      return opts.fetchResult || { status: 200, body: '{}' };
    },
    async goto() {
      /* no-op */
    },
    async setUserAgent() {
      /* no-op */
    },
    async close() {
      /* no-op */
    },
  };
}

function makeFakeBrowser() {
  return {
    async newPage() {
      return makeFakePage();
    },
    async close() {
      /* no-op */
    },
  };
}

function installTestDoubles(opts) {
  browser._setTestDoubles({ browser: makeFakeBrowser(), page: makeFakePage(opts) });
  upstream.resetForTests();
}

function teardown() {
  browser._setTestDoubles(null);
  upstream.resetForTests();
}

function withApp(opts, runner) {
  return async () => {
    installTestDoubles(opts);
    const app = buildApp({ logger: false });
    await app.ready();
    try {
      await runner(app);
    } finally {
      await app.close();
      teardown();
    }
  };
}

// ---------------------------------------------------------------------------
// 27-01-01 — validation
// ---------------------------------------------------------------------------

test('validation: /release without session returns 400', withApp({}, async (app) => {
  const res = await app.inject({ method: 'GET', url: '/release' });
  assert.strictEqual(res.statusCode, 400);
  const body = res.json();
  assert.strictEqual(body.error, 'bad_request');
}));

test('validation: /play without animeSession returns 400', withApp({}, async (app) => {
  const res = await app.inject({
    method: 'GET',
    url: '/play?episodeSession=7bf604bac56a6a9269bc0ce04083169abeaa4815c65e2a320e0ad185334c85e7',
  });
  assert.strictEqual(res.statusCode, 400);
}));

test('validation: /play with non-UUID-shaped episodeSession returns 400', withApp({}, async (app) => {
  // The schema pattern allows [A-Za-z0-9-]{16,128}. A short value like "bad" must 400.
  const res = await app.inject({
    method: 'GET',
    url: '/play?animeSession=65a00d22-e684-4a33-5fa2-707b8e64a84d&episodeSession=bad',
  });
  assert.strictEqual(res.statusCode, 400);
}));

test('validation: /play with characters outside the allowed set returns 400', withApp({}, async (app) => {
  // `;` and `/` are not in [A-Za-z0-9-].
  const res = await app.inject({
    method: 'GET',
    url: '/play?animeSession=65a00d22-e684-4a33-5fa2-707b8e64a84d&episodeSession=' +
         encodeURIComponent('abc;/etc/passwd'),
  });
  assert.strictEqual(res.statusCode, 400);
}));

test('validation: /search without q returns 400', withApp({}, async (app) => {
  const res = await app.inject({ method: 'GET', url: '/search' });
  assert.strictEqual(res.statusCode, 400);
}));

// ---------------------------------------------------------------------------
// 27-01-02 — host-allowlist (defense-in-depth)
// ---------------------------------------------------------------------------

test('host-allowlist: fetchWithRetry rejects non-animepahe.pw hosts', async () => {
  installTestDoubles({});
  try {
    await assert.rejects(
      () => upstream.fetchWithRetry('https://evil.example.com/api?m=search&q=x'),
      (err) => {
        assert.strictEqual(err.name, 'ResolverError');
        assert.strictEqual(err.status, 400);
        assert.strictEqual(err.code, 'host_not_allowed');
        return true;
      },
    );
    // animepahe.pw itself MUST be accepted (no throw at the allowlist guard).
    upstream.assertAllowedHost('https://animepahe.pw/api?m=search&q=x');
    // Sneaky lookalikes: animepahe.pw.evil.com, sub.animepahe.pw — only EXACT match.
    assert.throws(
      () => upstream.assertAllowedHost('https://animepahe.pw.evil.com/'),
      (err) => err.code === 'host_not_allowed',
    );
    assert.throws(
      () => upstream.assertAllowedHost('https://sub.animepahe.pw/'),
      (err) => err.code === 'host_not_allowed',
    );
  } finally {
    teardown();
  }
});

// ---------------------------------------------------------------------------
// 27-01-03 — healthz two-layer probe
// ---------------------------------------------------------------------------

test('healthz: returns 200 {browser:"up"} when page.evaluate(()=>1) resolves to 1',
  withApp({ probeValue: 1 }, async (app) => {
    const res = await app.inject({ method: 'GET', url: '/healthz' });
    assert.strictEqual(res.statusCode, 200);
    const body = res.json();
    assert.strictEqual(body.browser, 'up');
    assert.ok(Object.prototype.hasOwnProperty.call(body, 'lastChallengeSolveAt'));
    assert.ok(Object.prototype.hasOwnProperty.call(body, 'pageCount'));
  }),
);

test('healthz: returns 503 when page.evaluate throws',
  withApp({ probeThrows: 'browser_crashed' }, async (app) => {
    const res = await app.inject({ method: 'GET', url: '/healthz' });
    assert.strictEqual(res.statusCode, 503);
    const body = res.json();
    assert.strictEqual(body.browser, 'down');
    assert.strictEqual(body.reason, 'browser_crashed');
  }),
);

test('healthz: returns 503 when browser is not initialized', async () => {
  browser._setTestDoubles(null);
  upstream.resetForTests();
  const app = buildApp({ logger: false });
  await app.ready();
  try {
    const res = await app.inject({ method: 'GET', url: '/healthz' });
    assert.strictEqual(res.statusCode, 503);
    const body = res.json();
    assert.strictEqual(body.browser, 'down');
    assert.strictEqual(body.reason, 'not_initialized');
  } finally {
    await app.close();
    teardown();
  }
});

// ---------------------------------------------------------------------------
// 27-01-04 / 05 / 06 — /search /release /play passthrough (with stubbed upstream)
// ---------------------------------------------------------------------------

function stubUpstream(handler) {
  const original = upstream.fetchWithRetry;
  upstream.fetchWithRetry = handler;
  return () => {
    upstream.fetchWithRetry = original;
  };
}

test('search: returns the fixture m=search JSON',
  withApp({}, async (app) => {
    const expectedUrl = 'https://animepahe.pw/api?m=search&q=Frieren';
    const restore = stubUpstream(async (url) => {
      assert.strictEqual(url, expectedUrl);
      return { status: 200, body: JSON.stringify(fixture.search) };
    });
    try {
      const res = await app.inject({ method: 'GET', url: '/search?q=Frieren' });
      assert.strictEqual(res.statusCode, 200);
      assert.deepStrictEqual(res.json(), fixture.search);
    } finally {
      restore();
    }
  }),
);

test('release: returns the fixture m=release JSON for page=1',
  withApp({}, async (app) => {
    const expectedUrl = 'https://animepahe.pw/api?m=release&id=65a00d22-e684-4a33-5fa2-707b8e64a84d&sort=episode_asc&page=1';
    const restore = stubUpstream(async (url) => {
      assert.strictEqual(url, expectedUrl);
      return { status: 200, body: JSON.stringify(fixture.release) };
    });
    try {
      const res = await app.inject({
        method: 'GET',
        url: '/release?session=65a00d22-e684-4a33-5fa2-707b8e64a84d&page=1',
      });
      assert.strictEqual(res.statusCode, 200);
      assert.deepStrictEqual(res.json(), fixture.release);
    } finally {
      restore();
    }
  }),
);

test('play: returns the fixture play HTML verbatim with text/html content-type',
  withApp({}, async (app) => {
    const expectedUrl = 'https://animepahe.pw/play/65a00d22-e684-4a33-5fa2-707b8e64a84d/7bf604bac56a6a9269bc0ce04083169abeaa4815c65e2a320e0ad185334c85e7';
    const restore = stubUpstream(async (url) => {
      assert.strictEqual(url, expectedUrl);
      return { status: 200, body: fixture.play };
    });
    try {
      const res = await app.inject({
        method: 'GET',
        url: '/play?animeSession=65a00d22-e684-4a33-5fa2-707b8e64a84d&episodeSession=7bf604bac56a6a9269bc0ce04083169abeaa4815c65e2a320e0ad185334c85e7',
      });
      assert.strictEqual(res.statusCode, 200);
      assert.ok(res.headers['content-type'].startsWith('text/html'));
      assert.strictEqual(res.payload, fixture.play);
    } finally {
      restore();
    }
  }),
);

test('search: 502 propagates as 502 with resolver_error body',
  withApp({}, async (app) => {
    const restore = stubUpstream(async () => {
      const err = new upstream.ResolverError(502, 'stealth_challenge_failed', 'two 403s');
      throw err;
    });
    try {
      const res = await app.inject({ method: 'GET', url: '/search?q=Frieren' });
      assert.strictEqual(res.statusCode, 502);
      const body = res.json();
      assert.strictEqual(body.error, 'stealth_challenge_failed');
    } finally {
      restore();
    }
  }),
);

test('search: non-JSON upstream body surfaces as 502 upstream_bad_json',
  withApp({}, async (app) => {
    const restore = stubUpstream(async () => ({ status: 200, body: 'not json {{{' }));
    try {
      const res = await app.inject({ method: 'GET', url: '/search?q=Frieren' });
      assert.strictEqual(res.statusCode, 502);
      assert.strictEqual(res.json().error, 'upstream_bad_json');
    } finally {
      restore();
    }
  }),
);

// ---------------------------------------------------------------------------
// CR-01 — upstream 404/5xx must propagate as sidecar error, not HTTP 200
// ---------------------------------------------------------------------------

test('release: upstream 404 propagates as sidecar 404 not_found',
  withApp({}, async (app) => {
    const restore = stubUpstream(async () => ({ status: 404, body: '{"message":""}' }));
    try {
      const res = await app.inject({
        method: 'GET',
        url: '/release?session=65a00d22-e684-4a33-5fa2-707b8e64a84d',
      });
      assert.strictEqual(res.statusCode, 404);
      assert.strictEqual(res.json().error, 'not_found');
    } finally {
      restore();
    }
  }),
);

test('release: upstream 5xx propagates as sidecar 502 upstream_error',
  withApp({}, async (app) => {
    const restore = stubUpstream(async () => ({ status: 503, body: 'Service Unavailable' }));
    try {
      const res = await app.inject({
        method: 'GET',
        url: '/release?session=65a00d22-e684-4a33-5fa2-707b8e64a84d',
      });
      assert.strictEqual(res.statusCode, 502);
      assert.strictEqual(res.json().error, 'upstream_error');
    } finally {
      restore();
    }
  }),
);

test('play: upstream 404 propagates as sidecar 404 not_found',
  withApp({}, async (app) => {
    const restore = stubUpstream(async () => ({ status: 404, body: '<html>Not Found</html>' }));
    try {
      const res = await app.inject({
        method: 'GET',
        url: '/play?animeSession=65a00d22-e684-4a33-5fa2-707b8e64a84d&episodeSession=7bf604bac56a6a9269bc0ce04083169abeaa4815c65e2a320e0ad185334c85e7',
      });
      assert.strictEqual(res.statusCode, 404);
      assert.strictEqual(res.json().error, 'not_found');
    } finally {
      restore();
    }
  }),
);

// ---------------------------------------------------------------------------
// /metrics smoke test — registry exposes the four required counter names
// ---------------------------------------------------------------------------

test('metrics: /metrics exposes the four sidecar counters',
  withApp({}, async (app) => {
    const res = await app.inject({ method: 'GET', url: '/metrics' });
    assert.strictEqual(res.statusCode, 200);
    const text = res.payload;
    for (const name of [
      'stealth_challenge_failures_total',
      'stealth_challenge_solves_total',
      'page_recycle_total',
      'upstream_403_total',
    ]) {
      assert.ok(text.includes(name), `metrics output missing ${name}`);
    }
  }),
);
