---
name: go-security
description: Go security review — scans for OWASP Top 10 issues, dependency CVEs, hardcoded secrets, injection flaws, broken auth, weak crypto, and supply-chain risks via `govulncheck` and the go-guardian scan cache. Use when the user asks about security, auth, authorization, crypto, tokens, secrets, passwords, CVEs, vulnerabilities, OWASP, or threat modeling in Go — and proactively when changed files touch `crypto/`, `auth/`, `internal/sql/`, or secret-handling paths.
argument-hint: "[owasp|cve|deps|threat-model] [path]"
paths: "*.go,go.mod"
tools:
  - mcp__go-guardian__query_knowledge
  - Read
  - Bash
  - Grep
  - Glob
---

# /go-security — Go Security Review

This skill reads scan results from `.go-guardian/*.md` artifacts and writes
findings to `.go-guardian/inbox/*.md`. The surviving MCP read path is
`query_knowledge`.

## Gotchas

- **Do NOT delegate to a subagent.** `mcp__go-guardian__query_knowledge`
  only works in the main conversation context.
- **`/team-spawn security` is reserved for HIGH/CRITICAL findings.**
  Spawning four parallel reviewers for a single LOW finding is wasteful
  — handle LOW/MEDIUM inline in this skill.
- **Cross-reference `govulncheck` with `.go-guardian/dep-vulns.md`.**
  The scan cache can lag the Go vuln database by up to a day, and
  `govulncheck` is authoritative.
- **Never echo secrets.** If a secret, API key, token, or credential is
  spotted, flag it as a finding but NEVER include its value in output,
  logs, or MCP tool arguments.
- **`security-auditor` escalation is for architecture-level concerns
  only** (threat modeling, compliance, auth architecture, supply chain).
  Code-level OWASP/CVE findings belong in this skill.

## Step 1: Read Prior Session Context

Read `.go-guardian/session-findings.md` to see what other agents (reviewer,
linter, tester) have already flagged this session so the security pass can
concentrate on overlapping hotspots first.

## Step 2: Refresh Scan Artifacts

Run `go-guardian scan --all` once via Bash (skip if artifacts are fresh for
this project — check `.go-guardian/staleness.md` first). This produces the
canonical markdown artifacts the skill consumes in the next steps.

## Step 3: Read the Scan Artifacts

Read the scan outputs directly from disk:

- `.go-guardian/owasp-findings.md` — OWASP A01-A10 pattern matches across the project
- `.go-guardian/dep-vulns.md` — per-module vulnerability report against the go-guardian vulnerability cache
- `.go-guardian/staleness.md` — last-scan timestamps per dimension
- `.go-guardian/pattern-stats.md` — current learning dashboard

Also run `govulncheck ./...` via Bash for authoritative Go vulnerability
database results. Cross-reference with `.go-guardian/dep-vulns.md`.

## Step 4: Enrich With Learned Patterns

Call **query_knowledge** for each HIGH/CRITICAL finding to pull related
learned anti-patterns from the knowledge base. Attach the enrichment to the
finding notes so the final report cites both the scan and the learned
context.

## Step 5: Thin-Dispatcher Gate for HIGH/CRITICAL

After reading the OWASP findings and dependency vulnerabilities, filter to
the subset with severity HIGH or CRITICAL. If any HIGH/CRITICAL findings
exist:

1. Invoke `/team-spawn security` from the agent-teams plugin. This fans the
   deep review out to four parallel reviewers:
   - **OWASP/Vulns** — injection, XSS, CSRF, deserialization, SSRF
   - **Auth/Access** — authentication, authorization, session management
   - **Dependencies** — CVEs, supply chain, outdated packages, license risks
   - **Secrets/Config** — hardcoded secrets, env vars, debug endpoints, CORS
2. Collect the parallel reviewer results.
3. Enrich each result with Go-specific notes drawn from `query_knowledge`.
4. Write the enriched results to `.go-guardian/inbox/` as
   `review-<timestamp>-<shortsha>.md` and
   `finding-<timestamp>-<shortsha>.md` markdown documents so the Stop-hook
   learning loop picks them up.

For LOW and MEDIUM severity findings, handle the review directly in this
skill — spawning four parallel reviewers for a single LOW finding is overkill.
Still write the outcomes to `.go-guardian/inbox/` so the learning loop
records them.

## Step 6: Persist Findings to the Inbox

Write every finding (from the scan artifacts, from govulncheck, and from
`/team-spawn security`) to `.go-guardian/inbox/finding-<timestamp>-<shortsha>.md`
markdown documents. Include severity, category (OWASP identifier or CVE ID),
file path and line range, attack vector, and concrete fix.

`<timestamp>` is `YYYYMMDDTHHMMSS` UTC. `<shortsha>` is `git rev-parse --short=7 HEAD`,
or `nogit` when the workspace is not a git repo.

## Step 7: Merge and Report

Combine findings into a single report:

1. **Scan artifacts** — from `.go-guardian/owasp-findings.md`, `.go-guardian/dep-vulns.md`, `govulncheck`
2. **Security team findings** — from the 4 parallel security reviewers (HIGH/CRITICAL only)
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

Escalate to `security-auditor` for: threat modeling, compliance
(GDPR/SOC2/HIPAA), auth architecture (OAuth2/OIDC design), supply chain
(SBOM/SLSA).
