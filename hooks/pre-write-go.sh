#!/usr/bin/env bash
# go-guardian: PreToolUse(Write) hook -- prevention injection
# STDIN receives JSON: {"tool":"Write","input":{"file_path":"...","content":"..."}}
# Output to STDOUT injects additional context into the agent's prompt.

set -euo pipefail

PROJECT_PATH="${PWD}"
MCP_BIN="${PROJECT_PATH}/.go-guardian/go-guardian-mcp"
DB_PATH="${PROJECT_PATH}/.go-guardian/guardian.db"

# Only run if binary exists.
if [[ ! -x "${MCP_BIN}" ]]; then
  exit 0
fi

# Read hook payload.
PAYLOAD=$(cat)

# Extract file path.
FILE_PATH=$(echo "${PAYLOAD}" | python3 -c "import json,sys; d=json.load(sys.stdin); print(d.get('input',{}).get('file_path',''))" 2>/dev/null || true)

# Only care about .go files.
if [[ -z "${FILE_PATH}" ]] || [[ "${FILE_PATH}" != *.go ]]; then
  exit 0
fi

# Extract a brief code context from the content being written.
CODE_CONTEXT=$(echo "${PAYLOAD}" | python3 -c "
import json, sys
d = json.load(sys.stdin)
content = d.get('input', {}).get('content', '')
# Use first 200 chars as context hint.
print(content[:200].replace('\n', ' '))
" 2>/dev/null || true)

# Query the knowledge base for patterns relevant to this file.
PATTERNS=$("${MCP_BIN}" \
  --query-knowledge \
  --db "${DB_PATH}" \
  --file-path "${FILE_PATH}" \
  --code-context "${CODE_CONTEXT}" \
  2>/dev/null || true)

if [[ -z "${PATTERNS}" ]] || [[ "${PATTERNS}" == "null" ]]; then
  exit 0
fi

# Print the prevention context block -- Claude Code will prepend this to the prompt.
echo "<!-- go-guardian prevention context -->"
echo "${PATTERNS}"
echo "<!-- end go-guardian prevention context -->"

exit 0
