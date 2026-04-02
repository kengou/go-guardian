-- Seed: trained Dockerfile patterns DOCKER-1 through DOCKER-15
-- Source: Kubernetes, Prometheus, Grafana, Istio, Linkerd2, Traefik, ArgoCD, etcd, CoreDNS,
--         containerd, Podman, cert-manager, Cilium, Calico, Cosign, StackRox, Vault, Flux2

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-1',
    'Multi-Stage Build: single-stage Dockerfiles ship build tools (Go toolchain, gcc, make) in the final image. 900MB+ images with unnecessary attack surface. Use builder stage for compilation, copy only the binary to a minimal final image.',
    'FROM golang:1.22
WORKDIR /app
COPY . .
RUN go build -o /app/server .
EXPOSE 8080
CMD ["/app/server"]',
    'FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags=''-s -w'' -o /server .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /server /server
ENTRYPOINT ["/server"]',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-2',
    'Dependency Layer Caching: COPY . . before go build busts the Docker cache on every source file change, forcing full dependency re-download. Copy go.mod/go.sum first and run go mod download as a separate layer.',
    'FROM golang:1.22 AS builder
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o /server .',
    'FROM golang:1.22 AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /server .',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-3',
    'Non-Root Execution: containers running as root (UID 0) can escape to the host in container breakout attacks. Use nonroot user in distroless or create a dedicated user. Kubernetes Pod Security Standards (restricted) enforce this.',
    'FROM gcr.io/distroless/static-debian12
COPY --from=builder /server /server
# runs as root by default
ENTRYPOINT ["/server"]',
    'FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /server /server
USER 65532:65532
ENTRYPOINT ["/server"]',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-4',
    'Static Binary for Scratch/Distroless: CGO_ENABLED=1 produces dynamically linked binaries requiring glibc. These crash on scratch or distroless images with "not found" errors. Disable CGO and strip debug symbols for minimal static binaries.',
    'FROM golang:1.22 AS builder
RUN go build -o /server .
FROM scratch
COPY --from=builder /server /server
# crashes: exec /server: no such file or directory (missing glibc)',
    'FROM golang:1.22 AS builder
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags=''-s -w -extldflags "-static"'' \
    -o /server .
FROM scratch
COPY --from=builder /server /server',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-5',
    'Pin Base Image by Digest: tag-only references (golang:1.22) can silently change when the tag is republished. Pin by digest for reproducible builds. Critical for supply chain security (SLSA, Cosign verification).',
    'FROM golang:1.22
FROM gcr.io/distroless/static-debian12',
    'FROM golang:1.22.5@sha256:abc123... AS builder
FROM gcr.io/distroless/static-debian12@sha256:def456...',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-6',
    'Minimal Final Image: using the build image (golang:1.22, 900MB+) as the runtime image ships compilers, shells, package managers — massive attack surface. Use distroless (2MB, no shell) or scratch (0MB) for Go static binaries.',
    'FROM golang:1.22
WORKDIR /app
COPY . .
RUN go build -o server .
CMD ["./server"]
# final image: ~900MB, includes gcc, shell, package manager',
    '# Option 1: distroless (recommended — has CA certs, tzdata)
FROM gcr.io/distroless/static-debian12:nonroot
# Option 2: scratch (absolute minimum — add CA certs manually)
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
# final image: 2-10MB',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-7',
    'No Secrets in Build Layers: ARG/ENV values and COPY-ed secret files are permanently embedded in image layers. Anyone with docker history or image pull access can extract them. Use runtime secrets (K8s Secrets, Vault, mounted volumes).',
    'FROM golang:1.22
ARG DB_PASSWORD
ENV DB_PASSWORD=${DB_PASSWORD}
COPY .env /app/.env
COPY credentials.json /app/credentials.json
RUN go build -o /server .',
    'FROM golang:1.22 AS builder
