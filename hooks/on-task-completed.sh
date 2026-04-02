#!/usr/bin/env bash
# go-guardian: TaskCompleted hook -- quality gate for agent team tasks
# Only fires when CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1 is enabled.
# Exit code 2 = reject completion (send feedback to agent).
# STDIN receives JSON with task metadata.

set -euo pipefail

# Only enforce on Go projects.
if [[ ! -f "go.mod" ]]; then
  exit 0
fi

PAYLOAD=$(cat)

# Check if Go files were modified since the task started.
CHANGED_GO_FILES=$(git diff --name-only HEAD 2>/dev/null | grep '\.go$' || true)

if [[ -z "${CHANGED_GO_FILES}" ]]; then
  # No Go files changed — nothing to gate.
  exit 0
fi

# Gate: Go code must compile.
if ! go build ./... 2>/dev/null; then
  echo "Task blocked: go build ./... fails. Fix compilation errors before marking complete."
  exit 2
fi

# Gate: Go vet must pass.
if ! go vet ./... 2>/dev/null; then
  echo "Task blocked: go vet ./... reports issues. Fix vet findings before marking complete."
  exit 2
fi

exit 0
