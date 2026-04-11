---
name: go
description: Central Go development orchestrator. MoE gating: classifies intent, runs one unified scan, spawns only reviewers with non-empty findings.
argument-hint: "[scan|review|test|lint|security|deps|design|plan|implement|validate|docs|explain|diagram|adr|api-docs] [path|topic]"
paths: "*.go,go.mod,go.sum"
tools:
  - mcp__go-guardian__query_knowledge
  - mcp__go-guardian__suggest_fix
  - Read
  - Bash
  - Grep
  - Glob
---

# /go — Go Guardian Orchestrator (MoE Gating)

This skill is the central entry point for Go work. It operates as a
**Mixture-of-Experts** gating network: it classifies intent, runs a
single unified scan via `go-guardian scan --all`, reads the resulting
markdown artifacts in `.go-guardian/`, and then spawns **only** the
reviewer agents whose dimensions have non-empty findings. Experts with an
empty work queue are not activated. Do NOT delegate routing to a subagent —
the classifier must run in the main conversation so it stays near-zero
latency.

## Explicit Subcommand Routing

If the user invoked `/go` with an explicit subcommand, route directly to
the matching per-dimension skill or beastmode verb and return. The
four-stage gating pipeline below runs only for argument-less invocations.

**Per-dimension skills** — delegate and return:
- `review` → invoke `/go-review`
- `security` → invoke `/go-security`
- `lint` → invoke `/go-lint`
- `test` → invoke `/go-test`
- `patterns` → invoke `/go-patterns`
- `renovate` → invoke `/renovate`

**Beastmode lifecycle** — delegate and return:
- `design` → invoke `/beastmode:design <topic>`
- `plan` → invoke `/beastmode:plan <epic-name>`
- `implement` → invoke `/beastmode:implement <epic-name>-<feature-name>`
- `validate` → invoke `/beastmode:validate <epic-name>`

Keywords that trigger beastmode routing:
- **design**: "design", "new feature", "add feature", "feature request", "PRD", "spec"
- **plan**: "plan", "break down", "decompose", "task breakdown"
- **implement**: "implement", "build", "develop", "code this", "create feature"
- **validate**: "validate", "verify", "release check", "pre-release"

**Documentation routing** — delegate and return:
- `docs` → invoke `/doc-generate`
- `explain` → invoke `/code-explain`
- `changelog` → invoke `/changelog-automation`
- `docs --full` → invoke `docs-architect` agent, then `mermaid-expert`, then `reference-builder`
- `diagram` → invoke `mermaid-expert` agent
- `adr` → invoke `/architecture-decision-records`
- `api-docs` → invoke `/openapi-spec-generation`

Keywords:
- **docs**: "docs", "documentation", "document", "readme", "generate docs"
- **docs --full**: "full docs", "comprehensive docs", "technical manual", "ebook"
- **explain**: "explain", "how does this work", "walk me through", "what does this do"
- **diagram**: "diagram", "architecture diagram", "flowchart", "mermaid", "ERD", "sequence diagram"
- **adr**: "ADR", "architecture decision", "decision record", "document decision"
- **api-docs**: "API docs", "OpenAPI", "swagger", "API reference", "API spec"
- **changelog**: "changelog", "release notes", "what changed"

## Full Scan — MoE Gating Pipeline

When invoked with no subcommand on a project containing `go.mod`, run the
four-stage gating pipeline. This is the hot path; it must stay fast.

### Stage 1: Intent Classification (rule-based, deterministic)

Classify which Go review dimensions are relevant to this session. Use a
**rule-based** classifier — lookup tables and regex only, **no LLM call**.
`/go` runs near the top of every relevant session; an LLM classification
round-trip would defeat the latency goal this feature exists to enforce.
The classifier must be deterministic so repeat invocations on the same
input produce the same routing decision.

Two signals feed the classifier:

**Signal A — user message keywords.** Match keywords and phrases in the
user's message against dimension hints:

| Dimension | Keywords |
|-----------|----------|
| review    | "review", "audit", "look at" |
| security  | "security", "auth", "crypto", "secret", "token", "password", "owasp", "cve" |
| lint      | "lint", "format", "style", "golangci" |
| test      | "test", "coverage", "race", "flake", "fixture" |
| patterns  | "pattern", "anti-pattern", "architecture", "design", "smell" |
| deps      | "deps", "dependency", "vuln", "govulncheck", "go.mod", "upgrade" |
| staleness | "stale", "freshness", "rescan" |
| renovate  | "renovate", "renovate.json", "dependency bot" |

**Signal B — changed files via git state.** Run `git diff --name-only HEAD`
via Bash and bias dimensions from path prefixes and extensions:

