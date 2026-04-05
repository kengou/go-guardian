#!/usr/bin/env bash
# go-guardian e2e test runner
#
# Usage:
#   ./test/e2e/run.sh --login                          # First time: authenticate Claude Code
#   ./test/e2e/run.sh                                  # Run tests (uses saved auth)
#   ANTHROPIC_API_KEY=sk-... ./test/e2e/run.sh         # Run with API key instead
#   GO_GUARDIAN_VERSION=v0.2.1 ./test/e2e/run.sh       # Pin version
#   ./test/e2e/run.sh --debug                          # Keep container alive on failure
#
# Auth modes (pick one):
#   --login     Interactive login with Max/Pro plan (persisted in Docker volume)
#   API key     Set ANTHROPIC_API_KEY env var
#
# Flags:
#   --login    Launch interactive shell to run 'claude login' (first-time setup)
#   --debug    Keep container alive on failure for interactive debugging
#   --clean    Remove saved auth volume and start fresh

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
IMAGE_NAME="go-guardian-e2e"
CONTAINER_NAME="go-guardian-e2e"
AUTH_VOLUME="go-guardian-e2e-auth"
BUILD_TIMEOUT=600    # 10 minutes
RUN_TIMEOUT=900      # 15 minutes
DEBUG=false
LOGIN=false
CLEAN=false

# ── Parse flags ──────────────────────────────────────────────────────────────
for arg in "$@"; do
  case "$arg" in
    --debug) DEBUG=true ;;
    --login) LOGIN=true ;;
    --clean) CLEAN=true ;;
    --help|-h)
      echo "Usage: ./test/e2e/run.sh [--login|--debug|--clean]"
      echo ""
      echo "Auth (pick one):"
      echo "  --login                  Interactive login for Max/Pro plan (first time)"
      echo "  ANTHROPIC_API_KEY=...    Use API key directly"
      echo ""
      echo "Environment variables:"
      echo "  ANTHROPIC_API_KEY        (optional) Anthropic API key — skip if using --login"
      echo "  GO_GUARDIAN_VERSION      (optional) Version tag or branch (default: latest release)"
      echo "  GO_GUARDIAN_ADMIN_PORT   (optional) Admin UI port (default: 9090)"
      echo "  GO_GUARDIAN_GATEWAY      (optional) Gateway mode: native|docker|1=auto (default: native)"
      echo "  GITHUB_TOKEN             (optional) GitHub token for advisory API"
      echo "  NVD_API_KEY              (optional) NVD API key for CVE lookups"
      echo ""
      echo "Flags:"
      echo "  --login   Launch container for interactive 'claude login'"
      echo "  --debug   Keep container alive on failure for debugging"
      echo "  --clean   Remove saved auth volume and start fresh"
      exit 0
      ;;
  esac
done

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: docker is required but not found"
  exit 1
fi

# ── Clean mode ──────────────────────────────────────────────────────────────
if [ "$CLEAN" = true ]; then
  echo "=== Removing auth volume ==="
  docker volume rm "$AUTH_VOLUME" 2>/dev/null && echo "Removed $AUTH_VOLUME" || echo "Volume not found"
  exit 0
fi

# ── Build ────────────────────────────────────────────────────────────────────
echo "=== Building e2e test image (timeout: ${BUILD_TIMEOUT}s) ==="
if ! timeout "$BUILD_TIMEOUT" docker build -t "$IMAGE_NAME" "$SCRIPT_DIR"; then
  EXIT_CODE=$?
  if [ $EXIT_CODE -eq 124 ]; then
    echo "ERROR: Docker build timed out after ${BUILD_TIMEOUT}s"
  else
    echo "ERROR: Docker build failed (exit $EXIT_CODE)"
  fi
  exit 1
fi
echo "=== Image built successfully ==="

