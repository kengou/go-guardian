package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestGetSessionFindingsEmpty(t *testing.T) {
	store := newTestStore(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}

	text, err := handleGetSessionFindings(store, "sess-100", req)
	if err != nil {
		t.Fatalf("handleGetSessionFindings: %v", err)
	}
	if !strings.Contains(text, "No session findings") {
		t.Errorf("expected empty message, got: %q", text)
	}
}

func TestGetSessionFindingsNoSession(t *testing.T) {
	store := newTestStore(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}

	text, err := handleGetSessionFindings(store, "", req)
	if err != nil {
		t.Fatalf("handleGetSessionFindings: %v", err)
	}
	if !strings.Contains(text, "No active session") {
		t.Errorf("expected no-session message, got: %q", text)
	}
}

func TestGetSessionFindingsFormatting(t *testing.T) {
	store := newTestStore(t)
	sid := "sess-200"

	_, _ = store.InsertSessionFinding(sid, "reviewer", "race-condition", "service.go", "Map access unsync", "HIGH")
	_, _ = store.InsertSessionFinding(sid, "security", "sql-injection", "db.go", "String concat in query", "CRITICAL")
	_, _ = store.InsertSessionFinding(sid, "linter", "errcheck", "handler.go", "Unchecked error", "MEDIUM")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{}

	text, err := handleGetSessionFindings(store, sid, req)
	if err != nil {
		t.Fatalf("handleGetSessionFindings: %v", err)
	}

	for _, substr := range []string{
		"Session Findings (3)",
		"[HIGH] reviewer",
		"[CRITICAL] security",
		"[MEDIUM] linter",
		"race-condition",
		"service.go",
	} {
		if !strings.Contains(text, substr) {
			t.Errorf("output missing %q:\n%s", substr, text)
		}
	}
}

func TestGetSessionFindingsFilterByAgent(t *testing.T) {
	store := newTestStore(t)
	sid := "sess-300"

	_, _ = store.InsertSessionFinding(sid, "reviewer", "bug", "", "Bug found", "MEDIUM")
	_, _ = store.InsertSessionFinding(sid, "security", "vuln", "", "Vuln found", "HIGH")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"agent": "reviewer"}

	text, err := handleGetSessionFindings(store, sid, req)
	if err != nil {
		t.Fatalf("handleGetSessionFindings: %v", err)
	}

	if !strings.Contains(text, "reviewer") {
		t.Errorf("expected reviewer finding: %q", text)
	}
	if strings.Contains(text, "security") {
		t.Errorf("security should not appear in reviewer-filtered output: %q", text)
	}
}

func TestGetSessionFindingsFilterByFile(t *testing.T) {
	store := newTestStore(t)
	sid := "sess-400"

	_, _ = store.InsertSessionFinding(sid, "reviewer", "bug", "service.go", "Bug in service", "MEDIUM")
	_, _ = store.InsertSessionFinding(sid, "security", "vuln", "handler.go", "Vuln in handler", "HIGH")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"file_path": "service"}

	text, err := handleGetSessionFindings(store, sid, req)
	if err != nil {
		t.Fatalf("handleGetSessionFindings: %v", err)
	}

	if !strings.Contains(text, "service.go") {
		t.Errorf("expected service.go finding: %q", text)
	}
	if strings.Contains(text, "handler.go") {
		t.Errorf("handler.go should not appear in service-filtered output: %q", text)
	}
}

func TestGetSessionFindingsFilterByType(t *testing.T) {
	store := newTestStore(t)
	sid := "sess-500"

	_, _ = store.InsertSessionFinding(sid, "reviewer", "race-condition", "", "Race found", "HIGH")
	_, _ = store.InsertSessionFinding(sid, "linter", "errcheck", "", "Error unchecked", "MEDIUM")

	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{"finding_type": "race-condition"}

	text, err := handleGetSessionFindings(store, sid, req)
	if err != nil {
		t.Fatalf("handleGetSessionFindings: %v", err)
	}

	if !strings.Contains(text, "race-condition") {
		t.Errorf("expected race-condition: %q", text)
	}
	if strings.Contains(text, "errcheck") {
		t.Errorf("errcheck should not appear: %q", text)
	}
}
