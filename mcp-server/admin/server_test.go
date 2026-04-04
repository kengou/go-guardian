package admin

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/tools"
)

func newTestServer(t *testing.T) *Server {
	t.Helper()
	store, err := db.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	staticFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>admin</html>")},
	}
	return New(store, staticFS, "")
}

func seedRequests(t *testing.T, srv *Server) {
	t.Helper()
	requests := []struct {
		tool     string
		agent    string
		params   string
		duration int64
		errMsg   string
	}{
		{"query_knowledge", "reviewer", `{"file":"main.go"}`, 15, ""},
		{"check_owasp", "security", `{"path":"/src"}`, 42, ""},
		{"query_knowledge", "linter", `{"file":"util.go"}`, 8, ""},
		{"learn_from_lint", "reviewer", `{"diff":"..."}`, 120, "parse error"},
	}
	for _, r := range requests {
		if err := srv.store.InsertMCPRequest(r.tool, r.agent, r.params, r.duration, r.errMsg); err != nil {
			t.Fatalf("InsertMCPRequest: %v", err)
		}
	}
}

func TestHandleActivity(t *testing.T) {
	tests := []struct {
		name      string
		query     string
		seed      bool
		wantCount int
	}{
		{
			name:      "empty state returns empty array",
			query:     "/api/v1/activity",
			seed:      false,
			wantCount: 0,
		},
		{
			name:      "returns all seeded requests",
			query:     "/api/v1/activity",
			seed:      true,
			wantCount: 4,
		},
		{
			name:      "filter by tool",
			query:     "/api/v1/activity?tool=query_knowledge",
			seed:      true,
			wantCount: 2,
		},
		{
			name:      "filter by agent",
			query:     "/api/v1/activity?agent=reviewer",
			seed:      true,
			wantCount: 2,
		},
		{
			name:      "limit results",
			query:     "/api/v1/activity?limit=1",
			seed:      true,
			wantCount: 1,
		},
		{
			name:      "filter by tool and agent",
			query:     "/api/v1/activity?tool=query_knowledge&agent=reviewer",
			seed:      true,
			wantCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newTestServer(t)
			if tt.seed {
				seedRequests(t, srv)
			}

			req := httptest.NewRequest(http.MethodGet, tt.query, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body: %s", rec.Code, http.StatusOK, rec.Body.String())
			}

			ct := rec.Header().Get("Content-Type")
			if ct != "application/json" {
				t.Errorf("Content-Type = %q, want %q", ct, "application/json")
			}

			var entries []activityEntry
			if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
				t.Fatalf("decode JSON: %v", err)
			}

			if len(entries) != tt.wantCount {
				t.Errorf("got %d entries, want %d", len(entries), tt.wantCount)
			}
		})
	}
}

func TestHandleActivityEmptyArray(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	body := rec.Body.String()
	// Must be "[]" (with optional trailing newline), not "null".
	trimmed := body
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	if trimmed != "[]" {
		t.Errorf("empty response body = %q, want %q", body, "[]")
	}
}

func TestHandleActivityFieldFormat(t *testing.T) {
	srv := newTestServer(t)
	seedRequests(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/activity?limit=1", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	var entries []activityEntry
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("decode JSON: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected at least 1 entry")
	}

	e := entries[0]
	if e.ID == 0 {
		t.Error("id should be non-zero")
	}
	if e.ToolName == "" {
		t.Error("tool_name should not be empty")
	}
	if e.CreatedAt == "" {
		t.Error("created_at should not be empty")
	}
}

func TestSPAFallback(t *testing.T) {
	srv := newTestServer(t)

	tests := []struct {
		name string
		path string
	}{
		{"root serves index", "/"},
		{"unknown path falls back to index", "/some/spa/route"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()
			srv.ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
			}
			if body := rec.Body.String(); body != "<html>admin</html>" {
				t.Errorf("body = %q, want index.html content", body)
			}
		})
	}
}

