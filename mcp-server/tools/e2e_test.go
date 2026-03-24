package tools

import (
	"context"
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

// TestE2ELearnQuerySuggest exercises the full learning loop:
//
//  1. learn_from_lint stores a DON'T/DO pattern
//  2. query_knowledge retrieves it for the same file type
//  3. suggest_fix matches it when given the bad snippet
func TestE2ELearnQuerySuggest(t *testing.T) {
	store := newTestStore(t)

	// -- Step 1: learn --------------------------------------------------------
	const lintOut = `utils.go:22:5: Error return value of 'rows.Close' is not checked (errcheck)
`
	const diff = `diff --git a/utils.go b/utils.go
--- a/utils.go
+++ b/utils.go
@@ -20,5 +20,7 @@
 func fetchRows(db *sql.DB) {
-	rows, _ := db.Query("SELECT id FROM users")
-	rows.Close()
+	rows, err := db.Query("SELECT id FROM users")
+	if err != nil {
+		return err
+	}
+	defer rows.Close()
 }
`
	learnResult := callLearnTool(t, store, diff, lintOut, "e2e-project")

	if int(learnResult["learned"].(float64)) < 1 {
		t.Fatalf("expected at least 1 learned pattern, got %v", learnResult["learned"])
	}

	// -- Step 2: query_knowledge ----------------------------------------------
	queryReq := mcp.CallToolRequest{}
	queryReq.Params.Arguments = map[string]interface{}{
		"file_path":    "utils.go",
		"code_context": "rows.Close()",
	}

	queryResult, err := handleQueryKnowledge(context.Background(), queryReq, store)
	if err != nil {
		t.Fatalf("handleQueryKnowledge error: %v", err)
	}
	if queryResult == "" {
		t.Fatal("handleQueryKnowledge returned empty result")
	}
	if !strings.Contains(queryResult, "errcheck") {
		t.Errorf("query result should mention 'errcheck', got: %s", queryResult)
	}

	// -- Step 3: suggest_fix --------------------------------------------------
	matches, err := findMatches(store, "rows.Close()", "")
	if err != nil {
		t.Fatalf("findMatches error: %v", err)
	}
	if len(matches) == 0 {
		t.Fatal("suggest_fix returned no matches for a pattern that was just learned")
	}

	formatted := formatSuggestMatches(matches)
	if !strings.Contains(formatted, "errcheck") {
		t.Errorf("suggest_fix output should mention 'errcheck', got: %s", formatted)
	}
	if !strings.Contains(formatted, "rows.Close") {
		t.Errorf("suggest_fix output should mention 'rows.Close', got: %s", formatted)
	}
}

// TestE2EStalenessAfterScan verifies that after UpdateScanHistory the
// checkStaleness function no longer reports that scan type as stale.
func TestE2EStalenessAfterScan(t *testing.T) {
	store := newTestStore(t)
	projectPath := t.TempDir()

	// Before any scan: vuln, owasp, and full should be stale (never run).
	result1, err := checkStaleness(store, projectPath)
	if err != nil {
		t.Fatalf("checkStaleness (before): %v", err)
	}

	// lint has threshold=0, so it should never appear as stale.
	if strings.Contains(result1, "lint scan") && strings.Contains(result1, "never run") {
		t.Error("lint should never be reported as stale")
	}

	// vuln should be stale (never run).
	if !strings.Contains(result1, "never run") {
		t.Errorf("expected at least one 'never run' scan before any history, got:\n%s", result1)
	}

	// Record a vuln scan now, using the ProjectID that checkStaleness will
	// derive from projectPath.
	projectID := ProjectID(projectPath)
	if err := store.UpdateScanHistory("vuln", projectID, 0); err != nil {
		t.Fatalf("UpdateScanHistory: %v", err)
	}

	// After recording vuln, it should no longer say "never run" for vuln.
	result2, err := checkStaleness(store, projectPath)
	if err != nil {
		t.Fatalf("checkStaleness (after): %v", err)
	}

	// Parse each line: vuln should now show a checkmark, not a warning.
	for _, line := range strings.Split(result2, "\n") {
		if strings.Contains(line, "vuln") && strings.Contains(line, "never run") {
			t.Errorf("vuln should be fresh after just running, but got: %s", line)
		}
	}

	// Record owasp, owasp_rules, and full scans too, then everything should be current.
	for _, scanType := range []string{"owasp", "owasp_rules", "full"} {
		if err := store.UpdateScanHistory(scanType, projectID, 0); err != nil {
			t.Fatalf("UpdateScanHistory(%s): %v", scanType, err)
		}
	}

	result3, err := checkStaleness(store, projectPath)
	if err != nil {
		t.Fatalf("checkStaleness (all current): %v", err)
	}
	if !strings.Contains(result3, "All scans current") {
		t.Errorf("expected 'All scans current' after recording all scans, got:\n%s", result3)
	}
}