| Path signal | Biases ON |
|-------------|-----------|
| `crypto/`, `auth/`, `internal/sql/`, `*secrets*`, `*.pem` | security |
| `*_test.go` | test |
| `Dockerfile`, `*.Dockerfile` | patterns, security |
| `Chart.yaml`, `values.yaml`, `templates/*.yaml` | patterns, security |
| `*.go`, `go.mod`, `go.sum` (general) | review, lint, patterns |
| `go.mod`, `go.sum` | deps |
| `renovate.json`, `.renovaterc*` | renovate |
| only `README.md`, `docs/**`, `*.md` | (no Go dimensions — return early) |

**Emit the relevant set.** Union the two signals into a set drawn from
`{review, security, lint, test, patterns, deps, staleness, renovate}`.
If the union is empty (e.g. "explain this Python script"), print "no Go
dimensions relevant" and return immediately — do NOT run the scan.

### Stage 2: Single Unified Scan

Run exactly once per `/go` invocation via Bash:

```bash
go-guardian scan --all
```

This command is warm-start cached by the SQLite file-hash cache in the
`go-guardian` binary; a second run on unchanged source returns in
milliseconds. The scan produces six markdown artifacts in `.go-guardian/`
that drive the Stage 3 gating decision:

- `.go-guardian/owasp-findings.md`
- `.go-guardian/dep-vulns.md`
- `.go-guardian/staleness.md`
- `.go-guardian/pattern-stats.md`
- `.go-guardian/health-trends.md`
- `.go-guardian/session-findings.md`

Do NOT spawn any review agent in this stage — the orchestrator itself is
doing the scan. Agents only get spawned after Stage 3 decides they have
work to do.

### Stage 3: Empty-Findings Short-Circuit (the MoE Gate)

For each dimension in the relevant set from Stage 1, read the matching
artifact file and decide whether to spawn its reviewer. The mapping from
dimension to artifact to spawn target:

| Dimension | Artifact                             | Spawn target   |
|-----------|--------------------------------------|----------------|
| review    | `.go-guardian/session-findings.md`   | `/go-review`   |
| security  | `.go-guardian/owasp-findings.md`     | `/go-security` |
| deps      | `.go-guardian/dep-vulns.md`          | `/go-security` |
| staleness | `.go-guardian/staleness.md`          | (report only)  |
| patterns  | `.go-guardian/pattern-stats.md`      | `/go-patterns` |
| lint      | `.go-guardian/session-findings.md`   | `/go-lint`     |
| test      | `.go-guardian/session-findings.md`   | `/go-test`     |
| renovate  | (renovate.json validation)           | `/renovate`    |

**Emptiness test.** A findings file is "empty" if, after trimming
whitespace, it contains no finding entries — the file may still carry a
header or a "no findings detected" sentinel line. Use `Grep` or a plain
`Read` to check for finding markers (severity labels like `HIGH`,
`CRITICAL`, `MEDIUM`, `LOW`, or `file:line` citations).

**Gating rule.** For each relevant dimension whose artifact is empty, do
**not** spawn the reviewer — record the skip with reason "empty findings
artifact". For each relevant dimension whose artifact has findings,
invoke the matching per-dimension skill via slash command. Each spawned
skill reads the same scan artifacts (never re-running the scan) and
drops its own refined findings into `.go-guardian/inbox/` as markdown
documents, which the Stop hook flushes into the knowledge base at session
end.

Use `query_knowledge` to pull learned patterns relevant to the spawned
dimensions. Use `suggest_fix` when the orchestrator wants to offer an
inline fix preview before spawning `/go-review`.

### Stage 4: Run Report

After the pipeline completes, print a concise run report naming:

1. **Relevant dimensions** — with a one-line classifier reason per
   dimension (e.g. `security: keyword "auth" matched`).
2. **Irrelevant dimensions** — with the reason each was excluded (e.g.
   `deps: no go.mod/go.sum changes`).
3. **Spawned reviewers** — with the findings count that triggered each
   spawn.
4. **Skipped reviewers** — with "empty findings artifact" as the reason.
5. **Wall-clock cost** — total duration of the `/go` invocation.

The Stop hook at `hooks/hooks.json` ingests `.go-guardian/inbox/` via
`go-guardian ingest` regardless of whether any reviewer spawned, so the
learning loop stays closed even on a clean run.

## Idempotence

A second `/go` invocation on unchanged source must be a small fraction of
the first run's wall-clock time. This is enforced by the warm-start
cache in `go-guardian scan --all` — the scan reads file hashes from
SQLite and skips any file whose content has not changed since the last
scan. The classifier, artifact reads, and gating logic are stateless and
cheap, so the only non-negligible cost on a repeat run is the incremental
scan itself, which on unchanged source is near-zero.

Use the `go-guardian:orchestrator` agent definition (loaded in
conversation context) for the full intent classification table, force
routes, and context injection rules.