// TestDashboardEndpoint tests the GET /api/v1/dashboard endpoint.
func TestDashboardEndpoint(t *testing.T) {
	t.Run("empty state", func(t *testing.T) {
		srv := newTestServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}

		// Verify key fields exist.
		if _, ok := resp["total_patterns"]; !ok {
			t.Error("missing total_patterns")
		}
		if _, ok := resp["total_anti_patterns"]; !ok {
			t.Error("missing total_anti_patterns")
		}
		if _, ok := resp["recent_learning_count"]; !ok {
			t.Error("missing recent_learning_count")
		}
		if _, ok := resp["owasp_counts"]; !ok {
			t.Error("missing owasp_counts")
		}
		// owasp_counts should be {} not null.
		if resp["owasp_counts"] == nil {
			t.Error("owasp_counts should be {} not null")
		}
		// recent_scans should be [] not null.
		scans, ok := resp["recent_scans"].([]interface{})
		if !ok {
			t.Error("recent_scans should be []")
		}
		if scans == nil {
			t.Error("recent_scans should be [] not null")
		}
	})

	t.Run("with data", func(t *testing.T) {
		srv := newTestServer(t)

		// Seed some patterns.
		_ = srv.store.InsertLintPattern("errcheck", "*.go", "bad()", "good()", "learned")
		_ = srv.store.InsertLintPattern("govet", "*.go", "bad2()", "good2()", "review")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}

		total := resp["total_patterns"].(float64)
		if total != 2 {
			t.Errorf("total_patterns = %v, want 2", total)
		}

		recent := resp["recent_learning_count"].(float64)
		if recent != 2 {
			t.Errorf("recent_learning_count = %v, want 2", recent)
		}
	})
}

// TestTrendsEndpoint tests the GET /api/v1/trends endpoint.
func TestTrendsEndpoint(t *testing.T) {
	t.Run("empty state", func(t *testing.T) {
		srv := newTestServer(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/trends", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}

		snapshots, ok := resp["snapshots"].([]interface{})
		if !ok || snapshots == nil {
			t.Error("snapshots should be []")
		}
		if len(snapshots) != 0 {
			t.Errorf("expected 0 snapshots, got %d", len(snapshots))
		}
	})

	t.Run("with snapshots", func(t *testing.T) {
		srv := newTestServer(t)

		// Seed snapshots: 10, 8, 5 (improving — findings decreasing).
		_ = srv.store.InsertScanSnapshot("lint", "test-project", 10, "{}")
		_ = srv.store.InsertScanSnapshot("lint", "test-project", 8, "{}")
		_ = srv.store.InsertScanSnapshot("lint", "test-project", 5, "{}")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/trends?project=test-project", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
			t.Fatalf("decode: %v", err)
		}

		snapshots := resp["snapshots"].([]interface{})
		if len(snapshots) != 3 {
			t.Fatalf("expected 3 snapshots, got %d", len(snapshots))
		}

		directions := resp["directions"].([]interface{})
		if len(directions) != 1 {
			t.Fatalf("expected 1 direction entry, got %d", len(directions))
		}

		dir := directions[0].(map[string]interface{})
		if dir["direction"] != "improving" {
			t.Errorf("direction = %v, want improving", dir["direction"])
		}
	})

	t.Run("filter by scan_type", func(t *testing.T) {
		srv := newTestServer(t)

		_ = srv.store.InsertScanSnapshot("lint", "p", 5, "{}")
		_ = srv.store.InsertScanSnapshot("owasp", "p", 3, "{}")

		req := httptest.NewRequest(http.MethodGet, "/api/v1/trends?scan_type=lint&project=p", nil)
		rec := httptest.NewRecorder()
		srv.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", rec.Code)
		}

		var resp map[string]interface{}
		json.NewDecoder(rec.Body).Decode(&resp)

		snapshots := resp["snapshots"].([]interface{})
		if len(snapshots) != 1 {
			t.Errorf("expected 1 lint snapshot, got %d", len(snapshots))
		}
	})
}

// TestComputeTrendDirection tests the direction computation.
func TestComputeTrendDirection(t *testing.T) {
	tests := []struct {
		name     string
		counts   []int64
		expected string
	}{
		{"insufficient", []int64{5}, "insufficient_data"},
		{"improving", []int64{3, 5, 8}, "improving"},    // newest=3 < 5 < 8=oldest
		{"degrading", []int64{8, 5, 3}, "degrading"},     // newest=8 > 5 > 3=oldest
		{"stable", []int64{5, 5, 5}, "stable"},
		{"mixed", []int64{5, 3, 7}, "stable"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			snapshots := make([]db.ScanSnapshot, len(tt.counts))
			for i, c := range tt.counts {
				snapshots[i] = db.ScanSnapshot{FindingsCount: c}
			}
			got := computeTrendDirection(snapshots)
			if got != tt.expected {
				t.Errorf("computeTrendDirection = %q, want %q", got, tt.expected)
			}
		})
	}
}

