# syntax=docker/dockerfile:1

# ── Stage 1: builder ─────────────────────────────────────────────────────────
FROM golang:1.26 AS builder

WORKDIR /src

# Download deps in a separate layer (cached unless go.mod/go.sum change).
COPY go.mod go.sum ./
RUN go mod download

# Copy source (web/ must be present so go:embed can include templates + static).
COPY . .

# Build a fully-static binary: no CGO, trimmed paths, stripped debug info.
RUN CGO_ENABLED=0 GOOS=linux \
    go build \
      -trimpath \
      -ldflags="-s -w" \
      -o /out/kopia-browser \
      ./cmd/kopia-browser

# Create the cache directory in the builder stage so we can COPY it with the
# right ownership into the distroless image (which has no shell / mkdir).
RUN mkdir -p /out/cache

# ── Stage 2: runtime ─────────────────────────────────────────────────────────
# gcr.io/distroless/static-debian12:nonroot — includes CA certs, no shell,
# runs as uid 65532 ("nonroot") by default.
FROM gcr.io/distroless/static-debian12:nonroot

# Copy the static binary.
COPY --from=builder /out/kopia-browser /usr/local/bin/kopia-browser

# Copy the pre-created (empty) cache directory with correct ownership.
# The kopia manager resolves KOPIA_CACHE_DIR to an absolute path and calls
# os.MkdirAll on it, so the dir must be writable by the nonroot uid (65532).
COPY --from=builder --chown=65532:65532 /out/cache /var/cache/kopia-browser

# Override the default relative KOPIA_CACHE_DIR so it resolves to a writable
# absolute path inside the container. Override at runtime to mount a volume
# elsewhere (e.g. -e KOPIA_CACHE_DIR=/data).
ENV KOPIA_CACHE_DIR=/var/cache/kopia-browser

# Default listen address (override with LISTEN_ADDR env var).
EXPOSE 8080

# Run as nonroot (uid 65532) — distroless default, stated explicitly.
USER nonroot

# Declare the cache dir as a volume so orchestrators can mount persistent
# storage here (preserves per-namespace repo config + stats-cache.json across
# restarts). Mounting is optional; the image works without it.
VOLUME ["/var/cache/kopia-browser"]

# Health probe: GET /healthz → "ok" (200). Wire this as a liveness probe in
# k8s; distroless has no shell/curl so an in-image HEALTHCHECK is not feasible.

ENTRYPOINT ["/usr/local/bin/kopia-browser"]
