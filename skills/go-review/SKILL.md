---
name: go-review
description: Review Go code for architecture, style, correctness, concurrency bugs, error handling, and idiomatic Go usage. Use when the user asks to review, audit, look over, give feedback on, check, or critique Go code — even without explicitly saying "review". Covers individual files, packages, and whole-repo reviews; escalates to parallel multi-dimension review via `agent-teams:team-review` for large changesets (>10 files or >500 lines).
argument-hint: "[file-or-package] [--focus concurrency|errors|style]"
paths: "*.go"
tools:
  - mcp__go-guardian__query_knowledge
  - mcp__go-guardian__suggest_fix
  - Read
  - Bash
  - Grep
  - Glob
---

# /go-review — Go Code Review

This skill drives Go code review through the hooks-only architecture: scan
results are read from `.go-guardian/*.md` artifacts, findings are written
to `.go-guardian/inbox/*.md`, and the surviving MCP read path is
`query_knowledge` / `suggest_fix`.

## Gotchas

- **Do NOT delegate to a subagent.** `mcp__go-guardian__query_knowledge`
  and `mcp__go-guardian__suggest_fix` only work in the main conversation
  context.
- **Every finding and accepted fix MUST be written to
  `.go-guardian/inbox/review-*.md`** before the session ends. Skipping
  this leaves the Stop-hook with nothing to ingest and the learning loop
  silently stays flat.
- **`/team-spawn` / team-review escalation is for large changesets only**
  (>10 files or >500 lines). For small reviews, a single pass in this
  skill is cheaper and catches the same issues.

## Step 1: Read Prior Session Context

Read `.go-guardian/session-findings.md` to see what other agents have already
flagged in this session. Concentrate your review on those areas first.

## Step 2: Automated Checks via Bash

Run and report results:
- `go build ./...` — must compile
- `go vet ./...` — must be clean
- `golangci-lint run ./...` — note all findings

## Step 3: Load Learned Patterns

Call **query_knowledge** with the file path(s) being reviewed. Prepend the
returned learned patterns to your review context so they inform the pass.

## Step 4: Read the Pattern Dashboard

Read `.go-guardian/pattern-stats.md` for the current learning summary. This
file is produced by `go-guardian scan --all` (or `go-guardian scan --stats`)
and replaces the deprecated pattern-stats MCP call.

## Step 5: Deep Code Review via team-review

After automated checks and loaded patterns, invoke `agent-teams:team-review`
for multi-dimension manual analysis:

```
/agent-teams:team-review . --reviewers security,performance,architecture
```

This spawns parallel reviewers for security, performance, and architecture
dimensions. They read actual source code and produce structured findings
that pattern matching alone would miss.

## Step 6: Propose Inline Fixes

For each finding you identify, call **suggest_fix** with the file path and
the code context. This returns a known fix pattern from the knowledge base
when one exists; otherwise propose the fix from first principles.

## Step 7: Persist Findings and the Learning Loop

Write every finding and accepted fix to `.go-guardian/inbox/` as markdown
documents so the Stop-hook ingest pipeline picks them up at session end:

- `review-<timestamp>-<shortsha>.md` — one per finding, with severity,
  category, `dont_code`, `do_code`, and the target file path
- `finding-<timestamp>-<shortsha>.md` — optional cross-agent notification
  for findings that overlap the tester or security domains

Define `<timestamp>` as `YYYYMMDDTHHMMSS` UTC at the moment the finding is
recorded. Define `<shortsha>` as `git rev-parse --short=7 HEAD`, or the
literal `nogit` when the workspace is not a git repository.

## Step 8: Merge and Report

Combine findings from all passes:
1. **Learned patterns** — from `query_knowledge` (issues fixed before in this codebase)
2. **Automated checks** — from go build/vet/lint
3. **Team review findings** — from security, performance, architecture reviewers
4. **Deduplicate** — same file:line from multiple sources → keep most detailed
5. **Inbox writes** — one `.go-guardian/inbox/review-*.md` per finding, so the
   learning loop records them

Use the `go-guardian:reviewer` agent definition (loaded in conversation
context) for the finding format, severity levels, and Go-specific pattern
knowledge.
