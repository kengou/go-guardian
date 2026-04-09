package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterLearnRenovatePreference registers the learn_renovate_preference MCP tool on the server.
func RegisterLearnRenovatePreference(s ToolRegistrar, store *db.Store) {
	tool := mcp.NewTool("learn_renovate_preference",
		mcp.WithDescription(
			"Record a Renovate configuration preference so future suggestions "+
				"incorporate team conventions. If the same category+description already "+
				"exists, the frequency counter is incremented.",
		),
		mcp.WithString("category",
			mcp.Required(),
			mcp.Description("Category: automerge, grouping, scheduling, security, custom_datasources, or automation"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Human-readable description of the preference"),
		),
		mcp.WithString("dont_config",
			mcp.Description("JSON snippet showing the bad/avoid configuration (optional)"),
		),
		mcp.WithString("do_config",
			mcp.Description("JSON snippet showing the recommended configuration (optional)"),
		),
	)
	s.AddTool(tool, handleLearnRenovatePreference(store))
}

// RunLearnRenovatePreference records a Renovate configuration preference in the database.
// It is the package-level entry point used by both the MCP handler and the CLI.
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

func handleLearnRenovatePreference(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		category := request.GetString("category", "")
		description := request.GetString("description", "")
		dontConfig := request.GetString("dont_config", "")
		doConfig := request.GetString("do_config", "")

		result, err := RunLearnRenovatePreference(store, category, description, dontConfig, doConfig)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}
