#!/bin/bash
set -e

echo "=== Generating API code ==="

# Generate protobuf code
echo "Generating protobuf..."
buf generate api/proto

# Generate OpenAPI TypeScript client for frontend
echo "Generating OpenAPI TypeScript client..."
npx openapi-typescript-codegen \
  --input api/openapi/catalog.yaml \
  --output frontend/web/src/api/generated \
  --client axios \
  --useUnionTypes

echo "API generation complete!"
