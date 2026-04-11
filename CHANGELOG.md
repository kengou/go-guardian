# Changelog

All notable changes to go-guardian are documented in this file.

## v0.4.0 — 2026-04-11

The hooks-only architecture cutover. The MCP server's tool surface shrinks from
17 tools to 2, and all scan/learning work moves into dedicated CLI subcommands
whose output is picked up by the Stop-hook ingest loop. This is the breaking
change the minor bump signals.

### Breaking Changes

The following 15 MCP tools have been **removed** from the `go-guardian-mcp`
server. Any agent, skill, or external caller that still invokes them via
`mcp__go-guardian__<name>` will get an `unknown tool` error.

- `learn_from_lint`
- `learn_from_review`
- `report_finding`
- `check_owasp`
- `check_deps`
- `check_staleness`
- `get_pattern_stats`
- `get_health_trends`
- `get_session_findings`
- `validate_renovate_config`
- `analyze_renovate_config`
- `suggest_renovate_rule`
- `learn_renovate_preference`
- `query_renovate_knowledge`
- `get_renovate_stats`

The MCP server now registers exactly two tools: `query_knowledge` (learned-pattern
lookup by file glob and context) and `suggest_fix` (snippet-similarity search).
Both are read-only pattern-retrieval tools — nothing the agents call during a
session writes to the database through MCP. All writes happen through the CLI
and the Stop-hook ingest path.

### New CLI Subcommands

These subcommands are the primary surface in v0.4.0. They all write their output
into `.go-guardian/` as plain Markdown or JSON files that agents then read.

- `go-guardian scan` — runs the full scan pipeline (lint, OWASP, deps,
  staleness, pattern stats) and writes one report file per dimension under
  `.go-guardian/`. Replaces `check_owasp`, `check_deps`, `check_staleness`,
  `get_pattern_stats`, and `get_health_trends`.
- `go-guardian ingest` — consumes `.go-guardian/inbox/*.md` at session end,
  upserts the learnings into the SQLite store, then archives the inbox files.
  Replaces `learn_from_lint`, `learn_from_review`, and `report_finding`.
- `go-guardian renovate <verb>` — runs Renovate config analysis locally.
  Verbs: `validate`, `analyze`, `suggest`, `query`, `stats`. Replaces the six
  `*_renovate_*` MCP tools.
- `go-guardian admin` — runs the admin web UI (pattern browser, trends
  dashboard, ingest log viewer).
- `go-guardian healthcheck` — diagnostic: DB schema, seed data, tool
  registration, inbox directory health. Used by the session-start hook and by
  `go-guardian:orchestrator` before running the full scan.

Most of these subcommands landed in earlier waves of the cutover (CLI foundation,
scan subcommands, inbox ingest, renovate CLI pack, admin CLI); v0.4.0 is the
version where they become the primary surface and the MCP tool equivalents are
finally deleted.

### Migration Path

Users upgrading from v0.3.x should replace MCP tool calls in custom agents,
skills, or hooks as follows:

- **Scanning:** replace `check_owasp`, `check_deps`, `check_staleness`, and
  `get_pattern_stats` with a single `go-guardian scan --all` invocation, then
  read the resulting `.go-guardian/owasp-findings.md`, `.go-guardian/dep-vulns.md`,
  `.go-guardian/staleness.md`, and `.go-guardian/pattern-stats.md` files.
- **Learning:** replace `learn_from_lint`, `learn_from_review`, and
  `report_finding` calls with file writes to `.go-guardian/inbox/<kind>-<id>.md`.
  The Stop-hook runs `go-guardian ingest` automatically at session end, so
  anything the agents drop into `inbox/` during the session gets persisted
  atomically.
- **Renovate:** replace the six `*_renovate_*` tools with `go-guardian renovate
  validate|analyze|suggest|query|stats` and read `.go-guardian/inbox/renovate-*.md`
  for learned preferences.

Pre-existing `.go-guardian/` workspaces from v0.3.x continue to work unchanged —
the SQLite schema, seed data, and learned patterns all migrate without touch.
Users keep every learned pattern across the upgrade.

### Performance

The v0.4.0 architecture reduces the MCP server's per-session system-prompt tax
by approximately 40 seconds. Previously, Claude Code loaded the full 17-tool
schema plus per-agent tool allowlists on every message; the shrunk 2-tool
surface drops that tax by two orders of magnitude. The scan/ingest work that
used to happen inside MCP round-trips now runs once per session in a shell
subprocess and writes its results to disk, so the hot path for every Claude
message is a single `query_knowledge` lookup against a warm SQLite connection.

### Unchanged

- **SQLite schema.** All tables (`lint_patterns`, `anti_patterns`, `owasp_findings`,
  `vuln_cache`, `dep_decisions`, `scan_history`, `scan_snapshots`,
  `session_findings`, and the Renovate tables) are byte-identical to v0.3.x.
- **Learning data.** Every pattern the v0.3.x installation learned is still
  there; the upgrade is a drop-in binary replacement.
- **Stop-hook ingest loop.** The behavior the hook exposes is the same — the
  implementation just moved from MCP tool calls into the `go-guardian ingest`
  subcommand. Users do not need to re-register hooks.
- **Agent personas and skill routing.** `/go`, `/go-review`, `/go-security`,
  `/go-lint`, `/go-test`, `/go-patterns`, `/renovate`, and `/go-doctor` all still
  exist and dispatch to the same agents. Only the implementation underneath
  the agents changed.
