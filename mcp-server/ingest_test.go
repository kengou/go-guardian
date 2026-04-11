package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"

	_ "modernc.org/sqlite"
)

func writeTestDoc(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

func TestParseInboxDoc_Lint(t *testing.T) {
	dir := t.TempDir()
	path := writeTestDoc(t, dir, "lint-1.md", `---
kind: lint
rule: errcheck
file_glob: "*.go"
dont_code: defer f.Close()
do_code: _ = f.Close()
---

Prose body here.
`)
	doc, err := parseInboxDoc(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if doc.kind != "lint" {
		t.Errorf("kind=%q want lint", doc.kind)
	}
	if doc.fields["rule"] != "errcheck" {
		t.Errorf("rule=%q want errcheck", doc.fields["rule"])
	}
	if doc.fields["file_glob"] != "*.go" {
		t.Errorf("file_glob=%q want *.go (quotes stripped)", doc.fields["file_glob"])
	}
	if !strings.Contains(doc.body, "Prose body") {
		t.Errorf("body missing prose; got: %q", doc.body)
	}
}

func TestParseInboxDoc_BlockScalar(t *testing.T) {
	dir := t.TempDir()
	path := writeTestDoc(t, dir, "lint-block.md", `---
kind: lint
rule: errcheck
file_glob: "*.go"
dont_code: |
  defer f.Close()
  return nil
do_code: |
  defer func() {
      if err := f.Close(); err != nil {
          log.Printf("close: %v", err)
      }
  }()
  return nil
---
`)
	doc, err := parseInboxDoc(path)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !strings.Contains(doc.fields["dont_code"], "defer f.Close()") ||
		!strings.Contains(doc.fields["dont_code"], "return nil") {
		t.Errorf("dont_code block scalar not joined: %q", doc.fields["dont_code"])
	}
	if !strings.Contains(doc.fields["do_code"], "log.Printf") {
		t.Errorf("do_code block scalar not captured: %q", doc.fields["do_code"])
	}
}

func TestParseInboxDoc_MissingKind(t *testing.T) {
	dir := t.TempDir()
	path := writeTestDoc(t, dir, "no-kind.md", `---
rule: errcheck
---
`)
	if _, err := parseInboxDoc(path); err == nil {
		t.Errorf("expected error for missing kind")
	}
}

func TestParseInboxDoc_MissingClosingDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := writeTestDoc(t, dir, "bad.md", `---
kind: lint
rule: errcheck
`)
	if _, err := parseInboxDoc(path); err == nil {
		t.Errorf("expected error for missing closing ---")
	}
}

func TestParseInboxDoc_MissingOpeningDelimiter(t *testing.T) {
	dir := t.TempDir()
	path := writeTestDoc(t, dir, "plain.md", `kind: lint
`)
	if _, err := parseInboxDoc(path); err == nil {
		t.Errorf("expected error for missing opening ---")
	}
}

func TestParseIngestArgs_Defaults(t *testing.T) {
	var stderr strings.Builder
	opts, exit := parseIngestArgs([]string{"--db", "/tmp/x.db"}, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if opts.dbPath != "/tmp/x.db" {
		t.Errorf("dbPath=%q", opts.dbPath)
	}
	if opts.inboxDir != "/tmp/inbox" {
		t.Errorf("inboxDir default=%q want /tmp/inbox", opts.inboxDir)
	}
}

func TestParseIngestArgs_ExplicitInboxDir(t *testing.T) {
	var stderr strings.Builder
	opts, exit := parseIngestArgs([]string{"--db", "/tmp/x.db", "--inbox-dir", "/elsewhere"}, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if opts.inboxDir != "/elsewhere" {
		t.Errorf("inboxDir=%q want /elsewhere", opts.inboxDir)
	}
}

func TestEnsureInboxDirs(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	if err := ensureInboxDirs(inbox); err != nil {
		t.Fatalf("ensureInboxDirs: %v", err)
	}
	for _, sub := range []string{"", "processed", "failed"} {
		if _, err := os.Stat(filepath.Join(inbox, sub)); err != nil {
			t.Errorf("expected %s to exist: %v", sub, err)
		}
	}
}

func TestMoveToProcessed(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	if err := ensureInboxDirs(inbox); err != nil {
		t.Fatalf("ensureInboxDirs: %v", err)
	}
	src := filepath.Join(inbox, "lint-move.md")
	if err := os.WriteFile(src, []byte("body"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := moveToProcessed(inbox, src); err != nil {
		t.Fatalf("moveToProcessed: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected source removed")
	}
	if _, err := os.Stat(filepath.Join(inbox, "processed", "lint-move.md")); err != nil {
		t.Errorf("expected processed/lint-move.md: %v", err)
	}
}

func TestMoveToFailed_PrependsHeader(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	if err := ensureInboxDirs(inbox); err != nil {
		t.Fatalf("ensureInboxDirs: %v", err)
	}
	src := filepath.Join(inbox, "lint-bad.md")
	if err := os.WriteFile(src, []byte("original content\n"), 0o600); err != nil {
		t.Fatalf("write src: %v", err)
	}
	if err := moveToFailed(inbox, src, "parse error: missing kind"); err != nil {
		t.Fatalf("moveToFailed: %v", err)
	}
	if _, err := os.Stat(src); !os.IsNotExist(err) {
		t.Errorf("expected source removed")
	}
	data, err := os.ReadFile(filepath.Join(inbox, "failed", "lint-bad.md"))
	if err != nil {
		t.Fatalf("read failed: %v", err)
	}
	if !strings.HasPrefix(string(data), "<!-- ingest error: parse error: missing kind -->") {
		t.Errorf("expected error header prefix; got: %q", string(data)[:60])
	}
	if !strings.Contains(string(data), "original content") {
		t.Errorf("expected original content preserved; got: %q", string(data))
	}
}

func TestProcessedHasSibling(t *testing.T) {
	root := t.TempDir()
	inbox := filepath.Join(root, "inbox")
	if err := ensureInboxDirs(inbox); err != nil {
		t.Fatalf("ensureInboxDirs: %v", err)
	}
	src := filepath.Join(inbox, "lint-sib.md")
	if processedHasSibling(inbox, src) {
		t.Errorf("expected no sibling for fresh file")
	}
	if err := os.WriteFile(
		filepath.Join(inbox, "processed", "lint-sib.md"),
		[]byte("already-there"), 0o600); err != nil {
		t.Fatalf("seed processed sibling: %v", err)
	}
	if !processedHasSibling(inbox, src) {
		t.Errorf("expected sibling present after seeding processed/")
	}
}

func newTestStore(t *testing.T) *db.Store {
	t.Helper()
	store, err := db.NewStore(filepath.Join(t.TempDir(), "guardian.db"))
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store
}

func TestRouteLintDoc(t *testing.T) {
	store := newTestStore(t)
	doc := &inboxDoc{
		kind: "lint",
		fields: map[string]string{
			"rule": "errcheck", "file_glob": "*.go",
			"dont_code": "x", "do_code": "y",
		},
	}
	if err := routeLintDoc(store, doc); err != nil {
		t.Fatalf("routeLintDoc: %v", err)
	}
	_, total, err := store.GetAllLintPatterns("", "", "", "", false, 100, 0)
	if err != nil {
		t.Fatalf("GetAllLintPatterns: %v", err)
	}
	if total != 1 {
		t.Errorf("expected 1 lint pattern, got %d", total)
	}
}

func TestRouteLintDoc_MissingFields(t *testing.T) {
	store := newTestStore(t)
	doc := &inboxDoc{kind: "lint", fields: map[string]string{"rule": "errcheck"}}
	if err := routeLintDoc(store, doc); err == nil {
		t.Errorf("expected error for missing fields")
	}
}

func TestRouteReviewDoc(t *testing.T) {
	store := newTestStore(t)
	doc := &inboxDoc{
		kind: "review",
		fields: map[string]string{
			"description": "d", "severity": "HIGH", "category": "concurrency",
			"dont_code": "x", "do_code": "y", "file_path": "a.go",
		},
	}
	if err := routeReviewDoc(store, doc); err != nil {
		t.Fatalf("routeReviewDoc: %v", err)
	}
	patterns, _, err := store.GetAllLintPatterns("", "review", "", "", false, 100, 0)
	if err != nil {
		t.Fatalf("GetAllLintPatterns: %v", err)
	}
	if len(patterns) == 0 {
		t.Errorf("expected at least one review pattern")
	}
}

func TestRouteFindingDoc(t *testing.T) {
	store := newTestStore(t)
	doc := &inboxDoc{
		kind: "finding",
		fields: map[string]string{
			"agent": "reviewer", "finding_type": "race", "description": "d",
		},
	}
	if err := routeFindingDoc(store, "sess-1", doc); err != nil {
		t.Fatalf("routeFindingDoc: %v", err)
	}
	findings, err := store.GetSessionFindings("sess-1", "")
	if err != nil {
		t.Fatalf("GetSessionFindings: %v", err)
	}
	if len(findings) != 1 {
		t.Errorf("expected 1 session finding, got %d", len(findings))
	}
}

func TestRouteFindingDoc_NoSession(t *testing.T) {
	store := newTestStore(t)
	doc := &inboxDoc{
		kind: "finding",
		fields: map[string]string{
			"agent": "reviewer", "finding_type": "race", "description": "d",
		},
	}
	if err := routeFindingDoc(store, "", doc); err == nil {
		t.Errorf("expected error when sessionID empty")
	}
}

func TestRouteRenovatePrefDoc(t *testing.T) {
	store := newTestStore(t)
	doc := &inboxDoc{
		kind: "renovate-pref",
		fields: map[string]string{
			"category": "automerge", "description": "auto-merge patch updates",
		},
	}
	if err := routeRenovatePrefDoc(store, doc); err != nil {
		t.Fatalf("routeRenovatePrefDoc: %v", err)
	}
}
