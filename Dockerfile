# ==========================================
# TempMail Ultra-Light — Multi-stage Dockerfile
# Final image: ~20MB (Alpine + single Go binary)
# ==========================================

# --- Stage 1: Build ---
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git gcc musl-dev

WORKDIR /app

# Copy go mod files first for better caching
COPY receiver/go.mod receiver/go.sum ./
RUN go mod download

# Copy source code
COPY receiver/ ./

# Build static binary (CGO disabled for static linking)
# Note: go-sqlite3 needs CGO, so we keep it enabled
RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w -extldflags=-static" -o /tempmail .

# --- Stage 2: Runtime ---
FROM alpine:3.20

RUN apk add --no-cache ca-certificates sqlite-libs tzdata
ENV TZ=Asia/Jakarta

WORKDIR /app

# Copy single binary
COPY --from=builder /tempmail /app/tempmail

# Create data directory for SQLite + attachments
RUN mkdir -p /app/data/attachments

VOLUME ["/app/data"]

EXPOSE 25 8080

# Health check
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
  CMD wget -qO- http://localhost:8080/health || exit 1

ENTRYPOINT ["/app/tempmail"]
