#!/usr/bin/env bash
# go-guardian: PreToolUse(Edit) hook -- prevention injection
# Only spawned for *.go files (filtered by "if": "Edit(*.go)" in hooks.json).
# STDIN receives JSON: {"tool_name":"Edit","tool_input":{"file_path":"...","old_string":"...","new_string":"..."}}
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

# Read hook payload.
PAYLOAD=$(cat)

# Extract file path.
FILE_PATH=$(echo "${PAYLOAD}" | jq -r '.tool_input.file_path // ""' 2>/dev/null || true)

if [[ -z "${FILE_PATH}" ]]; then
  exit 0
fi

# Use the new_string as code context.
CODE_CONTEXT=$(echo "${PAYLOAD}" | jq -r '.tool_input.new_string // "" | .[0:200] | gsub("\n"; " ")' 2>/dev/null || true)

# Query knowledge base.
# Pass code context via stdin for security (avoids CLI arg injection).
PATTERNS=$(echo "${CODE_CONTEXT}" | "${MCP_BIN}" \
  --query-knowledge \
  --db "${DB_PATH}" \
  --file-path "${FILE_PATH}" \
  2>/dev/null || true)

if [[ -z "${PATTERNS}" ]] || [[ "${PATTERNS}" == "null" ]]; then
  exit 0
fi

echo "<!-- go-guardian prevention context -->"
echo "${PATTERNS}"
echo "<!-- end go-guardian prevention context -->"

exit 0
