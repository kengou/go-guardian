#!/usr/bin/env bash
# go-guardian: FileChanged(go.mod) hook -- prefetch dependency vulnerability data
# Triggered when go.mod changes on disk (after go get, go mod tidy, etc.).
# STDIN receives JSON: {"file_path":"...","cwd":"..."}
# Dual-mode: works as plugin (CLAUDE_PLUGIN_DATA set) or standalone (.go-guardian/).

set -euo pipefail

# ── Resolve paths (plugin vs fallback) ───────────────────────────────────────
if [[ -n "${CLAUDE_PLUGIN_DATA:-}" ]]; then
  MCP_BIN="${CLAUDE_PLUGIN_DATA}/go-guardian-mcp"
  DB_PATH="${CLAUDE_PLUGIN_DATA}/guardian.db"
else
  MCP_BIN="${PWD}/.go-guardian/go-guardian-mcp"
  DB_PATH="${PWD}/.go-guardian/guardian.db"
fi

# Only run if binary exists.
if [[ ! -x "${MCP_BIN}" ]]; then
  exit 0
fi

# Find go.mod — use file_path from payload, fall back to PWD.
PAYLOAD=$(cat)
GO_MOD=$(echo "${PAYLOAD}" | jq -r '.file_path // ""' 2>/dev/null || true)

# SECURITY: validate file_path is actually a go.mod (prevent path traversal).
if [[ -n "${GO_MOD}" ]] && [[ "$(basename "${GO_MOD}")" != "go.mod" ]]; then
  exit 0
fi

if [[ -z "${GO_MOD}" ]] || [[ ! -f "${GO_MOD}" ]]; then
  GO_MOD="${PWD}/go.mod"
fi

if [[ ! -f "${GO_MOD}" ]]; then
  exit 0
fi

# SECURITY: use mktemp to avoid predictable temp file symlink attacks.
PREFETCH_LOG=$(mktemp /tmp/go-guardian-prefetch.XXXXXX)

# Run prefetch in the background so the hook returns immediately.
NVD_KEY="${NVD_API_KEY:-}"
if [[ -n "${NVD_KEY}" ]]; then
  "${MCP_BIN}" \
    --prefetch \
    --db "${DB_PATH}" \
    --go-mod "${GO_MOD}" \
    --nvd-key "${NVD_KEY}" \
    >"${PREFETCH_LOG}" 2>&1 &
else
  "${MCP_BIN}" \
    --prefetch \
    --db "${DB_PATH}" \
    --go-mod "${GO_MOD}" \
    >"${PREFETCH_LOG}" 2>&1 &
fi

exit 0
