---
name: go
description: Central Go development orchestrator — dispatches to review, security, lint, test, patterns, renovate, dependency, documentation, and feature-lifecycle workflows. Use when the user works on Go code (`.go`, `go.mod`, `go.sum`) and asks to review, check, analyze, scan, fix, audit, plan, implement, validate, or document it — even without explicitly saying "Go". Serves as the entry point for all `/go <subcommand>` invocations.
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
empty work queue are not activated.

## Gotchas

- **Do NOT delegate the Stage 1 classifier to a subagent.** It must run
  in the main conversation so the MoE gate stays near-zero latency — a
  subagent round-trip defeats the feature this skill exists to enforce.
- **Stage 1 is rule-based (keyword + path regex), not LLM-backed.**
  Keep it that way. An LLM round-trip on every `/go` invocation would
  break the hot-path cost.
- **`go-guardian scan --all` is SQLite-hash cached.** A second run on
  unchanged source is near-zero cost. Do not add a force-rescan flag
  unless the user explicitly asks.
- **Stage 3 skips reviewers with empty findings.** "Empty" means no
  severity markers (`HIGH`/`CRITICAL`/`MEDIUM`/`LOW`) and no `file:line`
  citations — a header line or "no findings detected" sentinel still
  counts as empty.
- **References are loaded on demand.** The Stage 1 classifier tables
  live in `references/classifier.md`; the Stage 3 dimension map lives
  in `references/dimension-map.md`. Only load them on the full-scan hot
  path, not on explicit-subcommand routing.

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

Classify which Go review dimensions are relevant. Use a **rule-based**
classifier — lookup tables and regex only, **no LLM call**. The
classifier must be deterministic so repeat invocations on the same
input produce the same routing.

Load `references/classifier.md` for the two signal tables (keywords by
dimension, file-path biases) and the union rule. Apply both signals via
`git diff --name-only HEAD` and user-message keyword matching, union
the results, and emit the relevant set. If the union is empty, return
immediately — do NOT run the scan.

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

Load `references/dimension-map.md` for the dimension → artifact →
spawn-target table, the emptiness test, and the gating rule. Apply them
to each dimension in the relevant set.

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
