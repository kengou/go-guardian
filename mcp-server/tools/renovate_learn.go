package tools

import (
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// RunLearnRenovatePreference records a Renovate configuration preference in the database.
// It is the package-level entry point used by the CLI and ingest pipeline.
func RunLearnRenovatePreference(store *db.Store, category, description, dontConfig, doConfig string) (string, error) {
	category = strings.TrimSpace(category)
	description = strings.TrimSpace(description)

	if category == "" {
		return "", fmt.Errorf("category is required")
	}
	if description == "" {
		return "", fmt.Errorf("description is required")
	}

	if !renovateValidCategories[category] {
		valid := make([]string, 0, len(renovateValidCategories))
		for k := range renovateValidCategories {
			valid = append(valid, k)
		}
		return "", fmt.Errorf(
			"invalid category %q — must be one of: %s",
			category, strings.Join(valid, ", "),
		)
	}

	if err := store.InsertRenovatePreference(category, description, dontConfig, doConfig); err != nil {
		return "", fmt.Errorf("store error: %w", err)
	}

	return fmt.Sprintf(
		"Preference learned: [%s] %s (frequency will increment on repeat)",
		category, description,
	), nil
}
