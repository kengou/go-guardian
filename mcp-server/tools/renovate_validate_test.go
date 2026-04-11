package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// callValidate is a test helper that invokes RunValidateRenovateConfig directly.
// It returns (text, isError) where isError is true when the underlying call
// returned an error (e.g. missing file).
func callValidate(t *testing.T, configPath string) (string, bool) {
	t.Helper()
	text, err := RunValidateRenovateConfig(nil, configPath)
	if err != nil {
		return err.Error(), true
	}
	return text, false
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
		name           string
		config         string
		wantErr        bool // expect the underlying call to return an error
		wantContain    []string
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

			text, isErr := callValidate(t, configPath)

			if tt.wantErr {
				if !isErr {
					t.Errorf("expected error, got success.\nOutput:\n%s", text)
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
