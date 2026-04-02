package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
)

// writeAnalyzeConfig writes a JSON config to a temp file and returns the path.
func writeAnalyzeConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "renovate.json")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

// callAnalyze invokes the analyze handler and returns the text output.
func callAnalyze(t *testing.T, store *db.Store, configPath string) (string, bool) {
	t.Helper()
	handler := handleAnalyzeRenovateConfig(store)
	req := mcp.CallToolRequest{}
	req.Params.Name = "analyze_renovate_config"
	req.Params.Arguments = map[string]interface{}{
		"config_path": configPath,
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}

	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	return tc.Text, result.IsError
}

func TestAnalyzeConfig(t *testing.T) {
	tests := []struct {
		name           string
		config         string
		wantContain    []string
		wantNotContain []string
		wantScoreBelow int // 0 means don't check
		wantScoreAbove int // 0 means don't check
	}{
		{
			name:   "empty config gets low score",
			config: `{}`,
			wantContain: []string{
				"Renovate Config Analysis",
				"Score:",
				"Score saved for trend tracking.",
			},
			wantScoreBelow: 80,
		},
		{
			name: "config with vulnerability alerts scores higher than empty",
			config: `{
				"vulnerabilityAlerts": {"enabled": true, "labels": ["security"]},
				"prConcurrentLimit": 10,
				"packageRules": [
					{"matchUpdateTypes": ["patch"], "automerge": true, "automergeType": "pr"},
					{"matchManagers": ["github-actions"], "pinDigests": true}
				]
			}`,
			wantContain: []string{
				"Renovate Config Analysis",
				"Score:",
			},
			wantScoreAbove: 30,
		},
		{
			name: "config with disabled vulnerability alerts triggers CRITICAL",
			config: `{
				"vulnerabilityAlerts": {"enabled": false}
			}`,
			wantContain: []string{
				"CRITICAL",
				"SEC-1",
				"vulnerability",
			},
		},
		{
			name: "config with rangeStrategy auto triggers WARN",
			config: `{
				"rangeStrategy": "auto",
				"vulnerabilityAlerts": {"enabled": true}
			}`,
			wantContain: []string{
				"SEC-6",
				"rangeStrategy",
			},
		},
		{
			name: "config with major automerge triggers CRITICAL",
			config: `{
				"vulnerabilityAlerts": {"enabled": true},
				"packageRules": [
					{"matchUpdateTypes": ["major"], "automerge": true}
				]
			}`,
			wantContain: []string{
				"CRITICAL",
				"AM-4",
			},
		},
		{
			name: "findings reference rule IDs",
			config: `{}`,
			wantContain: []string{
				"[SEC-",
				"[SCH-",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			store := newRenovateTestStore(t)
			configPath := writeAnalyzeConfig(t, tt.config)

			text, isErr := callAnalyze(t, store, configPath)
			if isErr {
				t.Fatalf("unexpected error result:\n%s", text)
			}

			for _, want := range tt.wantContain {
				if !strings.Contains(text, want) {
					t.Errorf("output missing %q.\nFull output:\n%s", want, text)
				}
			}
			for _, notWant := range tt.wantNotContain {
				if strings.Contains(text, notWant) {
					t.Errorf("output unexpectedly contains %q.\nFull output:\n%s", notWant, text)
				}
			}

			// Extract score from output.
			score := extractScore(t, text)
			if tt.wantScoreBelow > 0 && score >= tt.wantScoreBelow {
				t.Errorf("score %d should be below %d.\nFull output:\n%s", score, tt.wantScoreBelow, text)
			}
			if tt.wantScoreAbove > 0 && score <= tt.wantScoreAbove {
				t.Errorf("score %d should be above %d.\nFull output:\n%s", score, tt.wantScoreAbove, text)
			}
		})
	}
}

