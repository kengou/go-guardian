package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
)

// RunReportFinding records a cross-agent session finding and returns a
// human-readable acknowledgment. sessionID must be non-empty; callers of the
// CLI variant will read it from .go-guardian/session-id. severity is
// normalized to CRITICAL/HIGH/MEDIUM/LOW; unknown or empty values fall back
// to MEDIUM.
func RunReportFinding(store *db.Store, sessionID, agent, findingType, filePath, description, severity string) (string, error) {
	if sessionID == "" {
		return "", fmt.Errorf("no active session — findings require a session ID")
	}

	agent = strings.TrimSpace(agent)
	findingType = strings.TrimSpace(findingType)
	description = strings.TrimSpace(description)

	if agent == "" || findingType == "" || description == "" {
		return "", fmt.Errorf("agent, finding_type, and description are required")
	}

	filePath = strings.TrimSpace(filePath)
	severity = strings.ToUpper(strings.TrimSpace(severity))
	if severity == "" {
		severity = "MEDIUM"
	}
	switch severity {
	case "CRITICAL", "HIGH", "MEDIUM", "LOW":
	default:
		severity = "MEDIUM"
	}

	id, err := store.InsertSessionFinding(sessionID, agent, findingType, filePath, description, severity)
	if err != nil {
		return "", fmt.Errorf("store: %w", err)
	}

	return fmt.Sprintf("Finding #%d recorded (%s/%s: %s)", id, agent, severity, findingType), nil
}

// RegisterReportFinding registers the report_finding MCP tool.
// sessionID is captured in the closure so agents don't need to pass it.
func RegisterReportFinding(s ToolRegistrar, store *db.Store, sessionID string) {
	tool := mcp.NewTool(
		"report_finding",
		mcp.WithDescription(
			"Report a finding for cross-agent visibility within the current scan session. "+
				"Other agents can query these findings to focus their analysis on flagged areas.",
		),
		mcp.WithString("agent",
			mcp.Required(),
			mcp.Description("Agent reporting the finding (e.g. reviewer, security, linter, tester)"),
		),
		mcp.WithString("finding_type",
			mcp.Required(),
			mcp.Description("Type of finding (e.g. race-condition, sql-injection, error-handling, missing-test)"),
		),
		mcp.WithString("file_path",
			mcp.Description("File path where the finding was detected (optional)"),
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Human-readable description of the finding"),
		),
		mcp.WithString("severity",
			mcp.Description("Severity: CRITICAL, HIGH, MEDIUM, LOW (default: MEDIUM)"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		text, err := RunReportFinding(
			store,
			sessionID,
			req.GetString("agent", ""),
			req.GetString("finding_type", ""),
			req.GetString("file_path", ""),
			req.GetString("description", ""),
			req.GetString("severity", ""),
		)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("report_finding: %v", err)), nil
		}
		return mcp.NewToolResultText(text), nil
	})
}