# ── Login mode ──────────────────────────────────────────────────────────────
if [ "$LOGIN" = true ]; then
  echo ""
  echo "=== Interactive login mode ==="
  echo "=== A shell will open inside the test container.                ==="
  echo "=== Run 'claude login' and follow the URL to authenticate.     ==="
  echo "=== Your auth will be saved in Docker volume: $AUTH_VOLUME      ==="
  echo "=== Type 'exit' when done.                                      ==="
  echo ""

  docker rm -f "$CONTAINER_NAME" 2>/dev/null || true
  docker run -it --rm \
    --name "${CONTAINER_NAME}-login" \
    -v "${AUTH_VOLUME}:/root/.claude-auth" \
    --entrypoint bash \
    "$IMAGE_NAME" \
    -c '
      mkdir -p /root/.claude-auth

      echo ""
      echo "  Step 1: Run '\''claude login'\'' and follow the URL."
      echo "  Step 2: After login succeeds, type '\''exit'\'' to save."
      echo ""

      # Interactive shell for the user to run claude login.
      bash --login

      # After user exits, snapshot auth files to the persistent volume.
      echo ""
      echo "=== Saving auth to volume ==="
      cp -r /root/.claude/* /root/.claude-auth/ 2>/dev/null || true
      echo "=== Done ==="
    '

  echo ""
  echo "=== Auth saved. Now run tests: ./test/e2e/run.sh ==="
  exit 0
fi

# ── Validate auth ───────────────────────────────────────────────────────────
HAS_API_KEY=false
HAS_VOLUME_AUTH=false

if [ -n "${ANTHROPIC_API_KEY:-}" ]; then
  HAS_API_KEY=true
fi

if docker volume inspect "$AUTH_VOLUME" >/dev/null 2>&1; then
  HAS_VOLUME_AUTH=true
fi

if [ "$HAS_API_KEY" = false ] && [ "$HAS_VOLUME_AUTH" = false ]; then
  echo "ERROR: No authentication configured."
  echo ""
  echo "Option 1 (Max/Pro plan): ./test/e2e/run.sh --login"
  echo "Option 2 (API key):      ANTHROPIC_API_KEY=sk-... ./test/e2e/run.sh"
  exit 1
fi

# ── Cleanup previous run ────────────────────────────────────────────────────
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

# ── Run ──────────────────────────────────────────────────────────────────────
echo "=== Running e2e tests (timeout: ${RUN_TIMEOUT}s) ==="

DOCKER_ARGS=(
  --name "$CONTAINER_NAME"
  -e "GO_GUARDIAN_VERSION=${GO_GUARDIAN_VERSION:-latest}"
  -e "E2E_OVERALL_TIMEOUT=${RUN_TIMEOUT}"
  -e "GO_GUARDIAN_ADMIN_PORT=${GO_GUARDIAN_ADMIN_PORT:-9090}"
  -e "GO_GUARDIAN_GATEWAY=${GO_GUARDIAN_GATEWAY:-native}"
  -e "GITHUB_TOKEN=${GITHUB_TOKEN:-}"
  -e "NVD_API_KEY=${NVD_API_KEY:-}"
)

# Pass API key if available.
if [ "$HAS_API_KEY" = true ]; then
  DOCKER_ARGS+=(-e "ANTHROPIC_API_KEY=${ANTHROPIC_API_KEY}")
  echo "  Auth: API key"
fi

# Mount auth volume if available.
if [ "$HAS_VOLUME_AUTH" = true ]; then
  DOCKER_ARGS+=(-v "${AUTH_VOLUME}:/root/.claude-auth:ro")
  echo "  Auth: saved login (Docker volume)"
fi

if [ "$DEBUG" = true ]; then
  # In debug mode, run with entrypoint override to keep alive on failure.
  echo "=== Debug mode: container will stay alive on failure ==="
  echo "=== Use: docker exec -it $CONTAINER_NAME bash ==="

  timeout "$RUN_TIMEOUT" docker run "${DOCKER_ARGS[@]}" -d "$IMAGE_NAME" \
    bash -c "/entrypoint.sh; EXIT=\$?; if [ \$EXIT -ne 0 ]; then echo 'Container kept alive for debugging. Use: docker exec -it $CONTAINER_NAME bash'; sleep infinity; fi; exit \$EXIT"

  # Follow logs and wait.
  docker logs -f "$CONTAINER_NAME" &
  LOG_PID=$!
  docker wait "$CONTAINER_NAME" 2>/dev/null
  EXIT_CODE=$(docker inspect "$CONTAINER_NAME" --format='{{.State.ExitCode}}' 2>/dev/null || echo "1")
  kill $LOG_PID 2>/dev/null || true

  if [ "$EXIT_CODE" != "0" ]; then
    echo ""
    echo "=== Container still running for debugging ==="
    echo "=== docker exec -it $CONTAINER_NAME bash ==="
    echo "=== docker rm -f $CONTAINER_NAME  (when done) ==="
    exit 1
  fi
else
  # Normal mode: run and cleanup.
  if timeout "$RUN_TIMEOUT" docker run --rm "${DOCKER_ARGS[@]}" "$IMAGE_NAME"; then
    echo ""
    echo "=== E2E TESTS PASSED ==="
    exit 0
  else
    EXIT_CODE=$?
    echo ""
    if [ $EXIT_CODE -eq 124 ]; then
      echo "=== E2E TESTS TIMED OUT (${RUN_TIMEOUT}s) ==="
    else
      echo "=== E2E TESTS FAILED (exit $EXIT_CODE) ==="
    fi
    exit 1
  fi
fi
