package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestRenovateQueryByCategory(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleRenovateQueryKnowledge(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"category": "automerge",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "=== Renovate Knowledge: automerge ===") {
		t.Errorf("expected header with category name:\n%s", text)
	}
	if !strings.Contains(text, "Rules (") {
		t.Errorf("expected Rules section:\n%s", text)
	}
	if !strings.Contains(text, "AM-") {
		t.Errorf("expected automerge rules (AM-*) in output:\n%s", text)
	}
	if !strings.Contains(text, "results") {
		t.Errorf("expected results summary line:\n%s", text)
	}
}

func TestRenovateQueryByKeyword(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleRenovateQueryKnowledge(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query": "automerge",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	// Should find rules that mention automerge in title or description.
	if !strings.Contains(text, "Rules (") {
		t.Errorf("expected Rules section:\n%s", text)
	}
	if !strings.Contains(text, "results") {
		t.Errorf("expected results summary:\n%s", text)
	}
}

func TestRenovateQueryEmptyResults(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleRenovateQueryKnowledge(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query": "xyzzy_nonexistent_keyword_99999",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "No results found") {
		t.Errorf("expected 'No results found' for nonsense query:\n%s", text)
	}
}

func TestRenovateQueryCategoryWithPreferences(t *testing.T) {
	store := newRenovateTestStore(t)

	// Insert a preference.
	if err := store.InsertRenovatePreference("scheduling", "Limit PRs to 5 per hour", "", ""); err != nil {
		t.Fatalf("InsertRenovatePreference: %v", err)
	}

	handler := handleRenovateQueryKnowledge(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"category": "scheduling",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "Rules (") {
		t.Errorf("expected Rules section:\n%s", text)
	}
	if !strings.Contains(text, "Learned Preferences (") {
		t.Errorf("expected Learned Preferences section:\n%s", text)
	}
	if !strings.Contains(text, "Limit PRs to 5 per hour") {
		t.Errorf("expected preference description in output:\n%s", text)
	}
}

func TestRenovateQueryAllNoCriteria(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleRenovateQueryKnowledge(store)

	// No category and no query returns everything (capped to 10).
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "=== Renovate Knowledge: all ===") {
		t.Errorf("expected 'all' label:\n%s", text)
	}
	if !strings.Contains(text, "Rules (") {
		t.Errorf("expected Rules section:\n%s", text)
	}
}

func TestRenovateQueryResultsCappedAtTen(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleRenovateQueryKnowledge(store)

	// Query all rules — the seed data has more than 10 total.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	// Count rule entries (lines matching "  [XXX-N] SEVERITY:").
	ruleCount := 0
	prefCount := 0
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "[") && strings.Contains(trimmed, "]") &&
			(strings.Contains(trimmed, "CRITICAL:") || strings.Contains(trimmed, "WARN:") || strings.Contains(trimmed, "INFO:")) {
			ruleCount++
		}
		if strings.HasPrefix(trimmed, "[freq:") {
			prefCount++
		}
	}

	total := ruleCount + prefCount
	if total > 10 {
		t.Errorf("expected at most 10 total results, got %d (rules=%d, prefs=%d):\n%s",
			total, ruleCount, prefCount, text)
	}
}

func TestRenovateQueryKeywordWithPreferences(t *testing.T) {
	store := newRenovateTestStore(t)

	// Insert a preference that matches the keyword.
	if err := store.InsertRenovatePreference("automerge", "Always use PR automerge type for visibility", "", ""); err != nil {
		t.Fatalf("InsertRenovatePreference: %v", err)
	}

	handler := handleRenovateQueryKnowledge(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"query": "automerge",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "Learned Preferences (") {
		t.Errorf("expected Learned Preferences section for keyword search:\n%s", text)
	}
	if !strings.Contains(text, "Always use PR automerge type") {
		t.Errorf("expected matching preference in output:\n%s", text)
	}
}

func TestRenovateQueryInvalidCategoryReturnsEmpty(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleRenovateQueryKnowledge(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"category": "nonexistent_category_xyz",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)

	if !strings.Contains(text, "No results found") {
		t.Errorf("expected 'No results found' for invalid category:\n%s", text)
	}
}
