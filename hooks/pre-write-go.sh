#!/usr/bin/env bash
# go-guardian: PreToolUse(Write) hook -- prevention injection
# Only spawned for *.go files (filtered by "if": "Write(*.go)" in hooks.json).
# STDIN receives JSON: {"tool_name":"Write","tool_input":{"file_path":"...","content":"..."}}
# Output to STDOUT injects additional context into the agent's prompt.
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

# Extract a brief code context from the content being written.
CODE_CONTEXT=$(echo "${PAYLOAD}" | jq -r '.tool_input.content // "" | .[0:200] | gsub("\n"; " ")' 2>/dev/null || true)

# Query the knowledge base for patterns relevant to this file.
# Pass code context via stdin for security (avoids CLI arg injection).
PATTERNS=$(echo "${CODE_CONTEXT}" | "${MCP_BIN}" \
  --query-knowledge \
  --db "${DB_PATH}" \
  --file-path "${FILE_PATH}" \
  2>/dev/null || true)

if [[ -z "${PATTERNS}" ]] || [[ "${PATTERNS}" == "null" ]]; then
  exit 0
fi

# Print the prevention context block -- Claude Code will prepend this to the prompt.
echo "<!-- go-guardian prevention context -->"
echo "${PATTERNS}"
echo "<!-- end go-guardian prevention context -->"

exit 0
