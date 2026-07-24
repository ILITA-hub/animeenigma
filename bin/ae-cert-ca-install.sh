#!/usr/bin/env bash
# Exports the platform user-CA from the auth service and installs it where the
# cert.animeenigma.org vhost expects it, then reloads nginx.
# Run on the HOST after `make redeploy-auth` first boots the CA.
set -euo pipefail

DEST=/etc/nginx/certs/ae-user-ca.pem
TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

curl -fsS http://127.0.0.1:8080/cert/ca.pem -o "$TMP"
grep -q "BEGIN CERTIFICATE" "$TMP" || { echo "unexpected CA payload"; exit 1; }
grep -q "END CERTIFICATE" "$TMP" || { echo "truncated CA payload"; exit 1; }

install -m 0644 "$TMP" "$DEST"
nginx -t && systemctl reload nginx
echo "installed $DEST"