// ── Pattern management tests ───────────────────────────────────────────────

// baselineCounts returns the number of lint and anti patterns already in the
// store before any test-specific seeding (the DB seeds anti_patterns on creation).
func baselineCounts(t *testing.T, srv *Server) (lintCount, antiCount int64) {
	t.Helper()
	_, lc, err := srv.store.GetAllLintPatterns("", "", "", "frequency", false, 1, 0)
	if err != nil {
		t.Fatalf("baseline lint count: %v", err)
	}
	_, ac, err := srv.store.GetAllAntiPatterns("", "", false, 1, 0)
	if err != nil {
		t.Fatalf("baseline anti count: %v", err)
	}
	return lc, ac
}

func seedPatterns(t *testing.T, srv *Server) {
	t.Helper()
	// Seed lint patterns.
	if err := srv.store.InsertLintPattern("errcheck", "*.go", "_ = f()", "if err := f(); err != nil {}", "learned"); err != nil {
		t.Fatalf("InsertLintPattern: %v", err)
	}
	if err := srv.store.InsertLintPattern("govet", "*.go", "bad()", "good()", "review"); err != nil {
		t.Fatalf("InsertLintPattern: %v", err)
	}
	// Seed anti-pattern.
	if err := srv.store.InsertAntiPattern("TEST-AP-001", "Do not use fmt.Println in production", "fmt.Println(...)", "log.Info(...)", "seeded", "logging"); err != nil {
		t.Fatalf("InsertAntiPattern: %v", err)
	}
}

func TestListPatterns(t *testing.T) {
	srv := newTestServer(t)
	baseLint, baseAnti := baselineCounts(t, srv)
	seedPatterns(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patterns?per_page=200", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp patternListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	wantTotal := baseLint + baseAnti + 3 // 2 lint + 1 anti added by seedPatterns
	if resp.Total != wantTotal {
		t.Errorf("total = %d, want %d", resp.Total, wantTotal)
	}
	if resp.Page != 1 {
		t.Errorf("page = %d, want 1", resp.Page)
	}

	// Verify items has at least our seeded patterns.
	foundLint := 0
	foundAnti := 0
	for _, item := range resp.Items {
		if item.Type == "lint" && (item.Rule == "errcheck" || item.Rule == "govet") {
			foundLint++
		}
		if item.Type == "anti" && item.PatternID == "TEST-AP-001" {
			foundAnti++
		}
	}
	if foundLint != 2 {
		t.Errorf("found %d seeded lint patterns, want 2", foundLint)
	}
	if foundAnti != 1 {
		t.Errorf("found %d seeded anti patterns, want 1", foundAnti)
	}
}

func TestListPatternsSearch(t *testing.T) {
	srv := newTestServer(t)
	seedPatterns(t, srv)

	// Use a unique search term that only matches our seeded lint pattern.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/patterns?search=errcheck&type=lint", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp patternListResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Only the errcheck lint pattern should match.
	if len(resp.Items) != 1 {
		t.Errorf("got %d items, want 1", len(resp.Items))
	}
	if len(resp.Items) > 0 && resp.Items[0].Rule != "errcheck" {
		t.Errorf("rule = %q, want errcheck", resp.Items[0].Rule)
	}
}

func TestListPatternsTypeFilter(t *testing.T) {
	srv := newTestServer(t)
	_, baseAnti := baselineCounts(t, srv)
	seedPatterns(t, srv)

	// Filter lint only.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/patterns?type=lint", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp patternListResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.Total != 2 {
		t.Errorf("lint total = %d, want 2", resp.Total)
	}
	for _, item := range resp.Items {
		if item.Type != "lint" {
			t.Errorf("expected type=lint, got %q", item.Type)
		}
	}

	// Filter anti only.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/patterns?type=anti&per_page=200", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	json.NewDecoder(rec.Body).Decode(&resp)

	wantAntiTotal := baseAnti + 1
	if resp.Total != wantAntiTotal {
		t.Errorf("anti total = %d, want %d", resp.Total, wantAntiTotal)
	}
	for _, item := range resp.Items {
		if item.Type != "anti" {
			t.Errorf("expected type=anti, got %q", item.Type)
		}
	}
}

