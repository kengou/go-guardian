---
name: go
description: Central Go development orchestrator. Routes to specialized agents.
argument-hint: "[scan|review|test|lint|security|deps] [path]"
paths: "*.go,go.mod,go.sum"
---

# /go — Go Guardian Orchestrator

Delegate to `go-guardian:orchestrator` with the user's request.

The agent owns the full workflow including MCP tool calls, learning loop, and reporting.

**Context to provide the agent:**
- The user's request text (or "full scan" if no arguments given)
- Current project path and go.mod location
- Any specific files or packages mentioned by the user
