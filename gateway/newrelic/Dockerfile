# New Relic MCP bridge — mcp-remote stdio proxy for agentgateway.
#
# Bridges New Relic's hosted Streamable HTTP MCP server to stdio so that
# agentgateway can consume it as a regular MCP target.
#
# Build:
#   docker build -t newrelic-mcp:latest .
#
# Run (standalone):
#   docker run --rm -e NEW_RELIC_API_KEY=NRAK-... newrelic-mcp:latest
#
# Environment variables (see bridge.mjs for full list):
#   NEW_RELIC_API_KEY   — User API key (required)
#   NEW_RELIC_REGION    — "eu" for EU endpoint (default: US)
#   NEW_RELIC_TAGS      — Comma-separated tool filter tags

# ---------------------------------------------------------------------------
# Build stage — install mcp-remote with npm
# ---------------------------------------------------------------------------
FROM node:24-slim@sha256:06e5c9f86bfa0aaa7163cf37a5eaa8805f16b9acb48e3f85645b09d459fc2a9f AS builder

WORKDIR /app
COPY package.json ./
RUN npm install --omit=dev && npm cache clean --force
COPY bridge.mjs ./

# ---------------------------------------------------------------------------
# Runtime: slim — has shell, used as Kubernetes init container to copy
# Node.js + mcp-remote to a shared emptyDir volume.
#
# Build:  docker build --target slim -t newrelic-mcp:slim .
# ---------------------------------------------------------------------------
FROM node:24-slim@sha256:06e5c9f86bfa0aaa7163cf37a5eaa8805f16b9acb48e3f85645b09d459fc2a9f AS slim

LABEL org.opencontainers.image.source="https://github.com/kengou/go-guardian"
LABEL org.opencontainers.image.description="New Relic MCP bridge — mcp-remote stdio proxy for agentgateway"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /app /app
WORKDIR /app

USER node

ENTRYPOINT ["node", "/app/bridge.mjs"]

# ---------------------------------------------------------------------------
# Runtime: distroless (default) — minimal attack surface, ~50 MB.
#
# The distroless nodejs image ships /nodejs/bin/node as its ENTRYPOINT.
# No shell, no package manager, no debugging tools — production only.
#
# Alternative: replace with dhi.io/node:24 if preferred.
# ---------------------------------------------------------------------------
FROM gcr.io/distroless/nodejs22-debian12:nonroot@sha256:13593b7570658e8477de39e2f4a1dd25db2f836d68a0ba771251572d23bb4f8e AS distroless

LABEL org.opencontainers.image.source="https://github.com/kengou/go-guardian"
LABEL org.opencontainers.image.description="New Relic MCP bridge — mcp-remote stdio proxy for agentgateway"
LABEL org.opencontainers.image.licenses="Apache-2.0"

COPY --from=builder /app /app
WORKDIR /app

USER 65532:65532

# distroless nodejs ENTRYPOINT is /nodejs/bin/node — CMD is the script path.
CMD ["/app/bridge.mjs"]
