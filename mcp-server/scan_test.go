package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteScanOutput_FrontmatterAndBody(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.md")
	ts, _ := time.Parse(time.RFC3339, "2026-04-09T12:34:56Z")

	if err := writeScanOutput(path, "owasp", "deadbeef", ts, 3, "body line 1\nbody line 2"); err != nil {
		t.Fatalf("writeScanOutput: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	s := string(data)
	if !strings.HasPrefix(s, "---\n") {
		t.Errorf("missing leading ---; got: %q", s)
	}
	for _, want := range []string{
		"scan_type: owasp",
		"source_checksum: deadbeef",
		"generated_at: 2026-04-09T12:34:56Z",
		"count: 3",
		"body line 1",
		"body line 2",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("output missing %q; got:\n%s", want, s)
		}
	}
	if !strings.HasSuffix(s, "\n") {
		t.Errorf("output does not end with newline; got: %q", s[len(s)-10:])
	}
}

func TestWriteScanOutput_ZeroCountValid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.md")
	if err := writeScanOutput(path, "deps", "cafebabe", time.Now(), 0, "no findings"); err != nil {
		t.Fatalf("writeScanOutput zero: %v", err)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "count: 0") {
		t.Errorf("zero count not rendered; got:\n%s", data)
	}
}

func TestParseScanArgs_RejectsEmpty(t *testing.T) {
	var stderr strings.Builder
	_, exit := parseScanArgs([]string{"--db", "x.db"}, &stderr)
	if exit == 0 {
		t.Fatalf("expected non-zero exit for no dimensions; stderr=%s", stderr.String())
	}
}

func TestParseScanArgs_AllExpandsEveryDimension(t *testing.T) {
	var stderr strings.Builder
	opts, exit := parseScanArgs([]string{"--all", "--db", "x.db", "--source-dir", "/tmp"}, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if !opts.dims.owasp || !opts.dims.deps || !opts.dims.staleness || !opts.dims.patterns {
		t.Errorf("--all did not enable every dimension: %+v", opts.dims)
	}
}

func TestParseScanArgs_ComposableFlags(t *testing.T) {
	var stderr strings.Builder
	opts, exit := parseScanArgs([]string{"--owasp", "--deps", "--db", "x.db", "--source-dir", "/tmp"}, &stderr)
	if exit != 0 {
		t.Fatalf("exit=%d stderr=%s", exit, stderr.String())
	}
	if !opts.dims.owasp || !opts.dims.deps {
		t.Errorf("composable flags lost: %+v", opts.dims)
	}
	if opts.dims.staleness || opts.dims.patterns {
		t.Errorf("composable flags leaked to other dimensions: %+v", opts.dims)
	}
}
