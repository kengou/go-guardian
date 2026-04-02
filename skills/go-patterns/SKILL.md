---
name: go-patterns
description: Query and manage learned Go anti-patterns and fix suggestions.
argument-hint: "[list|search|stats] [keyword]"
paths: "*.go"
---

# /go-patterns — Go Pattern Library

Delegate to `go-guardian:patterns` with the user's request.

The agent owns the full workflow including MCP tool calls, learning loop, and reporting.

**Context to provide the agent:**
- The subcommand or query (list, search, fix, learn, stats)
- Any code snippet or keyword for pattern matching
- Whether to show the full pattern dashboard or a filtered view
