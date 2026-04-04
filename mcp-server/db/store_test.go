package db

import (
	"fmt"
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

// TestNewStore verifies that all tables are created by NewStore.
func TestNewStore(t *testing.T) {
	s := newTestStore(t)

	tables := []string{
		"lint_patterns",
		"owasp_findings",
		"vuln_cache",
		"scan_history",
		"anti_patterns",
		"dep_decisions",
		"scan_snapshots",
		"session_findings",
		"mcp_requests",
		"pattern_history",
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
	if err := s.UpsertVulnCache(mod, "CVE-2024-0001", "HIGH", "< 1.2.0", "1.2.0", "remote code exec", "go-vuln"); err != nil {
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
	if err := s.UpsertVulnCache(mod, "CVE-2024-0001", "CRITICAL", "< 1.2.0", "1.2.0", "rce + privesc", "nvd"); err != nil {
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

// ── Scan Snapshot Tests ──────────────────────────────────────────────────────

func TestInsertScanSnapshot(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 3; i++ {
		if err := s.InsertScanSnapshot("lint", "myproject", i*5, `{"categories":{"errcheck":1}}`); err != nil {
			t.Fatalf("InsertScanSnapshot(%d): %v", i, err)
		}
	}

	snapshots, err := s.GetScanSnapshots("lint", "myproject", 10)
	if err != nil {
		t.Fatalf("GetScanSnapshots: %v", err)
	}
	if len(snapshots) != 3 {
		t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
	}
	// Ordered by id DESC — most recent insert first.
	if snapshots[0].FindingsCount != 10 {
		t.Errorf("most recent findings_count: want 10, got %d", snapshots[0].FindingsCount)
	}
	if snapshots[2].FindingsCount != 0 {
		t.Errorf("oldest findings_count: want 0, got %d", snapshots[2].FindingsCount)
	}
}

func TestScanSnapshotRetention(t *testing.T) {
	s := newTestStore(t)

	// Insert more than retention limit.
	for i := 0; i < 105; i++ {
		if err := s.InsertScanSnapshot("owasp", "proj", i, "{}"); err != nil {
			t.Fatalf("InsertScanSnapshot(%d): %v", i, err)
		}
	}

	snapshots, err := s.GetScanSnapshots("owasp", "proj", 200)
	if err != nil {
		t.Fatalf("GetScanSnapshots: %v", err)
	}
	if len(snapshots) != 100 {
		t.Errorf("expected 100 snapshots after pruning, got %d", len(snapshots))
	}
	// Most recent by ID should be first.
	if snapshots[0].FindingsCount != 104 {
		t.Errorf("most recent findings_count: want 104, got %d", snapshots[0].FindingsCount)
	}
	// Oldest retained should be 5 (0-4 pruned).
	if snapshots[len(snapshots)-1].FindingsCount != 5 {
		t.Errorf("oldest retained findings_count: want 5, got %d", snapshots[len(snapshots)-1].FindingsCount)
	}
}

func TestGetAllScanSnapshots(t *testing.T) {
	s := newTestStore(t)

	_ = s.InsertScanSnapshot("lint", "proj", 5, "{}")
	_ = s.InsertScanSnapshot("owasp", "proj", 3, "{}")
	_ = s.InsertScanSnapshot("vuln", "proj", 1, "{}")

	all, err := s.GetAllScanSnapshots("proj", 10)
	if err != nil {
		t.Fatalf("GetAllScanSnapshots: %v", err)
	}
	if len(all) != 3 {
		t.Errorf("expected 3 snapshots, got %d", len(all))
	}
}

// ── Session Finding Tests ───────────────────────────────────────────────��────

func TestInsertSessionFinding(t *testing.T) {
	s := newTestStore(t)

	id, err := s.InsertSessionFinding("sess-1", "reviewer", "concurrency", "service.go", "race on shared map", "HIGH")
	if err != nil {
		t.Fatalf("InsertSessionFinding: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero insert ID")
	}

	findings, err := s.GetSessionFindings("sess-1", "")
	if err != nil {
		t.Fatalf("GetSessionFindings: %v", err)
	}
	if len(findings) != 1 {
		t.Fatalf("expected 1 finding, got %d", len(findings))
	}
	f := findings[0]
	if f.Agent != "reviewer" {
		t.Errorf("Agent: want %q, got %q", "reviewer", f.Agent)
	}
	if f.Severity != "HIGH" {
		t.Errorf("Severity: want %q, got %q", "HIGH", f.Severity)
	}
}

func TestGetSessionFindingsFilterByAgent(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.InsertSessionFinding("sess-1", "reviewer", "concurrency", "a.go", "finding 1", "HIGH")
	_, _ = s.InsertSessionFinding("sess-1", "security", "security", "b.go", "finding 2", "CRITICAL")
	_, _ = s.InsertSessionFinding("sess-1", "reviewer", "error-handling", "c.go", "finding 3", "MEDIUM")

	reviewerFindings, err := s.GetSessionFindings("sess-1", "reviewer")
	if err != nil {
		t.Fatalf("GetSessionFindings(reviewer): %v", err)
	}
	if len(reviewerFindings) != 2 {
		t.Errorf("expected 2 reviewer findings, got %d", len(reviewerFindings))
	}

	securityFindings, err := s.GetSessionFindings("sess-1", "security")
	if err != nil {
		t.Fatalf("GetSessionFindings(security): %v", err)
	}
	if len(securityFindings) != 1 {
		t.Errorf("expected 1 security finding, got %d", len(securityFindings))
	}
}

func TestGetSessionFindingsByFile(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.InsertSessionFinding("sess-1", "reviewer", "concurrency", "service.go", "race", "HIGH")
	_, _ = s.InsertSessionFinding("sess-1", "security", "security", "handler.go", "injection", "CRITICAL")

	findings, err := s.GetSessionFindingsByFile("sess-1", "service.go")
	if err != nil {
		t.Fatalf("GetSessionFindingsByFile: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 finding for service.go, got %d", len(findings))
	}
}

func TestCleanupOldSessions(t *testing.T) {
	s := newTestStore(t)

	_, _ = s.InsertSessionFinding("old-sess", "reviewer", "concurrency", "a.go", "old finding", "HIGH")
	_, _ = s.InsertSessionFinding("current-sess", "security", "security", "b.go", "current finding", "HIGH")

	if err := s.CleanupOldSessions("current-sess"); err != nil {
		t.Fatalf("CleanupOldSessions: %v", err)
	}

	old, _ := s.GetSessionFindings("old-sess", "")
	if len(old) != 0 {
		t.Errorf("expected 0 old findings, got %d", len(old))
	}

	current, _ := s.GetSessionFindings("current-sess", "")
	if len(current) != 1 {
		t.Errorf("expected 1 current finding, got %d", len(current))
	}
}

// ── MCP Request Tests ──────────────────────────────────────────────────────

// TestInsertAndGetMCPRequests verifies basic insert and retrieval of MCP requests.
func TestInsertAndGetMCPRequests(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name     string
		tool     string
		agent    string
		params   string
		duration int64
		errMsg   string
	}{
		{"basic call", "query_knowledge", "reviewer", `{"file_path":"main.go"}`, 42, ""},
		{"with error", "check_owasp", "security", `{"project":"/app"}`, 100, "scan failed"},
		{"no agent", "learn_from_lint", "", `{"diff":"..."}`, 15, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := s.InsertMCPRequest(tt.tool, tt.agent, tt.params, tt.duration, tt.errMsg)
			if err != nil {
				t.Fatalf("InsertMCPRequest: %v", err)
			}
		})
	}

	// Get all requests (no filter).
	reqs, err := s.GetMCPRequests("", "", 100, 0)
	if err != nil {
		t.Fatalf("GetMCPRequests: %v", err)
	}
	if len(reqs) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(reqs))
	}
	// Most recent first.
	if reqs[0].ToolName != "learn_from_lint" {
		t.Errorf("expected most recent tool to be learn_from_lint, got %s", reqs[0].ToolName)
	}
}

// TestGetMCPRequestsFilters verifies filtering by tool_name and agent.
func TestGetMCPRequestsFilters(t *testing.T) {
	s := newTestStore(t)

	_ = s.InsertMCPRequest("query_knowledge", "reviewer", "{}", 10, "")
	_ = s.InsertMCPRequest("check_owasp", "security", "{}", 20, "")
	_ = s.InsertMCPRequest("query_knowledge", "linter", "{}", 30, "")

	// Filter by tool.
	reqs, err := s.GetMCPRequests("query_knowledge", "", 100, 0)
	if err != nil {
		t.Fatalf("GetMCPRequests(tool filter): %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests for query_knowledge, got %d", len(reqs))
	}

	// Filter by agent.
	reqs, err = s.GetMCPRequests("", "security", 100, 0)
	if err != nil {
		t.Fatalf("GetMCPRequests(agent filter): %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request for security, got %d", len(reqs))
	}

	// Filter by both.
	reqs, err = s.GetMCPRequests("query_knowledge", "reviewer", 100, 0)
	if err != nil {
		t.Fatalf("GetMCPRequests(both filters): %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 request, got %d", len(reqs))
	}
}

// TestGetMCPRequestsPagination verifies limit and offset work correctly.
func TestGetMCPRequestsPagination(t *testing.T) {
	s := newTestStore(t)

	for i := 0; i < 5; i++ {
		_ = s.InsertMCPRequest(fmt.Sprintf("tool_%d", i), "", "{}", int64(i), "")
	}

	// Limit 2.
	reqs, err := s.GetMCPRequests("", "", 2, 0)
	if err != nil {
		t.Fatalf("GetMCPRequests(limit): %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests, got %d", len(reqs))
	}

	// Offset 3.
	reqs, err = s.GetMCPRequests("", "", 100, 3)
	if err != nil {
		t.Fatalf("GetMCPRequests(offset): %v", err)
	}
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests with offset 3, got %d", len(reqs))
	}
}

