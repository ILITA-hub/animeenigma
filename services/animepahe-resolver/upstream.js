/*
 * services/animepahe-resolver/upstream.js
 *
 * Phase 27 / Task 1 SCAFFOLD — Task 2 expands this file with the full
 * Pattern 2 (403 retry) + Pattern 3 (page recycle at N=PAGE_RECYCLE_AT)
 * + V4 ASVS host-allowlist behavior. The exported surface here is the
 * stable contract that server.js consumes:
 *
 *   fetchWithRetry(url)              → { status, body }, throws ResolverError
 *   getLastChallengeSolveAt()        → ISO timestamp string or null
 *   getRequestCount()                → integer
 *   resetForTests()                  → test hook (reset counters + counters)
 *
 * Task 1 ships a thin host-allowlist + single-attempt upstream call to keep
 * server.js loadable; Task 2 adds 403 retry, page recycle, and metric wires.
 */

const { URL } = require('url');
const browserMod = require('./browser');

const ALLOWED_HOST = 'animepahe.pw';

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

/**
 * V4 ASVS host-allowlist (T-27-01-01). Throws ResolverError(400, 'host_not_allowed')
 * for any URL whose hostname is not exactly `animepahe.pw`. server.js builds the URL
 * from the hardcoded base, so this is defense-in-depth — any non-allowlisted host
 * reaching this function means a bug in route construction, NOT user-supplied input.
 */
function assertAllowedHost(urlStr) {
  let parsed;
  try {
    parsed = new URL(urlStr);
  } catch (e) {
    throw new ResolverError(400, 'host_not_allowed', 'invalid URL: ' + e.message);
  }
  if (parsed.hostname !== ALLOWED_HOST) {
    throw new ResolverError(400, 'host_not_allowed',
      `host '${parsed.hostname}' is not in the allowlist (expected '${ALLOWED_HOST}')`);
  }
}

/**
 * Phase 27 / Task 1 SCAFFOLD — single in-page fetch with host-allowlist guard.
 * Task 2 wraps this in Pattern 2 (403 → re-navigate → retry) + Pattern 3
 * (page recycle on every Nth call).
 */
async function fetchWithRetry(url) {
  assertAllowedHost(url);
  const page = browserMod.getPage();
  if (!page) {
    throw new ResolverError(503, 'browser_down', 'warm page is not initialized');
  }
  requestCount += 1;
  const result = await page.evaluate(async (u) => {
    const r = await fetch(u, { credentials: 'include' });
    return { status: r.status, body: await r.text() };
  }, url);
  if (result.status === 403) {
    throw new ResolverError(502, 'stealth_challenge_failed',
      'upstream returned 403 (DDoS-Guard) — retry/recycle wiring lands in Task 2');
  }
  return result;
}

function getLastChallengeSolveAt() {
  return lastChallengeSolveAt;
}

function getRequestCount() {
  return requestCount;
}

/**
 * Test hook (Node test-runner). Resets module-scoped counters so each
 * test starts from a clean slate.
 */
function resetForTests() {
  requestCount = 0;
  lastChallengeSolveAt = null;
}

module.exports = {
  fetchWithRetry,
  assertAllowedHost,
  getLastChallengeSolveAt,
  getRequestCount,
  resetForTests,
  ResolverError,
  ALLOWED_HOST,
};
