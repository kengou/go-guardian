package admin

import (
	"context"
	"encoding/json"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// WrapToolHandler wraps a server.ToolHandlerFunc so that every invocation
// is logged to the mcp_requests table. The original handler's result and
// error are returned unchanged.
func WrapToolHandler(store *db.Store, toolName string, handler server.ToolHandlerFunc) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		start := time.Now()

		// Call the original handler.
		result, handlerErr := handler(ctx, req)

		durationMS := time.Since(start).Milliseconds()

		// Type-assert Arguments (which is any) to map[string]interface{}.
		args, _ := req.Params.Arguments.(map[string]interface{})

		// Extract agent from request params if available.
		agent := ""
		if args != nil {
			if v, ok := args["agent"]; ok {
				if s, ok := v.(string); ok {
					agent = s
				}
			}
		}

		// Build a truncated JSON summary of the params.
		paramsSummary := truncateParams(args, 200)

		// Capture error message if the handler returned an error.
		errMsg := ""
		if handlerErr != nil {
			errMsg = handlerErr.Error()
		}

		// Best-effort insert; don't let logging failures affect the tool.
		_ = store.InsertMCPRequest(toolName, agent, paramsSummary, durationMS, errMsg)

		return result, handlerErr
	}
}

// truncateParams JSON-encodes the arguments map and truncates to maxLen chars.
func truncateParams(args map[string]interface{}, maxLen int) string {
	if len(args) == 0 {
		return "{}"
	}
	b, err := json.Marshal(args)
	if err != nil {
		return "{}"
	}
	s := string(b)
	if len(s) > maxLen {
		s = s[:maxLen]
	}
	return s
}
