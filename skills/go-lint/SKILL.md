---
name: go-lint
description: Run golangci-lint and teach go-guardian from fixes.
argument-hint: "[package] [--fix]"
paths: "*.go,.golangci.yml,.golangci.yaml"
tools:
  - mcp__go-guardian__learn_from_lint
  - mcp__go-guardian__query_knowledge
  - mcp__go-guardian__get_pattern_stats
  - mcp__go-guardian__report_finding
---

# /go-lint — Go Lint + Learn

You MUST call the MCP tools below as part of every lint session. Do NOT delegate to a subagent — MCP tools only work in the main conversation.

## Required MCP Tool Calls

### Before linting:
1. **query_knowledge** — call with the target file path(s) to get previously learned patterns for context.

### After fixing lint issues:
2. **learn_from_lint** — ALWAYS call after fixing lint issues. Pass the lint output and git diff. This is what makes Go Guardian smarter over time. Never skip this step.
3. **report_finding** — report significant findings so other agents can benefit.

### At end of report:
4. **get_pattern_stats** — show learning summary.

## Lint Workflow

1. Run linter via Bash:
   ```bash
   golangci-lint run --config .golangci.yml ./...
   # If no .golangci.yml exists:
   golangci-lint run --config golangci-lint.template.yml ./...
   ```
2. Auto-fix safe rules only: `golangci-lint run --fix --config .golangci.yml --enable-only errcheck,gofmt,goimports,misspell ./...`
3. Fix remaining findings manually — show rule, current code, fix, and explanation
4. Re-run linter to verify clean

Use the `go-guardian:linter` agent definition (loaded in conversation context) for recommended linter configuration tiers, package bans, import organization, and project-specific linter notes.
