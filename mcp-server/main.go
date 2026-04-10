package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/kengou/go-guardian/mcp-server/admin"
	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/tools"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

const version = "0.3.1"

func main() {
	// ── Subcommand dispatch (new CLI mode) ────────────────────────────────
	// If invoked with a positional subcommand (e.g. `go-guardian healthcheck`),
	// enter CLI mode and exit. No-arg and flag-only invocations fall through
	// to the legacy MCP stdio server path below.
	if isSubcommandInvocation(os.Args) {
		os.Exit(Dispatch(os.Args[1:], os.Stdout, os.Stderr))
	}

	dbPath := flag.String("db", ".go-guardian/guardian.db", "path to the SQLite database file")
	showVersion := flag.Bool("version", false, "print version and exit")
	prefetch := flag.Bool("prefetch", false, "fetch CVE data for modules in go.mod and exit")
	nvdKey := flag.String("nvd-key", os.Getenv("NVD_API_KEY"), "NVD API key for CVSS enrichment (or set NVD_API_KEY env var)")
	updateOWASP := flag.Bool("update-owasp", false, "fetch new OWASP-tagged Go advisories from GHSA and update rule patterns")
	githubToken := flag.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token for GHSA API (optional, increases rate limits)")
	goModPath := flag.String("go-mod", "go.mod", "path to go.mod for --prefetch mode")
	projectDir := flag.String("project", "", "project root for scan path validation (defaults to directory of --db)")

	// Runtime toggles for MCP server mode.
	noAdmin := flag.Bool("no-admin", false, "disable admin UI HTTP server even if GO_GUARDIAN_ADMIN_PORT is set")
	noPrefetch := flag.Bool("no-prefetch", false, "disable background CVE prefetch on startup")
	auditLog := flag.Bool("audit-log", false, "enable MCP request audit logging to mcp_requests table (always on when admin UI is active)")
	debug := flag.Bool("debug", false, "enable debug logging: log every MCP request/response to a file next to the DB")
	logFile := flag.String("log-file", "", "path to debug log file (defaults to <db-dir>/guardian.log)")

	// CLI one-shot modes: staleness, learn, query-knowledge, healthcheck.
	healthcheckFlag := flag.Bool("healthcheck", false, "run diagnostic checks on DB schema, seeds, and tool registration, then exit")
	checkStalenessFlag := flag.Bool("check-staleness", false, "check scan staleness and print warnings, then exit")
	learnFlag := flag.Bool("learn", false, "learn lint patterns from lint output and diff, then exit")
	lintOutputPath := flag.String("lint-output", "", "path to file containing lint output (used with --learn)")
	diffPath := flag.String("diff", "", "path to file containing unified diff (used with --learn)")
	queryKnowledgeFlag := flag.Bool("query-knowledge", false, "query knowledge base for patterns relevant to a file, then exit")
	filePath := flag.String("file-path", "", "file path for --query-knowledge mode")
	sourceChecksumFlag := flag.Bool("source-checksum", false, "compute source checksum for a directory and print it, then exit")
	checksumDir := flag.String("checksum-dir", "", "directory to compute source checksum for (used with --source-checksum)")

	flag.Parse()

	if *projectDir == "" {
		// Default to CWD so check_owasp can scan the user's actual project.
		// The MCP server inherits CWD from Claude Code (the user's project dir).
		// Falling back to the DB directory would restrict scans to the plugin
		// data directory, making check_owasp unusable.
		if cwd, err := os.Getwd(); err == nil {
			*projectDir = cwd
		} else {
			*projectDir = filepath.Dir(*dbPath)
		}
	}

	if *showVersion {
		fmt.Printf("go-guardian-mcp v%s\n", version)
		os.Exit(0)
	}

	// ── --source-checksum: compute and print source checksum ───────────────
	if *sourceChecksumFlag {
		dir := *checksumDir
		if dir == "" {
			fmt.Fprintln(os.Stderr, "error: --checksum-dir is required with --source-checksum")
			os.Exit(1)
		}
		checksum, err := computeSourceChecksum(dir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Println(checksum)
		os.Exit(0)
	}

	// ── --healthcheck: diagnostic checks ───────────────────────────────────
	if *healthcheckFlag {
		os.Exit(runHealthcheck(*dbPath))
	}

	// Create parent directories for the db path if they don't exist.
	// SECURITY: use 0o700 so the guardian directory is not world-accessible (fixes FINDING-06).
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		log.Fatalf("failed to create db directory: %v", err)
	}

	// Initialize the database store.
	store, err := db.NewStore(*dbPath)
	if err != nil {
		log.Fatalf("failed to initialize database: %v", err)
	}
	defer store.Close()

	if *prefetch {
		result, err := tools.FetchVulns(context.Background(), store, tools.FetchOptions{
			GoModPath: *goModPath,
			NVDAPIKey: *nvdKey,
			Out:       os.Stdout,
		})
		if err != nil {
			log.Fatalf("prefetch failed: %v", err)
		}
		fmt.Printf("Prefetch complete: %d modules checked, %d CVEs found, %d enriched via NVD\n",
			result.ModulesChecked, result.CVEsFound, result.CVEsEnriched)
		os.Exit(0)
	}

	if *updateOWASP {
		result, err := tools.UpdateOWASPRules(context.Background(), store, tools.OWASPUpdateOptions{
			GitHubToken: *githubToken,
			Out:         os.Stdout,
		})
		if err != nil {
			log.Fatalf("owasp update failed: %v", err)
		}
		fmt.Printf("OWASP update complete: %d advisories fetched, %d patterns stored\n",
			result.AdvisoriesFetched, result.PatternsStored)
		for cat, count := range result.CategoriesUpdated {
			fmt.Printf("  %s: %d new patterns\n", cat, count)
		}
		os.Exit(0)
	}

	// ── --check-staleness: one-shot staleness check ─────────────────────────
	if *checkStalenessFlag {
		runCheckStaleness(store, *projectDir)
		os.Exit(0)
	}

	// ── --learn: one-shot learn from lint output + diff ─────────────────────
	if *learnFlag {
		runLearn(store, *projectDir, *lintOutputPath, *diffPath)
		os.Exit(0)
	}

	// ── --query-knowledge: one-shot knowledge query (code context via stdin) ─
	if *queryKnowledgeFlag {
		runQueryKnowledge(store, *filePath)
		os.Exit(0)
	}

	// Read session ID from environment or .go-guardian/session-id file.
	sessionID := os.Getenv("GO_GUARDIAN_SESSION_ID")
	if sessionID == "" {
		sidPath := filepath.Join(filepath.Dir(*dbPath), "session-id")
		if data, err := os.ReadFile(sidPath); err == nil {
			sessionID = strings.TrimSpace(string(data))
		}
	}
	if sessionID != "" {
		if err := store.CleanupOldSessions(sessionID); err != nil {
			log.Printf("warning: session cleanup failed: %v", err)
		}
		log.Printf("session: %s", sessionID)
	}

	log.Printf("go-guardian MCP server v%s started, db: %s\n", version, *dbPath)

	// ── Debug log file ───────────────────────────────────────────────────
	// When --debug is set, redirect log output to a file so MCP request
	// arrivals and tool calls are visible for post-mortem debugging.
	if *debug {
		logPath := *logFile
		if logPath == "" {
			logPath = filepath.Join(filepath.Dir(*dbPath), "guardian.log")
		}
		f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
		if err != nil {
			log.Fatalf("open debug log: %v", err)
		}
		defer f.Close()
		// Write to both stderr (MCP stdio doesn't use stderr) and the file.
		log.SetOutput(io.MultiWriter(os.Stderr, f))
		log.Printf("debug logging enabled: %s", logPath)
	}

	// Shared prefetch status tracker for admin UI progress reporting.
	prefetchStatus := &tools.PrefetchStatus{}
	prefetchStatus.SetPhase("idle", "")

	// ── Auto go.mod discovery ─────────────────────────────────────────────
	// If the default "go.mod" doesn't exist, walk up from the project dir.
	goMod := *goModPath
	if _, err := os.Stat(goMod); err != nil {
		if found := findGoMod(*projectDir); found != "" {
			goMod = found
			log.Printf("auto-discovered go.mod: %s", goMod)
		}
	}

	// ── Admin UI: start HTTP server if GO_GUARDIAN_ADMIN_PORT is set ──────
	adminPort := os.Getenv("GO_GUARDIAN_ADMIN_PORT")
	if *noAdmin {
		adminPort = ""
	}
	if adminPort != "" {
		// Prune old MCP request log entries (>7 days).
		if pruned, err := store.PruneMCPRequests(7 * 24 * time.Hour); err != nil {
			log.Printf("warning: prune mcp_requests failed: %v", err)
		} else if pruned > 0 {
			log.Printf("admin: pruned %d old request log entries", pruned)
		}

		// Serve the embedded frontend from admin/ui/dist.
		staticFS, err := fs.Sub(admin.UIAssets, "ui/dist")
		if err != nil {
			log.Printf("warning: admin UI assets not available: %v", err)
		} else {
			adminSrv := admin.New(store, staticFS, sessionID,
				admin.WithPrefetchStatus(prefetchStatus))
			addr := "127.0.0.1:" + adminPort
			go func() {
				log.Printf("admin UI: http://%s", addr)
				if err := adminSrv.ListenAndServe(addr); err != nil && err != http.ErrServerClosed {
					log.Printf("admin server error: %v", err)
				}
			}()
		}
	}

	// Background CVE prefetch: populate vuln_cache so the admin UI and
	// check_deps have data. Runs once on startup and then daily.
	runPrefetch := func() {
		if _, err := os.Stat(goMod); err != nil {
			prefetchStatus.SetDone(0, 0)
			return
		}
		log.Printf("background prefetch: starting for %s", goMod)
		prefetchStatus.SetPhase("go-vuln", "vuln.go.dev")
		result, err := tools.FetchVulns(context.Background(), store, tools.FetchOptions{
			GoModPath: goMod,
			NVDAPIKey: *nvdKey,
			Quiet:     true,
			Status:    prefetchStatus,
		})
		if err != nil {
			log.Printf("background prefetch failed: %v", err)
			prefetchStatus.SetError(err.Error())
			return
		}
		log.Printf("background prefetch: %d modules checked, %d CVEs found, %d enriched via NVD",
			result.ModulesChecked, result.CVEsFound, result.CVEsEnriched)
	}

	// Run initial prefetch in background (unless --no-prefetch).
	if !*noPrefetch {
		go runPrefetch()

		// Daily refresh ticker.
		go func() {
			ticker := time.NewTicker(24 * time.Hour)
			defer ticker.Stop()
			for range ticker.C {
				log.Printf("daily CVE refresh: starting")
				runPrefetch()
			}
		}()
	}

	// Create the MCP server and register all tools.
	s := server.NewMCPServer("go-guardian", version)

	// Audit/debug logging: wrap tool registration to log invocations.
	enableAudit := *auditLog || adminPort != ""
	var reg tools.ToolRegistrar = s
	if enableAudit || *debug {
		reg = &loggingRegistrar{inner: s, store: store, dbLog: enableAudit, debugLog: *debug}
		if enableAudit && adminPort == "" {
			if pruned, err := store.PruneMCPRequests(7 * 24 * time.Hour); err != nil {
				log.Printf("warning: prune mcp_requests failed: %v", err)
			} else if pruned > 0 {
				log.Printf("audit: pruned %d old request log entries", pruned)
			}
		}
		if enableAudit {
			log.Printf("audit logging: enabled (DB)")
		}
		if *debug {
			log.Printf("debug logging: tool calls will be logged")
		}
	}

	tools.RegisterLearnFromLint(reg, store)
	tools.RegisterQueryKnowledge(reg, store, sessionID)
	tools.RegisterCheckOWASP(reg, store, *projectDir)
	tools.RegisterCheckStaleness(reg, store)
	tools.RegisterCheckDeps(reg, store)
	tools.RegisterGetPatternStats(reg, store)
	tools.RegisterSuggestFix(reg, store)
	tools.RegisterLearnFromReview(reg, store)
	tools.RegisterGetHealthTrends(reg, store)
	tools.RegisterReportFinding(reg, store, sessionID)
	tools.RegisterGetSessionFindings(reg, store, sessionID)
	tools.RegisterValidateRenovateConfig(reg, store)
	tools.RegisterAnalyzeRenovateConfig(reg, store)
	tools.RegisterSuggestRenovateRule(reg, store)
	tools.RegisterLearnRenovatePreference(reg, store)
	tools.RegisterRenovateQueryKnowledge(reg, store)
	tools.RegisterGetRenovateStats(reg, store)

	log.Printf("registered 17 tools: learn_from_lint, learn_from_review, query_knowledge, check_owasp, check_staleness, check_deps, get_pattern_stats, suggest_fix, get_health_trends, report_finding, get_session_findings, validate_renovate_config, analyze_renovate_config, suggest_renovate_rule, learn_renovate_preference, query_renovate_knowledge, get_renovate_stats\n")

	// Start serving via stdio. ServeStdio handles SIGINT/SIGTERM gracefully.
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}

