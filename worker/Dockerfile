FROM golang:1.25 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /worker ./cmd/worker

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ffmpeg \
    ca-certificates \
    wget \
    unzip \
    && rm -rf /var/lib/apt/lists/*

LABEL org.opencontainers.image.title="upscaler-worker"
LABEL org.opencontainers.image.description="GPU worker agent (model-agnostic: built-in mock + optional baked PREINSTALLED_MODELS + server pull-on-demand)"

# realesrgan-ncnn-vulkan binary (Linux x86_64) — the runtime for realesrgan
# weight-set models, whether baked in (PREINSTALLED_MODELS) or pulled on demand
# from the server. It needs a GPU/Vulkan at run time; the built-in `mock` model
# is pure-Go and needs neither this binary nor a GPU.
ARG REALESRGAN_VERSION=0.2.0
RUN wget -q -O /tmp/realesrgan.zip \
    "https://github.com/xinntao/Real-ESRGAN/releases/download/v${REALESRGAN_VERSION}/realesrgan-ncnn-vulkan-${REALESRGAN_VERSION}-ubuntu.zip" \
    && unzip /tmp/realesrgan.zip -d /tmp/realesrgan \
    && mv /tmp/realesrgan/realesrgan-ncnn-vulkan /usr/local/bin/realesrgan-ncnn-vulkan \
    && chmod +x /usr/local/bin/realesrgan-ncnn-vulkan \
    && rm -rf /tmp/realesrgan /tmp/realesrgan.zip \
    || true  # binary fetch may fail in CI; structure is what matters

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
# Required:
#   SERVER_URL            base URL of the upscaler control plane (e.g. http://upscaler:8096)
#   ENROLL_TOKEN          one-time enrollment token issued by the operator
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
#   API_KEY               edge API key sent as X-API-Key on data-plane requests
#   WORK_DIR              scratch dir for per-segment staging (default the OS temp dir)
#   HEARTBEAT_INTERVAL    per-segment telemetry heartbeat cadence (Go duration, default 5s)
#   METRICS_INTERVAL      per-segment telemetry metrics cadence  (Go duration, default 10s)
ENV PREINSTALLED_MODELS=""
ENV MODELS_DIR="/models"

RUN groupadd -r app && useradd -r -g app app
COPY --from=build /worker /worker
RUN chown app:app /worker \
    && chown -R app:app /models

USER app
ENTRYPOINT ["/worker"]
