/*
 * services/animepahe-resolver/server.js
 *
 * ============================================================================
 *  SECURITY INVARIANT — DO NOT REMOVE
 * ============================================================================
 *  This sidecar is HARDCODED to https://animepahe.pw.
 *
 *  - Every route handler in this file constructs its upstream URL with the
 *    hardcoded base `https://animepahe.pw`. User-supplied query params
 *    (`q`, `session`, `animeSession`, `episodeSession`, `page`) are encoded
 *    via encodeURIComponent and substituted into a server-built path. The
 *    request hostname is NEVER taken from user input. (Threat T-27-01-01)
 *
 *  - upstream.js applies a defense-in-depth host-allowlist check before the
 *    in-page fetch — it rejects any URL whose hostname is not exactly
 *    `animepahe.pw`. (T-27-01-01, V4 ASVS host allowlist)
 *
 *  - Chromium runs with --no-sandbox under Docker (network-internal sidecar,
 *    no untrusted JS source). Adding a second upstream requires either
 *    (a) sandbox re-enablement with a Docker `cap_add: SYS_ADMIN` grant, or
 *    (b) explicit security review for the new domain. (T-27-01-02)
 *
 *  Plan-checker should block any PR that adds a second `page.goto` host or
 *  a second base URL constant in this file.
 *
 *  Refresh policy when DDoS-Guard defeats the stealth plugin:
 *    See services/animepahe-resolver/STEALTH-PINS.md
 * ============================================================================
 */

const Fastify = require('fastify');
const browser = require('./browser');
const upstream = require('./upstream');
const metrics = require('./metrics');

const UPSTREAM_BASE_URL = 'https://animepahe.pw';
const PORT = parseInt(process.env.PORT || '3000', 10);
const HOST = process.env.HOST || '0.0.0.0';
const HEALTHZ_PROBE_TIMEOUT_MS = 2_000;

// ---------------------------------------------------------------------------
// JSON schemas (Fastify route validation) — per CONTEXT.md D6 + T-27-01-01.
// ---------------------------------------------------------------------------

// Animepahe `session` tokens come in two flavors:
//   * animeSession (UUID v4 with hyphens): "65a00d22-e684-4a33-5fa2-707b8e64a84d"
//   * episodeSession (lowercase hex, 64 chars): "7bf604bac56a6a9269bc0ce04083169abeaa4815c65e2a320e0ad185334c85e7"
// Both are alphanumeric with hyphens — pattern allows letters, digits, and hyphens
// only. Length range covers both shapes (UUID = 36, hex episode = 64).
const SESSION_PATTERN = '^[A-Za-z0-9-]{16,128}$';

const searchSchema = {
  querystring: {
    type: 'object',
    required: ['q'],
    properties: {
      q: { type: 'string', minLength: 1, maxLength: 200 },
    },
    additionalProperties: false,
  },
};

const releaseSchema = {
  querystring: {
    type: 'object',
    required: ['session'],
    properties: {
      session: { type: 'string', pattern: SESSION_PATTERN },
      page: { type: 'integer', minimum: 1, maximum: 50, default: 1 },
    },
    additionalProperties: false,
  },
};

const playSchema = {
  querystring: {
    type: 'object',
    required: ['animeSession', 'episodeSession'],
    properties: {
      animeSession: { type: 'string', pattern: SESSION_PATTERN },
      episodeSession: { type: 'string', pattern: SESSION_PATTERN },
    },
    additionalProperties: false,
  },
};

// ---------------------------------------------------------------------------
// Fastify app factory — exported for unit tests (fastify.inject(...)).
// ---------------------------------------------------------------------------

