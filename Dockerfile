# syntax=docker/dockerfile:1
ARG GO_VERSION=1.26
ARG UV_VERSION=0.5.0
ARG DEPCRUISER_VERSION=17
# 0.44.0 is the version --inline-rules (used by the syntax-facts adapter) was
# validated against; matches the local toolchain. Bump only after re-validating.
ARG ASTGREP_VERSION=0.44.0

# ── uv binary stage: FROM expands the global ARG; COPY --from=<image:${VAR}>
# does not (buildkit rejects variable expansion in --from), so alias it here. ──
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-bin

# ── Stage 1: Go build (CGO_ENABLED=0, fully static binary) ───────────────────
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-bookworm AS go-builder
ARG TARGETOS TARGETARCH VERSION COMMIT DATE
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# GOWORK=off: go.work references testdata modules that .dockerignore excludes;
# the binary builds from the main module only.
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} GOWORK=off \
    go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/archfit ./cmd/archfit

# ── Stage 2: Final runtime image (Debian bookworm-slim, glibc) ───────────────
FROM debian:bookworm-slim
ARG DEPCRUISER_VERSION
ARG ASTGREP_VERSION

# Install runtime tools + Node 24 (NodeSource) in one layer.
# curl and gnupg are only needed to add the NodeSource repo; purge them after.
SHELL ["/bin/bash", "-o", "pipefail", "-c"]
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    git \
    curl \
    gnupg \
    python3 \
    && curl -fsSL https://deb.nodesource.com/setup_24.x | bash - \
    && apt-get install -y --no-install-recommends nodejs \
    && apt-get purge -y --auto-remove curl gnupg \
    && rm -rf /var/lib/apt/lists/*

# util-linux ships /usr/bin/sg (setgid helper); @ast-grep/cli postinstall writes
# the same path and fails EEXIST. Remove it first so ast-grep owns the slot.
RUN rm -f /usr/bin/sg

# dependency-cruiser (TS import analysis) + ast-grep (structural patterns).
# @ast-grep/cli resolves a glibc native binary — works on bookworm-slim.
RUN npm install -g \
    dependency-cruiser@${DEPCRUISER_VERSION} \
    @ast-grep/cli@${ASTGREP_VERSION} \
    && npm cache clean --force

# uv (static binary): archfit runs `uv run --with grimp` for Python import
# analysis, resolving grimp at runtime (cached after first use).
COPY --from=uv-bin /uv /uvx /usr/local/bin/
ENV UV_PYTHON_DOWNLOADS=never \
    UV_SYSTEM_PYTHON=1

# archfit binary (static, CGO_ENABLED=0).
COPY --from=go-builder /out/archfit /usr/local/bin/archfit

# Go toolchain: the Go extractor uses go/packages, which shells out to `go list`,
# so analyzing a Go target requires the SDK at runtime. golang:*-bookworm uses
# glibc, matching this debian-slim base. GOTOOLCHAIN=local pins to it; GOCACHE/
# GOPATH point at world-writable /tmp so the non-root user can run `go list`.
COPY --from=go-builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" \
    GOTOOLCHAIN=local \
    GOCACHE=/tmp/.gocache \
    GOPATH=/tmp/.gopath

# Non-root user.
RUN groupadd --system archfit && useradd --system --gid archfit archfit
USER archfit

LABEL org.opencontainers.image.title="archfit" \
      org.opencontainers.image.description="Architecture fitness checker for Go, TypeScript, and Python repositories" \
      org.opencontainers.image.source="https://github.com/alexei-led/archfit" \
      org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/archfit"]