func TestGetPattern(t *testing.T) {
	srv := newTestServer(t)
	seedPatterns(t, srv)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patterns/1?type=lint", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp patternDetailResponse
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp.Pattern.Type != "lint" {
		t.Errorf("type = %q, want lint", resp.Pattern.Type)
	}
	if resp.Pattern.Rule != "errcheck" {
		t.Errorf("rule = %q, want errcheck", resp.Pattern.Rule)
	}
	if resp.History == nil {
		t.Error("history should be [] not nil")
	}
}

func TestUpdatePattern(t *testing.T) {
	srv := newTestServer(t)
	seedPatterns(t, srv)

	body := `{"type":"lint","dont_code":"updated_dont","do_code":"updated_do","rule":"errcheck","file_glob":"*.go"}`
	req := httptest.NewRequest(http.MethodPut, "/api/v1/patterns/1", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var entry patternEntry
	if err := json.NewDecoder(rec.Body).Decode(&entry); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if entry.DontCode != "updated_dont" {
		t.Errorf("dont_code = %q, want updated_dont", entry.DontCode)
	}
	if entry.DoCode != "updated_do" {
		t.Errorf("do_code = %q, want updated_do", entry.DoCode)
	}

	// Verify history was recorded.
	histReq := httptest.NewRequest(http.MethodGet, "/api/v1/patterns/1/history?type=lint", nil)
	histRec := httptest.NewRecorder()
	srv.ServeHTTP(histRec, histReq)

	var history []historyEntry
	json.NewDecoder(histRec.Body).Decode(&history)

	if len(history) != 1 {
		t.Fatalf("expected 1 history entry, got %d", len(history))
	}
	if history[0].Action != "edit" {
		t.Errorf("action = %q, want edit", history[0].Action)
	}
}

func TestDeletePattern(t *testing.T) {
	srv := newTestServer(t)
	seedPatterns(t, srv)

	// Delete lint pattern 1.
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/patterns/1?type=lint", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var okResp map[string]any
	json.NewDecoder(rec.Body).Decode(&okResp)
	if okResp["ok"] != true {
		t.Errorf("expected ok=true, got %v", okResp["ok"])
	}

	// Verify it no longer appears in default list.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/patterns?type=lint", nil)
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)

	var resp patternListResponse
	json.NewDecoder(listRec.Body).Decode(&resp)
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 lint pattern after delete, got %d", len(resp.Items))
	}

	// Verify it appears with include_deleted=true.
	listReq = httptest.NewRequest(http.MethodGet, "/api/v1/patterns?type=lint&include_deleted=true", nil)
	listRec = httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)

	json.NewDecoder(listRec.Body).Decode(&resp)
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 lint patterns with include_deleted, got %d", len(resp.Items))
	}
}

func TestRestorePattern(t *testing.T) {
	srv := newTestServer(t)
	seedPatterns(t, srv)

	// Delete lint pattern 1.
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/patterns/1?type=lint", nil)
	delRec := httptest.NewRecorder()
	srv.ServeHTTP(delRec, delReq)

	if delRec.Code != http.StatusOK {
		t.Fatalf("delete status = %d, want 200", delRec.Code)
	}

	// Restore it.
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/patterns/1/restore?type=lint", nil)
	restoreRec := httptest.NewRecorder()
	srv.ServeHTTP(restoreRec, restoreReq)

	if restoreRec.Code != http.StatusOK {
		t.Fatalf("restore status = %d, want 200; body: %s", restoreRec.Code, restoreRec.Body.String())
	}

	// Verify it appears in the default list again.
	listReq := httptest.NewRequest(http.MethodGet, "/api/v1/patterns?type=lint", nil)
	listRec := httptest.NewRecorder()
	srv.ServeHTTP(listRec, listReq)

	var resp patternListResponse
	json.NewDecoder(listRec.Body).Decode(&resp)
	if len(resp.Items) != 2 {
		t.Errorf("expected 2 lint patterns after restore, got %d", len(resp.Items))
	}
}