// ── Subcommand dispatch layer ──────────────────────────────────────────────
//
// Dispatch enters CLI mode when go-guardian is invoked with a positional
// subcommand (e.g. `go-guardian healthcheck`). It parses the subcommand from
// args[0], routes to the corresponding handler, and returns an exit code.
//
// Supported subcommands (wave 1 reality):
//
//	version       — prints the binary version, exits 0
//	help / --help / -h — prints subcommand listing, exits 0
//	healthcheck   — runs diagnostic checks (real handler, calls runHealthcheck)
//	scan          — placeholder (wave 2: scan-subcommands feature)
//	ingest        — placeholder (wave 2: inbox-ingest feature)
//	renovate      — placeholder (wave 2: renovate-cli-pack feature)
//	admin         — placeholder (wave 2: admin-cli feature)
//
// Placeholder handlers exit 1 with a "not yet implemented in this build"
// message so scripts calling them fail loud rather than silently succeeding.
//
// Dispatch writes exclusively to the provided stdout/stderr. Callers that
// test it pass bytes.Buffer; main() passes os.Stdout/os.Stderr.
func Dispatch(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		return dispatchHelp(nil, stdout, stderr)
	}

	head := args[0]
	rest := args[1:]

	// Legacy long-form aliases.
	switch head {
	case "--help", "-h":
		return dispatchHelp(rest, stdout, stderr)
	case "--version":
		return dispatchVersion(rest, stdout, stderr)
	}

	handler, ok := subcommandRegistry[head]
	if !ok {
		fmt.Fprintf(stderr, "go-guardian: unknown subcommand %q\n\n", head)
		printSubcommandListing(stderr)
		return 1
	}
	return handler(rest, stdout, stderr)
}

