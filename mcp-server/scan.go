package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/tools"
)

// scanDimensions captures which scan dimensions the user requested on the CLI.
// Order of fields reflects --all execution order.
type scanDimensions struct {
	owasp     bool
	deps      bool
	staleness bool
	patterns  bool
}

// any returns true when at least one dimension is enabled.
func (d scanDimensions) any() bool {
	return d.owasp || d.deps || d.staleness || d.patterns
}

// enabledList returns dimension names in a stable order for logging.
func (d scanDimensions) enabledList() []string {
	var out []string
	if d.owasp {
		out = append(out, "owasp")
	}
	if d.deps {
		out = append(out, "deps")
	}
	if d.staleness {
		out = append(out, "staleness")
	}
	if d.patterns {
		out = append(out, "patterns")
	}
	return out
}

// scanOptions is the parsed CLI flag state for a `scan` invocation.
type scanOptions struct {
	dims      scanDimensions
	dbPath    string
	sourceDir string
}

// parseScanArgs parses scan-subcommand flags into scanOptions. It mirrors
// the dispatchHealthcheck convention: use a ContinueOnError flag.FlagSet
// so a parse failure returns exit code 2 without panicking.
func parseScanArgs(args []string, stderr io.Writer) (scanOptions, int) {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	fs.SetOutput(stderr)

	owasp := fs.Bool("owasp", false, "run the OWASP Go advisory scan and write .go-guardian/owasp-findings.md")
	deps := fs.Bool("deps", false, "run the dependency CVE scan and write .go-guardian/dep-vulns.md")
	staleness := fs.Bool("staleness", false, "run the scan staleness check and write .go-guardian/staleness.md")
	patterns := fs.Bool("patterns", false, "write pattern-stats.md, health-trends.md, and session-findings.md snapshots")
	all := fs.Bool("all", false, "run every scan dimension in sequence")

	dbPath := fs.String("db", ".go-guardian/guardian.db", "path to the SQLite learning database")
	sourceDir := fs.String("source-dir", "", "project root whose *.go files (excluding vendor/) are scanned (defaults to the current working directory)")

	if err := fs.Parse(args); err != nil {
		return scanOptions{}, 2
	}

	opts := scanOptions{
		dbPath:    *dbPath,
		sourceDir: *sourceDir,
	}
	if *all {
		opts.dims = scanDimensions{owasp: true, deps: true, staleness: true, patterns: true}
	} else {
		opts.dims = scanDimensions{
			owasp:     *owasp,
			deps:      *deps,
			staleness: *staleness,
			patterns:  *patterns,
		}
	}

	if !opts.dims.any() {
		fmt.Fprintln(stderr, "go-guardian scan: at least one of --owasp / --deps / --staleness / --patterns / --all is required")
		return scanOptions{}, 2
	}

	if strings.TrimSpace(opts.sourceDir) == "" {
		if cwd, err := os.Getwd(); err == nil {
			opts.sourceDir = cwd
		}
	}
	// Normalize to an absolute path so downstream file I/O is deterministic.
	if abs, err := filepath.Abs(opts.sourceDir); err == nil {
		opts.sourceDir = abs
	}

	return opts, 0
}

// dispatchScan is the subcommandHandler that implements `go-guardian scan`.
// This skeleton wires flag parsing + dimension detection only; Task 5 fills
// in the cache gate and Run* invocations.
func dispatchScan(args []string, stdout, stderr io.Writer) int {
	opts, exit := parseScanArgs(args, stderr)
	if exit != 0 {
		return exit
	}
	fmt.Fprintf(stdout,
		"go-guardian scan: dimensions=%s db=%s source-dir=%s (skeleton — not wired)\n",
		strings.Join(opts.dims.enabledList(), ","),
		opts.dbPath, opts.sourceDir)
	return 1
}

// writeScanOutput writes a findings file with YAML frontmatter followed by
// the body text. It writes atomically (tmp + rename) so a crash mid-write
// never leaves a half-formed file. Parent directory must already exist.
//
// Frontmatter fields:
//
//	scan_type       — e.g. "owasp", "deps", "staleness", "pattern-stats"
//	source_checksum — sha256 over *.go files excluding vendor/
//	generated_at    — RFC3339 UTC timestamp
//	count           — finding count (0 is a valid "no findings" result)
func writeScanOutput(path, scanType, checksum string, generatedAt time.Time, count int, body string) error {
	var sb strings.Builder
	sb.WriteString("---\n")
	fmt.Fprintf(&sb, "scan_type: %s\n", scanType)
	fmt.Fprintf(&sb, "source_checksum: %s\n", checksum)
	fmt.Fprintf(&sb, "generated_at: %s\n", generatedAt.UTC().Format(time.RFC3339))
	fmt.Fprintf(&sb, "count: %d\n", count)
	sb.WriteString("---\n\n")
	sb.WriteString(body)
	if !strings.HasSuffix(body, "\n") {
		sb.WriteString("\n")
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(sb.String()), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

// scanChecksumFile is the conventional path (relative to the guardian dir)
// where the warm-start cache stores the last successful source checksum.
const scanChecksumFile = ".scan-checksum"

// readScanChecksum returns the previously-recorded source checksum, or ""
// if the file does not exist. A read error other than "not exist" is
// surfaced so the caller can decide whether to proceed conservatively.
func readScanChecksum(guardianDir string) (string, error) {
	data, err := os.ReadFile(filepath.Join(guardianDir, scanChecksumFile))
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// writeScanChecksum persists the given checksum atomically. Parent directory
// must already exist.
func writeScanChecksum(guardianDir, checksum string) error {
	path := filepath.Join(guardianDir, scanChecksumFile)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(checksum+"\n"), 0o600); err != nil {
		return fmt.Errorf("write checksum tmp: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename checksum: %w", err)
	}
	return nil
}

// expectedOutputFiles returns the .go-guardian file basenames that should
// exist after a successful scan for the given dimensions. Order is stable.
func expectedOutputFiles(dims scanDimensions) []string {
	var out []string
	if dims.owasp {
		out = append(out, "owasp-findings.md")
	}
	if dims.deps {
		out = append(out, "dep-vulns.md")
	}
	if dims.staleness {
		out = append(out, "staleness.md")
	}
	if dims.patterns {
		out = append(out, "pattern-stats.md", "health-trends.md", "session-findings.md")
	}
	return out
}

// allFilesExist returns true when every file in names exists under dir.
// Missing files (ENOENT) return false without error. Other stat errors also
// return false — a stat failure is treated as "cannot confirm" and therefore
// triggers a re-scan rather than a silent cache hit.
func allFilesExist(dir string, names []string) bool {
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			return false
		}
	}
	return true
}

