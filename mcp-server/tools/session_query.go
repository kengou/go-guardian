package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterGetSessionFindings registers the get_session_findings MCP tool.
// sessionID is captured in the closure so agents don't need to pass it.
func RegisterGetSessionFindings(s *server.MCPServer, store *db.Store, sessionID string) {
	tool := mcp.NewTool(
		"get_session_findings",
		mcp.WithDescription(
			"Query findings reported by other agents in the current scan session. "+
				"Use this to see what other agents have flagged before starting your analysis.",
		),
		mcp.WithString("agent",
			mcp.Description("Filter by reporting agent (e.g. reviewer, security) (optional)"),
		),
		mcp.WithString("file_path",
			mcp.Description("Filter by file path (substring match) (optional)"),
		),
		mcp.WithString("finding_type",
			mcp.Description("Filter by finding type (optional)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := handleGetSessionFindings(store, sessionID, req)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("get_session_findings: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})
}

func handleGetSessionFindings(store *db.Store, sessionID string, req mcp.CallToolRequest) (string, error) {
	if sessionID == "" {
		return "No active session — session findings are only available during a /go scan.", nil
	}

	agent := strings.TrimSpace(req.GetString("agent", ""))
	filePath := strings.TrimSpace(req.GetString("file_path", ""))
	findingType := strings.TrimSpace(req.GetString("finding_type", ""))

	var findings []db.SessionFinding
	var err error

	if filePath != "" {
		findings, err = store.GetSessionFindingsByFile(sessionID, filePath)
	} else {
		findings, err = store.GetSessionFindings(sessionID, agent)
	}
	if err != nil {
		return "", fmt.Errorf("query: %w", err)
	}

	// Apply finding_type filter in memory (not a store-level filter).
	if findingType != "" {
		filtered := findings[:0]
		for _, f := range findings {
			if strings.EqualFold(f.FindingType, findingType) {
				filtered = append(filtered, f)
			}
		}
		findings = filtered
	}

	if len(findings) == 0 {
		return "No session findings reported yet.", nil
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Session Findings (%d)\n", len(findings))
	sb.WriteString(strings.Repeat("─", 30))
	sb.WriteString("\n")

	for _, f := range findings {
		fmt.Fprintf(&sb, "\n[%s] %s — %s", f.Severity, f.Agent, f.FindingType)
		if f.FilePath != "" {
			fmt.Fprintf(&sb, " (%s)", f.FilePath)
		}
		fmt.Fprintf(&sb, "\n  %s\n", f.Description)
	}

	return strings.TrimRight(sb.String(), "\n"), nil
}
