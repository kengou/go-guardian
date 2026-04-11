// Package tools registers MCP tool handlers for go-guardian.
package tools

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/owasp"
)

// RunCheckOWASP scans scanPath for OWASP findings and returns a formatted
// report. It performs the same store inserts (OWASP findings, scan history,
// scan snapshot) as the prior MCP handler, so callers get identical
// side-effects. scanPath is validated to be within projectRoot — paths
// outside it return an error without any store mutations.
//
// This function is the CLI-callable surface for OWASP scanning and is
// invoked by the go-guardian scan subcommand.
func RunCheckOWASP(store *db.Store, projectRoot, scanPath string) (string, error) {
	scanPath = strings.TrimSpace(scanPath)
	if scanPath == "" {
		return "", fmt.Errorf("'path' parameter is required")
	}

	if err := validateScanPath(scanPath, projectRoot); err != nil {
		return "", err
	}

	rules := owasp.DefaultRules()

	info, err := os.Stat(scanPath)
	if err != nil {
		return "", fmt.Errorf("cannot stat %q: %w", scanPath, err)
	}

	var findings []owasp.Finding
	if info.IsDir() {
		findings, err = owasp.ScanDirectory(scanPath, rules)
	} else {
		findings, err = owasp.ScanFile(scanPath, rules)
	}
	if err != nil {
		return "", fmt.Errorf("scanning %q: %w", scanPath, err)
	}

	// Persist findings to the database in a single batch.
	items := make([]db.OWASPFindingItem, len(findings))
	for i, f := range findings {
		items[i] = db.OWASPFindingItem{
			Category:    f.Category,
			FilePattern: filepath.Base(f.File),
			Finding:     f.Message,
			FixPattern:  "",
		}
	}
	if err := store.InsertOWASPFindingsBatch(items); err != nil {
		log.Printf("owasp: batch insert failed: %v", err)
	}

	_ = store.UpdateScanHistory("owasp", scanPath, len(findings))
	_ = store.InsertScanSnapshot("owasp", scanPath, len(findings), buildOWASPSnapshotDetail(findings))

	return formatFindings(scanPath, findings), nil
}

// buildOWASPSnapshotDetail creates a JSON detail blob from OWASP findings,
// grouping counts by OWASP category (e.g. A02, A03).
func buildOWASPSnapshotDetail(findings []owasp.Finding) string {
	cats := make(map[string]int)
	for _, f := range findings {
		cats[string(f.Category)]++
	}
	detail := map[string]interface{}{"categories": cats}
	b, _ := json.Marshal(detail)
	return string(b)
}

// severityOrder defines the display order for severity levels.
var severityOrder = []owasp.Severity{
	owasp.SeverityCritical,
	owasp.SeverityHigh,
	owasp.SeverityMedium,
	owasp.SeverityLow,
}

// formatFindings produces the human-readable OWASP scan report.
func formatFindings(scanPath string, findings []owasp.Finding) string {
	if len(findings) == 0 {
		return fmt.Sprintf("OWASP scan clean: no findings in %s", scanPath)
	}

	// Group findings by severity.
	bySeverity := make(map[owasp.Severity][]owasp.Finding)
	for _, f := range findings {
		bySeverity[f.Severity] = append(bySeverity[f.Severity], f)
	}

	// Sort each group by file then line for deterministic output.
	for sev := range bySeverity {
		sort.Slice(bySeverity[sev], func(i, j int) bool {
			a, b := bySeverity[sev][i], bySeverity[sev][j]
			if a.File != b.File {
				return a.File < b.File
			}
			return a.Line < b.Line
		})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "OWASP Scan Results: %s\n", scanPath)
	fmt.Fprintf(&sb, "Found %d finding(s)\n", len(findings))

	for _, sev := range severityOrder {
		group, ok := bySeverity[sev]
		if !ok || len(group) == 0 {
			continue
		}
		fmt.Fprintf(&sb, "\n%s (%d):\n", string(sev), len(group))
		for _, f := range group {
			relFile := f.File
			if rel, err := filepath.Rel(scanPath, f.File); err == nil {
				relFile = rel
			}
			fmt.Fprintf(&sb, "  [%s] %s:%d — %s\n", f.Category, relFile, f.Line, f.Message)
			if f.Fix != "" {
				fmt.Fprintf(&sb, "  → FIX: %s\n", f.Fix)
			}
		}
	}

	return sb.String()
}
