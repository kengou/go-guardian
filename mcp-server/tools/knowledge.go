package tools

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// RegisterQueryKnowledge registers the query_knowledge MCP tool on s.
// The tool surfaces learned lint patterns, anti-patterns, OWASP findings,
// and session findings relevant to the file and code context provided by the caller.
// If sessionID is non-empty, session findings for the file are also included.
func RegisterQueryKnowledge(s *server.MCPServer, store *db.Store, sessionID string) {
	tool := mcp.NewTool("query_knowledge",
		mcp.WithDescription("Return learned Go patterns, anti-patterns, and OWASP findings relevant to the file being written."),
		mcp.WithString("file_path",
			mcp.Description("Path to the Go file being written or edited (optional)."),
		),
		mcp.WithString("code_context",
			mcp.Description("Snippet of the code about to be written — first 1000 chars is sufficient (optional)."),
		),
		mcp.WithString("project",
			mcp.Description("Project identifier (optional)."),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		result, err := handleQueryKnowledge(ctx, req, store, sessionID)
		if err != nil {
			return mcp.NewToolResultError(err.Error()), nil
		}
		return mcp.NewToolResultText(result), nil
	})
}

// handleQueryKnowledge contains the core logic, separated for testability.
func handleQueryKnowledge(_ context.Context, req mcp.CallToolRequest, store *db.Store, sessionID string) (string, error) {
	filePath := req.GetString("file_path", "")
	codeContext := req.GetString("code_context", "")

	// Truncate code_context to 1000 chars to keep queries lean.
	if len(codeContext) > 1000 {
		codeContext = codeContext[:1000]
	}

	// 1. Determine the file glob from the file path.
	glob := fileGlobFor(filepath.Base(filePath))

	// 2. Query lint patterns (limit 10, sorted by frequency via the store).
	lintPatterns, err := store.QueryPatterns(glob, codeContext, 10)
	if err != nil {
		return "", fmt.Errorf("querying lint patterns: %w", err)
	}

	// 3. Determine anti-pattern category from code_context keywords, then query.
	category := categoryFromContext(codeContext)
	antiPatterns, err := store.QueryAntiPatterns(category)
	if err != nil {
		return "", fmt.Errorf("querying anti-patterns: %w", err)
	}

	// 4. Query OWASP findings matching the file glob (limit 3).
	owaspFindings, err := store.QueryOWASPFindings(glob, 3)
	if err != nil {
		return "", fmt.Errorf("querying OWASP findings: %w", err)
	}

	// 5. Query session findings for this file (if session is active).
	var sessionFindings []db.SessionFinding
	if sessionID != "" && filePath != "" {
		sessionFindings, err = store.GetSessionFindingsByFile(sessionID, filePath)
		if err != nil {
			return "", fmt.Errorf("querying session findings: %w", err)
		}
	}

	// 6. Check whether anything was found at all.
	if len(lintPatterns) == 0 && len(antiPatterns) == 0 && len(owaspFindings) == 0 && len(sessionFindings) == 0 {
		return "No learned patterns for this context yet.", nil
	}

	// 7. Format and return the context block.
	return formatKnowledge(lintPatterns, antiPatterns, owaspFindings, sessionFindings), nil
}

// categoryFromContext maps keywords in the code snippet to an anti-pattern category.
func categoryFromContext(code string) string {
	lower := strings.ToLower(code)
	switch {
	case strings.Contains(lower, "goroutine") ||
		strings.Contains(code, "go func") ||
		strings.Contains(lower, "chan") ||
		strings.Contains(lower, "sync"):
		return "concurrency"
	case strings.Contains(lower, "err") ||
		strings.Contains(code, "Errorf") ||
		strings.Contains(lower, "errors"):
		return "error-handling"
	case strings.Contains(lower, "test") ||
		strings.Contains(lower, "testing"):
		return "testing"
	case strings.Contains(lower, "interface") ||
		strings.Contains(code, "func("):
		return "design"
	default:
		return "general"
	}
}

// firstLine returns the first non-empty line of s, trimmed of whitespace.
// If s is empty the empty string is returned.
func firstLine(s string) string {
	for _, line := range strings.SplitN(s, "\n", -1) {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(s)
}

// formatKnowledge assembles the injectable context block from the result
// slices. It caps lint patterns at 5, anti-patterns at 3, and session findings at 5.
func formatKnowledge(lintPatterns []db.LintPattern, antiPatterns []db.AntiPattern, owaspFindings []db.OWASPFinding, sessionFindings ...[]db.SessionFinding) string {
	var b strings.Builder
	b.WriteString("LEARNED PATTERNS FOR THIS CONTEXT:\n")

	// Lint patterns — up to 5.
	if len(lintPatterns) > 0 {
		cap5 := lintPatterns
		if len(cap5) > 5 {
			cap5 = cap5[:5]
		}
		for _, p := range cap5 {
			fmt.Fprintf(&b, "• [lint:%s ×%d] %s\n", p.Rule, p.Frequency, firstLine(p.DontCode))
			fmt.Fprintf(&b, "  → DO: %s\n", firstLine(p.DoCode))
		}
	}

	// Anti-patterns — up to 3.
	if len(antiPatterns) > 0 {
		cap3 := antiPatterns
		if len(cap3) > 3 {
			cap3 = cap3[:3]
		}
		for _, ap := range cap3 {
			fmt.Fprintf(&b, "• [pattern:%s] %s\n", ap.PatternID, ap.Description)
			fmt.Fprintf(&b, "  → DON'T: %s\n", firstLine(ap.DontCode))
			fmt.Fprintf(&b, "  → DO: %s\n", firstLine(ap.DoCode))
		}
	}

	// OWASP findings — up to 3 (already limited by the query).
	if len(owaspFindings) > 0 {
		for _, f := range owaspFindings {
			fmt.Fprintf(&b, "• [owasp:%s] %s\n", f.Category, f.Finding)
			fmt.Fprintf(&b, "  → FIX: %s\n", f.FixPattern)
		}
	}

	// Session findings — up to 5 (from other agents in the current session).
	if len(sessionFindings) > 0 && len(sessionFindings[0]) > 0 {
		sf := sessionFindings[0]
		if len(sf) > 5 {
			sf = sf[:5]
		}
		b.WriteString("\nSESSION FINDINGS (from other agents):\n")
		for _, f := range sf {
			fmt.Fprintf(&b, "• [%s:%s] %s", f.Severity, f.Agent, f.FindingType)
			if f.FilePath != "" {
				fmt.Fprintf(&b, " (%s)", f.FilePath)
			}
			fmt.Fprintf(&b, "\n  %s\n", f.Description)
		}
	}

	return strings.TrimRight(b.String(), "\n")
}
