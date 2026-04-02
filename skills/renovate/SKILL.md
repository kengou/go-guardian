---
name: renovate
description: Analyze, validate, and improve Renovate configurations
argument-hint: "[path|--auto|dry-run|suggest|learn|stats]"
paths: "renovate.json,.renovaterc,.renovaterc.json"
agent: go-guardian:advisor
---

# /renovate

Analyze and improve your Renovate configuration using best practices and learned preferences.

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

## Routing

Parse the arguments and delegate to the `go-guardian:advisor` agent with context:

- **No args or path only**: Default analyze workflow
- **--auto**: Set auto-apply mode, delegate with instruction to auto-apply safe changes
- **dry-run**: Delegate with instruction to focus on validation with dry-run
- **suggest <problem>**: Pass problem description to agent for targeted suggestions
- **learn**: Delegate with instruction to review recent analysis and prompt for feedback
- **stats**: Delegate with instruction to show dashboard only

## Context Hints

- Config path: auto-detected or from first argument
- RENOVATE_TOKEN: check if set, inform agent if dry-run is available
- Current directory: pass to agent for repo context
