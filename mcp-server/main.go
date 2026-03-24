package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/tools"
	"github.com/mark3labs/mcp-go/server"
)

const version = "0.1.0"

func main() {
	dbPath := flag.String("db", ".go-guardian/guardian.db", "path to the SQLite database file")
	showVersion := flag.Bool("version", false, "print version and exit")
	prefetch := flag.Bool("prefetch", false, "fetch CVE data for modules in go.mod and exit")
	nvdKey := flag.String("nvd-key", os.Getenv("NVD_API_KEY"), "NVD API key for CVSS enrichment (or set NVD_API_KEY env var)")
	updateOWASP := flag.Bool("update-owasp", false, "fetch new OWASP-tagged Go advisories from GHSA and update rule patterns")
	githubToken := flag.String("github-token", os.Getenv("GITHUB_TOKEN"), "GitHub token for GHSA API (optional, increases rate limits)")
	goModPath := flag.String("go-mod", "go.mod", "path to go.mod for --prefetch mode")
	projectDir := flag.String("project", "", "project root for scan path validation (defaults to directory of --db)")
	flag.Parse()

	if *projectDir == "" {
		*projectDir = filepath.Dir(*dbPath)
	}

	if *showVersion {
		fmt.Printf("go-guardian-mcp v%s\n", version)
		os.Exit(0)
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

	log.Printf("go-guardian MCP server v%s started, db: %s\n", version, *dbPath)

	// Create the MCP server and register all tools.
	s := server.NewMCPServer("go-guardian", version)
	tools.RegisterLearnFromLint(s, store)
	tools.RegisterQueryKnowledge(s, store)
	tools.RegisterCheckOWASP(s, store, *projectDir)
	tools.RegisterCheckStaleness(s, store)
	tools.RegisterCheckDeps(s, store)
	tools.RegisterGetPatternStats(s, store)
	tools.RegisterSuggestFix(s, store)

	log.Printf("registered 7 tools: learn_from_lint, query_knowledge, check_owasp, check_staleness, check_deps, get_pattern_stats, suggest_fix (use --prefetch to pre-populate CVE data, --update-owasp to refresh OWASP rules)\n")

	// Start serving via stdio. ServeStdio handles SIGINT/SIGTERM gracefully.
	if err := server.ServeStdio(s); err != nil {
		log.Fatalf("MCP server error: %v", err)
	}
}
