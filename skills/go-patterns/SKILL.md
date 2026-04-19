---
name: go-patterns
description: Query, browse, and analyze learned Go anti-patterns, fix suggestions, and pattern-stats dashboards from the go-guardian knowledge base. Use when the user asks about code patterns, anti-patterns, code smells, architecture issues, design problems, recurring bugs, or wants to see what go-guardian has learned. Also handles pattern-stat dashboards, health trend analysis, and architecture-level reviews.
argument-hint: "[list|search|stats|trends] [keyword]"
paths: "*.go"
tools:
  - mcp__go-guardian__query_knowledge
  - Read
  - Bash
  - Grep
  - Glob
---

# /go-patterns — Go Pattern Library

This skill routes pattern queries through `query_knowledge` (the one
surviving MCP read path) and reads the pattern dashboard and health trends
from markdown artifacts produced by `go-guardian scan --all`.

## Gotchas

- **Do NOT delegate to a subagent.** `mcp__go-guardian__query_knowledge`
  only works in the main conversation context.
- **This skill is read-only.** It never modifies source files — for
  inline fix proposals, hand off to `/go-review`.
- **Regenerate stale artifacts before presenting them.** If
  `.go-guardian/pattern-stats.md` or `.go-guardian/health-trends.md` is
  missing or stale, run `go-guardian scan --all` first; present a stale
  dashboard and the user acts on outdated data.

## Routing

Based on the user's subcommand:

- **No args / list** — call `query_knowledge` with a broad query scoped to
  the current file or package, then read `.go-guardian/pattern-stats.md` for
  the summary
- **search \<keyword\>** — call `query_knowledge` with the keyword
- **stats** — read `.go-guardian/pattern-stats.md`; present it verbatim to
  the user as the dashboard view
- **trends** — read `.go-guardian/health-trends.md`; present the direction
  (improving/stable/degrading) and the recurring pattern breakdown
- **fix \<file\>** — call `query_knowledge` with the file path and code
  context, then hand-off to the `go-review` skill if the user wants an
  inline fix proposal (this skill is read-only)
- **scan** — run full architecture review (see Step 2)

## Step 2: Architecture Review via team-review

When scanning for anti-patterns across the codebase (not just querying the
pattern DB), invoke `agent-teams:team-review` for deep architectural
analysis:

```
/agent-teams:team-review . --reviewers architecture
```

This spawns an architecture reviewer that will:
- Check SOLID principles, separation of concerns, coupling
- Identify god structs, leaky abstractions, circular dependencies
- Review API contract design and error handling patterns
- Produce structured findings that complement the scan artifacts

Merge team-review findings with the scan artifact data for the final
report.

## Step 3: Refresh Artifacts If Stale

If `.go-guardian/pattern-stats.md` or `.go-guardian/health-trends.md` is
stale or missing, run `go-guardian scan --all` via Bash once to regenerate
the markdown artifacts, then re-read them.

Use the `go-guardian:patterns` agent definition (loaded in conversation
context) for anti-pattern categories, infrastructure patterns
(Docker/Helm/K8s), and pattern analysis methodology.
