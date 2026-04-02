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
func RegisterLearnRenovatePreference(s *server.MCPServer, store *db.Store) {
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

func handleLearnRenovatePreference(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, request mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		category := request.GetString("category", "")
		description := request.GetString("description", "")
		dontConfig := request.GetString("dont_config", "")
		doConfig := request.GetString("do_config", "")

		category = strings.TrimSpace(category)
		description = strings.TrimSpace(description)

		if category == "" {
			return mcp.NewToolResultError("category is required"), nil
		}
		if description == "" {
			return mcp.NewToolResultError("description is required"), nil
		}

		if !renovateValidCategories[category] {
			valid := make([]string, 0, len(renovateValidCategories))
			for k := range renovateValidCategories {
				valid = append(valid, k)
			}
			return mcp.NewToolResultError(fmt.Sprintf(
				"invalid category %q — must be one of: %s",
				category, strings.Join(valid, ", "),
			)), nil
		}

		if err := store.InsertRenovatePreference(category, description, dontConfig, doConfig); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("store error: %v", err)), nil
		}

		return mcp.NewToolResultText(fmt.Sprintf(
			"Preference learned: [%s] %s (frequency will increment on repeat)",
			category, description,
		)), nil
	}
}