// subcommandHandler is the signature every dispatch handler must satisfy.
type subcommandHandler func(args []string, stdout, stderr io.Writer) int

// subcommandRegistry is the authoritative list of CLI subcommands. Placeholder
// handlers for wave-2 features are present so that --help listing covers the
// full future surface from day one.
var subcommandRegistry = map[string]subcommandHandler{
	"version":     dispatchVersion,
	"help":        dispatchHelp,
	"healthcheck": dispatchHealthcheck,
	"scan":        dispatchScan,
	"ingest":      dispatchIngest,
	"renovate":    dispatchPlaceholderFor("renovate", "renovate-cli-pack (wave 2)"),
	"admin":       dispatchPlaceholderFor("admin", "admin-cli (wave 2)"),
}

// subcommandDescriptions backs the --help listing. Keys must mirror
// subcommandRegistry so a drift check is possible at test time.
var subcommandDescriptions = map[string]string{
	"scan":        "Run OWASP / deps / staleness / pattern scans and write findings files under .go-guardian/",
	"ingest":      "Ingest pending agent-inbox documents into the learning database",
	"renovate":    "Validate, analyze, suggest, query, or report stats on renovate configurations",
	"admin":       "Start the on-demand admin dashboard in the foreground",
	"healthcheck": "Run diagnostic checks on DB schema, seeds, and tool registration",
	"version":     "Print the go-guardian binary version",
	"help":        "Print this help listing",
}

