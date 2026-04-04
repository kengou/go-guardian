package tools

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// renovateValidCategories lists the accepted Renovate rule categories.
var renovateValidCategories = map[string]bool{
	"automerge":          true,
	"grouping":           true,
	"scheduling":         true,
	"security":           true,
	"custom_datasources": true,
	"automation":         true,
}

// renovateCategoryKeywords maps keywords found in problem descriptions to categories.
var renovateCategoryKeywords = []struct {
	keywords []string
	category string
}{
	{[]string{"too many prs", "flood", "limit"}, "scheduling"},
	{[]string{"automerge", "auto merge", "auto-merge"}, "automerge"},
	{[]string{"group", "monorepo", "together"}, "grouping"},
	{[]string{"security", "vulnerability", "cve"}, "security"},
	{[]string{"custom", "datasource", "wiz", "binary"}, "custom_datasources"},
	{[]string{"action", "makefile", "dockerfile", "pin", "digest", "track"}, "automation"},
}

// renovateCategoryFromProblem maps a problem description to a rule category by
// searching for known keywords. Returns empty string if no match.
func renovateCategoryFromProblem(problem string) string {
	lower := strings.ToLower(problem)
	for _, ck := range renovateCategoryKeywords {
		for _, kw := range ck.keywords {
			if strings.Contains(lower, kw) {
				return ck.category
			}
		}
	}
	return ""
}

// RegisterSuggestRenovateRule registers the suggest_renovate_rule MCP tool on the server.
func RegisterSuggestRenovateRule(s ToolRegistrar, store *db.Store) {
	tool := mcp.NewTool("suggest_renovate_rule",
		mcp.WithDescription(
			"Suggest Renovate configuration rules for a given problem. "+
				"Searches the rule database and learned preferences by keyword and category, "+
				"returning up to 3 matches with DON'T/DO examples.",
		),
		mcp.WithString("problem",
			mcp.Required(),
			mcp.Description("Description of the Renovate problem (e.g., \"too many PRs\", \"automerge patches\")"),
		),
		mcp.WithString("config_path",
			mcp.Description("Optional path to current renovate.json — if provided, shows a concrete diff of what to change"),
		),
	)
	s.AddTool(tool, handleSuggestRenovateRule(store))
}

func handleSuggestRenovateRule(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		problem := request.GetString("problem", "")
		if strings.TrimSpace(problem) == "" {
			return mcp.NewToolResultError("problem is required"), nil
		}
		configPath := request.GetString("config_path", "")

		category := renovateCategoryFromProblem(problem)

		// Collect matching rules from category and keyword search.
		var rules []db.RenovateRule
		seen := make(map[string]bool)

		if category != "" {
			catRules, err := store.QueryRenovateRules(category)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("query rules: %v", err)), nil
			}
			for _, r := range catRules {
				if !seen[r.RuleID] {
					seen[r.RuleID] = true
					rules = append(rules, r)
				}
			}
		}

		searchRules, err := store.SearchRenovateRules(problem, 10)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("search rules: %v", err)), nil
		}
		for _, r := range searchRules {
			if !seen[r.RuleID] {
				seen[r.RuleID] = true
				rules = append(rules, r)
			}
		}

		// Query learned preferences.
		var prefs []db.RenovatePreference
		if category != "" {
			prefs, err = store.QueryRenovatePreferences(category, 5)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("query preferences: %v", err)), nil
			}
		}

		if len(rules) == 0 && len(prefs) == 0 {
			return mcp.NewToolResultText(fmt.Sprintf(
				"No matching rules found for: %q\nTry different keywords or use query_renovate_knowledge to browse categories.",
				problem,
			)), nil
		}

		// Read config file if provided.
		var configData []byte
		if configPath != "" {
			configData, _ = os.ReadFile(configPath)
		}

		// Format output: up to 3 results.
		ruleCount := len(rules)
		prefCount := len(prefs)
		return mcp.NewToolResultText(formatRenovateSuggestions(problem, rules, prefs, configData, ruleCount, prefCount)), nil
	}
}

// formatRenovateSuggestions renders the suggestion output.
func formatRenovateSuggestions(problem string, rules []db.RenovateRule, prefs []db.RenovatePreference, configData []byte, totalRules, totalPrefs int) string {
	var out strings.Builder
	fmt.Fprintf(&out, "=== Suggestions for: %q ===\n", problem)

	shown := 0
	limit := 3

	for i, r := range rules {
		if shown >= limit {
			break
		}
		_ = i
		shown++
		fmt.Fprintf(&out, "\n%d. [%s] %s (%s)\n", shown, r.RuleID, r.Title, r.Severity)
		fmt.Fprintf(&out, "   %s\n", r.Description)
		if r.DontConfig != "" {
			fmt.Fprintf(&out, "\n   DON'T: %s\n", r.DontConfig)
		}
		if r.DoConfig != "" {
			fmt.Fprintf(&out, "   DO: %s\n", r.DoConfig)
		}
		if len(configData) > 0 && r.DoConfig != "" {
			fmt.Fprintf(&out, "\n   Config diff for your file:\n")
			fmt.Fprintf(&out, "   - current: (your config)\n")
			fmt.Fprintf(&out, "   + add: %s\n", r.DoConfig)
		}
	}

	for _, p := range prefs {
		if shown >= limit {
			break
		}
		shown++
		fmt.Fprintf(&out, "\n%d. [learned] %s (freq: %d)\n", shown, p.Description, p.Frequency)
		if p.DontConfig != "" {
			fmt.Fprintf(&out, "\n   DON'T: %s\n", p.DontConfig)
		}
		if p.DoConfig != "" {
			fmt.Fprintf(&out, "   DO: %s\n", p.DoConfig)
		}
	}

	ruleShown := shown
	if ruleShown > len(rules) {
		ruleShown = len(rules)
	}
	prefShown := shown - ruleShown

	fromRules := ruleShown
	fromPrefs := prefShown
	if fromRules > totalRules {
		fromRules = totalRules
	}
	if fromPrefs > totalPrefs {
		fromPrefs = totalPrefs
	}

	fmt.Fprintf(&out, "\n%d matching rules found", totalRules+totalPrefs)
	if totalRules > 0 || totalPrefs > 0 {
		fmt.Fprintf(&out, " (%d from rules, %d from learned preferences)", totalRules, totalPrefs)
	}
	fmt.Fprintln(&out)

	return out.String()
}
