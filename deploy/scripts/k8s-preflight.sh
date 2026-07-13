#!/usr/bin/env bash
# k8s-preflight — refuse to deploy the prod overlay with missing or placeholder secrets.
# (audit 2026-06-21 #10: the old committed secrets.yaml shipped 'change-this-in-production')
set -euo pipefail

ROOT="$(git rev-parse --show-toplevel)"
OVERLAY="$ROOT/deploy/kustomize/overlays/prod"
FAIL=0

for example in "$OVERLAY"/*/*.example; do
  real="${example%.example}"
  if [[ ! -f "$real" ]]; then
    echo "MISSING: $real (copy from ${example##*/} and fill in real values)" >&2
    FAIL=1
    continue
  fi
  if grep -nE 'CHANGE_ME|change-this-in-production|minioadmin|admin:admin' "$real" >&2; then
    echo "PLACEHOLDER VALUES in $real (see lines above)" >&2
    FAIL=1
  fi
done

# jwt/stream secrets must be non-trivial
appenv="$OVERLAY/app-secrets/animeenigma.env"
if [[ -f "$appenv" ]]; then
  for key in jwt-secret stream-token-secret; do
    val="$(grep -E "^$key=" "$appenv" | head -1 | cut -d= -f2- || true)"
    if [[ "${#val}" -lt 32 ]]; then
      echo "WEAK: $key in animeenigma.env is shorter than 32 chars (use: openssl rand -hex 32)" >&2
      FAIL=1
    fi
  done
fi

if [[ "$FAIL" -ne 0 ]]; then
  echo "k8s-preflight: FAILED — fix the issues above before deploying" >&2
  exit 1
fi
echo "k8s-preflight: OK"
