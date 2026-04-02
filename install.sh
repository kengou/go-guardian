#!/usr/bin/env bash
# go-guardian installer (standalone fallback)
# Prefer plugin marketplace installation:
#   claude plugin add github:kengou/go-guardian
# This script is the fallback for environments without plugin support.
# Usage: ./install.sh [--global] [--project PATH]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="${PWD}"
GLOBAL=false

# ── Parse args ──────────────────────────────────────────────────────────────
while [[ $# -gt 0 ]]; do
  case "$1" in
    --global)
      GLOBAL=true
      shift
      ;;
    --project)
      PROJECT_ROOT="$(realpath "$2")"
      shift 2
      ;;
    --help|-h)
      cat <<EOF
go-guardian installer

Usage: ./install.sh [OPTIONS]

Options:
  --global          Install agents and skills globally to ~/.claude/
  --project PATH    Target project directory (default: current directory)
  --help            Show this help

Examples:
  ./install.sh                          # Install locally into ./
  ./install.sh --global                 # Install agents/skills globally
  ./install.sh --project ~/myproject    # Install into a specific project
EOF
      exit 0
      ;;
    *)
      echo "Unknown flag: $1" >&2
      exit 1
      ;;
  esac
done

# ── Colours ──────────────────────────────────────────────────────────────────
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

info()  { echo -e "${GREEN}[go-guardian]${NC} $*"; }
warn()  { echo -e "${YELLOW}[go-guardian]${NC} $*"; }
error() { echo -e "${RED}[go-guardian]${NC} $*" >&2; exit 1; }

# ── Prerequisites ────────────────────────────────────────────────────────────
info "Checking prerequisites..."
command -v go  >/dev/null 2>&1 || error "go not found — install Go 1.26+"
command -v git >/dev/null 2>&1 || error "git not found"
info "  go: $(go version | awk '{print $3}')"

# ── Build binary ─────────────────────────────────────────────────────────────
GUARDIAN_DIR="${PROJECT_ROOT}/.go-guardian"
MCP_BIN="${GUARDIAN_DIR}/go-guardian-mcp"

info "Building MCP server binary..."
mkdir -p "${GUARDIAN_DIR}"

(cd "${SCRIPT_DIR}/mcp-server" && \
  go build -ldflags="-s -w" -o "${MCP_BIN}" .) \
  || error "Build failed — run 'cd ${SCRIPT_DIR}/mcp-server && go mod tidy && go build .' manually"

chmod +x "${MCP_BIN}"
info "  Binary: ${MCP_BIN}"

# ── Prefetch CVE data for existing go.mod ─────────────────────────────────────
if [[ -f "${PROJECT_ROOT}/go.mod" ]]; then
  info "Pre-populating CVE cache from go.mod..."
  NVD_KEY="${NVD_API_KEY:-}"
  if [[ -n "${NVD_KEY}" ]]; then
    info "  NVD API key found — will enrich with CVSS scores"
    "${MCP_BIN}" \
      --prefetch \
      --db "${GUARDIAN_DIR}/guardian.db" \
      --go-mod "${PROJECT_ROOT}/go.mod" \
      --nvd-key "${NVD_KEY}" \
      2>&1 | sed 's/^/  /' || warn "CVE prefetch failed (non-fatal; run manually with --prefetch)"
  else
    "${MCP_BIN}" \
      --prefetch \
      --db "${GUARDIAN_DIR}/guardian.db" \
      --go-mod "${PROJECT_ROOT}/go.mod" \
      2>&1 | sed 's/^/  /' || warn "CVE prefetch failed (non-fatal; run manually with --prefetch)"
    info "  Tip: set NVD_API_KEY before running install.sh to enable CVSS score enrichment"
  fi
fi

# ── Initial OWASP rules fetch ─────────────────────────────────────────────────
info "Fetching initial OWASP rule patterns from GitHub Security Advisory database..."
GH_TOKEN="${GITHUB_TOKEN:-}"
if [[ -n "${GH_TOKEN}" ]]; then
  info "  GitHub token found — higher rate limits"
  "${MCP_BIN}" \
    --update-owasp \
    --db "${GUARDIAN_DIR}/guardian.db" \
    --github-token "${GH_TOKEN}" \
    2>&1 | sed 's/^/  /' || warn "OWASP rules fetch failed (non-fatal; run manually with --update-owasp)"
else
  "${MCP_BIN}" \
    --update-owasp \
    --db "${GUARDIAN_DIR}/guardian.db" \
    2>&1 | sed 's/^/  /' || warn "OWASP rules fetch failed (non-fatal; run manually with --update-owasp)"
  info "  Tip: set GITHUB_TOKEN to increase GHSA API rate limits (60/h → 5000/h)"
fi

# ── Install agents ────────────────────────────────────────────────────────────
if [[ "${GLOBAL}" == "true" ]]; then
  AGENTS_DIR="${HOME}/.claude/agents"
  SKILLS_DIR="${HOME}/.claude/skills"
