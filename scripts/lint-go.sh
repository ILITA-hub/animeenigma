#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mapfile -t MODULES < <("$ROOT/scripts/go-modules.sh")

echo "=== Linting ${#MODULES[@]} Go modules ==="

for module in "${MODULES[@]}"; do
  relative=${module#"$ROOT/"}
  echo "Linting $relative..."

  if [[ "$relative" == "worker" ]]; then
    (cd "$module" && GOWORK=off golangci-lint run --config="$ROOT/.golangci.yml" ./...)
  else
    (cd "$module" && golangci-lint run --config="$ROOT/.golangci.yml" ./...)
  fi
done

echo "=== All Go modules passed linting ==="
