# go-guardian

A self-learning Claude Code plugin for Go development. Prevents bad patterns from being written by learning from golangci-lint fixes and building a project-specific knowledge base that grows with every session.

Every lint finding that gets fixed becomes a DON'T/DO pattern. Those patterns are injected into Claude Code's context before it writes Go code, so the same mistake is not repeated.

---

## Table of Contents

1. [What it does](#what-it-does)
2. [Architecture](#architecture)
3. [Quick Install](#quick-install)
4. [Plugin Ecosystem — what else to install](#plugin-ecosystem)
5. [Integrating everything together](#integrating-everything-together)
6. [Daily usage workflows](#daily-usage-workflows)
7. [MCP tools reference](#mcp-tools)
8. [CVE data and OWASP rules](#cve-data-and-owasp-rules)
9. [AgentGateway (optional)](#agentgateway)
10. [Project layout](#project-layout)
11. [Development](#development)

---

## What it does

| Capability | How |
|---|---|
| Learns from lint fixes | `post-bash` hook captures golangci-lint output + git diff, extracts DON'T/DO pairs |
| Prevents recurrence | `pre-write-go` / `pre-edit-go` hooks inject learned patterns before code is written |
| OWASP scanning | Local AST + regex rules for Go-specific A01-A10 patterns |
| CVE dependency check | Fetches Go vulnerability database once, caches locally, enriches with NVD CVSS |
| Code review | 6-phase review with learned pattern context |
| Test quality | Table-driven tests, race detection, coverage enforcement |
| Anti-pattern detection | 25+ pre-seeded Go anti-patterns (AP, CONC, ERR, TEST series) |
| Staleness nudges | Session-start hook warns when scans are overdue |

---

## Architecture

```
Claude Code
├── Skills: /go  /go-review  /go-security  /go-lint  /go-test  /go-patterns
├── Agents: orchestrator  reviewer  security  linter  tester  patterns
└── Hooks:  session-start  post-bash  pre-write-go  pre-edit-go
                    │
                    │ MCP (stdio)
                    ▼
           go-guardian-mcp  (Go binary)
                    │
                    ▼
              SQLite  guardian.db
                    │
                    │ (optional)
                    ▼
           AgentGateway  → GitHub Advisories, NVD, pkg.go.dev
```

**MCP server** — Go binary using `mcp-go` and pure-Go SQLite (`modernc.org/sqlite`). No CGo. Communicates over stdio.

**Agents** — Markdown definitions loaded by Claude Code for specialized tasks.

**Skills** — Slash commands that route to agents and MCP tools.

**Hooks** — Shell scripts that fire on Claude Code events. Prevention injection + learning loop.

---

## Quick Install

```bash
git clone <repo-url> go-guardian
cd /path/to/your-go-project
/path/to/go-guardian/install.sh
```

The installer:
1. Builds the MCP server binary → `.go-guardian/go-guardian-mcp`
2. Pre-populates CVE cache from your `go.mod`
3. Fetches initial OWASP rule patterns from GitHub Security Advisories
4. Copies agents → `.claude/agents/`, skills → `.claude/skills/`
5. Copies hooks → `.go-guardian/hooks/`
6. Generates a settings snippet → `.go-guardian/settings-snippet.json`

Merge the generated snippet into your `.claude/settings.json`, then restart Claude Code.

```bash
# Install globally (agents/skills available in all projects)
./install.sh --global

# Target a specific project
./install.sh --project ~/myproject

# With API keys for richer data
NVD_API_KEY=your-key GITHUB_TOKEN=your-token ./install.sh
```

Add to `.gitignore`:
```
.go-guardian/guardian.db
.go-guardian/go-guardian-mcp
```

**Requires:** Go 1.26+, git.

---

## Plugin Ecosystem

go-guardian is the Go domain layer. It works best alongside a small stack of complementary plugins, each owning a distinct layer of the workflow. None of these are required — but together they cover the full development lifecycle without duplication.

### Layer overview

```
Lifecycle     →  beastmode        plan → implement → validate → release
Parallelism   →  agent-teams      parallel code review, parallel debugging
Security+     →  security-scanning  threat modeling, compliance, SAST setup
Go domain     →  go-guardian      MCP learning, OWASP/CVE, learned patterns
```

### beastmode

**What it adds:** Structured lifecycle management. `/plan` writes bite-sized implementation plans. `/implement` dispatches subagents per task. `/validate` runs quality gates. `/release` generates changelogs.

**Install:**
```bash
claude plugins install beastmode-marketplace/beastmode
```

**When to use:** Any time you're building or refactoring a non-trivial feature. Run `/plan` before starting, `/implement` to execute, `/validate` before merging.

### agent-teams (wshobson/agents)

**What it adds:** Parallel agent coordination. `/team-spawn` creates a team of specialist agents working in parallel. `/team-review` runs a multi-dimensional code review (security, performance, architecture, testing dimensions simultaneously).

**Install:**
```bash
claude plugins install claude-code-workflows/agent-teams
```

**When to use:** Large PR reviews (>10 files or >500 lines). go-guardian:reviewer automatically delegates Performance and Architecture dimensions to `team-reviewer` agents when it detects a large PR — you don't need to invoke this manually.

### security-scanning (wshobson/agents)

**What it adds:** Architecture-level security work — STRIDE threat modeling, attack tree construction, compliance frameworks (GDPR, SOC2, HIPAA, PCI-DSS), SAST tool configuration (Semgrep, CodeQL, SonarQube).

**Install:**
```bash
claude plugins install claude-code-workflows/security-scanning
```

**When to use:** When you need more than code-level OWASP scanning. go-guardian:security automatically escalates to `security-auditor` when it detects requests for threat modeling, compliance, or authentication architecture.

### Summary: what to install

| Plugin | Required | Install command |
|---|---|---|
| go-guardian (this repo) | Yes | `./install.sh` |
| beastmode | Recommended | `claude plugins install beastmode-marketplace/beastmode` |
| agent-teams | Recommended | `claude plugins install claude-code-workflows/agent-teams` |
| security-scanning | Optional | `claude plugins install claude-code-workflows/security-scanning` |

---

## Integrating everything together

Each tool occupies a distinct layer. The key rule: **go-guardian agents always handle Go-specific work first** because only they can call the MCP tools (`mcp__go-guardian__*`). Other plugins extend them, never replace them.

### How the layers interact

```
You type: /go review this PR
    │
    ▼
go-guardian:orchestrator classifies intent → "review"
    │
    ▼
go-guardian:reviewer
    ├── calls query_knowledge (MCP) → loads learned patterns
    ├── assesses PR size
    │     ├── Small PR (≤10 files, ≤500 lines) → runs 6-phase review alone
    │     └── Large PR → spawns team-reviewer agents in parallel
    │           ├── team-reviewer: Performance dimension
    │           ├── team-reviewer: Architecture dimension
    │           └── go-guardian:reviewer: Go patterns + synthesis
    └── security issues → defers to go-guardian:security
```

```
You type: /go check for vulns
    │
    ▼
go-guardian:security
    ├── calls check_deps (MCP) → CVE scan from local cache
    ├── runs govulncheck
    ├── calls check_owasp (MCP) → A01-A10 pattern scan
    └── if request involves threat modeling / compliance
          └── escalates to security-auditor (security-scanning plugin)
```

```
You type: /plan add OAuth2 login
    │
    ▼
beastmode:plan → writes implementation plan
    │
    ▼
/implement → executes plan via subagents
    │
    ▼
/validate → runs tests, quality gates
    │
    ▼
(during implementation)
pre-write-go hook → calls query_knowledge → injects learned patterns
post-bash hook → detects golangci-lint run → calls learn_from_lint
```

### Settings.json integration

After running `./install.sh`, a snippet is generated at `.go-guardian/settings-snippet.json`. Merge its `mcpServers` and `hooks` keys into your `.claude/settings.json`:

```json
{
  "mcpServers": {
    "go-guardian": {
      "type": "stdio",
      "command": "/path/to/project/.go-guardian/go-guardian-mcp",
      "args": ["--db", "/path/to/project/.go-guardian/guardian.db"],
      "env": {
        "NVD_API_KEY": "your-key-here",
        "GITHUB_TOKEN": "your-token-here"
      }
    }
  },
  "hooks": {
    "SessionStart": [
      { "type": "command", "command": "/path/to/project/.go-guardian/hooks/session-start.sh" }
    ],
    "PostToolUse": [
      { "type": "command", "matcher": "Bash", "command": "/path/to/project/.go-guardian/hooks/post-bash.sh" }
    ],
    "PreToolUse": [
      { "type": "command", "matcher": "Write", "command": "/path/to/project/.go-guardian/hooks/pre-write-go.sh" },
      { "type": "command", "matcher": "Edit", "command": "/path/to/project/.go-guardian/hooks/pre-edit-go.sh" }
    ]
  }
}
```

Beastmode and agent-teams manage their own settings via `claude plugins install` — no manual configuration needed.

---

## Daily usage workflows

### Starting a session

On session start, the hook automatically:
- Calls `check_staleness` and injects a warning if any scan is overdue (vuln > 3 days, OWASP > 7 days, full scan > 14 days)
- Loads the top learned patterns into context

### Full project scan

```
/go
```

Runs everything in sequence: staleness check → dep CVEs → golangci-lint + learn → go vet → OWASP scan → race tests → pattern stats report.

### Code review

```
/go-review
/go review this PR
/go comprehensive review        ← triggers parallel team-reviewer for large PRs
```

### Security check

```
/go-security
/go check for vulns
/go threat model the auth layer  ← escalates to security-auditor
```

### Fix lint issues (with learning)

```
/go-lint
```

Runs golangci-lint, fixes findings, and automatically calls `learn_from_lint` with the diff. Each fix is stored as a prevention pattern.

### Write new tests

```
/go-test
/go write tests for handler.go
```

### Feature work

```
/plan add rate limiting to the API
/implement
/validate
```

beastmode handles the lifecycle. go-guardian hooks fire automatically during implementation to inject patterns and learn from fixes.

### When go.mod changes (auto-refresh)

The `post-bash` hook detects `go get`, `go mod tidy`, or `go mod download` commands and automatically triggers a background CVE prefetch for the new dependency list. No manual action needed.

---

## MCP Tools

| Tool | Description |
|---|---|
| `learn_from_lint` | Parse golangci-lint output + git diff, extract DON'T/DO pairs, store in knowledge base |
| `query_knowledge` | Return learned patterns relevant to a file — called before any Go code is written |
| `check_owasp` | Scan Go source files for OWASP Top 10 security issues (A01-A10) |
| `check_staleness` | Report which scan types are overdue based on configurable thresholds |
| `check_deps` | Analyse go.mod dependencies for known CVEs using cached vulnerability data |
| `get_pattern_stats` | Dashboard: top lint patterns, OWASP posture, anti-pattern counts, scan history |
| `suggest_fix` | Search knowledge base for patterns matching a code snippet, return up to 3 fixes |

### Staleness thresholds

| Scan type | Threshold | Refresh command |
|---|---|---|
| `vuln` | 3 days | `/go check for vulns` or `--prefetch` |
| `owasp` | 7 days | `/go-security` |
| `full` | 14 days | `/go` |
| `owasp_rules` | 30 days | `go-guardian-mcp --update-owasp` |
| `lint` | never stale | continuous learning |

---

## CVE data and OWASP rules

### Vulnerability data (go-vuln + NVD)

go-guardian fetches all Go advisories in one HTTP call from `vuln.go.dev`. With an NVD API key, each CVE is enriched with CVSS v3.1 scores.

```bash
# Get a free NVD API key: https://nvd.nist.gov/developers/request-an-api-key

# Prefetch manually
.go-guardian/go-guardian-mcp --prefetch --db .go-guardian/guardian.db \
  --go-mod go.mod --nvd-key $NVD_API_KEY
```

Without a key: vulnerability data is fetched and cached — severity shows as UNKNOWN until enriched.

### OWASP rule patterns

Refreshed monthly from GitHub Security Advisories tagged with OWASP-relevant CWEs (A01-A10).

```bash
# Refresh manually
.go-guardian/go-guardian-mcp --update-owasp --db .go-guardian/guardian.db \
  --github-token $GITHUB_TOKEN
```

Without a GitHub token: rate-limited to 60 requests/hour (usually sufficient for monthly refresh).

See [docs/cve-fetching.md](docs/cve-fetching.md) for the full fetch strategy, HTTP call budget, and CWE-to-OWASP mapping.

---

## AgentGateway (optional)

Proxies CVE API calls through an [AgentGateway](https://github.com/agentgateway/agentgateway) instance. Useful for team deployments or when you want RBAC and observability on external API calls.

### Standalone

```bash
cd gateway/standalone
agentgateway --config config.yaml
```

Exposes go-guardian MCP plus three OpenAPI-backed CVE APIs on port 3000:
- GitHub Advisories (`api.github.com`)
- NVD (`services.nvd.nist.gov`)
- Go Vulnerability Database (`vuln.go.dev`)

### Kubernetes (team deployment)

```bash
cd gateway/kubernetes
kubectl apply -k .
```

Deploys AgentGateway + go-guardian-mcp as a shared team service. All developers on the team connect to the same knowledge base.

---

## Baseline patterns

The database is pre-seeded with curated patterns from the [notque Go toolkit](https://github.com/notque/claude-code-toolkit):

| Category | IDs | Count | Examples |
|---|---|---|---|
| Anti-patterns | AP-1 – AP-7 | 7 | Premature interfaces, goroutine overkill, context soup |
| Concurrency | CONC-1 – CONC-6 | 6 | Goroutine leaks, channel misuse, mutex copying |
| Error handling | ERR-1 – ERR-6 | 6 | Swallowed errors, sentinel misuse, panic in libs |
| Testing | TEST-1 – TEST-6 | 6 | Missing table tests, t.Helper(), race tests |
| OWASP Go baseline | A01-A10 | 30+ | SQL injection, hardcoded secrets, insecure TLS |

---

## Project layout

```
go-guardian/
├── CLAUDE.md                   # Claude Code operating instructions (plugin layer map)
├── agents/                     # Claude Code agent definitions
│   ├── orchestrator.md         #   Central routing + plugin-aware coordination
│   ├── reviewer.md             #   Code review (delegates to team-reviewer for large PRs)
│   ├── security.md             #   OWASP + CVE (escalates to security-auditor)
│   ├── linter.md               #   Lint + learning loop
│   ├── tester.md               #   Test quality
│   └── patterns.md             #   Anti-pattern detection
├── skills/                     # Slash command definitions
│   ├── go/SKILL.md             #   /go
│   ├── go-review/SKILL.md      #   /go-review
│   ├── go-security/SKILL.md    #   /go-security
│   ├── go-lint/SKILL.md        #   /go-lint
│   ├── go-test/SKILL.md        #   /go-test
│   └── go-patterns/SKILL.md    #   /go-patterns
├── hooks/                      # Claude Code hook scripts
│   ├── session-start.sh        #   Staleness check on session start
│   ├── post-bash.sh            #   Learning loop + auto-prefetch on go.mod changes
│   ├── pre-write-go.sh         #   Prevention injection before Write tool
│   └── pre-edit-go.sh          #   Prevention injection before Edit tool
├── mcp-server/                 # Go MCP server (the learning engine)
│   ├── main.go
│   ├── go.mod
│   ├── db/
│   │   ├── store.go
│   │   └── seed/               #   Baseline pattern SQL files
│   ├── tools/                  #   MCP tool handlers + tests
│   └── owasp/                  #   OWASP rule engine
├── gateway/
│   ├── standalone/config.yaml  #   AgentGateway standalone config
│   └── kubernetes/             #   K8s manifests (team deployment)
├── docs/
│   └── cve-fetching.md         #   CVE fetch strategy, HTTP budget, CWE mapping
├── golangci-lint.template.yml  #   Recommended linter config
├── settings-template.json      #   Claude Code settings template
└── install.sh                  #   Installer
```

---

## Development

### Run tests

```bash
cd mcp-server
go test -count=1 -timeout=30s ./...
```

### Build manually

```bash
cd mcp-server
go build -ldflags="-s -w" -o go-guardian-mcp .
```

### Copy linter config to a project

```bash
cp go-guardian/golangci-lint.template.yml .golangci.yml
```

### Add seed patterns

SQL seed files live in `mcp-server/db/seed/`. They are loaded on first database initialization. Follow the existing INSERT format and rebuild.

---

## License

See LICENSE file.
