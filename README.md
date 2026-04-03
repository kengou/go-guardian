# go-guardian

A self-learning Claude Code plugin for Go development. Prevents bad patterns from being written by learning from golangci-lint fixes, code reviews, and building a project-specific knowledge base that grows with every session.

Every lint finding that gets fixed becomes a DON'T/DO pattern. Those patterns are injected into Claude Code's context before it writes Go code, so the same mistake is not repeated.

---

## Table of Contents

1. [What it does](#what-it-does)
2. [Architecture](#architecture)
3. [Installation](#installation)
4. [Plugin Ecosystem — what else to install](#plugin-ecosystem)
5. [Integrating everything together](#integrating-everything-together)
6. [Daily usage workflows](#daily-usage-workflows)
7. [MCP tools reference](#mcp-tools)
8. [CVE data and OWASP rules](#cve-data-and-owasp-rules)
9. [AgentGateway (optional)](#agentgateway)
10. [Project layout](#project-layout)
11. [Development](#development)
12. [Troubleshooting](#troubleshooting)

---

## What it does

| Capability | How |
|---|---|
| Learns from lint fixes | `post-bash` hook captures golangci-lint output + git diff, extracts DON'T/DO pairs |
| Learns from reviews | Reviewer agent stores accepted fixes as prevention patterns |
| Prevents recurrence | `pre-write-go` / `pre-edit-go` hooks inject learned patterns before code is written |
| OWASP scanning | Local AST + regex rules for Go-specific A01-A10 patterns |
| CVE dependency check | Fetches Go vulnerability database once, caches locally, enriches with NVD CVSS |
| Code review | 6-phase review with learned pattern context, parallel delegation for large PRs |
| Test quality | Table-driven tests, race detection, coverage enforcement |
| Anti-pattern detection | 80+ pre-seeded patterns (AP, CONC, ERR, TEST, OP, GITOPS, MESH, DIST, AUTH, DOCKER, HELM, K8SRES series) |
| Renovate config analysis | Validates, scores, and suggests improvements for Renovate configurations |
| Health trends | Tracks scan results over time, shows improving/degrading/stable direction |
| Cross-agent sharing | Agents share findings within a session so the tester knows what the reviewer flagged |
| Staleness nudges | Session-start hook warns when scans are overdue |

---

## Architecture

```
Claude Code
├── Skills: /go  /go-review  /go-security  /go-lint  /go-test  /go-patterns  /renovate  /newrelic
├── Agents: orchestrator  reviewer  security  linter  tester  patterns  advisor  newrelic
└── Hooks:  session-start  post-bash  pre-write-go  pre-edit-go  on-gomod-change
                    │
                    │ MCP (stdio)
                    ▼
           go-guardian-mcp  (Go binary, 17 tools)
                    │
                    ▼
              SQLite  guardian.db
                    │
                    │ (optional)
                    ▼
           AgentGateway  (native or Docker)
              ├── GitHub Advisories, NVD, pkg.go.dev  (OpenAPI)
              └── New Relic MCP  (stdio → mcp-remote → Streamable HTTP)
```

**MCP server** — Go binary using `mcp-go` and pure-Go SQLite (`modernc.org/sqlite`). No CGo. Communicates over stdio.

**Agents** — Markdown definitions loaded by Claude Code for specialized tasks.

**Skills** — Slash commands that route to agents and MCP tools.

**Hooks** — Shell scripts that fire on Claude Code events. Prevention injection + learning loop.

---

## Installation

### Plugin Marketplace (recommended)

```bash
claude plugin marketplace add kengou/go-guardian
claude plugin install go-guardian@go-guardian-marketplace
```

That's it. On first session start, the plugin:
1. Builds the MCP server binary from source (requires Go 1.22+)
2. Stores the binary and database in the persistent plugin data directory
3. Registers all agents, skills, hooks, and the MCP server automatically

No manual settings merge needed. Plugin updates are handled by Claude Code.

To share with your team, add to your project's `.claude/settings.json`:
```json
{
  "extraKnownMarketplaces": {
    "go-guardian-marketplace": {
      "source": {
        "source": "github",
        "repo": "kengou/go-guardian"
      }
    }
  },
  "enabledPlugins": {
    "go-guardian@go-guardian-marketplace": true
  }
}
```

### Standalone Fallback

For environments without plugin marketplace support:

```bash
git clone https://github.com/kengou/go-guardian.git
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

**Requires:** Go 1.22+, git, [ripgrep](https://github.com/BurntSushi/ripgrep) (Claude Code uses `rg` to discover agents, skills, and `@file` mentions — without it, go-guardian components may not load).

---

## Plugin Ecosystem

go-guardian is the Go domain layer. It works best alongside a small stack of complementary plugins, each owning a distinct layer of the workflow. None of these are required — but together they cover the full development lifecycle without duplication.

### Layer overview

```
Token savings →  rtk              60-90% token savings on Bash commands (transparent hook)
Lifecycle     →  beastmode        plan → implement → validate → release
Parallelism   →  agent-teams      parallel code review, parallel debugging
Security+     →  security-scanning  threat modeling, compliance, SAST setup
Go domain     →  go-guardian      MCP learning, OWASP/CVE, learned patterns, Renovate config
```

### rtk (Rust Token Killer)

**What it adds:** Transparent token optimization. A PreToolUse hook rewrites Bash commands through RTK, which filters and compresses output before it reaches Claude Code's context window. Saves 60-90% tokens on commands like `git status`, `go test`, `kubectl get`, `helm list`.

**Install:**
```bash
# Homebrew (recommended)
brew install rtk

# Or quick install script
curl -fsSL https://raw.githubusercontent.com/rtk-ai/rtk/refs/heads/master/install.sh | sh

# Initialize the Claude Code hook (global)
rtk init -g
```

**When to use:** Always. Once installed, RTK works transparently via the hook — no manual invocation needed. Use `rtk gain` to see token savings analytics.

### beastmode

**What it adds:** Structured lifecycle management. `/plan` writes bite-sized implementation plans. `/implement` dispatches subagents per task. `/validate` runs quality gates. `/release` generates changelogs.

**Install:**
```bash
claude plugin marketplace add BugRoger/beastmode
claude plugin install beastmode@beastmode-marketplace
```

**When to use:** Any time you're building or refactoring a non-trivial feature. Run `/plan` before starting, `/implement` to execute, `/validate` before merging.

### agent-teams (wshobson/agents)

**What it adds:** Parallel agent coordination. `/team-spawn` creates a team of specialist agents working in parallel. `/team-review` runs a multi-dimensional code review (security, performance, architecture, testing dimensions simultaneously).

**Install:**
```bash
claude plugin marketplace add wshobson/agents
claude plugin install agent-teams@claude-code-workflows
```

**When to use:** Large PR reviews (>10 files or >500 lines). go-guardian:reviewer automatically delegates Performance and Architecture dimensions to `team-reviewer` agents when it detects a large PR — you don't need to invoke this manually.

### security-scanning (wshobson/agents)

**What it adds:** Architecture-level security work — STRIDE threat modeling, attack tree construction, compliance frameworks (GDPR, SOC2, HIPAA, PCI-DSS), SAST tool configuration (Semgrep, CodeQL, SonarQube).

**Install:**
```bash
# Same marketplace as agent-teams (already added above)
claude plugin install security-scanning@claude-code-workflows
```

**When to use:** When you need more than code-level OWASP scanning. go-guardian:security automatically escalates to `security-auditor` when it detects requests for threat modeling, compliance, or authentication architecture.

### Summary: what to install

| Plugin | Required | Install command |
|---|---|---|
| go-guardian (this repo) | Yes | `claude plugin marketplace add kengou/go-guardian` then `claude plugin install go-guardian@go-guardian-marketplace` |
| rtk | Recommended | `brew install rtk` then `rtk init -g` |
| beastmode | Recommended | `claude plugin marketplace add BugRoger/beastmode` then `claude plugin install beastmode@beastmode-marketplace` |
| agent-teams | Recommended | `claude plugin marketplace add wshobson/agents` then `claude plugin install agent-teams@claude-code-workflows` |
| security-scanning | Optional | `claude plugin install security-scanning@claude-code-workflows` (same marketplace as agent-teams) |

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
You type: /renovate
    │
    ▼
go-guardian:advisor
    ├── detects config (renovate.json / .renovaterc / .renovaterc.json)
    ├── calls analyze_renovate_config (MCP) → scores config
    ├── calls suggest_renovate_rule (MCP) → improvement suggestions
    └── calls learn_renovate_preference (MCP) → remembers accepted/rejected suggestions
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

### Settings integration

**Plugin mode:** No manual settings merge needed — the plugin registers everything automatically.

**Standalone mode:** After running `./install.sh`, merge the generated `.go-guardian/settings-snippet.json` into your `.claude/settings.json`.

Beastmode and agent-teams manage their own settings via `claude plugins install` — no manual configuration needed.

---

## Daily usage workflows

### Starting a session

On session start, the hook automatically:
- Builds the MCP server binary if source has changed (plugin mode only)
- Generates a session ID for cross-agent finding sharing
- Generates agentgateway config with resolved paths (if gateway OpenAPI schemas are present)
- Starts agentgateway if `GO_GUARDIAN_GATEWAY` is set
- Calls `check_staleness` and injects a warning if any scan is overdue

### Full project scan

```
/go
```

Runs everything in sequence: staleness check → dep CVEs → golangci-lint + learn → go vet → OWASP scan → race tests → pattern stats report → health trends.

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

### Anti-pattern scan

```
/go-patterns
```

Scans for known anti-patterns across all categories (AP, CONC, ERR, TEST, OP, GITOPS, MESH, DIST, AUTH, DOCKER, HELM, K8SRES).

### Write new tests

```
/go-test
/go write tests for handler.go
```

### Renovate config

```
/renovate
/renovate validate
/renovate suggest rules for my Go project
```

### New Relic observability

```
/newrelic k8s cluster dashboard
/newrelic why is my-service slow
/newrelic golden signals for production
/newrelic show me error rate for the last hour
/newrelic incidents                          ← list active alerts/issues
/newrelic terraform dashboard for my-app     ← output as HCL
```

Requires the New Relic MCP server (via agentgateway bridge or direct connection). See [AgentGateway](#agentgateway) for setup.

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

### Go Guardian tools (11)

| Tool | Description |
|---|---|
| `learn_from_lint` | Parse golangci-lint output + git diff, extract DON'T/DO pairs, store in knowledge base |
| `query_knowledge` | Return learned patterns relevant to a file — called before any Go code is written |
| `check_owasp` | Scan Go source files for OWASP Top 10 security issues (A01-A10) |
| `check_staleness` | Report which scan types are overdue based on configurable thresholds |
| `check_deps` | Analyse go.mod dependencies for known CVEs using cached vulnerability data |
| `get_pattern_stats` | Dashboard: top lint patterns, OWASP posture, anti-pattern counts, scan history |
| `suggest_fix` | Search knowledge base for patterns matching a code snippet, return up to 3 fixes |
| `learn_from_review` | Store review findings as learned patterns for future prevention |
| `get_health_trends` | Return health trend data showing scan results over time |
| `report_finding` | Report a finding to the session findings table for cross-agent sharing |
| `get_session_findings` | Retrieve all findings reported during the current session across all agents |

### Renovate tools (6)

| Tool | Description |
|---|---|
| `validate_renovate_config` | Validate Renovate config structure, detect common misconfigurations |
| `analyze_renovate_config` | Score config quality, identify missing best practices |
| `suggest_renovate_rule` | Suggest Renovate rules based on project type and dependencies |
| `learn_renovate_preference` | Store user preferences when suggestions are accepted or rejected |
| `query_renovate_knowledge` | Return learned Renovate preferences relevant to the current config |
| `get_renovate_stats` | Dashboard: suggestion acceptance rate, preference history |

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

## AgentGateway

Proxies CVE API calls and external MCP servers through an [AgentGateway](https://github.com/agentgateway/agentgateway) instance. Optional — go-guardian works fully without it. Useful for team deployments, additional OpenAPI backends, or when you want RBAC and observability on external API calls.

### Plugin auto-start

When installed as a plugin, the session-start hook auto-generates a resolved gateway config and can optionally start agentgateway for you. Set the `GO_GUARDIAN_GATEWAY` environment variable:

| Value | Behavior |
|---|---|
| *(unset)* | No gateway. go-guardian via stdio only. |
| `native` | Starts native `agentgateway` binary. Full config: go-guardian stdio + OpenAPI backends through one SSE endpoint. |
| `docker` | Starts `cr.agentgateway.dev/agentgateway:1.0.1` container. OpenAPI backends only (go-guardian stays on stdio via plugin). |
| `1` | Auto-detect: tries native first, falls back to Docker. |

**Native mode is preferred** for local development — it multiplexes go-guardian + all OpenAPI backends through a single SSE connection. Docker mode requires two MCP connections (stdio for go-guardian, SSE for OpenAPI backends) because the container can't reach the host's Go binary via stdio.

After starting, add the gateway to your settings (one-time):
```json
{
  "mcpServers": {
    "go-guardian-gateway": {
      "type": "sse",
      "url": "http://localhost:3000/sse"
    }
  }
}
```

### Standalone (manual)

```bash
cd gateway/standalone
agentgateway --config config.yaml
```

Exposes go-guardian MCP plus three OpenAPI-backed CVE APIs and the New Relic MCP bridge on port 3000:
- GitHub Advisories (`api.github.com`)
- NVD (`services.nvd.nist.gov`)
- Go Vulnerability Database (`vuln.go.dev`)
- New Relic MCP (`mcp.newrelic.com` via `mcp-remote` bridge)

Requires Node.js for the New Relic bridge (runs `mcp-remote` locally).

### Kubernetes (team deployment)

```bash
cd gateway/kubernetes
kubectl apply -k .
```

Deploys AgentGateway + go-guardian-mcp + New Relic MCP bridge as a shared team service. All developers on the team connect to the same knowledge base.

### New Relic MCP

New Relic's [MCP server](https://docs.newrelic.com/docs/agentic-ai/mcp/overview/) is a hosted remote service (public preview). The bridge container runs `mcp-remote` to convert its Streamable HTTP transport to stdio for agentgateway.

**Build the bridge container:**

```bash
# Distroless (default, production)
make -C gateway/newrelic docker-build

# Slim variant (has shell, used as K8s init container)
docker build --target slim -t newrelic-mcp:slim gateway/newrelic/
```

**Environment variables:**

| Variable | Description | Default |
|---|---|---|
| `NEW_RELIC_API_KEY` | User API key (`NRAK-...`). Required unless using OAuth. | — |
| `NEW_RELIC_REGION` | Set to `eu` for the EU endpoint. | `us` |
| `NEW_RELIC_TAGS` | Comma-separated tool filter (e.g. `discovery,alerting`). | all tools |
| `NEW_RELIC_MCP_URL` | Override the MCP endpoint URL entirely. | region-based |

**Regions:**
- US (default): `https://mcp.newrelic.com/mcp/`
- EU: `https://mcp.eu.newrelic.com/mcp/`

**Available tool tags:** `discovery`, `data-access`, `alerting`, `incident-response`, `performance-analytics`, `advanced-analysis`.

**Permissions:** The API key user must have an organization-level role (`Organization Read Only`, `Organization Manager`, or `Organization Product Admin`) and the "New Relic AI MCP Server" preview must be enabled in the New Relic UI.

See the [New Relic MCP docs](https://docs.newrelic.com/docs/agentic-ai/mcp/setup/) for API key creation and OAuth setup.

---

## Baseline patterns

The database is pre-seeded with curated patterns from real-world Go projects:

| Category | IDs | Count | Source |
|---|---|---|---|
| Anti-patterns | AP-1 – AP-7 | 7 | General Go best practices |
| Concurrency | CONC-1 – CONC-6 | 6 | K8s, Prometheus, VictoriaMetrics, OTel Go |
| Error handling | ERR-1 – ERR-6 | 6 | K8s, Grafana, Thanos |
| Testing | TEST-1 – TEST-6 | 6 | Prometheus, Crossplane, cert-manager |
| Operator | OP-1 – OP-14 | 14 | K8s, Gardener, Crossplane, Flux, Chaos-Mesh |
| GitOps | GITOPS-1 – GITOPS-6 | 6 | Flux2, ArgoCD |
| Mesh/Proxy | MESH-1 – MESH-16 | 16 | Traefik, Linkerd2, Istio, gRPC-Go |
| Distributed systems | DIST-1 – DIST-8 | 8 | etcd, Vault, Cilium |
| Auth | AUTH-1 – AUTH-6 | 6 | StackRox, Vault, Zitadel |
| Observability | OBS-1 – OBS-10 | 10 | Thanos, OTel Go |
| Dockerfile | DOCKER-1 – DOCKER-15 | 15 | Multi-stage builds, distroless, security |
| Helm chart | HELM-1 – HELM-15 | 15 | Standard labels, RBAC, security context |
| K8s resources | K8SRES-1 – K8SRES-16 | 16 | PSA, NetworkPolicy, probes, CRD schemas |
| OWASP Go baseline | A01-A10 | 30+ | SQL injection, hardcoded secrets, insecure TLS |

---

## Project layout

```
go-guardian/
├── .claude-plugin/
│   ├── plugin.json              # Plugin manifest (name, version, description)
│   └── marketplace.json         # Marketplace catalog
├── .mcp.json                    # MCP server config (uses ${CLAUDE_PLUGIN_DATA} paths)
├── CLAUDE.md                    # Claude Code operating instructions (plugin layer map)
├── agents/                      # Claude Code agent definitions
│   ├── orchestrator.md          #   Central routing + plugin-aware coordination
│   ├── reviewer.md              #   Code review (delegates to team-reviewer for large PRs)
│   ├── security.md              #   OWASP + CVE (escalates to security-auditor)
│   ├── linter.md                #   Lint + learning loop
│   ├── tester.md                #   Test quality
│   ├── patterns.md              #   Anti-pattern detection
│   ├── advisor.md               #   Renovate config analysis + learning
│   └── newrelic.md              #   New Relic observability
├── skills/                      # Slash command definitions
│   ├── go/SKILL.md              #   /go
│   ├── go-review/SKILL.md       #   /go-review
│   ├── go-security/SKILL.md     #   /go-security
│   ├── go-lint/SKILL.md         #   /go-lint
│   ├── go-test/SKILL.md         #   /go-test
│   ├── go-patterns/SKILL.md     #   /go-patterns
│   ├── renovate/SKILL.md        #   /renovate
│   └── newrelic/skill.md        #   /newrelic
├── hooks/                       # Claude Code hook scripts
│   ├── hooks.json               #   Plugin hook event → script mapping
│   ├── session-start.sh         #   Build binary + gateway config + staleness check
│   ├── post-bash.sh             #   Learning loop + auto-prefetch on go.mod changes
│   ├── pre-write-go.sh          #   Prevention injection before Write tool
│   └── pre-edit-go.sh           #   Prevention injection before Edit tool
├── mcp-server/                  # Go MCP server (the learning engine)
│   ├── main.go                  #   17 MCP tools registered
│   ├── go.mod
│   ├── db/
│   │   ├── store.go
│   │   └── seed/                #   Baseline pattern SQL files
│   ├── tools/                   #   MCP tool handlers + tests (284 tests)
│   └── owasp/                   #   OWASP rule engine
├── gateway/
│   ├── standalone/config.yaml   #   AgentGateway standalone config
│   ├── kubernetes/              #   K8s manifests (team deployment)
│   ├── openapi/                 #   OpenAPI schemas (NVD, GHSA, go-vuln)
│   └── newrelic/                #   New Relic MCP bridge container
│       ├── Dockerfile           #     Multi-target: distroless (default) + slim (K8s init)
│       ├── bridge.mjs           #     Entry point (env → mcp-remote args)
│       ├── package.json         #     mcp-remote dependency
│       └── Makefile             #     docker-build / docker-push-multi
├── docs/
│   └── cve-fetching.md          #   CVE fetch strategy, HTTP budget, CWE mapping
├── golangci-lint.template.yml   #   Recommended linter config
├── settings-template.json       #   Claude Code settings template (standalone mode)
└── install.sh                   #   Standalone installer (fallback)
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

## Troubleshooting

### Agents, skills, or @file not loading

Claude Code uses `ripgrep` (`rg`) to discover plugin components. If it's missing:

```bash
# macOS
brew install ripgrep

# Debian/Ubuntu
sudo apt install ripgrep

# Alpine
apk add ripgrep
```

After installing, set `USE_BUILTIN_RIPGREP=0` in your environment or settings if the bundled version still fails. The session-start hook will warn on startup if `rg` is not found.

### MCP server won't start

Run `/doctor` inside Claude Code — it checks for MCP server configuration errors, plugin/agent loading failures, and context usage warnings.

Common causes:
- **Go not installed**: the MCP binary is built from source on first session. Requires Go 1.22+.
- **Build cache stale**: delete `${CLAUDE_PLUGIN_DATA}/go-guardian-mcp` (plugin mode) or `.go-guardian/go-guardian-mcp` (standalone) to force rebuild.
- **Corporate proxy**: if behind a TLS-intercepting proxy, set `NODE_EXTRA_CA_CERTS=/path/to/corporate-ca.pem` before launching Claude Code.

### Hooks not firing

1. Check `if` field syntax in `hooks.json` — permission rule format like `Edit(*.go)`, not glob
2. Verify scripts are executable: `chmod +x hooks/*.sh`
3. Hook stdout is capped at 10,000 characters — excess is silently dropped
4. `async: true` hooks ignore exit codes and stdout (fire-and-forget)

### Agent teams integration

go-guardian agents work as both subagents and agent-team teammates. If using agent teams (`CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS=1`):
- Teammates load CLAUDE.md, MCP servers, and skills automatically — go-guardian agents will work
- The `TaskCompleted` hook enforces `go build` and `go vet` gates before tasks can be marked complete
- Avoid two teammates editing the same Go file — break work by file ownership

---

## License

See LICENSE file.
