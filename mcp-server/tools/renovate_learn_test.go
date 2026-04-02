package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestLearnRenovatePreferenceInsert(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleLearnRenovatePreference(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"category":    "automerge",
		"description": "Always use PR automerge type for visibility",
		"dont_config": `{"automergeType": "branch"}`,
		"do_config":   `{"automergeType": "pr"}`,
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	text := resultText(t, result)
	if result.IsError {
		t.Fatalf("unexpected error result: %s", text)
	}

	if !strings.Contains(text, "Preference learned") {
		t.Errorf("expected 'Preference learned' in output: %s", text)
	}
	if !strings.Contains(text, "[automerge]") {
		t.Errorf("expected category in output: %s", text)
	}
	if !strings.Contains(text, "Always use PR automerge type") {
		t.Errorf("expected description in output: %s", text)
	}

	// Verify it landed in the database.
	prefs, err := store.QueryRenovatePreferences("automerge", 10)
	if err != nil {
		t.Fatalf("QueryRenovatePreferences: %v", err)
	}
	if len(prefs) == 0 {
		t.Fatal("expected at least 1 preference in DB after insert")
	}

	found := false
	for _, p := range prefs {
		if p.Description == "Always use PR automerge type for visibility" {
			found = true
			if p.Frequency != 1 {
				t.Errorf("expected frequency 1, got %d", p.Frequency)
			}
		}
	}
	if !found {
		t.Error("inserted preference not found in DB")
	}
}

func TestLearnRenovatePreferenceDedup(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleLearnRenovatePreference(store)

	args := map[string]interface{}{
		"category":    "scheduling",
		"description": "Limit PRs to 5 per day",
	}

	// Insert twice.
	for i := 0; i < 2; i++ {
		req := mcp.CallToolRequest{}
		req.Params.Arguments = args
		result, err := handler(context.Background(), req)
		if err != nil {
			t.Fatalf("handler error on iteration %d: %v", i, err)
		}
		if result.IsError {
			text := resultText(t, result)
			t.Fatalf("unexpected error result on iteration %d: %s", i, text)
		}
	}

	// Verify frequency incremented to 2.
	prefs, err := store.QueryRenovatePreferences("scheduling", 10)
	if err != nil {
		t.Fatalf("QueryRenovatePreferences: %v", err)
	}

	found := false
	for _, p := range prefs {
		if p.Description == "Limit PRs to 5 per day" {
			found = true
			if p.Frequency != 2 {
				t.Errorf("expected frequency 2 after dedup, got %d", p.Frequency)
			}
		}
	}
	if !found {
		t.Error("preference not found in DB after two inserts")
	}
}

func TestLearnRenovatePreferenceInvalidCategory(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleLearnRenovatePreference(store)

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"category":    "nonexistent",
		"description": "Some preference",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}

	if !result.IsError {
		t.Error("expected error result for invalid category")
	}

	text := resultText(t, result)
	if !strings.Contains(text, "invalid category") {
		t.Errorf("expected 'invalid category' in error: %s", text)
	}
}

func TestLearnRenovatePreferenceEmptyFields(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleLearnRenovatePreference(store)

	tests := []struct {
		name string
		args map[string]interface{}
		want string
	}{
		{
			name: "empty category",
			args: map[string]interface{}{
				"category":    "",
				"description": "Some preference",
			},
			want: "category is required",
		},
		{
			name: "empty description",
			args: map[string]interface{}{
				"category":    "automerge",
				"description": "",
			},
			want: "description is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tt.args

			result, err := handler(context.Background(), req)
			if err != nil {
				t.Fatalf("handler error: %v", err)
			}
			if !result.IsError {
				t.Error("expected error result")
			}
			text := resultText(t, result)
			if !strings.Contains(text, tt.want) {
				t.Errorf("expected %q in error: %s", tt.want, text)
			}
		})
	}
}

func TestLearnRenovatePreferenceOptionalConfigs(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleLearnRenovatePreference(store)

	// Insert without dont_config and do_config.
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"category":    "security",
		"description": "Always enable vulnerability alerts",
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		text := resultText(t, result)
		t.Fatalf("unexpected error: %s", text)
	}

	// Verify it was stored.
	prefs, err := store.QueryRenovatePreferences("security", 10)
	if err != nil {
		t.Fatalf("QueryRenovatePreferences: %v", err)
	}
	found := false
	for _, p := range prefs {
		if p.Description == "Always enable vulnerability alerts" {
			found = true
			if p.DontConfig != "" {
				t.Errorf("expected empty dont_config, got %q", p.DontConfig)
			}
			if p.DoConfig != "" {
				t.Errorf("expected empty do_config, got %q", p.DoConfig)
			}
		}
	}
	if !found {
		t.Error("preference not found in DB")
	}
}
