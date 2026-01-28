# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy go.mod files for dependency caching
COPY go.work go.work.sum ./
COPY libs/logger/go.mod libs/logger/go.sum ./libs/logger/
COPY libs/errors/go.mod libs/errors/go.sum ./libs/errors/
COPY libs/authz/go.mod libs/authz/go.sum ./libs/authz/
COPY libs/httputil/go.mod libs/httputil/go.sum ./libs/httputil/
COPY services/gateway/go.mod services/gateway/go.sum ./services/gateway/

# Download dependencies
RUN cd services/gateway && go mod download

# Copy source code
COPY libs/ ./libs/
COPY services/gateway/ ./services/gateway/

# Build
RUN cd services/gateway && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /gateway-api ./cmd/gateway-api

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /gateway-api .

EXPOSE 8000

CMD ["./gateway-api"]
