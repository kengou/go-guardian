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

## Step 2: Deep Security Audit via team-spawn security

After MCP tool calls complete, invoke the agent-teams security preset for deep manual source code analysis. MCP tools check cached patterns and known CVEs; the security team reads actual source and finds issues that pattern matching misses.

Invoke:
```
/team-spawn security
```

This spawns 4 parallel security reviewers:
- **OWASP/Vulns** — injection, XSS, CSRF, deserialization, SSRF
- **Auth/Access** — authentication, authorization, session management
- **Dependencies** — CVEs, supply chain, outdated packages, license risks
- **Secrets/Config** — hardcoded secrets, env vars, debug endpoints, CORS

## Step 3: Merge and Report

Combine findings from both passes into a single report:

1. **MCP findings** — from check_owasp, check_deps, govulncheck
2. **Security team findings** — from 4 parallel security reviewers (OWASP, auth, deps, config)
3. **Deduplicate** — if both found the same issue, keep the one with more detail
4. **Severity ordering** — CRITICAL → HIGH → MEDIUM → LOW

Finding format:
```
[SEVERITY] file.go:line — Short description
Evidence: <the actual code>
Attack vector: <how it could be exploited>
Fix: <concrete secure code example>
OWASP: <category reference>
```

Include dependency status, banned dependencies, and summary counts.

## Escalation

Escalate to `security-auditor` for: threat modeling, compliance (GDPR/SOC2/HIPAA), auth architecture (OAuth2/OIDC design), supply chain (SBOM/SLSA).
