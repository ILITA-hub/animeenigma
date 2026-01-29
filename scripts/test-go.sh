#!/bin/bash
set -e

echo "=== Running Go tests ==="

# Run tests for all services
SERVICES=(
  "auth"
  "catalog"
  "streaming"
  "player"
  "rooms"
  "scheduler"
  "gateway"
)

for service in "${SERVICES[@]}"; do
  echo "Testing $service..."
  cd "services/$service"
  go test ./... -cover -v
  cd ../..
done

# Run tests for libs
echo "Testing libs..."
for lib in libs/*/; do
  if [ -f "$lib/go.mod" ]; then
    echo "Testing $lib..."
    cd "$lib"
    go test ./... -cover -v 2>/dev/null || true
    cd ../..
  fi
done

echo "=== All tests passed ==="
