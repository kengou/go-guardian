package tools

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterLearnFromReview registers the learn_from_review MCP tool with the
// given server. The tool stores review findings as reusable patterns so they
// surface in future query_knowledge and suggest_fix calls.
func RegisterLearnFromReview(s ToolRegistrar, store *db.Store) {
	tool := mcp.NewTool(
		"learn_from_review",
		mcp.WithDescription(
			"Record a code review finding and its fix as a reusable pattern. "+
				"The finding is stored so that future reviews and prevention hooks "+
				"can surface the same guidance. For HIGH/CRITICAL findings, an "+
				"anti-pattern entry is also created.",
		),
		mcp.WithString("description",
			mcp.Required(),
			mcp.Description("Short description of the finding"),
		),
		mcp.WithString("severity",
			mcp.Required(),
			mcp.Description("Finding severity: CRITICAL, HIGH, MEDIUM, or LOW"),
		),
		mcp.WithString("category",
			mcp.Required(),
			mcp.Description("Pattern category: concurrency, error-handling, testing, design, security, general, mesh-proxy, observability, operator, policy, gitops, api-design"),
		),
		mcp.WithString("dont_code",
			mcp.Required(),
			mcp.Description("The flagged code snippet (the bad pattern)"),
		),
		mcp.WithString("do_code",
			mcp.Required(),
			mcp.Description("The fix code snippet (the correct pattern)"),
		),
		mcp.WithString("file_path",
			mcp.Description("Path to the file where the finding was detected (optional, used to derive file_glob)"),
		),
	)
	s.AddTool(tool, learnFromReviewHandler(store))
}

// learnFromReviewHandler returns the ToolHandlerFunc for learn_from_review.
// It is separated for testability.
func learnFromReviewHandler(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		description := strings.TrimSpace(req.GetString("description", ""))
		severity := strings.TrimSpace(strings.ToUpper(req.GetString("severity", "")))
		category := strings.TrimSpace(strings.ToLower(req.GetString("category", "")))
		dontCode := strings.TrimSpace(req.GetString("dont_code", ""))
		doCode := strings.TrimSpace(req.GetString("do_code", ""))
		filePath := strings.TrimSpace(req.GetString("file_path", ""))

		// Validate required params.
		if description == "" {
			return mcp.NewToolResultError("description is required"), nil
		}
		if severity == "" {
			return mcp.NewToolResultError("severity is required"), nil
		}
		if category == "" {
			return mcp.NewToolResultError("category is required"), nil
		}
		if dontCode == "" {
			return mcp.NewToolResultError("dont_code is required"), nil
		}
		if doCode == "" {
			return mcp.NewToolResultError("do_code is required"), nil
		}

		// Validate severity.
		switch severity {
		case "CRITICAL", "HIGH", "MEDIUM", "LOW":
			// ok
		default:
			return mcp.NewToolResultError(fmt.Sprintf("invalid severity %q: must be CRITICAL, HIGH, MEDIUM, or LOW", severity)), nil
		}

		// Derive file_glob from file_path.
		glob := "*.go"
		if filePath != "" {
			glob = fileGlobFor(filepath.Base(filePath))
		}

		// Truncate snippets to 500 chars (reuses trimSnippet from learn.go —
		// same package, so directly accessible).
		dontCode = trimSnippet(dontCode, 500)
		doCode = trimSnippet(doCode, 500)

		// Build rule as "review:<category>".
		rule := "review:" + category

		// Store as lint pattern with source="review".
		if err := store.InsertLintPattern(rule, glob, dontCode, doCode, "review"); err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("store lint pattern: %v", err)), nil
		}

		// For HIGH/CRITICAL findings, also create an anti-pattern entry.
		alsoAntiPattern := false
		if severity == "CRITICAL" || severity == "HIGH" {
			patternID := reviewPatternID(dontCode)
			err := store.InsertAntiPattern(patternID, description, dontCode, doCode, "review", category)
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("store anti-pattern: %v", err)), nil
			}
			alsoAntiPattern = true
		}

		result := map[string]interface{}{
			"stored":            true,
			"rule":              rule,
			"file_glob":         glob,
			"also_anti_pattern": alsoAntiPattern,
		}
		b, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("json encode: %v", err)), nil
		}
		return mcp.NewToolResultText(string(b)), nil
	}
}

// reviewPatternID generates a deterministic pattern_id from the dont_code
// snippet: "REV-" followed by the first 8 hex characters of SHA-256.
func reviewPatternID(dontCode string) string {
	h := sha256.Sum256([]byte(dontCode))
	return fmt.Sprintf("REV-%x", h[:4])
}
