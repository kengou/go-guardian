package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
)

// lintFinding holds one parsed line from golangci-lint output.
type lintFinding struct {
	file    string // basename of the file
	rule    string // linter/rule name
	message string // human-readable message
}

// diffHunk holds the extracted before/after code from one hunk of a unified diff.
type diffHunk struct {
	file     string // basename of the changed file
	dontCode string // removed lines (the "bad" code)
	doCode   string // added lines (the "good" code)
}

// lintPattern is a fully resolved pattern ready to be stored.
type lintPattern struct {
	rule     string
	fileGlob string
	dontCode string
	doCode   string
}

// lintLineRe matches golangci-lint output lines of the forms:
//
//	path/to/file.go:12:34: rule-name message (linter-name)
//	path/to/file.go:12:34: message (linter-name)
var lintLineRe = regexp.MustCompile(
	`^([^\s:][^:]*\.go):\d+:\d+:\s+(.+?)\s+\(([^)]+)\)\s*$`,
)

// RegisterLearnFromLint registers the learn_from_lint MCP tool with the given server.
func RegisterLearnFromLint(s *server.MCPServer, store *db.Store) {
	tool := mcp.NewTool(
		"learn_from_lint",
		mcp.WithDescription(
			"Record a golangci-lint finding and its fix as a reusable pattern. "+
				"Provide the unified diff of the fix and the raw lint output; "+
				"the tool extracts rule names, before/after code snippets, and "+
				"stores them so future reviews can surface the same guidance.",
		),
		mcp.WithString("diff",
			mcp.Required(),
			mcp.Description("Unified diff of the fix in git diff format"),
		),
		mcp.WithString("lint_output",
			mcp.Required(),
			mcp.Description("Raw golangci-lint output captured before the fix was applied"),
		),
		mcp.WithString("project",
			mcp.Description("Optional project identifier for scoping (informational only)"),
		),
	)
	s.AddTool(tool, learnFromLintHandler(store))
}

// learnFromLintHandler returns the ToolHandlerFunc for learn_from_lint.
// It is a separate function so that tests can call the handler directly
// without needing a live MCP transport.
func learnFromLintHandler(store *db.Store) server.ToolHandlerFunc {
	return func(ctx context.Context, req mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		diff := req.GetString("diff", "")
		lintOutput := req.GetString("lint_output", "")

		// Edge case: nothing useful provided.
		if strings.TrimSpace(diff) == "" && strings.TrimSpace(lintOutput) == "" {
			return mcp.NewToolResultText(`{"learned":0,"updated":0,"patterns":[]}`), nil
		}

		findings := parseLintOutput(lintOutput)
		hunks := parseDiff(diff)
		patterns := matchFindingsToHunks(findings, hunks)

		type patternSummary struct {
			Rule     string `json:"rule"`
			FileGlob string `json:"file_glob"`
		}

		learned := 0
		updated := 0
		summaries := make([]patternSummary, 0, len(patterns))

		for _, p := range patterns {
			err := store.InsertLintPattern(p.rule, p.fileGlob, p.dontCode, p.doCode, "learned")
			if err != nil {
				return mcp.NewToolResultError(fmt.Sprintf("store error: %v", err)), nil
			}
			// InsertLintPattern uses ON CONFLICT … DO UPDATE, so every call
			// either inserts (learned) or increments frequency (updated).
			// We approximate: if both code snippets are non-empty it is a
			// full pattern (learned), otherwise a signal-only pattern (updated).
			if p.dontCode == "" && p.doCode == "" {
				updated++
			} else {
				learned++
			}
			summaries = append(summaries, patternSummary{Rule: p.rule, FileGlob: p.fileGlob})
		}

		result := map[string]interface{}{
			"learned":  learned,
			"updated":  updated,
			"patterns": summaries,
		}
		b, err := json.Marshal(result)
		if err != nil {
			return mcp.NewToolResultError(fmt.Sprintf("json encode: %v", err)), nil
		}
		return mcp.NewToolResultText(string(b)), nil
	}
}

// parseLintOutput parses raw golangci-lint output and returns one lintFinding
// per recognised diagnostic line.  Unrecognised lines (build errors, blank
// lines, summary lines) are silently skipped.
func parseLintOutput(output string) []lintFinding {
	var findings []lintFinding
	seen := make(map[string]bool) // deduplicate rule+file combos

	for _, line := range strings.Split(output, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		m := lintLineRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		// m[1] = file path, m[2] = message body, m[3] = linter name
		filePath := m[1]
		body := strings.TrimSpace(m[2])
		linter := strings.TrimSpace(m[3])

		// The body may start with "rule-name: remainder" or just be a plain message.
		// We try to extract a more specific rule name from the body prefix.
		rule := linter
		if idx := strings.Index(body, ":"); idx > 0 {
			candidate := strings.TrimSpace(body[:idx])
			// Treat it as a rule name only if it looks like one (no spaces, reasonable length).
			if len(candidate) <= 60 && !strings.ContainsAny(candidate, " \t()") {
				rule = candidate
			}
		}

		base := filepath.Base(filePath)
		key := rule + "|" + base
		if seen[key] {
			continue
		}
		seen[key] = true

		findings = append(findings, lintFinding{
			file:    base,
			rule:    rule,
			message: body,
		})
	}
	return findings
}

