#!/bin/bash
# Redeploy services script
# Usage: ./deploy/scripts/redeploy.sh [service1] [service2] ...
# Example: ./deploy/scripts/redeploy.sh auth gateway
# Without arguments: redeploys all backend services

set -e

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/../.." && pwd)"
COMPOSE_FILE="$PROJECT_ROOT/docker/docker-compose.yml"

cd "$PROJECT_ROOT"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${GREEN}[INFO]${NC} $1"
}

log_warn() {
    echo -e "${YELLOW}[WARN]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# All backend services that can be redeployed
ALL_SERVICES="auth catalog gateway player rooms scheduler streaming"

NETWORK="animeenigma-network"

# ensure_network_alias guarantees the compose short-name network alias is registered for a
# just-recreated service. Recreating a single container can occasionally leave it WITHOUT its
# `<svc>` alias on animeenigma-network: the host-published port still works, but every sibling
# that connects via the short name `<svc>` gets Docker-DNS SERVFAIL -> 500s, and Grafana fires
# `Service Unreachable` even though the process is perfectly healthy (incident AUTO-392,
# 2026-06-05 — a `docker restart` does NOT fix it; only re-attaching the alias does).
ensure_network_alias() {
    local svc="$1"
    local container="animeenigma-$svc"   # fixed container_name convention (same as the restart self-heal below)
    local aliases dnsnames
    if ! docker inspect "$container" >/dev/null 2>&1; then
        log_warn "alias-check: container $container not found, skipping"
        return 0
    fi
    # NB: the dashed network name forces `index` (Go templates can't do `.animeenigma-network`).
    aliases=$(docker inspect "$container" --format '{{json (index .NetworkSettings.Networks "animeenigma-network").Aliases}}' 2>/dev/null)
    dnsnames=$(docker inspect "$container" --format '{{json (index .NetworkSettings.Networks "animeenigma-network").DNSNames}}' 2>/dev/null)
    # `"$svc"` (quoted) cannot false-match inside `"animeenigma-$svc"` — the preceding char is `-`, not `"`.
    if printf '%s%s' "$aliases" "$dnsnames" | grep -q "\"$svc\""; then
        log_info "alias-check: $svc resolves by short name OK"
        return 0
    fi
    log_warn "alias-check: $svc MISSING '$svc' network alias (aliases=$aliases dnsnames=$dnsnames) — re-attaching"
    docker network disconnect "$NETWORK" "$container" >/dev/null 2>&1 || true
    docker network connect --alias "$svc" "$NETWORK" "$container" >/dev/null 2>&1 || true
    sleep 1
    aliases=$(docker inspect "$container" --format '{{json (index .NetworkSettings.Networks "animeenigma-network").Aliases}}' 2>/dev/null)
    if printf '%s' "$aliases" | grep -q "\"$svc\""; then
        log_info "alias-check: $svc alias restored OK"
    else
        log_error "alias-check: FAILED to restore $svc alias (aliases=$aliases) — siblings may 500"
    fi
}

if [ $# -eq 0 ]; then
    SERVICES="$ALL_SERVICES"
    log_info "Redeploying all services: $SERVICES"
else
    SERVICES="$@"
    log_info "Redeploying services: $SERVICES"
fi

for SERVICE in $SERVICES; do
    log_info "Rebuilding $SERVICE..."
    docker compose -f "$COMPOSE_FILE" build "$SERVICE"

    # Recreate in a single atomic compose op. The previous stop -> rm -f -> up sequence left a
    # race window where the old network endpoint's teardown could clobber the new container's
    # service-name alias registration (incident AUTO-392). --force-recreate closes that gap.
    log_info "Recreating $SERVICE..."
    docker compose -f "$COMPOSE_FILE" up -d --force-recreate --no-deps "$SERVICE"

    # Wait a moment for the service to start
    sleep 2

    # Check if it's running
    if docker compose -f "$COMPOSE_FILE" ps "$SERVICE" | grep -q "Up"; then
        log_info "$SERVICE is running"
        # Guarantee the Docker-network short-name alias survived the recreate (see AUTO-392).
        ensure_network_alias "$SERVICE"
    else
        log_error "$SERVICE failed to start!"
        docker compose -f "$COMPOSE_FILE" logs --tail=50 "$SERVICE"
        exit 1
    fi
done

log_info "Deployment complete!"
log_info "Checking service health..."

# Port mapping for all services (not just redeployed ones)
declare -A SERVICE_PORTS=(
    [auth]=8080
    [catalog]=8081
    [streaming]=8082
    [player]=8083
    [rooms]=8084
    [scheduler]=8085
    [gateway]=8000
    [themes]=8086
)

# Health check redeployed services
for SERVICE in $SERVICES; do
    PORT="${SERVICE_PORTS[$SERVICE]:-}"
    if [ -n "$PORT" ]; then
        if curl -sf "http://localhost:$PORT/health" > /dev/null 2>&1; then
            log_info "$SERVICE:$PORT - healthy"
        else
            log_warn "$SERVICE:$PORT - health check failed (service may still be starting)"
        fi
    fi
done

# Verify port bindings for ALL running services (docker-proxy can die during container recreation)
log_info "Verifying port bindings for other services..."
RESTARTED=""
for SERVICE in "${!SERVICE_PORTS[@]}"; do
    PORT="${SERVICE_PORTS[$SERVICE]}"
    # Skip services we just redeployed (they're fine)
    if echo "$SERVICES" | grep -qw "$SERVICE"; then
        continue
    fi
    # Check if container is running but port is unreachable
    if docker compose -f "$COMPOSE_FILE" ps "$SERVICE" 2>/dev/null | grep -q "Up"; then
        if ! curl -sf --max-time 2 "http://localhost:$PORT/health" > /dev/null 2>&1; then
            log_warn "$SERVICE:$PORT - port binding lost, restarting..."
            docker restart "animeenigma-$SERVICE" > /dev/null 2>&1 || true
            sleep 2
            if curl -sf --max-time 2 "http://localhost:$PORT/health" > /dev/null 2>&1; then
                log_info "$SERVICE:$PORT - recovered"
            else
                log_error "$SERVICE:$PORT - still unreachable after restart"
            fi
            RESTARTED="$RESTARTED $SERVICE"
        fi
    fi
done

if [ -n "$RESTARTED" ]; then
    log_warn "Had to restart services with lost port bindings:$RESTARTED"
fi
