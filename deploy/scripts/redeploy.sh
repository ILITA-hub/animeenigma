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

if [ $# -eq 0 ]; then
    SERVICES="$ALL_SERVICES"
    log_info "Redeploying all services: $SERVICES"
else
    SERVICES="$@"
    log_info "Redeploying services: $SERVICES"
fi

for SERVICE in $SERVICES; do
    log_info "Rebuilding $SERVICE..."
    docker-compose -f "$COMPOSE_FILE" build "$SERVICE"

    log_info "Stopping $SERVICE..."
    docker-compose -f "$COMPOSE_FILE" stop "$SERVICE" || true

    log_info "Removing $SERVICE container..."
    docker-compose -f "$COMPOSE_FILE" rm -f "$SERVICE" || true

    log_info "Starting $SERVICE..."
    docker-compose -f "$COMPOSE_FILE" up -d --no-deps "$SERVICE"

    # Wait a moment for the service to start
    sleep 2

    # Check if it's running
    if docker-compose -f "$COMPOSE_FILE" ps "$SERVICE" | grep -q "Up"; then
        log_info "$SERVICE is running"
    else
        log_error "$SERVICE failed to start!"
        docker-compose -f "$COMPOSE_FILE" logs --tail=50 "$SERVICE"
        exit 1
    fi
done

log_info "Deployment complete!"
log_info "Checking service health..."

# Quick health check
for SERVICE in $SERVICES; do
    case $SERVICE in
        auth)     PORT=8080 ;;
        catalog)  PORT=8081 ;;
        streaming) PORT=8082 ;;
        player)   PORT=8083 ;;
        rooms)    PORT=8084 ;;
        scheduler) PORT=8085 ;;
        gateway)  PORT=8000 ;;
        *)        PORT="" ;;
    esac

    if [ -n "$PORT" ]; then
        if curl -sf "http://localhost:$PORT/health" > /dev/null 2>&1; then
            log_info "$SERVICE:$PORT - healthy"
        else
            log_warn "$SERVICE:$PORT - health check failed (service may still be starting)"
        fi
    fi
done
