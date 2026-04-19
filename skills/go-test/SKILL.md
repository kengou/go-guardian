---
name: go-test
description: Run, write, analyze, and debug Go tests with race detector, coverage reporting, and test quality review. Use when the user asks to run tests, write new tests, debug failing or flaky tests, improve coverage, add race detection, check test quality, or review test fixtures — even without explicitly saying "test". Catches anti-patterns like `time.Sleep`, missing error assertions, flaky fixtures, and coverage gaps.
argument-hint: "[package-or-file] [--race] [--cover]"
paths: "*.go,*_test.go"
tools:
  - mcp__go-guardian__query_knowledge
  - Read
  - Bash
  - Grep
  - Glob
---

# /go-test — Go Test

This skill reads prior session context from
`.go-guardian/session-findings.md` and pulls learned patterns via
`query_knowledge`.

## Gotchas

- **Do NOT delegate to a subagent.** `mcp__go-guardian__query_knowledge`
  only works in the main conversation context.
- **Always use `-race -count=1`.** Caching (`-count=0`) hides flaky
  tests, and missing `-race` hides concurrency bugs until production.
- **Coverage thresholds: 60% overall, 80% for security-relevant
  packages.** Packages below threshold are a finding, not silently
  accepted.
- **`time.Sleep` in tests is an anti-pattern** — slow and flaky. Flag
  it when seen and suggest synchronization primitives (channels,
  `sync.WaitGroup`, `context`) instead.

## Step 1: Read Prior Session Context

Read `.go-guardian/session-findings.md` to see what the reviewer and security
skills have already flagged in this session. Focus test efforts on flagged
areas first — a race condition the reviewer spotted in `service.go` deserves
a dedicated `-race` test, and a security finding near an input handler
deserves a negative test case.

## Step 2: Load Learned Patterns

Call **query_knowledge** with the target file path(s) to pull previously
learned test patterns. Use them to prioritise which edge cases to cover and
which anti-patterns (`time.Sleep`, unchecked errors, flaky fixtures) to
avoid.

## Step 3: Run Tests via Bash

```bash
go test -race -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Analyze results: failures, race conditions, coverage gaps.

## Step 4: Test Quality Review via team-review

After running tests, invoke `agent-teams:team-review` for test quality
analysis:

```
/agent-teams:team-review . --reviewers testing
```

This spawns a testing reviewer that will:
- Review test quality: table-driven tests, `t.Helper()`, `t.Parallel()`, `t.Cleanup()`
- Identify coverage gaps and missing edge cases
- Check for test anti-patterns (`time.Sleep`, missing error assertions, flaky patterns)
- Verify test isolation and independence

## Step 5: Merge and Report

Combine findings:

1. **Session context** — from `.go-guardian/session-findings.md` + learned patterns from `query_knowledge`
2. **Test results** — failures, race conditions, coverage numbers
3. **Test quality findings** — from team-review testing dimension
4. **Coverage thresholds** — 60% overall, 80% for security-related packages

Use the `go-guardian:tester` agent definition (loaded in conversation
context) for test patterns, coverage analysis, fixture patterns, and
project-type-specific test recommendations (controllers, HTTP services,
operators, gRPC).
