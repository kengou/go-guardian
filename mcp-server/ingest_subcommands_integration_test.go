package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
	_ "modernc.org/sqlite"
)

// seedInboxProject creates a minimal inbox layout under root and returns
// the guardian DB path + the inbox dir path. Used by every inbox-ingest
// integration subtest. Helper name is distinct from seedProject (defined in
// scan_subcommands_integration_test.go) because both live in package main.
func seedInboxProject(t *testing.T, root string) (dbPath, inboxDir string) {
	t.Helper()
	gdir := filepath.Join(root, ".go-guardian")
	if err := os.MkdirAll(gdir, 0o700); err != nil {
		t.Fatalf("mkdir .go-guardian: %v", err)
	}
	inboxDir = filepath.Join(gdir, "inbox")
	if err := os.MkdirAll(inboxDir, 0o700); err != nil {
		t.Fatalf("mkdir inbox: %v", err)
	}
	dbPath = filepath.Join(gdir, "guardian.db")
	store, err := db.NewStore(dbPath)
	if err != nil {
		t.Fatalf("seed NewStore: %v", err)
	}
	_ = store.Close()
	return dbPath, inboxDir
}

// runIngest is a thin wrapper that calls Dispatch with an ingest arg list
// and returns exit code + captured stdout/stderr.
func runIngest(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	full := append([]string{"ingest"}, args...)
	exit := Dispatch(full, stdout, stderr)
	return exit, stdout.String(), stderr.String()
}

// writeInboxDoc writes an inbox document at path with the given frontmatter
// map and body. Keys are emitted in deterministic order.
func writeInboxDoc(t *testing.T, path string, frontmatter map[string]string, body string) {
	t.Helper()
	var sb strings.Builder
	sb.WriteString("---\n")
	// Emit in a stable order so tests are deterministic.
	order := []string{
		"kind", "rule", "severity", "file_glob", "dont_code", "do_code",
		"description", "category", "file_path",
		"agent", "finding_type",
		"dont_config", "do_config",
	}
	seen := map[string]bool{}
	for _, k := range order {
		if v, ok := frontmatter[k]; ok {
			sb.WriteString(k)
			sb.WriteString(": ")
			sb.WriteString(v)
			sb.WriteString("\n")
			seen[k] = true
		}
	}
	for k, v := range frontmatter {
		if seen[k] {
			continue
		}
		sb.WriteString(k)
		sb.WriteString(": ")
		sb.WriteString(v)
		sb.WriteString("\n")
	}
	sb.WriteString("---\n\n")
	sb.WriteString(body)
	if err := os.WriteFile(path, []byte(sb.String()), 0o600); err != nil {
		t.Fatalf("write inbox doc %s: %v", path, err)
	}
}