// dispatchVersion implements `go-guardian version` and `go-guardian --version`.
func dispatchVersion(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, "go-guardian-mcp v%s\n", version)
	return 0
}

// dispatchHelp prints the subcommand listing.
func dispatchHelp(_ []string, stdout, _ io.Writer) int {
	fmt.Fprintf(stdout, "go-guardian-mcp v%s\n\n", version)
	fmt.Fprintln(stdout, "Usage: go-guardian [subcommand] [flags]")
	fmt.Fprintln(stdout, "       go-guardian          (no subcommand starts the MCP stdio server)")
	fmt.Fprintln(stdout)
	printSubcommandListing(stdout)
	return 0
}

// printSubcommandListing writes the subcommand table to w. Used by both help
// and by unknown-subcommand error output.
func printSubcommandListing(w io.Writer) {
	// Deterministic order: hand-rolled to group real commands then placeholders.
	order := []string{"scan", "ingest", "renovate", "admin", "healthcheck", "version", "help"}
	fmt.Fprintln(w, "Subcommands:")
	for _, name := range order {
		desc, ok := subcommandDescriptions[name]
		if !ok {
			continue
		}
		fmt.Fprintf(w, "  %-12s %s\n", name, desc)
	}
}

// dispatchHealthcheck implements `go-guardian healthcheck`. It parses a
// standalone --db flag so subcommand mode is self-contained (does not lean
// on the legacy top-level flag parser).
func dispatchHealthcheck(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("healthcheck", flag.ContinueOnError)
	fs.SetOutput(stderr)
	dbPath := fs.String("db", ".go-guardian/guardian.db", "path to the SQLite database file")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	return runHealthcheckOn(*dbPath, stdout, stderr)
}

// runHealthcheckOn is a thin wrapper around runHealthcheck that redirects
// stdout/stderr for the duration of the call. It exists so Dispatch handlers
// can inject buffers for testing while runHealthcheck itself remains a
// straightforward fmt.Print* based diagnostic.
func runHealthcheckOn(dbPath string, stdout, stderr io.Writer) int {
	origOut := os.Stdout
	origErr := os.Stderr

	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr

	// Copy pipe output to the dispatch writers in background goroutines.
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(stdout, rOut); done <- struct{}{} }()
	go func() { _, _ = io.Copy(stderr, rErr); done <- struct{}{} }()

	defer func() {
		_ = wOut.Close()
		_ = wErr.Close()
		<-done
		<-done
		os.Stdout = origOut
		os.Stderr = origErr
	}()

	return runHealthcheck(dbPath)
}

// dispatchPlaceholderFor returns a subcommandHandler for subcommands not yet
// implemented in this wave. It prints a clear "not yet implemented" message
// naming which wave-2 feature will add the real handler, then exits 1.
func dispatchPlaceholderFor(name, feature string) subcommandHandler {
	return func(_ []string, _, stderr io.Writer) int {
		fmt.Fprintf(stderr,
			"go-guardian %s: not yet implemented in this build — will be added in the %s feature\n",
			name, feature)
		return 1
	}
}

// isSubcommandInvocation returns true when the process was invoked with a
// positional subcommand OR a recognized long-form flag alias (--help, -h,
// --version) that the new dispatch layer owns.
func isSubcommandInvocation(osArgs []string) bool {
	if len(osArgs) < 2 {
		return false
	}
	first := osArgs[1]
	switch first {
	case "--help", "-h", "--version":
		return true
	}
	return first != "" && !strings.HasPrefix(first, "-")
}

// loggingRegistrar wraps MCP tool registration to log every tool invocation.
// When dbLog is true, writes to mcp_requests table. When debugLog is true,
// logs request arrival and duration to the log output (file/stderr).
type loggingRegistrar struct {
	inner    *server.MCPServer
	store    *db.Store
	dbLog    bool
	debugLog bool
}

func (l *loggingRegistrar) AddTool(tool mcp.Tool, handler server.ToolHandlerFunc) {
	wrapped := handler
	if l.dbLog {
		wrapped = admin.WrapToolHandler(l.store, tool.Name, wrapped)
	}
	if l.debugLog {
		inner := wrapped
		wrapped = func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			start := time.Now()
			log.Printf("[DEBUG] tool request: %s", tool.Name)
			result, err := inner(ctx, req)
			dur := time.Since(start)
			if err != nil {
				log.Printf("[DEBUG] tool response: %s err=%v (%s)", tool.Name, err, dur)
			} else {
				log.Printf("[DEBUG] tool response: %s ok (%s)", tool.Name, dur)
			}
			return result, err
		}
	}
	l.inner.AddTool(tool, wrapped)
}

// findGoMod walks up from startDir looking for go.mod. Returns the path if
// found, or empty string if not.
func findGoMod(startDir string) string {
	if startDir == "" {
		startDir = "."
	}
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return ""
	}
	candidate := filepath.Join(dir, "go.mod")
	if _, err := os.Stat(candidate); err == nil {
		return candidate
	}
	return ""
}