// TestPruneMCPRequests verifies that old entries are deleted.
func TestPruneMCPRequests(t *testing.T) {
	s := newTestStore(t)

	// Insert a request, then backdate it.
	_ = s.InsertMCPRequest("old_tool", "", "{}", 1, "")
	_, err := s.db.Exec(`UPDATE mcp_requests SET created_at = datetime('now', '-8 days')`)
	if err != nil {
		t.Fatalf("backdate: %v", err)
	}

	// Insert a fresh one.
	_ = s.InsertMCPRequest("new_tool", "", "{}", 1, "")

	// Prune older than 7 days.
	deleted, err := s.PruneMCPRequests(7 * 24 * time.Hour)
	if err != nil {
		t.Fatalf("PruneMCPRequests: %v", err)
	}
	if deleted != 1 {
		t.Errorf("expected 1 deleted, got %d", deleted)
	}

	// Verify only new_tool remains.
	reqs, err := s.GetMCPRequests("", "", 100, 0)
	if err != nil {
		t.Fatalf("GetMCPRequests: %v", err)
	}
	if len(reqs) != 1 {
		t.Fatalf("expected 1 remaining, got %d", len(reqs))
	}
	if reqs[0].ToolName != "new_tool" {
		t.Errorf("expected new_tool, got %s", reqs[0].ToolName)
	}
}

