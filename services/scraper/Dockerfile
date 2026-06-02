# Build stage
FROM golang:1.24-alpine AS builder

WORKDIR /app

RUN apk add --no-cache git

# Copy go.work and all module files. Every workspace member must be
# reachable from inside the build context because `go work sync` resolves
# the entire workspace graph; missing members fail the build.
#
# REVIEW.md WR-06: many of the lib go.mods below are not directly required
# by services/scraper/go.mod (which only needs errors/httputil/logger/metrics
# today), but go.work requires every workspace member's go.mod to exist at
# build time. Removing them would diverge from the other services'
# Dockerfiles — keep the full set and document the reason instead.
COPY go.work go.work.sum ./
COPY libs/logger/go.mod libs/logger/go.sum* ./libs/logger/
COPY libs/tracing/go.mod libs/tracing/go.sum* ./libs/tracing/
COPY libs/errors/go.mod libs/errors/go.sum* ./libs/errors/
COPY libs/cache/go.mod libs/cache/go.sum* ./libs/cache/
COPY libs/database/go.mod libs/database/go.sum* ./libs/database/
COPY libs/authz/go.mod libs/authz/go.sum* ./libs/authz/
COPY libs/httputil/go.mod libs/httputil/go.sum* ./libs/httputil/
COPY libs/idmapping/go.mod libs/idmapping/go.sum* ./libs/idmapping/
COPY libs/pagination/go.mod libs/pagination/go.sum* ./libs/pagination/
COPY libs/animeparser/go.mod libs/animeparser/go.sum* ./libs/animeparser/
COPY libs/videoutils/go.mod libs/videoutils/go.sum* ./libs/videoutils/
COPY libs/metrics/go.mod libs/metrics/go.sum* ./libs/metrics/
COPY libs/streamprobe/go.mod libs/streamprobe/go.sum* ./libs/streamprobe/
COPY services/auth/go.mod services/auth/go.sum* ./services/auth/
COPY services/catalog/go.mod services/catalog/go.sum* ./services/catalog/
COPY services/streaming/go.mod services/streaming/go.sum* ./services/streaming/
COPY services/player/go.mod services/player/go.sum* ./services/player/
COPY services/rooms/go.mod services/rooms/go.sum* ./services/rooms/
COPY services/scheduler/go.mod services/scheduler/go.sum* ./services/scheduler/
COPY services/gateway/go.mod services/gateway/go.sum* ./services/gateway/
COPY services/themes/go.mod services/themes/go.sum* ./services/themes/
COPY services/notifications/go.mod services/notifications/go.sum* ./services/notifications/
COPY services/maintenance/go.mod services/maintenance/go.sum* ./services/maintenance/
COPY services/scraper/go.mod services/scraper/go.sum* ./services/scraper/
COPY services/library/go.mod services/library/go.sum* ./services/library/
COPY services/watch-together/go.mod services/watch-together/go.sum* ./services/watch-together/
COPY services/analytics/go.mod services/analytics/go.sum* ./services/analytics/

RUN cd services/scraper && go mod download

# Copy source
COPY libs/ ./libs/
COPY services/scraper/ ./services/scraper/

# Build
RUN cd services/scraper && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /scraper-api ./cmd/scraper-api

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /scraper-api .

EXPOSE 8088

CMD ["./scraper-api"]
