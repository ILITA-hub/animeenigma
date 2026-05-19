/*
 * services/animepahe-resolver/upstream.js
 *
 * In-page fetch wrapper for the animepahe-resolver sidecar. Implements:
 *
 *   - Pattern 2 (RESEARCH §): 403-on-first-attempt → re-navigate the warm
 *     page to https://animepahe.pw/ to refresh DDoS-Guard cookies → retry
 *     ONCE. Second 403 → throw ResolverError(502, 'stealth_challenge_failed').
 *
 *   - Pattern 3 (RESEARCH §): after every PAGE_RECYCLE_AT (env-overridable,
 *     default 100) requests, call browser.recyclePage() to close the warm
 *     tab and open a fresh one on the same default BrowserContext (Pitfall 6
 *     — cookies persist; Pitfall 4 — overlap order avoids 503 window).
 *
 *   - V4 ASVS host-allowlist (T-27-01-01): every URL must have hostname
 *     exactly `animepahe.pw`; non-matching → ResolverError(400, 'host_not_allowed').
 *     Defense-in-depth — route handlers in server.js already build URLs from
 *     the hardcoded base, but this guard prevents future regressions.
 *
 *   - Metric increments (T-27-01-04 cookie redaction is owned by metrics.js
 *     label shape — this module ONLY increments counters, never labels with
 *     cookie values):
 *
 *       upstream_403_total{stage="first"}   — every first-attempt 403
 *       upstream_403_total{stage="second"}  — every second-attempt 403
 *       stealth_challenge_solves_total      — every successful retry-after-403
 *       stealth_challenge_failures_total    — every second 403 (un-solvable)
 *       page_recycle_total                  — every Pattern-3 recycle
 */

const { URL } = require('url');
const browserMod = require('./browser');
const metrics = require('./metrics');

const ALLOWED_HOST = 'animepahe.pw';
const DEFAULT_PAGE_RECYCLE_AT = 100;

class ResolverError extends Error {
  constructor(status, code, message) {
    super(message || code || 'resolver_error');
    this.name = 'ResolverError';
    this.status = status;
    this.code = code;
  }
}

let requestCount = 0;
let lastChallengeSolveAt = null;

function pageRecycleAt() {
  const raw = process.env.PAGE_RECYCLE_AT;
  if (!raw) return DEFAULT_PAGE_RECYCLE_AT;
  const n = parseInt(raw, 10);
  if (!Number.isFinite(n) || n <= 0) return DEFAULT_PAGE_RECYCLE_AT;
  return n;
}

function assertAllowedHost(urlStr) {
  let parsed;
  try {
    parsed = new URL(urlStr);
  } catch (e) {
    throw new ResolverError(400, 'host_not_allowed', 'invalid URL: ' + e.message);
  }
  if (parsed.hostname !== ALLOWED_HOST) {
    throw new ResolverError(
      400,
      'host_not_allowed',
      `host '${parsed.hostname}' is not in the allowlist (expected '${ALLOWED_HOST}')`,
    );
  }
}

/**
 * Single in-page fetch through the warm page. Returns { status, body } or throws
 * ResolverError(503, 'browser_down') if no warm page exists.
 */
async function fetchOnce(url) {
  const page = browserMod.getPage();
  if (!page) {
    throw new ResolverError(503, 'browser_down', 'warm page is not initialized');
  }
  return await page.evaluate(async (u) => {
    const r = await fetch(u, { credentials: 'include' });
    return { status: r.status, body: await r.text() };
  }, url);
}

/**
 * Pattern 2 — 403 retry with re-navigation. Returns { status, body } from upstream.
 * Increments metrics on each path:
 *
 *   first 403  → upstream_403_total{stage:"first"} + refreshChallenge() + retry
 *   retry OK   → stealth_challenge_solves_total++ ; lastChallengeSolveAt = now
 *   second 403 → upstream_403_total{stage:"second"} + stealth_challenge_failures_total++
 *                + throw ResolverError(502, 'stealth_challenge_failed')
 *
 * Also drives Pattern 3 — maybeRecycle() runs AFTER every successful fetch.
 */
async function fetchWithRetry(url) {
  assertAllowedHost(url);
  if (!browserMod.getPage()) {
    throw new ResolverError(503, 'browser_down', 'warm page is not initialized');
  }
  let result;
  try {
    result = await fetchOnce(url);
  } catch (e) {
    if (e instanceof ResolverError) throw e;
    throw new ResolverError(502, 'upstream_evaluate_failed', e.message);
  }
  if (result.status === 403) {
    metrics.upstream403Total.labels('first').inc();
    try {
      await browserMod.refreshChallenge();
    } catch (e) {
      metrics.stealthChallengeFailuresTotal.inc();
      throw new ResolverError(
        502,
        'stealth_challenge_failed',
        'refreshChallenge failed: ' + e.message,
      );
    }
    try {
      result = await fetchOnce(url);
    } catch (e) {
      if (e instanceof ResolverError) throw e;
      throw new ResolverError(502, 'upstream_evaluate_failed', e.message);
    }
    if (result.status === 403) {
      metrics.upstream403Total.labels('second').inc();
      metrics.stealthChallengeFailuresTotal.inc();
      throw new ResolverError(
        502,
        'stealth_challenge_failed',
        'upstream returned 403 twice; stealth plugin may need refresh — see STEALTH-PINS.md',
      );
    }
    metrics.stealthChallengeSolvesTotal.inc();
    lastChallengeSolveAt = new Date().toISOString();
  }
  requestCount += 1;
  try {
    await maybeRecycle();
  } catch (e) {
    // A recycle failure should NOT fail the just-completed fetch — the warm
    // page is still usable; we log and continue. The next request that hits
    // the recycle window will retry the recycle.
    // eslint-disable-next-line no-console
    console.error('animepahe-resolver: maybeRecycle failed', e);
  }
  return result;
}

/**
 * Pattern 3 — page recycle at every PAGE_RECYCLE_AT-th request. Counter is
 * INCREMENTED then compared so the first recycle fires at requestCount = N,
 * not 0. Recycle uses the default overlap strategy (new page warmed before
 * old close) per Pitfall 4; close-first is opt-in via opts argument and
 * documented in STEALTH-PINS.md if the D5 100-req soak peaks > 450 MB RSS.
 */
async function maybeRecycle() {
  const at = pageRecycleAt();
  if (requestCount === 0 || requestCount % at !== 0) return;
  await browserMod.recyclePage();
  metrics.pageRecycleTotal.inc();
}

function getLastChallengeSolveAt() {
  return lastChallengeSolveAt;
}

function getRequestCount() {
  return requestCount;
}

function resetForTests() {
  requestCount = 0;
  lastChallengeSolveAt = null;
  metrics._resetForTests();
}

module.exports = {
  fetchWithRetry,
  assertAllowedHost,
  getLastChallengeSolveAt,
  getRequestCount,
  resetForTests,
  pageRecycleAt,
  ResolverError,
  ALLOWED_HOST,
  // Test hook — lets upstream.test.js poke maybeRecycle directly.
  maybeRecycle,
};
