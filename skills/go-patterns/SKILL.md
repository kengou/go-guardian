---
name: go-patterns
description: Query and manage learned Go anti-patterns and fix suggestions.
argument-hint: "[list|search|stats] [keyword]"
paths: "*.go"
tools:
  - mcp__go-guardian__query_knowledge
  - mcp__go-guardian__get_pattern_stats
  - mcp__go-guardian__get_health_trends
  - mcp__go-guardian__suggest_fix
---

# /go-patterns — Go Pattern Library

You MUST call the MCP tools below directly. Do NOT delegate to a subagent — MCP tools only work in the main conversation.

## Required MCP Tool Calls

Based on user's request, call the appropriate tools:

- **query_knowledge** — search learned patterns by file path or keyword. Use for `list`, `search` subcommands.
- **get_pattern_stats** — show the pattern dashboard with learning statistics. Use for `stats` subcommand or as summary at end.
- **get_health_trends** — show trend data (improving/stable/degrading) across scan history.
- **suggest_fix** — given a file path and code context, check if Go Guardian has a known fix pattern. Use for `fix` subcommand.

## Routing

- **No args / list**: call `query_knowledge` with broad query, then `get_pattern_stats`
- **search \<keyword\>**: call `query_knowledge` with the keyword
- **stats**: call `get_pattern_stats` and `get_health_trends`
- **fix \<file\>**: call `suggest_fix` with the file path and code context
- **scan**: run full architecture review (see Step 2)

## Step 2: Architecture Review via team-review

When scanning for anti-patterns across the codebase (not just querying the pattern DB), invoke `agent-teams:team-review` for deep architectural analysis:

```
/agent-teams:team-review . --reviewers architecture
```

This spawns an architecture reviewer that will:
- Check SOLID principles, separation of concerns, coupling
- Identify god structs, leaky abstractions, circular dependencies
- Review API contract design and error handling patterns
- Produce structured findings that complement the MCP pattern database

Merge team-review findings with MCP pattern data for the final report.

Use the `go-guardian:patterns` agent definition (loaded in conversation context) for anti-pattern categories, infrastructure patterns (Docker/Helm/K8s), and pattern analysis methodology.
