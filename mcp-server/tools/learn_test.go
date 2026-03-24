package tools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/go-guardian/mcp-server/db"
	"github.com/mark3labs/mcp-go/mcp"
)

// ── helpers ───────────────────────────────────────────────────────────────────

// callLearnTool invokes the learn_from_lint handler directly (no live
// transport needed) and returns the decoded JSON result map.
func callLearnTool(t *testing.T, store *db.Store, diff, lintOutput, project string) map[string]interface{} {
	t.Helper()

	handler := learnFromLintHandler(store)

	args := map[string]interface{}{
		"diff":        diff,
		"lint_output": lintOutput,
	}
	if project != "" {
		args["project"] = project
	}

	req := mcp.CallToolRequest{}
	req.Params.Name = "learn_from_lint"
	req.Params.Arguments = args

	result, err := handler(context.Background(), req)
	if err != nil {
		t.Fatalf("handler returned error: %v", err)
	}
	if result == nil {
		t.Fatal("handler returned nil result")
	}
	if result.IsError {
		t.Fatalf("handler returned tool error: %+v", result.Content)
	}
	if len(result.Content) == 0 {
		t.Fatal("result has no content")
	}
	tc, ok := result.Content[0].(mcp.TextContent)
	if !ok {
		t.Fatalf("expected mcp.TextContent, got %T", result.Content[0])
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(tc.Text), &out); err != nil {
		t.Fatalf("unmarshal result JSON %q: %v", tc.Text, err)
	}
	return out
}

// ── TestParseLintOutput ───────────────────────────────────────────────────────

func TestParseLintOutput(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		wantLen     int
		wantRule    string
		wantFile    string
		wantMessage string // substring
	}{
		{
			name: "standard errcheck finding",
			input: `handler.go:42:5: Error return value of 'db.Close' is not checked (errcheck)
`,
			wantLen:     1,
			wantRule:    "errcheck",
			wantFile:    "handler.go",
			wantMessage: "Error return value",
		},
		{
			name: "govet shadow finding",
			input: `main.go:18:3: declaration of "err" shadows declaration at line 10 (govet)
`,
			wantLen:  1,
			wantRule: "govet",
			wantFile: "main.go",
		},
		{
			name: "multiple findings from same rule+file deduplicated",
			input: `service.go:10:1: exported function Foo should have comment (revive)
service.go:20:1: exported function Bar should have comment (revive)
`,
			// Same rule+file pair → deduplicated to 1 finding.
			wantLen:  1,
			wantRule: "revive",
			wantFile: "service.go",
		},
		{
			name: "non-go line mixed with go line",
			input: `README.md:1:1: something (linter)
foo.go:3:3: unused variable (unused)
`,
			wantLen:  1,
			wantRule: "unused",
			wantFile: "foo.go",
		},
		{
			name:    "blank and info lines produce no findings",
			input:   "\n\nINFO [config_reader] Linters are:\n\n",
			wantLen: 0,
		},
		{
			name: "staticcheck rule prefix extracted from body",
			input: `repo.go:55:2: SA4006: this value of 'ctx' is never used (staticcheck)
`,
			wantLen:  1,
			wantRule: "SA4006",
			wantFile: "repo.go",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			findings := parseLintOutput(tc.input)
			if len(findings) != tc.wantLen {
				t.Fatalf("len(findings): want %d, got %d  findings=%+v", tc.wantLen, len(findings), findings)
			}
			if tc.wantLen == 0 {
				return
			}
			f := findings[0]
			if tc.wantRule != "" && f.rule != tc.wantRule {
				t.Errorf("rule: want %q, got %q", tc.wantRule, f.rule)
			}
			if tc.wantFile != "" && f.file != tc.wantFile {
				t.Errorf("file: want %q, got %q", tc.wantFile, f.file)
			}
			if tc.wantMessage != "" && !strings.Contains(f.message, tc.wantMessage) {
				t.Errorf("message: want substring %q in %q", tc.wantMessage, f.message)
			}
		})
	}
}

// ── TestParseDiff ─────────────────────────────────────────────────────────────

func TestParseDiff(t *testing.T) {
	cases := []struct {
		name         string
		diff         string
		wantHunks    int
		wantFile     string
		wantDontCode string // substring expected in dontCode
		wantDoCode   string // substring expected in doCode
	}{
		{
			name: "simple errcheck fix",
			diff: `diff --git a/handler.go b/handler.go
index abc..def 100644
--- a/handler.go
+++ b/handler.go
@@ -40,7 +40,7 @@
 func closeDB(db *sql.DB) {
-	db.Close()
+	if err := db.Close(); err != nil {
+		log.Printf("close db: %v", err)
+	}
 }
`,
			wantHunks:    1,
			wantFile:     "handler.go",
			wantDontCode: "db.Close()",
			wantDoCode:   "if err := db.Close(); err != nil {",
		},
		{
			name: "non-go file excluded",
			diff: `diff --git a/README.md b/README.md
--- a/README.md
+++ b/README.md
@@ -1 +1 @@
-old text
+new text
`,
			wantHunks: 0,
		},
		{
			name: "comment-only change produces empty snippets",
			diff: `diff --git a/util.go b/util.go
--- a/util.go
+++ b/util.go
@@ -5,4 +5,4 @@
-// bad comment
+// good comment
`,
			wantHunks:    1,
			wantFile:     "util.go",
			wantDontCode: "",
			wantDoCode:   "",
		},
		{
			name: "multiple files produce multiple hunks",
			diff: `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,2 +1,2 @@
-badA()
+goodA()
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,2 +1,2 @@
-badB()
+goodB()
`,
			wantHunks: 2,
		},
		{
			name:      "empty diff returns no hunks",
			diff:      "",
			wantHunks: 0,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			hunks := parseDiff(tc.diff)
			if len(hunks) != tc.wantHunks {
				t.Fatalf("len(hunks): want %d, got %d  hunks=%+v", tc.wantHunks, len(hunks), hunks)
			}
			if tc.wantHunks == 0 {
				return
			}
			h := hunks[0]
			if tc.wantFile != "" && h.file != tc.wantFile {
				t.Errorf("file: want %q, got %q", tc.wantFile, h.file)
			}
			if tc.wantDontCode != "" && !strings.Contains(h.dontCode, tc.wantDontCode) {
				t.Errorf("dontCode: want substring %q in %q", tc.wantDontCode, h.dontCode)
			}
			if tc.wantDoCode != "" && !strings.Contains(h.doCode, tc.wantDoCode) {
				t.Errorf("doCode: want substring %q in %q", tc.wantDoCode, h.doCode)
			}
		})
	}
}

