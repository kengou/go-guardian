package tools

import (
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// RunQueryRenovateKnowledge queries the Renovate knowledge base by category and/or
// keyword. It is the package-level entry point used by the CLI.
func RunQueryRenovateKnowledge(store *db.Store, category, query string) (string, error) {
	category = strings.TrimSpace(category)
	query = strings.TrimSpace(query)

	var rules []db.RenovateRule
	var prefs []db.RenovatePreference
	var err error

	seenRules := make(map[string]bool)

	if category != "" {
		rules, err = store.QueryRenovateRules(category)
		if err != nil {
			return "", fmt.Errorf("query rules: %w", err)
		}
		for _, r := range rules {
			seenRules[r.RuleID] = true
		}

		prefs, err = store.QueryRenovatePreferences(category, 10)
		if err != nil {
			return "", fmt.Errorf("query preferences: %w", err)
		}
	}

	if query != "" {
		searchRules, err := store.SearchRenovateRules(query, 10)
		if err != nil {
			return "", fmt.Errorf("search rules: %w", err)
		}
		for _, r := range searchRules {
			if !seenRules[r.RuleID] {
				seenRules[r.RuleID] = true
				rules = append(rules, r)
			}
		}

		// If no category was specified, also query all preferences and filter
		// by keyword in the description.
		if category == "" {
			allPrefs, err := store.QueryRenovatePreferences("", 50)
			if err != nil {
				return "", fmt.Errorf("query preferences: %w", err)
			}
			lower := strings.ToLower(query)
			for _, p := range allPrefs {
				if strings.Contains(strings.ToLower(p.Description), lower) ||
					strings.Contains(strings.ToLower(p.Category), lower) {
					prefs = append(prefs, p)
				}
			}
		}
	}

	// If neither category nor query was specified, return everything.
	if category == "" && query == "" {
		rules, err = store.QueryRenovateRules("")
		if err != nil {
			return "", fmt.Errorf("query rules: %w", err)
		}
		prefs, err = store.QueryRenovatePreferences("", 10)
		if err != nil {
			return "", fmt.Errorf("query preferences: %w", err)
		}
	}

	// Limit combined output to 10 items total (rules first, then prefs).
	maxResults := 10
	ruleLimit := len(rules)
	if ruleLimit > maxResults {
		ruleLimit = maxResults
	}
	rules = rules[:ruleLimit]

	remaining := maxResults - ruleLimit
	if len(prefs) > remaining {
		prefs = prefs[:remaining]
	}

	if len(rules) == 0 && len(prefs) == 0 {
		label := "all"
		if category != "" {
			label = category
		} else if query != "" {
			label = query
		}
		return fmt.Sprintf(
			"No results found for %q. Try a different category or keyword.", label,
		), nil
	}

	return formatRenovateQueryResults(category, query, rules, prefs), nil
}

// formatRenovateQueryResults renders the knowledge query output.
func formatRenovateQueryResults(category, query string, rules []db.RenovateRule, prefs []db.RenovatePreference) string {
	var out strings.Builder

	label := "all"
	if category != "" {
		label = category
	} else if query != "" {
		label = fmt.Sprintf("%q", query)
	}
	fmt.Fprintf(&out, "=== Renovate Knowledge: %s ===\n", label)

	if len(rules) > 0 {
		fmt.Fprintf(&out, "\nRules (%d):\n", len(rules))
		for _, r := range rules {
			fmt.Fprintf(&out, "  [%s] %s: %s\n", r.RuleID, r.Severity, r.Title)
		}
	}

	if len(prefs) > 0 {
		fmt.Fprintf(&out, "\nLearned Preferences (%d):\n", len(prefs))
		for _, p := range prefs {
			fmt.Fprintf(&out, "  [freq:%d] %s\n", p.Frequency, p.Description)
		}
	}

	total := len(rules) + len(prefs)
	fmt.Fprintf(&out, "\n%d results (%d rules, %d preferences)\n", total, len(rules), len(prefs))

	return out.String()
}
