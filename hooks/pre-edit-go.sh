#!/usr/bin/env bash
# go-guardian: PreToolUse(Edit) hook -- prevention injection
# STDIN receives JSON: {"tool":"Edit","input":{"file_path":"...","old_string":"...","new_string":"..."}}

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

# Use the new_string as code context.
CODE_CONTEXT=$(echo "${PAYLOAD}" | python3 -c "
import json, sys
d = json.load(sys.stdin)
new_string = d.get('input', {}).get('new_string', '')
print(new_string[:200].replace('\n', ' '))
" 2>/dev/null || true)

# Query knowledge base.
PATTERNS=$("${MCP_BIN}" \
  --query-knowledge \
  --db "${DB_PATH}" \
  --file-path "${FILE_PATH}" \
  --code-context "${CODE_CONTEXT}" \
  2>/dev/null || true)

if [[ -z "${PATTERNS}" ]] || [[ "${PATTERNS}" == "null" ]]; then
  exit 0
fi

echo "<!-- go-guardian prevention context -->"
echo "${PATTERNS}"
echo "<!-- end go-guardian prevention context -->"

exit 0