# no secrets at build time
RUN go build -o /server .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /server /server
# secrets injected at runtime via K8s Secrets, Vault, or env vars
ENTRYPOINT ["/server"]',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-8',
    'Exec Form for Signal Handling: shell form CMD wraps the process in sh -c, which swallows SIGTERM. The Go process never receives the signal, preventing graceful shutdown. Always use exec form (JSON array) for ENTRYPOINT/CMD.',
    'CMD ./server
# or: CMD "server --port 8080"
# sh -c wraps the process, SIGTERM goes to sh, not server
# server never calls shutdown handlers, connections dropped',
    'ENTRYPOINT ["/server"]
# or with args:
ENTRYPOINT ["/server", "--port", "8080"]
# Go process is PID 1, receives SIGTERM directly
# server.Shutdown(ctx) called, connections drain gracefully',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-9',
    'Comprehensive .dockerignore: COPY . . without .dockerignore sends .git (100MB+), vendor, test fixtures, IDE configs, and CI artifacts to the build context. Slows builds and may leak secrets.',
    '# no .dockerignore file
COPY . .',
    '# .dockerignore
.git
.github
.vscode
.idea
vendor
*_test.go
**/*_test.go
docs
*.md
.env
.env.*
Makefile
hack/',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-10',
    'Build Cache Mounts: go mod download runs every build even when dependencies have not changed. Cache mounts persist the Go module and build caches across builds, cutting rebuild time by 60-80%.',
    'FROM golang:1.22 AS builder
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN go build -o /server .',
    'FROM golang:1.22 AS builder
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -o /server .',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-11',
    'OCI Image Labels: images without metadata are untraceable — no way to map a running container back to source code, commit, or build. Use OCI standard labels (org.opencontainers.image.*) set via build args.',
    'FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /server /server
# no labels — impossible to trace image provenance',
    'ARG VERSION
ARG VCS_REF
ARG BUILD_DATE
LABEL org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${VCS_REF}" \
      org.opencontainers.image.created="${BUILD_DATE}" \
      org.opencontainers.image.source="https://github.com/org/repo" \
      org.opencontainers.image.title="my-service"',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-12',
    'Version Injection via ldflags: hardcoded version strings in source code require code changes for each release. Inject version, commit, and build date via -ldflags at build time for zero-touch release builds.',
    'FROM golang:1.22 AS builder
RUN go build -o /server .
# version.go: var Version = "1.2.3" — must edit for each release',
    'FROM golang:1.22 AS builder
ARG VERSION=dev
ARG VCS_REF=unknown
RUN CGO_ENABLED=0 go build \
    -ldflags="-s -w \
      -X main.version=${VERSION} \
      -X main.commit=${VCS_REF} \
      -X main.buildDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    -o /server .',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-13',
    'Copy Binary Only from Builder: copying the entire build directory from the builder stage includes source code, test files, and intermediate objects in the final image. Copy only the compiled binary.',
    'FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /app/ /app/
# includes: source code, go.mod, test files, docs',
    'FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /server /server
# only: compiled binary (5-20MB)',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-14',
    'CA Certificates in Scratch Images: scratch images have no CA certificate bundle. HTTPS clients fail with x509: certificate signed by unknown authority. Copy certs from the builder stage or use distroless which includes them.',
    'FROM scratch
COPY --from=builder /server /server
# HTTP calls fail: x509 certificate signed by unknown authority',
    '# Option 1: copy certs from builder
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /server /server
# Option 2: use distroless (includes certs + tzdata)
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /server /server',
    'trained',
    'docker'
);

INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'DOCKER-15',
    'Timezone Data in Minimal Images: scratch and some distroless variants lack timezone data. time.LoadLocation() panics or returns errors. Either embed tzdata in the Go binary or copy it from the builder.',
    'FROM scratch
COPY --from=builder /server /server
# time.LoadLocation("Europe/Berlin") panics: unknown time zone',
    '# Option 1: embed in Go binary (Go 1.15+)
import _ "time/tzdata"
# Option 2: copy from builder
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo
# Option 3: use distroless (includes tzdata)
FROM gcr.io/distroless/static-debian12:nonroot',
    'trained',
    'docker'
);
