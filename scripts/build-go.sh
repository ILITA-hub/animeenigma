#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
mapfile -t MODULES < <("$ROOT/scripts/go-modules.sh")

echo "=== Building ${#MODULES[@]} Go modules ==="

for module in "${MODULES[@]}"; do
  relative=${module#"$ROOT/"}
  echo "Building $relative..."

  if [[ "$relative" == "worker" ]]; then
    (cd "$module" && GOWORK=off go build ./...)
  else
    (cd "$module" && go build ./...)
  fi
done

echo "=== All Go modules built ==="
