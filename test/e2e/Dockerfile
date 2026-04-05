# go-guardian e2e testing clean-room container
# Ubuntu 24.04 LTS with Go, Node.js, Claude Code, and debug tools.
# Usage: see run.sh

FROM ubuntu:24.04

ENV DEBIAN_FRONTEND=noninteractive

# ── System layer: debug tools + build essentials ─────────────────────────────
RUN apt-get update && apt-get install -y --no-install-recommends \
    git \
    curl \
    ca-certificates \
    jq \
    ripgrep \
    sqlite3 \
    strace \
    ltrace \
    htop \
    vim \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

# ── Go installation ─────────────────────────────────────────────────────────
ARG GO_VERSION=1.24.2
RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
    | tar -C /usr/local -xzf - \
    && ln -s /usr/local/go/bin/go /usr/local/bin/go \
    && ln -s /usr/local/go/bin/gofmt /usr/local/bin/gofmt
ENV GOPATH=/root/go
ENV PATH="${GOPATH}/bin:${PATH}"

# ── Node.js (required for Claude Code) ──────────────────────────────────────
RUN curl -fsSL https://deb.nodesource.com/setup_22.x | bash - \
    && apt-get install -y nodejs \
    && rm -rf /var/lib/apt/lists/*

# ── Claude Code ─────────────────────────────────────────────────────────────
RUN curl -fsSL https://claude.ai/install.sh | bash \
    && CLAUDE_BIN=$(find /root -name "claude" -type f -executable 2>/dev/null | head -1) \
    && if [ -n "$CLAUDE_BIN" ]; then ln -sf "$CLAUDE_BIN" /usr/local/bin/claude; fi
# Fallback: add common install locations to PATH.
ENV PATH="/root/.claude/bin:/root/.local/bin:${PATH}"

# ── Agent Gateway (native binary) ──────────────────────────────────────────
ARG AGENTGATEWAY_VERSION=1.0.1
RUN curl -fsSL "https://github.com/agentgateway/agentgateway/releases/download/v${AGENTGATEWAY_VERSION}/agentgateway-linux-amd64" \
    -o /usr/local/bin/agentgateway \
    && chmod +x /usr/local/bin/agentgateway

# ── Copy test fixture ───────────────────────────────────────────────────────
COPY fixture/ /workspace/
WORKDIR /workspace

# Pre-download Go dependencies so tests don't wait on network.
RUN go mod download

# ── Copy entrypoint ─────────────────────────────────────────────────────────
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

ENTRYPOINT ["/entrypoint.sh"]
