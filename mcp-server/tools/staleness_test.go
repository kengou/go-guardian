//go:build testing

package tools

import (
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestProjectID — verify path normalisation
// ---------------------------------------------------------------------------

func TestProjectID(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"/home/user/projects/myapp", "projects/myapp"},
		{"/home/user/myapp/", "user/myapp"},
		{"/myapp", "myapp"},
		{"/a/b/c/d", "c/d"},
		{"myapp", "myapp"},
		{"/foo/bar", "foo/bar"},
	}
	for _, tc := range cases {
		got := ProjectID(tc.input)
		if got != tc.want {
			t.Errorf("ProjectID(%q) = %q, want %q", tc.input, got, tc.want)
		}
	}
}

// ---------------------------------------------------------------------------
// TestAllCurrent — recent scans for all types → "All scans current"
// ---------------------------------------------------------------------------

func TestAllCurrent(t *testing.T) {
	store := newTestStore(t)
	project := "testorg/myapp"

	// Insert scans well within their thresholds.
	for _, scanType := range []string{"vuln", "owasp", "owasp_rules", "full"} {
		if err := store.UpdateScanHistory(scanType, project, 0); err != nil {
			t.Fatalf("UpdateScanHistory %q: %v", scanType, err)
		}
	}

	result, err := checkStaleness(store, "/some/path/testorg/myapp")
	if err != nil {
		t.Fatalf("checkStaleness: %v", err)
	}

	if !strings.Contains(result, "All scans current") {
		t.Errorf("expected 'All scans current', got:\n%s", result)
	}
	if strings.Contains(result, "never run") {
		t.Errorf("unexpected 'never run' in result:\n%s", result)
	}
}

// ---------------------------------------------------------------------------
// TestStaleScan — vuln scan from 5 days ago → staleness warning
// ---------------------------------------------------------------------------

func TestStaleScan(t *testing.T) {
	store := newTestStore(t)
	project := "testorg/myapp"

	// Insert a vuln scan and backdate it to 5 days ago (threshold is 3 days).
	if err := store.UpdateScanHistory("vuln", project, 0); err != nil {
		t.Fatalf("UpdateScanHistory vuln: %v", err)
	}
	past := time.Now().Add(-5 * 24 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	if err := store.SetLastRunForTest("vuln", project, past); err != nil {
		t.Fatalf("SetLastRunForTest: %v", err)
	}

	// owasp, owasp_rules, and full: fresh.
	for _, scanType := range []string{"owasp", "owasp_rules", "full"} {
		if err := store.UpdateScanHistory(scanType, project, 0); err != nil {
			t.Fatalf("UpdateScanHistory %q: %v", scanType, err)
		}
	}

	result, err := checkStaleness(store, "/some/path/testorg/myapp")
	if err != nil {
		t.Fatalf("checkStaleness: %v", err)
	}

	if !strings.Contains(result, "Stale scans detected") {
		t.Errorf("expected 'Stale scans detected', got:\n%s", result)
	}
	if !strings.Contains(result, "vuln scan") {
		t.Errorf("expected 'vuln scan' in result:\n%s", result)
	}
	if !strings.Contains(result, "5 days ago") {
		t.Errorf("expected '5 days ago' in result:\n%s", result)
	}
	if !strings.Contains(result, "threshold: 3 days") {
		t.Errorf("expected 'threshold: 3 days' in result:\n%s", result)
	}
}

// ---------------------------------------------------------------------------
// TestNeverRun — empty scan_history → all three types appear as "never run"
// ---------------------------------------------------------------------------

func TestNeverRun(t *testing.T) {
	store := newTestStore(t)

	result, err := checkStaleness(store, "/some/path/testorg/myapp")
	if err != nil {
		t.Fatalf("checkStaleness: %v", err)
	}

	if !strings.Contains(result, "Stale scans detected") {
		t.Errorf("expected 'Stale scans detected', got:\n%s", result)
	}
	for _, scanType := range []string{"vuln", "owasp", "owasp_rules", "full"} {
		needle := scanType + " scan: never run"
		if !strings.Contains(result, needle) {
			t.Errorf("expected %q in result:\n%s", needle, result)
		}
	}
}

// ---------------------------------------------------------------------------
// TestLintNeverStale — lint is excluded from staleness checks entirely
// ---------------------------------------------------------------------------

func TestLintNeverStale(t *testing.T) {
	store := newTestStore(t)
	project := "testorg/myapp"

	// Only insert a lint scan (no vuln/owasp/full).
	if err := store.UpdateScanHistory("lint", project, 0); err != nil {
		t.Fatalf("UpdateScanHistory lint: %v", err)
	}

	result, err := checkStaleness(store, "/some/path/testorg/myapp")
	if err != nil {
		t.Fatalf("checkStaleness: %v", err)
	}

	// lint must not appear in the staleness output.
	if strings.Contains(result, "lint") {
		t.Errorf("lint should not appear in staleness output, got:\n%s", result)
	}

	// vuln, owasp, full are missing → should appear as "never run".
	if !strings.Contains(result, "Stale scans detected") {
		t.Errorf("expected 'Stale scans detected' because vuln/owasp/full are absent:\n%s", result)
	}
}
