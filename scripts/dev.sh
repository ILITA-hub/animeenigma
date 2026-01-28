#!/bin/bash
set -e

# Start development environment
echo "=== Starting AnimeEnigma Development Environment ==="

# Start infrastructure
echo "Starting infrastructure..."
docker compose -f docker/docker-compose.yml up -d postgres redis minio nats meilisearch

# Wait for services to be ready
echo "Waiting for services to be ready..."
sleep 5

# Check health
echo "Checking service health..."
docker compose -f docker/docker-compose.yml ps

echo ""
echo "=== Infrastructure is ready ==="
echo ""
echo "Services:"
echo "  PostgreSQL: localhost:5432"
echo "  Redis:      localhost:6379"
echo "  MinIO:      localhost:9000 (console: localhost:9001)"
echo "  NATS:       localhost:4222"
echo "  Meilisearch: localhost:7700"
echo ""
echo "To start backend services, run individual services or use:"
echo "  docker compose -f docker/docker-compose.yml up -d"
echo ""
echo "To start frontend dev server:"
echo "  cd frontend/web && npm run dev"
