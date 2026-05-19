/*
 * services/animepahe-resolver/metrics.js
 *
 * Phase 27 / Task 1 SCAFFOLD — prom-client registry stub. Task 2 wires the
 * real counter increments inside upstream.js. The counter names + label
 * shapes here are the FINAL contract; Task 2 only ADDS .inc() calls, it
 * does NOT change the metric surface.
 *
 * Counter surface (T-27-01-04 — never emit cookie values as label values):
 *   stealth_challenge_failures_total   – Counter (no labels)
 *   stealth_challenge_solves_total     – Counter (no labels)
 *   page_recycle_total                 – Counter (no labels)
 *   upstream_403_total{stage}          – Counter with one label `stage`
 *                                          stage ∈ {"first","second"}
 *
 * Plus prom-client default process metrics (CPU, memory, GC, event loop lag).
 */

const promClient = require('prom-client');

const register = new promClient.Registry();
register.setDefaultLabels({ service: 'animepahe-resolver' });
promClient.collectDefaultMetrics({ register });

const stealthChallengeFailuresTotal = new promClient.Counter({
  name: 'stealth_challenge_failures_total',
  help: 'Number of times the DDoS-Guard challenge was un-solvable after retry (second 403).',
  registers: [register],
});

const stealthChallengeSolvesTotal = new promClient.Counter({
  name: 'stealth_challenge_solves_total',
  help: 'Number of times the DDoS-Guard challenge was solved by re-navigating the warm page on a first-attempt 403.',
  registers: [register],
});

const pageRecycleTotal = new promClient.Counter({
  name: 'page_recycle_total',
  help: 'Number of times the warm page was recycled (closed + reopened) after N=PAGE_RECYCLE_AT requests.',
  registers: [register],
});

const upstream403Total = new promClient.Counter({
  name: 'upstream_403_total',
  help: 'Number of HTTP 403 responses observed from animepahe.pw, labelled by which attempt produced them.',
  labelNames: ['stage'],
  registers: [register],
});

function getRegister() {
  return register;
}

/**
 * Test hook: reset all counters. Lets unit tests (Task 3) assert counter deltas
 * across independent test cases without leaking state.
 */
function _resetForTests() {
  stealthChallengeFailuresTotal.reset();
  stealthChallengeSolvesTotal.reset();
  pageRecycleTotal.reset();
  upstream403Total.reset();
}

module.exports = {
  getRegister,
  stealthChallengeFailuresTotal,
  stealthChallengeSolvesTotal,
  pageRecycleTotal,
  upstream403Total,
  _resetForTests,
};
