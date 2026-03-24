#!/usr/bin/env bash
# go-guardian: SessionStart hook -- staleness check
# Called by Claude Code at the start of every session.

set -euo pipefail

PROJECT_PATH="${PWD}"
MCP_BIN="${PROJECT_PATH}/.go-guardian/go-guardian-mcp"
DB_PATH="${PROJECT_PATH}/.go-guardian/guardian.db"

# Only run if the guardian binary and DB exist.
if [[ ! -x "${MCP_BIN}" ]] || [[ ! -f "${DB_PATH}" ]]; then
  exit 0
fi

# SECURITY: pipe binary stdout directly to Python stdin.
# The quoted heredoc delimiter (<<'PYEOF') prevents all shell expansion.
# The MCP binary's output is NEVER interpolated into shell or Python source.
"${MCP_BIN}" --check-staleness --db "${DB_PATH}" --project "${PROJECT_PATH}" 2>/dev/null \
| python3 - <<'PYEOF' || true
import json, sys
try:
    raw = sys.stdin.read()
    if not raw.strip():
        sys.exit(0)
    data = json.loads(raw)
    stale = data.get("stale_scans", [])
    if stale:
        print("\n  go-guardian: stale scans detected")
        for s in stale:
            print(f"  - {s['scan_type']}: last run {s['last_run_ago']} ago (threshold: {s['threshold']})")
        print("  Run /go to refresh all scans.\n")
except Exception:
    pass
PYEOF

exit 0
