---
name: go
description: Central Go development orchestrator. Routes to specialized agents.
argument-hint: "[scan|review|test|lint|security|deps|design|plan|implement|validate] [path|topic]"
paths: "*.go,go.mod,go.sum"
tools:
  - mcp__go-guardian__query_knowledge
  - mcp__go-guardian__check_staleness
  - mcp__go-guardian__get_pattern_stats
  - mcp__go-guardian__get_health_trends
  - mcp__go-guardian__check_owasp
  - mcp__go-guardian__check_deps
  - mcp__go-guardian__report_finding
  - mcp__go-guardian__get_session_findings
---

# /go — Go Guardian Orchestrator

You MUST call MCP tools directly. Do NOT delegate to a subagent for MCP calls — they only work in the main conversation.

## Intent Classification

Use the `go-guardian:orchestrator` agent definition (loaded in conversation context) for the full intent classification table, force routes, and context injection rules.

**Routing to skills** (preferred — skills also call MCP directly):
- review → invoke `/go-review`
- security → invoke `/go-security`
- lint → invoke `/go-lint`
- test → invoke `/go-test`
- patterns → invoke `/go-patterns`
- renovate → invoke `/renovate`

**Routing to beastmode** (feature lifecycle):
- design → invoke `/beastmode:design <topic>`
- plan → invoke `/beastmode:plan <epic-name>`
- implement → invoke `/beastmode:implement <epic-name>-<feature-name>`
- validate → invoke `/beastmode:validate <epic-name>`

Keywords that trigger beastmode routing:
- **design**: "design", "new feature", "add feature", "feature request", "PRD", "spec"
- **plan**: "plan", "break down", "decompose", "task breakdown"
- **implement**: "implement", "build", "develop", "code this", "create feature"
- **validate**: "validate", "verify", "release check", "pre-release"

When beastmode intent is detected, pass the remaining arguments as the topic/epic name.
Example: `/go design a caching layer` → `/beastmode:design a caching layer`

## Full Scan (no args on existing project)

When invoked with no arguments on a project with `go.mod`, execute the full scan directly:

### Phase 1: MCP + Automated Tools
1. **check_staleness** — if stale scans exist, report them first
2. Announce: "Running full Go Guardian scan..."
3. Run via Bash:
   - `golangci-lint run ./...` (use project's `.golangci.yml` or template)
   - `go vet ./...`
   - `go test -race ./... -count=1`
   - `govulncheck ./...`
4. **check_owasp** — call with project root path
5. **check_deps** — read go.mod, extract modules, call with modules array
6. **query_knowledge** — get anti-pattern context

### Phase 2: Deep Analysis via agent-teams

After MCP and automated tools complete, invoke agent-teams for deep manual analysis:

```
/team-spawn security
```
Spawns 4 parallel security reviewers (OWASP, auth, deps, config).

```
/agent-teams:team-review . --reviewers performance,architecture,testing
```
Spawns 3 parallel reviewers for performance, architecture, and testing dimensions.

### Phase 3: Consolidate Report
1. Merge MCP findings + automated tool output + agent-teams findings
2. Deduplicate (same file:line → keep most detailed)
3. **get_pattern_stats** — show learning summary
4. **get_health_trends** — append trends section

Consolidate all findings into a single report using the format from the orchestrator agent definition.
