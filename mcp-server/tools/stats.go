package tools

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterGetPatternStats registers the get_pattern_stats MCP tool with the given server.
func RegisterGetPatternStats(s *server.MCPServer, store *db.Store) {
	tool := mcp.NewTool(
		"get_pattern_stats",
		mcp.WithDescription(
			"Return a dashboard-style summary of the go-guardian knowledge base: "+
				"top lint patterns by frequency, OWASP posture, anti-pattern counts, "+
				"and recent scan history. Optionally scoped to a specific project.",
		),
		mcp.WithString("project",
			mcp.Description("Optional project identifier; if empty, returns global stats"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		project := req.GetString("project", "")

		stats, err := store.GetPatternStats(project)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to get stats: %v", err)), nil
		}

		text := formatPatternStats(stats)
		return mcp.NewToolResultText(text), nil
	})
}

// formatPatternStats renders a PatternStats as a human-readable dashboard.
func formatPatternStats(stats *db.PatternStats) string {
	// Check if the knowledge base is empty.
	if stats.TotalLintPatterns == 0 && stats.TotalAntiPatterns == 0 && len(stats.RecentScans) == 0 {
		return "No patterns learned yet. Run /go to start building your knowledge base."
	}

	var sb strings.Builder

	sb.WriteString("Go Guardian Knowledge Base — Stats\n")
	sb.WriteString("===================================\n")
	sb.WriteString("\n")

	// Lint patterns section.
	fmt.Fprintf(&sb, "Lint Patterns Learned: %d total\n", stats.TotalLintPatterns)

	if len(stats.TopLintPatterns) > 0 {
		sb.WriteString("\nTop 10 by frequency:\n")
		for i, p := range stats.TopLintPatterns {
			fmt.Fprintf(&sb, "  %2d. %s (×%d) — %s\n", i+1, p.Rule, p.Frequency, p.FileGlob)
		}
	}

	sb.WriteString("\n")

	// Anti-patterns section.
	// The seeded count is approximated as total anti-patterns minus learned ones
	// (patterns with source="learned"). For display purposes we report total.
	// PatternStats doesn't break out seeded vs learned counts, so we show total only.
	fmt.Fprintf(&sb, "Anti-Patterns Tracked: %d total\n", stats.TotalAntiPatterns)

	sb.WriteString("\n")

	// OWASP posture section.
	if len(stats.OWASPCounts) > 0 {
		sb.WriteString("OWASP Posture:\n")

		// Sort categories for deterministic output.
		cats := make([]string, 0, len(stats.OWASPCounts))
		for cat := range stats.OWASPCounts {
			cats = append(cats, cat)
		}
		sort.Strings(cats)

		// Find the longest category name for alignment.
		maxLen := 0
		for _, cat := range cats {
			if len(cat) > maxLen {
				maxLen = len(cat)
			}
		}

		for _, cat := range cats {
			cnt := stats.OWASPCounts[cat]
			noun := "findings"
			if cnt == 1 {
				noun = "finding"
			}
			fmt.Fprintf(&sb, "  %-*s %d %s\n", maxLen+1, cat+":", cnt, noun)
		}
	} else {
		sb.WriteString("OWASP Posture:\n")
		sb.WriteString("  (no findings recorded)\n")
	}

	sb.WriteString("\n")

	// Recent scans section.
	if len(stats.RecentScans) > 0 {
		sb.WriteString("Recent Scans:\n")

		// Find the longest scan type for alignment.
		maxLen := 0
		for _, h := range stats.RecentScans {
			if len(h.ScanType) > maxLen {
				maxLen = len(h.ScanType)
			}
		}

		for _, h := range stats.RecentScans {
			noun := "findings"
			if h.FindingsCount == 1 {
				noun = "finding"
			}
			ts := h.LastRun.Format("2006-01-02 15:04")
			fmt.Fprintf(&sb, "  %-*s %s (%d %s)\n",
				maxLen+1, h.ScanType+":", ts, h.FindingsCount, noun)
		}
	} else {
		sb.WriteString("Recent Scans:\n")
		sb.WriteString("  (none)\n")
	}

	return strings.TrimRight(sb.String(), "\n")
}