// ── --check-staleness implementation ────────────────────────────────────────

// staleThresholdsCLI mirrors tools.staleThresholds for the CLI mode.
// We replicate rather than export to avoid changing the tools package interface.
var staleThresholdsCLI = map[string]time.Duration{
	"vuln":        3 * 24 * time.Hour,
	"owasp":       7 * 24 * time.Hour,
	"owasp_rules": 30 * 24 * time.Hour,
	"full":        14 * 24 * time.Hour,
}

// runCheckStaleness checks scan staleness and prints a JSON report to stdout.
func runCheckStaleness(store *db.Store, projectPath string) {
	projectID := tools.ProjectID(projectPath)

	history, err := store.GetScanHistory(projectID)
	if err != nil {
		log.Fatalf("check-staleness: failed to read scan history: %v", err)
	}

	// Build a map of scan_type -> most-recent ScanHistory record.
	latest := make(map[string]db.ScanHistory)
	for _, h := range history {
		if _, seen := latest[h.ScanType]; !seen {
			latest[h.ScanType] = h
		}
	}

	// Collect scan types sorted for deterministic output.
	var tracked []string
	for scanType := range staleThresholdsCLI {
		tracked = append(tracked, scanType)
	}
	sort.Strings(tracked)

	type staleEntry struct {
		ScanType   string `json:"scan_type"`
		LastRunAgo string `json:"last_run_ago"`
		Threshold  string `json:"threshold"`
	}

	var staleScans []staleEntry
	for _, scanType := range tracked {
		threshold := staleThresholdsCLI[scanType]
		h, found := latest[scanType]
		if !found {
			staleScans = append(staleScans, staleEntry{
				ScanType:   scanType,
				LastRunAgo: "never",
				Threshold:  formatDuration(threshold),
			})
			continue
		}
		age := time.Since(h.LastRun)
		if age > threshold {
			staleScans = append(staleScans, staleEntry{
				ScanType:   scanType,
				LastRunAgo: formatDuration(age),
				Threshold:  formatDuration(threshold),
			})
		}
	}

	result := map[string]interface{}{
		"stale_scans": staleScans,
	}
	enc := json.NewEncoder(os.Stdout)
	if err := enc.Encode(result); err != nil {
		log.Fatalf("check-staleness: failed to encode result: %v", err)
	}
}

// formatDuration returns a human-readable duration string like "3 days" or "14 days".
func formatDuration(d time.Duration) string {
	days := int(d.Hours() / 24)
	if days == 1 {
		return "1 day"
	}
	return fmt.Sprintf("%d days", days)
}

// ── --learn implementation ──────────────────────────────────────────────────

// cliLintLineRe matches golangci-lint output lines. Mirrors the regex in tools/learn.go.
var cliLintLineRe = regexp.MustCompile(
	`^([^\s:][^:]*\.go):\d+:\d+:\s+(.+?)\s+\(([^)]+)\)\s*$`,
)

// cliLintFinding holds one parsed line from golangci-lint output.
type cliLintFinding struct {
	file    string
	rule    string
	message string
}

// cliDiffHunk holds extracted before/after code from a unified diff hunk.
type cliDiffHunk struct {
	file     string
	dontCode string
	doCode   string
}

// cliLintPattern is a resolved pattern ready to be stored.
type cliLintPattern struct {
	Rule     string
	FileGlob string
	DontCode string
	DoCode   string
}

// runLearn reads lint output and diff from files and learns patterns.
func runLearn(store *db.Store, projectPath, lintOutputPath, diffPath string) {
	if lintOutputPath == "" {
		log.Fatalf("learn: --lint-output is required")
	}

	lintData, err := os.ReadFile(lintOutputPath)
	if err != nil {
		log.Fatalf("learn: failed to read lint output file: %v", err)
	}
	lintOutput := string(lintData)

	var diff string
	if diffPath != "" {
		diffData, err := os.ReadFile(diffPath)
		if err != nil {
			log.Fatalf("learn: failed to read diff file: %v", err)
		}
		diff = string(diffData)
	}

	// Parse lint output and diff, then store patterns.
	findings := cliParseLintOutput(lintOutput)
	hunks := cliParseDiff(diff)
	patterns := cliMatchFindingsToHunks(findings, hunks)

	learned := 0
	for _, p := range patterns {
		if err := store.InsertLintPattern(p.Rule, p.FileGlob, p.DontCode, p.DoCode, "learned"); err != nil {
			log.Fatalf("learn: store error: %v", err)
		}
		learned++
	}

	// Record scan snapshot for trend tracking.
	projectID := tools.ProjectID(projectPath)
	_ = store.InsertScanSnapshot("lint", projectID, len(findings), "{}")

	fmt.Printf("Learned %d patterns from lint output.\n", learned)
}

// cliParseLintOutput parses raw golangci-lint output. Mirrors tools/learn.go parseLintOutput.
func cliParseLintOutput(output string) []cliLintFinding {
	var findings []cliLintFinding
	seen := make(map[string]bool)

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		m := cliLintLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		filePath := m[1]
		body := strings.TrimSpace(m[2])
		linter := strings.TrimSpace(m[3])

		rule := linter
		if idx := strings.Index(body, ":"); idx > 0 {
			candidate := strings.TrimSpace(body[:idx])
			if len(candidate) <= 60 && !strings.ContainsAny(candidate, " \t()") {
				rule = candidate
			}
		}

		base := filepath.Base(filePath)
		key := rule + "|" + base
		if seen[key] {
			continue
		}
		seen[key] = true

		findings = append(findings, cliLintFinding{
			file:    base,
			rule:    rule,
			message: body,
		})
	}
	return findings
}

