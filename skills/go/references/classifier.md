# Stage 1 Classifier — Dimension Lookup Tables

Two signals feed the rule-based classifier. Match user-message keywords and
changed-file paths, then take the **union** of both signal sets as the
relevant dimension set.

## Signal A — user message keywords

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

## Signal B — changed files via `git diff --name-only HEAD`

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

## Union rule

Emit a set drawn from `{review, security, lint, test, patterns, deps,
staleness, renovate}`. If the union is empty (e.g. "explain this Python
script"), print "no Go dimensions relevant" and return immediately — do
NOT run the scan.
