package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// skillsDir is the path from mcp-server/ to the skills directory at the repo root.
const cutoverSkillsDir = "../skills"

// cutoverSkillFiles lists the six per-dimension skill SKILL.md files in scope
// for the skill-rewrites feature. Each entry is a directory name under
// ../skills containing a SKILL.md file.
var cutoverSkillFiles = []string{
	"go-review",
	"go-security",
	"go-lint",
	"go-test",
	"go-patterns",
	"renovate",
}

// allowedSkillFrontmatterMCP lists, per skill directory, which
// mcp__go-guardian__* entries are permitted in the frontmatter tools: list.
// Any mcp__go-guardian__* match not in this map is a failure.
//
// - go-review retains query_knowledge (semantic read) and suggest_fix (inline fix proposals).
// - go-security, go-lint, go-test, go-patterns retain only query_knowledge.
// - renovate retains nothing — all operations drive the go-guardian CLI via Bash.
var allowedSkillFrontmatterMCP = map[string]map[string]bool{
	"go-review": {
		"mcp__go-guardian__query_knowledge": true,
		"mcp__go-guardian__suggest_fix":     true,
	},
	"go-security": {"mcp__go-guardian__query_knowledge": true},
	"go-lint":     {"mcp__go-guardian__query_knowledge": true},
	"go-test":     {"mcp__go-guardian__query_knowledge": true},
	"go-patterns": {"mcp__go-guardian__query_knowledge": true},
	"renovate":    {},
}

// skillScanArtifactMarkers are the markdown artifact paths every review-capable
// skill should reference (either as an input to read or the inbox output).
var skillScanArtifactMarkers = []string{
	".go-guardian/owasp-findings.md",
	".go-guardian/dep-vulns.md",
	".go-guardian/staleness.md",
	".go-guardian/pattern-stats.md",
	".go-guardian/health-trends.md",
	".go-guardian/session-findings.md",
	".go-guardian/inbox/",
}

func readCutoverSkill(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(cutoverSkillsDir, dir, "SKILL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func TestCutoverSkillFrontmatterToolsWhitelist(t *testing.T) {
	re := regexp.MustCompile(`mcp__go-guardian__[a-zA-Z0-9_]+`)
	for _, dir := range cutoverSkillFiles {
		t.Run(dir, func(t *testing.T) {
			content := readCutoverSkill(t, dir)
			fm, _ := splitFrontmatter(t, content)
			allowed := allowedSkillFrontmatterMCP[dir]
			for _, match := range re.FindAllString(fm, -1) {
				if !allowed[match] {
					t.Errorf("skill %s: disallowed frontmatter tool %q (allowed: %v)", dir, match, allowed)
				}
			}
		})
	}
}

func TestCutoverSkillBodyNoForbiddenToolIdentifiers(t *testing.T) {
	for _, dir := range cutoverSkillFiles {
		t.Run(dir, func(t *testing.T) {
			content := readCutoverSkill(t, dir)
			_, body := splitFrontmatter(t, content)
			for _, needle := range forbiddenToolIdentifiers {
				if strings.Contains(body, needle) {
					t.Errorf("skill %s: body references forbidden tool identifier %q", dir, needle)
				}
			}
		})
	}
}

func TestCutoverSkillReferencesFileArtifacts(t *testing.T) {
	// renovate is CLI-only; it drives the go-guardian binary directly and
	// doesn't need to reference the scan artifacts. Every other skill must
	// reference at least one scan artifact or the inbox directory.
	for _, dir := range cutoverSkillFiles {
		if dir == "renovate" {
			continue
		}
		t.Run(dir, func(t *testing.T) {
			content := readCutoverSkill(t, dir)
			found := false
			for _, marker := range skillScanArtifactMarkers {
				if strings.Contains(content, marker) {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("skill %s: does not reference any scan artifact or inbox path", dir)
			}
		})
	}
}

func TestCutoverRenovateSkillCLI(t *testing.T) {
	content := readCutoverSkill(t, "renovate")
	verbs := []string{"validate", "analyze", "suggest", "query", "stats"}
	for _, v := range verbs {
		needle := "go-guardian renovate " + v
		if !strings.Contains(content, needle) {
			t.Errorf("skills/renovate/SKILL.md missing CLI invocation %q", needle)
		}
	}
	// The learn operation has no CLI verb — it must drop an inbox document.
	if !strings.Contains(content, ".go-guardian/inbox/") {
		t.Error("skills/renovate/SKILL.md missing inbox fallback for learn")
	}
}