// cliParseDiff parses a unified diff. Mirrors tools/learn.go parseDiff.
func cliParseDiff(diff string) []cliDiffHunk {
	const maxSnippet = 500

	var hunks []cliDiffHunk
	var current *cliDiffHunk
	var dontBuf, doBuf strings.Builder

	flushCurrent := func() {
		if current == nil {
			return
		}
		current.dontCode = cliTrimSnippet(dontBuf.String(), maxSnippet)
		current.doCode = cliTrimSnippet(doBuf.String(), maxSnippet)
		hunks = append(hunks, *current)
		current = nil
		dontBuf.Reset()
		doBuf.Reset()
	}

	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "diff -"):
			flushCurrent()

		case strings.HasPrefix(line, "--- "):
			flushCurrent()
			rest := strings.TrimPrefix(line, "--- ")
			rest = strings.TrimPrefix(rest, "a/")
			rest = strings.TrimPrefix(rest, "b/")
			if rest == "/dev/null" {
				rest = ""
			}
			base := filepath.Base(rest)
			if !strings.HasSuffix(base, ".go") && base != "" {
				current = nil
				continue
			}
			current = &cliDiffHunk{file: base}
			dontBuf.Reset()
			doBuf.Reset()

		case strings.HasPrefix(line, "+++ "):
			// Skip -- file name already captured from "---" line.

		case current == nil:
			// No active hunk yet; skip context/header lines.

		case strings.HasPrefix(line, "-"):
			code := line[1:]
			if cliIsUsefulCodeLine(code) {
				if dontBuf.Len() > 0 {
					dontBuf.WriteByte('\n')
				}
				dontBuf.WriteString(strings.TrimRight(code, " \t"))
			}

		case strings.HasPrefix(line, "+"):
			code := line[1:]
			if cliIsUsefulCodeLine(code) {
				if doBuf.Len() > 0 {
					doBuf.WriteByte('\n')
				}
				doBuf.WriteString(strings.TrimRight(code, " \t"))
			}
		}
	}
	flushCurrent()
	return hunks
}

// cliIsUsefulCodeLine returns true when the line is not blank or a standalone comment.
func cliIsUsefulCodeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	if strings.HasPrefix(trimmed, "//") {
		return false
	}
	return true
}

