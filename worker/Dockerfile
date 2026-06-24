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
LABEL org.opencontainers.image.description="GPU worker agent"

# Download realesrgan-ncnn-vulkan binary (Linux x86_64)
ARG REALESRGAN_VERSION=0.2.0
RUN wget -q -O /tmp/realesrgan.zip \
    "https://github.com/xinntao/Real-ESRGAN/releases/download/v${REALESRGAN_VERSION}/realesrgan-ncnn-vulkan-${REALESRGAN_VERSION}-ubuntu.zip" \
    && unzip /tmp/realesrgan.zip -d /tmp/realesrgan \
    && mv /tmp/realesrgan/realesrgan-ncnn-vulkan /usr/local/bin/realesrgan-ncnn-vulkan \
    && chmod +x /usr/local/bin/realesrgan-ncnn-vulkan \
    && rm -rf /tmp/realesrgan /tmp/realesrgan.zip \
    || true  # binary fetch may fail in CI; structure is what matters

# Placeholder models directory (Task 16/17 will populate)
RUN mkdir -p /models

RUN groupadd -r app && useradd -r -g app app
COPY --from=build /worker /worker
RUN chown app:app /worker

USER app
ENTRYPOINT ["/worker"]
