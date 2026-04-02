---
name: go-review
description: Go code review — architecture, style, correctness, Go idioms.
argument-hint: "[file-or-package] [--focus concurrency|errors|style]"
paths: "*.go"
---

# /go-review — Go Code Review

Delegate to `go-guardian:reviewer` with the user's request.

The agent owns the full workflow including MCP tool calls, learning loop, and reporting.

**Context to provide the agent:**
- Changed files from the current branch or PR
- Any specific review focus areas mentioned by the user (e.g. concurrency, error handling)
- PR size context (number of files and lines changed) for large-PR delegation decisions
