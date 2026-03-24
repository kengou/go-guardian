---
name: go-patterns
description: Query and manage learned Go anti-patterns and fix suggestions.
---

# /go-patterns — Go Pattern Library

## Subcommands

### /go-patterns list
Show all known patterns grouped by category.
- Call `get_pattern_stats` and display the full pattern dashboard.

### /go-patterns search <query>
Find patterns matching a code snippet or keyword.
- Call `query_knowledge` with `code_context` = the query string.
- Display DON'T / DO pairs with confidence scores.

### /go-patterns fix <snippet>
Get a fix suggestion for a specific code snippet.
- Call `suggest_fix` with `code_snippet` = the provided snippet.
- Show the matched pattern, confidence, and suggested replacement.

### /go-patterns learn
Manually trigger learning from the last golangci-lint run.
- Requires: lint output file at `/tmp/go-guardian-lint.txt`
- Captures: `git diff HEAD`
- Calls: `learn_from_lint` and reports results.

### /go-patterns stats
Show pattern effectiveness dashboard.
- Calls `get_pattern_stats`.
- Shows: top-10 by frequency, OWASP posture, recent scan history.

## Notes

- Patterns are stored in `.go-guardian/guardian.db`
- Baseline includes: AP-1 through AP-7, CONC-1 through CONC-6,
  ERR-1 through ERR-6, TEST-1 through TEST-6, OWASP A01-A10
- Patterns accumulate with each `learn_from_lint` call
