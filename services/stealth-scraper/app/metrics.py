"""Prometheus metrics for the stealth-scraper sidecar.

Isolated import of ``prometheus_client`` (not pulled in by the pure-logic unit
tests, which never import this module).
"""

from __future__ import annotations

from prometheus_client import Counter, Gauge, Histogram

RESOLVE_TOTAL = Counter(
    "stealth_resolve_total",
    "Resolve attempts by provider and result.",
    ["provider", "result"],  # result: ok|not_found|challenge|error|exhausted
)

ACTIVE_SESSIONS = Gauge(
    "stealth_active_sessions",
    "Retained browser stream sessions (each pins one pool profile).",
)

POOL_EXHAUSTED_TOTAL = Counter(
    "stealth_pool_exhausted_total",
    "Resolves rejected because every browser profile was leased (pool full).",
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

POOL_FREE = Gauge(
    "stealth_pool_free",
    "Browser profiles currently free (status=healthy and not leased).",
)

POOL_CRASHED = Gauge(
    "stealth_pool_crashed",
    "Browser profiles currently marked crashed (awaiting resurrection).",
)

SLOT_RESURRECT_TOTAL = Counter(
    "stealth_slot_resurrect_total",
    "Crashed-slot resurrection attempts in the reaper, by result.",
    ["result"],  # ok|fail|retired
)

STEALTH_PROXY_FETCH_TOTAL = Counter(
    "stealth_proxy_fetch_total",
    "In-page restream fetches (proxy_fetch), by result.",
    ["result"],  # ok|timeout|too_large|host_denied
)

STEALTH_PROXY_FETCH_DURATION = Histogram(
    "stealth_proxy_fetch_duration_seconds",
    "Wall-clock duration of a restream in-page fetch, by result.",
    ["result"],  # ok|timeout|too_large|host_denied
    buckets=(0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10, 20, 30),
)

STEALTH_PROXY_FETCH_BYTES = Histogram(
    "stealth_proxy_fetch_bytes",
    "Body size (bytes) returned by a successful restream in-page fetch.",
    buckets=(
        1024,
        16 * 1024,
        128 * 1024,
        512 * 1024,
        2 * 1024 * 1024,
        8 * 1024 * 1024,
        32 * 1024 * 1024,
    ),
)

RAM_BYTES = Gauge(
    "stealth_ram_bytes",
    "Combined RSS of the Camoufox/Firefox process tree, last sample.",
)

ADMISSION_TOTAL = Counter(
    "stealth_admission_total",
    "Admission-controller actions by type.",
    ["action"],  # soft_evict|hard_refuse|hard_evict
)

USER_QUOTA_REJECTED_TOTAL = Counter(
    "stealth_user_quota_rejected_total",
    "Resolves/fetches rejected because the user_key held >= quota sessions.",
)
