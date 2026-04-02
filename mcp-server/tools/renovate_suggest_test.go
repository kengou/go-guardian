package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// --- renovateCategoryFromProblem tests ---

func TestRenovateCategoryFromProblem(t *testing.T) {
	cases := []struct {
		problem      string
		wantCategory string
	}{
		{"too many PRs flooding my repo", "scheduling"},
		{"How to limit PR creation", "scheduling"},
		{"I want to automerge patches", "automerge"},
		{"enable auto merge for minor", "automerge"},
		{"auto-merge dev dependencies", "automerge"},
		{"group monorepo packages together", "grouping"},
		{"keep packages together in one PR", "grouping"},
		{"security vulnerability found", "security"},
		{"CVE in dependencies", "security"},
		{"custom datasource for Wiz", "custom_datasources"},
		{"binary artifact tracking", "custom_datasources"},
		{"GitHub Actions pinning", "automation"},
		{"Dockerfile digest tracking", "automation"},
		{"pin Makefile versions", "automation"},
		{"something completely unrelated", ""},
	}

	for _, tc := range cases {
		t.Run(tc.problem, func(t *testing.T) {
			got := renovateCategoryFromProblem(tc.problem)
			if got != tc.wantCategory {
				t.Errorf("renovateCategoryFromProblem(%q) = %q, want %q", tc.problem, got, tc.wantCategory)
			}
		})
	}
}

// --- suggest tool integration tests ---

func TestSuggestReturnsRelevantRules(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleSuggestRenovateRule(store)

	// The store is seeded with scheduling rules including SCH-1 (prConcurrentLimit).
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"problem": "too many PRs",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	// Should contain scheduling-related rules.
	if !strings.Contains(text, "SCH-") {
		t.Errorf("expected scheduling rules (SCH-*) in output:\n%s", text)
	}
	if !strings.Contains(text, "Suggestions for:") {
		t.Errorf("expected header in output:\n%s", text)
	}
	if !strings.Contains(text, "matching rules found") {
		t.Errorf("expected summary line in output:\n%s", text)
	}
}

func TestSuggestAutomergeCategory(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleSuggestRenovateRule(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"problem": "how to automerge patches",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "AM-") {
		t.Errorf("expected automerge rules (AM-*) in output:\n%s", text)
	}
	if !strings.Contains(text, "DON'T:") || !strings.Contains(text, "DO:") {
		t.Errorf("expected DON'T/DO examples in output:\n%s", text)
	}
}

func TestSuggestEmptyProblem(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleSuggestRenovateRule(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"problem": "",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for empty problem")
	}
}

func TestSuggestNoMatches(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleSuggestRenovateRule(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"problem": "xyzzy_nonexistent_keyword_12345",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)
	if !strings.Contains(text, "No matching rules found") {
		t.Errorf("expected 'No matching rules found' in output:\n%s", text)
	}
}

func TestSuggestIncludesPreferences(t *testing.T) {
	store := newRenovateTestStore(t)

	// Insert a learned preference for scheduling.
	if err := store.InsertRenovatePreference("scheduling", "Always limit to 5 PRs per hour", "", `{"prConcurrentLimit": 5}`); err != nil {
		t.Fatalf("InsertRenovatePreference: %v", err)
	}

	handler := handleSuggestRenovateRule(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"problem": "too many PRs flooding CI",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "from learned preferences") {
		t.Errorf("expected learned preferences count in output:\n%s", text)
	}
}

func TestSuggestLimitsToThree(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleSuggestRenovateRule(store)

	// Query for scheduling which has 6 seeded rules — should only show 3.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"problem": "too many PRs flooding",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	// Count numbered entries (lines starting with "1.", "2.", "3.").
	count := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if len(trimmed) > 2 && trimmed[0] >= '1' && trimmed[0] <= '9' && trimmed[1] == '.' {
			count++
		}
	}
	if count > 3 {
		t.Errorf("expected at most 3 suggestions, got %d:\n%s", count, text)
	}
}
