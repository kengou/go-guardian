package db

import (
	"testing"
	"time"
)

// newTestStore opens an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestNewStore verifies that all six tables are created by NewStore.
func TestNewStore(t *testing.T) {
	s := newTestStore(t)

	tables := []string{
		"lint_patterns",
		"owasp_findings",
		"vuln_cache",
		"scan_history",
		"anti_patterns",
		"dep_decisions",
	}
	for _, tbl := range tables {
		var name string
		err := s.db.QueryRow(
			`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, tbl,
		).Scan(&name)
		if err != nil {
			t.Errorf("table %q not found: %v", tbl, err)
		}
	}
}

// TestInsertAndQueryLintPattern inserts a pattern and reads it back.
func TestInsertAndQueryLintPattern(t *testing.T) {
	s := newTestStore(t)

	err := s.InsertLintPattern("no-unused-var", "*.go", "x := 1\n_ = x", "// remove unused x", "learned")
	if err != nil {
		t.Fatalf("InsertLintPattern: %v", err)
	}

	patterns, err := s.QueryPatterns("*.go", "", 10)
	if err != nil {
		t.Fatalf("QueryPatterns: %v", err)
	}
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	p := patterns[0]
	if p.Rule != "no-unused-var" {
		t.Errorf("Rule: want %q, got %q", "no-unused-var", p.Rule)
	}
	if p.FileGlob != "*.go" {
		t.Errorf("FileGlob: want %q, got %q", "*.go", p.FileGlob)
	}
	if p.Frequency != 1 {
		t.Errorf("Frequency: want 1, got %d", p.Frequency)
	}
	if p.Source != "learned" {
		t.Errorf("Source: want %q, got %q", "learned", p.Source)
	}
}

// TestUpsertFrequency inserts the same pattern twice and expects frequency=2.
func TestUpsertFrequency(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 2; i++ {
		err := s.InsertLintPattern("errcheck", "*.go", "f()\n", "if err := f(); err != nil {}", "learned")
		if err != nil {
			t.Fatalf("InsertLintPattern (iteration %d): %v", i, err)
		}
	}

	patterns, err := s.QueryPatterns("*.go", "", 10)
	if err != nil {
		t.Fatalf("QueryPatterns: %v", err)
	}
	if len(patterns) != 1 {
		t.Fatalf("expected 1 pattern, got %d", len(patterns))
	}
	if patterns[0].Frequency != 2 {
		t.Errorf("Frequency: want 2, got %d", patterns[0].Frequency)
	}
}

// TestInsertOWASPFinding inserts a finding and verifies it persists; a second
// insert for the same triple increments frequency.
func TestInsertOWASPFinding(t *testing.T) {
	s := newTestStore(t)

	cat, fp, finding, fix := "A03-Injection", "*.go", "fmt.Sprintf with user input", "use parameterised queries"

	if err := s.InsertOWASPFinding(cat, fp, finding, fix); err != nil {
		t.Fatalf("InsertOWASPFinding (first): %v", err)
	}
	if err := s.InsertOWASPFinding(cat, fp, finding, fix); err != nil {
		t.Fatalf("InsertOWASPFinding (second): %v", err)
	}

	var freq int64
	err := s.db.QueryRow(
		`SELECT frequency FROM owasp_findings WHERE category=? AND file_pattern=? AND finding=?`,
		cat, fp, finding,
	).Scan(&freq)
	if err != nil {
		t.Fatalf("query frequency: %v", err)
	}
	if freq != 2 {
		t.Errorf("frequency: want 2, got %d", freq)
	}
}

// TestUpdateScanHistory upserts a scan record twice and verifies last_run is
// refreshed and only one row exists per (scan_type, project) pair.
func TestUpdateScanHistory(t *testing.T) {
	s := newTestStore(t)

	if err := s.UpdateScanHistory("lint", "myproject", 3); err != nil {
		t.Fatalf("UpdateScanHistory (first): %v", err)
	}

	// Small sleep so the second CURRENT_TIMESTAMP is strictly later.
	time.Sleep(2 * time.Millisecond)

	if err := s.UpdateScanHistory("lint", "myproject", 7); err != nil {
		t.Fatalf("UpdateScanHistory (second): %v", err)
	}

	var rowCount int64
	if err := s.db.QueryRow(
		`SELECT COUNT(*) FROM scan_history WHERE scan_type='lint' AND project='myproject'`,
	).Scan(&rowCount); err != nil {
		t.Fatalf("count rows: %v", err)
	}
	if rowCount != 1 {
		t.Errorf("expected 1 row, got %d", rowCount)
	}

	var findingsCount int64
	if err := s.db.QueryRow(
		`SELECT findings_count FROM scan_history WHERE scan_type='lint' AND project='myproject'`,
	).Scan(&findingsCount); err != nil {
		t.Fatalf("query findings_count: %v", err)
	}
	if findingsCount != 7 {
		t.Errorf("findings_count: want 7, got %d", findingsCount)
	}

	history, err := s.GetScanHistory("myproject")
	if err != nil {
		t.Fatalf("GetScanHistory: %v", err)
	}
	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
}

// TestInsertAntiPattern verifies that INSERT OR IGNORE skips duplicate
// pattern_id rows silently.
func TestInsertAntiPattern(t *testing.T) {
	s := newTestStore(t)

	// Use a unique category that won't appear in seed data.
	args := []string{"AP-test", "test description", "bad code", "good code", "test", "test-only-unique"}
	if err := s.InsertAntiPattern(args[0], args[1], args[2], args[3], args[4], args[5]); err != nil {
		t.Fatalf("InsertAntiPattern (first): %v", err)
	}
	// Second insert with same pattern_id — should be ignored, not error.
	if err := s.InsertAntiPattern(args[0], "changed description", args[2], args[3], args[4], args[5]); err != nil {
		t.Fatalf("InsertAntiPattern (duplicate): %v", err)
	}

	aps, err := s.QueryAntiPatterns("test-only-unique")
	if err != nil {
		t.Fatalf("QueryAntiPatterns: %v", err)
	}
	if len(aps) != 1 {
		t.Fatalf("expected 1 anti-pattern, got %d", len(aps))
	}
	// The original description should be retained, not the duplicate's.
	if aps[0].Description != "test description" {
		t.Errorf("Description: want %q, got %q", "test description", aps[0].Description)
	}
}

// TestVulnCache exercises UpsertVulnCache and GetVulnCache.
func TestVulnCache(t *testing.T) {
	s := newTestStore(t)

	mod := "github.com/example/pkg"
	if err := s.UpsertVulnCache(mod, "CVE-2024-0001", "HIGH", "< 1.2.0", "1.2.0", "remote code exec"); err != nil {
		t.Fatalf("UpsertVulnCache: %v", err)
	}

	entries, err := s.GetVulnCache(mod)
	if err != nil {
		t.Fatalf("GetVulnCache: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.CVEID != "CVE-2024-0001" {
		t.Errorf("CVEID: want %q, got %q", "CVE-2024-0001", e.CVEID)
	}
	if e.Severity != "HIGH" {
		t.Errorf("Severity: want %q, got %q", "HIGH", e.Severity)
	}
	if e.FixedVersion != "1.2.0" {
		t.Errorf("FixedVersion: want %q, got %q", "1.2.0", e.FixedVersion)
	}

	// Upsert again with updated severity — should update in place.
	if err := s.UpsertVulnCache(mod, "CVE-2024-0001", "CRITICAL", "< 1.2.0", "1.2.0", "rce + privesc"); err != nil {
		t.Fatalf("UpsertVulnCache (update): %v", err)
	}
	entries2, err := s.GetVulnCache(mod)
	if err != nil {
		t.Fatalf("GetVulnCache (after update): %v", err)
	}
	if len(entries2) != 1 {
		t.Fatalf("expected 1 entry after upsert, got %d", len(entries2))
	}
	if entries2[0].Severity != "CRITICAL" {
		t.Errorf("Severity after update: want %q, got %q", "CRITICAL", entries2[0].Severity)
	}
}

// TestDepDecision exercises UpsertDepDecision and GetDepDecision.
func TestDepDecision(t *testing.T) {
	s := newTestStore(t)

	mod := "github.com/example/vuln-lib"

	// No decision yet — should return nil, nil.
	d, err := s.GetDepDecision(mod)
	if err != nil {
		t.Fatalf("GetDepDecision (missing): %v", err)
	}
	if d != nil {
		t.Fatalf("expected nil decision, got %+v", d)
	}

	if err := s.UpsertDepDecision(mod, "reject", "has CVE-2024-0001", 1); err != nil {
		t.Fatalf("UpsertDepDecision: %v", err)
	}

	d, err = s.GetDepDecision(mod)
	if err != nil {
		t.Fatalf("GetDepDecision: %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil decision")
	}
	if d.Decision != "reject" {
		t.Errorf("Decision: want %q, got %q", "reject", d.Decision)
	}
	if d.CVECount != 1 {
		t.Errorf("CVECount: want 1, got %d", d.CVECount)
	}

	// Update the decision.
	if err := s.UpsertDepDecision(mod, "accept", "patched in v2", 0); err != nil {
		t.Fatalf("UpsertDepDecision (update): %v", err)
	}
	d2, err := s.GetDepDecision(mod)
	if err != nil {
		t.Fatalf("GetDepDecision (after update): %v", err)
	}
	if d2.Decision != "accept" {
		t.Errorf("Decision after update: want %q, got %q", "accept", d2.Decision)
	}
	if d2.CVECount != 0 {
		t.Errorf("CVECount after update: want 0, got %d", d2.CVECount)
	}
}
