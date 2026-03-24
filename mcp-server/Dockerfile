# Build stage — runs on the build machine's native platform for speed.
# Cross-compilation is handled by GOOS/GOARCH, not by emulation.
FROM --platform=${BUILDPLATFORM} golang:1.26 AS builder

ARG TARGETOS
ARG TARGETARCH
ENV CGO_ENABLED=0

WORKDIR /build

# Download dependencies first — this layer is cached independently of source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

# Build the binary for the target platform.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -trimpath -o go-guardian-mcp .

# Runtime stage — distroless/static has no shell, no libc, no package manager.
# Safe for CGO_ENABLED=0 binaries; nonroot runs as uid/gid 65532.
FROM gcr.io/distroless/static:nonroot

LABEL org.opencontainers.image.source="https://github.com/go-guardian/go-guardian"
LABEL org.opencontainers.image.description="go-guardian MCP server — self-learning Go development assistant"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /build/go-guardian-mcp /go-guardian-mcp

# The database is written to /data — mount a volume here in production.
# The nonroot user (65532) must have write access to the mounted volume.
VOLUME ["/data"]

USER 65532:65532

ENTRYPOINT ["/go-guardian-mcp"]
CMD ["--db", "/data/guardian.db"]
