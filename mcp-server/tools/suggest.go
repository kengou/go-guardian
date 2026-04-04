package tools

import (
	"context"
	"fmt"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
)

const (
	// maxSuggestMatches is the maximum number of matches returned.
	maxSuggestMatches = 3
	// highConfidenceThreshold is the minimum similarity score for "high" confidence.
	highConfidenceThreshold = 0.6
	// mediumConfidenceThreshold is the minimum similarity score for "medium" confidence.
	mediumConfidenceThreshold = 0.3
	// earlyTerminationThreshold is the score above which a match counts as
	// high-confidence for early termination purposes.
	earlyTerminationThreshold = 0.8
)

// suggestMatch holds a matched pattern and its similarity score.
type suggestMatch struct {
	rule       string // display label, e.g. "lint:errcheck" or "pattern:AP-3"
	dontCode   string
	doCode     string
	frequency  int64
	similarity float64
}

// RegisterSuggestFix registers the suggest_fix MCP tool with the given server.
func RegisterSuggestFix(s ToolRegistrar, store *db.Store) {
	tool := mcp.NewTool(
		"suggest_fix",
		mcp.WithDescription(
			"Search the go-guardian knowledge base for patterns that match a "+
				"problematic code snippet and return up to 3 suggested fixes. "+
				"Optionally filter by issue type: lint, owasp, pattern, or leave empty for all.",
		),
		mcp.WithString("code_snippet",
			mcp.Required(),
			mcp.Description("The problematic code snippet to find fixes for"),
		),
		mcp.WithString("issue_type",
			mcp.Description("Optional hint: \"lint\", \"owasp\", \"pattern\", or empty for all"),
		),
	)

	s.AddTool(tool, func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		snippet := req.GetString("code_snippet", "")
		issueType := req.GetString("issue_type", "")

		if strings.TrimSpace(snippet) == "" {
			return mcp.NewToolResultText(
				"No matching patterns found. If this is a recurring issue, it will be learned automatically when golangci-lint runs.",
			), nil
		}

		matches, err := findMatches(store, snippet, issueType)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("knowledge base query error: %v", err)), nil
		}

		if len(matches) == 0 {
			return mcp.NewToolResultText(
				"No matching patterns found. If this is a recurring issue, it will be learned automatically when golangci-lint runs.",
			), nil
		}

		return mcp.NewToolResultText(formatSuggestMatches(matches)), nil
	})
}

// findMatches queries the DB for lint patterns and anti-patterns that are
// similar to snippet. issueType filters to "lint", "owasp", "pattern", or all.
func findMatches(store *db.Store, snippet, issueType string) ([]suggestMatch, error) {
	var matches []suggestMatch
	highCount := 0

	issueType = strings.ToLower(strings.TrimSpace(issueType))

	// Pre-lowercase the snippet once for all similarity comparisons.
	snippetLower := strings.ToLower(snippet)

	// Search lint_patterns (unless caller restricted to "owasp" or "pattern").
	if issueType == "" || issueType == "lint" {
		lintPatterns, err := store.QueryPatterns("*.go", "", 200)
		if err != nil {
			return nil, fmt.Errorf("query lint patterns: %w", err)
		}
		for _, p := range lintPatterns {
			if p.DontCode == "" {
				continue
			}
			sim := snippetSimilarityPreLowered(snippetLower, p.DontCode)
			if sim > 0 {
				matches = append(matches, suggestMatch{
					rule:       fmt.Sprintf("lint:%s", p.Rule),
					dontCode:   p.DontCode,
					doCode:     p.DoCode,
					frequency:  p.Frequency,
					similarity: sim,
				})
				if sim > earlyTerminationThreshold {
					highCount++
					if highCount >= maxSuggestMatches {
						break
					}
				}
			}
		}
	}

	// Search anti_patterns (unless caller restricted to "lint" or "owasp").
	if (issueType == "" || issueType == "pattern") && highCount < maxSuggestMatches {
		antiPatterns, err := store.QueryAntiPatterns("")
		if err != nil {
			return nil, fmt.Errorf("query anti patterns: %w", err)
		}
		for _, ap := range antiPatterns {
			if ap.DontCode == "" {
				continue
			}
			sim := snippetSimilarityPreLowered(snippetLower, ap.DontCode)
			if sim > 0 {
				matches = append(matches, suggestMatch{
					rule:       fmt.Sprintf("pattern:%s", ap.PatternID),
					dontCode:   ap.DontCode,
					doCode:     ap.DoCode,
					frequency:  0, // anti-patterns don't have a frequency field
					similarity: sim,
				})
				if sim > earlyTerminationThreshold {
					highCount++
					if highCount >= maxSuggestMatches {
						break
					}
				}
			}
		}
	}

	// OWASP findings have fix_pattern but not structured dont_code/do_code pairs.
	// We skip them for snippet similarity (no dont_code to compare) unless the
	// caller explicitly asked for owasp, in which case we return a notice.
	// (The OWASP data model stores findings at the category level, not code level.)

	// Sort by similarity descending, then frequency descending as tiebreaker.
	sortMatches(matches)

	if len(matches) > maxSuggestMatches {
		matches = matches[:maxSuggestMatches]
	}
	return matches, nil
}

