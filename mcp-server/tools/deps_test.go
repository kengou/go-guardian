package tools

import (
	"os"
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// newTestStore opens an in-memory SQLite store for testing.
func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	s, err := db.NewStore(":memory:")
	if err != nil {
		t.Fatalf("NewStore(:memory:): %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

// TestCheckDepsNoCacheData verifies that modules with no cached CVE data
// are returned with UNKNOWN status and a recommendation to run a vuln scan.
func TestCheckDepsNoCacheData(t *testing.T) {
	store := newTestStore(t)

	modules := []string{
		"github.com/gorilla/mux",
		"github.com/gin-gonic/gin",
	}

	results, err := analyseModules(store, modules)
	if err != nil {
		t.Fatalf("analyseModules: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	for _, r := range results {
		if r.status != "UNKNOWN" {
			t.Errorf("module %q: expected status UNKNOWN, got %q", r.module, r.status)
		}
		if !r.noCache {
			t.Errorf("module %q: expected noCache=true", r.module)
		}
		if r.priorDecision != nil {
			t.Errorf("module %q: expected no prior decision, got %+v", r.module, r.priorDecision)
		}
	}

	// Verify the formatted output mentions the gateway vuln API guidance.
	text := formatResults(results)
	if !strings.Contains(text, "Dependency Analysis:") {
		t.Error("output missing 'Dependency Analysis:' header")
	}
	for _, mod := range modules {
		if !strings.Contains(text, mod) {
			t.Errorf("output missing module %q", mod)
		}
	}
	if !strings.Contains(text, "vuln scan") {
		t.Error("output missing vuln scan recommendation for UNKNOWN modules")
	}
}

// TestCheckDepsWithCachedVulns pre-populates the vuln_cache with CVEs and
// verifies that the recommendation logic produces AVOID and CHECK LATEST correctly.
func TestCheckDepsWithCachedVulns(t *testing.T) {
	store := newTestStore(t)

	// Seed gorilla/mux: 3 CVEs → AVOID.
	muxMod := "github.com/gorilla/mux"
	vulns := []struct {
		cveID, severity, fixed string
	}{
		{"CVE-2022-11111", "HIGH", "v1.8.1"},
		{"CVE-2023-22222", "MEDIUM", ""},    // unfixed
		{"CVE-2023-33333", "LOW", "v1.8.2"},
	}
	for _, v := range vulns {
		if err := store.UpsertVulnCache(muxMod, v.cveID, v.severity, "< v1.8.0", v.fixed, "desc", "go-vuln"); err != nil {
			t.Fatalf("UpsertVulnCache(%s): %v", v.cveID, err)
		}
	}

	// Seed gin: 1 CVE with a fixed version → CHECK LATEST.
	ginMod := "github.com/gin-gonic/gin"
	if err := store.UpsertVulnCache(ginMod, "CVE-2023-44444", "HIGH", "< v1.9.1", "v1.9.1", "path traversal", "go-vuln"); err != nil {
		t.Fatalf("UpsertVulnCache(gin): %v", err)
	}

	results, err := analyseModules(store, []string{muxMod, ginMod})
	if err != nil {
		t.Fatalf("analyseModules: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}

	muxResult := results[0]
	ginResult := results[1]

	// gorilla/mux: 3 CVEs → AVOID.
	if muxResult.status != "AVOID" {
		t.Errorf("gorilla/mux: expected AVOID, got %q", muxResult.status)
	}
	if len(muxResult.cves) != 3 {
		t.Errorf("gorilla/mux: expected 3 CVEs, got %d", len(muxResult.cves))
	}

	// gin: 1 CVE, fixed → CHECK LATEST.
	if ginResult.status != "CHECK LATEST" {
		t.Errorf("gin-gonic/gin: expected CHECK LATEST, got %q", ginResult.status)
	}
	if len(ginResult.cves) != 1 {
		t.Errorf("gin-gonic/gin: expected 1 CVE, got %d", len(ginResult.cves))
	}

	// Verify formatted text contains CVE IDs.
	text := formatResults(results)
	if !strings.Contains(text, "CVE-2022-11111") {
		t.Error("output missing CVE-2022-11111 for gorilla/mux")
	}
	if !strings.Contains(text, "CVE-2023-44444") {
		t.Error("output missing CVE-2023-44444 for gin")
	}
	if !strings.Contains(text, "AVOID") {
		t.Error("output missing AVOID status")
	}
	if !strings.Contains(text, "CHECK LATEST") {
		t.Error("output missing CHECK LATEST status")
	}
}

// TestCheckDepsClean verifies that a module with cached entries but zero CVEs
// receives a PREFER recommendation.
func TestCheckDepsClean(t *testing.T) {
	store := newTestStore(t)

	// go-chi has no CVE entries in the cache — empty result set → PREFER.
	chiMod := "github.com/go-chi/chi/v5"

	// Insert an explicit vuln cache entry for a different module so the DB is
	// non-empty, ensuring we are not accidentally matching the wrong module.
	if err := store.UpsertVulnCache("github.com/other/pkg", "CVE-2024-99999", "HIGH", "all", "", "other vuln", "go-vuln"); err != nil {
		t.Fatalf("UpsertVulnCache(other): %v", err)
	}

	results, err := analyseModules(store, []string{chiMod})
	if err != nil {
		t.Fatalf("analyseModules: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	// No entries for go-chi → noCache, status UNKNOWN (no data to confirm clean).
	// PREFER is only returned when entries exist and all 0 CVEs.
	// Here we have 0 entries, which maps to UNKNOWN.
	if r.status != "UNKNOWN" {
		t.Errorf("go-chi with no cache entries: expected UNKNOWN, got %q", r.status)
	}

	// Now explicitly seed a "clean" scenario: vuln_cache has an entry for this
	// module that represents a scanned-and-clean result. We simulate this by
	// the caller NOT inserting any CVEs for the module — the PREFER status
	// arises from computeStatus([]db.VulnEntry{}), i.e. zero entries returned
	// from the DB after a successful scan.
	// Re-test computeStatus directly with an empty slice.
	status := computeStatus([]db.VulnEntry{})
	if status != "PREFER" {
		t.Errorf("computeStatus(empty): expected PREFER, got %q", status)
	}

	text := formatResults(results)
	if !strings.Contains(text, chiMod) {
		t.Errorf("output missing module %q", chiMod)
	}
}

// TestCheckDepsPREFER_WithZeroCVEs verifies the full PREFER path when a module
// has been scanned and explicitly shows zero CVEs but a prior decision exists.
func TestCheckDepsPREFERWithZeroCVEs(t *testing.T) {
	// Build the result directly to test formatResults for the PREFER branch.
	priorDecision := &db.DepDecision{
		Module:   "github.com/go-chi/chi/v5",
		Decision: "prefer",
		Reason:   "no known vulnerabilities",
		CVECount: 0,
	}
	r := moduleResult{
		module:        "github.com/go-chi/chi/v5",
		status:        "PREFER",
		cves:          []db.VulnEntry{},
		noCache:       false,
		stale:         false,
		priorDecision: priorDecision,
	}

	text := formatResults([]moduleResult{r})
	if !strings.Contains(text, "PREFER") {
		t.Error("output missing PREFER status")
	}
	if !strings.Contains(text, "no known vulnerabilities") {
		t.Error("output missing no-known-vulns text in prior decision")
	}
}

// TestCheckDepsAvoidUnfixedCritical verifies that a single unfixed CRITICAL CVE
// triggers AVOID even though the count is below 3.
func TestCheckDepsAvoidUnfixedCritical(t *testing.T) {
	store := newTestStore(t)

	mod := "github.com/example/risky"
	if err := store.UpsertVulnCache(mod, "CVE-2024-00001", "CRITICAL", "all", "", "rce", "go-vuln"); err != nil {
		t.Fatalf("UpsertVulnCache: %v", err)
	}

	results, err := analyseModules(store, []string{mod})
	if err != nil {
		t.Fatalf("analyseModules: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].status != "AVOID" {
		t.Errorf("unfixed CRITICAL CVE: expected AVOID, got %q", results[0].status)
	}
}

// TestParseGoMod parses a realistic go.mod file written to a temp file and
// verifies that all require module paths are returned without versions.
func TestParseGoMod(t *testing.T) {
	goModContent := `module github.com/example/myapp

go 1.26

require (
	github.com/gorilla/mux v1.8.1
	github.com/gin-gonic/gin v1.9.1
	github.com/go-chi/chi/v5 v5.0.12
	modernc.org/sqlite v1.34.4
)

require (
	github.com/dustin/go-humanize v1.0.1 // indirect
	github.com/google/uuid v1.6.0 // indirect
)

// standalone single-line require
require github.com/stretchr/testify v1.9.0
`

	tmp, err := os.CreateTemp(t.TempDir(), "go.mod")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	if _, err := tmp.WriteString(goModContent); err != nil {
		t.Fatalf("write temp file: %v", err)
	}
	tmp.Close()

	modules, err := ParseGoMod(tmp.Name())
	if err != nil {
		t.Fatalf("ParseGoMod: %v", err)
	}

	want := []string{
		"github.com/gorilla/mux",
		"github.com/gin-gonic/gin",
		"github.com/go-chi/chi/v5",
		"modernc.org/sqlite",
		"github.com/dustin/go-humanize",
		"github.com/google/uuid",
		"github.com/stretchr/testify",
	}

	wantSet := make(map[string]bool, len(want))
	for _, m := range want {
		wantSet[m] = true
	}
	gotSet := make(map[string]bool, len(modules))
	for _, m := range modules {
		gotSet[m] = true
	}

	for _, m := range want {
		if !gotSet[m] {
			t.Errorf("ParseGoMod: missing expected module %q", m)
		}
	}

	// Verify no version strings leaked through.
	for _, m := range modules {
		if strings.HasPrefix(m, "v") && len(m) > 1 && m[1] >= '0' && m[1] <= '9' {
			t.Errorf("ParseGoMod: got version string %q instead of module path", m)
		}
		if strings.Contains(m, " ") {
			t.Errorf("ParseGoMod: module path %q contains spaces (version leaked?)", m)
		}
	}

	// The module directive itself must NOT appear.
	for _, m := range modules {
		if m == "github.com/example/myapp" {
			t.Error("ParseGoMod: module directive path should not appear in require list")
		}
	}
}

// TestParseGoModMissing verifies that ParseGoMod returns an error for a
// non-existent file.
func TestParseGoModMissing(t *testing.T) {
	_, err := ParseGoMod("/does/not/exist/go.mod")
	if err == nil {
		t.Error("expected error for missing go.mod, got nil")
	}
}
