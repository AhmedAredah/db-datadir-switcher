# DBSwitcher Dockerfile
# Multi-stage build for minimal final image

FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache \
    git \
    make \
    gcc \
    musl-dev \
    pkgconfig \
    gtk+3.0-dev \
    libayatana-appindicator3-dev

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=1 GOOS=linux go build \
    -ldflags "-s -w -extldflags=-static" \
    -a -installsuffix cgo \
    -o dbswitcher main.go

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    gtk+3.0 \
    libayatana-appindicator3

# Create non-root user
RUN addgroup -g 1000 dbswitcher && \
    adduser -D -s /bin/sh -u 1000 -G dbswitcher dbswitcher

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/dbswitcher .

# Change ownership
RUN chown dbswitcher:dbswitcher /app/dbswitcher

# Switch to non-root user
USER dbswitcher

# Set the entrypoint
ENTRYPOINT ["./dbswitcher"]

# Default command
CMD ["help"]

# Labels
LABEL maintainer="Ahmed Aredah <Ahmed.Aredah@gmail.com>"
LABEL description="DBSwitcher - MariaDB Configuration Manager"
LABEL version="0.0.1"