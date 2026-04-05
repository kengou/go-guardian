---
name: renovate
description: Analyze, validate, and improve Renovate configurations
argument-hint: "[path|--auto|dry-run|suggest|learn|stats]"
paths: "renovate.json,.renovaterc,.renovaterc.json"
tools:
  - mcp__go-guardian__validate_renovate_config
  - mcp__go-guardian__analyze_renovate_config
  - mcp__go-guardian__suggest_renovate_rule
  - mcp__go-guardian__learn_renovate_preference
  - mcp__go-guardian__query_renovate_knowledge
  - mcp__go-guardian__get_renovate_stats
---

# /renovate

Analyze and improve your Renovate configuration using best practices and learned preferences.

You MUST call the MCP tools below directly. Do NOT delegate to a subagent — MCP tools only work in the main conversation.

## Usage

```
/renovate                        -> analyze current renovate.json (auto-detect path)
/renovate <path>                 -> analyze specific config file
/renovate --auto                 -> analyze + apply safe suggestions + learn
/renovate dry-run                -> full renovate --dry-run simulation (needs RENOVATE_TOKEN)
/renovate suggest <problem>      -> targeted suggestion for a specific problem
/renovate learn                  -> interactive learning session from recent decisions
/renovate stats                  -> show dashboard
```

## Required MCP Tool Calls

Based on subcommand:

### No args or path only (default analyze):
1. **query_renovate_knowledge** — get previously learned preferences
2. **validate_renovate_config** — validate the config for errors
3. **analyze_renovate_config** — analyze for improvements
4. **get_renovate_stats** — show dashboard summary

### --auto:
1-4. Same as default, plus:
5. **suggest_renovate_rule** — generate safe suggestions
6. **learn_renovate_preference** — learn from any applied changes

### suggest \<problem\>:
1. **query_renovate_knowledge** — check existing knowledge
2. **suggest_renovate_rule** — generate targeted suggestion

### learn:
1. **query_renovate_knowledge** — review recent analysis
2. **learn_renovate_preference** — learn from user feedback

### stats:
1. **get_renovate_stats** — show full dashboard

### dry-run:
1. **validate_renovate_config** — validate first
2. Run `renovate --dry-run` via Bash (needs RENOVATE_TOKEN)

Use the `go-guardian:advisor` agent definition (loaded in conversation context) for Renovate best practices, scoring methodology, and custom datasource guidance.