else
  AGENTS_DIR="${PROJECT_ROOT}/.claude/agents"
  SKILLS_DIR="${PROJECT_ROOT}/.claude/skills"
fi

info "Installing agents -> ${AGENTS_DIR}/"
mkdir -p "${AGENTS_DIR}"
for f in "${SCRIPT_DIR}/agents/"*.md; do
  cp "$f" "${AGENTS_DIR}/"
  info "  $(basename "$f")"
done

# ── Install skills ────────────────────────────────────────────────────────────
info "Installing skills -> ${SKILLS_DIR}/"
for skill_dir in "${SCRIPT_DIR}/skills/"/*/; do
  skill_name="$(basename "${skill_dir}")"
  dest_dir="${SKILLS_DIR}/${skill_name}"
  mkdir -p "${dest_dir}"
  skill_file=$(find "${skill_dir}" -maxdepth 1 -iname "skill.md" -print -quit 2>/dev/null)
  if [ -n "${skill_file}" ]; then
      cp "${skill_file}" "${dest_dir}/$(basename "${skill_file}")"
  fi
  info "  ${skill_name}/SKILL.md"
done

# ── Install hooks ─────────────────────────────────────────────────────────────
HOOKS_DEST="${GUARDIAN_DIR}/hooks"
info "Installing hooks -> ${HOOKS_DEST}/"
mkdir -p "${HOOKS_DEST}"
cp "${SCRIPT_DIR}/hooks/"*.sh "${HOOKS_DEST}/"
chmod +x "${HOOKS_DEST}/"*.sh
info "  session-start.sh, post-bash.sh, pre-write-go.sh, pre-edit-go.sh"

# ── Generate settings snippet ─────────────────────────────────────────────────
SNIPPET="${GUARDIAN_DIR}/settings-snippet.json"
info "Generating settings snippet -> ${SNIPPET}"

cat > "${SNIPPET}" <<EOF
{
  "mcpServers": {
    "go-guardian": {
      "type": "stdio",
      "command": "${MCP_BIN}",
      "args": ["--db", "${GUARDIAN_DIR}/guardian.db"],
      "env": {
        "NVD_API_KEY": "",
        "GITHUB_TOKEN": ""
      }
    }
  },
  "hooks": {
    "SessionStart": [
      {
        "type": "command",
        "command": "${HOOKS_DEST}/session-start.sh"
      }
    ],
    "PostToolUse": [
      {
        "type": "command",
        "matcher": "Bash",
        "command": "${HOOKS_DEST}/post-bash.sh"
      }
    ],
    "PreToolUse": [
      {
        "type": "command",
        "matcher": "Write",
        "command": "${HOOKS_DEST}/pre-write-go.sh"
      },
      {
        "type": "command",
        "matcher": "Edit",
        "command": "${HOOKS_DEST}/pre-edit-go.sh"
      }
    ]
  }
}
EOF

# ── Print next steps ──────────────────────────────────────────────────────────
echo ""
echo -e "${GREEN}======================================================${NC}"
echo -e "${GREEN}   go-guardian installed successfully!${NC}"
echo -e "${GREEN}======================================================${NC}"
echo ""
echo -e "${YELLOW}NOTE: Plugin marketplace installation is now preferred:${NC}"
echo "  claude plugin add github:kengou/go-guardian"
echo ""
echo "If you prefer this standalone installation, continue with:"
echo ""
echo "  1. Merge the MCP + hooks config into your Claude Code settings:"
echo "     * Project-level:  ${PROJECT_ROOT}/.claude/settings.json"
echo "     * User-level:     ~/.claude/settings.json"
echo ""
echo "     The generated snippet is at: ${SNIPPET}"
echo "     Merge the 'mcpServers' and 'hooks' keys into your settings file."
echo ""
echo "  2. (Optional) Set NVD_API_KEY for CVSS enrichment:"
echo "     export NVD_API_KEY=your-key-here"
echo "     ${MCP_BIN} --prefetch --db ${GUARDIAN_DIR}/guardian.db --go-mod ${PROJECT_ROOT}/go.mod --nvd-key \$NVD_API_KEY"
echo ""
echo "  3. (Optional) Set GITHUB_TOKEN for higher GHSA rate limits:"
echo "     export GITHUB_TOKEN=your-token"
echo "     ${MCP_BIN} --update-owasp --db ${GUARDIAN_DIR}/guardian.db --github-token \$GITHUB_TOKEN"
echo ""
echo "  4. Add .go-guardian/ to .gitignore:"
echo "     echo '.go-guardian/guardian.db' >> ${PROJECT_ROOT}/.gitignore"
echo "     echo '.go-guardian/go-guardian-mcp' >> ${PROJECT_ROOT}/.gitignore"
echo ""
echo "  5. Restart Claude Code to pick up the new MCP server and hooks."
echo ""
echo "  6. Run /go in Claude Code to start your first full scan."
echo ""
