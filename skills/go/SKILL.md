---
name: go
description: Central Go development orchestrator. Routes to specialized agents.
---

# /go — Go Guardian Orchestrator

Analyse the request and the current project state, then route to the right specialist(s).

## Routing Table

| Trigger                              | Action                                    |
|--------------------------------------|-------------------------------------------|
| No arguments / "full scan"           | Run full pipeline (see below)             |
| review / check / audit               | Delegate → reviewer agent                 |
| security / owasp / cve / vuln        | Delegate → security agent                 |
| lint / golangci / errcheck           | Delegate → linter agent                   |
| test / coverage / race               | Delegate → tester agent                   |
| patterns / antipattern / learn       | Delegate → patterns agent                 |

## Full Scan Pipeline (no args)

Phase 1 – Prevention check (always first):
1. Call MCP tool `check_staleness` with the current project path.
   - If any scan is overdue, surface the staleness warning before proceeding.
2. Call MCP tool `check_deps` with all modules from go.mod.
   - Summarise CVE status; block on CRITICAL unfixed CVEs.

Phase 2 – Code quality:
3. Run: `golangci-lint run ./... 2>&1`
4. If output contains findings, call MCP tool `learn_from_lint` with:
   - `diff`: output of `git diff HEAD`
   - `lint_output`: golangci-lint output
5. Run: `go vet ./...`

Phase 3 – Security:
6. Delegate to security agent for OWASP check on changed files.
7. Run: `govulncheck ./...` (if installed).

Phase 4 – Tests:
8. Run: `go test -race ./... 2>&1`

Phase 5 – Summary:
9. Call `get_pattern_stats` and include top recurring issues in the summary.
10. Present a structured report: passed / warnings / failures.

## Prevention-First Rule

Before writing ANY .go file, call `query_knowledge` with:
- `file_path`: the target file path
- `code_context`: a brief description of what will be written

Inject the returned DON'T patterns as constraints before generating code.

## Notes

- MCP server binary: `.go-guardian/go-guardian-mcp` (started automatically by Claude Code via MCP config)
- DB path default: `.go-guardian/guardian.db`
- Always prefer fixing root causes over suppressing lint warnings
