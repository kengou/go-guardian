package main

import (
	"bytes"
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kengou/go-guardian/mcp-server/db"
	_ "modernc.org/sqlite"
)

// TestCLIFoundation_IntegrationScenarios exercises the 7 Gherkin scenarios
// from .beastmode/artifacts/plan/2026-04-06-hooks-only-architecture-cli-foundation.md
// by calling the Dispatch function directly. Running the built binary would
// also work but is slower; behavior is fully captured by Dispatch().
func TestCLIFoundation_IntegrationScenarios(t *testing.T) {
	t.Run("Scenario: Binary reports its version when asked", func(t *testing.T) {
		for _, args := range [][]string{
			{"version"},
			{"--version"},
		} {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			exitCode := Dispatch(args, stdout, stderr)
			if exitCode != 0 {
				t.Fatalf("Dispatch(%v) exit=%d, want 0; stderr=%s", args, exitCode, stderr.String())
			}
			if !strings.Contains(stdout.String(), version) {
				t.Errorf("Dispatch(%v) stdout=%q, want it to contain version %q", args, stdout.String(), version)
			}
		}
	})

	t.Run("Scenario: Binary exposes a discoverable list of subcommands", func(t *testing.T) {
		for _, args := range [][]string{
			{"help"},
			{"--help"},
			{"-h"},
		} {
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}
			exitCode := Dispatch(args, stdout, stderr)
			if exitCode != 0 {
				t.Fatalf("Dispatch(%v) exit=%d, want 0; stderr=%s", args, exitCode, stderr.String())
			}
			out := stdout.String()
			for _, sub := range []string{"scan", "ingest", "renovate", "admin", "healthcheck"} {
				if !strings.Contains(out, sub) {
					t.Errorf("Dispatch(%v) help output missing %q; got:\n%s", args, sub, out)
				}
			}
		}
	})

	t.Run("Scenario: Existing learning database is preserved across the migration", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "guardian.db")

		// Simulate a pre-migration workspace by opening the store and inserting
		// a learned lint pattern.
		store1, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("pre-migration NewStore: %v", err)
		}
		if err := store1.InsertLintPattern("errcheck", "*.go",
			"rows, _ := db.Query(...)",
			"rows, err := db.Query(...); if err != nil { ... }",
			"learned"); err != nil {
			t.Fatalf("seed InsertLintPattern: %v", err)
		}
		_ = store1.Close()

		// Run the new-binary healthcheck against the same DB path.
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := Dispatch([]string{"healthcheck", "--db", dbPath}, stdout, stderr)
		if exitCode != 0 {
			t.Fatalf("healthcheck on healthy DB exit=%d, want 0; stdout=%s stderr=%s",
				exitCode, stdout.String(), stderr.String())
		}

		// Re-open the store — the learned pattern must still be queryable.
		store2, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("post-migration NewStore: %v", err)
		}
		defer store2.Close()
		patterns, err := store2.QueryPatterns("*.go", "rows.Close", 10)
		if err != nil {
			t.Fatalf("QueryPatterns: %v", err)
		}
		if len(patterns) == 0 {
			t.Fatal("learned pattern lost after running new binary against pre-migration DB")
		}
		foundErrcheck := false
		for _, p := range patterns {
			if p.Rule == "errcheck" {
				foundErrcheck = true
				break
			}
		}
		if !foundErrcheck {
			t.Errorf("errcheck pattern missing after migration; got rules: %+v", patterns)
		}
	})

	t.Run("Scenario: Existing guardian directory layout remains compatible", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "guardian.db")

		// Create a pre-existing workspace with the store + some history.
		store1, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("seed NewStore: %v", err)
		}
		if err := store1.UpdateScanHistory("owasp", "testproj", 3); err != nil {
			t.Fatalf("seed UpdateScanHistory: %v", err)
		}
		_ = store1.Close()

		// Running a command against the same DB must reuse (not reinit) it.
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := Dispatch([]string{"healthcheck", "--db", dbPath}, stdout, stderr)
		if exitCode != 0 {
			t.Fatalf("healthcheck exit=%d, want 0; stderr=%s", exitCode, stderr.String())
		}

		// History must still be present.
		store2, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("post NewStore: %v", err)
		}
		defer store2.Close()
		history, err := store2.GetScanHistory("testproj")
		if err != nil {
			t.Fatalf("GetScanHistory: %v", err)
		}
		if len(history) == 0 {
			t.Fatal("scan history lost after running new binary against existing workspace")
		}
	})

	t.Run("Scenario: Unknown subcommand produces an actionable error", func(t *testing.T) {
		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := Dispatch([]string{"bogus-subcommand"}, stdout, stderr)
		if exitCode == 0 {
			t.Fatalf("Dispatch(bogus) exit=0, want non-zero; stdout=%s", stdout.String())
		}
		errOut := stderr.String() + stdout.String()
		if !strings.Contains(errOut, "bogus-subcommand") {
			t.Errorf("error message should mention the unknown subcommand; got: %s", errOut)
		}
		// Error must list at least a few valid subcommands.
		validMentions := 0
		for _, sub := range []string{"scan", "ingest", "renovate", "admin", "healthcheck"} {
			if strings.Contains(errOut, sub) {
				validMentions++
			}
		}
		if validMentions < 3 {
			t.Errorf("error message should list valid subcommands; got: %s", errOut)
		}
	})

	t.Run("Scenario: Diagnostic healthcheck succeeds on a healthy installation", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "guardian.db")

		// db.NewStore initializes the schema + seeds. A fresh store from NewStore
		// is considered a "healthy installation" by the healthcheck.
		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("seed NewStore: %v", err)
		}
		_ = store.Close()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := Dispatch([]string{"healthcheck", "--db", dbPath}, stdout, stderr)
		if exitCode != 0 {
			t.Fatalf("healthcheck on healthy DB exit=%d, want 0; stdout=%s stderr=%s",
				exitCode, stdout.String(), stderr.String())
		}
	})

	t.Run("Scenario: Diagnostic healthcheck reports a broken installation", func(t *testing.T) {
		tmpDir := t.TempDir()
		dbPath := filepath.Join(tmpDir, "guardian.db")

		// Create a healthy DB first.
		store, err := db.NewStore(dbPath)
		if err != nil {
			t.Fatalf("seed NewStore: %v", err)
		}
		_ = store.Close()

		// Drop a required table via a raw sqlite connection to simulate corruption.
		// We use the same modernc.org/sqlite driver as the db package so the
		// on-disk format is identical.
		raw, err := sql.Open("sqlite", dbPath)
		if err != nil {
			t.Fatalf("raw sqlite open: %v", err)
		}
		if _, err := raw.ExecContext(context.Background(), "DROP TABLE lint_patterns"); err != nil {
			t.Fatalf("drop lint_patterns: %v", err)
		}
		_ = raw.Close()

		stdout := &bytes.Buffer{}
		stderr := &bytes.Buffer{}
		exitCode := Dispatch([]string{"healthcheck", "--db", dbPath}, stdout, stderr)
		if exitCode == 0 {
			t.Fatalf("healthcheck on broken DB exit=0, want non-zero; stdout=%s", stdout.String())
		}
		out := stdout.String() + stderr.String()
		if !strings.Contains(strings.ToLower(out), "lint_patterns") && !strings.Contains(strings.ToLower(out), "missing") {
			t.Errorf("expected healthcheck to identify the missing table; got: %s", out)
		}
	})
}
