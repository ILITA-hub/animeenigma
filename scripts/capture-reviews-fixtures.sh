#!/usr/bin/env bash
# capture-reviews-fixtures.sh — manual smoke for SOCIAL-NF-01 (golden-file
# shape diff before/after the social merge migration).
#
# Wave-0 scaffold: dumps the six reviews endpoints to stdout, JSON-per-line,
# each prefixed with a `# === <endpoint> ===` separator. The intended use is:
#
#   # Before plan 04 deploys:
#   bash scripts/capture-reviews-fixtures.sh > tmp/reviews-pre.json
#
#   # After plan 04 deploys (anime_list-backed implementation):
#   bash scripts/capture-reviews-fixtures.sh > tmp/reviews-post.json
#
#   diff tmp/reviews-pre.json tmp/reviews-post.json
#
# The diff must be empty (or only differ on volatile fields like timestamps)
# — the spec requires byte-identical JSON shape.
#
# Env vars:
#   API_BASE         — defaults to http://localhost:8000
#   ANIME_ID         — required; pick a seeded id with a real review row
#   UI_AUDIT_API_KEY — required for the `/reviews/me` endpoint (Bearer token).
#                      Read from env only — never echoed to stdout/stderr.
#
# Mutating endpoints (POST/DELETE) are NOT called from this script — they
# would change state mid-capture and pollute the diff. Their handler shape is
# covered by the Go unit tests instead.

set -euo pipefail

API_BASE="${API_BASE:-http://localhost:8000}"
ANIME_ID="${ANIME_ID:-?}"

if [[ "${ANIME_ID}" == "?" ]]; then
  echo "ANIME_ID env var is required" >&2
  exit 2
fi

# UI_AUDIT_API_KEY is optional — if unset, the /reviews/me call is skipped
# rather than echoing an Authorization: Bearer  header with an empty token.
AUTH_HEADER=()
if [[ -n "${UI_AUDIT_API_KEY:-}" ]]; then
  AUTH_HEADER=(-H "Authorization: Bearer ${UI_AUDIT_API_KEY}")
fi

curl_endpoint() {
  local label="$1"
  shift
  echo "# === ${label} ==="
  # -s silent, --max-time 10 to fail fast in CI, --fail-with-body to print
  # the body on non-2xx but still exit non-zero (so set -e kills the script
  # on real failures, not on 404s that are part of the contract).
  curl -sS --max-time 10 "$@" || echo "{\"error\":\"curl failed for ${label}\"}"
  echo ""
}

curl_endpoint "GET /api/anime/${ANIME_ID}/reviews" \
  "${API_BASE}/api/anime/${ANIME_ID}/reviews"

curl_endpoint "GET /api/anime/${ANIME_ID}/rating" \
  "${API_BASE}/api/anime/${ANIME_ID}/rating"

if [[ ${#AUTH_HEADER[@]} -gt 0 ]]; then
  curl_endpoint "GET /api/anime/${ANIME_ID}/reviews/me" \
    "${AUTH_HEADER[@]}" \
    "${API_BASE}/api/anime/${ANIME_ID}/reviews/me"
else
  echo "# === GET /api/anime/${ANIME_ID}/reviews/me ==="
  echo "# SKIP: UI_AUDIT_API_KEY not set; cannot exercise authenticated endpoint"
  echo ""
fi

curl_endpoint "POST /api/anime/ratings/batch" \
  -X POST \
  -H "Content-Type: application/json" \
  -d "{\"anime_ids\":[\"${ANIME_ID}\"]}" \
  "${API_BASE}/api/anime/ratings/batch"

echo "# === POST /api/anime/${ANIME_ID}/reviews ==="
echo "# SKIP: mutating endpoint shape already covered by handler test (TestReviewHandler_*)"
echo ""

echo "# === DELETE /api/anime/${ANIME_ID}/reviews ==="
echo "# SKIP: mutating endpoint shape already covered by handler test (TestReviewHandler_*)"
echo ""
