# syntax=docker/dockerfile:1
ARG ALPINE_VERSION=3.20
ARG GO_VERSION=1.26
ARG UV_VERSION=0.5.0
ARG DEPCRUISER_VERSION=17

# ── uv binary stage: FROM expands the global ARG; COPY --from=<image:${VAR}>
# does not (buildkit rejects variable expansion in --from), so alias it here. ──
FROM ghcr.io/astral-sh/uv:${UV_VERSION} AS uv-bin

# ── Stage 1: Go cross-compile (musl, fully static; runs on builder native arch)
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS go-builder
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

# ── Stage 2: Final runtime image (Alpine) ────────────────────────────────────
FROM alpine:${ALPINE_VERSION}
ARG DEPCRUISER_VERSION

# Runtime tools for all archfit checks:
#   git           — repo root + history
#   python3 + uv  — Python import analysis (grimp, installed below)
#   nodejs + npm  — TS analysis (dependency-cruiser) and ast-grep (sg)
#   libstdc++     — required by the Node native runtime on musl
RUN apk add --no-cache ca-certificates git nodejs npm python3 libstdc++

# uv (static musl binary): archfit runs `uv run --with grimp` for Python import
# analysis, which resolves grimp at runtime (cached after first use). Pre-baking
# grimp into the system Python is bypassed by that path and trips PEP 668 on
# Alpine's externally-managed interpreter, so it is intentionally omitted.
COPY --from=uv-bin /uv /uvx /usr/local/bin/
ENV UV_PYTHON_DOWNLOADS=never \
    UV_SYSTEM_PYTHON=1

# dependency-cruiser (TS import analysis) — pure JS, installs cleanly on musl.
RUN npm install -g dependency-cruiser@${DEPCRUISER_VERSION} \
    && npm cache clean --force

# ast-grep (the `sg` binary archfit invokes for structural patterns). WIP on
# Alpine: @ast-grep/cli has no resolvable musl native binary and it is not yet in
# apk. Best-effort for now; structural pattern checks degrade to "absent" without
# it. TODO(v0.1.1): wire a musl ast-grep (apk edge or pinned static binary).
RUN npm install -g @ast-grep/cli 2>/dev/null || echo "ast-grep (musl) install skipped — WIP"

# archfit binary (static).
COPY --from=go-builder /out/archfit /usr/local/bin/archfit

# Go toolchain: the Go extractor uses go/packages, which shells out to `go list`,
# so analyzing a Go target needs the SDK at runtime. golang:*-alpine is musl, so
# the toolchain runs on this Alpine base. GOTOOLCHAIN=local pins to it; GOCACHE/
# GOPATH point at world-writable /tmp so the non-root user can run `go list`.
COPY --from=go-builder /usr/local/go /usr/local/go
ENV PATH="/usr/local/go/bin:${PATH}" \
    GOTOOLCHAIN=local \
    GOCACHE=/tmp/.gocache \
    GOPATH=/tmp/.gopath

# Non-root user.
RUN addgroup -S archfit && adduser -S -G archfit archfit
USER archfit

LABEL org.opencontainers.image.title="archfit" \
      org.opencontainers.image.description="Architecture fitness checker for Go, TypeScript, and Python repositories" \
      org.opencontainers.image.source="https://github.com/alexei-led/archfit" \
      org.opencontainers.image.licenses="MIT"

ENTRYPOINT ["/usr/local/bin/archfit"]
