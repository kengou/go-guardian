---
name: go-security
description: Go security review — OWASP Top 10, CVE scanning, dependency audit.
argument-hint: "[owasp|cve|deps|threat-model] [path]"
paths: "*.go,go.mod"
tools:
  - mcp__go-guardian__check_owasp
  - mcp__go-guardian__check_deps
  - mcp__go-guardian__check_staleness
  - mcp__go-guardian__get_pattern_stats
  - mcp__go-guardian__report_finding
  - mcp__go-guardian__get_session_findings
---

# /go-security — Go Security Review

You MUST call the MCP tools below as part of every security scan. Do NOT delegate to a subagent — MCP tools only work in the main conversation.

## Required MCP Tool Calls

Run these in order:

1. **check_staleness** — check when scans were last run for this project
2. **check_owasp** — call with the project root path (or specific file if targeted scan)
3. **check_deps** — read `go.mod`, extract all direct dependency module paths, call with the modules array
4. **get_session_findings** — check what other agents (reviewer, linter) have already flagged in this session
5. **get_pattern_stats** — show learning summary at end of report

Also run via Bash:
- `govulncheck ./...` — authoritative Go vulnerability database results
- Cross-reference govulncheck results with check_deps findings

After finding a security issue, call **report_finding** so other agents can benefit.

## Analysis Context

Use the `go-guardian:security` agent definition (loaded in conversation context) for:
- OWASP A01-A10 Go-specific patterns
- Project-specific security patterns (K8s, HTTP, TLS, signing, policy, operator, mesh, auth, container)
- Banned dependency list
- Remediation guidance and report format
- Escalation criteria for security-auditor (threat modeling, compliance, auth architecture)

## Report Format

Follow the report format from the security agent: group by severity (CRITICAL/HIGH/MEDIUM/LOW), include dependency status, banned dependencies, and summary counts.