// sortMatches sorts matches by similarity descending, breaking ties by
// frequency descending.
func sortMatches(matches []suggestMatch) {
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0; j-- {
			a, b := matches[j-1], matches[j]
			if a.similarity < b.similarity ||
				(a.similarity == b.similarity && a.frequency < b.frequency) {
				matches[j-1], matches[j] = matches[j], matches[j-1]
			}
		}
	}
}

// confidenceLabel maps a similarity score to a human-readable label.
func confidenceLabel(sim float64) string {
	switch {
	case sim >= highConfidenceThreshold:
		return "high"
	case sim >= mediumConfidenceThreshold:
		return "medium"
	default:
		return "low"
	}
}

// formatSuggestMatches renders matches as a human-readable fix suggestion.
func formatSuggestMatches(matches []suggestMatch) string {
	var sb strings.Builder
	sb.WriteString("Suggested fixes for your code:\n")

	for i, m := range matches {
		sb.WriteString("\n")
		label := m.rule
		if m.frequency > 0 {
			label = fmt.Sprintf("%s ×%d", m.rule, m.frequency)
		}
		fmt.Fprintf(&sb, "Match %d — [%s] (confidence: %s)\n",
			i+1, label, confidenceLabel(m.similarity))

		if m.dontCode != "" {
			sb.WriteString("DON'T:\n")
			for _, line := range strings.Split(m.dontCode, "\n") {
				fmt.Fprintf(&sb, "  %s\n", line)
			}
		}

		if m.doCode != "" {
			sb.WriteString("\nDO:\n")
			for _, line := range strings.Split(m.doCode, "\n") {
				fmt.Fprintf(&sb, "  %s\n", line)
			}
		}
	}

	return strings.TrimRight(sb.String(), "\n")
}

// snippetSimilarity returns a score in [0.0, 1.0] representing how many
// non-trivial lines of dontCode appear (as substrings, case-insensitive) in
// snippet. The score is the fraction of dontCode lines matched.
//
// A "non-trivial" line is one that, after trimming whitespace, has at least
// 4 characters and is not a standalone comment or punctuation-only token.
func snippetSimilarity(snippet, dontCode string) float64 {
	if snippet == "" || dontCode == "" {
		return 0.0
	}
	return snippetSimilarityPreLowered(strings.ToLower(snippet), dontCode)
}

// snippetSimilarityPreLowered is like snippetSimilarity but accepts a
// pre-lowercased snippet to avoid redundant ToLower calls in hot loops.
func snippetSimilarityPreLowered(snippetLower, dontCode string) float64 {
	if snippetLower == "" || dontCode == "" {
		return 0.0
	}

	var total, matched int
	for _, line := range strings.Split(dontCode, "\n") {
		trimmed := strings.TrimSpace(line)
		if !isNonTrivialLine(trimmed) {
			continue
		}
		total++
		if strings.Contains(snippetLower, strings.ToLower(trimmed)) {
			matched++
		}
	}

	if total == 0 {
		return 0.0
	}
	return float64(matched) / float64(total)
}

// isNonTrivialLine returns true when a trimmed line carries enough content to
// be a useful signal: at least 4 characters, not a pure comment, not
// punctuation-only (e.g. "{", "}", "//").
func isNonTrivialLine(trimmed string) bool {
	if len(trimmed) < 4 {
		return false
	}
	if strings.HasPrefix(trimmed, "//") {
		return false
	}
	// Reject lines that are only braces, brackets, or semicolons.
	onlyPunct := true
	for _, r := range trimmed {
		if r != '{' && r != '}' && r != '(' && r != ')' && r != '[' && r != ']' && r != ';' && r != ',' {
			onlyPunct = false
			break
		}
	}
	return !onlyPunct
}
