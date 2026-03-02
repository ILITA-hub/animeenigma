#!/bin/bash
set -eo pipefail

# Configuration
DATE=$(date +%Y-%m-%d)
DATABASES="animeenigma"
BACKUP_DIR="/tmp/backups"
S3_PATH="s3://${S3_BUCKET}/backups/${DATE}"
EXPECTED_COUNT=$(echo ${DATABASES} | wc -w | tr -d ' ')

# AWS CLI configuration for S3-compatible storage
export AWS_ACCESS_KEY_ID="${S3_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="${S3_SECRET_KEY}"
export AWS_DEFAULT_REGION="${S3_REGION:-default}"

# S3 endpoint URL
S3_ENDPOINT_URL="https://${S3_ENDPOINT}"

# Format bytes to human-readable (KB/MB/GB)
format_size() {
    local bytes=$1
    if [ "${bytes}" -ge 1073741824 ]; then
        echo "$(awk "BEGIN {printf \"%.1f GB\", ${bytes}/1073741824}")"
    elif [ "${bytes}" -ge 1048576 ]; then
        echo "$(awk "BEGIN {printf \"%.1f MB\", ${bytes}/1048576}")"
    elif [ "${bytes}" -ge 1024 ]; then
        echo "$(awk "BEGIN {printf \"%.1f KB\", ${bytes}/1024}")"
    else
        echo "${bytes} B"
    fi
}

# Telegram notification function
send_telegram() {
    local message="$1"
    if [ -n "${TELEGRAM_BOT_TOKEN}" ] && [ -n "${TELEGRAM_BACKUP_CHAT_ID}" ]; then
        curl -s -X POST "https://api.telegram.org/bot${TELEGRAM_BOT_TOKEN}/sendMessage" \
            -d "chat_id=${TELEGRAM_BACKUP_CHAT_ID}" \
            -d "text=${message}" \
            -d "parse_mode=HTML" > /dev/null 2>&1 || true
    fi
}

# Initialize
mkdir -p "${BACKUP_DIR}"
TOTAL_SIZE=0
SUCCESS_COUNT=0
FAILED_DBS=""

echo "=== Backup started at $(date) ==="

# Backup each database
for DB in ${DATABASES}; do
    echo "Backing up ${DB}..."
    BACKUP_FILE="${BACKUP_DIR}/${DB}.sql.gz"

    # Create backup
    DUMP_ERR="${BACKUP_DIR}/${DB}.err"
    if PGPASSWORD="${DB_PASSWORD}" pg_dump -h "${DB_HOST}" -U "${DB_USER}" "${DB}" 2>"${DUMP_ERR}" | gzip > "${BACKUP_FILE}"; then
        # Check if backup file is not empty (empty gzip is 20 bytes)
        FILE_SIZE=$(stat -c%s "${BACKUP_FILE}" 2>/dev/null || stat -f%z "${BACKUP_FILE}" 2>/dev/null || echo "0")

        if [ "${FILE_SIZE}" -gt 100 ]; then
            TOTAL_SIZE=$((TOTAL_SIZE + FILE_SIZE))

            # Upload to S3
            if aws s3 cp "${BACKUP_FILE}" "${S3_PATH}/${DB}.sql.gz" --endpoint-url "${S3_ENDPOINT_URL}" --quiet; then
                echo "  Uploaded ${DB}.sql.gz ($(format_size ${FILE_SIZE}))"
                SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
            else
                echo "  ERROR: Failed to upload ${DB}.sql.gz to S3"
                FAILED_DBS="${FAILED_DBS} ${DB}(upload-failed)"
            fi
        else
            echo "  ERROR: Database ${DB} backup is empty (${FILE_SIZE} bytes)"
            [ -s "${DUMP_ERR}" ] && echo "  pg_dump stderr: $(cat "${DUMP_ERR}")"
            FAILED_DBS="${FAILED_DBS} ${DB}(empty)"
        fi

        # Remove local files
        rm -f "${BACKUP_FILE}" "${DUMP_ERR}"
    else
        echo "  ERROR: Failed to backup ${DB}"
        [ -s "${DUMP_ERR}" ] && echo "  pg_dump stderr: $(cat "${DUMP_ERR}")"
        FAILED_DBS="${FAILED_DBS} ${DB}(dump-failed)"
        rm -f "${BACKUP_FILE}" "${DUMP_ERR}"
    fi
done

# Cleanup old backups
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
echo "Cleaning up backups older than ${RETENTION_DAYS} days..."

# List all backup folders and delete old ones
# || true to prevent pipefail from killing the script before notification
aws s3 ls "s3://${S3_BUCKET}/backups/" --endpoint-url "${S3_ENDPOINT_URL}" 2>/dev/null | while read -r line; do
    FOLDER_DATE=$(echo "${line}" | awk '{print $2}' | tr -d '/')
    if [ -n "${FOLDER_DATE}" ]; then
        # Calculate age in days
        FOLDER_TIMESTAMP=$(date -d "${FOLDER_DATE}" +%s 2>/dev/null || date -j -f "%Y-%m-%d" "${FOLDER_DATE}" +%s 2>/dev/null || echo "0")
        CURRENT_TIMESTAMP=$(date +%s)
        AGE_DAYS=$(( (CURRENT_TIMESTAMP - FOLDER_TIMESTAMP) / 86400 ))

        if [ "${AGE_DAYS}" -gt "${RETENTION_DAYS}" ]; then
            echo "  Deleting old backup: ${FOLDER_DATE} (${AGE_DAYS} days old)"
            aws s3 rm "s3://${S3_BUCKET}/backups/${FOLDER_DATE}/" --recursive --endpoint-url "${S3_ENDPOINT_URL}" --quiet
        fi
    fi
done || true

# Format total size
TOTAL_SIZE_HUMAN=$(format_size ${TOTAL_SIZE})

echo "=== Backup completed at $(date) ==="

# Send notification
if [ -z "${FAILED_DBS}" ] && [ "${SUCCESS_COUNT}" -eq "${EXPECTED_COUNT}" ]; then
    send_telegram "<b>✅ Backup completed</b>

<b>Date:</b> ${DATE}
<b>Databases:</b> ${SUCCESS_COUNT}/${EXPECTED_COUNT}
<b>Total size:</b> ${TOTAL_SIZE_HUMAN}
<b>Retention:</b> ${RETENTION_DAYS} days"
else
    send_telegram "<b>❌ Backup FAILED</b>

<b>Date:</b> ${DATE}
<b>Successful:</b> ${SUCCESS_COUNT}/${EXPECTED_COUNT}
<b>Failed:</b>${FAILED_DBS:- none, but expected ${EXPECTED_COUNT}}

Check logs: docker logs animeenigma-backup"
    exit 1
fi
