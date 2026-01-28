# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go.mod files for dependency caching
COPY go.work go.work.sum ./
COPY libs/logger/go.mod libs/logger/go.sum ./libs/logger/
COPY libs/errors/go.mod libs/errors/go.sum ./libs/errors/
COPY libs/database/go.mod libs/database/go.sum ./libs/database/
COPY libs/authz/go.mod libs/authz/go.sum ./libs/authz/
COPY libs/httputil/go.mod libs/httputil/go.sum ./libs/httputil/
COPY services/player/go.mod services/player/go.sum ./services/player/

# Download dependencies
RUN cd services/player && go mod download

# Copy source code
COPY libs/ ./libs/
COPY services/player/ ./services/player/

# Build
RUN cd services/player && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /player-api ./cmd/player-api

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /player-api .

EXPOSE 8082

CMD ["./player-api"]
