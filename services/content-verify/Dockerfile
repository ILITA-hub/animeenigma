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
COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/
COPY libs/animeparser/go.mod libs/animeparser/go.sum* ./libs/animeparser/
COPY libs/videoutils/go.mod libs/videoutils/go.sum* ./libs/videoutils/
COPY libs/idmapping/go.mod libs/idmapping/go.sum* ./libs/idmapping/
COPY libs/kodikextract/go.mod libs/kodikextract/go.sum* ./libs/kodikextract/
COPY libs/metrics/go.mod libs/metrics/go.sum* ./libs/metrics/
COPY libs/maintenancegate/go.mod libs/maintenancegate/go.sum* ./libs/maintenancegate/
COPY libs/storageclient/go.mod libs/storageclient/go.sum* ./libs/storageclient/
COPY services/auth/go.mod services/auth/go.sum* ./services/auth/
COPY services/upscaler/go.mod services/upscaler/go.sum* ./services/upscaler/
COPY services/catalog/go.mod services/catalog/go.sum* ./services/catalog/
COPY services/content-verify/go.mod services/content-verify/go.sum* ./services/content-verify/
COPY services/streaming/go.mod services/streaming/go.sum* ./services/streaming/
COPY services/player/go.mod services/player/go.sum* ./services/player/
COPY services/recs/go.mod services/recs/go.sum* ./services/recs/
COPY services/rooms/go.mod services/rooms/go.sum* ./services/rooms/
COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/
COPY services/scheduler/go.mod services/scheduler/go.sum* ./services/scheduler/
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/
COPY services/governor/go.mod services/governor/go.sum* ./services/governor/
COPY services/themes/go.mod services/themes/go.sum* ./services/themes/
COPY services/notifications/go.mod services/notifications/go.sum* ./services/notifications/
COPY services/watch-together/go.mod services/watch-together/go.sum* ./services/watch-together/
COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/
COPY services/maintenance/go.mod services/maintenance/go.sum* ./services/maintenance/
COPY services/library/go.mod services/library/go.sum* ./services/library/
COPY services/gacha/go.mod services/gacha/go.sum* ./services/gacha/
COPY services/policy/go.mod services/policy/go.sum* ./services/policy/
COPY services/anidle/go.mod services/anidle/go.sum* ./services/anidle/
COPY services/fanfic/go.mod services/fanfic/go.sum* ./services/fanfic/
COPY services/storage/go.mod services/storage/go.sum* ./services/storage/

RUN cd services/content-verify && go mod download

# Copy source
COPY libs/ ./libs/
COPY services/content-verify/ ./services/content-verify/

# Build
RUN cd services/content-verify && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /content-verify-api ./cmd/content-verify-api

# Runtime: debian-slim because tesseract-ocr language packs + faster-whisper
# (ctranslate2 wheels) are apt/pip-clean here; alpine is not.
FROM debian:bookworm-slim
ENV PYTHONUNBUFFERED=1 \
    PIP_NO_CACHE_DIR=1 \
    HF_HOME=/opt/models \
    OMP_NUM_THREADS=2
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    tesseract-ocr tesseract-ocr-eng tesseract-ocr-rus tesseract-ocr-jpn \
    python3 python3-pip \
    ca-certificates wget tzdata \
 && rm -rf /var/lib/apt/lists/*
COPY services/content-verify/analyzers/requirements.txt /app/analyzers/requirements.txt
RUN pip3 install --break-system-packages -r /app/analyzers/requirements.txt
# Bake the whisper tiny model into the image (no first-probe download stall).
RUN python3 -c "from faster_whisper import WhisperModel; WhisperModel('tiny', device='cpu', compute_type='int8')"
COPY services/content-verify/analyzers/ /app/analyzers/
COPY --from=builder /content-verify-api /app/
WORKDIR /app
RUN groupadd -r app && useradd -r -g app app && mkdir -p /tmp/cv && chown -R app:app /app /tmp/cv /opt/models
USER app
EXPOSE 8101
CMD ["./content-verify-api"]
