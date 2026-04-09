package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
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
//   scan_type       — e.g. "owasp", "deps", "staleness", "pattern-stats"
//   source_checksum — sha256 over *.go files excluding vendor/
//   generated_at    — RFC3339 UTC timestamp
//   count           — finding count (0 is a valid "no findings" result)
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
