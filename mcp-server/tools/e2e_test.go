package tools

import (
	"strings"
	"testing"
)

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
