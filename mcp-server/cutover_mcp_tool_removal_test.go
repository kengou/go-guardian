package main

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// mainGoPath is the path from mcp-server/ to the go-guardian MCP server entrypoint.
const mainGoPath = "main.go"

// toolsDirPath is the path from mcp-server/ to the tools package directory.
const toolsDirPath = "tools"

// forbiddenRegisterNames is the full list of Register<Tool> identifiers whose
// call sites must disappear from main.go after the cutover, and whose function
// definitions must disappear from the tools package.
var forbiddenRegisterNames = []string{
	"RegisterLearnFromLint",
	"RegisterLearnFromReview",
	"RegisterReportFinding",
	"RegisterCheckOWASP",
	"RegisterCheckDeps",
	"RegisterCheckStaleness",
	"RegisterGetPatternStats",
	"RegisterGetHealthTrends",
	"RegisterGetSessionFindings",
	"RegisterValidateRenovateConfig",
	"RegisterAnalyzeRenovateConfig",
	"RegisterSuggestRenovateRule",
	"RegisterLearnRenovatePreference",
	"RegisterRenovateQueryKnowledge",
	"RegisterGetRenovateStats",
}

// keptRegisterNames is the pair of Register<Tool> identifiers that must survive
// the cutover. Both are expected to appear in both main.go registration blocks.
var keptRegisterNames = []string{
	"RegisterQueryKnowledge",
	"RegisterSuggestFix",
}

// consumerOnlyHandlerNames lists Run<Tool> handler function definitions that
// must disappear from the tools package because they have no non-MCP caller.
// Handlers that are still reachable from a CLI subcommand (owasp, deps,
// staleness, stats, trends, review, session_report, session_query, and the
// six renovate handlers) are intentionally NOT listed here — the cutover
// preserves them.
var consumerOnlyHandlerNames = []string{
	"RunLearnFromLint",
}

func readMainGo(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(mainGoPath)
	if err != nil {
		t.Fatalf("read %s: %v", mainGoPath, err)
	}
	return string(data)
}

// TestCutoverMCPToolRemovalMainGoHasOnlyTwoRegistrations asserts that main.go
// contains no forbidden tools.Register<Deprecated>( call and that both kept
// registrations appear at least once (they appear twice in practice — once in
// the server bootstrap block and once in dispatchHealthcheck).
func TestCutoverMCPToolRemovalMainGoHasOnlyTwoRegistrations(t *testing.T) {
	content := readMainGo(t)
	for _, name := range forbiddenRegisterNames {
		needle := "tools." + name + "("
		if strings.Contains(content, needle) {
			t.Errorf("main.go: forbidden Register call %q still present", needle)
		}
	}
	for _, name := range keptRegisterNames {
		needle := "tools." + name + "("
		if !strings.Contains(content, needle) {
			t.Errorf("main.go: expected Register call %q is missing", needle)
		}
	}
}

// TestCutoverMCPToolRemovalMainGoReportsTwoTools asserts that the log line in
// the server bootstrap and the pass line in dispatchHealthcheck both report
// the new tool count rather than the pre-cutover count.
func TestCutoverMCPToolRemovalMainGoReportsTwoTools(t *testing.T) {
	content := readMainGo(t)
	// The server bootstrap log line must mention exactly 2 tools.
	// It used to say "registered 17 tools: ..." — any digit other than 2 is a regression.
	logRe := regexp.MustCompile(`registered\s+(\d+)\s+tools?`)
	matches := logRe.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		t.Fatalf("main.go: no 'registered N tool(s)' log line found")
	}
	for _, m := range matches {
		if m[1] != "2" {
			t.Errorf("main.go: log line reports %q tools, want 2 (full match: %q)", m[1], m[0])
		}
	}
	// The dispatchHealthcheck pass line must also mention exactly 2 tools.
	passRe := regexp.MustCompile(`"(\d+)\s+tools?\s+registered"`)
	passMatches := passRe.FindAllStringSubmatch(content, -1)
	if len(passMatches) == 0 {
		t.Fatalf("main.go: no 'N tools registered' healthcheck pass line found")
	}
	for _, m := range passMatches {
		if m[1] != "2" {
			t.Errorf("main.go: healthcheck pass line reports %q tools, want 2 (full match: %q)", m[1], m[0])
		}
	}
}

// TestCutoverMCPToolRemovalToolsPackageNoForbiddenRegisterDefs walks every
// .go source file in mcp-server/tools/ and asserts that no forbidden
// Register<Deprecated> function DEFINITION remains. Test files (*_test.go)
// are allowed to reference the names only inside comments or strings; the
// assertion is against function definitions only.
func TestCutoverMCPToolRemovalToolsPackageNoForbiddenRegisterDefs(t *testing.T) {
	entries, err := os.ReadDir(toolsDirPath)
	if err != nil {
		t.Fatalf("read %s: %v", toolsDirPath, err)
	}
	defRe := make(map[string]*regexp.Regexp, len(forbiddenRegisterNames))
	for _, name := range forbiddenRegisterNames {
		// Matches `func RegisterXxx(` with arbitrary receiver-less form.
		defRe[name] = regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(name) + `\s*\(`)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(toolsDirPath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(data)
		for name, re := range defRe {
			if re.MatchString(body) {
				t.Errorf("%s: forbidden Register function definition %q still present", path, name)
			}
		}
	}
}

// TestCutoverMCPToolRemovalNoConsumerOnlyHandlers asserts that handler
// functions with no non-MCP caller are fully deleted from the tools package.
// This is the one case where the Run<Tool> handler disappears together with
// its Register wrapper — every other handler is preserved because a CLI
// subcommand still reaches it.
func TestCutoverMCPToolRemovalNoConsumerOnlyHandlers(t *testing.T) {
	entries, err := os.ReadDir(toolsDirPath)
	if err != nil {
		t.Fatalf("read %s: %v", toolsDirPath, err)
	}
	defRe := make(map[string]*regexp.Regexp, len(consumerOnlyHandlerNames))
	for _, name := range consumerOnlyHandlerNames {
		defRe[name] = regexp.MustCompile(`(?m)^func\s+` + regexp.QuoteMeta(name) + `\s*\(`)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		path := filepath.Join(toolsDirPath, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		body := string(data)
		for name, re := range defRe {
			if re.MatchString(body) {
				t.Errorf("%s: consumer-only handler %q still present — should have been deleted", path, name)
			}
		}
	}
}
