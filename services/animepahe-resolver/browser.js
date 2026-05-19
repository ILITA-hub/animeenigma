/*
 * services/animepahe-resolver/browser.js
 *
 * Singleton Chromium launcher + warm-page management for the animepahe-resolver
 * sidecar. Implements Phase 27 RESEARCH Pattern 1 (warm browser, single page on the
 * DEFAULT BrowserContext so DDoS-Guard cookies survive page recycle — Pitfall 6).
 *
 * Surface:
 *   - initBrowser()          – idempotent one-time launch + warm-page.goto(animepahe.pw)
 *   - getBrowser()           – Browser handle (or null pre-init)
 *   - getPage()              – current warm Page (or null pre-init)
 *   - recyclePage()          – Pattern 3 + Pitfall 4: open new tab, warm it, close old
 *                              (overlap strategy by default; switch to close-first if
 *                              the D5 100-request soak peaks > 450 MB RSS).
 *
 * Browser context invariant: page is created via `browser.newPage()` which uses the
 * DEFAULT BrowserContext (NOT createIncognitoBrowserContext) so the DDoS-Guard cookie
 * jar persists across recycles (Pitfall 6).
 */

const puppeteer = require('puppeteer-extra');
const StealthPlugin = require('puppeteer-extra-plugin-stealth');

puppeteer.use(StealthPlugin());

// Chrome 130-compatible UA — keep this loosely aligned with the puppeteer:24 base
// image's bundled Chrome version so navigator.userAgent matches the actual binary.
const USER_AGENT =
  'Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/130.0.0.0 Safari/537.36';

const UPSTREAM_BASE_URL = 'https://animepahe.pw';
const WARMUP_GOTO_TIMEOUT_MS = 30_000;

let browser = null;
let page = null;
let initialized = false;
let initPromise = null;

const LAUNCH_ARGS = Object.freeze([
  // Container sandbox — see Pitfall 1 / T-27-01-02. We run on the internal docker
  // network only and the HARDCODED upstream is animepahe.pw; adding a second upstream
  // requires sandbox re-enablement or explicit security review.
  '--no-sandbox',
  '--disable-setuid-sandbox',
  // /dev/shm default is 64 MB; this is well below what Chromium wants for tabs.
  '--disable-dev-shm-usage',
  '--disable-gpu',
  // --single-process: lower memory ceiling (~80-150 MB savings); the D5 100-request
  // soak in Plan 27-01 Task 4 confirms steady-state RSS stays under the 500 MB
  // budget. If a future Chromium build segfaults under load, drop this flag and
  // re-run the soak.
  '--single-process',
  '--js-flags=--max-old-space-size=384',
  '--disable-extensions',
  '--disable-background-networking',
]);

/**
 * Launch Chromium once + warm the page against animepahe.pw to collect DDoS-Guard
 * cookies. Safe to call repeatedly — subsequent calls await the first init.
 */
async function initBrowser() {
  if (initialized) return;
  if (initPromise) return initPromise;
  initPromise = (async () => {
    let newBrowser;
    try {
      newBrowser = await puppeteer.launch({
        headless: 'new',
        // Honor PUPPETEER_EXECUTABLE_PATH if set (e.g. for local dev pointing at
        // /usr/bin/google-chrome-stable on a host that has one); otherwise let
        // puppeteer resolve via PUPPETEER_CACHE_DIR (set by the puppeteer:24 base
        // image to /home/pptruser/.cache/puppeteer).
        executablePath: process.env.PUPPETEER_EXECUTABLE_PATH || undefined,
        args: LAUNCH_ARGS.slice(),
      });
      const newPage = await newBrowser.newPage();
      await newPage.setUserAgent(USER_AGENT);
      await newPage.goto(UPSTREAM_BASE_URL + '/', {
        waitUntil: 'networkidle2',
        timeout: WARMUP_GOTO_TIMEOUT_MS,
      });
      // Assign to module-level singletons only after all init steps succeed
      // so a partial failure never leaves a leaked browser instance.
      browser = newBrowser;
      page = newPage;
      initialized = true;
    } catch (e) {
      if (newBrowser) {
        try { await newBrowser.close(); } catch (_) {}
      }
      throw e;
    }
  })();
  try {
    await initPromise;
  } finally {
    initPromise = null;
  }
}

function getBrowser() {
  return browser;
}

function getPage() {
  return page;
}

/**
 * Pattern 3 + Pitfall 4: open a fresh tab on the SAME default BrowserContext, warm
 * it, then close the old tab. Order is overlap-first (new ready before old closed)
 * to avoid the no-page window the close-first strategy creates. If the D5 100-req
 * soak in Plan 27-01 Task 4 peaks > 450 MB RSS, swap to close-first AND lower
 * PAGE_RECYCLE_AT default to 50 (documented in STEALTH-PINS.md).
 *
 * @param {{ closeFirst?: boolean }} [opts] – override the overlap order.
 */
async function recyclePage(opts) {
  if (!browser) throw new Error('recyclePage called before initBrowser');
  const closeFirst = !!(opts && opts.closeFirst);
  if (closeFirst) {
    const old = page;
    if (old) await old.close();
    page = await browser.newPage();
    await page.setUserAgent(USER_AGENT);
    await page.goto(UPSTREAM_BASE_URL + '/', {
      waitUntil: 'networkidle2',
      timeout: WARMUP_GOTO_TIMEOUT_MS,
    });
    return;
  }
  const old = page;
  const next = await browser.newPage();
  await next.setUserAgent(USER_AGENT);
  await next.goto(UPSTREAM_BASE_URL + '/', {
    waitUntil: 'networkidle2',
    timeout: WARMUP_GOTO_TIMEOUT_MS,
  });
  page = next;
  if (old) await old.close();
}

/**
 * Re-navigate the warm page to animepahe.pw to refresh DDoS-Guard cookies. Called
 * from upstream.js on a first-attempt 403 (Pattern 2).
 */
async function refreshChallenge() {
  if (!page) throw new Error('refreshChallenge called before initBrowser');
  await page.goto(UPSTREAM_BASE_URL + '/', {
    waitUntil: 'networkidle2',
    timeout: WARMUP_GOTO_TIMEOUT_MS,
  });
}

/**
 * Test hook (Node test-runner via node:test). Lets server.test.js + upstream.test.js
 * inject a fake browser/page object without spinning up a real Chromium.
 */
function _setTestDoubles(doubles) {
  if (!doubles) {
    browser = null;
    page = null;
    initialized = false;
    return;
  }
  browser = doubles.browser || null;
  page = doubles.page || null;
  initialized = true;
}

module.exports = {
  initBrowser,
  getBrowser,
  getPage,
  recyclePage,
  refreshChallenge,
  UPSTREAM_BASE_URL,
  _setTestDoubles,
};