// cliTrimSnippet truncates s to maxLen bytes, appending "..." if truncated.
func cliTrimSnippet(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// cliMatchFindingsToHunks pairs each lint finding with the diff hunk for the same file.
func cliMatchFindingsToHunks(findings []cliLintFinding, hunks []cliDiffHunk) []cliLintPattern {
	hunkByFile := make(map[string]cliDiffHunk, len(hunks))
	for _, h := range hunks {
		if _, exists := hunkByFile[h.file]; !exists {
			hunkByFile[h.file] = h
		}
	}

	var patterns []cliLintPattern
	for _, f := range findings {
		hunk, matched := hunkByFile[f.file]
		glob := cliFileGlobFor(f.file)

		if matched {
			patterns = append(patterns, cliLintPattern{
				Rule:     f.rule,
				FileGlob: glob,
				DontCode: hunk.dontCode,
				DoCode:   hunk.doCode,
			})
		} else {
			patterns = append(patterns, cliLintPattern{
				Rule:     f.rule,
				FileGlob: glob,
				DontCode: "",
				DoCode:   "",
			})
		}
	}

	// If there were hunks but no lint findings, store diff hunks with a synthetic rule.
	if len(findings) == 0 {
		for _, h := range hunks {
			if h.dontCode == "" && h.doCode == "" {
				continue
			}
			patterns = append(patterns, cliLintPattern{
				Rule:     "diff-only",
				FileGlob: cliFileGlobFor(h.file),
				DontCode: h.dontCode,
				DoCode:   h.doCode,
			})
		}
	}

	return patterns
}

// cliFileGlobFor derives a file glob from a Go source basename. Mirrors tools/learn.go fileGlobFor.
func cliFileGlobFor(base string) string {
	if base == "" {
		return "*.go"
	}
	stem := strings.TrimSuffix(base, ".go")

	bareWordGlobs := map[string]string{
		"handler":    "*_handler.go",
		"handlers":   "*_handler.go",
		"server":     "*_server.go",
		"client":     "*_client.go",
		"repo":       "*_repo.go",
		"repository": "*_repository.go",
		"service":    "*_service.go",
		"model":      "*_model.go",
		"controller": "*_controller.go",
	}
	if glob, ok := bareWordGlobs[stem]; ok {
		return glob
	}

	domainSuffixes := []string{
		"_handler", "_handlers",
		"_test",
		"_server", "_client",
		"_middleware",
		"_controller",
		"_repository", "_repo",
		"_service",
		"_model",
		"_mock",
		"_gen", "_generated",
	}
	for _, suffix := range domainSuffixes {
		if strings.HasSuffix(stem, suffix) {
			return "*" + suffix + ".go"
		}
	}
	return "*.go"
}

// ── --query-knowledge implementation ────────────────────────────────────────

// runQueryKnowledge reads code context from stdin and queries the knowledge base.
func runQueryKnowledge(store *db.Store, filePath string) {
	// Derive file glob from the file path.
	glob := "*.go"
	if filePath != "" {
		base := filepath.Base(filePath)
		switch {
		case strings.HasSuffix(base, "_test.go"):
			glob = "*_test.go"
		case strings.HasSuffix(base, "_handler.go"):
			glob = "*_handler.go"
		case strings.HasSuffix(base, "_middleware.go"):
			glob = "*_middleware.go"
		}
	}

	// Read code context from stdin (for security -- avoid CLI arg).
	var codeContext string
	reader := bufio.NewReader(os.Stdin)
	data, err := io.ReadAll(reader)
	if err == nil {
		codeContext = strings.TrimSpace(string(data))
	}
	// Truncate to 1000 chars.
	if len(codeContext) > 1000 {
		codeContext = codeContext[:1000]
	}

	// Query lint patterns (limit 10, sorted by frequency).
	lintPatterns, err := store.QueryPatterns(glob, codeContext, 10)
	if err != nil {
		log.Fatalf("query-knowledge: querying lint patterns: %v", err)
	}

	if len(lintPatterns) == 0 {
		fmt.Println("No learned patterns for this context yet.")
		return
	}

	// Format patterns as prevention context.
	fmt.Println("LEARNED PATTERNS FOR THIS CONTEXT:")
	cap5 := lintPatterns
	if len(cap5) > 5 {
		cap5 = cap5[:5]
	}
	for _, p := range cap5 {
		dontLine := cliFirstLine(p.DontCode)
		doLine := cliFirstLine(p.DoCode)
		fmt.Printf("- [lint:%s x%d] %s\n", p.Rule, p.Frequency, dontLine)
		fmt.Printf("  -> DO: %s\n", doLine)
	}
}

// cliFirstLine returns the first non-empty line of s, trimmed of whitespace.
func cliFirstLine(s string) string {
	for _, line := range strings.SplitN(s, "\n", -1) {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(s)
}

// ── --healthcheck implementation ───────────────────────────────────────────

// healthcheckResult holds a single diagnostic check outcome.
type healthcheckResult struct {
	Name   string `json:"name"`
	Status string `json:"status"` // "pass", "fail", "warn"
	Detail string `json:"detail,omitempty"`
}

// runHealthcheck opens the database (or creates it), verifies schema, seeds,
// and tool registration, then prints a JSON report and returns 0 (all pass)
// or 1 (any failure).
func runHealthcheck(dbPath string) int {
	var results []healthcheckResult
	pass := func(name, detail string) { results = append(results, healthcheckResult{name, "pass", detail}) }
	fail := func(name, detail string) { results = append(results, healthcheckResult{name, "fail", detail}) }
	warn := func(name, detail string) { results = append(results, healthcheckResult{name, "warn", detail}) }

	// 1. Binary version
	pass("binary", fmt.Sprintf("go-guardian-mcp v%s", version))

	// 2. DB file and early schema check
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o700); err != nil {
		fail("db_directory", fmt.Sprintf("cannot create: %v", err))
		printHealthcheck(results)
		return 1
	}

	info, err := os.Stat(dbPath)
	if err != nil {
		fail("db_file", fmt.Sprintf("stat failed: %v", err))
	} else {
		perm := info.Mode().Perm()
		if perm&0o077 != 0 {
			warn("db_permissions", fmt.Sprintf("0%o — should be 0600 (owner-only)", perm))
		} else {
			pass("db_file", fmt.Sprintf("%s (%d bytes, 0%o)", dbPath, info.Size(), perm))
		}
	}

	// 3. Schema tables — check BEFORE NewStore to detect corruption
	// (NewStore triggers migration which auto-recovers missing tables).
	expectedTables := []string{
		"lint_patterns", "owasp_findings", "vuln_cache", "scan_history",
		"anti_patterns", "dep_decisions", "scan_snapshots", "session_findings",
		"renovate_preferences", "renovate_rules", "config_scores",
	}
	rawDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		fail("schema_precheck", fmt.Sprintf("cannot open db for schema check: %v", err))
	} else {
		defer rawDB.Close()
		tableRows, queryErr := func() ([]string, error) {
			rows, err := rawDB.Query(`SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
			if err != nil {
				return nil, err
			}
			defer rows.Close()
			var tables []string
			for rows.Next() {
				var name string
				if err := rows.Scan(&name); err != nil {
					return nil, err
				}
				tables = append(tables, name)
			}
			return tables, rows.Err()
		}()
		if queryErr != nil {
			fail("schema", fmt.Sprintf("cannot query tables: %v", queryErr))
		} else {
			tableSet := make(map[string]bool, len(tableRows))
			for _, t := range tableRows {
				tableSet[t] = true
			}
			missing := 0
			for _, t := range expectedTables {
				if !tableSet[t] {
					missing++
					fail("table_"+t, "missing")
				}
			}
			if missing == 0 {
				pass("schema", fmt.Sprintf("%d/%d tables present", len(expectedTables), len(expectedTables)))
			}
		}
	}

	store, err := db.NewStore(dbPath)
	if err != nil {
		fail("db_open", fmt.Sprintf("cannot open: %v", err))
		printHealthcheck(results)
		return 1
	}
	defer store.Close()

	// 4. Seed data
	counts, err := store.HealthcheckCounts()
	if err != nil {
		fail("seed_data", fmt.Sprintf("count query failed: %v", err))
	} else {
		for table, count := range counts {
			switch {
			case table == "anti_patterns" && count == 0:
				fail("seed_"+table, "0 rows — seed data missing")
			case table == "renovate_rules" && count == 0:
				fail("seed_"+table, "0 rows — seed data missing")
			case count == 0:
				pass("data_"+table, "0 rows (normal — populates during use)")
			default:
				pass("data_"+table, fmt.Sprintf("%d rows", count))
			}
		}
	}

	// 5. Tool registration
	s := server.NewMCPServer("go-guardian", version)
	tools.RegisterLearnFromLint(s, store)
	tools.RegisterQueryKnowledge(s, store, "")
	tools.RegisterCheckOWASP(s, store, ".")
	tools.RegisterCheckStaleness(s, store)
	tools.RegisterCheckDeps(s, store)
	tools.RegisterGetPatternStats(s, store)
	tools.RegisterSuggestFix(s, store)
	tools.RegisterLearnFromReview(s, store)
	tools.RegisterGetHealthTrends(s, store)
	tools.RegisterReportFinding(s, store, "")
	tools.RegisterGetSessionFindings(s, store, "")
	tools.RegisterValidateRenovateConfig(s, store)
	tools.RegisterAnalyzeRenovateConfig(s, store)
	tools.RegisterSuggestRenovateRule(s, store)
	tools.RegisterLearnRenovatePreference(s, store)
	tools.RegisterRenovateQueryKnowledge(s, store)
	tools.RegisterGetRenovateStats(s, store)
	pass("tools", "17 tools registered")

	// 6. Environment
	if os.Getenv("GO_GUARDIAN_SESSION_ID") != "" {
		pass("session", os.Getenv("GO_GUARDIAN_SESSION_ID"))
	} else {
		sidPath := filepath.Join(filepath.Dir(dbPath), "session-id")
		if data, err := os.ReadFile(sidPath); err == nil && strings.TrimSpace(string(data)) != "" {
			pass("session", strings.TrimSpace(string(data))+" (from file)")
		} else {
			warn("session", "no active session — run inside Claude Code or set GO_GUARDIAN_SESSION_ID")
		}
	}

	checksumPath := filepath.Join(filepath.Dir(dbPath), ".source-checksum")
	if storedChecksum, err := os.ReadFile(checksumPath); err == nil {
		short := strings.TrimSpace(string(storedChecksum))
		if len(short) > 12 {
			short = short[:12]
		}
		// If plugin root is available, verify checksum matches current source.
		pluginRoot := os.Getenv("CLAUDE_PLUGIN_ROOT")
		if pluginRoot != "" {
			sourceDir := filepath.Join(pluginRoot, "mcp-server")
			if currentChecksum, err := computeSourceChecksum(sourceDir); err == nil {
				stored := strings.TrimSpace(string(storedChecksum))
				if stored == currentChecksum {
					pass("build_cache", fmt.Sprintf("checksum %s — binary matches source", short))
				} else {
					warn("build_cache", fmt.Sprintf("checksum mismatch — binary is stale (have %s, want %s)", short, currentChecksum[:12]))
				}
			} else {
				pass("build_cache", fmt.Sprintf("checksum %s — source not readable for verification", short))
			}
		} else {
			pass("build_cache", fmt.Sprintf("checksum %s — source not available for verification", short))
		}
	} else {
		warn("build_cache", "no source checksum — binary may be stale")
	}

	printHealthcheck(results)

	for _, r := range results {
		if r.Status == "fail" {
			return 1
		}
	}
	return 0
}

// printHealthcheck renders the results to stdout as a human-readable summary.
func printHealthcheck(results []healthcheckResult) {
	passes, fails, warns := 0, 0, 0
	for _, r := range results {
		switch r.Status {
		case "pass":
			passes++
		case "fail":
			fails++
		case "warn":
			warns++
		}
		icon := "[OK]"
		if r.Status == "fail" {
			icon = "[FAIL]"
		} else if r.Status == "warn" {
			icon = "[WARN]"
		}
		fmt.Printf("  %-6s %-25s %s\n", icon, r.Name, r.Detail)
	}
	fmt.Printf("\n  %d passed, %d warnings, %d failed\n", passes, warns, fails)
}

// computeSourceChecksum computes a SHA-256 over all .go files in sourceDir
// (sorted, excluding vendor/), matching the shell checksum logic in launcher.sh.
func computeSourceChecksum(sourceDir string) (string, error) {
	var files []string
	err := filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == "vendor" {
			return filepath.SkipDir
		}
		if !d.IsDir() && strings.HasSuffix(path, ".go") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)

	// Hash each file's content individually, then hash the concatenated per-file hashes.
	// This mirrors: find ... | sort | xargs shasum -a 256 | shasum -a 256
	outer := sha256.New()
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			return "", err
		}
		h := sha256.Sum256(data)
		fmt.Fprintf(outer, "%s  %s\n", hex.EncodeToString(h[:]), f)
	}
	return hex.EncodeToString(outer.Sum(nil)), nil
}
