# Build stage
FROM golang:1.24-alpine AS builder

# Install git and ca-certificates (needed for go mod download and git operations)
RUN apk add --no-cache git ca-certificates

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod ./
COPY go.su[m] ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o review-agent cmd/agent/main.go

# Runtime stage
FROM alpine:latest

# Install git (needed for repository cloning) and ca-certificates (for HTTPS)
RUN apk add --no-cache git ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

# Set working directory
WORKDIR /app

# Copy binary from builder stage
COPY --from=builder /app/review-agent .

# Create directory for workspace operations
RUN mkdir -p /tmp/workspaces && chown -R appuser:appgroup /tmp/workspaces

# Switch to non-root user
USER appuser

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD ./review-agent version || exit 1

# Default command
CMD ["./review-agent", "server"]

# Expose port
EXPOSE 8080