#!/bin/bash
# k8s-compose-parity.sh — fail if docker-compose and the kustomize tree drift apart.
#
# WHY: The project has TWO deploy targets: docker compose (the live host) and
# deploy/kustomize/ (k8s). The 2026-06-21 audit found the kustomize tree had
# silently fallen ~15 services behind compose — it deployed a version of the
# platform that no longer existed. This check compares the set of first-party
# application services in docker/docker-compose.yml (services with a `build:`
# key, i.e. built from this repo — infra images like postgres/redis don't
# count) against the Deployment manifests in deploy/kustomize/base/services/,
# so a new service can't ship to compose without either a k8s manifest or an
# explicit exclusion here.
#
# Usage:   deploy/scripts/k8s-compose-parity.sh
# Exit:    0 = in sync ("k8s-compose-parity: OK"), 1 = drift (diff printed)
set -euo pipefail

REPO_ROOT="$(git rev-parse --show-toplevel)"
cd "$REPO_ROOT"

COMPOSE_FILE="docker/docker-compose.yml"
SERVICES_DIR="deploy/kustomize/base/services"

# Host-only services that intentionally have NO k8s manifest. Every entry
# needs a reason — this list is the documented escape hatch, not a dumping
# ground.
EXCLUSIONS=(
    backup            # host cron/rsync sidecar bound to host volumes; backups are a host concern, not a k8s workload
    vnstat-exporter   # scrapes the host's physical NICs via /proc; meaningless inside a pod network namespace
)

[ -f "$COMPOSE_FILE" ] || { echo "k8s-compose-parity: FAIL — $COMPOSE_FILE not found" >&2; exit 1; }
[ -d "$SERVICES_DIR" ] || { echo "k8s-compose-parity: FAIL — $SERVICES_DIR not found" >&2; exit 1; }

# First-party services = compose services carrying a `build:` key.
compose_services="$(python3 - "$COMPOSE_FILE" <<'PY'
import sys, yaml

with open(sys.argv[1]) as f:
    doc = yaml.safe_load(f)

for name, svc in sorted((doc.get("services") or {}).items()):
    if isinstance(svc, dict) and "build" in svc:
        print(name)
PY
)"

# Expected = compose build services minus the documented exclusions.
expected="$(
    for svc in $compose_services; do
        skip=""
        for excl in "${EXCLUSIONS[@]}"; do
            [ "$svc" = "$excl" ] && skip=1 && break
        done
        [ -z "$skip" ] && echo "$svc"
    done | sort
)"

# Actual = basenames of the manifests in base/services/ (kustomization.yaml,
# if one ever appears there, is plumbing — not a service).
actual="$(
    find "$SERVICES_DIR" -maxdepth 1 \( -name '*.yaml' -o -name '*.yml' \) -printf '%f\n' \
        | sed -E 's/\.(yaml|yml)$//' \
        | grep -vx 'kustomization' \
        | sort
)"

missing="$(comm -23 <(echo "$expected") <(echo "$actual"))"
extra="$(comm -13 <(echo "$expected") <(echo "$actual"))"

if [ -n "$missing" ] || [ -n "$extra" ]; then
    echo "k8s-compose-parity: FAIL — compose and kustomize service sets differ" >&2
    if [ -n "$missing" ]; then
        echo "" >&2
        echo "  In $COMPOSE_FILE (build:) but MISSING a manifest in $SERVICES_DIR/:" >&2
        echo "$missing" | sed 's/^/    - /' >&2
    fi
    if [ -n "$extra" ]; then
        echo "" >&2
        echo "  Manifest in $SERVICES_DIR/ but NO build: service in $COMPOSE_FILE:" >&2
        echo "$extra" | sed 's/^/    - /' >&2
    fi
    echo "" >&2
    echo "Fix: add the missing manifest / remove the stale one, or (host-only" >&2
    echo "services only) add it to the EXCLUSIONS list in $0 with a reason." >&2
    exit 1
fi

echo "k8s-compose-parity: OK ($(echo "$expected" | grep -c .) services in sync, ${#EXCLUSIONS[@]} documented exclusions)"
