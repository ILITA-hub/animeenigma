#!/bin/bash
set -euo pipefail

# Usage: restore.sh <date>
# Example: restore.sh 2026-02-08

if [ $# -ne 1 ]; then
    echo "Usage: restore.sh <YYYY-MM-DD>"
    echo "Example: restore.sh 2026-02-08"
    exit 1
fi

RESTORE_DATE="$1"
DATABASES="animeenigma_auth animeenigma_catalog animeenigma_player animeenigma_rooms"
RESTORE_DIR="/tmp/restore"
S3_PATH="s3://${S3_BUCKET}/backups/${RESTORE_DATE}"

# AWS CLI configuration for S3-compatible storage
export AWS_ACCESS_KEY_ID="${S3_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="${S3_SECRET_KEY}"
export AWS_DEFAULT_REGION="${S3_REGION:-default}"

S3_ENDPOINT_URL="https://${S3_ENDPOINT}"

# Initialize
mkdir -p "${RESTORE_DIR}"
SUCCESS_COUNT=0
FAILED_DBS=""

echo "=== Restore started at $(date) ==="
echo "Restoring from: ${S3_PATH}"

# Verify backup exists
echo "Checking backup availability..."
if ! aws s3 ls "${S3_PATH}/" --endpoint-url "${S3_ENDPOINT_URL}" > /dev/null 2>&1; then
    echo "ERROR: No backup found at ${S3_PATH}"
    exit 1
fi

echo "Available backup files:"
aws s3 ls "${S3_PATH}/" --endpoint-url "${S3_ENDPOINT_URL}"
echo ""

# Restore each database
for DB in ${DATABASES}; do
    BACKUP_FILE="${RESTORE_DIR}/${DB}.sql.gz"
    S3_FILE="${S3_PATH}/${DB}.sql.gz"

    echo "--- Restoring ${DB} ---"

    # Download backup
    echo "  Downloading ${DB}.sql.gz..."
    if ! aws s3 cp "${S3_FILE}" "${BACKUP_FILE}" --endpoint-url "${S3_ENDPOINT_URL}" --quiet; then
        echo "  SKIP: ${DB}.sql.gz not found in backup (database may not have existed)"
        FAILED_DBS="${FAILED_DBS} ${DB}(missing)"
        continue
    fi

    # Verify file is not empty
    FILE_SIZE=$(stat -c%s "${BACKUP_FILE}" 2>/dev/null || stat -f%z "${BACKUP_FILE}" 2>/dev/null || echo "0")
    if [ "${FILE_SIZE}" -le 100 ]; then
        echo "  SKIP: ${DB}.sql.gz is empty or corrupted"
        rm -f "${BACKUP_FILE}"
        FAILED_DBS="${FAILED_DBS} ${DB}(empty)"
        continue
    fi

    echo "  Downloaded $(numfmt --to=iec ${FILE_SIZE} 2>/dev/null || echo "${FILE_SIZE} bytes")"

    # Drop and recreate database
    echo "  Dropping and recreating database..."
    PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -U "${DB_USER}" -d postgres -c "
        SELECT pg_terminate_backend(pg_stat_activity.pid)
        FROM pg_stat_activity
        WHERE pg_stat_activity.datname = '${DB}' AND pid <> pg_backend_pid();
    " > /dev/null 2>&1 || true

    PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -U "${DB_USER}" -d postgres -c "DROP DATABASE IF EXISTS \"${DB}\";" 2>/dev/null
    PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -U "${DB_USER}" -d postgres -c "CREATE DATABASE \"${DB}\";" 2>/dev/null

    # Restore from backup
    echo "  Restoring data..."
    if gunzip -c "${BACKUP_FILE}" | PGPASSWORD="${DB_PASSWORD}" psql -h "${DB_HOST}" -U "${DB_USER}" -d "${DB}" --quiet 2>/dev/null; then
        echo "  OK: ${DB} restored successfully"
        SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
    else
        echo "  ERROR: Failed to restore ${DB}"
        FAILED_DBS="${FAILED_DBS} ${DB}(restore-failed)"
    fi

    # Cleanup downloaded file
    rm -f "${BACKUP_FILE}"
done

# Cleanup
rm -rf "${RESTORE_DIR}"

echo ""
echo "=== Restore completed at $(date) ==="
echo "Successful: ${SUCCESS_COUNT}/$(echo ${DATABASES} | wc -w | tr -d ' ')"

if [ -n "${FAILED_DBS}" ]; then
    echo "Failed:${FAILED_DBS}"
    exit 1
else
    echo "All databases restored successfully"
fi