function buildApp(opts) {
  opts = opts || {};
  const fastify = Fastify({
    logger: opts.logger !== undefined
      ? opts.logger
      : {
          level: process.env.LOG_LEVEL || 'info',
          redact: {
            paths: ['req.headers.cookie', 'res.headers["set-cookie"]'],
            censor: '[REDACTED]',
          },
        },
  });

  // ----- /healthz : Pattern 4 — two-layer probe (HTTP + browser-eval) -----
  fastify.get('/healthz', async (req, reply) => {
    const page = browser.getPage();
    const br = browser.getBrowser();
    if (!br || !page) {
      return reply.code(503).send({ browser: 'down', reason: 'not_initialized' });
    }
    let probeResult;
    try {
      probeResult = await Promise.race([
        page.evaluate(() => 1),
        new Promise((_, rej) =>
          setTimeout(() => rej(new Error('probe_timeout')), HEALTHZ_PROBE_TIMEOUT_MS),
        ),
      ]);
    } catch (e) {
      return reply.code(503).send({ browser: 'down', reason: e.message });
    }
    if (probeResult !== 1) {
      return reply.code(503).send({ browser: 'down', reason: 'probe_returned_unexpected' });
    }
    return reply.send({
      browser: 'up',
      lastChallengeSolveAt: upstream.getLastChallengeSolveAt(),
      pageCount: upstream.getRequestCount(),
    });
  });

  // ----- /search : passthrough of upstream m=search -----
  fastify.get('/search', { schema: searchSchema }, async (req, reply) => {
    const q = req.query.q;
    const url = `${UPSTREAM_BASE_URL}/api?m=search&q=${encodeURIComponent(q)}`;
    return handleJSON(req, reply, url);
  });

  // ----- /release : passthrough of upstream m=release with pagination -----
  fastify.get('/release', { schema: releaseSchema }, async (req, reply) => {
    const sess = req.query.session;
    const page = req.query.page || 1;
    const url =
      `${UPSTREAM_BASE_URL}/api?m=release&id=${encodeURIComponent(sess)}` +
      `&sort=episode_asc&page=${encodeURIComponent(String(page))}`;
    return handleJSON(req, reply, url);
  });

  // ----- /play : verbatim play-page HTML passthrough -----
  fastify.get('/play', { schema: playSchema }, async (req, reply) => {
    const anime = req.query.animeSession;
    const ep = req.query.episodeSession;
    const url =
      `${UPSTREAM_BASE_URL}/play/` +
      `${encodeURIComponent(anime)}/${encodeURIComponent(ep)}`;
    try {
      const { body } = await upstream.fetchWithRetry(url);
      reply.type('text/html; charset=utf-8').send(body);
    } catch (e) {
      sendResolverError(reply, e);
    }
  });

  // ----- /metrics : Prometheus scrape endpoint -----
  fastify.get('/metrics', async (req, reply) => {
    const reg = metrics.getRegister();
    reply.type(reg.contentType).send(await reg.metrics());
  });

  // ----- Custom error handler for validation 400s -----
  fastify.setErrorHandler((err, req, reply) => {
    if (err.validation) {
      return reply.code(400).send({
        error: 'bad_request',
        message: err.message,
        details: err.validation,
      });
    }
    req.log.error({ err }, 'unhandled request error');
    return reply.code(500).send({ error: 'internal_error' });
  });

  return fastify;
}

async function handleJSON(req, reply, url) {
  try {
    const { body } = await upstream.fetchWithRetry(url);
    let parsed;
    try {
      parsed = JSON.parse(body);
    } catch (e) {
      req.log.error({ url, err: e }, 'upstream returned non-JSON body');
      return reply.code(502).send({ error: 'upstream_bad_json' });
    }
    return reply.send(parsed);
  } catch (e) {
    return sendResolverError(reply, e);
  }
}

function sendResolverError(reply, e) {
  if (e && e.name === 'ResolverError') {
    const code = typeof e.status === 'number' ? e.status : 502;
    return reply.code(code).send({ error: e.code || 'resolver_error', message: e.message });
  }
  reply.log && reply.log.error && reply.log.error({ err: e }, 'unhandled resolver error');
  return reply.code(502).send({ error: 'resolver_error', message: (e && e.message) || 'unknown' });
}

// ---------------------------------------------------------------------------
// Entry point — Open Question Q4: warm browser BEFORE fastify.listen so the
// `/healthz` probe is meaningful from the first accepted connection.
// ---------------------------------------------------------------------------

async function start() {
  await browser.initBrowser();
  const app = buildApp();
  await app.listen({ port: PORT, host: HOST });
  app.log.info({ port: PORT }, 'animepahe-resolver listening');
  const shutdown = async (signal) => {
    app.log.info({ signal }, 'animepahe-resolver shutting down');
    try {
      await app.close();
    } finally {
      const br = browser.getBrowser();
      if (br) await br.close();
      process.exit(0);
    }
  };
  process.on('SIGTERM', () => shutdown('SIGTERM'));
  process.on('SIGINT', () => shutdown('SIGINT'));
}

if (require.main === module) {
  start().catch((err) => {
    // eslint-disable-next-line no-console
    console.error('fatal: animepahe-resolver failed to start', err);
    process.exit(1);
  });
}

module.exports = { buildApp };
