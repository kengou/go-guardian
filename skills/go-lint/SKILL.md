---
name: go-lint
description: Run golangci-lint and teach go-guardian from fixes.
argument-hint: "[package] [--fix]"
paths: "*.go,.golangci.yml,.golangci.yaml"
tools:
  - mcp__go-guardian__query_knowledge
  - Read
  - Bash
  - Grep
  - Glob
---

# /go-lint — Go Lint + Learn

This skill drives Go lint fixes through the hooks-only architecture. Learned
patterns still flow back into the knowledge base, but through inbox-writes
instead of the deprecated lint learning MCP call. Do NOT delegate to a
subagent — MCP tools only work in the main conversation.

## Step 1: Load Learned Patterns

Call **query_knowledge** with the target file path(s) to pull previously
learned lint patterns. Use them to predict recurring findings before you
run the linter.

## Step 2: Read the Pattern Dashboard

Read `.go-guardian/pattern-stats.md` for the current learning summary. This
file is produced by `go-guardian scan --all` (or `go-guardian scan --stats`)
and replaces the deprecated pattern-stats MCP call.

## Step 3: Lint Workflow

Run the linter via Bash:

```bash
golangci-lint run --config .golangci.yml ./...
# If no .golangci.yml exists:
golangci-lint run --config golangci-lint.template.yml ./...
```

Auto-fix safe rules only:

```bash
golangci-lint run --fix --config .golangci.yml --enable-only errcheck,gofmt,goimports,misspell ./...
```

Fix remaining findings manually — show rule, current code, fix, and
explanation. Re-run the linter to verify clean.

## Step 4: Learning Loop — Write to the Inbox

After fixing lint issues, write a `.go-guardian/inbox/lint-<timestamp>-<shortsha>.md`
markdown document capturing every fix in the session. The Stop-hook ingest
pipeline flushes the inbox into the SQLite knowledge base at session end.

Each entry must include:
- Rule ID (e.g. `errcheck`, `gosec G204`, `forbidigo`)
- File path and line range
- `dont_code`: the pre-fix snippet
- `do_code`: the post-fix snippet
- Severity inferred from the linter category
- A one-line rationale

Define `<timestamp>` as `YYYYMMDDTHHMMSS` UTC at the moment the document is
written. Define `<shortsha>` as `git rev-parse --short=7 HEAD`, or `nogit`
when the workspace is not a git repository.

**ALWAYS write this document after fixing lint issues.** This is what makes
go-guardian smarter over time — never skip this step.

## Step 5: Report

Produce a short human-readable report summarising:
- Rules violated before the fix pass
- Auto-fixes applied
- Manual fixes applied
- Learned patterns written to `.go-guardian/inbox/lint-*.md`

Use the `go-guardian:linter` agent definition (loaded in conversation
context) for recommended linter configuration tiers, package bans, import
organization, and project-specific linter notes.
