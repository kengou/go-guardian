package tools

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
)

// newRenovateTestStore creates a Store backed by a temporary SQLite database.
// This is the shared test store constructor for all renovate_*_test.go files.
func newRenovateTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.NewStore(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

// callRenovateStats invokes the stats handler with optional arguments.
func callRenovateStats(t *testing.T, store *db.Store, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	handler := handleGetRenovateStats(store)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = args
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	return result
}

func renovateStatsText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text
}

func TestGetRenovateStats_FreshDatabase(t *testing.T) {
	store := newRenovateTestStore(t)
	result := callRenovateStats(t, store, nil)
	text := renovateStatsText(t, result)

	if result.IsError {
		t.Fatalf("unexpected error result:\n%s", text)
	}

	// Should contain header.
	if !strings.Contains(text, "Renovate Guardian Dashboard") {
		t.Errorf("missing dashboard header.\nOutput:\n%s", text)
	}

	// Seed rules should be present (each seed file has 8 rules, 6 categories).
	if !strings.Contains(text, "Total rules:") {
		t.Errorf("missing total rules line.\nOutput:\n%s", text)
	}
	// The seed data contains rules across multiple categories.
	for _, cat := range []string{"automerge:", "grouping:", "scheduling:", "security:", "custom_datasources:", "automation:"} {
		if !strings.Contains(text, cat) {
			t.Errorf("missing category %q in output.\nOutput:\n%s", cat, text)
		}
	}

	// Zero preferences on a fresh database.
	if !strings.Contains(text, "0 total") {
		t.Errorf("expected '0 total' preferences.\nOutput:\n%s", text)
	}

	// No score history.
	if !strings.Contains(text, "No score history available") {
		t.Errorf("expected 'No score history available'.\nOutput:\n%s", text)
	}
}

func TestGetRenovateStats_ScoreTrendImproving(t *testing.T) {
	store := newRenovateTestStore(t)

	// Insert scores: older score first (lower), then newer (higher).
	if err := store.InsertConfigScore("renovate.json", 70, 10, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}
	if err := store.InsertConfigScore("renovate.json", 85, 5, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}

	result := callRenovateStats(t, store, map[string]any{"config_path": "renovate.json"})
	text := renovateStatsText(t, result)

	if !strings.Contains(text, "improving") {
		t.Errorf("expected 'improving' trend.\nOutput:\n%s", text)
	}
	if !strings.Contains(text, "85/100") {
		t.Errorf("expected latest score '85/100'.\nOutput:\n%s", text)
	}
}

func TestGetRenovateStats_ScoreTrendDegrading(t *testing.T) {
	store := newRenovateTestStore(t)

	if err := store.InsertConfigScore("renovate.json", 90, 2, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}
	if err := store.InsertConfigScore("renovate.json", 75, 8, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}

	result := callRenovateStats(t, store, map[string]any{"config_path": "renovate.json"})
	text := renovateStatsText(t, result)

	if !strings.Contains(text, "degrading") {
		t.Errorf("expected 'degrading' trend.\nOutput:\n%s", text)
	}
}

func TestGetRenovateStats_ScoreTrendStable(t *testing.T) {
	store := newRenovateTestStore(t)

	if err := store.InsertConfigScore("renovate.json", 80, 5, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}
	if err := store.InsertConfigScore("renovate.json", 80, 5, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}

	result := callRenovateStats(t, store, map[string]any{"config_path": "renovate.json"})
	text := renovateStatsText(t, result)

	if !strings.Contains(text, "stable") {
		t.Errorf("expected 'stable' trend.\nOutput:\n%s", text)
	}
}

func TestGetRenovateStats_ScoreTrendSingleEntry(t *testing.T) {
	store := newRenovateTestStore(t)

	if err := store.InsertConfigScore("renovate.json", 80, 5, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}

	result := callRenovateStats(t, store, map[string]any{"config_path": "renovate.json"})
	text := renovateStatsText(t, result)

	// Single score should report stable.
	if !strings.Contains(text, "stable") {
		t.Errorf("expected 'stable' trend for single score entry.\nOutput:\n%s", text)
	}
}

func TestGetRenovateStats_WithPreferences(t *testing.T) {
	store := newRenovateTestStore(t)

	if err := store.InsertRenovatePreference("automerge", "Always use pr automerge type", "{}", "{}"); err != nil {
		t.Fatalf("InsertRenovatePreference: %v", err)
	}
	if err := store.InsertRenovatePreference("scheduling", "Use Europe/Berlin timezone", "{}", "{}"); err != nil {
		t.Fatalf("InsertRenovatePreference: %v", err)
	}
	// Insert same preference again to bump frequency.
	if err := store.InsertRenovatePreference("automerge", "Always use pr automerge type", "{}", "{}"); err != nil {
		t.Fatalf("InsertRenovatePreference: %v", err)
	}

	result := callRenovateStats(t, store, nil)
	text := renovateStatsText(t, result)

	if !strings.Contains(text, "2 total") {
		t.Errorf("expected '2 total' preferences.\nOutput:\n%s", text)
	}
	if !strings.Contains(text, "[freq:2] automerge") {
		t.Errorf("expected automerge preference with freq:2.\nOutput:\n%s", text)
	}
	if !strings.Contains(text, "[freq:1] scheduling") {
		t.Errorf("expected scheduling preference with freq:1.\nOutput:\n%s", text)
	}
}

func TestGetRenovateStats_RecentScoresWithoutConfigPath(t *testing.T) {
	store := newRenovateTestStore(t)

	// Insert scores for different paths.
	if err := store.InsertConfigScore("renovate.json", 70, 10, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}
	if err := store.InsertConfigScore("other/renovate.json", 90, 2, "{}"); err != nil {
		t.Fatalf("InsertConfigScore: %v", err)
	}

	// Without config_path, should show recent scores across all paths.
	result := callRenovateStats(t, store, nil)
	text := renovateStatsText(t, result)

	if strings.Contains(text, "No score history available") {
		t.Errorf("should show score history when scores exist.\nOutput:\n%s", text)
	}
	if !strings.Contains(text, "renovate.json") {
		t.Errorf("expected renovate.json in output.\nOutput:\n%s", text)
	}
	if !strings.Contains(text, "other/renovate.json") {
		t.Errorf("expected other/renovate.json in output.\nOutput:\n%s", text)
	}
}

func TestRenovateComputeTrend(t *testing.T) {
	tests := []struct {
		name   string
		scores []db.ConfigScore
		want   string
	}{
		{
			name:   "empty scores",
			scores: nil,
			want:   "stable",
		},
		{
			name:   "single score",
			scores: []db.ConfigScore{{Score: 80}},
			want:   "stable",
		},
		{
			name:   "improving",
			scores: []db.ConfigScore{{Score: 90}, {Score: 70}},
			want:   "improving",
		},
		{
			name:   "degrading",
			scores: []db.ConfigScore{{Score: 60}, {Score: 80}},
			want:   "degrading",
		},
		{
			name:   "equal",
			scores: []db.ConfigScore{{Score: 80}, {Score: 80}},
			want:   "stable",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := renovateComputeTrend(tt.scores)
			if !strings.Contains(got, tt.want) {
				t.Errorf("renovateComputeTrend() = %q, want containing %q", got, tt.want)
			}
		})
	}
}
