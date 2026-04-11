package tools

import (
	"strings"
	"testing"
)

func TestLearnRenovatePreferenceInsert(t *testing.T) {
	store := newRenovateTestStore(t)

	text, err := RunLearnRenovatePreference(
		store,
		"automerge",
		"Always use PR automerge type for visibility",
		`{"automergeType": "branch"}`,
		`{"automergeType": "pr"}`,
	)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
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

	// Insert twice.
	for i := 0; i < 2; i++ {
		if _, err := RunLearnRenovatePreference(store, "scheduling", "Limit PRs to 5 per day", "", ""); err != nil {
			t.Fatalf("unexpected error on iteration %d: %v", i, err)
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

	_, err := RunLearnRenovatePreference(store, "nonexistent", "Some preference", "", "")
	if err == nil {
		t.Fatal("expected error for invalid category, got nil")
	}
	if !strings.Contains(err.Error(), "invalid category") {
		t.Errorf("expected 'invalid category' in error: %v", err)
	}
}

func TestLearnRenovatePreferenceEmptyFields(t *testing.T) {
	store := newRenovateTestStore(t)

	tests := []struct {
		name        string
		category    string
		description string
		want        string
	}{
		{
			name:        "empty category",
			category:    "",
			description: "Some preference",
			want:        "category is required",
		},
		{
			name:        "empty description",
			category:    "automerge",
			description: "",
			want:        "description is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := RunLearnRenovatePreference(store, tt.category, tt.description, "", "")
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("expected %q in error: %v", tt.want, err)
			}
		})
	}
}

func TestLearnRenovatePreferenceOptionalConfigs(t *testing.T) {
	store := newRenovateTestStore(t)

	// Insert without dont_config and do_config.
	if _, err := RunLearnRenovatePreference(store, "security", "Always enable vulnerability alerts", "", ""); err != nil {
		t.Fatalf("unexpected error: %v", err)
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