func TestPatternHistory(t *testing.T) {
	srv := newTestServer(t)
	seedPatterns(t, srv)

	// 1. Update (creates "edit" history).
	body := `{"type":"lint","dont_code":"v2","do_code":"v2","rule":"errcheck","file_glob":"*.go"}`
	putReq := httptest.NewRequest(http.MethodPut, "/api/v1/patterns/1", bytes.NewBufferString(body))
	putRec := httptest.NewRecorder()
	srv.ServeHTTP(putRec, putReq)

	// 2. Delete (creates "delete" history).
	delReq := httptest.NewRequest(http.MethodDelete, "/api/v1/patterns/1?type=lint", nil)
	delRec := httptest.NewRecorder()
	srv.ServeHTTP(delRec, delReq)

	// 3. Restore (creates "restore" history).
	restoreReq := httptest.NewRequest(http.MethodPost, "/api/v1/patterns/1/restore?type=lint", nil)
	restoreRec := httptest.NewRecorder()
	srv.ServeHTTP(restoreRec, restoreReq)

	// Fetch history.
	histReq := httptest.NewRequest(http.MethodGet, "/api/v1/patterns/1/history?type=lint", nil)
	histRec := httptest.NewRecorder()
	srv.ServeHTTP(histRec, histReq)

	if histRec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", histRec.Code)
	}

	var history []historyEntry
	if err := json.NewDecoder(histRec.Body).Decode(&history); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if len(history) != 3 {
		t.Fatalf("expected 3 history entries, got %d", len(history))
	}

	// Most recent first (reverse chronological).
	if history[0].Action != "restore" {
		t.Errorf("history[0].action = %q, want restore", history[0].Action)
	}
	if history[1].Action != "delete" {
		t.Errorf("history[1].action = %q, want delete", history[1].Action)
	}
	if history[2].Action != "edit" {
		t.Errorf("history[2].action = %q, want edit", history[2].Action)
	}
}

