#!/usr/bin/env bash
# Mint a single-use upscaler worker enroll token and print ONLY the token to
# stdout (so it can be captured into a variable / passed to a worker container).
#
# The upscaler control-plane has no admin endpoint or CLI to MINT enroll tokens
# (the worker can only CONSUME one via POST /worker/enroll). This is that missing
# operator path: it inserts a fresh random token directly into the upscaler
# Postgres table `upscale_enroll_tokens` (columns: token TEXT PK, consumed_at
# NULLABLE, created_at) so a brand-new worker can enroll exactly once.
#
# The worker enroll matches `WHERE token = ? AND consumed_at IS NULL`, so the
# token is single-use: once a worker enrolls, consumed_at is set and the token
# can never be replayed. A given worker only needs a fresh token for its FIRST
# enroll — the session it receives is permanent (no 12h re-enroll), so you only
# mint again for a brand-new worker.
#
# Usage:
#   bin/upscaler-mint-enroll-token.sh                 # uses the defaults below
#   bin/upscaler-mint-enroll-token.sh <container>     # override postgres container
#   bin/upscaler-mint-enroll-token.sh <container> <db_name> <db_user>
#
# Configuration (env overrides args; args override defaults):
#   PG_CONTAINER  postgres container name        (default: animeenigma-postgres)
#   PG_DB         upscaler database name          (default: upscaler)
#   PG_USER       postgres role                   (default: postgres)
#
# Examples:
#   # Prod stack (default postgres container):
#   TOKEN=$(bin/upscaler-mint-enroll-token.sh)
#
#   # Isolated handover stack (compose project upscaler-handover):
#   TOKEN=$(PG_CONTAINER=upscaler-handover-postgres-1 \
#           bin/upscaler-mint-enroll-token.sh)
#
# Then run a worker:
#   docker run --rm -e SERVER_URL=https://ext.animeenigma.org \
#     -e ENROLL_TOKEN="$TOKEN" -e MODEL=mock ae-upscaler-worker:handover
#
# SECURITY NOTE: tokens are stored in PLAINTEXT at rest (single-use, short-lived
# operational secret). Hashing the token at rest is a Phase-2 hardening — for
# Phase 1 plaintext single-use is acceptable.
set -euo pipefail

PG_CONTAINER="${PG_CONTAINER:-${1:-animeenigma-postgres}}"
PG_DB="${PG_DB:-${2:-upscaler}}"
PG_USER="${PG_USER:-${3:-postgres}}"

# Generate a 48-hex-char (24-byte) cryptographically-random token.
TOKEN="$(openssl rand -hex 24)"

# Insert it. psql -v ON_ERROR_STOP=1 so any failure (bad container/db) exits non-zero
# and we never print a token that was not actually persisted. The value is passed via
# a psql variable (:'tok') so it is properly quoted/escaped — no SQL injection surface.
# The SQL is fed on stdin (not -c) because psql only performs :'var' interpolation in
# script/stdin input, not in -c command strings.
docker exec -i "$PG_CONTAINER" \
  psql -v ON_ERROR_STOP=1 -q -U "$PG_USER" -d "$PG_DB" \
  -v tok="$TOKEN" \
  >/dev/null <<'SQL'
INSERT INTO upscale_enroll_tokens (token, created_at) VALUES (:'tok', now());
SQL

# Print ONLY the token (no trailing decoration) so callers can capture it.
printf '%s\n' "$TOKEN"
