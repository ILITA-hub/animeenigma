# Multi-stage build for Vue 3 application

# Stage 1: Build stage
FROM oven/bun:1 AS builder

WORKDIR /app

# Build arguments for environment variables
ARG VITE_API_URL="http://localhost:8000/api"
ARG VITE_RAW_PROVIDER_ENABLED="false"
ARG VITE_ANIME18_ENABLED="false"
ARG VITE_GACHA_ADMIN_ONLY="true"

# Set environment variables for build
ENV VITE_API_URL=$VITE_API_URL
ENV VITE_RAW_PROVIDER_ENABLED=$VITE_RAW_PROVIDER_ENABLED
ENV VITE_ANIME18_ENABLED=$VITE_ANIME18_ENABLED
ENV VITE_GACHA_ADMIN_ONLY=$VITE_GACHA_ADMIN_ONLY

# Copy package files
COPY package.json bun.lock ./

# Install dependencies
RUN bun install

# Copy source code
COPY . .

# Build the application
RUN bun run build

# Stage 2: Production stage with nginx
# Alpine's own nginx package (not the nginx:alpine official image): the
# official image ships no brotli module, while Alpine's nginx-mod-http-brotli
# matches its nginx build exactly and auto-loads via /etc/nginx/modules/*.conf.
# Needed for `brotli_static on` — serving the pre-compressed .br twins Vite
# emits (page-load optimization 2026-06-11).
FROM alpine:3.22 AS production

RUN apk add --no-cache nginx nginx-mod-http-brotli \
    # Route logs to the container runtime like the official image does
    && ln -sf /dev/stdout /var/log/nginx/access.log \
    && ln -sf /dev/stderr /var/log/nginx/error.log

# Copy custom nginx config (alpine's nginx.conf includes /etc/nginx/http.d/*)
COPY nginx.conf /etc/nginx/http.d/default.conf

# Copy built assets from builder stage
COPY --from=builder /app/dist /usr/share/nginx/html

# Expose port
EXPOSE 80

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --quiet --tries=1 --spider http://127.0.0.1/health || exit 1

# Start nginx
CMD ["nginx", "-g", "daemon off;"]
