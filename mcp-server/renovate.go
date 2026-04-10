package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/tools"
)

// dispatchRenovate implements `go-guardian renovate <verb> [flags] [args...]`.
// It peels the first positional as the verb and routes to a per-verb
// dispatcher. Unknown verbs exit 2 with a verb listing. Each per-verb
// dispatcher owns its own flag.FlagSet and positional handling.
func dispatchRenovate(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "go-guardian renovate: missing verb")
		printRenovateVerbListing(stderr)
		return 2
	}

	verb := args[0]
	rest := args[1:]

	switch verb {
	case "validate":
		return dispatchRenovateValidate(rest, stdout, stderr)
	case "analyze":
		return dispatchRenovateAnalyze(rest, stdout, stderr)
	case "suggest":
		return dispatchRenovateSuggest(rest, stdout, stderr)
	case "query":
		return dispatchRenovateQuery(rest, stdout, stderr)
	case "stats":
		return dispatchRenovateStats(rest, stdout, stderr)
	case "-h", "--help", "help":
		printRenovateVerbListing(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "go-guardian renovate: unknown verb %q\n", verb)
		printRenovateVerbListing(stderr)
		return 2
	}
}

// printRenovateVerbListing writes the supported verbs to w.
func printRenovateVerbListing(w io.Writer) {
	fmt.Fprintln(w, "Verbs:")
	fmt.Fprintln(w, "  validate <config-path>          Validate a renovate configuration file")
	fmt.Fprintln(w, "  analyze  <config-path>          Analyze a config and write .go-guardian/renovate-analysis.md")
	fmt.Fprintln(w, "  suggest  <problem>              Suggest a rule and write .go-guardian/renovate-suggestions.md")
	fmt.Fprintln(w, "  query    [--category] [--keyword]  Query the renovate knowledge base (stdout)")
	fmt.Fprintln(w, "  stats    [--config <path>]      Print the renovate dashboard (stdout)")
}

// dispatchRenovateValidate implements `go-guardian renovate validate
// <config-path>`. It prints the validator's report to stdout and exits
// non-zero if the report contains any "✗ ERR:" markers. The validator
// itself never returns an error — all failure modes are encoded in the body.
func dispatchRenovateValidate(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("renovate validate", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := addRenovateCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	positional := fs.Args()
	if len(positional) != 1 {
		fmt.Fprintln(stderr, "go-guardian renovate validate: exactly one positional <config-path> is required")
		return 2
	}
	configPath := positional[0]

	store, exit := openRenovateStore(*dbPath, stderr, "validate")
	if exit != 0 {
		return exit
	}
	defer store.Close()

	body, err := tools.RunValidateRenovateConfig(store, configPath)
	if err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate validate: %v\n", err)
		return 1
	}

	fmt.Fprint(stdout, body)
	if !strings.HasSuffix(body, "\n") {
		fmt.Fprintln(stdout)
	}

	if strings.Contains(body, "✗ ERR:") {
		return 1
	}
	return 0
}

// dispatchRenovateAnalyze implements `go-guardian renovate analyze
// <config-path>`. It writes the analysis report to
// .go-guardian/renovate-analysis.md and prints a one-line confirmation to
// stdout. The underlying Run* helper already persists the config score to
// the store for trend tracking.
func dispatchRenovateAnalyze(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("renovate analyze", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := addRenovateCommonFlags(fs)
	if err := fs.Parse(args); err != nil {
		return 2
	}

	positional := fs.Args()
	if len(positional) != 1 {
		fmt.Fprintln(stderr, "go-guardian renovate analyze: exactly one positional <config-path> is required")
		return 2
	}
	configPath := positional[0]

	outDir := renovateOutputDir(*dbPath)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate analyze: mkdir %s: %v\n", outDir, err)
		return 1
	}

	store, exit := openRenovateStore(*dbPath, stderr, "analyze")
	if exit != 0 {
		return exit
	}
	defer store.Close()

	body, err := tools.RunAnalyzeRenovateConfig(store, configPath)
	if err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate analyze: %v\n", err)
		return 1
	}

	artifactPath := filepath.Join(outDir, "renovate-analysis.md")
	if err := atomicWriteMarkdown(artifactPath, body); err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate analyze: %v\n", err)
		return 1
	}

	// Extract the Score line for the stdout confirmation. The body format is
	// "=== Renovate Config Analysis: <path> ===\nScore: N/100\n...".
	scoreLine := ""
	for _, ln := range strings.Split(body, "\n") {
		if strings.HasPrefix(ln, "Score:") {
			scoreLine = " (" + strings.TrimSpace(ln) + ")"
			break
		}
	}
	fmt.Fprintf(stdout, "go-guardian renovate analyze: wrote %s%s\n", artifactPath, scoreLine)
	return 0
}

