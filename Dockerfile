# Both stages run on TARGETPLATFORM. Never copy a BUILDPLATFORM SDK into runtime.
FROM python:3.14.9-slim-bookworm@sha256:945c2676dc2b18e0c1f16879244122c57c0ffede60006bf3df159c507a31bcae AS tools-builder
ARG TARGETARCH
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates build-essential pkg-config libssl-dev git \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src
COPY Dockerfile ./
COPY tools/ tools/
COPY scripts/provision_tools.py scripts/provision-analyzers.sh scripts/
RUN python scripts/provision_tools.py sdks --arch "${TARGETARCH}" \
    && bash scripts/provision-analyzers.sh /opt/archfit /usr/local/bin/python
ENV PATH="/opt/archfit/go/bin:/opt/archfit/node/bin:/opt/archfit/rust/bin:/opt/archfit/bin:${PATH}" \
    GOTOOLCHAIN=local GOWORK=off
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION COMMIT DATE
RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-s -w -X main.version=${VERSION} -X main.commit=${COMMIT} -X main.date=${DATE}" \
    -o /out/archfit ./cmd/archfit

FROM python:3.14.9-slim-bookworm@sha256:945c2676dc2b18e0c1f16879244122c57c0ffede60006bf3df159c507a31bcae
# cc/linker is needed by Cargo/Go target analysis, not just by this image build.
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates git build-essential pkg-config libssl-dev \
    && rm -rf /var/lib/apt/lists/*
COPY --from=tools-builder /opt/archfit /opt/archfit
COPY --from=tools-builder /out/archfit /usr/local/bin/archfit
ENV PATH="/opt/archfit/analyzers/node/node_modules/.bin:/opt/archfit/bin:/opt/archfit/go/bin:/opt/archfit/node/bin:/opt/archfit/rust/bin:${PATH}" \
    ARCHFIT_TOOL_PYTHON=/opt/archfit/python/bin/python \
    HOME=/tmp \
    GOTOOLCHAIN=local \
    GOCACHE=/tmp/archfit-go-build \
    GOPATH=/tmp/archfit-go \
    CARGO_HOME=/tmp/archfit-cargo \
    RUSTUP_HOME=/tmp/archfit-rustup \
    RUST_SRC_PATH=/opt/archfit/rust/lib/rustlib/src/rust/library \
    XDG_CACHE_HOME=/tmp/archfit-cache \
    UV_CACHE_DIR=/tmp/archfit-uv \
    UV_PYTHON_DOWNLOADS=never \
    UV_OFFLINE=1 \
    npm_config_cache=/tmp/archfit-npm \
    npm_config_offline=true \
    PYTHONNOUSERSITE=1
USER 65532:65532
LABEL org.opencontainers.image.title="archfit" \
      org.opencontainers.image.description="Architecture fitness checker for Go, TypeScript, JavaScript, Python and Rust" \
      org.opencontainers.image.source="https://github.com/alexei-led/archfit" \
      org.opencontainers.image.licenses="Apache-2.0"
ENTRYPOINT ["/usr/local/bin/archfit"]
