# Build stage
FROM golang:1.25-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

# Copy go.work and all module files
COPY go.work go.work.sum ./
COPY libs/logger/go.mod libs/logger/go.sum* ./libs/logger/
COPY libs/tracing/go.mod libs/tracing/go.sum* ./libs/tracing/
COPY libs/errors/go.mod libs/errors/go.sum* ./libs/errors/
COPY libs/cache/go.mod libs/cache/go.sum* ./libs/cache/
COPY libs/database/go.mod libs/database/go.sum* ./libs/database/
COPY libs/authz/go.mod libs/authz/go.sum* ./libs/authz/
COPY libs/httputil/go.mod libs/httputil/go.sum* ./libs/httputil/
COPY libs/pagination/go.mod libs/pagination/go.sum* ./libs/pagination/
COPY libs/animeparser/go.mod libs/animeparser/go.sum* ./libs/animeparser/
COPY libs/videoutils/go.mod libs/videoutils/go.sum* ./libs/videoutils/
COPY libs/storageclient/go.mod libs/storageclient/go.sum* ./libs/storageclient/
COPY libs/idmapping/go.mod libs/idmapping/go.sum* ./libs/idmapping/
COPY libs/kodikextract/go.mod libs/kodikextract/go.sum* ./libs/kodikextract/
COPY libs/metrics/go.mod libs/metrics/go.sum* ./libs/metrics/
COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/
COPY libs/maintenancegate/go.mod libs/maintenancegate/go.sum* ./libs/maintenancegate/
COPY services/auth/go.mod services/auth/go.sum* ./services/auth/
COPY services/upscaler/go.mod services/upscaler/go.sum* ./services/upscaler/
COPY services/catalog/go.mod services/catalog/go.sum* ./services/catalog/
COPY services/streaming/go.mod services/streaming/go.sum* ./services/streaming/
COPY services/player/go.mod services/player/go.sum* ./services/player/
COPY services/rooms/go.mod services/rooms/go.sum* ./services/rooms/
COPY services/scheduler/go.mod services/scheduler/go.sum* ./services/scheduler/
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/
COPY services/governor/go.mod services/governor/go.sum* ./services/governor/
COPY services/themes/go.mod services/themes/go.sum* ./services/themes/
COPY services/notifications/go.mod services/notifications/go.sum* ./services/notifications/
COPY services/maintenance/go.mod services/maintenance/go.sum* ./services/maintenance/
COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/
COPY services/library/go.mod services/library/go.sum* ./services/library/
COPY services/watch-together/go.mod services/watch-together/go.sum* ./services/watch-together/
COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/
COPY services/gacha/go.mod services/gacha/go.sum* ./services/gacha/
COPY services/recs/go.mod services/recs/go.sum* ./services/recs/
COPY services/anidle/go.mod services/anidle/go.sum* ./services/anidle/
COPY services/fanfic/go.mod services/fanfic/go.sum* ./services/fanfic/
COPY services/policy/go.mod services/policy/go.sum* ./services/policy/
COPY services/storage/go.mod services/storage/go.sum* ./services/storage/

RUN cd services/library && go mod download

# Copy source
COPY libs/ ./libs/
COPY services/library/ ./services/library/

# Build the API server + the one-shot batch-ingest operator tool
# (cmd/library-batchingest — used via `docker compose run` to wire a
# multi-file season pack into MinIO; see its package doc).
RUN cd services/library && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /library-api ./cmd/library-api
RUN cd services/library && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /library-batchingest ./cmd/library-batchingest
# One-shot storage-migrate operator tool (cmd/library-storage-migrate — used via
# `docker compose run` to move episode content minio → external S3 with a
# copy-verify-flip-delete safety sequence; see its package doc).
RUN cd services/library && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /library-storage-migrate ./cmd/library-storage-migrate

# Runtime stage
FROM alpine:3.19

# Phase 04 (raw-jp / v0.2): ffmpeg + ffprobe baked into the runtime so
# the encoder worker (services/library/internal/service/encoder_worker.go)
# can shell out via os/exec at the SPEC-locked paths
# /usr/bin/ffmpeg + /usr/bin/ffprobe. Alpine 3.19's `ffmpeg` package
# ships both binaries; image bloat (~80 MB) is acceptable per SPEC.
RUN apk add --no-cache ca-certificates tzdata ffmpeg

WORKDIR /app

COPY --from=builder /library-api .
COPY --from=builder /library-batchingest /app/library-batchingest
COPY --from=builder /library-storage-migrate /app/library-storage-migrate

EXPOSE 8089

CMD ["./library-api"]
