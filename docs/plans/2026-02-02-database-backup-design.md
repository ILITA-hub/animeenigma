# Database Backup System Design

## Overview

Automated daily PostgreSQL backup system with S3 storage (FirstVDS) and Telegram notifications.

## Requirements

- **Frequency:** Daily at 03:00
- **Retention:** 14 days
- **Storage:** FirstVDS S3 (s3.firstvds.ru)
- **Notifications:** Telegram (success and failure)
- **Databases:** animeenigma_auth, animeenigma_catalog, animeenigma_player, animeenigma_rooms

## Architecture

```
┌─────────────────────────────────────────────────────┐
│  backup-container (Alpine + cron)                   │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  │
│  │  pg_dump    │  │  aws-cli    │  │  curl       │  │
│  │  (backup)   │  │  (S3 upload)│  │  (telegram) │  │
│  └─────────────┘  └─────────────┘  └─────────────┘  │
└─────────────────────────────────────────────────────┘
           │                              │
           ▼                              ▼
    ┌────────────┐                 ┌─────────────┐
    │ PostgreSQL │                 │ FirstVDS S3 │
    └────────────┘                 └─────────────┘
```

## File Structure

```
docker/
├── backup/
│   ├── Dockerfile
│   ├── backup.sh
│   └── crontab
├── docker-compose.yml  (updated)
└── .env.example        (updated)
```

## S3 Structure

```
s3://animeenigma/backups/
├── 2026-02-02/
│   ├── animeenigma_auth.sql.gz
│   ├── animeenigma_catalog.sql.gz
│   ├── animeenigma_player.sql.gz
│   └── animeenigma_rooms.sql.gz
└── ...
```

## Configuration

```env
S3_ENDPOINT=s3.firstvds.ru
S3_ACCESS_KEY=<key>
S3_SECRET_KEY=<secret>
S3_BUCKET=animeenigma
S3_REGION=default
TELEGRAM_BACKUP_CHAT_ID=468462557
BACKUP_RETENTION_DAYS=14
```

## Backup Script Logic

1. Get current date (YYYY-MM-DD)
2. For each database:
   - pg_dump with gzip compression
   - Upload to S3: backups/{date}/{db_name}.sql.gz
   - Remove local temp file
3. Delete folders older than 14 days from S3
4. Send Telegram notification with result
