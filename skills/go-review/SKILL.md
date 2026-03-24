---
name: go-review
description: Go code review — architecture, style, correctness, Go idioms.
---

# /go-review — Go Code Review

## Scope

Review Go source files for:
- Correctness and logic errors
- Go idioms and standard library usage
- Error handling (no ignored errors, wrapped with context)
- Interface design and package boundaries
- Concurrency safety (goroutine leaks, race conditions, mutex misuse)
- Test quality and coverage gaps

## Process

1. Identify changed files: `git diff --name-only HEAD | grep '\.go$'`
2. For each file, call `query_knowledge` with the file path to load learned DON'T patterns.
3. Review each file against:
   - The anti-patterns returned by `query_knowledge`
   - Standard Go review checklist (see below)
4. Call `suggest_fix` for any snippet matching a known bad pattern.
5. Present findings grouped by severity: blocker / warning / suggestion.

## Review Checklist

- [ ] All errors checked and wrapped with `fmt.Errorf("context: %w", err)`
- [ ] No `panic` in library code
- [ ] Context propagated through call chain
- [ ] Goroutines have clear ownership and termination
- [ ] Exported types/functions have doc comments
- [ ] No global mutable state without synchronisation
- [ ] Table-driven tests with `t.Run`
- [ ] No `time.Sleep` in tests; use channels or `require.Eventually`
