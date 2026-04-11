package tools

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// staleThresholds maps scan type to the duration after which the scan is
// considered stale. A zero value means the scan type is never stale.
var staleThresholds = map[string]time.Duration{
	"vuln":        3 * 24 * time.Hour,
	"owasp":       7 * 24 * time.Hour,
	"owasp_rules": 30 * 24 * time.Hour,
	"full":        14 * 24 * time.Hour,
	"lint":        0, // never stale — continuous learning
}

// scanRunCommand maps scan type to the suggested CLI command shown in warnings.
var scanRunCommand = map[string]string{
	"vuln":        "/go security",
	"owasp":       "/go security",
	"full":        "/go security",
	"owasp_rules": "go-guardian-mcp --update-owasp",
}

// ProjectID returns a normalised project identifier derived from a filesystem
// path. It cleans the path and returns the last two path components joined
// with "/". For paths with fewer than two meaningful components it returns the
// cleaned basename.
func ProjectID(path string) string {
	cleaned := filepath.Clean(path)
	base := filepath.Base(cleaned)
	parent := filepath.Base(filepath.Dir(cleaned))
	if parent == "." || parent == "/" || parent == cleaned {
		return base
	}
	return parent + "/" + base
}

// RunCheckStaleness returns a human-readable staleness report for the project
// rooted at projectPath. It is a thin exported wrapper around the existing
// checkStaleness helper so that CLI callers (go-guardian scan) and MCP
// callers share the same implementation.
func RunCheckStaleness(store *db.Store, projectPath string) (string, error) {
	projectPath = strings.TrimSpace(projectPath)
	if projectPath == "" {
		return "", fmt.Errorf("project_path is required")
	}
	return checkStaleness(store, projectPath)
}

// checkStaleness contains the core staleness-check logic, separated for
// testability. It returns the formatted report string.
func checkStaleness(store *db.Store, projectPath string) (string, error) {
	projectID := ProjectID(projectPath)

	history, err := store.GetScanHistory(projectID)
	if err != nil {
		return "", err
	}

	// Build a map of scan_type -> most-recent ScanHistory record.
	// GetScanHistory returns rows ordered most-recent first, so the first
	// occurrence of each type is the latest.
	latest := make(map[string]db.ScanHistory)
	for _, h := range history {
		if _, seen := latest[h.ScanType]; !seen {
			latest[h.ScanType] = h
		}
	}

	// Collect scan types with non-zero thresholds (i.e. those that can go stale),
	// sorted for deterministic output.
	var tracked []string
	for scanType, threshold := range staleThresholds {
		if threshold > 0 {
			tracked = append(tracked, scanType)
		}
	}
	sort.Strings(tracked)

	type scanResult struct {
		scanType  string
		stale     bool
		neverRun  bool
		age       time.Duration
		threshold time.Duration
	}

	var results []scanResult
	anyStale := false

	for _, scanType := range tracked {
		threshold := staleThresholds[scanType]
		h, found := latest[scanType]
		if !found {
			results = append(results, scanResult{
				scanType:  scanType,
				stale:     true,
				neverRun:  true,
				threshold: threshold,
			})
			anyStale = true
			continue
		}
		age := time.Since(h.LastRun)
		stale := age > threshold
		if stale {
			anyStale = true
		}
		results = append(results, scanResult{
			scanType:  scanType,
			stale:     stale,
			neverRun:  false,
			age:       age,
			threshold: threshold,
		})
	}

	var sb strings.Builder

	if !anyStale {
		sb.WriteString("All scans current.\n")
		for _, r := range results {
			days := int(r.age.Hours() / 24)
			fmt.Fprintf(&sb, "  \u2713 %s scan: %s\n", r.scanType, daysAgo(days))
		}
	} else {
		sb.WriteString("Stale scans detected:\n")
		for _, r := range results {
			if r.stale {
				if r.neverRun {
					cmd := scanRunCommand[r.scanType]
					fmt.Fprintf(&sb, "  \u26a0 %s scan: never run \u2014 run: %s\n", r.scanType, cmd)
				} else {
					days := int(r.age.Hours() / 24)
					threshDays := int(r.threshold.Hours() / 24)
					cmd := scanRunCommand[r.scanType]
					fmt.Fprintf(&sb, "  \u26a0 %s scan: %s (threshold: %d days) \u2014 run: %s\n",
						r.scanType, daysAgo(days), threshDays, cmd)
				}
			} else {
				days := int(r.age.Hours() / 24)
				fmt.Fprintf(&sb, "  \u2713 %s scan: %s\n", r.scanType, daysAgo(days))
			}
		}
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}

// daysAgo formats a day count as a human-readable string, e.g. "1 day ago" or
// "5 days ago".
func daysAgo(days int) string {
	if days == 1 {
		return "1 day ago"
	}
	return fmt.Sprintf("%d days ago", days)
}