func TestSuggestions(t *testing.T) {
	srv := newTestServer(t)

	// Seed a pattern with empty do_code.
	if err := srv.store.InsertLintPattern("empty-rule", "*.go", "bad()", "", "learned"); err != nil {
		t.Fatalf("InsertLintPattern: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/suggestions", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var suggestions []suggestionEntry
	if err := json.NewDecoder(rec.Body).Decode(&suggestions); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Should have at least empty_do_code and low_frequency suggestions.
	found := map[string]bool{}
	for _, s := range suggestions {
		found[s.Type] = true
		if s.PatternIDs == nil {
			t.Errorf("pattern_ids should be [] not null for type %q", s.Type)
		}
	}
	if !found["empty_do_code"] {
		t.Error("expected empty_do_code suggestion")
	}
	if !found["low_frequency"] {
		t.Error("expected low_frequency suggestion")
	}
}

func TestGetPatternNotFound(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/patterns/999?type=lint", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// ── Domain browser tests ──────────────────────────────────────────────────

func newTestServerWithSession(t *testing.T, sessionID string) *Server {
	t.Helper()
	store, err := db.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { store.Close() })

	staticFS := fstest.MapFS{
		"index.html": &fstest.MapFile{Data: []byte("<html>admin</html>")},
	}
	return New(store, staticFS, sessionID)
}

func TestSessionFindings(t *testing.T) {
	srv := newTestServerWithSession(t, "test-session")

	// Seed session findings.
	_, _ = srv.store.InsertSessionFinding("test-session", "reviewer", "race-condition", "service.go", "Concurrent map write", "HIGH")
	_, _ = srv.store.InsertSessionFinding("test-session", "security", "injection", "handler.go", "SQL injection", "CRITICAL")
	_, _ = srv.store.InsertSessionFinding("test-session", "reviewer", "error-handling", "util.go", "Unchecked error", "MEDIUM")

	// Get all findings.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/session-findings", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var entries []map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("got %d entries, want 3", len(entries))
	}

	// Test agent filter.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/session-findings?agent=reviewer", nil)
	rec = httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if err := json.NewDecoder(rec.Body).Decode(&entries); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(entries) != 2 {
		t.Errorf("got %d entries for reviewer, want 2", len(entries))
	}

	// Verify JSON shape.
	for _, e := range entries {
		if e["agent"] != "reviewer" {
			t.Errorf("agent = %v, want reviewer", e["agent"])
		}
	}
}

func TestSessionFindingsEmpty(t *testing.T) {
	// No session ID set.
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/session-findings", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	body := rec.Body.String()
	trimmed := body
	if len(trimmed) > 0 && trimmed[len(trimmed)-1] == '\n' {
		trimmed = trimmed[:len(trimmed)-1]
	}
	if trimmed != "[]" {
		t.Errorf("empty session response = %q, want %q", body, "[]")
	}
}

func TestOWASP(t *testing.T) {
	srv := newTestServer(t)

	// Use unique category names to avoid interference from seeded data.
	_ = srv.store.InsertOWASPFinding("T01-TestCat", "*.go", "Missing auth check", "Add auth middleware")
	_ = srv.store.InsertOWASPFinding("T01-TestCat", "*.go", "Missing auth check", "Add auth middleware") // frequency=2
	_ = srv.store.InsertOWASPFinding("T01-TestCat", "*.go", "Broken access control", "Check roles")
	_ = srv.store.InsertOWASPFinding("T02-TestCat", "*.go", "SQL injection", "Use parameterized queries")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/owasp", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	categories, ok := resp["categories"].([]any)
	if !ok {
		t.Fatal("categories should be an array")
	}
	if len(categories) < 2 {
		t.Fatalf("expected at least 2 categories, got %d", len(categories))
	}

	// Verify categories are sorted alphabetically.
	for i := 1; i < len(categories); i++ {
		prev := categories[i-1].(map[string]any)["category"].(string)
		curr := categories[i].(map[string]any)["category"].(string)
		if prev > curr {
			t.Errorf("categories not sorted: %q before %q", prev, curr)
		}
	}

	// Find T01-TestCat category and verify frequency sorting.
	for _, c := range categories {
		cm := c.(map[string]any)
		if cm["category"] == "T01-TestCat" {
			findings := cm["findings"].([]any)
			if len(findings) != 2 {
				t.Errorf("T01-TestCat should have 2 findings, got %d", len(findings))
			}
			if len(findings) >= 2 {
				f0 := findings[0].(map[string]any)
				f1 := findings[1].(map[string]any)
				if f0["frequency"].(float64) < f1["frequency"].(float64) {
					t.Error("findings should be sorted by frequency DESC")
				}
			}
		}
	}
}

func TestOWASPCategoryFilter(t *testing.T) {
	srv := newTestServer(t)

	_ = srv.store.InsertOWASPFinding("T01-Filter", "*.go", "Missing auth", "Add auth")
	_ = srv.store.InsertOWASPFinding("T03-Filter", "*.go", "Injection", "Parameterize")

	req := httptest.NewRequest(http.MethodGet, "/api/v1/owasp?category=T01-Filter", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	categories := resp["categories"].([]any)
	if len(categories) != 1 {
		t.Fatalf("expected 1 category with filter, got %d", len(categories))
	}
	cat := categories[0].(map[string]any)
	if cat["category"] != "T01-Filter" {
		t.Errorf("category = %v, want T01-Filter", cat["category"])
	}
}

func TestVulnerabilities(t *testing.T) {
	srv := newTestServer(t)

	// Seed vulns and decisions.
	_ = srv.store.UpsertVulnCache("github.com/foo/bar", "CVE-2024-1234", "CRITICAL", "<1.0", "1.0.1", "RCE vulnerability", "nvd")
	_ = srv.store.UpsertVulnCache("github.com/foo/bar", "CVE-2024-1235", "HIGH", "<2.0", "2.0.0", "SSRF vulnerability", "go-vuln")
	_ = srv.store.UpsertVulnCache("github.com/baz/qux", "CVE-2024-1236", "MEDIUM", "<3.0", "3.0.0", "XSS vulnerability", "nvd")
	_ = srv.store.UpsertDepDecision("github.com/foo/bar", "upgrade", "Critical CVE", 2)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vulnerabilities", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	modules, ok := resp["modules"].([]any)
	if !ok {
		t.Fatal("modules should be an array")
	}
	if len(modules) != 2 {
		t.Fatalf("expected 2 modules, got %d", len(modules))
	}

	// Verify sorted alphabetically.
	m0 := modules[0].(map[string]any)
	m1 := modules[1].(map[string]any)
	if m0["module"].(string) > m1["module"].(string) {
		t.Error("modules should be sorted alphabetically")
	}

	// Verify github.com/baz/qux has no decision (null).
	for _, m := range modules {
		mm := m.(map[string]any)
		if mm["module"] == "github.com/baz/qux" {
			if mm["decision"] != nil {
				t.Error("baz/qux should have null decision")
			}
		}
		if mm["module"] == "github.com/foo/bar" {
			if mm["decision"] == nil {
				t.Error("foo/bar should have a decision")
			}
			vulnsList := mm["vulnerabilities"].([]any)
			if len(vulnsList) != 2 {
				t.Errorf("foo/bar should have 2 vulns, got %d", len(vulnsList))
			}
			// Verify source field is present.
			for _, v := range vulnsList {
				vm := v.(map[string]any)
				src, ok := vm["source"].(string)
				if !ok || (src != "nvd" && src != "go-vuln") {
					t.Errorf("vuln %v: expected source 'nvd' or 'go-vuln', got %q", vm["cve_id"], src)
				}
			}
		}
	}
}

func TestVulnerabilitiesEmpty(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/vulnerabilities", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	modules, ok := resp["modules"].([]any)
	if !ok {
		t.Fatal("modules should be an array, not null")
	}
	if len(modules) != 0 {
		t.Errorf("expected 0 modules, got %d", len(modules))
	}
}

func TestPrefetchStatusIdle(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prefetch-status", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["phase"] != "idle" {
		t.Errorf("expected phase 'idle', got %q", resp["phase"])
	}
}

func TestPrefetchStatusWithTracker(t *testing.T) {
	srv := newTestServer(t)
	ps := &tools.PrefetchStatus{}
	srv.prefetchStatus = ps
	ps.SetPhase("nvd", "NVD (CVE-2024-1234)")
	ps.SetProgress(5, 20)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/prefetch-status", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	if resp["phase"] != "nvd" {
		t.Errorf("expected phase 'nvd', got %q", resp["phase"])
	}
	if resp["source"] != "NVD (CVE-2024-1234)" {
		t.Errorf("expected source 'NVD (CVE-2024-1234)', got %q", resp["source"])
	}
	if int(resp["progress"].(float64)) != 5 {
		t.Errorf("expected progress 5, got %v", resp["progress"])
	}
	if int(resp["total"].(float64)) != 20 {
		t.Errorf("expected total 20, got %v", resp["total"])
	}
}

