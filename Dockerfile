# syntax=docker/dockerfile:1
ARG DEBIAN_VERSION=bookworm-slim
ARG NODE_VERSION=22
ARG UV_VERSION=0.5.0
ARG DEPCRUISER_VERSION=17

# ── Stage 1: Go cross-compile (runs on builder native arch, no QEMU) ─────
FROM --platform=$BUILDPLATFORM golang:1.26-bookworm AS go-builder
ARG TARGETOS TARGETARCH VERSION COMMIT DATE
WORKDIR /src
COPY go.mod go.sum go.work go.work.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/archfit ./cmd/archfit

# ── Stage 2: Python builder — install grimp into system Python ───────────
FROM --platform=$TARGETPLATFORM python:3.12-slim-bookworm AS py-builder
ARG UV_VERSION
COPY --from=ghcr.io/astral-sh/uv:${UV_VERSION} /uv /uvx /bin/
ENV UV_COMPILE_BYTECODE=1 \
    UV_LINK_MODE=copy \
    UV_PYTHON_DOWNLOADS=never \
    UV_SYSTEM_PYTHON=1
RUN --mount=type=cache,target=/root/.cache/uv \
    uv pip install grimp

# ── Stage 3: Final runtime image ─────────────────────────────────────────
FROM debian:${DEBIAN_VERSION}
ARG NODE_VERSION
ARG UV_VERSION
ARG DEPCRUISER_VERSION

# Install Node.js via NodeSource (official, supports arm64) + Python 3.12 runtime
RUN apt-get update && apt-get install -y --no-install-recommends \
        ca-certificates curl git gnupg \
        python3.12 libpython3.12 \
    && mkdir -p /etc/apt/keyrings \
    && curl -fsSL https://deb.nodesource.com/gpgkey/nodesource-repo.gpg.key \
        | gpg --dearmor -o /etc/apt/keyrings/nodesource.gpg \
    && echo "deb [signed-by=/etc/apt/keyrings/nodesource.gpg] https://deb.nodesource.com/node_${NODE_VERSION}.x nodistro main" \
        > /etc/apt/sources.list.d/nodesource.list \
    && apt-get update && apt-get install -y nodejs \
    # Install dependency-cruiser globally
    && npm install -g dependency-cruiser@${DEPCRUISER_VERSION} \
    && npm cache clean --force \
    && rm -rf /var/lib/apt/lists/*

# Copy uv binary (multi-arch manifest — Docker resolves correct arch)
COPY --from=ghcr.io/astral-sh/uv:${UV_VERSION} /uv /uvx /usr/local/bin/

# Copy grimp + deps from Python builder (installed into system Python)
COPY --from=py-builder /usr/local/lib/python3.12/dist-packages /usr/local/lib/python3.12/dist-packages

# Copy archfit binary
COPY --from=go-builder /out/archfit /usr/local/bin/archfit

# Non-root user
RUN groupadd -r archfit && useradd -r -g archfit archfit
USER archfit

# uv should not try to download Python inside the container
ENV UV_PYTHON_DOWNLOADS=never \
    UV_SYSTEM_PYTHON=1

LABEL org.opencontainers.image.title="archfit" \
      org.opencontainers.image.description="Architecture fitness checker for Go, TypeScript, and Python repositories" \
      org.opencontainers.image.source="https://github.com/alexei-led/archfit" \
      org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/archfit"]
