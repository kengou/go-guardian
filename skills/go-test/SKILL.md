---
name: go-test
description: Run Go tests with race detector and coverage reporting.
---

# /go-test — Go Test

## Process

1. Check for race conditions:
   ```
   go test -race -count=1 ./... 2>&1
   ```

2. Run with coverage:
   ```
   go test -coverprofile=/tmp/go-guardian-cover.out ./... 2>&1
   go tool cover -func=/tmp/go-guardian-cover.out | tail -1
   ```

3. Analyse failures:
   - For each failing test, call `query_knowledge` with the test file path.
   - Check returned patterns for known testing anti-patterns (e.g. TEST-1 through TEST-6).
   - Suggest fixes referencing the relevant pattern ID.

4. Coverage gate:
   - Warn if total coverage < 60%.
   - Warn specifically if security-related packages have < 80% coverage.

## Testing Best Practices (from learned patterns)

- Use `require.NoError` / `require.Equal` (testify) — not `if err != nil { t.Fatal }` inline
- Table-driven tests with descriptive `name` fields
- No `time.Sleep` — use channels, `require.Eventually`, or mock clocks
- Test files in same package for white-box, `_test` suffix package for black-box
