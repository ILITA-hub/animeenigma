#!/bin/bash
set -e

# Configuration
DATE=$(date +%Y-%m-%d)
DATABASES="animeenigma_auth animeenigma_catalog animeenigma_player animeenigma_rooms"
BACKUP_DIR="/tmp/backups"
S3_PATH="s3://${S3_BUCKET}/backups/${DATE}"

# AWS CLI configuration for S3-compatible storage
export AWS_ACCESS_KEY_ID="${S3_ACCESS_KEY}"
export AWS_SECRET_ACCESS_KEY="${S3_SECRET_KEY}"
export AWS_DEFAULT_REGION="${S3_REGION:-default}"

# S3 endpoint URL
S3_ENDPOINT_URL="https://${S3_ENDPOINT}"

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

    # Create backup (pg_dump will fail if database doesn't exist)
    if PGPASSWORD="${DB_PASSWORD}" pg_dump -h "${DB_HOST}" -U "${DB_USER}" "${DB}" 2>/dev/null | gzip > "${BACKUP_FILE}"; then
        # Check if backup file is not empty (empty gzip is 20 bytes)
        FILE_SIZE=$(stat -c%s "${BACKUP_FILE}" 2>/dev/null || stat -f%z "${BACKUP_FILE}" 2>/dev/null || echo "0")

        if [ "${FILE_SIZE}" -gt 100 ]; then
            TOTAL_SIZE=$((TOTAL_SIZE + FILE_SIZE))

            # Upload to S3
            if aws s3 cp "${BACKUP_FILE}" "${S3_PATH}/${DB}.sql.gz" --endpoint-url "${S3_ENDPOINT_URL}" --quiet; then
                echo "  Uploaded ${DB}.sql.gz ($(numfmt --to=iec ${FILE_SIZE} 2>/dev/null || echo "${FILE_SIZE} bytes"))"
                SUCCESS_COUNT=$((SUCCESS_COUNT + 1))
            else
                echo "  ERROR: Failed to upload ${DB}.sql.gz to S3"
                FAILED_DBS="${FAILED_DBS} ${DB}"
            fi
        else
            echo "  SKIP: Database ${DB} is empty or does not exist"
        fi

        # Remove local backup
        rm -f "${BACKUP_FILE}"
    else
        echo "  ERROR: Failed to backup ${DB}"
        FAILED_DBS="${FAILED_DBS} ${DB}"
        rm -f "${BACKUP_FILE}"
    fi
done

# Cleanup old backups
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
echo "Cleaning up backups older than ${RETENTION_DAYS} days..."

# List all backup folders and delete old ones
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
done

# Format total size
TOTAL_SIZE_HUMAN=$(numfmt --to=iec ${TOTAL_SIZE} 2>/dev/null || echo "${TOTAL_SIZE} bytes")

echo "=== Backup completed at $(date) ==="

# Send notification
if [ -z "${FAILED_DBS}" ]; then
    send_telegram "<b>Backup completed</b>

<b>Date:</b> ${DATE}
<b>Databases:</b> ${SUCCESS_COUNT}
<b>Total size:</b> ${TOTAL_SIZE_HUMAN}
<b>Retention:</b> ${RETENTION_DAYS} days"
else
    send_telegram "<b>Backup FAILED</b>

<b>Date:</b> ${DATE}
<b>Successful:</b> ${SUCCESS_COUNT}
<b>Failed:</b>${FAILED_DBS}

Please check the logs!"
    exit 1
fi