// ── TestLearnFromLintTool ─────────────────────────────────────────────────────

func TestLearnFromLintTool(t *testing.T) {
	const lintOut = `handler.go:42:5: Error return value of 'db.Close' is not checked (errcheck)
`
	const diff = `diff --git a/handler.go b/handler.go
--- a/handler.go
+++ b/handler.go
@@ -40,7 +40,9 @@
 func closeDB(db *sql.DB) {
-	db.Close()
+	if err := db.Close(); err != nil {
+		log.Printf("close db: %v", err)
+	}
 }
`

	cases := []struct {
		name        string
		diff        string
		lintOutput  string
		project     string
		wantLearned int
		wantUpdated int
		wantRule    string // if non-empty, must appear in DB after the call
		wantGlob    string // if non-empty, the pattern's file_glob must match
	}{
		{
			name:        "full match: lint finding paired with diff hunk",
			diff:        diff,
			lintOutput:  lintOut,
			project:     "myproject",
			wantLearned: 1,
			wantUpdated: 0,
			wantRule:    "errcheck",
			wantGlob:    "*_handler.go",
		},
		{
			name:        "both inputs empty returns zero counts",
			diff:        "",
			lintOutput:  "",
			wantLearned: 0,
			wantUpdated: 0,
		},
		{
			name: "lint finding with no matching diff file stored as signal pattern",
			diff: `diff --git a/other.go b/other.go
--- a/other.go
+++ b/other.go
@@ -1 +1 @@
-x := 1
+x := 2
`,
			lintOutput:  lintOut, // mentions handler.go, not other.go
			wantLearned: 0,
			wantUpdated: 1,
			wantRule:    "errcheck",
		},
		{
			name: "test file gets *_test.go glob",
			diff: `diff --git a/foo_test.go b/foo_test.go
--- a/foo_test.go
+++ b/foo_test.go
@@ -1 +1 @@
-badAssert()
+require.NoError(t, err)
`,
			lintOutput: `foo_test.go:10:3: assertion should use require (testify)
`,
			wantLearned: 1,
			wantUpdated: 0,
			wantRule:    "testify",
			wantGlob:    "*_test.go",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)
			out := callLearnTool(t, store, tc.diff, tc.lintOutput, tc.project)

			gotLearned := int(out["learned"].(float64))
			gotUpdated := int(out["updated"].(float64))

			if gotLearned != tc.wantLearned {
				t.Errorf("learned: want %d, got %d", tc.wantLearned, gotLearned)
			}
			if gotUpdated != tc.wantUpdated {
				t.Errorf("updated: want %d, got %d", tc.wantUpdated, gotUpdated)
			}

			if tc.wantRule == "" {
				return
			}

			// Verify the pattern landed in the DB — query with empty glob to match all.
			patterns, err := store.QueryPatterns("", "", 20)
			if err != nil {
				t.Fatalf("QueryPatterns: %v", err)
			}
			found := false
			for _, p := range patterns {
				if p.Rule == tc.wantRule {
					found = true
					if tc.wantGlob != "" && p.FileGlob != tc.wantGlob {
						t.Errorf("file_glob: want %q, got %q", tc.wantGlob, p.FileGlob)
					}
				}
			}
			if !found {
				t.Errorf("rule %q not found in DB; all patterns: %+v", tc.wantRule, patterns)
			}
		})
	}
}

// ── TestFileGlobFor ───────────────────────────────────────────────────────────

func TestFileGlobFor(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"handler.go", "*_handler.go"},
		{"user_handler.go", "*_handler.go"},
		{"foo_test.go", "*_test.go"},
		{"auth_middleware.go", "*_middleware.go"},
		{"middleware.go", "*.go"}, // stem "middleware" has no domain suffix
		{"server.go", "*_server.go"},
		{"main.go", "*.go"},
		{"", "*.go"},
		{"repo.go", "*_repo.go"},
		{"db_mock.go", "*_mock.go"},
		{"mock_db.go", "*.go"}, // stem ends in "_db", not "_mock"
	}

	for _, tc := range cases {
		t.Run(tc.base, func(t *testing.T) {
			got := fileGlobFor(tc.base)
			if got != tc.want {
				t.Errorf("fileGlobFor(%q): want %q, got %q", tc.base, tc.want, got)
			}
		})
	}
}
