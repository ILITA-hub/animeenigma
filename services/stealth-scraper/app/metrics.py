"""Prometheus metrics for the stealth-scraper sidecar.

Isolated import of ``prometheus_client`` (not pulled in by the pure-logic unit
tests, which never import this module).
"""

from __future__ import annotations

from prometheus_client import Counter, Gauge, Histogram

RESOLVE_TOTAL = Counter(
    "stealth_resolve_total",
    "Resolve attempts by provider and result.",
    ["provider", "result"],  # result: ok|not_found|challenge|error
)

RESOLVE_DURATION = Histogram(
    "stealth_resolve_duration_seconds",
    "Wall-clock duration of a resolve, by provider.",
    ["provider"],
    buckets=(0.5, 1, 2, 5, 10, 20, 30, 45, 60, 90),
)

CHALLENGE_TOTAL = Counter(
    "stealth_challenge_total",
    "Bot/anti-DDoS challenges encountered, by host and kind.",
    ["host", "kind"],  # kind: navigation|getsources|cdn|unknown
)

PROXY_BLOCK_TOTAL = Counter(
    "stealth_proxy_block_total",
    "Times a tunnel was benched after a challenge, by proxy id.",
    ["proxy_id"],
)

PROXY_SELECT_TOTAL = Counter(
    "stealth_proxy_select_total",
    "Tunnel selections, by proxy id and type.",
    ["proxy_id", "type"],
)

BROWSER_POOL_SIZE = Gauge(
    "stealth_browser_pool_size",
    "Number of live browser profiles in the pool.",
)

BROWSER_RELAUNCH_TOTAL = Counter(
    "stealth_browser_relaunch_total",
    "Browser (re)launches, by reason.",
    ["reason"],  # cold|recycle|crash|rotate
)