// runOwaspDimension runs tools.RunCheckOWASP against sourceDir and writes
// .go-guardian/owasp-findings.md. The OWASP handler already persists its
// findings + scan history + snapshot as a side-effect — this wrapper adds
// only the markdown file output.
func runOwaspDimension(store *db.Store, sourceDir, guardianDir, checksum string, now time.Time) error {
	text, err := tools.RunCheckOWASP(store, sourceDir, sourceDir)
	if err != nil {
		return fmt.Errorf("owasp scan: %w", err)
	}
	path := filepath.Join(guardianDir, "owasp-findings.md")
	return writeScanOutput(path, "owasp", checksum, now, countLines(text), text)
}

// runDepsDimension reads sourceDir/go.mod (if present), runs
// tools.RunCheckDeps over the module list, and writes
// .go-guardian/dep-vulns.md. A missing go.mod is treated as "no modules" and
// produces a valid empty report rather than an error — the scan is still
// considered successful.
func runDepsDimension(store *db.Store, sourceDir, guardianDir, checksum string, now time.Time) error {
	var modules []string
	goMod := filepath.Join(sourceDir, "go.mod")
	if _, err := os.Stat(goMod); err == nil {
		mods, pErr := tools.ParseGoMod(goMod)
		if pErr != nil {
			return fmt.Errorf("parse go.mod: %w", pErr)
		}
		modules = mods
	}
	text, err := tools.RunCheckDeps(store, modules)
	if err != nil {
		return fmt.Errorf("deps scan: %w", err)
	}
	path := filepath.Join(guardianDir, "dep-vulns.md")
	return writeScanOutput(path, "deps", checksum, now, len(modules), text)
}

// runStalenessDimension runs tools.RunCheckStaleness against sourceDir and
// writes .go-guardian/staleness.md. Staleness has no concept of "count" — it
// reports per-dimension currency — so count is 0.
func runStalenessDimension(store *db.Store, sourceDir, guardianDir, checksum string, now time.Time) error {
	text, err := tools.RunCheckStaleness(store, sourceDir)
	if err != nil {
		return fmt.Errorf("staleness scan: %w", err)
	}
	path := filepath.Join(guardianDir, "staleness.md")
	return writeScanOutput(path, "staleness", checksum, now, 0, text)
}

// runPatternsDimension writes three snapshots of the learning DB:
//   - pattern-stats.md   (tools.RunGetPatternStats)
//   - health-trends.md   (tools.RunGetHealthTrends)
//   - session-findings.md (tools.RunGetSessionFindings with empty session)
//
// The session-findings snapshot is intentionally session-less at this point —
// CLI scans do not own a session; the "No active session..." message is a
// valid snapshot of "nothing reported yet".
func runPatternsDimension(store *db.Store, guardianDir, checksum string, now time.Time) error {
	stats, err := tools.RunGetPatternStats(store, "")
	if err != nil {
		return fmt.Errorf("pattern-stats: %w", err)
	}
	if err := writeScanOutput(filepath.Join(guardianDir, "pattern-stats.md"),
		"pattern-stats", checksum, now, countLines(stats), stats); err != nil {
		return err
	}

	trends, err := tools.RunGetHealthTrends(store, "", "")
	if err != nil {
		return fmt.Errorf("health-trends: %w", err)
	}
	if err := writeScanOutput(filepath.Join(guardianDir, "health-trends.md"),
		"health-trends", checksum, now, countLines(trends), trends); err != nil {
		return err
	}

	// Empty sessionID returns "No active session — ..." which is exactly the
	// snapshot we want to record when scans run outside of a /go session.
	findings, err := tools.RunGetSessionFindings(store, "", "", "", "")
	if err != nil {
		return fmt.Errorf("session-findings: %w", err)
	}
	return writeScanOutput(filepath.Join(guardianDir, "session-findings.md"),
		"session-findings", checksum, now, 0, findings)
}

// countLines returns the number of non-empty lines in s. Used as a coarse
// "count" for frontmatter when the Run* helper doesn't expose a structured
// finding total.
func countLines(s string) int {
	n := 0
	for _, ln := range strings.Split(s, "\n") {
		if strings.TrimSpace(ln) != "" {
			n++
		}
	}
	return n
}
