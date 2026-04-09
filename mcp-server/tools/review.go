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

// RunLearnFromReview records a code review finding as a reusable pattern.
// For HIGH/CRITICAL severities an anti-pattern entry is also created.
// The returned string is a JSON blob matching the MCP handler output.
func RunLearnFromReview(store *db.Store, description, severity, category, dontCode, doCode, filePath string) (string, error) {
	description = strings.TrimSpace(description)
	severity = strings.TrimSpace(strings.ToUpper(severity))
	category = strings.TrimSpace(strings.ToLower(category))
	dontCode = strings.TrimSpace(dontCode)
	doCode = strings.TrimSpace(doCode)
	filePath = strings.TrimSpace(filePath)

	if description == "" {
		return "", fmt.Errorf("description is required")
	}
	if severity == "" {
		return "", fmt.Errorf("severity is required")
	}
	if category == "" {
		return "", fmt.Errorf("category is required")
	}
	if dontCode == "" {
		return "", fmt.Errorf("dont_code is required")
	}
	if doCode == "" {
		return "", fmt.Errorf("do_code is required")
	}

	switch severity {
	case "CRITICAL", "HIGH", "MEDIUM", "LOW":
	default:
		return "", fmt.Errorf("invalid severity %q: must be CRITICAL, HIGH, MEDIUM, or LOW", severity)
	}

	glob := "*.go"
	if filePath != "" {
		glob = fileGlobFor(filepath.Base(filePath))
	}

	dontCode = trimSnippet(dontCode, 500)
	doCode = trimSnippet(doCode, 500)

	rule := "review:" + category

	if err := store.InsertLintPattern(rule, glob, dontCode, doCode, "review"); err != nil {
		return "", fmt.Errorf("store lint pattern: %w", err)
	}

	alsoAntiPattern := false
	if severity == "CRITICAL" || severity == "HIGH" {
		patternID := reviewPatternID(dontCode)
		if err := store.InsertAntiPattern(patternID, description, dontCode, doCode, "review", category); err != nil {
			return "", fmt.Errorf("store anti-pattern: %w", err)
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
		return "", fmt.Errorf("json encode: %w", err)
	}
	return string(b), nil
}

// learnFromReviewHandler returns the ToolHandlerFunc for learn_from_review.
// It is separated for testability.
func learnFromReviewHandler(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := RunLearnFromReview(
			store,
			req.GetString("description", ""),
			req.GetString("severity", ""),
			req.GetString("category", ""),
			req.GetString("dont_code", ""),
			req.GetString("do_code", ""),
			req.GetString("file_path", ""),
		)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	}
}

// reviewPatternID generates a deterministic pattern_id from the dont_code
// snippet: "REV-" followed by the first 8 hex characters of SHA-256.
func reviewPatternID(dontCode string) string {
	h := sha256.Sum256([]byte(dontCode))
	return fmt.Sprintf("REV-%x", h[:4])
}
