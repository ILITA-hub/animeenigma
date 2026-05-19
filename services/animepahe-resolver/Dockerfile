# Source: synthesis of https://pptr.dev/guides/docker + Phase 27 RESEARCH Pitfall 8
# services/animepahe-resolver/Dockerfile
#
# Base image: official puppeteer image (Chrome for Testing pre-bundled, dbus + fonts + userns).
# Build context: project root (`docker build -t animepahe-resolver:dev \
#   -f services/animepahe-resolver/Dockerfile .` from /data/animeenigma/).
FROM ghcr.io/puppeteer/puppeteer:24

# PUPPETEER_SKIP_DOWNLOAD prevents the puppeteer package from re-downloading
# Chrome for Testing at install time — the base image already ships one in
# /home/pptruser/.cache/puppeteer/. puppeteer.launch() locates that binary
# automatically via the cache directory (PUPPETEER_CACHE_DIR is preset by the
# base image), so we deliberately do NOT pin PUPPETEER_EXECUTABLE_PATH.
ENV PUPPETEER_SKIP_DOWNLOAD=true \
    NODE_ENV=production \
    PORT=3000

WORKDIR /app

# Lockfile-first copy for layer caching.
COPY services/animepahe-resolver/package.json services/animepahe-resolver/package-lock.json ./

# npm ci (NOT npm install) — fails fast on stale lockfile; this IS the pin enforcement
# documented in STEALTH-PINS.md.
RUN npm ci --omit=dev

# JS sources only; tests + fixtures live in __fixtures__/ and *.test.js, none of which
# the production image needs.
COPY services/animepahe-resolver/server.js services/animepahe-resolver/browser.js \
     services/animepahe-resolver/upstream.js services/animepahe-resolver/metrics.js ./

EXPOSE 3000

CMD ["node", "server.js"]
