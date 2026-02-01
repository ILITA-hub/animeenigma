# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app

# Install build dependencies
RUN apk add --no-cache git

# Copy libs go.mod files for dependency caching
COPY libs/logger/go.mod libs/logger/go.sum ./libs/logger/
COPY libs/errors/go.mod libs/errors/go.sum ./libs/errors/
COPY libs/cache/go.mod libs/cache/go.sum ./libs/cache/
COPY libs/database/go.mod libs/database/go.sum ./libs/database/
COPY libs/httputil/go.mod libs/httputil/go.sum ./libs/httputil/
COPY services/scheduler/go.mod services/scheduler/go.sum ./services/scheduler/

# Download dependencies without go.work (use replace directives in go.mod)
WORKDIR /app/services/scheduler
RUN go mod download

# Copy source code
WORKDIR /app
COPY libs/ ./libs/
COPY services/scheduler/ ./services/scheduler/

# Build
WORKDIR /app/services/scheduler
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /scheduler-api ./cmd/scheduler-api

# Runtime stage
FROM alpine:3.19

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app

COPY --from=builder /scheduler-api .

EXPOSE 8085

CMD ["./scheduler-api"]
