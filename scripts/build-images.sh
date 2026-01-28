#!/bin/bash
set -e

REGISTRY=${REGISTRY:-"ghcr.io/ilita-hub/animeenigma"}
TAG=${TAG:-"latest"}

SERVICES=(
  "gateway"
  "auth"
  "catalog"
  "streaming"
  "player"
  "rooms"
  "scheduler"
)

echo "=== Building Docker images ==="

# Build backend services
for service in "${SERVICES[@]}"; do
  echo "Building $service..."
  docker build -t "$REGISTRY/$service:$TAG" -f "services/$service/Dockerfile" .
done

# Build frontend
echo "Building frontend..."
docker build -t "$REGISTRY/web:$TAG" -f "frontend/web/Dockerfile" frontend/web

echo "=== All images built successfully ==="
echo ""
echo "To push images:"
echo "  docker push $REGISTRY/gateway:$TAG"
echo "  # ... etc"
