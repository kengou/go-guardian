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

## Step 2: Automated Checks via Bash

Run and report results:
- `go build ./...` — must compile
- `go vet ./...` — must be clean
- `golangci-lint run ./...` — note all findings

## Step 3: Deep Code Review via team-review

After MCP tools and automated checks, invoke `agent-teams:team-review` for multi-dimension manual analysis:

```
/agent-teams:team-review . --reviewers security,performance,architecture
```

This spawns parallel reviewers for security, performance, and architecture dimensions. They read actual source code and produce structured findings that MCP pattern matching alone would miss.

## Step 4: Merge and Report

Combine findings from all passes:
1. **MCP learned patterns** — from query_knowledge (issues that were fixed before in this codebase)
2. **Automated checks** — from go build/vet/lint
3. **Team review findings** — from security, performance, architecture reviewers
4. **Deduplicate** — same file:line from multiple sources → keep most detailed
5. **Learning loop** — after user accepts a fix, call `learn_from_review`

Use the `go-guardian:reviewer` agent definition (loaded in conversation context) for the finding format, severity levels, and Go-specific pattern knowledge.
