package tools

import (
	"strings"
	"testing"
)

func TestReportFindingHappyPath(t *testing.T) {
	store := newTestStore(t)

	text, err := RunReportFinding(store, "sess-001", "reviewer", "race-condition", "service.go", "Unsynchronised map access in handler", "HIGH")
	if err != nil {
		t.Fatalf("RunReportFinding: %v", err)
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

	text, err := RunReportFinding(store, "sess-002", "linter", "errcheck", "", "Unchecked error return", "")
	if err != nil {
		t.Fatalf("RunReportFinding: %v", err)
	}
	if !strings.Contains(text, "MEDIUM") {
		t.Errorf("expected default MEDIUM severity, got: %q", text)
	}
}

func TestReportFindingInvalidSeverity(t *testing.T) {
	store := newTestStore(t)

	text, err := RunReportFinding(store, "sess-003", "security", "sqli", "", "SQL injection risk", "BOGUS")
	if err != nil {
		t.Fatalf("RunReportFinding: %v", err)
	}
	// Invalid severity falls back to MEDIUM.
	if !strings.Contains(text, "MEDIUM") {
		t.Errorf("expected MEDIUM fallback, got: %q", text)
	}
}

func TestReportFindingMissingParams(t *testing.T) {
	store := newTestStore(t)
	cases := []struct {
		name        string
		agent       string
		findingType string
		description string
	}{
		{"missing agent", "", "x", "y"},
		{"missing finding_type", "a", "", "y"},
		{"missing description", "a", "x", ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := RunReportFinding(store, "sess-004", tc.agent, tc.findingType, "", tc.description, "")
			if err == nil {
				t.Error("expected error for missing params")
			}
		})
	}
}

func TestReportFindingNoSession(t *testing.T) {
	store := newTestStore(t)

	_, err := RunReportFinding(store, "", "reviewer", "bug", "", "something", "")
	if err == nil {
		t.Error("expected error for empty session ID")
	}
}
