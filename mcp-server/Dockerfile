# Build stage — runs on the build machine's native platform for speed.
# Cross-compilation is handled by GOOS/GOARCH, not by emulation.
FROM --platform=${BUILDPLATFORM} golang:1.26@sha256:b54cbf583d390341599d7bcbc062425c081105cc5ef6d170ced98ef9d047c716 AS builder

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
FROM gcr.io/distroless/static:nonroot@sha256:e3f945647ffb95b5839c07038d64f9811adf17308b9121d8a2b87b6a22a80a39

LABEL org.opencontainers.image.source="https://github.com/kengou/go-guardian"
LABEL org.opencontainers.image.description="go-guardian MCP server — self-learning Go development assistant"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /build/go-guardian-mcp /go-guardian-mcp

# The database is written to /data — mount a volume here in production.
# The nonroot user (65532) must have write access to the mounted volume.
VOLUME ["/data"]

USER 65532:65532

ENTRYPOINT ["/go-guardian-mcp"]
CMD ["--db", "/data/guardian.db"]