// dispatchRenovateSuggest implements `go-guardian renovate suggest <problem>
// [--config <path>]`. It writes the suggestion to
// .go-guardian/renovate-suggestions.md and prints a confirmation to stdout.
// The optional --config flag lets the Run* helper include a concrete diff
// against the user's current renovate.json.
func dispatchRenovateSuggest(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("renovate suggest", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := addRenovateCommonFlags(fs)
	configPath := fs.String("config", "", "optional path to the user's current renovate.json for diff context")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	positional := fs.Args()
	if len(positional) != 1 {
		fmt.Fprintln(stderr, "go-guardian renovate suggest: exactly one positional <problem> is required")
		return 2
	}
	problem := positional[0]

	outDir := renovateOutputDir(*dbPath)
	if err := os.MkdirAll(outDir, 0o700); err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate suggest: mkdir %s: %v\n", outDir, err)
		return 1
	}

	store, exit := openRenovateStore(*dbPath, stderr, "suggest")
	if exit != 0 {
		return exit
	}
	defer store.Close()

	body, err := tools.RunSuggestRenovateRule(store, problem, *configPath)
	if err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate suggest: %v\n", err)
		return 1
	}

	artifactPath := filepath.Join(outDir, "renovate-suggestions.md")
	if err := atomicWriteMarkdown(artifactPath, body); err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate suggest: %v\n", err)
		return 1
	}

	fmt.Fprintf(stdout, "go-guardian renovate suggest: wrote %s\n", artifactPath)
	return 0
}

// dispatchRenovateQuery is the skeleton for `renovate query`.
// Wired in Task 5.
func dispatchRenovateQuery(args []string, stdout, stderr io.Writer) int {
	_ = args
	_ = stdout
	fmt.Fprintln(stderr, "go-guardian renovate query: not wired (task 5)")
	return 1
}

// dispatchRenovateStats is the skeleton for `renovate stats`.
// Wired in Task 6.
func dispatchRenovateStats(args []string, stdout, stderr io.Writer) int {
	_ = args
	_ = stdout
	fmt.Fprintln(stderr, "go-guardian renovate stats: not wired (task 6)")
	return 1
}

// renovateCommonOptions captures the flags shared by every verb.
type renovateCommonOptions struct {
	dbPath string
}

// addRenovateCommonFlags attaches the shared --db flag to fs and returns a
// pointer the caller can dereference after fs.Parse. Per-verb dispatchers
// call this to standardize --db handling across all verbs.
func addRenovateCommonFlags(fs *flag.FlagSet) *string {
	return fs.String("db", ".go-guardian/guardian.db", "path to the SQLite learning database")
}

// renovateOutputDir returns the directory where artifact markdown files are
// written. It is derived from the --db flag's parent directory so that every
// go-guardian invocation writes its artifacts under the same .go-guardian/.
func renovateOutputDir(dbPath string) string {
	return filepath.Dir(dbPath)
}

// openRenovateStore opens the SQLite store at dbPath with consistent error
// formatting for the renovate verb family. Caller owns Close().
func openRenovateStore(dbPath string, stderr io.Writer, verb string) (*db.Store, int) {
	store, err := db.NewStore(dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "go-guardian renovate %s: open db %s: %v\n", verb, dbPath, err)
		return nil, 1
	}
	return store, 0
}

// atomicWriteMarkdown writes body to path atomically (tmp + rename) with 0o600
// permissions. Used by analyze/suggest to emit artifact files. Parent directory
// must already exist.
func atomicWriteMarkdown(path string, body string) error {
	if !strings.HasSuffix(body, "\n") {
		body += "\n"
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("rename %s -> %s: %w", tmp, path, err)
	}
	return nil
}

