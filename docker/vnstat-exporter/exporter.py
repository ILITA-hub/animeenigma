#!/usr/bin/env python3
"""Minimal vnstat -> Prometheus exporter (stdlib only).

Reads the host vnstat SQLite database (mounted read-only) and exposes
per-interface bandwidth as Prometheus metrics on :9100/metrics.

Design notes:
  * The DB is COPIED to a temp dir before every read and queried via
    `vnstat --dbdir <tmp> --json`. Copying decouples us entirely from the
    live host vnstatd (no SQLite lock contention, no need for a writable
    mount) and `--json` is a stable cross-version contract (jsonversion 2,
    all traffic values in BYTES).
  * The image is built FROM ubuntu:24.04 so the in-container vnstat matches
    the host's 2.12 exactly — no DB-format skew.
"""
import json
import os
import shutil
import subprocess
import tempfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer

DB_SRC = os.environ.get("VNSTAT_DB_SRC", "/host-vnstat/vnstat.db")
PORT = int(os.environ.get("LISTEN_PORT", "9100"))
# Optional comma-separated allowlist; empty = export every interface in the DB.
IFACE_FILTER = [s.strip() for s in os.environ.get("INTERFACES", "").split(",") if s.strip()]


def _read_vnstat():
    """Copy the DB to a temp dir and return parsed `vnstat --json` output."""
    tmp = tempfile.mkdtemp(prefix="vnstat-")
    try:
        shutil.copy(DB_SRC, os.path.join(tmp, "vnstat.db"))
        out = subprocess.check_output(
            ["vnstat", "--dbdir", tmp, "--json"],
            stderr=subprocess.STDOUT,
            timeout=10,
        )
        return json.loads(out)
    finally:
        shutil.rmtree(tmp, ignore_errors=True)


def _last(arr):
    """Most recent entry of a vnstat traffic period array (or None)."""
    return arr[-1] if arr else None


def _render():
    lines = []

    def metric(name, mtype, help_text):
        lines.append(f"# HELP {name} {help_text}")
        lines.append(f"# TYPE {name} {mtype}")

    def sample(name, iface, value):
        lines.append(f'{name}{{interface="{iface}"}} {int(value)}')

    try:
        data = _read_vnstat()
        ifaces = data.get("interfaces", [])
    except Exception as exc:  # noqa: BLE001 - surface failure as a scrape flag
        lines.append("# HELP vnstat_up 1 if the last vnstat read succeeded, 0 otherwise")
        lines.append("# TYPE vnstat_up gauge")
        lines.append("vnstat_up 0")
        lines.append("# HELP vnstat_scrape_error_info Last scrape error (label only)")
        lines.append("# TYPE vnstat_scrape_error_info gauge")
        msg = str(exc).replace("\\", " ").replace('"', "'").replace("\n", " ")[:200]
        lines.append(f'vnstat_scrape_error_info{{error="{msg}"}} 1')
        return ("\n".join(lines) + "\n").encode()

    metric("vnstat_interface_rx_bytes_total", "counter",
           "Total bytes received since vnstat tracking began")
    metric("vnstat_interface_tx_bytes_total", "counter",
           "Total bytes transmitted since vnstat tracking began")
    metric("vnstat_month_rx_bytes", "gauge", "Bytes received in the current month")
    metric("vnstat_month_tx_bytes", "gauge", "Bytes transmitted in the current month")
    metric("vnstat_day_rx_bytes", "gauge", "Bytes received today")
    metric("vnstat_day_tx_bytes", "gauge", "Bytes transmitted today")
    metric("vnstat_fiveminute_rx_bytes", "gauge",
           "Bytes received in the most recent 5-minute sample")
    metric("vnstat_fiveminute_tx_bytes", "gauge",
           "Bytes transmitted in the most recent 5-minute sample")

    for iface in ifaces:
        name = iface.get("name", "")
        if IFACE_FILTER and name not in IFACE_FILTER:
            continue
        t = iface.get("traffic", {})
        total = t.get("total", {})
        sample("vnstat_interface_rx_bytes_total", name, total.get("rx", 0))
        sample("vnstat_interface_tx_bytes_total", name, total.get("tx", 0))

        month = _last(t.get("month", [])) or {}
        sample("vnstat_month_rx_bytes", name, month.get("rx", 0))
        sample("vnstat_month_tx_bytes", name, month.get("tx", 0))

        day = _last(t.get("day", [])) or {}
        sample("vnstat_day_rx_bytes", name, day.get("rx", 0))
        sample("vnstat_day_tx_bytes", name, day.get("tx", 0))

        fm = _last(t.get("fiveminute", [])) or {}
        sample("vnstat_fiveminute_rx_bytes", name, fm.get("rx", 0))
        sample("vnstat_fiveminute_tx_bytes", name, fm.get("tx", 0))

    lines.append("# HELP vnstat_up 1 if the last vnstat read succeeded, 0 otherwise")
    lines.append("# TYPE vnstat_up gauge")
    lines.append("vnstat_up 1")
    return ("\n".join(lines) + "\n").encode()


class Handler(BaseHTTPRequestHandler):
    def do_GET(self):  # noqa: N802 - http.server contract
        if self.path.rstrip("/") in ("/metrics", ""):
            body = _render()
            self.send_response(200)
            self.send_header("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
            self.send_header("Content-Length", str(len(body)))
            self.end_headers()
            self.wfile.write(body)
        elif self.path == "/healthz":
            self.send_response(200)
            self.end_headers()
            self.wfile.write(b"ok\n")
        else:
            self.send_response(404)
            self.end_headers()

    def log_message(self, *args):  # silence per-request stderr noise
        pass


if __name__ == "__main__":
    ThreadingHTTPServer(("0.0.0.0", PORT), Handler).serve_forever()
