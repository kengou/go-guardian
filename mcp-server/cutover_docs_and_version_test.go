package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Cutover Wave 4 — docs-and-version. These tests assert the post-cutover
// state of version strings, CLAUDE.md tool-ownership text, and the v0.4.0
// CHANGELOG entry. They go RED when any of those three artifacts still
// carries pre-cutover content and GREEN once the wave has landed.

const (
	cutoverDocsTargetVersion = "0.4.0"
)

// repoRoot walks up from the test file's directory until it finds the
// go-guardian module root (the directory containing `mcp-server/` and
// `CLAUDE.md`). Keeps the tests location-independent so they can run
// from `go test ./mcp-server/...` or from inside `mcp-server/`.
func repoRoot(t *testing.T) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	dir := cwd
	for range 10 {
		if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err == nil {
			if _, err := os.Stat(filepath.Join(dir, "mcp-server")); err == nil {
				return dir
			}
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatalf("could not locate go-guardian repo root from %s", cwd)
	return ""
}

// deprecatedToolNames is the canonical list of 15 MCP tools removed in Wave 3.
// The integration tests assert these names no longer appear in CLAUDE.md and
// are explicitly enumerated in the CHANGELOG v0.4.0 entry.
var deprecatedToolNames = []string{
	"learn_from_lint",
	"learn_from_review",
	"report_finding",
	"check_owasp",
	"check_deps",
	"check_staleness",
	"get_pattern_stats",
	"get_health_trends",
	"get_session_findings",
	"validate_renovate_config",
	"analyze_renovate_config",
	"suggest_renovate_rule",
	"learn_renovate_preference",
	"query_renovate_knowledge",
	"get_renovate_stats",
}

// TestCutoverDocsAndVersionPluginJSONVersion asserts that plugin.json advertises
// the post-cutover version. The raw string match avoids pulling in an encoding/json
// dependency for a single field check.
func TestCutoverDocsAndVersionPluginJSONVersion(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	needle := `"version": "` + cutoverDocsTargetVersion + `"`
	if !strings.Contains(string(data), needle) {
		t.Errorf("plugin.json missing %q; version string was not bumped to %s", needle, cutoverDocsTargetVersion)
	}
}

// TestCutoverDocsAndVersionMainGoVersionConst asserts that main.go's version
// constant matches plugin.json. The two must stay in sync — the constant is
// what `go-guardian --version` prints.
func TestCutoverDocsAndVersionMainGoVersionConst(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "mcp-server", "main.go")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	needle := `const version = "` + cutoverDocsTargetVersion + `"`
	if !strings.Contains(string(data), needle) {
		t.Errorf("mcp-server/main.go missing %q; version constant was not bumped to %s", needle, cutoverDocsTargetVersion)
	}
}

// TestCutoverDocsAndVersionCLAUDEMdHasNoDeprecatedTools asserts the cutover
// has scrubbed all 15 deprecated tool names from CLAUDE.md. After the
// rewrite, only `query_knowledge` and `suggest_fix` should remain in the
// MCP Tool Ownership section.
func TestCutoverDocsAndVersionCLAUDEMdHasNoDeprecatedTools(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)
	for _, name := range deprecatedToolNames {
		if strings.Contains(content, name) {
			t.Errorf("CLAUDE.md still references deprecated tool %q — must be scrubbed from MCP Tool Ownership section", name)
		}
	}
}

// TestCutoverDocsAndVersionCLAUDEMdPluginLayerMapRewritten asserts the
// Plugin Layer Map's go-guardian row now references the CLI subcommands
// and the inbox-based learning loop, per the feature plan's target shape.
func TestCutoverDocsAndVersionCLAUDEMdPluginLayerMapRewritten(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "CLAUDE.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	content := string(data)

	// Must reference the inbox path that replaces the learn_from_* tools.
	if !strings.Contains(content, ".go-guardian/inbox/") {
		t.Error("CLAUDE.md Plugin Layer Map should reference .go-guardian/inbox/ learning loop")
	}
	// Must reference at least one of the new CLI subcommands.
	cliMarkers := []string{"go-guardian scan", "go-guardian ingest", "go-guardian renovate", "go-guardian admin", "go-guardian healthcheck"}
	foundCLI := false
	for _, m := range cliMarkers {
		if strings.Contains(content, m) {
			foundCLI = true
			break
		}
	}
	if !foundCLI {
		t.Errorf("CLAUDE.md Plugin Layer Map should name at least one CLI subcommand (%v)", cliMarkers)
	}
	// Must still reference the two retained MCP tools in the ownership section.
	if !strings.Contains(content, "query_knowledge") {
		t.Error("CLAUDE.md should still reference query_knowledge (retained MCP tool)")
	}
	if !strings.Contains(content, "suggest_fix") {
		t.Error("CLAUDE.md should still reference suggest_fix (retained MCP tool)")
	}
}

// TestCutoverDocsAndVersionChangelogExists asserts CHANGELOG.md was created.
func TestCutoverDocsAndVersionChangelogExists(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "CHANGELOG.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("CHANGELOG.md missing at %s: %v", path, err)
	}
}

// TestCutoverDocsAndVersionChangelogV040Entry asserts the CHANGELOG has the
// required v0.4.0 entry with all five required subsections.
func TestCutoverDocsAndVersionChangelogV040Entry(t *testing.T) {
	root := repoRoot(t)
	path := filepath.Join(root, "CHANGELOG.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read CHANGELOG.md: %v", err)
	}
	content := string(data)

	if !strings.Contains(content, "0.4.0") {
		t.Fatal("CHANGELOG.md missing v0.4.0 entry")
	}

	requiredSections := []string{
		"Breaking Changes",
		"CLI",
		"Migration",
		"Performance",
		"Unchanged",
	}
	for _, section := range requiredSections {
		if !strings.Contains(content, section) {
			t.Errorf("CHANGELOG.md v0.4.0 entry missing %q section", section)
		}
	}

	// Every deprecated tool name must appear in the changelog breaking-changes list.
	for _, name := range deprecatedToolNames {
		if !strings.Contains(content, name) {
			t.Errorf("CHANGELOG.md v0.4.0 entry does not enumerate removed tool %q", name)
		}
	}
}

// TestCutoverDocsAndVersionFinalGrepGate is the acceptance criterion's final
// grep: after this wave lands, no deprecated `mcp__go-guardian__*` reference
// may survive in agents/, skills/, hooks/, or CLAUDE.md.
func TestCutoverDocsAndVersionFinalGrepGate(t *testing.T) {
	root := repoRoot(t)
	// Use ripgrep if available (project standard) else fall back to grep -r.
	pattern := `mcp__go-guardian__(learn_from|check_|get_|report_finding|validate_renovate|analyze_renovate|suggest_renovate|learn_renovate|query_renovate|get_renovate)`
	targets := []string{"agents", "skills", "hooks", "CLAUDE.md"}
	args := append([]string{"-rE", pattern}, targets...)
	cmd := exec.Command("grep", args...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	// grep exits 1 when no matches — that is the success state for this gate.
	if err == nil && len(out) > 0 {
		t.Errorf("final cutover grep gate found matches:\n%s", out)
	}
}
