package tools

import (
	"strings"
	"testing"

	"github.com/mark3labs/mcp-go/mcp"
)

func TestReportFindingHappyPath(t *testing.T) {
	store := newTestStore(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"agent":        "reviewer",
		"finding_type": "race-condition",
		"file_path":    "service.go",
		"description":  "Unsynchronised map access in handler",
		"severity":     "HIGH",
	}

	text, err := handleReportFinding(store, "sess-001", req)
	if err != nil {
		t.Fatalf("handleReportFinding: %v", err)
	}
	if !strings.Contains(text, "Finding #") {
		t.Errorf("expected finding ID in output, got: %q", text)
	}
	if !strings.Contains(text, "reviewer/HIGH") {
		t.Errorf("expected agent/severity in output, got: %q", text)
	}

	// Verify it was stored.
	findings, err := store.GetSessionFindings("sess-001", "")
	if err != nil {
		t.Fatalf("GetSessionFindings: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	if findings[0].Agent != "reviewer" {
		t.Errorf("agent: want reviewer, got %q", findings[0].Agent)
	}
	if findings[0].Severity != "HIGH" {
		t.Errorf("severity: want HIGH, got %q", findings[0].Severity)
	}
}

func TestReportFindingDefaultSeverity(t *testing.T) {
	store := newTestStore(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"agent":        "linter",
		"finding_type": "errcheck",
		"description":  "Unchecked error return",
	}

	text, err := handleReportFinding(store, "sess-002", req)
	if err != nil {
		t.Fatalf("handleReportFinding: %v", err)
	}
	if !strings.Contains(text, "MEDIUM") {
		t.Errorf("expected default MEDIUM severity, got: %q", text)
	}
}

func TestReportFindingInvalidSeverity(t *testing.T) {
	store := newTestStore(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"agent":        "security",
		"finding_type": "sqli",
		"description":  "SQL injection risk",
		"severity":     "BOGUS",
	}

	text, err := handleReportFinding(store, "sess-003", req)
	if err != nil {
		t.Fatalf("handleReportFinding: %v", err)
	}
	// Invalid severity falls back to MEDIUM.
	if !strings.Contains(text, "MEDIUM") {
		t.Errorf("expected MEDIUM fallback, got: %q", text)
	}
}

func TestReportFindingMissingParams(t *testing.T) {
	store := newTestStore(t)
	cases := []struct {
		name string
		args map[string]interface{}
	}{
		{"missing agent", map[string]interface{}{"finding_type": "x", "description": "y"}},
		{"missing finding_type", map[string]interface{}{"agent": "a", "description": "y"}},
		{"missing description", map[string]interface{}{"agent": "a", "finding_type": "x"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req := mcp.CallToolRequest{}
			req.Params.Arguments = tc.args
			_, err := handleReportFinding(store, "sess-004", req)
			if err == nil {
				t.Error("expected error for missing params")
			}
		})
	}
}

func TestReportFindingNoSession(t *testing.T) {
	store := newTestStore(t)
	req := mcp.CallToolRequest{}
	req.Params.Arguments = map[string]interface{}{
		"agent":        "reviewer",
		"finding_type": "bug",
		"description":  "something",
	}

	_, err := handleReportFinding(store, "", req)
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}
