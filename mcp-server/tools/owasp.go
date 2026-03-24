// Package tools registers MCP tool handlers for go-guardian.
package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/owasp"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterCheckOWASP registers the check_owasp MCP tool.
// projectRoot is the absolute path to the project being guarded;
// scan paths are validated to be within this root (fixes FINDING-07).
func RegisterCheckOWASP(s *server.MCPServer, store *db.Store, projectRoot string) {
	tool := mcp.NewTool(
		"check_owasp",
		mcp.WithDescription("Scan Go source files for OWASP Top 10 security issues (A02, A03, A05, A09, A10)."),
		mcp.WithString(
			"path",
			mcp.Required(),
			mcp.Description("File path or directory to scan for OWASP issues."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		rawPath, ok := req.Params.Arguments["path"].(string)
		if !ok || strings.TrimSpace(rawPath) == "" {
			return mcp.NewToolResultText("error: 'path' parameter is required"), nil
		}
		scanPath := strings.TrimSpace(rawPath)

		// SECURITY: reject paths outside project root (fixes FINDING-07).
		if err := validateScanPath(scanPath, projectRoot); err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("error: %v", err)), nil
		}

		rules := owasp.DefaultRules()

		// Determine whether we are scanning a file or directory.
		info, err := os.Stat(scanPath)
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("error: cannot stat %q: %v", scanPath, err)), nil
		}

		var findings []owasp.Finding
		if info.IsDir() {
			findings, err = owasp.ScanDirectory(scanPath, rules)
		} else {
			findings, err = owasp.ScanFile(scanPath, rules)
		}
		if err != nil {
			return mcp.NewToolResultText(fmt.Sprintf("error scanning %q: %v", scanPath, err)), nil
		}

		// Persist each finding to the database.
		for _, f := range findings {
			filePattern := filepath.Base(f.File)
			_ = store.InsertOWASPFinding(f.Category, filePattern, f.Message, "")
		}

		// Update scan history.
		_ = store.UpdateScanHistory("owasp", scanPath, len(findings))

		// Format output.
		text := formatFindings(scanPath, findings)
		return mcp.NewToolResultText(text), nil
	})
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
