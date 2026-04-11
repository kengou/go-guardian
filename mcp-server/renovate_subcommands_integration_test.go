package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// seedRenovateProject creates a minimal .go-guardian layout under root, opens
// a store so renovate_rules are auto-seeded, and returns the DB path.
// Helper name is distinct from seedProject/seedInboxProject because all three
// live in package main.
func seedRenovateProject(t *testing.T, root string) string {
	t.Helper()
	gdir := filepath.Join(root, ".go-guardian")
	if err := os.MkdirAll(gdir, 0o700); err != nil {
		t.Fatalf("mkdir .go-guardian: %v", err)
	}
	dbPath := filepath.Join(gdir, "guardian.db")
	store, err := db.NewStore(dbPath)
	if err != nil {
		t.Fatalf("seed NewStore: %v", err)
	}
	_ = store.Close()
	return dbPath
}

// writeRenovateConfig writes a renovate config JSON file at path with the
// given body. Used by the validate/analyze/suggest subtests.
func writeRenovateConfig(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write renovate config %s: %v", path, err)
	}
}

// runRenovate wraps Dispatch with a renovate arg list and returns exit code
// plus captured stdout/stderr.
func runRenovate(t *testing.T, args ...string) (exit int, stdoutStr, stderrStr string) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	full := append([]string{"renovate"}, args...)
	exit = Dispatch(full, stdout, stderr)
	return exit, stdout.String(), stderr.String()
}

