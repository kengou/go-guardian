package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// agentsDir is the path from mcp-server/ to the agents directory at the repo root.
const cutoverAgentsDir = "../agents"

var cutoverAgentFiles = []string{
	"advisor.md",
	"linter.md",
	"orchestrator.md",
	"patterns.md",
	"reviewer.md",
	"security.md",
	"tester.md",
}

// forbiddenToolIdentifiers are the 15 deprecated MCP tool names that must not
// appear anywhere in an agent markdown file after the cutover.
var forbiddenToolIdentifiers = []string{
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

// allowedFrontmatterMCP lists, per agent, which mcp__go-guardian__* entries
// are permitted in the frontmatter tools: list. Any mcp__go-guardian__* match
// not in this map is a failure.
var allowedFrontmatterMCP = map[string]map[string]bool{
	"advisor.md":      {"mcp__go-guardian__query_knowledge": true},
	"linter.md":       {"mcp__go-guardian__query_knowledge": true},
	"orchestrator.md": {"mcp__go-guardian__query_knowledge": true},
	"patterns.md":     {"mcp__go-guardian__query_knowledge": true},
	"reviewer.md": {
		"mcp__go-guardian__query_knowledge": true,
		"mcp__go-guardian__suggest_fix":     true,
	},
	"security.md": {"mcp__go-guardian__query_knowledge": true},
	"tester.md":   {"mcp__go-guardian__query_knowledge": true},
}

// scanArtifactMarkers are the markdown artifact paths every review-capable
// agent should reference (either as an input to read or the inbox output).
var scanArtifactMarkers = []string{
	".go-guardian/owasp-findings.md",
	".go-guardian/dep-vulns.md",
	".go-guardian/staleness.md",
	".go-guardian/pattern-stats.md",
	".go-guardian/health-trends.md",
	".go-guardian/session-findings.md",
	".go-guardian/inbox/",
}

func readCutoverAgent(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(cutoverAgentsDir, name)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

// splitFrontmatter returns (frontmatter, body). If the file has no YAML
// frontmatter the whole content is returned as the body.
func splitFrontmatter(t *testing.T, content string) (string, string) {
	t.Helper()
	if !strings.HasPrefix(content, "---\n") {
		return "", content
	}
	rest := content[4:]
	end := strings.Index(rest, "\n---")
	if end < 0 {
		t.Fatalf("unterminated frontmatter")
	}
	fm := rest[:end]
	body := rest[end+4:]
	body = strings.TrimPrefix(body, "\n")
	return fm, body
}

func TestCutoverAgentFrontmatterToolsWhitelist(t *testing.T) {
	re := regexp.MustCompile(`mcp__go-guardian__[a-zA-Z0-9_]+`)
	for _, name := range cutoverAgentFiles {
		name := name
		t.Run(name, func(t *testing.T) {
			content := readCutoverAgent(t, name)
			fm, _ := splitFrontmatter(t, content)
			allowed := allowedFrontmatterMCP[name]
			for _, match := range re.FindAllString(fm, -1) {
				if !allowed[match] {
					t.Errorf("agent %s: disallowed frontmatter tool %q (allowed: %v)", name, match, allowed)
				}
			}
		})
	}
}

func TestCutoverAgentBodyNoForbiddenToolIdentifiers(t *testing.T) {
	for _, name := range cutoverAgentFiles {
		name := name
		t.Run(name, func(t *testing.T) {
			content := readCutoverAgent(t, name)
			_, body := splitFrontmatter(t, content)
			for _, needle := range forbiddenToolIdentifiers {
				if strings.Contains(body, needle) {
					t.Errorf("agent %s: body references forbidden tool identifier %q", name, needle)
				}
			}
		})
	}
}

func TestCutoverAgentReferencesFileArtifacts(t *testing.T) {
	// advisor.md is renovate-only; it drives the CLI directly and doesn't
	// need to mention the scan artifacts. Every other agent must reference
	// at least one scan artifact or the inbox directory.
	for _, name := range cutoverAgentFiles {
		if name == "advisor.md" {
			continue
		}
		name := name
		t.Run(name, func(t *testing.T) {
			content := readCutoverAgent(t, name)
			found := false
			for _, marker := range scanArtifactMarkers {
				if strings.Contains(content, marker) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("agent %s: does not reference any scan artifact or inbox path", name)
			}
		})
	}
}

func TestCutoverSecurityThinDispatcher(t *testing.T) {
	content := readCutoverAgent(t, "security.md")
	mustContain := []string{
		"/team-spawn security",
		".go-guardian/owasp-findings.md",
		".go-guardian/dep-vulns.md",
		"HIGH",
		"CRITICAL",
		"query_knowledge",
		".go-guardian/inbox/",
	}
	for _, s := range mustContain {
		if !strings.Contains(content, s) {
			t.Errorf("security.md missing required thin-dispatcher marker %q", s)
		}
	}
}

func TestCutoverAdvisorRenovateCLI(t *testing.T) {
	content := readCutoverAgent(t, "advisor.md")
	verbs := []string{"validate", "analyze", "suggest", "query", "stats"}
	for _, v := range verbs {
		needle := "go-guardian renovate " + v
		if !strings.Contains(content, needle) {
			t.Errorf("advisor.md missing CLI invocation %q", needle)
		}
	}
}
