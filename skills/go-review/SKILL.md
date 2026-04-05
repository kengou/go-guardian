---
name: go-review
description: Go code review — architecture, style, correctness, Go idioms.
argument-hint: "[file-or-package] [--focus concurrency|errors|style]"
paths: "*.go"
tools:
  - mcp__go-guardian__query_knowledge
  - mcp__go-guardian__get_pattern_stats
  - mcp__go-guardian__learn_from_review
  - mcp__go-guardian__report_finding
  - mcp__go-guardian__suggest_fix
---

# /go-review — Go Code Review

You MUST call the MCP tools below as part of every code review. Do NOT delegate to a subagent — MCP tools only work in the main conversation.

## Required MCP Tool Calls

### Before reviewing:
1. **query_knowledge** — call with the file path(s) being reviewed. Prepend returned learned patterns to your review context.

### During review:
2. **suggest_fix** — when you identify a fix for a finding, call with the file path and code context to check if Go Guardian already has a known fix pattern.
3. **report_finding** — after flagging an issue, call so other agents (tester, security) can prioritize areas you've identified. Include `file_path` and specific `finding_type`.

### After review (when user accepts a fix):
4. **learn_from_review** — ALWAYS call after a fix is accepted. Params: description, severity, category, dont_code, do_code, file_path. This is what makes Go Guardian smarter over time.

### At end of report:
5. **get_pattern_stats** — show learning summary.

## Review Workflow

1. Run `go build ./...`, `go vet ./...`, `golangci-lint run ./...` via Bash
2. Use the `go-guardian:reviewer` agent definition (loaded in conversation context) for the 6-phase review methodology, finding format, severity levels, and all pattern checks (error handling, concurrency, testing, security, observability, API design, K8s operators, mesh/proxy, infrastructure files)
3. For large PRs (>10 files or >500 lines), delegate Performance and Architecture dimensions to team-reviewer agents — but retain Go patterns review and MCP calls yourself
