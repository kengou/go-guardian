#!/usr/bin/env bash
# go-guardian: Stop hook -- ingest agent inbox findings into the learning DB.
# Runs after every Claude response so findings dropped into
# .go-guardian/inbox/ during the turn are persisted before the next turn.
#
# CRITICAL: Hook failure MUST NEVER block Claude Code sessions. This script
# always exits 0. Real errors are logged to stderr (captured by the hook
# runner) but never propagate back as a failure exit code.

set +e

MCP_BIN="${PWD}/.go-guardian/go-guardian-mcp"
DB_PATH="${PWD}/.go-guardian/guardian.db"
INBOX_DIR="${PWD}/.go-guardian/inbox"

# No-op if the binary isn't installed for this project yet.
if [[ ! -x "${MCP_BIN}" ]]; then
  exit 0
fi

# No-op if the DB doesn't exist yet — nothing to ingest into.
if [[ ! -f "${DB_PATH}" ]]; then
  exit 0
fi

# No-op if the inbox dir doesn't exist — avoid creating it just for the hook.
if [[ ! -d "${INBOX_DIR}" ]]; then
  exit 0
fi

"${MCP_BIN}" ingest --db "${DB_PATH}" --inbox-dir "${INBOX_DIR}" 2>&1 || true

exit 0
