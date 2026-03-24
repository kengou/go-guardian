---
name: go-lint
description: Run golangci-lint and teach go-guardian from fixes.
---

# /go-lint — Go Lint + Learn

## Process

1. Run golangci-lint:
   ```
   golangci-lint run ./... 2>&1 | tee /tmp/go-guardian-lint.txt
   ```

2. If findings exist:
   - Show grouped output to the user.
   - Offer to fix automatically where safe (errcheck, gofmt, goimports, misspell).

3. After fixes are applied:
   - Capture the diff: `git diff HEAD`
   - Call `learn_from_lint` with:
     - `diff`: the captured diff
     - `lint_output`: contents of `/tmp/go-guardian-lint.txt`
     - `project`: basename of current directory

4. Confirm learned patterns:
   - Report: "Learned N new patterns, updated M existing."
   - Call `get_pattern_stats` and show top-5 recurring rules.

## Lint Config

Use `.golangci.yml` if present, otherwise use the go-guardian template.
Recommended rules: errcheck, govet, staticcheck, gosec, revive, gocritic, wrapcheck.
