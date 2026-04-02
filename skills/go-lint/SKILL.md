---
name: go-lint
description: Run golangci-lint and teach go-guardian from fixes.
argument-hint: "[package] [--fix]"
paths: "*.go,.golangci.yml,.golangci.yaml"
---

# /go-lint — Go Lint + Learn

Delegate to `go-guardian:linter` with the user's request.

The agent owns the full workflow including MCP tool calls, learning loop, and reporting.

**Context to provide the agent:**
- Current project path and .golangci.yml location (if present)
- Any specific linter rules or packages to focus on
- Whether auto-fix is desired for safe rules (errcheck, gofmt, goimports, misspell)
