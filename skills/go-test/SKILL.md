---
name: go-test
description: Run Go tests with race detector and coverage reporting.
argument-hint: "[package-or-file] [--race] [--cover]"
paths: "*.go,*_test.go"
tools:
  - mcp__go-guardian__query_knowledge
  - mcp__go-guardian__get_session_findings
---

# /go-test — Go Test

You MUST call the MCP tools below as part of every test session. Do NOT delegate to a subagent — MCP tools only work in the main conversation.

## Required MCP Tool Calls

### Before testing:
1. **get_session_findings** — check what other agents (reviewer, security) have flagged in this session. Focus test efforts on flagged areas first.
2. **query_knowledge** — call with the target file path(s) to get previously learned patterns. Use these to inform what test cases to write or prioritize.

## Step 2: Run Tests via Bash

```bash
go test -race -count=1 -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Analyze results: failures, race conditions, coverage gaps.

## Step 3: Test Quality Review via team-review

After running tests, invoke `agent-teams:team-review` for test quality analysis:

```
/agent-teams:team-review . --reviewers testing
```

This spawns a testing reviewer that will:
- Review test quality: table-driven tests, `t.Helper()`, `t.Parallel()`, `t.Cleanup()`
- Identify coverage gaps and missing edge cases
- Check for test anti-patterns (`time.Sleep`, missing error assertions, flaky patterns)
- Verify test isolation and independence

## Step 4: Merge and Report

Combine findings:
1. **MCP context** — session findings from other agents + learned patterns from query_knowledge
2. **Test results** — failures, race conditions, coverage numbers
3. **Test quality findings** — from team-review testing dimension
4. Coverage thresholds: 60% overall, 80% for security-related packages

Use the `go-guardian:tester` agent definition (loaded in conversation context) for test patterns, coverage analysis, fixture patterns, and project-type-specific test recommendations (controllers, HTTP services, operators, gRPC).
