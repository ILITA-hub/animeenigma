# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go.mod files for dependency caching
COPY go.work go.work.sum ./
COPY libs/logger/go.mod libs/logger/go.sum ./libs/logger/
COPY libs/errors/go.mod libs/errors/go.sum ./libs/errors/
COPY libs/cache/go.mod libs/cache/go.sum ./libs/cache/
COPY libs/database/go.mod libs/database/go.sum ./libs/database/
COPY libs/authz/go.mod libs/authz/go.sum ./libs/authz/
COPY libs/httputil/go.mod libs/httputil/go.sum ./libs/httputil/
COPY services/rooms/go.mod services/rooms/go.sum ./services/rooms/

# Download dependencies
RUN cd services/rooms && go mod download

# Copy source code
COPY libs/ ./libs/
COPY services/rooms/ ./services/rooms/

# Build
RUN cd services/rooms && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /rooms-api ./cmd/rooms-api

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /rooms-api .

EXPOSE 8083

CMD ["./rooms-api"]
