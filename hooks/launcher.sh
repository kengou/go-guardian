#!/usr/bin/env bash
# go-guardian: MCP server launcher — per-project binary + DB + admin port.
# Called by .mcp.json as the MCP server command. Sets up .go-guardian/ in the
# project directory, copies the binary from plugin data, picks a random admin
# port, and execs the MCP server.

set -euo pipefail

PROJECT_DIR="${PWD}"
GUARDIAN_DIR="${PROJECT_DIR}/.go-guardian"
MCP_BIN="${GUARDIAN_DIR}/go-guardian-mcp"
DB_PATH="${GUARDIAN_DIR}/guardian.db"

# ── Create per-project directory ────────────────────────────────────────────
# SECURITY: 0700 so the directory is not world-accessible.
mkdir -p "${GUARDIAN_DIR}"
chmod 700 "${GUARDIAN_DIR}"

# ── Copy binary from plugin data if missing or outdated ─────────────────────
if [[ -n "${CLAUDE_PLUGIN_DATA:-}" ]]; then
  SOURCE_BIN="${CLAUDE_PLUGIN_DATA}/go-guardian-mcp"
  if [[ -x "${SOURCE_BIN}" ]]; then
    if [[ ! -x "${MCP_BIN}" ]] || [[ "${SOURCE_BIN}" -nt "${MCP_BIN}" ]]; then
      cp "${SOURCE_BIN}" "${MCP_BIN}"
      chmod +x "${MCP_BIN}"
    fi
  fi
fi

# ── Fallback: build from source if binary still missing ─────────────────────
if [[ ! -x "${MCP_BIN}" ]] && [[ -n "${CLAUDE_PLUGIN_ROOT:-}" ]]; then
  SOURCE_DIR="${CLAUDE_PLUGIN_ROOT}/mcp-server"
  if [[ -d "${SOURCE_DIR}" ]] && command -v go >/dev/null 2>&1; then
    (cd "${SOURCE_DIR}" && go build -ldflags="-s -w" -o "${MCP_BIN}" .) 2>/dev/null || true
  fi
fi

if [[ ! -x "${MCP_BIN}" ]]; then
  echo "go-guardian: binary not found — run session-start hook first or install Go 1.22+" >&2
  exit 1
fi

# ── Generate random admin port (persist per-project) ────────────────────────
PORT_FILE="${GUARDIAN_DIR}/admin-port"
if [[ ! -f "${PORT_FILE}" ]]; then
  # Range 9100-9999 to avoid common ports.
  PORT=$(( (RANDOM % 900) + 9100 ))
  echo "${PORT}" > "${PORT_FILE}"
fi
ADMIN_PORT=$(cat "${PORT_FILE}")
export GO_GUARDIAN_ADMIN_PORT="${ADMIN_PORT}"

# ── Exec the MCP server ────────────────────────────────────────────────────
exec "${MCP_BIN}" --db "${DB_PATH}" --audit-log