func TestIngestSubcommands_IntegrationScenarios(t *testing.T) {
	t.Run("Scenario: Ingest on an empty inbox is a safe no-op", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		exit, stdout, stderr := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("empty inbox exit=%d, want 0; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "nothing to ingest") {
			t.Errorf("expected 'nothing to ingest' in stdout; got: %s", stdout)
		}

		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		defer store.Close()
		_, total, err := store.GetAllLintPatterns("", "", "", "", false, 100, 0)
		if err != nil {
			t.Fatalf("GetAllLintPatterns: %v", err)
		}
		if total != 0 {
			t.Errorf("expected 0 lint patterns on empty inbox, got %d", total)
		}
	})

	t.Run("Scenario: A lint learning document is ingested", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		docPath := filepath.Join(inboxDir, "lint-20260409T120000Z-abc123.md")
		writeInboxDoc(t, docPath, map[string]string{
			"kind":      "lint",
			"rule":      "errcheck",
			"severity":  "WARNING",
			"file_glob": "*.go",
			"dont_code": "defer f.Close()",
			"do_code":   "defer func() { _ = f.Close() }()",
		}, "")

		exit, stdout, stderr := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("lint ingest exit=%d; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "lint: 1") {
			t.Errorf("expected 'lint: 1' in stdout; got: %s", stdout)
		}

		// Original removed, processed sibling exists.
		if _, err := os.Stat(docPath); !os.IsNotExist(err) {
			t.Errorf("expected inbox doc removed; stat err=%v", err)
		}
		processed := filepath.Join(inboxDir, "processed", "lint-20260409T120000Z-abc123.md")
		if _, err := os.Stat(processed); err != nil {
			t.Errorf("expected processed doc at %s: %v", processed, err)
		}

		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		defer store.Close()
		_, total, err := store.GetAllLintPatterns("", "", "", "", false, 100, 0)
		if err != nil {
			t.Fatalf("GetAllLintPatterns: %v", err)
		}
		if total != 1 {
			t.Errorf("expected 1 lint pattern after ingest, got %d", total)
		}
	})

	t.Run("Scenario: A review finding document is ingested", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		docPath := filepath.Join(inboxDir, "review-20260409T120001Z-def456.md")
		writeInboxDoc(t, docPath, map[string]string{
			"kind":        "review",
			"description": "unsynchronised map access",
			"severity":    "HIGH",
			"category":    "concurrency",
			"file_path":   "service.go",
			"dont_code":   "m[key] = val",
			"do_code":     "mu.Lock(); m[key] = val; mu.Unlock()",
		}, "")

		exit, stdout, stderr := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("review ingest exit=%d; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "review: 1") {
			t.Errorf("expected 'review: 1' in stdout; got: %s", stdout)
		}

		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		defer store.Close()
		patterns, _, err := store.GetAllLintPatterns("", "review", "", "", false, 100, 0)
		if err != nil {
			t.Fatalf("GetAllLintPatterns: %v", err)
		}
		if len(patterns) == 0 {
			t.Errorf("expected at least one lint_patterns row with source=review")
		}
	})

	t.Run("Scenario: A generic report-finding document is ingested", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		// Write session-id next to the DB so dispatchIngest picks up a session.
		sidPath := filepath.Join(filepath.Dir(dbPath), "session-id")
		if err := os.WriteFile(sidPath, []byte("sess-test-001\n"), 0o600); err != nil {
			t.Fatalf("write session-id: %v", err)
		}

		docPath := filepath.Join(inboxDir, "finding-20260409T120002Z-ghi789.md")
		writeInboxDoc(t, docPath, map[string]string{
			"kind":         "finding",
			"agent":        "reviewer",
			"finding_type": "race-condition",
			"file_path":    "handler.go",
			"description":  "goroutine leak on panic path",
			"severity":     "MEDIUM",
		}, "")

		exit, stdout, stderr := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("finding ingest exit=%d; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "finding: 1") {
			t.Errorf("expected 'finding: 1' in stdout; got: %s", stdout)
		}

		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		defer store.Close()
		findings, err := store.GetSessionFindings("sess-test-001", "")
		if err != nil {
			t.Fatalf("GetSessionFindings: %v", err)
		}
		if len(findings) != 1 {
			t.Errorf("expected 1 session finding, got %d", len(findings))
		}
	})

	t.Run("Scenario: Multiple documents are ingested in a single run", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		sidPath := filepath.Join(filepath.Dir(dbPath), "session-id")
		if err := os.WriteFile(sidPath, []byte("sess-multi\n"), 0o600); err != nil {
			t.Fatalf("write session-id: %v", err)
		}

		writeInboxDoc(t, filepath.Join(inboxDir, "lint-1.md"), map[string]string{
			"kind": "lint", "rule": "errcheck", "file_glob": "*.go",
			"dont_code": "a()", "do_code": "_ = a()",
		}, "")
		writeInboxDoc(t, filepath.Join(inboxDir, "review-1.md"), map[string]string{
			"kind": "review", "description": "X", "severity": "LOW", "category": "design",
			"dont_code": "x", "do_code": "y",
		}, "")
		writeInboxDoc(t, filepath.Join(inboxDir, "finding-1.md"), map[string]string{
			"kind": "finding", "agent": "linter", "finding_type": "errcheck",
			"description": "z",
		}, "")
		writeInboxDoc(t, filepath.Join(inboxDir, "renovate-pref-1.md"), map[string]string{
			"kind": "renovate-pref", "category": "automerge",
			"description": "auto-merge patch updates",
		}, "")

		exit, stdout, stderr := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("multi ingest exit=%d; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		for _, want := range []string{"lint: 1", "review: 1", "finding: 1", "renovate-pref: 1"} {
			if !strings.Contains(stdout, want) {
				t.Errorf("stdout missing %q; got: %s", want, stdout)
			}
		}
		for _, name := range []string{"lint-1.md", "review-1.md", "finding-1.md", "renovate-pref-1.md"} {
			if _, err := os.Stat(filepath.Join(inboxDir, "processed", name)); err != nil {
				t.Errorf("expected %s in processed/: %v", name, err)
			}
		}
	})

	t.Run("Scenario: A malformed document does not block siblings", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		writeInboxDoc(t, filepath.Join(inboxDir, "lint-good.md"), map[string]string{
			"kind": "lint", "rule": "errcheck", "file_glob": "*.go",
			"dont_code": "x", "do_code": "y",
		}, "")
		// Malformed: no closing --- delimiter.
		if err := os.WriteFile(filepath.Join(inboxDir, "lint-bad.md"),
			[]byte("---\nkind: lint\nrule: errcheck\n"), 0o600); err != nil {
			t.Fatalf("write bad doc: %v", err)
		}

		exit, stdout, _ := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("mixed ingest exit=%d; stdout=%s", exit, stdout)
		}
		if !strings.Contains(stdout, "lint: 1") {
			t.Errorf("expected 'lint: 1' in stdout; got: %s", stdout)
		}
		if !strings.Contains(stdout, "1 failed") {
			t.Errorf("expected '1 failed' in stdout; got: %s", stdout)
		}
		if _, err := os.Stat(filepath.Join(inboxDir, "processed", "lint-good.md")); err != nil {
			t.Errorf("expected good doc in processed/: %v", err)
		}
		badPath := filepath.Join(inboxDir, "failed", "lint-bad.md")
		badData, err := os.ReadFile(badPath)
		if err != nil {
			t.Errorf("expected bad doc in failed/: %v", err)
		} else if !strings.HasPrefix(string(badData), "<!-- ingest error:") {
			t.Errorf("expected failed doc to start with error header; got: %q", string(badData)[:40])
		}
	})

	t.Run("Scenario: Already-processed documents are not ingested a second time", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		// Pre-populate processed/ with a doc AND drop a sibling in inbox/
		// with the same basename.
		if err := os.MkdirAll(filepath.Join(inboxDir, "processed"), 0o700); err != nil {
			t.Fatalf("mkdir processed: %v", err)
		}
		basename := "lint-already.md"
		processedPath := filepath.Join(inboxDir, "processed", basename)
		if err := os.WriteFile(processedPath, []byte("---\nkind: lint\n---\n"), 0o600); err != nil {
			t.Fatalf("write processed doc: %v", err)
		}
		inboxPath := filepath.Join(inboxDir, basename)
		writeInboxDoc(t, inboxPath, map[string]string{
			"kind": "lint", "rule": "errcheck", "file_glob": "*.go",
			"dont_code": "x", "do_code": "y",
		}, "")

		exit, stdout, _ := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("idempotent run exit=%d; stdout=%s", exit, stdout)
		}
		// The inbox copy should have been removed (idempotent skip).
		if _, err := os.Stat(inboxPath); !os.IsNotExist(err) {
			t.Errorf("expected inbox copy removed on idempotent re-run; stat err=%v", err)
		}
		// No new lint pattern inserted — the skip must not touch the DB.
		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		defer store.Close()
		_, total, err := store.GetAllLintPatterns("", "", "", "", false, 100, 0)
		if err != nil {
			t.Fatalf("GetAllLintPatterns: %v", err)
		}
		if total != 0 {
			t.Errorf("expected 0 lint patterns on idempotent skip, got %d", total)
		}
	})

	t.Run("Scenario: Ingest preserves the existing learning state", func(t *testing.T) {
		root := t.TempDir()
		dbPath, inboxDir := seedInboxProject(t, root)

		// Seed a pre-existing pattern.
		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("open store for seed: %v", err)
		}
		if err := store.InsertLintPattern("manual-rule", "*.go", "seed dont", "seed do", "manual"); err != nil {
			t.Fatalf("seed InsertLintPattern: %v", err)
		}
		store.Close()

		writeInboxDoc(t, filepath.Join(inboxDir, "lint-new.md"), map[string]string{
			"kind": "lint", "rule": "errcheck", "file_glob": "*.go",
			"dont_code": "new dont", "do_code": "new do",
		}, "")

		exit, stdout, _ := runIngest(t, "--db", dbPath, "--inbox-dir", inboxDir)
		if exit != 0 {
			t.Fatalf("preservation ingest exit=%d; stdout=%s", exit, stdout)
		}

		store2, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("reopen store: %v", err)
		}
		defer store2.Close()
		_, total, err := store2.GetAllLintPatterns("", "", "", "", false, 100, 0)
		if err != nil {
			t.Fatalf("GetAllLintPatterns: %v", err)
		}
		if total != 2 {
			t.Errorf("expected 2 lint patterns (1 seeded + 1 ingested), got %d", total)
		}
	})
}
