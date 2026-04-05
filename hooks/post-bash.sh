#!/usr/bin/env bash
# go-guardian: PostToolUse(Bash) hook -- learn from lint output
# Only spawned for golangci-lint commands (filtered by "if": "Bash(golangci-lint *)" in hooks.json).
# STDIN receives JSON: {"tool_name":"Bash","tool_input":{"command":"..."},"tool_response":"..."}
# Per-project: binary and DB in .go-guardian/.

set -euo pipefail

# ── Resolve paths (always per-project) ──────────────────────────────────────
MCP_BIN="${PWD}/.go-guardian/go-guardian-mcp"
DB_PATH="${PWD}/.go-guardian/guardian.db"

# Only run if binary exists.
if [[ ! -x "${MCP_BIN}" ]]; then
  exit 0
fi

# SECURITY: use mktemp to avoid predictable temp file symlink attacks.
LINT_TMP=$(mktemp /tmp/go-guardian-lint.XXXXXX)

# Read hook payload from stdin.
PAYLOAD=$(cat)

# Extract lint output from the hook payload.
LINT_OUTPUT=$(echo "${PAYLOAD}" | jq -r '.tool_response // .output // ""' 2>/dev/null || true)

if [[ -z "${LINT_OUTPUT}" ]]; then
  exit 0
fi

# Capture the current diff for pairing with findings.
DIFF=$(git diff HEAD 2>/dev/null || true)

# Save lint output to temp file for reference.
echo "${LINT_OUTPUT}" > "${LINT_TMP}"

# Call learn_from_lint via the MCP server's --learn flag (one-shot mode).
PROJECT_ID=$(basename "$(dirname "${PWD}")")/$(basename "${PWD}")

"${MCP_BIN}" \
  --learn \
  --db "${DB_PATH}" \
  --project "${PROJECT_ID}" \
  --lint-output "${LINT_TMP}" \
  --diff <(echo "${DIFF}") \
  2>/dev/null || true

rm -f "${LINT_TMP}"

exit 0
