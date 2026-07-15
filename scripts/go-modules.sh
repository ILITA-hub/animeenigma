#!/usr/bin/env bash
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)

# Every first-party Go module is a test/build boundary. Keep worker separate from
# go.work because its Docker build intentionally uses worker/ as its context.
find "$ROOT/libs" "$ROOT/services" "$ROOT/worker" \
  -maxdepth 2 -name go.mod -printf '%h\n' | sort
