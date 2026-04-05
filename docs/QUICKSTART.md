# Go Guardian — Quick Start Guide

## Installation

```bash
# Add the marketplace
/plugin marketplace add kengou/go-guardian

# Install the plugin
/plugin install go-guardian
```

On first session start, the `session-start.sh` hook automatically:
1. Builds the MCP server binary (requires Go 1.26+)
2. Initializes the SQLite database with seed patterns from 37 projects
3. Generates a session ID for cross-agent communication

## Basic Usage

### One Command for Everything

```bash
/go                              # Full scan (lint + vet + test + security + patterns)
/go review                       # Code review with learned patterns
/go security                     # OWASP + CVE scan + deep security audit
/go lint                         # Lint and learn from fixes
/go test                         # Run tests with coverage analysis
/go patterns                     # Browse learned pattern library
/go docs                         # Generate project documentation
/go design <topic>               # Start feature design (beastmode)
```

### Direct Skill Invocation

```bash
/go-security                     # Security scan
/go-review                       # Code review
/go-lint                         # Lint + learn
/go-test                         # Test runner
/go-patterns                     # Pattern library
/go-patterns stats               # Learning dashboard
/renovate                        # Analyze renovate.json
```

## How It Learns

### Automatic (hooks — zero effort)

1. **Edit/Write a `.go` file** → `pre-edit-go.sh` queries learned patterns and warns about known issues
2. **Run `golangci-lint`** → `post-bash.sh` captures output + diff, learns new patterns automatically
3. **Change `go.mod`** → `on-gomod-change.sh` triggers dependency vulnerability check

### Explicit (agent calls)

1. **After code review fix accepted** → `learn_from_review` stores the finding as a reusable pattern
2. **After security scan** → `report_finding` shares findings with other agents in the session

## Common Workflows

### Full Project Scan

```bash
/go
```

Runs in 3 phases:
1. **MCP + automated tools**: golangci-lint, go vet, go test -race, govulncheck, check_owasp, check_deps
2. **Deep analysis**: `/team-spawn security` (4 reviewers) + `/team-review --reviewers performance,architecture,testing`
3. **Consolidated report** with trend data

### Security Audit

```bash
/go security
```

Two layers:
1. **MCP tools**: check_owasp (pattern matching), check_deps (CVE cache), govulncheck (Go vuln DB)
2. **team-spawn security**: 4 parallel reviewers (OWASP/vulns, auth/access, dependencies, secrets/config)

### Code Review Before PR

```bash
/go review
```

1. Queries learned patterns for files being reviewed
2. Runs go build + go vet + golangci-lint
3. Spawns parallel reviewers (security, performance, architecture)
4. After you accept a fix → learned for future sessions

### Feature Development

```bash
/go design add user caching        # Design interview → PRD
/go plan caching                    # Decompose into features
/go implement caching-redis-layer   # Build with task orchestration
/go validate caching                # Verify before release
```

## Environment Variables

| Variable | Purpose |
|----------|---------|
| `NVD_API_KEY` | Enables CVSS score enrichment for CVE findings |
| `GITHUB_TOKEN` | Higher rate limits for GitHub Advisory API |
| `GO_GUARDIAN_ADMIN_PORT` | Enable admin web UI (e.g., `8080`) |

## Diagnostics

```bash
# Run healthcheck
/go-doctor

# Check what's in the learning database
/go-patterns stats

# Check scan staleness
/go  # (shows stale warnings automatically)
```

## Troubleshooting

| Problem | Cause | Fix |
|---------|-------|-----|
| MCP tools not called | Skills delegating to subagents | Update to v0.2.8+ (skills call MCP directly) |
| check_owasp rejects paths | projectDir wrong | Update to v0.2.5+ (defaults to CWD) |
| Agent names doubled | `go-guardian:go-guardian:X` | Update to v0.2.6+ (name prefix fixed) |
| Slow startup | Building MCP binary | Normal on first run; cached after |
| No patterns learned | Hooks not firing | Check `hooks.json` is loaded |
