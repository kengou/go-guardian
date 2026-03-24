package owasp

import (
	"os"
	"testing"
)

// writeTemp writes src to a temporary .go file and returns the path.
// The file is removed automatically when the test finishes.
func writeTemp(t *testing.T, src string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "owasp_test_*.go")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	if _, err := f.WriteString(src); err != nil {
		t.Fatalf("WriteString: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	return f.Name()
}

// hasFinding returns true if any finding in fs matches category and severity.
func hasFinding(findings []Finding, category string, severity Severity) bool {
	for _, f := range findings {
		if f.Category == category && f.Severity == severity {
			return true
		}
	}
	return false
}

// TestA02WeakHashDetection verifies that md5.New() is flagged CRITICAL.
func TestA02WeakHashDetection(t *testing.T) {
	src := `package main

import (
	"crypto/md5"
	"fmt"
)

func main() {
	h := md5.New()
	fmt.Println(h)
}
`
	path := writeTemp(t, src)
	findings, err := ScanFile(path, DefaultRules())
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !hasFinding(findings, "A02", SeverityCritical) {
		t.Errorf("expected CRITICAL A02 finding for md5.New(), got: %+v", findings)
	}
}

// TestA02InsecureSkipVerify verifies that InsecureSkipVerify: true is flagged CRITICAL.
func TestA02InsecureSkipVerify(t *testing.T) {
	src := `package main

import (
	"crypto/tls"
	"net/http"
)

func main() {
	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
		},
	}
	_ = tr
}
`
	path := writeTemp(t, src)
	findings, err := ScanFile(path, DefaultRules())
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !hasFinding(findings, "A02", SeverityCritical) {
		t.Errorf("expected CRITICAL A02 finding for InsecureSkipVerify, got: %+v", findings)
	}
}

// TestA03SQLInjection verifies that fmt.Sprintf with SQL keywords is flagged CRITICAL.
func TestA03SQLInjection(t *testing.T) {
	src := `package main

import (
	"fmt"
)

func query(id string) string {
	return fmt.Sprintf("SELECT * FROM users WHERE id = %s", id)
}
`
	path := writeTemp(t, src)
	findings, err := ScanFile(path, DefaultRules())
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !hasFinding(findings, "A03", SeverityCritical) {
		t.Errorf("expected CRITICAL A03 finding for SQL injection, got: %+v", findings)
	}
}

// TestA05PprofImport verifies that a blank pprof import is flagged MEDIUM.
func TestA05PprofImport(t *testing.T) {
	src := `package main

import (
	_ "net/http/pprof"
	"net/http"
)

func main() {
	http.ListenAndServe(":8080", nil)
}
`
	path := writeTemp(t, src)
	findings, err := ScanFile(path, DefaultRules())
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !hasFinding(findings, "A05", SeverityMedium) {
		t.Errorf("expected MEDIUM A05 finding for pprof import, got: %+v", findings)
	}
}

// TestA10SSRF verifies that http.Get with a variable URL is flagged MEDIUM.
func TestA10SSRF(t *testing.T) {
	src := `package main

import (
	"net/http"
)

func fetch(userURL string) {
	resp, err := http.Get(userURL)
	_ = resp
	_ = err
}
`
	path := writeTemp(t, src)
	findings, err := ScanFile(path, DefaultRules())
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if !hasFinding(findings, "A10", SeverityMedium) {
		t.Errorf("expected MEDIUM A10 finding for SSRF, got: %+v", findings)
	}
}

// TestNoFalsePositives verifies that a clean file produces no findings.
func TestNoFalsePositives(t *testing.T) {
	src := `package main

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"fmt"
	"net/http"

	"golang.org/x/crypto/bcrypt"
)

func hashPassword(pw string) ([]byte, error) {
	return bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
}

func integrityHash(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

func queryUser(db *sql.DB, id int64) error {
	row := db.QueryRowContext(context.Background(),
		"SELECT id, name FROM users WHERE id = ?", id)
	var name string
	return row.Scan(&id, &name)
}

func safeRequest() {
	// Literal URL — no variable involved.
	resp, err := http.Get("https://api.example.com/status")
	if err != nil {
		fmt.Println(err)
		return
	}
	resp.Body.Close()
}
`
	path := writeTemp(t, src)
	findings, err := ScanFile(path, DefaultRules())
	if err != nil {
		t.Fatalf("ScanFile: %v", err)
	}
	if len(findings) != 0 {
		t.Errorf("expected no findings on clean file, got %d: %+v", len(findings), findings)
	}
}
