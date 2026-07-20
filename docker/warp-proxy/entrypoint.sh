#!/usr/bin/env bash
# Bring up Cloudflare WARP in PROXY mode and republish its loopback SOCKS5 on
# 0.0.0.0 so sibling containers can use it. Registration persists in the mounted
# /var/lib/cloudflare-warp volume, so re-registration is skipped on restart.
set -euo pipefail

PROXY_PORT="${WARP_PROXY_PORT:-40000}"   # warp-cli proxy binds 127.0.0.1:{port}
LISTEN_PORT="${WARP_LISTEN_PORT:-1080}"  # socat republishes on 0.0.0.0:{port}

log() { echo "[warp-proxy] $*"; }

log "starting warp-svc daemon..."
warp-svc >/var/log/warp-svc.log 2>&1 &

# Wait for the daemon IPC socket to accept warp-cli commands.
for _ in $(seq 1 30); do
    if warp-cli --accept-tos status >/dev/null 2>&1; then break; fi
    sleep 1
done

# Register a free WARP account once (persisted in the volume). A pre-existing
# registration makes `registration new` error "Old registration is still
# around" — non-fatal, we just reuse it.
if warp-cli --accept-tos registration new 2>&1 | grep -qi "still around"; then
    log "reusing existing WARP registration"
else
    log "registered new WARP account"
fi

# Scoped proxy mode — NEVER default/full-tunnel (would hijack all egress).
warp-cli --accept-tos mode proxy
warp-cli --accept-tos proxy port "${PROXY_PORT}"
warp-cli --accept-tos connect || true

# Wait for the tunnel to come up (best-effort; socat starts regardless so the
# healthcheck, not a boot race, decides readiness).
for _ in $(seq 1 30); do
    if warp-cli --accept-tos status 2>/dev/null | grep -q "Connected"; then
        log "WARP connected"; break
    fi
    sleep 1
done

log "relaying 0.0.0.0:${LISTEN_PORT} -> 127.0.0.1:${PROXY_PORT} (SOCKS5)"
exec socat "TCP-LISTEN:${LISTEN_PORT},fork,reuseaddr" "TCP:127.0.0.1:${PROXY_PORT}"
