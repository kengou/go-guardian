package tools

import (
	"strings"
	"testing"
)

// TestSuggestFixNoMatch verifies that a snippet with no DB match returns the
// "No matching patterns" fallback message.
func TestSuggestFixNoMatch(t *testing.T) {
	store := newTestStore(t)

	// The store is empty of lint patterns; anti-patterns may be seeded but
	// their dont_code won't match this invented snippet.
	matches, err := findMatches(store, "thisSnippetWillNeverMatchAnything_xyzzy_42", "")
	if err != nil {
		t.Fatalf("findMatches: %v", err)
	}
	if len(matches) != 0 {
		t.Errorf("expected 0 matches for unrecognised snippet, got %d", len(matches))
	}
}

// TestSuggestFixExactMatch seeds a lint pattern whose dont_code is a substring
// of the provided snippet, then verifies that pattern's do_code appears in the
// formatted output.
func TestSuggestFixExactMatch(t *testing.T) {
	store := newTestStore(t)

	dontCode := "f, err := os.Open(path)\nreturn f, nil"
	doCode := "f, err := os.Open(path)\nif err != nil {\n    return nil, fmt.Errorf(\"open %s: %w\", path, err)\n}\nreturn f, nil"

	if err := store.InsertLintPattern("errcheck", "*.go", dontCode, doCode, "learned"); err != nil {
		t.Fatalf("InsertLintPattern: %v", err)
	}

	// The snippet contains both lines of dontCode verbatim.
	snippet := "func openFile(path string) (*os.File, error) {\n" +
		"    f, err := os.Open(path)\n" +
		"    return f, nil\n" +
		"}"

	matches, err := findMatches(store, snippet, "lint")
	if err != nil {
		t.Fatalf("findMatches: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("expected at least 1 match, got 0")
	}

	// Verify the top match is the seeded errcheck pattern.
	top := matches[0]
	if !strings.Contains(top.rule, "errcheck") {
		t.Errorf("expected errcheck rule, got %q", top.rule)
	}
	if top.doCode != doCode {
		t.Errorf("do_code mismatch:\nwant: %q\n got: %q", doCode, top.doCode)
	}

	// Verify the formatted output contains the DO code.
	text := formatSuggestMatches(matches)
	if !strings.Contains(text, "Suggested fixes") {
		t.Errorf("missing header in formatted output:\n%s", text)
	}
	if !strings.Contains(text, "DON'T:") {
		t.Errorf("missing DON'T section:\n%s", text)
	}
	if !strings.Contains(text, "DO:") {
		t.Errorf("missing DO section:\n%s", text)
	}
	if !strings.Contains(text, "fmt.Errorf") {
		t.Errorf("expected do_code content in output:\n%s", text)
	}
}

// TestSuggestFixIssueTypeFilter verifies that the issue_type parameter scopes
// results: seeding a lint pattern and querying with issue_type="pattern" must
// return no lint matches.
func TestSuggestFixIssueTypeFilter(t *testing.T) {
	store := newTestStore(t)

	dontCode := "f, err := os.Open(path)\nreturn f, nil"
	doCode := "if err != nil { return nil, err }"

	if err := store.InsertLintPattern("errcheck", "*.go", dontCode, doCode, "learned"); err != nil {
		t.Fatalf("InsertLintPattern: %v", err)
	}

	// Querying with issue_type="pattern" should not return lint_patterns.
	snippet := "f, err := os.Open(path)\nreturn f, nil"
	matches, err := findMatches(store, snippet, "pattern")
	if err != nil {
		t.Fatalf("findMatches: %v", err)
	}
	for _, m := range matches {
		if strings.HasPrefix(m.rule, "lint:") {
			t.Errorf("issue_type=pattern should not return lint match, got %q", m.rule)
		}
	}
}

// TestSnippetSimilarity is a unit test for the snippetSimilarity helper.
func TestSnippetSimilarity(t *testing.T) {
	tests := []struct {
		name      string
		snippet   string
		dontCode  string
		wantMin   float64
		wantMax   float64
	}{
		{
			name:     "exact full match",
			snippet:  "f, err := os.Open(path)\nreturn f, nil",
			dontCode: "f, err := os.Open(path)\nreturn f, nil",
			wantMin:  1.0,
			wantMax:  1.0,
		},
		{
			name:     "snippet contains all dont lines",
			snippet:  "func foo() {\n    f, err := os.Open(path)\n    return f, nil\n}",
			dontCode: "f, err := os.Open(path)\nreturn f, nil",
			wantMin:  1.0,
			wantMax:  1.0,
		},
		{
			name:     "partial match — one of two non-trivial lines present",
			snippet:  "f, err := os.Open(path)",
			dontCode: "f, err := os.Open(path)\nreturn f, nil",
			wantMin:  0.49,
			wantMax:  0.51,
		},
		{
			name:     "no match",
			snippet:  "x := 42",
			dontCode: "f, err := os.Open(path)\nreturn f, nil",
			wantMin:  0.0,
			wantMax:  0.0,
		},
		{
			name:     "empty snippet",
			snippet:  "",
			dontCode: "f, err := os.Open(path)",
			wantMin:  0.0,
			wantMax:  0.0,
		},
		{
			name:     "empty dontCode",
			snippet:  "f, err := os.Open(path)",
			dontCode: "",
			wantMin:  0.0,
			wantMax:  0.0,
		},
		{
			name:     "trivial-only dontCode (all short/punct lines)",
			snippet:  "anything goes here in the snippet",
			dontCode: "{}\n//comment\n}",
			wantMin:  0.0,
			wantMax:  0.0,
		},
		{
			name:     "case insensitive match",
			snippet:  "F, ERR := OS.OPEN(PATH)\nRETURN F, NIL",
			dontCode: "f, err := os.Open(path)\nreturn f, nil",
			wantMin:  1.0,
			wantMax:  1.0,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := snippetSimilarity(tc.snippet, tc.dontCode)
			if got < tc.wantMin || got > tc.wantMax {
				t.Errorf("snippetSimilarity(%q, %q) = %.4f, want [%.4f, %.4f]",
					tc.snippet, tc.dontCode, got, tc.wantMin, tc.wantMax)
			}
		})
	}
}

// TestSnippetSimilarityNonTrivialFilter verifies that isNonTrivialLine
// correctly skips short, comment, and punctuation-only lines.
func TestSnippetSimilarityNonTrivialFilter(t *testing.T) {
	tests := []struct {
		line string
		want bool
	}{
		{"f, err := os.Open(path)", true},
		{"return f, nil", true},
		{"// this is a comment", false},
		{"{}", false},
		{"}", false},
		{"abc", false}, // only 3 chars — below threshold
		{"abcd", true}, // exactly 4 chars
		{"  ", false},
		{"", false},
	}

	for _, tc := range tests {
		t.Run(tc.line, func(t *testing.T) {
			got := isNonTrivialLine(tc.line)
			if got != tc.want {
				t.Errorf("isNonTrivialLine(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}
