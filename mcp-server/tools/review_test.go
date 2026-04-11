package tools

import (
	"encoding/json"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// callReviewTool invokes RunLearnFromReview directly and returns the decoded
// JSON result map. The second return value is true if the call produced an
// error, in which case the map has an "error" key with the error message.
func callReviewTool(t *testing.T, store *db.Store, args map[string]interface{}) (map[string]interface{}, bool) {
	t.Helper()

	getStr := func(key string) string {
		if v, ok := args[key].(string); ok {
			return v
		}
		return ""
	}

	result, err := RunLearnFromReview(
		store,
		getStr("description"),
		getStr("severity"),
		getStr("category"),
		getStr("dont_code"),
		getStr("do_code"),
		getStr("file_path"),
	)
	if err != nil {
		return map[string]interface{}{"error": err.Error()}, true
	}

	var out map[string]interface{}
	if err := json.Unmarshal([]byte(result), &out); err != nil {
		t.Fatalf("unmarshal result JSON %q: %v", result, err)
	}
	return out, false
}

func TestLearnFromReview(t *testing.T) {
	cases := []struct {
		name            string
		args            map[string]interface{}
		wantError       bool
		wantErrorSubstr string
		wantStored      bool
		wantRule        string
		wantGlob        string
		wantAntiPattern bool
	}{
		{
			name: "happy path: MEDIUM severity stores lint pattern only",
			args: map[string]interface{}{
				"description": "bare error return without context",
				"severity":    "MEDIUM",
				"category":    "error-handling",
				"dont_code":   "return err",
				"do_code":     "return fmt.Errorf(\"load config: %w\", err)",
				"file_path":   "handler.go",
			},
			wantStored:      true,
			wantRule:        "review:error-handling",
			wantGlob:        "*_handler.go",
			wantAntiPattern: false,
		},
		{
			name: "HIGH severity also creates anti-pattern",
			args: map[string]interface{}{
				"description": "race condition on shared map",
				"severity":    "HIGH",
				"category":    "concurrency",
				"dont_code":   "m[key] = value // no mutex",
				"do_code":     "mu.Lock(); m[key] = value; mu.Unlock()",
				"file_path":   "service.go",
			},
			wantStored:      true,
			wantRule:        "review:concurrency",
			wantGlob:        "*_service.go",
			wantAntiPattern: true,
		},
		{
			name: "CRITICAL severity also creates anti-pattern",
			args: map[string]interface{}{
				"description": "SQL injection via fmt.Sprintf",
				"severity":    "CRITICAL",
				"category":    "security",
				"dont_code":   "db.Query(fmt.Sprintf(\"SELECT * FROM users WHERE id=%s\", id))",
				"do_code":     "db.Query(\"SELECT * FROM users WHERE id=?\", id)",
				"file_path":   "repo.go",
			},
			wantStored:      true,
			wantRule:        "review:security",
			wantGlob:        "*_repo.go",
			wantAntiPattern: true,
		},
		{
			name: "no file_path defaults to *.go",
			args: map[string]interface{}{
				"description": "unused variable",
				"severity":    "LOW",
				"category":    "general",
				"dont_code":   "x := compute()",
				"do_code":     "_ = compute()",
			},
			wantStored:      true,
			wantRule:        "review:general",
			wantGlob:        "*.go",
			wantAntiPattern: false,
		},
		{
			name: "test file gets *_test.go glob",
			args: map[string]interface{}{
				"description": "missing t.Helper in test helper",
				"severity":    "MEDIUM",
				"category":    "testing",
				"dont_code":   "func assertOK(t *testing.T, err error) {",
				"do_code":     "func assertOK(t *testing.T, err error) { t.Helper()",
				"file_path":   "user_test.go",
			},
			wantStored:      true,
			wantRule:        "review:testing",
			wantGlob:        "*_test.go",
			wantAntiPattern: false,
		},
		{
			name:            "missing description returns error",
			args:            map[string]interface{}{"severity": "HIGH", "category": "general", "dont_code": "x", "do_code": "y"},
			wantError:       true,
			wantErrorSubstr: "description",
		},
		{
			name:            "missing severity returns error",
			args:            map[string]interface{}{"description": "d", "category": "general", "dont_code": "x", "do_code": "y"},
			wantError:       true,
			wantErrorSubstr: "severity",
		},
		{
			name:            "missing category returns error",
			args:            map[string]interface{}{"description": "d", "severity": "HIGH", "dont_code": "x", "do_code": "y"},
			wantError:       true,
			wantErrorSubstr: "category",
		},
		{
			name:            "missing dont_code returns error",
			args:            map[string]interface{}{"description": "d", "severity": "HIGH", "category": "general", "do_code": "y"},
			wantError:       true,
			wantErrorSubstr: "dont_code",
		},
		{
			name:            "missing do_code returns error",
			args:            map[string]interface{}{"description": "d", "severity": "HIGH", "category": "general", "dont_code": "x"},
			wantError:       true,
			wantErrorSubstr: "do_code",
		},
		{
			name:            "invalid severity returns error",
			args:            map[string]interface{}{"description": "d", "severity": "EXTREME", "category": "general", "dont_code": "x", "do_code": "y"},
			wantError:       true,
			wantErrorSubstr: "invalid severity",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newTestStore(t)

			out, isErr := callReviewTool(t, store, tc.args)

			if tc.wantError {
				if !isErr {
					t.Fatalf("expected error, got success: %+v", out)
				}
				if tc.wantErrorSubstr != "" {
					errMsg, _ := out["error"].(string)
					if errMsg == "" || !contains(errMsg, tc.wantErrorSubstr) {
						t.Errorf("error %q does not contain %q", errMsg, tc.wantErrorSubstr)
					}
				}
				return
			}
			if isErr {
				t.Fatalf("unexpected error: %+v", out)
			}

			if stored, _ := out["stored"].(bool); stored != tc.wantStored {
				t.Errorf("stored: want %v, got %v", tc.wantStored, stored)
			}
			if rule, _ := out["rule"].(string); rule != tc.wantRule {
				t.Errorf("rule: want %q, got %q", tc.wantRule, rule)
			}
			if glob, _ := out["file_glob"].(string); glob != tc.wantGlob {
				t.Errorf("file_glob: want %q, got %q", tc.wantGlob, glob)
			}
			if ap, _ := out["also_anti_pattern"].(bool); ap != tc.wantAntiPattern {
				t.Errorf("also_anti_pattern: want %v, got %v", tc.wantAntiPattern, ap)
			}

			// Verify the pattern landed in the DB.
			patterns, err := store.QueryPatterns("", "", 50)
			if err != nil {
				t.Fatalf("QueryPatterns: %v", err)
			}
			found := false
			for _, p := range patterns {
				if p.Rule == tc.wantRule && p.Source == "review" {
					found = true
					if p.FileGlob != tc.wantGlob {
						t.Errorf("DB file_glob: want %q, got %q", tc.wantGlob, p.FileGlob)
					}
				}
			}
			if !found {
				t.Errorf("rule %q with source=review not found in DB", tc.wantRule)
			}

			// Verify anti-pattern if expected.
			if tc.wantAntiPattern {
				aps, err := store.QueryAntiPatterns(tc.args["category"].(string))
				if err != nil {
					t.Fatalf("QueryAntiPatterns: %v", err)
				}
				apFound := false
				for _, ap := range aps {
					if ap.Source == "review" {
						apFound = true
					}
				}
				if !apFound {
					t.Errorf("anti-pattern with source=review not found for category %q", tc.args["category"])
				}
			}
		})
	}
}

