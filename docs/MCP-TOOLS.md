# Go Guardian — MCP Tool Reference

**Version**: 0.2.8 | **17 tools registered**

## Transport

- **Protocol**: MCP (Model Context Protocol) via JSON-RPC 2.0
- **Transport**: stdio (stdin/stdout)
- **Server**: `go-guardian-mcp` binary

## Go Development Tools

### `learn_from_lint`

Store patterns learned from golangci-lint fixes.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `lint_output` | string | yes | Raw golangci-lint output |
| `diff` | string | no | Unified diff of the fixes applied |

**Returns**: Count of new patterns learned, updated patterns, and scan snapshot recorded.

**Called by**: `go-guardian:linter` agent, `post-bash.sh` hook (async), `--learn` CLI mode

---

### `learn_from_review`

Store patterns learned from code review findings.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `description` | string | yes | Short description of the finding |
| `severity` | string | yes | CRITICAL, HIGH, MEDIUM, or LOW |
| `category` | string | yes | concurrency, error-handling, testing, design, security, general |
| `dont_code` | string | yes | The original flagged code |
| `do_code` | string | yes | The applied fix |
| `file_path` | string | yes | File where the finding was detected |

**Returns**: Confirmation with pattern ID. HIGH/CRITICAL findings also stored as anti-patterns.

**Called by**: `go-guardian:reviewer` agent after user accepts a fix

---

### `query_knowledge`

Query the learned pattern database for patterns relevant to a file.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `file_path` | string | yes | Path to the file being worked on |
| `code_context` | string | no | Code snippet for context matching (max 1000 chars) |

**Returns**: Up to 10 relevant patterns sorted by frequency, plus any session findings for this file.

**Called by**: `go-guardian:reviewer` (before review), `pre-edit-go.sh` hook, `pre-write-go.sh` hook, `--query-knowledge` CLI mode

---

### `check_owasp`

Scan Go source files for OWASP A01-A10 vulnerabilities.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `path` | string | yes | Project root or specific file path |

**Returns**: Findings grouped by OWASP category (A01-A10) with file:line references.

**Path validation**: Rejects paths outside `projectDir` to prevent directory traversal.

**Called by**: `go-guardian:security` agent, full scan mode

---

### `check_staleness`

Check when each scan type was last run for a project.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project_path` | string | no | Project path (defaults to CWD) |

**Returns**: List of stale scans with `last_run_ago` and threshold. Scan types: vuln (3 days), owasp (7 days), owasp_rules (30 days), full (14 days).

**Called by**: `go-guardian:orchestrator`, `--check-staleness` CLI mode

---

### `check_deps`

Check dependencies against the cached vulnerability database.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `modules` | string[] | yes | Array of Go module paths to check |

**Returns**: Per-module CVE status (clean, vulnerable, unknown). Cross-references with `vuln_cache` table populated by prefetch.

**Called by**: `go-guardian:security` agent

---

### `get_pattern_stats`

Show learning statistics dashboard.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | no | Project name for filtering |

**Returns**: Total patterns, patterns by source (lint/review), patterns by category, top rules, recent learnings.

**Called by**: All skills at end of reports

---

### `suggest_fix`

Check if Go Guardian has a known fix pattern for given code.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `file_path` | string | yes | File being reviewed |
| `code_context` | string | yes | The code snippet to check |

**Returns**: Matching fix pattern (do_code) if found, or "no known fix" if not.

**Called by**: `go-guardian:reviewer` during review

---

### `get_health_trends`

Compute trend direction from scan snapshot history.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `project` | string | yes | Project name |
| `scan_type` | string | no | Filter by scan type (lint, owasp, vuln) |

**Returns**: Direction (improving/stable/degrading) computed from last 3 snapshots, recurring patterns, new/resolved delta.

**Called by**: `go-guardian:orchestrator` in full scan, `/go-patterns`

---

### `report_finding`

Share a finding with other agents in the current session.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `agent` | string | yes | Reporting agent name |
| `finding_type` | string | yes | e.g., race-condition, error-handling, missing-validation |
| `file_path` | string | no | File where found |
| `description` | string | yes | Finding description |
| `severity` | string | no | CRITICAL, HIGH, MEDIUM, LOW (default: MEDIUM) |

**Returns**: Confirmation with finding ID. Scoped to current session ID.

**Called by**: reviewer, security, linter agents after flagging issues

---

### `get_session_findings`

Read findings reported by other agents in the current session.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `agent` | string | no | Filter by reporting agent |
| `file_path` | string | no | Filter by file |
| `finding_type` | string | no | Filter by type |

**Returns**: List of session findings matching filters.

**Called by**: security (before scanning), tester (before writing tests)

---

## Renovate Tools

### `validate_renovate_config`

Validate a Renovate configuration file for errors.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `config` | string | yes | Renovate config JSON content |

**Returns**: Validation results (valid/invalid) with specific error details.

---

### `analyze_renovate_config`

Score and analyze a Renovate configuration against best practices.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `config` | string | yes | Renovate config JSON content |

**Returns**: Score (0-100), breakdown by category, improvement suggestions.

---

### `suggest_renovate_rule`

Generate targeted improvement suggestions.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `problem` | string | yes | Description of the problem to solve |

**Returns**: Specific Renovate config rules/patterns to address the problem.

---

### `learn_renovate_preference`

Learn from user decisions about Renovate configuration.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `preference_type` | string | yes | Type of preference |
| `value` | string | yes | Preference value |
| `context` | string | no | Additional context |

**Returns**: Confirmation that preference was stored.

---

### `query_renovate_knowledge`

Query learned Renovate preferences and rules.

| Parameter | Type | Required | Description |
|-----------|------|----------|-------------|
| `query` | string | no | Search term |

**Returns**: Matching preferences and rules.

---

### `get_renovate_stats`

Show Renovate analysis dashboard.

**Returns**: Config scores over time, preferences learned, rules applied.
