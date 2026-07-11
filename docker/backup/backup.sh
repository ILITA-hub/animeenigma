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
            --data-urlencode "text=${message}" \
            -d "parse_mode=HTML" > /dev/null 2>&1 || true
    fi
}

# Russian plural for days: 1 день / 2 дня / 5 дней
plural_days() {
    local n=$1
    if [ $((n % 100)) -ge 11 ] && [ $((n % 100)) -le 14 ]; then
        echo "дней"
    elif [ $((n % 10)) -eq 1 ]; then
        echo "день"
    elif [ $((n % 10)) -ge 2 ] && [ $((n % 10)) -le 4 ]; then
        echo "дня"
    else
        echo "дней"
    fi
}

# Initialize
mkdir -p "${BACKUP_DIR}"
START_TS=$(date +%s)
TOTAL_SIZE=0
SUCCESS_COUNT=0
SUCCEEDED_DBS=""
FAILED_DBS=""
FAILED_LINES=""

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
                SUCCEEDED_DBS="${SUCCEEDED_DBS:+${SUCCEEDED_DBS}, }${DB}"
            else
                echo "  ERROR: Failed to upload ${DB}.sql.gz to S3"
                FAILED_DBS="${FAILED_DBS} ${DB}(upload-failed)"
                FAILED_LINES="${FAILED_LINES}❌ ${DB} — не загрузился в S3"$'\n'
            fi
        else
            echo "  ERROR: Database ${DB} backup is empty (${FILE_SIZE} bytes)"
            [ -s "${DUMP_ERR}" ] && echo "  pg_dump stderr: $(cat "${DUMP_ERR}")"
            FAILED_DBS="${FAILED_DBS} ${DB}(empty)"
            FAILED_LINES="${FAILED_LINES}❌ ${DB} — пустой дамп (${FILE_SIZE} B)"$'\n'
        fi

        # Remove local files
        rm -f "${BACKUP_FILE}" "${DUMP_ERR}"
    else
        echo "  ERROR: Failed to backup ${DB}"
        [ -s "${DUMP_ERR}" ] && echo "  pg_dump stderr: $(cat "${DUMP_ERR}")"
        FAILED_DBS="${FAILED_DBS} ${DB}(dump-failed)"
        FAILED_LINES="${FAILED_LINES}❌ ${DB} — pg_dump упал"$'\n'
        rm -f "${BACKUP_FILE}" "${DUMP_ERR}"
    fi
done

# Cleanup old backups
RETENTION_DAYS="${BACKUP_RETENTION_DAYS:-14}"
echo "Cleaning up backups older than ${RETENTION_DAYS} days..."

# List all backup folders and delete old ones. Herestring (not a pipe) so
# DELETED_COUNT survives the loop; || true keeps a listing failure from
# killing the script before notification.
DELETED_COUNT=0
BACKUP_FOLDERS=$(aws s3 ls "s3://${S3_BUCKET}/backups/" --endpoint-url "${S3_ENDPOINT_URL}" 2>/dev/null || true)
while read -r line; do
    FOLDER_DATE=$(echo "${line}" | awk '{print $2}' | tr -d '/')
    if [ -n "${FOLDER_DATE}" ]; then
        # Calculate age in days
        FOLDER_TIMESTAMP=$(date -d "${FOLDER_DATE}" +%s 2>/dev/null || date -j -f "%Y-%m-%d" "${FOLDER_DATE}" +%s 2>/dev/null || echo "0")
        CURRENT_TIMESTAMP=$(date +%s)
        AGE_DAYS=$(( (CURRENT_TIMESTAMP - FOLDER_TIMESTAMP) / 86400 ))

        if [ "${AGE_DAYS}" -gt "${RETENTION_DAYS}" ]; then
            echo "  Deleting old backup: ${FOLDER_DATE} (${AGE_DAYS} days old)"
            if aws s3 rm "s3://${S3_BUCKET}/backups/${FOLDER_DATE}/" --recursive --endpoint-url "${S3_ENDPOINT_URL}" --quiet; then
                DELETED_COUNT=$((DELETED_COUNT + 1))
            else
                echo "  WARN: failed to delete ${FOLDER_DATE}"
            fi
        fi
    fi
done <<< "${BACKUP_FOLDERS}"

# Format total size and duration
TOTAL_SIZE_HUMAN=$(format_size ${TOTAL_SIZE})
DURATION=$(( $(date +%s) - START_TS ))
if [ "${DURATION}" -ge 60 ]; then
    DURATION_HUMAN="$((DURATION / 60)) мин $((DURATION % 60)) с"
else
    DURATION_HUMAN="${DURATION} с"
fi

# Next cron run (daily at 03:00, container TZ) relative to now
if [ $((10#$(date +%H))) -lt 3 ]; then
    NEXT_RUN="сегодня в 03:00"
else
    NEXT_RUN="завтра в 03:00"
fi

CLEANUP_NOTE=""
if [ "${DELETED_COUNT}" -gt 0 ]; then
    CLEANUP_NOTE=" (удалено старых: ${DELETED_COUNT})"
fi

echo "=== Backup completed at $(date) ==="

# Send notification
if [ -z "${FAILED_DBS}" ] && [ "${SUCCESS_COUNT}" -eq "${EXPECTED_COUNT}" ]; then
    send_telegram "<b>✅ Бекап выполнен</b> — ${DATE}

🗄 Базы: ${SUCCESS_COUNT}/${EXPECTED_COUNT} · ${SUCCEEDED_DBS}
📦 Размер: ${TOTAL_SIZE_HUMAN}
⏱ Длительность: ${DURATION_HUMAN}
♻️ Хранение: ${RETENTION_DAYS} $(plural_days "${RETENTION_DAYS}")${CLEANUP_NOTE}
⏰ Следующий: ${NEXT_RUN}"
else
    if [ -z "${FAILED_LINES}" ]; then
        FAILED_LINES="❌ причина неизвестна — см. логи"$'\n'
    fi
    send_telegram "<b>🚨 БЕКАП НЕ ПРОШЁЛ</b> — ${DATE}

${FAILED_LINES}✅ Успешно: ${SUCCESS_COUNT}/${EXPECTED_COUNT}

Логи: <code>docker logs animeenigma-backup</code>"
    exit 1
fi
