#!/usr/bin/env bash
# go-guardian e2e: container entrypoint
# Installs go-guardian plugin, then runs the test suite.
# Called by run.sh via docker run.

set -euo pipefail

# ── Configuration ────────────────────────────────────────────────────────────
VERSION="${GO_GUARDIAN_VERSION:-latest}"
OVERALL_TIMEOUT="${E2E_OVERALL_TIMEOUT:-900}"  # 15 minutes

log() { echo "=== [e2e] $1"; }
pass() { echo "  [PASS] $1"; PASSES=$((PASSES + 1)); }
fail() { echo "  [FAIL] $1"; FAILURES=$((FAILURES + 1)); }
skip() { echo "  [SKIP] $1"; SKIPS=$((SKIPS + 1)); }
timeout_fail() { echo "  [TIMEOUT] $1"; TIMEOUTS=$((TIMEOUTS + 1)); }

PASSES=0
FAILURES=0
SKIPS=0
TIMEOUTS=0
MCP_BIN=""
DB_DIR=""
DB_PATH=""
CLAUDE_API_WORKS=true

# ── Restore auth from volume (Max/Pro plan login) ──────────────────────────
if [ -d /root/.claude-auth ] && [ "$(ls -A /root/.claude-auth 2>/dev/null)" ]; then
  log "Restoring saved auth from volume"
  cp -rn /root/.claude-auth/* /root/.claude/ 2>/dev/null || true
fi

# ── Verify prerequisites ────────────────────────────────────────────────────
log "Checking prerequisites"

if ! command -v claude >/dev/null 2>&1; then
  fail "Claude Code not found in PATH"
  echo "PATH=$PATH"
  exit 1
fi
pass "Claude Code installed: $(claude --version 2>/dev/null || echo 'unknown')"

if ! command -v go >/dev/null 2>&1; then
  fail "Go not found"
  exit 1
fi
pass "Go installed: $(go version)"

if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  pass "Auth: API key set"
elif [ -d /root/.claude-auth ] && [ "$(ls -A /root/.claude-auth 2>/dev/null)" ]; then
  pass "Auth: saved login (Max/Pro plan)"
else
  fail "No auth: set ANTHROPIC_API_KEY or run './run.sh --login' first"
  exit 1
fi

# ── Resolve version ─────────────────────────────────────────────────────────
if [ "$VERSION" = "latest" ]; then
  log "Resolving latest go-guardian release tag"
  VERSION=$(curl -fsSL "https://api.github.com/repos/kengou/go-guardian/releases/latest" \
    | jq -r '.tag_name // empty' 2>/dev/null || true)
  if [ -z "$VERSION" ]; then
    fail "Could not resolve latest release tag from GitHub"
    exit 1
  fi
  log "Resolved version: $VERSION"
fi

# ── Install go-guardian plugin ───────────────────────────────────────────────
log "Installing go-guardian plugin (version: $VERSION)"

# Step 1: Add the marketplace.
log "Adding go-guardian marketplace"
if timeout 60 claude plugin marketplace add kengou/go-guardian 2>&1; then
  pass "Marketplace added"
else
  EXIT_CODE=$?
  if [ $EXIT_CODE -eq 124 ]; then
    timeout_fail "Marketplace add timed out (60s)"
  else
    fail "Marketplace add failed (exit $EXIT_CODE)"
  fi
  exit 1
fi

# Step 2: Install the plugin from the marketplace.
log "Installing go-guardian plugin"
if timeout 120 claude plugin install go-guardian@go-guardian-marketplace 2>&1; then
  pass "go-guardian plugin installed"
else
  EXIT_CODE=$?
  if [ $EXIT_CODE -eq 124 ]; then
    timeout_fail "Plugin installation timed out (120s)"
  else
    fail "Plugin installation failed (exit $EXIT_CODE)"
  fi
  exit 1
fi

# ── Locate plugin source and data dirs ──────────────────────────────────────
# Plugin cache contains the source code; plugin data is where binaries go.
PLUGIN_SOURCE=$(find /root/.claude/plugins/cache -path "*/go-guardian/*/mcp-server" -type d 2>/dev/null | head -1 || true)
PLUGIN_DATA=$(find /root/.claude/plugins/data -name "go-guardian*" -type d 2>/dev/null | head -1 || true)

if [ -n "$PLUGIN_SOURCE" ]; then
  log "Plugin source found: $PLUGIN_SOURCE"
else
  log "WARNING: Plugin source not found in cache"
fi
if [ -n "$PLUGIN_DATA" ]; then
  log "Plugin data dir: $PLUGIN_DATA"
else
  log "WARNING: Plugin data dir not found"
fi

# ── Test 1: Full scan simulation ────────────────────────────────────────────
# Run this FIRST — the session-start hook builds the MCP binary on first
# Claude Code invocation. The binary doesn't exist until Claude starts.
log "Test 1: Full scan via Claude Code (5 min timeout)"

SCAN_OUTPUT=$(timeout 300 claude -p --model haiku \
  "Run /go full scan on the current directory. Report all findings." \
  2>&1) || true
SCAN_EXIT=$?

if [ $SCAN_EXIT -eq 124 ]; then
  timeout_fail "Full scan timed out (300s)"
elif echo "$SCAN_OUTPUT" | grep -qi "credit balance\|insufficient.*credit\|rate.limit\|billing"; then
  skip "Full scan: API credits unavailable — $(echo "$SCAN_OUTPUT" | head -1)"
  CLAUDE_API_WORKS=false
elif echo "$SCAN_OUTPUT" | grep -qi "fatal error\|panic:"; then
  fail "Full scan crashed: $(echo "$SCAN_OUTPUT" | grep -i 'fatal\|panic' | head -3)"
else
  pass "Full scan completed"
fi

# Save scan output for debugging.
echo "$SCAN_OUTPUT" > /tmp/e2e-scan-output.txt
SCAN_LINES=$(wc -l < /tmp/e2e-scan-output.txt)
log "Scan output saved to /tmp/e2e-scan-output.txt ($SCAN_LINES lines)"
if [ "$SCAN_LINES" -lt 5 ]; then
  log "Scan output (short): $(cat /tmp/e2e-scan-output.txt)"
fi

# ── Test 2: Doctor check ───────────────────────────────────────────────────
log "Test 2: /go-doctor via Claude Code (3 min timeout)"

if [ "$CLAUDE_API_WORKS" = true ]; then
  DOCTOR_OUTPUT=$(timeout 180 claude -p --model haiku \
    "Run /go-doctor on this project. Report the full health check results." \
    2>&1) || true
  DOCTOR_EXIT=$?

  if [ $DOCTOR_EXIT -eq 124 ]; then
    timeout_fail "/go-doctor timed out (180s)"
  elif echo "$DOCTOR_OUTPUT" | grep -qi "credit balance\|insufficient.*credit\|rate.limit\|billing"; then
    skip "/go-doctor: API credits unavailable"
    CLAUDE_API_WORKS=false
  elif echo "$DOCTOR_OUTPUT" | grep -qi "fatal error\|panic:"; then
    fail "/go-doctor crashed: $(echo "$DOCTOR_OUTPUT" | grep -i 'fatal\|panic' | head -3)"
  else
    pass "/go-doctor completed"
  fi

  echo "$DOCTOR_OUTPUT" > /tmp/e2e-doctor-output.txt
  DOCTOR_LINES=$(wc -l < /tmp/e2e-doctor-output.txt)
  log "Doctor output saved to /tmp/e2e-doctor-output.txt ($DOCTOR_LINES lines)"
  if [ "$DOCTOR_LINES" -lt 5 ]; then
    log "Doctor output (short): $(cat /tmp/e2e-doctor-output.txt)"
  fi
else
  skip "/go-doctor: skipped (API credits unavailable)"
fi

# ── Build MCP binary manually if session-start hook didn't run ──────────────
# This happens when the API key has no credits — Claude exits immediately
# without starting a session, so the hook never fires.
log "Searching for go-guardian-mcp binary..."
MCP_BIN=$(find /root -name "go-guardian-mcp" -type f 2>/dev/null | head -1 || true)

if [ -z "$MCP_BIN" ] && [ -n "$PLUGIN_SOURCE" ] && [ -n "$PLUGIN_DATA" ]; then
  log "Binary not found — building manually from plugin source"
  mkdir -p "$PLUGIN_DATA"
  if (cd "$PLUGIN_SOURCE" && go build -ldflags="-s -w" -o "${PLUGIN_DATA}/go-guardian-mcp" .) 2>&1; then
    chmod +x "${PLUGIN_DATA}/go-guardian-mcp"
    MCP_BIN="${PLUGIN_DATA}/go-guardian-mcp"
    pass "MCP binary built manually"
  else
    fail "MCP binary manual build failed"
  fi
fi

if [ -n "$MCP_BIN" ]; then
  DB_DIR=$(dirname "$MCP_BIN")
  DB_PATH="${DB_DIR}/guardian.db"
  log "MCP binary: $MCP_BIN"
else
  log "WARNING: MCP binary not found — healthcheck, admin UI, and DB tests will fail"
fi

# ── Test 3: MCP server healthcheck ──────────────────────────────────────────
log "Test 3: MCP server healthcheck"

if [ -n "$MCP_BIN" ]; then
  if timeout 30 "$MCP_BIN" --healthcheck --db "${DB_PATH}" 2>/dev/null; then
    pass "MCP server healthcheck"
  else
    EXIT_CODE=$?
    if [ $EXIT_CODE -eq 124 ]; then
      timeout_fail "MCP healthcheck timed out (30s)"
    else
      fail "MCP healthcheck failed (exit $EXIT_CODE)"
    fi
  fi
else
  fail "MCP healthcheck: skipped (binary not found)"
fi

# ── Test 4: Admin UI smoke test ─────────────────────────────────────────────
log "Test 4: Admin UI smoke test"

if [ -n "$MCP_BIN" ]; then
  export GO_GUARDIAN_ADMIN_PORT="${GO_GUARDIAN_ADMIN_PORT:-9090}"

  # Start MCP server in background with admin UI.
  "$MCP_BIN" --db "${DB_PATH}" &
  MCP_PID=$!

  # Give it a moment to start.
  sleep 2

  ADMIN_BASE="http://127.0.0.1:${GO_GUARDIAN_ADMIN_PORT}"

  # Check dashboard HTML.
  if timeout 10 curl -sf "${ADMIN_BASE}/" >/dev/null 2>&1; then
    pass "Admin UI: GET / returns 200"
  else
    fail "Admin UI: GET / failed"
  fi

  # Check /api/v1/vulns.
  VULNS_RESP=$(timeout 10 curl -sf "${ADMIN_BASE}/api/v1/vulns" 2>/dev/null || true)
  if echo "$VULNS_RESP" | jq . >/dev/null 2>&1; then
    pass "Admin UI: GET /api/v1/vulns returns valid JSON"
  else
    fail "Admin UI: GET /api/v1/vulns failed or invalid JSON"
  fi

  # Check /api/v1/patterns.
  PATTERNS_RESP=$(timeout 10 curl -sf "${ADMIN_BASE}/api/v1/patterns" 2>/dev/null || true)
  if echo "$PATTERNS_RESP" | jq . >/dev/null 2>&1; then
    pass "Admin UI: GET /api/v1/patterns returns valid JSON"
  else
    fail "Admin UI: GET /api/v1/patterns failed or invalid JSON"
  fi

  # Cleanup.
  kill $MCP_PID 2>/dev/null || true
  wait $MCP_PID 2>/dev/null || true
else
  fail "Admin UI: skipped (MCP binary not found)"
fi

# ── Test 5: Database integrity ──────────────────────────────────────────────
log "Test 5: Database integrity check"

if [ -n "$MCP_BIN" ] && [ -f "$DB_PATH" ]; then
  TABLE_COUNT=$(sqlite3 "$DB_PATH" "SELECT count(*) FROM sqlite_master WHERE type='table';" 2>/dev/null || echo "0")
  if [ "$TABLE_COUNT" -gt 5 ]; then
    pass "Database schema: $TABLE_COUNT tables"
  else
    fail "Database schema: only $TABLE_COUNT tables (expected >5)"
  fi

  # Check anti_patterns seed data exists.
  AP_COUNT=$(sqlite3 "$DB_PATH" "SELECT count(*) FROM anti_patterns;" 2>/dev/null || echo "0")
  if [ "$AP_COUNT" -gt 0 ]; then
    pass "anti_patterns: $AP_COUNT seed entries"
  else
    fail "anti_patterns: 0 entries (seed data missing)"
  fi

  # vuln_cache and scan_history require a real scan to populate.
  if [ "$CLAUDE_API_WORKS" = true ]; then
    VULN_COUNT=$(sqlite3 "$DB_PATH" "SELECT count(*) FROM vuln_cache;" 2>/dev/null || echo "0")
    if [ "$VULN_COUNT" -gt 0 ]; then
      pass "vuln_cache: $VULN_COUNT entries"
    else
      fail "vuln_cache: 0 entries (expected >0 after prefetch)"
    fi

    SCAN_COUNT=$(sqlite3 "$DB_PATH" "SELECT count(*) FROM scan_history;" 2>/dev/null || echo "0")
    if [ "$SCAN_COUNT" -gt 0 ]; then
      pass "scan_history: $SCAN_COUNT records"
    else
      fail "scan_history: 0 records (expected >0 after full scan)"
    fi
  else
    skip "vuln_cache check: requires API credits for scan"
    skip "scan_history check: requires API credits for scan"
  fi
elif [ -n "$MCP_BIN" ]; then
  fail "guardian.db not found at $DB_PATH"
else
  fail "Database check: skipped (MCP binary not found)"
fi

# ── Summary ─────────────────────────────────────────────────────────────────
TOTAL=$((PASSES + FAILURES + TIMEOUTS + SKIPS))
echo ""
log "Results: $TOTAL total, $PASSES passed, $FAILURES failed, $SKIPS skipped, $TIMEOUTS timed out"

if [ $FAILURES -gt 0 ] || [ $TIMEOUTS -gt 0 ]; then
  log "RESULT: FAIL"

  # Dump diagnostics on failure.
  echo ""
  log "Diagnostics:"
  echo "  claude --version: $(claude --version 2>/dev/null || echo 'N/A')"
  echo "  go version: $(go version 2>/dev/null || echo 'N/A')"
  echo "  MCP binary: ${MCP_BIN:-not found}"
  echo "  DB path: ${DB_PATH:-unknown}"
  if [ -f "${DB_PATH:-/nonexistent}" ]; then
    echo "  DB size: $(stat -c%s "$DB_PATH" 2>/dev/null || echo 'N/A') bytes"
    echo "  DB tables: $(sqlite3 "$DB_PATH" '.tables' 2>/dev/null || echo 'N/A')"
  fi
  echo "  Disk: $(df -h /workspace | tail -1)"
  echo "  Memory: $(free -h | grep Mem)"

  exit 1
else
  log "RESULT: PASS"
  exit 0
fi
