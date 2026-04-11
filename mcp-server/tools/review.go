package tools

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/kengou/go-guardian/mcp-server/db"
)

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

// reviewPatternID generates a deterministic pattern_id from the dont_code
// snippet: "REV-" followed by the first 8 hex characters of SHA-256.
func reviewPatternID(dontCode string) string {
	h := sha256.Sum256([]byte(dontCode))
	return fmt.Sprintf("REV-%x", h[:4])
}
