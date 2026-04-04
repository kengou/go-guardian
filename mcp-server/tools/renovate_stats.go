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

// RegisterGetRenovateStats registers the get_renovate_stats tool on the MCP server.
func RegisterGetRenovateStats(s ToolRegistrar, store *db.Store) {
	tool := mcp.NewTool("get_renovate_stats",
		mcp.WithDescription("Dashboard showing rule coverage, learned preferences, and config score history"),
		mcp.WithString("config_path", mcp.Description("Config path for score history (optional)")),
	)
	s.AddTool(tool, handleGetRenovateStats(store))
}

func handleGetRenovateStats(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var out strings.Builder

		fmt.Fprintln(&out, "=== Renovate Guardian Dashboard ===")

		// --- Rule Coverage ---
		fmt.Fprintln(&out, "\nRule Coverage:")

		totalRules, err := store.TotalRenovateRuleCount()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to query total rule count: %v", err)), nil
		}
		fmt.Fprintf(&out, "  Total rules: %d\n", totalRules)

		catCounts, err := store.RenovateRuleCountByCategory()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to query rule categories: %v", err)), nil
		}

		// Sort categories alphabetically for stable output.
		cats := make([]string, 0, len(catCounts))
		for cat := range catCounts {
			cats = append(cats, cat)
		}
		sort.Strings(cats)

		// Find the longest category name for alignment (including trailing colon).
		maxLen := 0
		for _, cat := range cats {
			if len(cat)+1 > maxLen {
				maxLen = len(cat) + 1
			}
		}

		for _, cat := range cats {
			label := cat + ":"
			fmt.Fprintf(&out, "  %-*s %d rules\n", maxLen, label, catCounts[cat])
		}

		// --- Learned Preferences ---
		fmt.Fprintln(&out, "\nLearned Preferences:")

		prefCount, err := store.RenovatePreferenceCount()
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("failed to query preference count: %v", err)), nil
		}
		fmt.Fprintf(&out, "  %d total\n", prefCount)

		if prefCount > 0 {
			topPrefs, err := store.QueryRenovatePreferences("", 5)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to query preferences: %v", err)), nil
			}
			for _, p := range topPrefs {
				fmt.Fprintf(&out, "  [freq:%d] %s — %s\n", p.Frequency, p.Category, p.Description)
			}
		}

		// --- Config Score History ---
		fmt.Fprintln(&out, "\nConfig Score History:")

		configPath := request.GetString("config_path", "")

		var scores []db.ConfigScore
		if configPath != "" {
			scores, err = store.GetConfigScores(configPath, 5)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to query config scores: %v", err)), nil
			}
			if len(scores) > 0 {
				fmt.Fprintf(&out, "  Path: %s\n", configPath)
			}
		} else {
			scores, err = store.GetRecentConfigScores(5)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("failed to query recent scores: %v", err)), nil
			}
		}

		if len(scores) == 0 {
			fmt.Fprintln(&out, "  No score history available")
		} else {
			for _, sc := range scores {
				fmt.Fprintf(&out, "  %s: %d/100 (%d findings) [%s]\n",
					sc.CreatedAt.Format("2006-01-02"), sc.Score, sc.FindingsCount, sc.ConfigPath)
			}
			fmt.Fprintf(&out, "  Trend: %s\n", renovateComputeTrend(scores))
		}

		return mcp.NewToolResultText(out.String()), nil
	}
}

// renovateComputeTrend examines the most recent scores (ordered newest-first) and
// returns a human-readable trend string.
func renovateComputeTrend(scores []db.ConfigScore) string {
	if len(scores) < 2 {
		return "stable \u2192"
	}

	// scores[0] is the latest, scores[1] is the previous.
	latest := scores[0].Score
	previous := scores[1].Score

	if latest > previous {
		return "improving \u2191"
	}
	if latest < previous {
		return "degrading \u2193"
	}
	return "stable \u2192"
}