// parseDiff parses a unified diff and returns one diffHunk per changed file.
// Lines beginning with `-` (but not `---`) are the removed ("don't") code;
// lines beginning with `+` (but not `+++`) are the added ("do") code.
// Blank lines and comment-only lines are excluded.  Each snippet is capped at
// 500 characters.
func parseDiff(diff string) []diffHunk {
	const maxSnippet = 500

	var hunks []diffHunk
	var current *diffHunk
	var dontBuf, doBuf strings.Builder

	flushCurrent := func() {
		if current == nil {
			return
		}
		current.dontCode = trimSnippet(dontBuf.String(), maxSnippet)
		current.doCode = trimSnippet(doBuf.String(), maxSnippet)
		hunks = append(hunks, *current)
		current = nil
		dontBuf.Reset()
		doBuf.Reset()
	}

	for _, line := range strings.Split(diff, "\n") {
		switch {
		case strings.HasPrefix(line, "diff --git") || strings.HasPrefix(line, "diff -"):
			flushCurrent()

		case strings.HasPrefix(line, "--- "):
			// New file section — extract file name.
			flushCurrent()
			// "--- a/path/to/file.go" or "--- /dev/null"
			rest := strings.TrimPrefix(line, "--- ")
			rest = strings.TrimPrefix(rest, "a/")
			rest = strings.TrimPrefix(rest, "b/")
			if rest == "/dev/null" {
				rest = ""
			}
			base := filepath.Base(rest)
			if !strings.HasSuffix(base, ".go") && base != "" {
				// Not a Go file — skip this hunk.
				current = nil
				continue
			}
			current = &diffHunk{file: base}
			dontBuf.Reset()
			doBuf.Reset()

		case strings.HasPrefix(line, "+++ "):
			// Skip — file name already captured from "---" line.

		case current == nil:
			// No active hunk yet; skip context/header lines.

		case strings.HasPrefix(line, "-"):
			code := line[1:] // strip leading '-'
			if isUsefulCodeLine(code) {
				if dontBuf.Len() > 0 {
					dontBuf.WriteByte('\n')
				}
				dontBuf.WriteString(strings.TrimRight(code, " \t"))
			}

		case strings.HasPrefix(line, "+"):
			code := line[1:] // strip leading '+'
			if isUsefulCodeLine(code) {
				if doBuf.Len() > 0 {
					doBuf.WriteByte('\n')
				}
				doBuf.WriteString(strings.TrimRight(code, " \t"))
			}
		}
	}
	flushCurrent()
	return hunks
}

// isUsefulCodeLine returns true when the line contains something other than
// whitespace or a standalone comment.
func isUsefulCodeLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" {
		return false
	}
	// Pure comment lines carry no structural information.
	if strings.HasPrefix(trimmed, "//") {
		return false
	}
	return true
}

// trimSnippet truncates s to at most maxLen bytes, appending "…" if truncated.
func trimSnippet(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// matchFindingsToHunks pairs each lint finding with the diff hunk for the same
// file (by basename).  If no matching hunk exists the finding is stored as a
// signal pattern with empty code snippets.
func matchFindingsToHunks(findings []lintFinding, hunks []diffHunk) []lintPattern {
	// Build a map from basename → first hunk for that file.
	hunkByFile := make(map[string]diffHunk, len(hunks))
	for _, h := range hunks {
		if _, exists := hunkByFile[h.file]; !exists {
			hunkByFile[h.file] = h
		}
	}

	var patterns []lintPattern
	for _, f := range findings {
		hunk, matched := hunkByFile[f.file]
		glob := fileGlobFor(f.file)

		if matched {
			patterns = append(patterns, lintPattern{
				rule:     f.rule,
				fileGlob: glob,
				dontCode: hunk.dontCode,
				doCode:   hunk.doCode,
			})
		} else {
			// No matching diff: store as a signal pattern (empty code).
			patterns = append(patterns, lintPattern{
				rule:     f.rule,
				fileGlob: glob,
				dontCode: "",
				doCode:   "",
			})
		}
	}

	// If there were hunks but no lint findings, store the diff hunks alone
	// with a synthetic "diff-only" rule so the code examples are not lost.
	if len(findings) == 0 {
		for _, h := range hunks {
			if h.dontCode == "" && h.doCode == "" {
				continue
			}
			patterns = append(patterns, lintPattern{
				rule:     "diff-only",
				fileGlob: fileGlobFor(h.file),
				dontCode: h.dontCode,
				doCode:   h.doCode,
			})
		}
	}

	return patterns
}

// fileGlobFor derives a file glob from a Go source basename.
// Domain-specific suffixes get a targeted glob; generic files get `*.go`.
func fileGlobFor(base string) string {
	if base == "" {
		return "*.go"
	}
	// Strip the .go extension for suffix inspection.
	stem := strings.TrimSuffix(base, ".go")

	// Bare-word stems that map to a domain glob even without an underscore prefix.
	// "middleware" is intentionally absent — it's too generic as a bare word.
	bareWordGlobs := map[string]string{
		"handler":    "*_handler.go",
		"handlers":   "*_handler.go",
		"server":     "*_server.go",
		"client":     "*_client.go",
		"repo":       "*_repo.go",
		"repository": "*_repository.go",
		"service":    "*_service.go",
		"model":      "*_model.go",
		"controller": "*_controller.go",
	}
	if glob, ok := bareWordGlobs[stem]; ok {
		return glob
	}

	domainSuffixes := []string{
		"_handler", "_handlers",
		"_test",
		"_server", "_client",
		"_middleware",
		"_controller",
		"_repository", "_repo",
		"_service",
		"_model",
		"_mock",
		"_gen", "_generated",
	}
	for _, suffix := range domainSuffixes {
		if strings.HasSuffix(stem, suffix) {
			return "*" + suffix + ".go"
		}
	}
	return "*.go"
}
