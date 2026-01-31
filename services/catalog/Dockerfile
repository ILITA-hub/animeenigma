# Build stage
FROM golang:1.24-alpine AS builder

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
COPY services/auth/go.mod services/auth/go.sum* ./services/auth/
COPY services/catalog/go.mod services/catalog/go.sum* ./services/catalog/
COPY services/streaming/go.mod services/streaming/go.sum* ./services/streaming/
COPY services/player/go.mod services/player/go.sum* ./services/player/
COPY services/rooms/go.mod services/rooms/go.sum* ./services/rooms/
COPY services/scheduler/go.mod services/scheduler/go.sum* ./services/scheduler/
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/

RUN cd services/catalog && go mod download

# Copy source
COPY libs/ ./libs/
COPY services/catalog/ ./services/catalog/

# Build
RUN cd services/catalog && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /catalog-api ./cmd/catalog-api

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /catalog-api .
COPY services/catalog/migrations ./migrations

EXPOSE 8081

CMD ["./catalog-api"]