func TestAnalyzeConfig_ScorePersisted(t *testing.T) {
	store := newRenovateTestStore(t)
	configPath := writeAnalyzeConfig(t, `{"vulnerabilityAlerts": {"enabled": true}}`)

	text, isErr := callAnalyze(t, store, configPath)
	if isErr {
		t.Fatalf("unexpected error:\n%s", text)
	}
	if !strings.Contains(text, "Score saved for trend tracking.") {
		t.Errorf("expected score saved message.\nFull output:\n%s", text)
	}

	// Verify the score was persisted.
	scores, err := store.GetConfigScores(configPath, 10)
	if err != nil {
		t.Fatalf("GetConfigScores: %v", err)
	}
	if len(scores) == 0 {
		t.Fatal("no config scores persisted after analyze")
	}
	if scores[0].ConfigPath != configPath {
		t.Errorf("config_path: want %q, got %q", configPath, scores[0].ConfigPath)
	}
	if scores[0].Score < 0 || scores[0].Score > 100 {
		t.Errorf("score out of range: %d", scores[0].Score)
	}
	if scores[0].FindingsCount < 0 {
		t.Errorf("findings_count should be >= 0, got %d", scores[0].FindingsCount)
	}

	// Findings detail should be valid JSON.
	var findings []renovateFinding
	if err := json.Unmarshal([]byte(scores[0].FindingsDetail), &findings); err != nil {
		t.Errorf("findings_detail is not valid JSON: %v\nDetail: %s", err, scores[0].FindingsDetail)
	}
}

func TestAnalyzeConfig_MultipleRunsCreateMultipleScores(t *testing.T) {
	store := newRenovateTestStore(t)
	configPath := writeAnalyzeConfig(t, `{}`)

	// Run twice.
	callAnalyze(t, store, configPath)
	callAnalyze(t, store, configPath)

	scores, err := store.GetConfigScores(configPath, 10)
	if err != nil {
		t.Fatalf("GetConfigScores: %v", err)
	}
	if len(scores) < 2 {
		t.Errorf("expected at least 2 scores after 2 runs, got %d", len(scores))
	}
}

func TestAnalyzeConfig_MissingConfigPath(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleAnalyzeRenovateConfig(store)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if !result.IsError {
		t.Error("expected IsError=true for missing config_path")
	}
	if !strings.Contains(tc.Text, "config_path is required") {
		t.Errorf("expected 'config_path is required', got: %s", tc.Text)
	}
}

func TestAnalyzeConfig_InvalidJSON(t *testing.T) {
	store := newRenovateTestStore(t)
	configPath := writeAnalyzeConfig(t, `{not valid json}`)

	handler := handleAnalyzeRenovateConfig(store)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"config_path": configPath,
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected TextContent, got %T", result.Content[0])
	}
	if !result.IsError {
		t.Error("expected IsError=true for invalid JSON")
	}
	if !strings.Contains(tc.Text, "invalid JSON") {
		t.Errorf("expected 'invalid JSON' error, got: %s", tc.Text)
	}
}

func TestAnalyzeConfig_NonexistentFile(t *testing.T) {
	store := newRenovateTestStore(t)
	handler := handleAnalyzeRenovateConfig(store)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"config_path": filepath.Join(t.TempDir(), "does-not-exist.json"),
	}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if !result.IsError {
		t.Error("expected IsError=true for nonexistent file")
	}
}

func TestAnalyzeConfig_GoodConfigScoresHigherThanBad(t *testing.T) {
	store := newRenovateTestStore(t)

	badPath := writeAnalyzeConfig(t, `{
		"vulnerabilityAlerts": {"enabled": false},
		"rangeStrategy": "auto",
		"packageRules": [
			{"matchUpdateTypes": ["major"], "automerge": true}
		]
	}`)

	goodPath := writeAnalyzeConfig(t, `{
		"vulnerabilityAlerts": {"enabled": true, "labels": ["security"]},
		"rangeStrategy": "pin",
		"prConcurrentLimit": 10,
		"branchConcurrentLimit": 5,
		"schedule": ["after 9pm and before 6am every weekday"],
		"timezone": "Europe/Berlin",
		"rebaseWhen": "behind-base-branch",
		"packageRules": [
			{"matchUpdateTypes": ["patch"], "automerge": true, "automergeType": "pr"},
			{"matchManagers": ["github-actions"], "pinDigests": true},
			{"matchUpdateTypes": ["major"], "automerge": false, "labels": ["breaking"]}
		]
	}`)

	badText, _ := callAnalyze(t, store, badPath)
	goodText, _ := callAnalyze(t, store, goodPath)

	badScore := extractScore(t, badText)
	goodScore := extractScore(t, goodText)

	if goodScore <= badScore {
		t.Errorf("good config score (%d) should be higher than bad config score (%d).\nGood output:\n%s\nBad output:\n%s",
			goodScore, badScore, goodText, badText)
	}
}

