---
name: go-security
description: Go security review — OWASP Top 10, CVE scanning, dependency audit.
---

# /go-security — Go Security Review

## Process

### 1. Dependency CVE Audit
- Parse `go.mod` for direct dependencies.
- Call `check_deps` with the module list.
- For UNKNOWN modules, use gateway tools `nvd_search_cves` and `ghsa_list_advisories`.
- Report: PREFER / CHECK LATEST / AVOID / UNKNOWN per module.

### 2. OWASP Check
- Call `check_owasp` with the project path.
- Review findings against OWASP Top 10 Go heuristics stored in the DB.
- Highlight A02 (crypto), A03 (injection), A07 (auth), A10 (SSRF).

### 3. Static Secret Detection
- Search for hardcoded credentials:
  `grep -rn 'password\|secret\|apikey\|token' --include='*.go' . | grep -v '_test.go' | grep -v '// '`
- Flag any match that isn't an environment variable lookup.

### 4. govulncheck
- Run: `govulncheck ./...` (skip if not installed, note the gap).

### 5. Report Format

```
## Security Report — <project>

### CVE Status
| Module | Status | Notes |
|--------|--------|-------|

### OWASP Findings
| Category | File | Finding | Suggested Fix |
|----------|------|---------|---------------|

### Recommendations
...
```
