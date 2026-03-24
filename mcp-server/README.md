# go-guardian MCP server

The learning engine behind go-guardian. A Go binary that exposes 7 MCP tools over stdio and persists all knowledge in a local SQLite database.

---

## Table of Contents

1. [What it does](#what-it-does)
2. [Package layout](#package-layout)
3. [Build](#build)
4. [CLI reference](#cli-reference)
5. [MCP tools](#mcp-tools)
6. [Database schema](#database-schema)
7. [Seed data](#seed-data)
8. [Data sources](#data-sources)
9. [Adding patterns](#adding-patterns)
10. [Testing](#testing)

---

## What it does

The MCP server sits between Claude Code and a SQLite database. When Claude Code calls a tool, the server:

1. Parses the input (lint output, diff, file path, module list, etc.)
2. Reads from or writes to `guardian.db`
3. Returns structured text that Claude Code agents inject into their context

The database grows with every session — each `learn_from_lint` call stores a new DON'T/DO pair, so the same mistake is never suggested twice.

```
Claude Code agent
      │ MCP (stdio)
      ▼
go-guardian-mcp  ←→  guardian.db (SQLite, WAL mode)
      │
      ▼
External APIs (optional):
  vuln.go.dev    — Go vulnerability database
  nvd.nist.gov   — CVSS v3.1 enrichment
  api.github.com — GHSA OWASP-tagged advisories
```

---

## Package layout

```
mcp-server/
├── main.go              — Binary entrypoint: flags, DB init, MCP server setup, tool registration
├── go.mod               — Module: github.com/kengou/go-guardian/mcp-server, Go 1.26
├── db/
│   ├── store.go         — All DB access: schema DDL, CRUD methods, PatternStats aggregation
│   ├── schema.sql        — Reference DDL (source of truth for the embedded schema in store.go)
│   └── seed/            — SQL files embedded at build time and loaded on first DB init
│       ├── notque_anti_patterns.sql   — AP-1 – AP-7: general Go anti-patterns
│       ├── notque_concurrency.sql     — CONC-1 – CONC-6: goroutine / channel patterns
│       ├── notque_error_handling.sql  — ERR-1 – ERR-6: error handling patterns
│       ├── notque_testing.sql         — TEST-1 – TEST-6: test quality patterns
│       └── owasp_go_baseline.sql      — A01-A10 baseline OWASP patterns for Go
├── owasp/
│   ├── rules.go         — AST + regex rule engine, DefaultRules(), ScanFile(), ScanDirectory()
│   └── rules_test.go
└── tools/               — One file per MCP tool handler + shared helpers
    ├── learn.go          — learn_from_lint: parse lint output + diff → store patterns
    ├── knowledge.go      — query_knowledge: return relevant patterns for a file/context
    ├── owasp.go          — check_owasp: run OWASP rule engine, persist findings
    ├── staleness.go      — check_staleness: compare scan history against thresholds
    ├── deps.go           — check_deps: look up modules in vuln cache, return status
    ├── stats.go          — get_pattern_stats: aggregate dashboard data
    ├── suggest.go        — suggest_fix: snippet similarity search over knowledge base
    ├── prefetch.go       — FetchVulns(): fetch vuln.go.dev + NVD enrichment (--prefetch mode)
    ├── owasp_update.go   — UpdateOWASPRules(): fetch GHSA advisories (--update-owasp mode)
    └── *_test.go         — Table-driven tests for every tool
```

---

## Build

```bash
cd mcp-server
go build -ldflags="-s -w" -o go-guardian-mcp .
```

No CGo. The SQLite driver (`modernc.org/sqlite`) is pure Go — the binary is self-contained.

**Requirements:** Go 1.23+

---

## CLI reference

```
go-guardian-mcp [flags]
```

| Flag | Default | Description |
|---|---|---|
| `--db` | `.go-guardian/guardian.db` | Path to the SQLite database file. Created automatically if missing. |
| `--version` | — | Print version and exit. |
| `--prefetch` | — | Fetch CVE data for all modules in `go.mod` and exit (no MCP server started). |
| `--go-mod` | `go.mod` | Path to `go.mod` used by `--prefetch`. |
| `--nvd-key` | `$NVD_API_KEY` | NVD API key for CVSS v3.1 enrichment. Without a key severity shows as UNKNOWN. |
| `--update-owasp` | — | Fetch GHSA advisories tagged with OWASP-relevant CWEs and update rule patterns, then exit. |
| `--github-token` | `$GITHUB_TOKEN` | GitHub personal access token for GHSA API. Without a token rate-limited to 60 req/hr. |

### Normal operation (MCP server over stdio)

```bash
go-guardian-mcp --db /path/to/project/.go-guardian/guardian.db
```

### Pre-populate CVE cache

```bash
go-guardian-mcp --prefetch --db .go-guardian/guardian.db \
  --go-mod go.mod --nvd-key $NVD_API_KEY
```

Output:
```
Prefetch complete: 42 modules checked, 3 CVEs found, 3 enriched via NVD
```

### Refresh OWASP rule patterns

```bash
go-guardian-mcp --update-owasp --db .go-guardian/guardian.db \
  --github-token $GITHUB_TOKEN
```

Output:
```
OWASP update complete: 87 advisories fetched, 12 patterns stored
  A03: 5 new patterns
  A02: 4 new patterns
  A09: 3 new patterns
```

---

## MCP tools

All tools communicate over stdio using the MCP protocol. Claude Code agents call them via `mcp__go-guardian__<tool_name>`.

### `learn_from_lint`

Records a golangci-lint finding and its fix as a reusable DON'T/DO pattern.

**Input:**

| Parameter | Required | Description |
|---|---|---|
| `diff` | Yes | Unified diff of the fix (`git diff` format) |
| `lint_output` | Yes | Raw golangci-lint output captured before the fix |
| `project` | No | Project identifier (informational only) |

**What it does:**
1. Parses lint output with a regex that extracts `file:line:col: message (linter)` lines
2. Parses the unified diff into per-file before/after hunks
3. Matches each lint finding to the hunk from the same file by basename
4. Upserts into `lint_patterns` — on conflict it increments `frequency` (same pattern seen again)
5. If no lint output is provided but a diff exists, stores it as a `diff-only` pattern

**Output:** JSON — `{"learned": N, "updated": M, "patterns": [{"rule": "...", "file_glob": "..."}]}`

**File glob logic:** The basename drives specificity — `auth_handler.go` → `*_handler.go`, `server_test.go` → `*_test.go`, anything else → `*.go`.

---

### `query_knowledge`

Returns learned patterns, anti-patterns, and OWASP findings relevant to a file being written.

**Input:**

| Parameter | Required | Description |
|---|---|---|
| `file_path` | No | Path to the Go file being written or edited |
| `code_context` | No | First ~1000 chars of the code about to be written |
| `project` | No | Project identifier |

**What it does:**
1. Derives a file glob from `file_path` (e.g. `*_handler.go`)
2. Queries `lint_patterns` matching that glob, ordered by frequency (top 5)
3. Infers an anti-pattern category from keywords in `code_context` (`goroutine`/`chan` → concurrency, `err` → error-handling, etc.) and queries `anti_patterns` (top 3)
4. Queries `owasp_findings` matching the glob (top 3)

**Output:** Formatted text block injected into the agent context:
```
LEARNED PATTERNS FOR THIS CONTEXT:
• [lint:errcheck ×7] if err != nil { return }
  → DO: return fmt.Errorf("...: %w", err)
• [pattern:ERR-2] errors.New("...")
  → DON'T: return errors.New("operation failed: " + err.Error())
  → DO: return fmt.Errorf("operation failed: %w", err)
```

---

### `check_owasp`

Scans Go source files for OWASP Top 10 security issues using AST + regex rules.

**Input:**

| Parameter | Required | Description |
|---|---|---|
| `path` | Yes | File path or directory to scan |

**Categories checked:**
- A02: weak crypto (MD5/SHA1 for hashing), hardcoded secrets, `InsecureSkipVerify`
- A03: SQL injection via `fmt.Sprintf`, command injection, unsafe template casts
- A05: `pprof` in production, wildcard CORS, debug flags
- A09: sensitive data in logs (`password`, `token`, `secret` in log calls)
- A10: SSRF via unvalidated URL construction

**What it does:**
1. Walks the directory (or scans a single file)
2. Runs `owasp.DefaultRules()` against each `.go` file
3. Persists each finding to `owasp_findings` (increments frequency on repeat)
4. Returns a grouped report by severity

**Output:** Formatted text with findings grouped by OWASP category, file, and line number.

---

### `check_staleness`

Reports which scan types are overdue for a project.

**Input:**

| Parameter | Required | Description |
|---|---|---|
| `project_path` | Yes | Filesystem path to the project root |

**Thresholds:**

| Scan type | Threshold | Refresh command |
|---|---|---|
| `vuln` | 3 days | `/go security` |
| `owasp` | 7 days | `/go security` |
| `full` | 14 days | `/go` |
| `owasp_rules` | 30 days | `go-guardian-mcp --update-owasp` |
| `lint` | never stale | continuous learning |

**Output:** Human-readable staleness report:
```
Stale scans detected:
  ⚠ vuln scan: 5 days ago (threshold: 3 days) — run: /go security
  ✓ owasp scan: 2 days ago
```

The project ID is derived from the last two path components (e.g. `/home/user/myproject` → `user/myproject`).

---

### `check_deps`

Analyses Go module dependencies for known CVEs using the cached vulnerability data.

**Input:**

| Parameter | Required | Description |
|---|---|---|
| `modules` | Yes | Array of module paths, e.g. `["github.com/gorilla/mux"]` |

**Status values:**

| Status | Meaning |
|---|---|
| `PREFER` | No CVEs found in cache |
| `CHECK LATEST` | CVEs exist but a fixed version is available |
| `AVOID` | CVEs exist with no known fix |
| `UNKNOWN` | No cache data — run `--prefetch` to populate |

**What it does:**
1. Looks up each module in `vuln_cache`
2. Checks whether cache is stale (>24h old) — flags it if so
3. Returns any recorded `dep_decisions` (prior AVOID/PREFER judgements)
4. Recommends running `--prefetch` for modules with no cache data

**Output:** Per-module status with CVE IDs, severity (CVSS v3.1 if enriched), affected versions, and fix version.

---

### `get_pattern_stats`

Returns an aggregate dashboard of the knowledge base state.

**Input:**

| Parameter | Required | Description |
|---|---|---|
| `project` | No | Project identifier for scan history |

**Output:**
- Top 10 lint patterns by frequency (rule, frequency, first line of DON'T code)
- OWASP finding counts grouped by category (A02, A03, etc.)
- Total lint pattern count and anti-pattern count
- Recent scan history for the project (last 10 scans)

---

### `suggest_fix`

Searches the knowledge base for patterns matching a problematic code snippet and returns up to 3 suggested fixes.

**Input:**

| Parameter | Required | Description |
|---|---|---|
| `code_snippet` | Yes | The problematic code to find fixes for |
| `issue_type` | No | Filter: `lint`, `owasp`, `pattern`, or empty for all |

**Similarity algorithm:** Token-level substring matching — counts what fraction of non-trivial lines from the stored `dont_code` appear in the snippet. Scores 0.0–1.0; results labelled `high` (≥0.6), `medium` (≥0.3), or `low`.

**Output:** Up to 3 matches with confidence labels, DON'T/DO code blocks.

---

## Database schema

Six tables, all created automatically on first run. WAL mode and foreign keys are enabled.

| Table | Purpose | Key columns |
|---|---|---|
| `lint_patterns` | Learned DON'T/DO pairs from golangci-lint fixes | `rule`, `file_glob`, `dont_code`, `do_code`, `frequency` |
| `anti_patterns` | Pre-seeded and manually added anti-patterns | `pattern_id`, `category`, `dont_code`, `do_code` |
| `owasp_findings` | OWASP scan results, persisted per finding | `category`, `file_pattern`, `finding`, `fix_pattern`, `frequency` |
| `vuln_cache` | Cached CVE data from vuln.go.dev + NVD | `module`, `cve_id`, `severity`, `affected_versions`, `fixed_version` |
| `dep_decisions` | Recorded PREFER/AVOID decisions per module | `module`, `decision`, `reason`, `cve_count` |
| `scan_history` | Timestamp of last run per scan type per project | `scan_type`, `project`, `last_run`, `findings_count` |

**Upsert behaviour:**
- `lint_patterns`: `ON CONFLICT(rule, file_glob, dont_code) DO UPDATE SET frequency = frequency + 1` — the same pattern seen again just increments the counter
- `vuln_cache`: `ON CONFLICT(module, cve_id) DO UPDATE` — refreshed on every prefetch
- `scan_history`: `ON CONFLICT(scan_type, project) DO UPDATE SET last_run` — one row per scan type per project

---

## Seed data

Five SQL files are embedded in the binary at build time (`//go:embed seed/*.sql`). They are loaded once when the `anti_patterns` table is empty (i.e. first database init).

| File | Patterns | IDs |
|---|---|---|
| `notque_anti_patterns.sql` | General Go anti-patterns | AP-1 – AP-7 |
| `notque_concurrency.sql` | Goroutine / channel misuse | CONC-1 – CONC-6 |
| `notque_error_handling.sql` | Error wrapping and propagation | ERR-1 – ERR-6 |
| `notque_testing.sql` | Table tests, t.Helper(), race tests | TEST-1 – TEST-6 |
| `owasp_go_baseline.sql` | OWASP A01-A10 Go-specific patterns | A01-A10 (30+) |

Seed data is never overwritten. To update a seed pattern, add a new SQL file or modify the existing one and re-initialize the database.

---

## Data sources

### Go vulnerability database (`vuln.go.dev`)

Fetched in a single HTTP call: `https://vuln.go.dev/api/vulns/list`. Each advisory is cross-referenced against `go.mod` modules. With an NVD API key each CVE is enriched with CVSS v3.1 severity. See `tools/prefetch.go`.

### NVD CVSS enrichment (`services.nvd.nist.gov`)

One API call per CVE found in the vuln database. Rate-limited to 650ms/call without a key. With `NVD_API_KEY` the limit is 50 req/30s. Get a free key at `https://nvd.nist.gov/developers/request-an-api-key`.

### GitHub Security Advisories (`api.github.com/graphql`)

Fetched by `--update-owasp` using the GHSA GraphQL API. Queries advisories with CWEs mapped to OWASP A01-A10. Paginated at 100 results/page. Without a GitHub token: 60 req/hr (usually sufficient for monthly refresh). With `GITHUB_TOKEN`: 5000 req/hr. See `tools/owasp_update.go`.

---

## Adding patterns

### Via the learning loop (preferred)

Run golangci-lint, fix the issues, then call `learn_from_lint` with the diff. The tool handles everything automatically.

### Via seed SQL files

Add a new `.sql` file to `db/seed/` following the existing format:

```sql
INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
VALUES (
    'AP-8',
    'Using init() for application setup',
    'func init() { db = connectDB() }',
    'func NewApp(cfg Config) (*App, error) { db, err := connectDB(cfg.DSN); ... }',
    'notque',
    'general'
);
```

Rebuild the binary. The new pattern is loaded on the next fresh database init (or by deleting the existing `guardian.db`).

### Via direct SQL

```bash
sqlite3 .go-guardian/guardian.db \
  "INSERT OR IGNORE INTO anti_patterns (pattern_id, description, dont_code, do_code, source, category)
   VALUES ('LOCAL-1', 'My custom pattern', 'bad code', 'good code', 'local', 'general');"
```

---

## Testing

```bash
cd mcp-server
go test -count=1 -timeout=30s ./...
```

133 tests across 4 packages. All tests use in-memory SQLite (`":memory:"`) — no external dependencies or file system state.

**Test packages:**

| Package | Tests | What they cover |
|---|---|---|
| `db` | Store CRUD, schema migration, seed loading | All store methods including upsert conflict logic |
| `tools` | Every MCP tool handler | Parsing, matching, formatting, edge cases |
| `tools` (e2e) | End-to-end: learn → query → suggest cycle | Full learning loop integration |
| `owasp` | Rule engine | Pattern matching, file scanning |

**Key test patterns:**
- Rate-limit delays are disabled in tests via `PageDelay: -1` / `NVDDelay: -1` (negative = skip)
- HTTP calls use `httptest.NewServer` — no real network access
- Pagination mock servers use `r.URL.Query().Get("page")` for exact matching (not substring)
