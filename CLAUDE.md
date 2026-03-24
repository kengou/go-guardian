# Go Guardian — Claude Code Operating Instructions

This directory contains go-guardian, a self-learning Go development assistant built as an MCP server
plus Claude Code agents, skills, and hooks.

## Plugin Layer Map

Every tool in this workflow owns a distinct layer. Do not duplicate work across layers.

| Layer | Tool | Owns |
|---|---|---|
| Lifecycle | beastmode (`/plan`, `/implement`, `/validate`) | Feature planning, task breakdown, implementation orchestration, release |
| Parallelism | agent-teams (`/team-spawn`, `/team-review`) | Spawning parallel agents, coordinating workstreams, synthesising multi-agent results |
| Broad security | security-scanning (`security-auditor`) | Threat modeling, compliance frameworks, auth architecture, supply chain, cloud security |
| Go domain + memory | go-guardian (`/go`, `go-guardian:*` agents) | Go-specific code review, OWASP A01-A10 pattern matching, CVE dependency scanning, learned patterns |

## MCP Tool Ownership

`mcp__go-guardian__*` tools are **exclusively** called by `go-guardian:*` agents.

- `go-guardian:reviewer` — `query_knowledge`, `get_pattern_stats`
- `go-guardian:security` — `check_owasp`, `check_deps`, `check_staleness`, `get_pattern_stats`
- `go-guardian:linter` — `learn_from_lint`, `query_knowledge`, `get_pattern_stats`
- `go-guardian:tester` — `query_knowledge`
- `go-guardian:patterns` — `query_knowledge`, `get_pattern_stats`

No other agent (team-reviewer, security-auditor, team-lead, etc.) should attempt MCP tool calls.
The learning loop only works if go-guardian:* agents are the ones calling these tools.

## Agent Selection Guide

```
Is this Go code work?
  Yes → start with go-guardian:* agents (they own the MCP learning loop)
    Need parallel review of large PR (>10 files or >500 lines)?
      Yes → go-guardian:reviewer self-delegates to team-reviewer agents for non-Go dims
    Security architecture, threat modeling, compliance?
      Yes → go-guardian:security escalates to security-auditor
  No → use whichever specialist fits the domain

Is this feature planning / implementation / release?
  Yes → beastmode (/plan → /implement → /validate → /release)

Need to run multiple independent agents in parallel?
  Yes → agent-teams (/team-spawn, /team-review)
```

## Operating Principles

1. **No duplicate checks** — if go-guardian:security already ran check_owasp, don't re-run it in security-auditor for the same files
2. **MCP learning loop must fire** — after every lint fix session, go-guardian:linter MUST call `learn_from_lint`; never skip
3. **security-auditor is escalation, not replacement** — go-guardian:security owns code-level OWASP/CVE; security-auditor owns architecture-level concerns
4. **team-reviewer extends, not replaces, go-guardian:reviewer** — team-reviewer handles performance/architecture dims in parallel; go-guardian:reviewer retains Go patterns and MCP calls
5. **beastmode for all feature work on this codebase** — when developing go-guardian itself, use /plan before implementing anything non-trivial. If implementation goes sideways, STOP and re-plan via `/plan` — don't keep pushing
6. **Verify before done** — after any Go code change, prove it works: `go build ./...`, `go test ./...`, demonstrate correctness. Never mark a task complete without running the test suite
7. **No temporary hacks** — find root causes, especially in MCP/SQLite work where a workaround can silently corrupt the learning loop. Senior developer standards apply

## Core Principles

- **Simplicity first** — make every change as simple as possible; minimal code impact; don't over-engineer
- **No laziness** — if something is broken, find the root cause; don't paper over it with guards or fallbacks
