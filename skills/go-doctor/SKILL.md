---
name: go-doctor
description: Diagnose go-guardian installation health — runs the built-in healthcheck against the MCP binary, SQLite database, schema, seed data, and session state, plus external deps (Go, ripgrep, sqlite3). Use when the user reports go-guardian isn't working, MCP tools are failing, knowledge queries return empty, `/go` hangs, or they ask to "check", "verify", or "debug" the go-guardian install.
argument-hint: ""
---

# /go-doctor — Go Guardian Health Check

Run the go-guardian MCP server's built-in healthcheck to verify the installation is working correctly.

## Gotchas

- **Binary resolution differs by mode.** In plugin mode, the MCP binary
  lives at `$CLAUDE_PLUGIN_DATA/go-guardian-mcp`. In standalone mode,
  it's at `.go-guardian/go-guardian-mcp` in the working tree. If
  neither exists, the install is broken.
- **`[WARN] session` outside a Claude Code session is normal** — it
  means `session-start.sh` didn't run. Only flag this as a problem
  inside an active session.
- **`[FAIL] table_*` means the binary is older than the DB schema.**
  Rebuild the binary (`go build -ldflags="-s -w" -o $MCP_BIN .`); do
  not delete the DB — that wipes learned patterns.

## Steps

1. Resolve the MCP binary path:
   - Plugin mode: `$CLAUDE_PLUGIN_DATA/go-guardian-mcp`
   - Standalone: `.go-guardian/go-guardian-mcp`

2. Run the healthcheck:
   ```bash
   "$MCP_BIN" --healthcheck --db "$DB_PATH"
   ```

3. Check external dependencies:
   - `go version` — Go compiler available
   - `rg --version` — ripgrep available (required for agent/skill discovery)
   - `sqlite3 --version` — SQLite CLI available (optional, for manual inspection)

4. Report results to the user with any recommended fixes.

## Expected output

All checks should show `[OK]`. Common issues:

| Check | Fix |
|-------|-----|
| `[FAIL] db_directory` | Check filesystem permissions on the parent directory |
| `[FAIL] db_open` | Delete the DB file and restart the session — it will be recreated |
| `[FAIL] db_file` | Verify DB file exists and is readable |
| `[FAIL] schema` | DB may be corrupt — delete and recreate |
| `[FAIL] table_*` | DB schema is outdated — rebuild the binary (`go build -ldflags="-s -w" -o $MCP_BIN .`) |
| `[FAIL] seed_data` | DB may be corrupt — delete and recreate |
| `[FAIL] seed_*` | Delete the DB file and restart — seeds are applied on creation |
| `[WARN] db_permissions` | Run `chmod 600 $DB_PATH` |
| `[WARN] session` | Normal outside Claude Code — inside a session this indicates session-start.sh didn't run |
| `[WARN] build_cache` | Binary was built manually, not by the plugin hook — harmless |
