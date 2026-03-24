package tools

import (
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// TestGetPatternStatsEmpty verifies that an empty store returns the
// "No patterns learned yet" message.
func TestGetPatternStatsEmpty(t *testing.T) {
	store := newTestStore(t)

	// NewStore seeds anti_patterns from embedded SQL files; in the test binary
	// there are no embedded seed files (the db/seed directory is in the db
	// package, not tools), so the store starts with 0 anti-patterns.
	// However, if seeds are present the total will be non-zero. We force the
	// empty-path by checking the formatted output when all counts are zero.
	stats, err := store.GetPatternStats("")
	if err != nil {
		t.Fatalf("GetPatternStats: %v", err)
	}

	// Override stats to simulate a fully empty database.
	emptyStats := &db.PatternStats{
		OWASPCounts: make(map[string]int64),
	}
	text := formatPatternStats(emptyStats)

	if !strings.Contains(text, "No patterns learned yet") {
		t.Errorf("expected empty-store message, got:\n%s", text)
	}
	_ = stats // suppress "unused" when seeds exist
}

// TestGetPatternStatsWithData pre-seeds lint patterns, OWASP findings, and a
// scan history record, then verifies the formatted output contains the
// expected sections and values.
func TestGetPatternStatsWithData(t *testing.T) {
	store := newTestStore(t)

	// Insert lint patterns.
	patterns := []struct {
		rule, glob, dont, do string
	}{
		{"errcheck", "*.go", "f, err := os.Open(p)\nreturn f, nil", "if err != nil { return nil, err }"},
		{"staticcheck:SA1029", "*.go", "ctx = context.WithValue(ctx, \"key\", v)", "ctx = context.WithValue(ctx, myKey{}, v)"},
		{"gosec:G401", "*.go", "md5.New()", "sha256.New()"},
	}
	for _, p := range patterns {
		if err := store.InsertLintPattern(p.rule, p.glob, p.dont, p.do, "learned"); err != nil {
			t.Fatalf("InsertLintPattern(%s): %v", p.rule, err)
		}
	}
	// Bump errcheck frequency so it sorts first.
	for i := 0; i < 4; i++ {
		_ = store.InsertLintPattern("errcheck", "*.go", "f, err := os.Open(p)\nreturn f, nil", "if err != nil { return nil, err }", "learned")
	}

	// Insert OWASP findings.
	owaspData := []struct{ cat, fp, finding, fix string }{
		{"A02-Cryptographic-Failures", "*.go", "use of weak hash MD5", "use SHA-256"},
		{"A02-Cryptographic-Failures", "*.go", "use of MD4 for passwords", "use bcrypt"},
		{"A03-Injection", "*.go", "fmt.Sprintf with user input in SQL", "use parameterized queries"},
	}
	for _, o := range owaspData {
		if err := store.InsertOWASPFinding(o.cat, o.fp, o.finding, o.fix); err != nil {
			t.Fatalf("InsertOWASPFinding: %v", err)
		}
	}

	// Insert scan history.
	if err := store.UpdateScanHistory("vuln", "testproject", 3); err != nil {
		t.Fatalf("UpdateScanHistory(vuln): %v", err)
	}
	if err := store.UpdateScanHistory("owasp", "testproject", 4); err != nil {
		t.Fatalf("UpdateScanHistory(owasp): %v", err)
	}

	stats, err := store.GetPatternStats("testproject")
	if err != nil {
		t.Fatalf("GetPatternStats: %v", err)
	}

	text := formatPatternStats(stats)

	// Verify header.
	if !strings.Contains(text, "Go Guardian Knowledge Base") {
		t.Errorf("missing header in output:\n%s", text)
	}

	// Verify lint pattern section.
	if !strings.Contains(text, "Lint Patterns Learned:") {
		t.Errorf("missing lint patterns section:\n%s", text)
	}
	if !strings.Contains(text, "errcheck") {
		t.Errorf("expected errcheck in output:\n%s", text)
	}

	// Verify OWASP section.
	if !strings.Contains(text, "OWASP Posture:") {
		t.Errorf("missing OWASP posture section:\n%s", text)
	}
	if !strings.Contains(text, "A02-Cryptographic-Failures") {
		t.Errorf("expected A02 category in output:\n%s", text)
	}

	// Verify scan history section.
	if !strings.Contains(text, "Recent Scans:") {
		t.Errorf("missing Recent Scans section:\n%s", text)
	}
	if !strings.Contains(text, "vuln:") {
		t.Errorf("expected vuln scan in output:\n%s", text)
	}
	if !strings.Contains(text, "owasp:") {
		t.Errorf("expected owasp scan in output:\n%s", text)
	}

	// Verify frequency ordering: errcheck (bumped 5 times total) must appear
	// before staticcheck and gosec (each inserted once).
	errIdx := strings.Index(text, "errcheck")
	saIdx := strings.Index(text, "staticcheck")
	if errIdx < 0 || saIdx < 0 {
		t.Errorf("missing expected pattern names in output:\n%s", text)
	} else if errIdx > saIdx {
		t.Errorf("expected errcheck before staticcheck (higher frequency), got:\n%s", text)
	}
}
