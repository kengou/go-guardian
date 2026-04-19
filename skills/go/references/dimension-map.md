# Stage 3 — Dimension → Artifact → Spawn Map

For each dimension in the relevant set from Stage 1, read the matching
artifact file and decide whether to spawn its reviewer.

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

## Emptiness test

A findings file is "empty" if, after trimming whitespace, it contains no
finding entries. The file may still carry a header or a "no findings
detected" sentinel — those still count as empty. Use `Grep` or a plain
`Read` to check for finding markers:

- severity labels: `HIGH`, `CRITICAL`, `MEDIUM`, `LOW`
- `file:line` citations

## Gating rule

- **Empty artifact** → do NOT spawn the reviewer. Record the skip with
  reason "empty findings artifact".
- **Artifact has findings** → invoke the matching per-dimension skill via
  slash command.

Each spawned skill reads the same scan artifacts (never re-running the
scan) and drops its own refined findings into `.go-guardian/inbox/` as
markdown documents. The Stop hook flushes the inbox into the SQLite
knowledge base at session end.
