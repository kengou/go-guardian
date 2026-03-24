#!/usr/bin/env bash
# go-guardian: PostToolUse(Bash) hook -- learn from lint output
# STDIN receives JSON: {"tool":"Bash","input":{"command":"..."},"output":"..."}

set -euo pipefail

PROJECT_PATH="${PWD}"
MCP_BIN="${PROJECT_PATH}/.go-guardian/go-guardian-mcp"
DB_PATH="${PROJECT_PATH}/.go-guardian/guardian.db"
LINT_TMP="/tmp/go-guardian-lint-$$.txt"

# Only run if binary exists.
if [[ ! -x "${MCP_BIN}" ]]; then
  exit 0
fi

# Read hook payload from stdin.
PAYLOAD=$(cat)

# Check if the command was a golangci-lint run.
COMMAND=$(echo "${PAYLOAD}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('input',{}).get('command',''))" 2>/dev/null || true)

if [[ -z "${COMMAND}" ]]; then
  exit 0
fi

# Only proceed if golangci-lint was in the command.
if ! echo "${COMMAND}" | grep -q "golangci-lint"; then
  exit 0
fi

# Extract lint output from the hook payload.
LINT_OUTPUT=$(echo "${PAYLOAD}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('output',''))" 2>/dev/null || true)

if [[ -z "${LINT_OUTPUT}" ]]; then
  exit 0
fi

# Capture the current diff for pairing with findings.
DIFF=$(git diff HEAD 2>/dev/null || true)

# Save lint output to temp file for reference.
echo "${LINT_OUTPUT}" > "${LINT_TMP}"

# Call learn_from_lint via the MCP server's --learn flag (one-shot mode).
PROJECT_ID=$(basename "$(dirname "${PROJECT_PATH}")")/$(basename "${PROJECT_PATH}")

"${MCP_BIN}" \
  --learn \
  --db "${DB_PATH}" \
  --project "${PROJECT_ID}" \
  --lint-output "${LINT_TMP}" \
  --diff <(echo "${DIFF}") \
  2>/dev/null || true

rm -f "${LINT_TMP}"

# ── Trigger background prefetch on dependency changes ─────────────────────────
if echo "${COMMAND}" | grep -qE 'go (get|mod tidy|mod download)'; then
  if [[ -x "${MCP_BIN}" ]] && [[ -f "${PROJECT_PATH}/go.mod" ]]; then
    NVD_KEY="${NVD_API_KEY:-}"
    if [[ -n "${NVD_KEY}" ]]; then
      "${MCP_BIN}" \
        --prefetch \
        --db "${DB_PATH}" \
        --go-mod "${PROJECT_PATH}/go.mod" \
        --nvd-key "${NVD_KEY}" \
        >/tmp/go-guardian-prefetch-$$.log 2>&1 &
    else
      "${MCP_BIN}" \
        --prefetch \
        --db "${DB_PATH}" \
        --go-mod "${PROJECT_PATH}/go.mod" \
        >/tmp/go-guardian-prefetch-$$.log 2>&1 &
    fi
  fi
fi

exit 0