func TestEvaluateRule_DontConfigMatch(t *testing.T) {
	rule := db.RenovateRule{
		RuleID:     "TEST-1",
		Category:   "test",
		Title:      "Test rule",
		DontConfig: `{"automerge": false}`,
		DoConfig:   `{"automerge": true}`,
		Severity:   "WARN",
	}

	tests := []struct {
		name       string
		config     map[string]interface{}
		wantViolation bool
	}{
		{
			name:          "matches dont pattern",
			config:        map[string]interface{}{"automerge": false},
			wantViolation: true,
		},
		{
			name:          "does not match — opposite value",
			config:        map[string]interface{}{"automerge": true},
			wantViolation: false,
		},
		{
			name:          "does not match — key absent",
			config:        map[string]interface{}{"other": "value"},
			wantViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, violated := evaluateRule(rule, tt.config)
			if violated != tt.wantViolation {
				t.Errorf("evaluateRule: want violation=%v, got %v", tt.wantViolation, violated)
			}
		})
	}
}

func TestEvaluateRule_MissingConfig(t *testing.T) {
	rule := db.RenovateRule{
		RuleID:     "TEST-2",
		Category:   "test",
		Title:      "Require prConcurrentLimit",
		DontConfig: "{}",
		DoConfig:   `{"prConcurrentLimit": 10}`,
		Severity:   "WARN",
	}

	tests := []struct {
		name          string
		config        map[string]interface{}
		wantViolation bool
	}{
		{
			name:          "key missing — violation",
			config:        map[string]interface{}{},
			wantViolation: true,
		},
		{
			name:          "key present — no violation",
			config:        map[string]interface{}{"prConcurrentLimit": float64(5)},
			wantViolation: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, violated := evaluateRule(rule, tt.config)
			if violated != tt.wantViolation {
				t.Errorf("evaluateRule: want violation=%v, got %v", tt.wantViolation, violated)
			}
		})
	}
}

func TestMatchesValue(t *testing.T) {
	tests := []struct {
		name      string
		configVal interface{}
		dontVal   interface{}
		want      bool
	}{
		{"bool match", false, false, true},
		{"bool mismatch", true, false, false},
		{"string match", "auto", "auto", true},
		{"string mismatch", "pin", "auto", false},
		{"float match", float64(10), float64(10), true},
		{"float mismatch", float64(5), float64(10), false},
		{
			"nested map match",
			map[string]interface{}{"enabled": false, "labels": []interface{}{"security"}},
			map[string]interface{}{"enabled": false},
			true,
		},
		{
			"nested map mismatch",
			map[string]interface{}{"enabled": true},
			map[string]interface{}{"enabled": false},
			false,
		},
		{
			"array element match",
			[]interface{}{
				map[string]interface{}{"matchUpdateTypes": []interface{}{"major"}, "automerge": true},
			},
			[]interface{}{
				map[string]interface{}{"matchUpdateTypes": []interface{}{"major"}, "automerge": true},
			},
			true,
		},
		{
			"array element no match",
			[]interface{}{
				map[string]interface{}{"matchUpdateTypes": []interface{}{"patch"}, "automerge": true},
			},
			[]interface{}{
				map[string]interface{}{"matchUpdateTypes": []interface{}{"major"}, "automerge": true},
			},
			false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchesValue(tt.configVal, tt.dontVal)
			if got != tt.want {
				t.Errorf("matchesValue: want %v, got %v", tt.want, got)
			}
		})
	}
}

// extractScore parses the "Score: NN/100" line from the output.
func extractScore(t *testing.T, text string) int {
	t.Helper()
	var score int
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "Score:") {
			if _, err := fmt.Sscanf(line, "Score: %d/100", &score); err == nil {
				return score
			}
		}
	}
	t.Fatalf("could not extract score from output:\n%s", text)
	return 0
}
