---
name: go-security
description: Go security review — OWASP Top 10, CVE scanning, dependency audit.
argument-hint: "[owasp|cve|deps|threat-model] [path]"
paths: "*.go,go.mod"
---

# /go-security — Go Security Review

Delegate to `go-guardian:security` with the user's request.

The agent owns the full workflow including MCP tool calls, learning loop, and reporting.

**Context to provide the agent:**
- Project path and go.mod location for dependency scanning
- Any specific security focus areas (e.g. OWASP category, specific CVE, threat modeling)
- Whether the request involves architecture-level concerns (triggers escalation to security-auditor)
