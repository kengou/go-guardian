package tools

import (
	"fmt"
	"os"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
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

// RunSuggestRenovateRule suggests Renovate configuration rules for a given problem.
// It is the package-level entry point used by the CLI.
func RunSuggestRenovateRule(store *db.Store, problem, configPath string) (string, error) {
	if strings.TrimSpace(problem) == "" {
		return "", fmt.Errorf("problem is required")
	}

	category := renovateCategoryFromProblem(problem)

	// Collect matching rules from category and keyword search.
	var rules []db.RenovateRule
	seen := make(map[string]bool)

	if category != "" {
		catRules, err := store.QueryRenovateRules(category)
		if err != nil {
			return "", fmt.Errorf("query rules: %w", err)
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
		return "", fmt.Errorf("search rules: %w", err)
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
			return "", fmt.Errorf("query preferences: %w", err)
		}
	}

	if len(rules) == 0 && len(prefs) == 0 {
		return fmt.Sprintf(
			"No matching rules found for: %q\nTry different keywords or use query_renovate_knowledge to browse categories.",
			problem,
		), nil
	}

	// Read config file if provided.
	var configData []byte
	if configPath != "" {
		configData, _ = os.ReadFile(configPath)
	}

	// Format output: up to 3 results.
	ruleCount := len(rules)
	prefCount := len(prefs)
	return formatRenovateSuggestions(problem, rules, prefs, configData, ruleCount, prefCount), nil
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
