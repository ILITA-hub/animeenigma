# Build on the NATIVE build host and cross-compile to the target arch (CGO off →
# no QEMU needed for the Go build; only the slim runtime stage is per-arch).
FROM --platform=$BUILDPLATFORM golang:1.25 AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -o /worker ./cmd/worker

# Runtime stage: buildx selects the per-arch base automatically (amd64/arm64).
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    ca-certificates \
    wget \
    unzip \
    && rm -rf /var/lib/apt/lists/*

LABEL org.opencontainers.image.title="upscaler-worker"
LABEL org.opencontainers.image.description="GPU worker agent (model-agnostic: built-in mock + optional baked PREINSTALLED_MODELS + server pull-on-demand)"

# realesrgan-ncnn-vulkan binary — the runtime for realesrgan weight-set models,
# whether baked in (PREINSTALLED_MODELS) or pulled on demand. It needs a
# GPU/Vulkan at run time; the built-in `mock` model is pure-Go and needs neither
# this binary nor a GPU.
#
# ARCH NOTE: upstream ships a Linux **x86_64** build only — there is no Linux
# arm64 release. So we fetch it on amd64 and SKIP it on other arches; an arm64
# image therefore runs `mock` (and any future CPU path) but cannot do realesrgan
# upscaling until an arm64 binary exists. The fetch is best-effort either way
# (a failed download must not break the build).
ARG TARGETARCH
ARG REALESRGAN_VERSION=0.2.0
RUN if [ "$TARGETARCH" = "amd64" ]; then \
        wget -q -O /tmp/realesrgan.zip \
        "https://github.com/xinntao/Real-ESRGAN/releases/download/v${REALESRGAN_VERSION}/realesrgan-ncnn-vulkan-${REALESRGAN_VERSION}-ubuntu.zip" \
        && unzip /tmp/realesrgan.zip -d /tmp/realesrgan \
        && mv /tmp/realesrgan/realesrgan-ncnn-vulkan /usr/local/bin/realesrgan-ncnn-vulkan \
        && chmod +x /usr/local/bin/realesrgan-ncnn-vulkan \
        && rm -rf /tmp/realesrgan /tmp/realesrgan.zip \
        || echo "realesrgan fetch failed (amd64) — image structure is what matters" ; \
    else \
        echo "realesrgan-ncnn-vulkan: no Linux ${TARGETARCH} build upstream — this arch is mock-only" ; \
    fi

# ── Model weights directory (MODELS_DIR, default /models) ────────────────────
# This single directory holds BOTH:
#   1. Baked-in weight sets the operator preinstalls into the image, and
#   2. Weight sets the worker pulls on demand from the server (T29 Install
#      extracts {name}.param + {name}.bin here).
# It MUST be writable by the runtime user so pull-on-demand installs can write
# pulled weights — hence the chown to the unprivileged `app` user below.
#
# To bake a model into the image, an operator (in a derived image or a build
# layer) copies its realesrgan weight files into this dir and lists the model
# names in PREINSTALLED_MODELS:
#
#   COPY realesr-animevideov3.param /models/realtime.param
#   COPY realesr-animevideov3.bin   /models/realtime.bin
#   ENV PREINSTALLED_MODELS="realtime"
#
# (file names must be {name}.param / {name}.bin — the same {name} used in
# PREINSTALLED_MODELS, since NewManager looks them up by name in MODELS_DIR.)
RUN mkdir -p /models

# ── Worker configuration (environment variables) ─────────────────────────────
# This image is SELF-CONTAINED: SERVER_URL and API_KEY are baked in at build time
# (below), and with server-side WORKER_SELF_ENROLL the worker enrolls with NO
# token. So a GPU operator runs it with ZERO configuration:
#
#   docker load < ae-upscaler-worker.tar.gz
#   docker run -d --restart unless-stopped --gpus all ae-upscaler-worker:handover
#
# Required (baked — override only for a different deployment):
#   SERVER_URL            base URL of the upscaler control plane. Baked default:
#                         https://ext.animeenigma.org (the public edge).
# Enroll credential (baked):
#   API_KEY               edge API key sent as X-API-Key. Baked at build via the
#                         API_KEY build-arg (NOT stored in this Dockerfile). With
#                         WORKER_SELF_ENROLL on the server, the API key alone
#                         enrolls the worker — no ENROLL_TOKEN needed.
#   ENROLL_TOKEN          OPTIONAL single-use token (legacy path). Leave unset to
#                         self-enroll; set it only against a server without
#                         self-enroll enabled.
# Model provisioning (NOTE: the legacy `MODEL` env var was REMOVED — the worker
# is model-agnostic; the server provisions models per-job):
#   PREINSTALLED_MODELS   comma-separated realesrgan model names whose weight
#                         files are baked into MODELS_DIR (default empty → the
#                         worker boots with ONLY the built-in `mock` model and
#                         pulls any other model on demand). Example: "realtime,best-quality".
#   MODELS_DIR            weights directory (default /models). Baked + pulled
#                         weights share this dir; must be writable by the runtime user.
# Optional:
#   MODE                  worker mode (default "batch")
#   SCALE                 default integer upscale factor (default 2; per-job scale overrides)
#   WORK_DIR              scratch dir for per-segment staging (default the OS temp dir)
#   HEARTBEAT_INTERVAL    per-segment telemetry heartbeat cadence (Go duration, default 5s)
#   METRICS_INTERVAL      per-segment telemetry metrics cadence  (Go duration, default 10s)
ENV PREINSTALLED_MODELS=""
ENV MODELS_DIR="/models"

# Baked self-contained config. SERVER_URL is public (safe to default here).
# API_KEY and IMAGE_VERSION come from build-args so NO secret is committed to
# this Dockerfile; the operator's image carries the value baked at build time.
# Build:  docker build --build-arg API_KEY=<edge key> -t ae-upscaler-worker:handover worker/
ENV SERVER_URL="https://ext.animeenigma.org"
ARG API_KEY=""
ENV API_KEY="${API_KEY}"
ARG IMAGE_VERSION="selfenroll"
ENV IMAGE_VERSION="${IMAGE_VERSION}"

RUN groupadd -r app && useradd -r -g app app
COPY --from=build /worker /worker
RUN chown app:app /worker \
    && chown -R app:app /models

USER app
ENTRYPOINT ["/worker"]
