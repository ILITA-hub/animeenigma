#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mapfile -t MODULES < <("$ROOT/scripts/go-modules.sh")

echo "=== Testing ${#MODULES[@]} Go modules ==="

for module in "${MODULES[@]}"; do
  relative=${module#"$ROOT/"}
  echo "Testing $relative..."

  if [[ "$relative" == "worker" ]]; then
    (cd "$module" && GOWORK=off go test ./... "$@")
  else
    (cd "$module" && go test ./... "$@")
  fi
done

echo "=== All Go modules passed ==="