func TestRenovateSubcommands_IntegrationScenarios(t *testing.T) {
	t.Run("Scenario: A well-formed renovate configuration validates successfully", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedRenovateProject(t, root)

		cfgPath := filepath.Join(root, "renovate.json")
		writeRenovateConfig(t, cfgPath, `{
  "extends": ["config:recommended"],
  "packageRules": [
    {"matchUpdateTypes": ["patch"], "automerge": true}
  ]
}
`)

		exit, stdout, stderr := runRenovate(t, "validate", "--db", dbPath, cfgPath)
		if exit != 0 {
			t.Fatalf("validate exit=%d, want 0; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "Valid JSON syntax") {
			t.Errorf("expected 'Valid JSON syntax' in stdout; got: %s", stdout)
		}
		if !strings.Contains(stdout, "0 error(s)") {
			t.Errorf("expected '0 error(s)' in stdout; got: %s", stdout)
		}
	})

	t.Run("Scenario: A malformed renovate configuration fails validation", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedRenovateProject(t, root)

		cfgPath := filepath.Join(root, "bad.json")
		writeRenovateConfig(t, cfgPath, `{"extends": [broken`)

		exit, stdout, _ := runRenovate(t, "validate", "--db", dbPath, cfgPath)
		if exit == 0 {
			t.Fatalf("validate on malformed config exit=0, want non-zero; stdout=%s", stdout)
		}
		if !strings.Contains(stdout, "✗ ERR:") {
			t.Errorf("expected '✗ ERR:' marker in stdout; got: %s", stdout)
		}
	})

	t.Run("Scenario: Analyzing a configuration produces an improvement report", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedRenovateProject(t, root)

		cfgPath := filepath.Join(root, "renovate.json")
		writeRenovateConfig(t, cfgPath, `{"extends": ["config:recommended"]}`)

		exit, stdout, stderr := runRenovate(t, "analyze", "--db", dbPath, cfgPath)
		if exit != 0 {
			t.Fatalf("analyze exit=%d, want 0; stdout=%s stderr=%s", exit, stdout, stderr)
		}

		artifactPath := filepath.Join(root, ".go-guardian", "renovate-analysis.md")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			t.Fatalf("expected analyze artifact at %s: %v", artifactPath, err)
		}
		body := string(data)
		if !strings.Contains(body, "Renovate Config Analysis") {
			t.Errorf("artifact missing 'Renovate Config Analysis' header; got: %s", body)
		}
		if !strings.Contains(body, "Score:") {
			t.Errorf("artifact missing 'Score:' line; got: %s", body)
		}
		if !strings.Contains(stdout, "renovate-analysis.md") {
			t.Errorf("stdout should reference the artifact path; got: %s", stdout)
		}
	})

	t.Run("Scenario: Requesting a suggestion for a named problem produces a rule", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedRenovateProject(t, root)

		exit, stdout, stderr := runRenovate(t, "suggest", "--db", dbPath, "automerge patch updates")
		if exit != 0 {
			t.Fatalf("suggest exit=%d, want 0; stdout=%s stderr=%s", exit, stdout, stderr)
		}

		artifactPath := filepath.Join(root, ".go-guardian", "renovate-suggestions.md")
		data, err := os.ReadFile(artifactPath)
		if err != nil {
			t.Fatalf("expected suggest artifact at %s: %v", artifactPath, err)
		}
		if !strings.Contains(string(data), "Suggestions for:") {
			t.Errorf("artifact missing 'Suggestions for:' header; got: %s", string(data))
		}
		if !strings.Contains(stdout, "renovate-suggestions.md") {
			t.Errorf("stdout should reference the artifact path; got: %s", stdout)
		}
	})

	t.Run("Scenario: Querying renovate knowledge returns seeded rules", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedRenovateProject(t, root)

		exit, stdout, stderr := runRenovate(t, "query", "--db", dbPath)
		if exit != 0 {
			t.Fatalf("query exit=%d, want 0; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "Renovate Knowledge") {
			t.Errorf("expected 'Renovate Knowledge' header in stdout; got: %s", stdout)
		}
		if !strings.Contains(stdout, "Rules") {
			t.Errorf("expected 'Rules' section in stdout (auto-seeded); got: %s", stdout)
		}
	})

	t.Run("Scenario: Renovate statistics reflect the current learning state", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedRenovateProject(t, root)

		exit, stdout, stderr := runRenovate(t, "stats", "--db", dbPath)
		if exit != 0 {
			t.Fatalf("stats exit=%d, want 0; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "Renovate Guardian Dashboard") {
			t.Errorf("expected dashboard header in stdout; got: %s", stdout)
		}
		if !strings.Contains(stdout, "Rule Coverage") {
			t.Errorf("expected 'Rule Coverage' section; got: %s", stdout)
		}
		if !strings.Contains(stdout, "Total rules:") {
			t.Errorf("expected 'Total rules:' line; got: %s", stdout)
		}
	})

	t.Run("Scenario: A learned renovate preference flows through the inbox", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedRenovateProject(t, root)

		inboxDir := filepath.Join(root, ".go-guardian", "inbox")
		if err := os.MkdirAll(inboxDir, 0o700); err != nil {
			t.Fatalf("mkdir inbox: %v", err)
		}
		docPath := filepath.Join(inboxDir, "renovate-pref-20260410T010203Z-zzz.md")
		if err := os.WriteFile(docPath, []byte(`---
kind: renovate-pref
category: automerge
description: auto-merge patch updates when CI is green xyz-unique-marker
---
`), 0o600); err != nil {
			t.Fatalf("write renovate-pref doc: %v", err)
		}

		// Step 1: ingest the inbox doc.
		ingestStdout := &bytes.Buffer{}
		ingestStderr := &bytes.Buffer{}
		if exit := Dispatch(
			[]string{"ingest", "--db", dbPath, "--inbox-dir", inboxDir},
			ingestStdout, ingestStderr,
		); exit != 0 {
			t.Fatalf("ingest exit=%d; stdout=%s stderr=%s",
				exit, ingestStdout.String(), ingestStderr.String())
		}

		// Step 2: confirm the preference is retrievable via query.
		exit, stdout, stderr := runRenovate(t, "query", "--db", dbPath, "--keyword", "xyz-unique-marker")
		if exit != 0 {
			t.Fatalf("query exit=%d; stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout, "xyz-unique-marker") {
			t.Errorf("expected ingested preference marker in query output; got: %s", stdout)
		}
	})
}
