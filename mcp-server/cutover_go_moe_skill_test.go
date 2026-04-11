package main

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

// goSkillPath is the path from mcp-server/ to the /go orchestrator skill file.
const goSkillPath = "../skills/go/SKILL.md"

// allowedGoSkillFrontmatterMCP lists the mcp__go-guardian__* entries
// permitted in the /go SKILL.md frontmatter tools: list after the cutover.
// Everything else in the go-guardian MCP surface must be absent.
var allowedGoSkillFrontmatterMCP = map[string]bool{
	"mcp__go-guardian__query_knowledge": true,
	"mcp__go-guardian__suggest_fix":     true,
}

// requiredGoSkillScanArtifacts are the six scan artifacts the MoE gating
// pipeline reads to decide which reviewer agents to spawn. All six must
// be referenced by name in the /go skill body.
var requiredGoSkillScanArtifacts = []string{
	".go-guardian/owasp-findings.md",
	".go-guardian/dep-vulns.md",
	".go-guardian/staleness.md",
	".go-guardian/pattern-stats.md",
	".go-guardian/health-trends.md",
	".go-guardian/session-findings.md",
}

// requiredGoSkillRoutes are the per-dimension skills the /go orchestrator
// must route to from its explicit subcommand section. Each must appear as
// a literal slash-invocation so the static router is discoverable.
var requiredGoSkillRoutes = []string{
	"/go-review",
	"/go-security",
	"/go-lint",
	"/go-test",
	"/go-patterns",
	"/renovate",
}

// requiredGoSkillMoEMarkers are structural markers that prove the body
// describes the four-stage MoE gating pipeline (classify → scan → gate →
// report), plus the idempotence / warm-cache story.
var requiredGoSkillMoEMarkers = []string{
	"go-guardian scan --all",
	"Mixture-of-Experts",
	"classif",       // "classify" / "classification" / "classifier"
	"deterministic", // rule-based, not LLM
	"empty",         // empty-findings short-circuit
	"spawn",         // spawn or spawned
	"skip",          // skip or skipped
	"run report",    // the stage 4 run report
	"cache",         // warm-start cache / idempotence
}

func readGoSkill(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(goSkillPath)
	if err != nil {
		t.Fatalf("read %s: %v", goSkillPath, err)
	}
	return string(data)
}

func TestCutoverGoMoeSkillFrontmatterToolsWhitelist(t *testing.T) {
	content := readGoSkill(t)
	fm, _ := splitFrontmatter(t, content)
	re := regexp.MustCompile(`mcp__go-guardian__[a-zA-Z0-9_]+`)
	for _, match := range re.FindAllString(fm, -1) {
		if !allowedGoSkillFrontmatterMCP[match] {
			t.Errorf("skills/go/SKILL.md: disallowed frontmatter tool %q (allowed: %v)", match, allowedGoSkillFrontmatterMCP)
		}
	}
}

func TestCutoverGoMoeSkillBodyNoForbiddenToolIdentifiers(t *testing.T) {
	content := readGoSkill(t)
	_, body := splitFrontmatter(t, content)
	for _, needle := range forbiddenToolIdentifiers {
		if strings.Contains(body, needle) {
			t.Errorf("skills/go/SKILL.md: body references forbidden tool identifier %q", needle)
		}
	}
}

func TestCutoverGoMoeSkillReferencesScanArtifacts(t *testing.T) {
	content := readGoSkill(t)
	for _, artifact := range requiredGoSkillScanArtifacts {
		if !strings.Contains(content, artifact) {
			t.Errorf("skills/go/SKILL.md: missing reference to scan artifact %q", artifact)
		}
	}
}

func TestCutoverGoMoeSkillHasMoEGatingStructure(t *testing.T) {
	content := readGoSkill(t)
	for _, marker := range requiredGoSkillMoEMarkers {
		if !strings.Contains(content, marker) {
			t.Errorf("skills/go/SKILL.md: missing MoE gating structural marker %q", marker)
		}
	}
	for _, route := range requiredGoSkillRoutes {
		if !strings.Contains(content, route) {
			t.Errorf("skills/go/SKILL.md: missing per-dimension skill route %q", route)
		}
	}
	// The classifier must be rule-based, not an LLM. If the skill
	// accidentally documents an LLM classifier, that defeats the latency
	// goal this feature exists to enforce.
	lowerContent := strings.ToLower(content)
	if strings.Contains(lowerContent, "llm classifier") || strings.Contains(lowerContent, "language model classifier") {
		t.Error("skills/go/SKILL.md: classifier must be rule-based, not LLM — found LLM classifier reference")
	}
}
