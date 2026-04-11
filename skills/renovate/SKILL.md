---
name: renovate
description: Analyze, validate, and improve Renovate configurations
argument-hint: "[path|--auto|dry-run|suggest|learn|stats]"
paths: "renovate.json,.renovaterc,.renovaterc.json"
tools:
  - Read
  - Bash
  - Grep
  - Glob
---

# /renovate

Analyze and improve your Renovate configuration through the `go-guardian`
CLI. Every operation runs via Bash — this skill has no MCP tools in its
frontmatter. Do NOT delegate to a subagent — the CLI binary lives in PATH
and is cheapest to invoke from the main conversation.

## CLI Contract

Every verb below is invoked via Bash:

- `go-guardian renovate validate <config-path>` — validate a renovate config; exits non-zero on `✗ ERR:` markers
- `go-guardian renovate analyze <config-path>` — analyze a config and write `.go-guardian/renovate-analysis.md`
- `go-guardian renovate suggest "<problem>" --config <config-path>` — targeted suggestion; the problem is a quoted positional string
- `go-guardian renovate query [--category <cat>] [--keyword <term>]` — query the renovate knowledge base; prints to stdout
- `go-guardian renovate stats [--config <config-path>]` — print the renovate dashboard to stdout

The `learn` operation has no CLI verb — write a
`.go-guardian/inbox/renovate-preference-<timestamp>-<shortsha>.md` document
instead, so the Stop-hook ingest pipeline writes the preference to the
knowledge base at session end.

## Usage

```
/renovate                        → analyze current renovate.json (auto-detect path)
/renovate <path>                 → analyze specific config file
/renovate --auto                 → analyze + apply safe suggestions + drop a learn inbox doc
/renovate dry-run                → full renovate --dry-run simulation (needs RENOVATE_TOKEN)
/renovate suggest "<problem>"    → targeted suggestion for a specific problem
/renovate learn                  → interactive learning session from recent decisions
/renovate stats                  → show dashboard
```

## Operation Routing

### Default (analyze)
1. `go-guardian renovate query` — pull previously learned preferences
2. `go-guardian renovate validate <config-path>` — validate the config for errors. If the command exits non-zero, report each error verbatim to the user and **abort the workflow before step 3**.
3. `go-guardian renovate analyze <config-path>` — analyze for improvements. Read the resulting `.go-guardian/renovate-analysis.md` for the detailed report.
4. `go-guardian renovate stats` — show the dashboard. Present the output verbatim.

### --auto
1-4. Same as default, plus:
5. `go-guardian renovate suggest "<problem>" --config <config-path>` — generate targeted suggestions for each actionable issue from step 3
6. After applying safe suggestions, write a
   `.go-guardian/inbox/renovate-preference-<timestamp>-<shortsha>.md` document
   summarising which suggestions were applied, which were skipped, and why

### suggest \<problem\>
1. `go-guardian renovate query --keyword <problem-keyword>` — check existing knowledge
2. `go-guardian renovate suggest "<problem>" --config <config-path>` — generate the targeted suggestion

### learn
1. `go-guardian renovate query` — review recent analysis
2. Write a `.go-guardian/inbox/renovate-preference-<timestamp>-<shortsha>.md` document capturing the user's feedback

### stats
1. `go-guardian renovate stats` — print the dashboard to stdout; present verbatim

### dry-run
1. `go-guardian renovate validate <config-path>` — validate first
2. Run `renovate --dry-run` via Bash (needs RENOVATE_TOKEN)

## Learning Loop Timestamp Format

Define `<timestamp>` as `YYYYMMDDTHHMMSS` UTC at the moment the document is
written. Define `<shortsha>` as `git rev-parse --short=7 HEAD`, or `nogit`
when the workspace is not a git repository.

Use the `go-guardian:advisor` agent definition (loaded in conversation
context) for Renovate best practices, scoring methodology, and custom
datasource guidance.
