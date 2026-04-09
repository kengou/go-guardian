package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kengou/go-guardian/mcp-server/db"
	_ "modernc.org/sqlite"
)

// seedProject creates a minimal Go project layout under root and returns
// the guardian DB path. Used by every scan-subcommands integration subtest.
func seedProject(t *testing.T, root string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o600); err != nil {
		t.Fatalf("write main.go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"),
		[]byte("module example.com/test\n\ngo 1.26\n"), 0o600); err != nil {
		t.Fatalf("write go.mod: %v", err)
	}
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

// runScan is a thin wrapper that calls Dispatch with a scan arg list and
// returns exit code + captured stdout/stderr.
func runScan(t *testing.T, args ...string) (int, string, string) {
	t.Helper()
	stdout := &bytes.Buffer{}
	stderr := &bytes.Buffer{}
	full := append([]string{"scan"}, args...)
	exit := Dispatch(full, stdout, stderr)
	return exit, stdout.String(), stderr.String()
}

func TestScanSubcommands_IntegrationScenarios(t *testing.T) {
	t.Run("Scenario: First scan on an unscanned project produces findings files", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedProject(t, root)

		exit, stdout, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root)
		if exit != 0 {
			t.Fatalf("first scan exit=%d, want 0; stdout=%s stderr=%s", exit, stdout, stderr)
		}

		expected := []string{
			"owasp-findings.md",
			"dep-vulns.md",
			"staleness.md",
			"pattern-stats.md",
			"health-trends.md",
			"session-findings.md",
		}
		for _, name := range expected {
			p := filepath.Join(root, ".go-guardian", name)
			if _, err := os.Stat(p); err != nil {
				t.Errorf("expected %s to exist after --all: %v", name, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, ".go-guardian", ".scan-checksum")); err != nil {
			t.Errorf(".scan-checksum not written after first scan: %v", err)
		}
	})

	t.Run("Scenario: Second scan on unchanged source is a no-op", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedProject(t, root)

		if exit, _, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root); exit != 0 {
			t.Fatalf("first scan exit=%d: %s", exit, stderr)
		}

		// Capture mtimes of each output file.
		outputs := []string{
			"owasp-findings.md", "dep-vulns.md", "staleness.md",
			"pattern-stats.md", "health-trends.md", "session-findings.md",
		}
		mtimes := map[string]time.Time{}
		for _, name := range outputs {
			fi, err := os.Stat(filepath.Join(root, ".go-guardian", name))
			if err != nil {
				t.Fatalf("stat %s: %v", name, err)
			}
			mtimes[name] = fi.ModTime()
		}

		// Ensure the clock can distinguish mtimes if the scan did rewrite.
		time.Sleep(10 * time.Millisecond)

		exit, stdout, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root)
		if exit != 0 {
			t.Fatalf("second scan exit=%d: %s", exit, stderr)
		}
		if !strings.Contains(stdout+stderr, "cached") {
			t.Errorf("second scan did not report 'cached'; stdout=%s stderr=%s", stdout, stderr)
		}
		for _, name := range outputs {
			fi, err := os.Stat(filepath.Join(root, ".go-guardian", name))
			if err != nil {
				t.Fatalf("stat after second scan %s: %v", name, err)
			}
			if !fi.ModTime().Equal(mtimes[name]) {
				t.Errorf("%s mtime changed across cached scan: was %v now %v",
					name, mtimes[name], fi.ModTime())
			}
		}
	})

	t.Run("Scenario: Scan is re-run after source code changes", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedProject(t, root)

		if exit, _, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root); exit != 0 {
			t.Fatalf("first scan exit=%d: %s", exit, stderr)
		}

		checksumPath := filepath.Join(root, ".go-guardian", ".scan-checksum")
		firstChecksum, err := os.ReadFile(checksumPath)
		if err != nil {
			t.Fatalf("read .scan-checksum: %v", err)
		}

		// Modify a tracked Go file.
		if err := os.WriteFile(filepath.Join(root, "main.go"),
			[]byte("package main\n\nfunc main() { println(\"changed\") }\n"), 0o600); err != nil {
			t.Fatalf("modify main.go: %v", err)
		}

		time.Sleep(10 * time.Millisecond)

		if exit, stdout, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root); exit != 0 {
			t.Fatalf("rescan exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
		}

		secondChecksum, err := os.ReadFile(checksumPath)
		if err != nil {
			t.Fatalf("read .scan-checksum after rescan: %v", err)
		}
		if string(firstChecksum) == string(secondChecksum) {
			t.Errorf("checksum did not change after editing main.go; both = %s", firstChecksum)
		}
	})

	t.Run("Scenario: Scan is re-run when findings artifacts are missing", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedProject(t, root)

		if exit, _, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root); exit != 0 {
			t.Fatalf("first scan exit=%d: %s", exit, stderr)
		}
		missingPath := filepath.Join(root, ".go-guardian", "owasp-findings.md")
		if err := os.Remove(missingPath); err != nil {
			t.Fatalf("remove owasp-findings.md: %v", err)
		}

		exit, stdout, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root)
		if exit != 0 {
			t.Fatalf("rescan-after-delete exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if _, err := os.Stat(missingPath); err != nil {
			t.Errorf("owasp-findings.md not regenerated after deletion: %v", err)
		}
	})

	t.Run("Scenario Outline: Individual scan dimensions can be requested on their own", func(t *testing.T) {
		// Each case asserts exit 0 AND that the expected output files are the
		// *only* files created under .go-guardian/ besides the DB and checksum.
		cases := []struct {
			name        string
			flag        string
			wantFiles   []string
		}{
			{"OWASP vulnerabilities", "--owasp", []string{"owasp-findings.md"}},
			{"dependency CVEs", "--deps", []string{"dep-vulns.md"}},
			{"staleness", "--staleness", []string{"staleness.md"}},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				root := t.TempDir()
				dbPath := seedProject(t, root)

				exit, stdout, stderr := runScan(t, tc.flag, "--db", dbPath, "--source-dir", root)
				if exit != 0 {
					t.Fatalf("%s scan exit=%d stdout=%s stderr=%s", tc.flag, exit, stdout, stderr)
				}
				for _, w := range tc.wantFiles {
					if _, err := os.Stat(filepath.Join(root, ".go-guardian", w)); err != nil {
						t.Errorf("%s scan did not produce %s: %v", tc.flag, w, err)
					}
				}
				// No cross-dimension leakage: assert the OTHER dimension files are absent.
				allDims := []string{
					"owasp-findings.md", "dep-vulns.md", "staleness.md",
					"pattern-stats.md", "health-trends.md", "session-findings.md",
				}
				wanted := map[string]bool{}
				for _, w := range tc.wantFiles {
					wanted[w] = true
				}
				for _, f := range allDims {
					if wanted[f] {
						continue
					}
					if _, err := os.Stat(filepath.Join(root, ".go-guardian", f)); err == nil {
						t.Errorf("%s scan produced unrelated file %s", tc.flag, f)
					}
				}
			})
		}
	})

	t.Run("Scenario: Vendored code is excluded from the source checksum", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedProject(t, root)

		// Seed a vendored file.
		vendorDir := filepath.Join(root, "vendor", "third")
		if err := os.MkdirAll(vendorDir, 0o700); err != nil {
			t.Fatalf("mkdir vendor: %v", err)
		}
		vendorFile := filepath.Join(vendorDir, "x.go")
		if err := os.WriteFile(vendorFile,
			[]byte("package third\n"), 0o600); err != nil {
			t.Fatalf("write vendor file: %v", err)
		}

		if exit, _, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root); exit != 0 {
			t.Fatalf("first scan exit=%d: %s", exit, stderr)
		}

		// Modify only the vendored file.
		if err := os.WriteFile(vendorFile,
			[]byte("package third\n\nvar X = 1\n"), 0o600); err != nil {
			t.Fatalf("modify vendor file: %v", err)
		}

		exit, stdout, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root)
		if exit != 0 {
			t.Fatalf("second scan exit=%d stdout=%s stderr=%s", exit, stdout, stderr)
		}
		if !strings.Contains(stdout+stderr, "cached") {
			t.Errorf("vendored-only change should be cached; stdout=%s stderr=%s", stdout, stderr)
		}
	})

	t.Run("Scenario: Scan writes a human-readable summary for agents to consume", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedProject(t, root)

		if exit, _, stderr := runScan(t, "--all", "--db", dbPath, "--source-dir", root); exit != 0 {
			t.Fatalf("scan exit=%d: %s", exit, stderr)
		}

		for _, name := range []string{
			"owasp-findings.md", "dep-vulns.md", "staleness.md",
			"pattern-stats.md", "health-trends.md", "session-findings.md",
		} {
			data, err := os.ReadFile(filepath.Join(root, ".go-guardian", name))
			if err != nil {
				t.Fatalf("read %s: %v", name, err)
			}
			content := string(data)
			if !strings.HasPrefix(content, "---\n") {
				t.Errorf("%s missing YAML frontmatter; starts with: %q", name, safeHead(content))
				continue
			}
			for _, key := range []string{"scan_type:", "source_checksum:", "generated_at:", "count:"} {
				if !strings.Contains(content, key) {
					t.Errorf("%s missing frontmatter key %q", name, key)
				}
			}
		}
	})
}

func safeHead(s string) string {
	if len(s) > 40 {
		return s[:40]
	}
	return s
}
