# --- Stage 1: Build the Go application ---
FROM golang:1.24.6-alpine AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -v -o myapp ./cmd/book-server

FROM linuxserver/calibre:latest

# Define an ARG for the plugin version from the jgoguen repository.
# As of now, v2.9.2 is a stable and recent version.
# https://github.com/jgoguen/calibre-kobo-driver/releases/download/v3.7.2/KoboTouchExtended.zip
ARG KOBO_PLUGIN_VERSION="v3.7.2"
ARG KOBO_PLUGIN_URL="https://github.com/jgoguen/calibre-kobo-driver/releases/download/${KOBO_PLUGIN_VERSION}/KoboTouchExtended.zip"

# Re-declare the ARG to make it available in this stage.
ARG KOBO_PLUGIN_URL

# The RUN command now installs the single, correct plugin.
RUN \
    apt-get update && \
    apt-get install -y --no-install-recommends psmisc wget && \
    echo "--- Installing KoboTouchExtended Plugin from jgoguen repository ---" && \
    echo "Downloading from: ${KOBO_PLUGIN_URL}" && \
    # Kill any running Calibre processes to unlock the configuration
    pkill -f calibre || true && \
    # Download and install the one required plugin
    wget -O /tmp/KoboTouchExtended.zip "${KOBO_PLUGIN_URL}" && \
    calibre-customize -a /tmp/KoboTouchExtended.zip && \
    # Clean up all downloaded files and packages
    echo "--- Cleaning up installation files ---" && \
    rm /tmp/KoboTouchExtended.zip && \
    apt-get purge -y --auto-remove psmisc wget && \
    rm -rf /var/lib/apt/lists/*

# --- Application Setup ---
WORKDIR /app
COPY --from=builder /app/myapp .
ENTRYPOINT ["./myapp"]
