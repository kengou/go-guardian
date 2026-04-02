---
name: go-test
description: Run Go tests with race detector and coverage reporting.
argument-hint: "[package-or-file] [--race] [--cover]"
paths: "*.go,*_test.go"
---

# /go-test — Go Test

Delegate to `go-guardian:tester` with the user's request.

The agent owns the full workflow including MCP tool calls, learning loop, and reporting.

**Context to provide the agent:**
- Target files or packages to test
- Whether to include race detection and coverage reporting
- Any specific test files or functions to focus on
- Coverage thresholds (default: 60% overall, 80% for security-related packages)