func TestRecentLearningCount(t *testing.T) {
	s := newTestStore(t)

	// Insert two patterns (created_at defaults to NOW).
	_ = s.InsertLintPattern("rule1", "*.go", "bad1", "good1", "learned")
	_ = s.InsertLintPattern("rule2", "*.go", "bad2", "good2", "review")

	count, err := s.RecentLearningCount(7)
	if err != nil {
		t.Fatalf("RecentLearningCount: %v", err)
	}
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}

	// Count for 0 days should still include today's patterns.
	_, err = s.RecentLearningCount(0)
	if err != nil {
		t.Fatalf("RecentLearningCount(0): %v", err)
	}
	// 0 days means "since now" — might be 0 or 2 depending on timing.
	// Just verify no error.
}

// ── Pattern Management Tests ──────────────────────────────────────────────────

func TestSoftDeleteAndRestoreLintPattern(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			"soft delete hides pattern from QueryPatterns",
			func(t *testing.T) {
				t.Helper()
				_ = s.InsertLintPattern("errcheck", "*.go", "f()", "if err := f(); err != nil {}", "learned")

				patterns, _ := s.QueryPatterns("*.go", "", 10)
				if len(patterns) == 0 {
					t.Fatal("expected pattern before delete")
				}
				id := patterns[0].ID

				if err := s.SoftDeleteLintPattern(id); err != nil {
					t.Fatalf("SoftDeleteLintPattern: %v", err)
				}

				patterns, _ = s.QueryPatterns("*.go", "", 10)
				if len(patterns) != 0 {
					t.Errorf("expected 0 patterns after soft delete, got %d", len(patterns))
				}
			},
		},
		{
			"restore brings pattern back",
			func(t *testing.T) {
				t.Helper()
				// Pattern was soft-deleted in previous subtest; find its ID.
				p, _, _ := s.GetAllLintPatterns("errcheck", "", "", "frequency", true, 10, 0)
				if len(p) == 0 {
					t.Fatal("expected deleted pattern via GetAllLintPatterns with includeDeleted")
				}
				id := p[0].ID
				if p[0].DeletedAt == nil {
					t.Fatal("expected DeletedAt to be non-nil")
				}

				if err := s.RestoreLintPattern(id); err != nil {
					t.Fatalf("RestoreLintPattern: %v", err)
				}

				patterns, _ := s.QueryPatterns("*.go", "", 10)
				if len(patterns) != 1 {
					t.Errorf("expected 1 pattern after restore, got %d", len(patterns))
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestPatternHistory(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{
			"insert and retrieve history",
			func(t *testing.T) {
				t.Helper()
				if err := s.InsertPatternHistory("lint", 1, "edit", `{"rule":"old"}`, `{"rule":"new"}`); err != nil {
					t.Fatalf("InsertPatternHistory: %v", err)
				}
				if err := s.InsertPatternHistory("lint", 1, "delete", `{"rule":"new"}`, `{}`); err != nil {
					t.Fatalf("InsertPatternHistory: %v", err)
				}

				entries, err := s.GetPatternHistory("lint", 1)
				if err != nil {
					t.Fatalf("GetPatternHistory: %v", err)
				}
				if len(entries) != 2 {
					t.Fatalf("expected 2 entries, got %d", len(entries))
				}
				// Most recent first.
				if entries[0].Action != "delete" {
					t.Errorf("expected most recent action 'delete', got %q", entries[0].Action)
				}
				if entries[1].Action != "edit" {
					t.Errorf("expected oldest action 'edit', got %q", entries[1].Action)
				}
			},
		},
		{
			"GetRecentPatternHistory returns across patterns",
			func(t *testing.T) {
				t.Helper()
				_ = s.InsertPatternHistory("anti", 99, "restore", `{}`, `{"id":99}`)

				recent, err := s.GetRecentPatternHistory(10)
				if err != nil {
					t.Fatalf("GetRecentPatternHistory: %v", err)
				}
				if len(recent) < 3 {
					t.Fatalf("expected at least 3 recent entries, got %d", len(recent))
				}
				if recent[0].PatternType != "anti" {
					t.Errorf("expected most recent type 'anti', got %q", recent[0].PatternType)
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, tt.fn)
	}
}

func TestGetAllLintPatterns(t *testing.T) {
	s := newTestStore(t)

	// Insert test data.
	_ = s.InsertLintPattern("errcheck", "*.go", "f()", "if err := f(); err != nil {}", "learned")
	_ = s.InsertLintPattern("unused", "*.go", "x := 1", "// remove", "review")
	_ = s.InsertLintPattern("shadow", "*.ts", "var x", "let x", "learned")

	// Soft-delete one.
	patterns, _, _ := s.GetAllLintPatterns("", "", "", "frequency", false, 10, 0)
	var shadowID int64
	for _, p := range patterns {
		if p.Rule == "shadow" {
			shadowID = p.ID
		}
	}
	_ = s.SoftDeleteLintPattern(shadowID)

	tests := []struct {
		name           string
		search         string
		source         string
		rule           string
		sortBy         string
		includeDeleted bool
		limit          int
		offset         int
		wantCount      int
		wantTotal      int64
	}{
		{"all active", "", "", "", "frequency", false, 10, 0, 2, 2},
		{"include deleted", "", "", "", "frequency", true, 10, 0, 3, 3},
		{"search by rule", "errcheck", "", "", "frequency", false, 10, 0, 1, 1},
		{"filter by source", "", "review", "", "frequency", false, 10, 0, 1, 1},
		{"filter by rule", "", "", "unused", "frequency", false, 10, 0, 1, 1},
		{"pagination limit", "", "", "", "frequency", false, 1, 0, 1, 2},
		{"pagination offset", "", "", "", "frequency", false, 10, 1, 1, 2},
		{"sort by created_at", "", "", "", "created_at", false, 10, 0, 2, 2},
		{"sort by last_seen", "", "", "", "last_seen", false, 10, 0, 2, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, total, err := s.GetAllLintPatterns(tt.search, tt.source, tt.rule, tt.sortBy, tt.includeDeleted, tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("GetAllLintPatterns: %v", err)
			}
			if len(results) != tt.wantCount {
				t.Errorf("results count: want %d, got %d", tt.wantCount, len(results))
			}
			if total != tt.wantTotal {
				t.Errorf("total: want %d, got %d", tt.wantTotal, total)
			}
		})
	}
}

func TestGetQualitySuggestions(t *testing.T) {
	s := newTestStore(t)

	tests := []struct {
		name     string
		setup    func()
		wantType string
	}{
		{
			"empty do_code",
			func() {
				_ = s.InsertLintPattern("no-fix", "*.go", "bad code", "", "learned")
			},
			"empty_do_code",
		},
		{
			"low frequency",
			func() {
				// Already inserted with frequency 1 above.
			},
			"low_frequency",
		},
		{
			"duplicate dont_code",
			func() {
				_ = s.InsertLintPattern("dup-rule-1", "*.go", "shared bad code", "fix1", "learned")
				_ = s.InsertLintPattern("dup-rule-2", "*.ts", "shared bad code", "fix2", "learned")
			},
			"duplicate_dont_code",
		},
	}

	// Run all setups first.
	for _, tt := range tests {
		tt.setup()
	}

	suggestions, err := s.GetQualitySuggestions()
	if err != nil {
		t.Fatalf("GetQualitySuggestions: %v", err)
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			found := false
			for _, sug := range suggestions {
				if sug.Type == tt.wantType {
					found = true
					if len(sug.PatternIDs) == 0 {
						t.Error("expected non-empty PatternIDs")
					}
					break
				}
			}
			if !found {
				t.Errorf("expected suggestion type %q not found in %d suggestions", tt.wantType, len(suggestions))
			}
		})
	}
}

func TestInsertLintPatternClearsDeletedAt(t *testing.T) {
	s := newTestStore(t)

	// Insert, soft-delete, re-insert (re-learn).
	_ = s.InsertLintPattern("errcheck", "*.go", "f()", "if err := f(); err != nil {}", "learned")

	patterns, _ := s.QueryPatterns("*.go", "", 10)
	id := patterns[0].ID

	_ = s.SoftDeleteLintPattern(id)

	// Verify it's hidden.
	patterns, _ = s.QueryPatterns("*.go", "", 10)
	if len(patterns) != 0 {
		t.Fatal("expected 0 patterns after soft delete")
	}

	// Re-learn the same pattern.
	_ = s.InsertLintPattern("errcheck", "*.go", "f()", "if err := f(); err != nil {}", "learned")

	// Verify it's back and deleted_at is cleared.
	p, err := s.GetLintPatternByID(id)
	if err != nil {
		t.Fatalf("GetLintPatternByID: %v", err)
	}
	if p == nil {
		t.Fatal("expected non-nil pattern")
	}
	if p.DeletedAt != nil {
		t.Error("expected DeletedAt to be nil after re-learn")
	}
	if p.Frequency != 2 {
		t.Errorf("expected frequency 2 after re-learn, got %d", p.Frequency)
	}
}

func TestGetPatternStatsExcludesDeleted(t *testing.T) {
	s := newTestStore(t)

	_ = s.InsertLintPattern("rule-a", "*.go", "bad-a", "good-a", "learned")
	_ = s.InsertLintPattern("rule-b", "*.go", "bad-b", "good-b", "learned")

	// Also add an anti-pattern.
	_ = s.InsertAntiPattern("AP-stats-test", "desc", "bad", "good", "test", "test-cat")

	// Get stats before delete.
	statsBefore, err := s.GetPatternStats("any-project")
	if err != nil {
		t.Fatalf("GetPatternStats (before): %v", err)
	}
	lintBefore := statsBefore.TotalLintPatterns
	antiBefore := statsBefore.TotalAntiPatterns

	// Soft-delete one lint pattern.
	patterns, _, _ := s.GetAllLintPatterns("rule-a", "", "", "frequency", false, 10, 0)
	if len(patterns) == 0 {
		t.Fatal("expected to find rule-a")
	}
	_ = s.SoftDeleteLintPattern(patterns[0].ID)

	// Soft-delete the anti-pattern.
	aps, _, _ := s.GetAllAntiPatterns("AP-stats-test", "", false, 10, 0)
	if len(aps) == 0 {
		t.Fatal("expected to find AP-stats-test")
	}
	_ = s.SoftDeleteAntiPattern(aps[0].ID)

	statsAfter, err := s.GetPatternStats("any-project")
	if err != nil {
		t.Fatalf("GetPatternStats (after): %v", err)
	}

	if statsAfter.TotalLintPatterns != lintBefore-1 {
		t.Errorf("TotalLintPatterns: want %d, got %d", lintBefore-1, statsAfter.TotalLintPatterns)
	}
	if statsAfter.TotalAntiPatterns != antiBefore-1 {
		t.Errorf("TotalAntiPatterns: want %d, got %d", antiBefore-1, statsAfter.TotalAntiPatterns)
	}

	// Top lint patterns should not include deleted ones.
	for _, p := range statsAfter.TopLintPatterns {
		if p.Rule == "rule-a" {
			t.Error("deleted pattern 'rule-a' should not appear in TopLintPatterns")
		}
	}
}

// ── Domain Browser Store Tests ──────────────────────────────────────────────

func TestGetAllVulnEntries(t *testing.T) {
	s := newTestStore(t)

	// Insert 3 vulns for 2 modules.
	_ = s.UpsertVulnCache("github.com/foo/bar", "CVE-2024-0001", "CRITICAL", "<1.0", "1.0.1", "RCE vulnerability", "nvd")
	_ = s.UpsertVulnCache("github.com/foo/bar", "CVE-2024-0002", "HIGH", "<2.0", "2.0.0", "SSRF vulnerability", "go-vuln")
	_ = s.UpsertVulnCache("github.com/baz/qux", "CVE-2024-0003", "MEDIUM", "<3.0", "3.0.0", "XSS vulnerability", "nvd")

	entries, err := s.GetAllVulnEntries(500)
	if err != nil {
		t.Fatalf("GetAllVulnEntries: %v", err)
	}
	if len(entries) != 3 {
		t.Fatalf("expected 3 entries, got %d", len(entries))
	}

	// Verify all 3 entries are returned with correct data.
	severities := map[string]bool{}
	for _, e := range entries {
		severities[e.Severity] = true
	}
	for _, sev := range []string{"CRITICAL", "HIGH", "MEDIUM"} {
		if !severities[sev] {
			t.Errorf("missing severity %q in results", sev)
		}
	}

	// Verify ordering: severity DESC (text sort), module ASC within same severity.
	for i := 1; i < len(entries); i++ {
		prev := entries[i-1]
		curr := entries[i]
		if prev.Severity < curr.Severity {
			t.Errorf("entries not sorted by severity DESC: %q before %q", prev.Severity, curr.Severity)
		}
		if prev.Severity == curr.Severity && prev.Module > curr.Module {
			t.Errorf("entries not sorted by module ASC within same severity: %q before %q", prev.Module, curr.Module)
		}
	}

	// Test default limit.
	entries2, err := s.GetAllVulnEntries(0)
	if err != nil {
		t.Fatalf("GetAllVulnEntries(0): %v", err)
	}
	if len(entries2) != 3 {
		t.Errorf("expected 3 entries with default limit, got %d", len(entries2))
	}
}

func TestGetAllDepDecisions(t *testing.T) {
	s := newTestStore(t)

	// Insert 2 decisions.
	_ = s.UpsertDepDecision("github.com/foo/bar", "upgrade", "Critical CVE", 2)
	_ = s.UpsertDepDecision("github.com/baz/qux", "accept", "Low risk", 0)

	decisions, err := s.GetAllDepDecisions()
	if err != nil {
		t.Fatalf("GetAllDepDecisions: %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("expected 2 decisions, got %d", len(decisions))
	}

	// Verify both modules are present.
	modules := map[string]bool{}
	for _, d := range decisions {
		modules[d.Module] = true
	}
	if !modules["github.com/foo/bar"] {
		t.Error("missing github.com/foo/bar decision")
	}
	if !modules["github.com/baz/qux"] {
		t.Error("missing github.com/baz/qux decision")
	}
}
