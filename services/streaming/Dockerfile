# Build stage
FROM golang:1.22-alpine AS builder

WORKDIR /app
RUN apk add --no-cache git

COPY go.work go.work.sum ./
COPY libs/ ./libs/
COPY services/streaming/go.mod services/streaming/go.sum ./services/streaming/

RUN cd services/streaming && go mod download

COPY services/streaming/ ./services/streaming/

RUN cd services/streaming && CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /streaming-api ./cmd/streaming-api

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /streaming-api .
EXPOSE 8082
CMD ["./streaming-api"]
