package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// callValidate is a test helper that invokes the validate handler directly.
func callValidate(t *testing.T, configPath string) *mcp.CallToolResult {
	t.Helper()
	handler := handleValidateRenovateConfig(nil)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{
		"config_path": configPath,
	}
	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}
	return result
}

// resultText extracts the text content from a CallToolResult.
func resultText(t *testing.T, result *mcp.CallToolResult) string {
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

func writeTestConfig(t *testing.T, name, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write test config: %v", err)
	}
	return p
}

func TestValidateConfig(t *testing.T) {
	// Ensure dry-run is skipped by clearing token.
	t.Setenv("RENOVATE_TOKEN", "")

	tests := []struct {
		name       string
		config     string
		wantErr    bool // expect IsError on the result
		wantContain []string
		wantNotContain []string
	}{
		{
			name:   "valid config clean result",
			config: `{"extends": ["config:recommended"], "packageRules": [{"matchPackageNames": ["lodash"], "enabled": false}]}`,
			wantContain: []string{
				"Valid JSON syntax",
				"Structure valid",
				"0 warning(s), 0 error(s)",
			},
		},
		{
			name:   "invalid JSON syntax",
			config: `{"extends": ["config:recommended",}`,
			wantContain: []string{
				"ERR",
				"Invalid JSON syntax",
				"line",
				"column",
			},
		},
		{
			name:   "deprecated option versionScheme",
			config: `{"versionScheme": "semver"}`,
			wantContain: []string{
				"Deprecated option 'versionScheme'",
				"versioning",
				"1 warning(s)",
			},
		},
		{
			name:   "deprecated option ignoreUnstable",
			config: `{"ignoreUnstable": true}`,
			wantContain: []string{
				"Deprecated option 'ignoreUnstable'",
			},
		},
		{
			name:   "deprecated option separatePatchReleases",
			config: `{"separatePatchReleases": true}`,
			wantContain: []string{
				"Deprecated option 'separatePatchReleases'",
				"separateMinorPatch",
			},
		},
		{
			name:   "deprecated option unpublishSafe",
			config: `{"unpublishSafe": true}`,
			wantContain: []string{
				"Deprecated option 'unpublishSafe'",
			},
		},
		{
			name:   "deprecated automergeType branch-merge-commit",
			config: `{"automergeType": "branch-merge-commit"}`,
			wantContain: []string{
				"Deprecated automergeType value 'branch-merge-commit'",
			},
		},
		{
			name:    "missing file",
			config:  "", // special-cased below
			wantErr: true,
			wantContain: []string{
				"cannot read config file",
			},
		},
		{
			name:   "empty extends array",
			config: `{"extends": []}`,
			wantContain: []string{
				"'extends' array is empty",
				"1 warning(s)",
			},
		},
		{
			name:   "extends not an array",
			config: `{"extends": "config:recommended"}`,
			wantContain: []string{
				"ERR",
				"'extends' must be an array of strings",
			},
		},
		{
			name:   "extends contains non-string",
			config: `{"extends": ["config:recommended", 42]}`,
			wantContain: []string{
				"ERR",
				"extends[1]",
				"must be a string",
			},
		},
		{
			name:   "packageRules not an array",
			config: `{"packageRules": "not-an-array"}`,
			wantContain: []string{
				"ERR",
				"'packageRules' must be an array of objects",
			},
		},
		{
			name:   "packageRules entry not an object",
			config: `{"packageRules": ["not-an-object"]}`,
			wantContain: []string{
				"ERR",
				"packageRules[0]",
				"must be an object",
			},
		},
		{
			name:   "top-level not an object",
			config: `[1, 2, 3]`,
			wantContain: []string{
				"ERR",
				"Top-level value must be a JSON object",
			},
		},
		{
			name:   "dry run skipped without token",
			config: `{}`,
			wantContain: []string{
				"Skipped: RENOVATE_TOKEN not set",
				"export RENOVATE_TOKEN",
			},
		},
		{
			name:   "multiple deprecated options",
			config: `{"versionScheme": "semver", "unpublishSafe": true, "separatePatchReleases": true}`,
			wantContain: []string{
				"Deprecated option 'versionScheme'",
				"Deprecated option 'unpublishSafe'",
				"Deprecated option 'separatePatchReleases'",
				"3 warning(s)",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var configPath string
			if tt.name == "missing file" {
				configPath = filepath.Join(t.TempDir(), "nonexistent.json")
			} else {
				configPath = writeTestConfig(t, "renovate.json", tt.config)
			}

			result := callValidate(t, configPath)

			text := resultText(t, result)

			if tt.wantErr {
				if !result.IsError {
					t.Errorf("expected IsError=true, got false.\nOutput:\n%s", text)
				}
			}

			for _, want := range tt.wantContain {
				if !strings.Contains(text, want) {
					t.Errorf("output missing expected substring %q.\nFull output:\n%s", want, text)
				}
			}

			for _, notWant := range tt.wantNotContain {
				if strings.Contains(text, notWant) {
					t.Errorf("output unexpectedly contains %q.\nFull output:\n%s", notWant, text)
				}
			}
		})
	}
}

func TestValidateConfig_MissingConfigPath(t *testing.T) {
	handler := handleValidateRenovateConfig(nil)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]any{}

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned unexpected error: %v", err)
	}

	text := resultText(t, result)
	if !result.IsError {
		t.Error("expected IsError=true for missing config_path")
	}
	if !strings.Contains(text, "config_path is required") {
		t.Errorf("expected 'config_path is required' in output, got: %s", text)
	}
}

func TestPositionFromOffset(t *testing.T) {
	tests := []struct {
		data     string
		offset   int64
		wantLine int
		wantCol  int
	}{
		{"hello", 3, 1, 4},
		{"hello\nworld", 6, 2, 1},
		{"line1\nline2\nline3", 12, 3, 1},
		{"", 0, 1, 1},
		{"abc", 0, 1, 1},
	}

	for _, tt := range tests {
		line, col := positionFromOffset([]byte(tt.data), tt.offset)
		if line != tt.wantLine || col != tt.wantCol {
			t.Errorf("positionFromOffset(%q, %d) = (%d, %d), want (%d, %d)",
				tt.data, tt.offset, line, col, tt.wantLine, tt.wantCol)
		}
	}
}