func TestRenovate(t *testing.T) {
	srv := newTestServer(t)

	// Insert a preference.
	_ = srv.store.InsertRenovatePreference("automerge", "Auto-merge minor updates", "{}", `{"automerge":true}`)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/renovate", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Verify preferences array has our entry.
	prefs, ok := resp["preferences"].([]any)
	if !ok {
		t.Fatal("preferences should be an array")
	}
	if len(prefs) < 1 {
		t.Error("expected at least 1 preference")
	}

	// Rules are pre-seeded — should have entries.
	rules, ok := resp["rules"].([]any)
	if !ok {
		t.Fatal("rules should be an array")
	}
	if len(rules) == 0 {
		t.Error("expected pre-seeded rules to be present")
	}

	// Config scores may be empty but must be array.
	scores, ok := resp["config_scores"].([]any)
	if !ok {
		t.Fatal("config_scores should be an array")
	}
	_ = scores // may be empty
}

func TestRenovateEmpty(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/renovate", nil)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}

	var resp map[string]any
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}

	// Preferences should be empty [].
	prefs, ok := resp["preferences"].([]any)
	if !ok {
		t.Fatal("preferences should be [] not null")
	}
	if len(prefs) != 0 {
		t.Errorf("expected 0 preferences, got %d", len(prefs))
	}

	// Rules are pre-seeded on :memory: creation, so they should have entries.
	rules, ok := resp["rules"].([]any)
	if !ok {
		t.Fatal("rules should be [] not null")
	}
	if len(rules) == 0 {
		t.Error("expected pre-seeded renovate rules to be present")
	}

	// Config scores should be empty [].
	scores, ok := resp["config_scores"].([]any)
	if !ok {
		t.Fatal("config_scores should be [] not null")
	}
	if len(scores) != 0 {
		t.Errorf("expected 0 config_scores, got %d", len(scores))
	}
}