func TestLearnFromReviewDedup(t *testing.T) {
	store := newTestStore(t)

	args := map[string]interface{}{
		"description": "bare error return",
		"severity":    "MEDIUM",
		"category":    "error-handling",
		"dont_code":   "return err",
		"do_code":     "return fmt.Errorf(\"op: %w\", err)",
		"file_path":   "handler.go",
	}

	// Call twice with same params.
	callReviewTool(t, store, args)
	callReviewTool(t, store, args)

	// Verify frequency incremented.
	patterns, err := store.QueryPatterns("*_handler.go", "", 10)
	if err != nil {
		t.Fatalf("QueryPatterns: %v", err)
	}
	for _, p := range patterns {
		if p.Rule == "review:error-handling" {
			if p.Frequency != 2 {
				t.Errorf("frequency: want 2, got %d", p.Frequency)
			}
			return
		}
	}
	t.Error("review:error-handling pattern not found")
}

func TestLearnFromReviewSnippetTruncation(t *testing.T) {
	store := newTestStore(t)

	// Create a dont_code snippet >500 chars.
	longCode := "x := " + string(make([]byte, 600))
	args := map[string]interface{}{
		"description": "long snippet",
		"severity":    "LOW",
		"category":    "general",
		"dont_code":   longCode,
		"do_code":     "short fix",
	}

	callReviewTool(t, store, args)

	patterns, err := store.QueryPatterns("", "", 10)
	if err != nil {
		t.Fatalf("QueryPatterns: %v", err)
	}
	for _, p := range patterns {
		if p.Rule == "review:general" {
			// 500 chars + "…" suffix = 501 or slightly more.
			if len(p.DontCode) > 510 {
				t.Errorf("dont_code not truncated: len=%d", len(p.DontCode))
			}
			return
		}
	}
	t.Error("review:general pattern not found")
}

func TestReviewPatternID(t *testing.T) {
	id1 := reviewPatternID("return err")
	id2 := reviewPatternID("return err")
	id3 := reviewPatternID("return fmt.Errorf(\"op: %w\", err)")

	if id1 != id2 {
		t.Errorf("same input should produce same ID: %q != %q", id1, id2)
	}
	if id1 == id3 {
		t.Error("different inputs should produce different IDs")
	}
	if len(id1) < 4 || id1[:4] != "REV-" {
		t.Errorf("pattern ID should start with REV-: %q", id1)
	}
}

// contains checks if s contains substr (case-sensitive).
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsHelper(s, substr))
}

func containsHelper(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
