// Package owasp implements the OWASP Top 10 rule engine for go-guardian.
// It provides static analysis rules for Go source code, combining regex-based
// line scanning with go/ast-based structural checks.
package owasp

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// Severity represents the criticality of a security finding.
type Severity string

const (
	SeverityCritical Severity = "CRITICAL"
	SeverityHigh     Severity = "HIGH"
	SeverityMedium   Severity = "MEDIUM"
	SeverityLow      Severity = "LOW"
)

// Finding is a single OWASP rule violation discovered in source code.
type Finding struct {
	File     string
	Line     int
	Category string   // e.g. "A02", "A03"
	Severity Severity
	Message  string
	Fix      string
	Evidence string // the actual code snippet that triggered the rule
}

// Rule is an OWASP check that can be applied to a parsed Go source file.
type Rule struct {
	Category    string
	Severity    Severity
	Description string
	Fix         string
	// Check is called with the file path, raw source bytes, AST, and FileSet.
	// It returns zero or more findings. AST/fset may be nil for syntax-error
	// files; implementations must handle nil gracefully.
	Check func(path string, src []byte, file *ast.File, fset *token.FileSet) []Finding
}

// lineNumber returns the 1-based line number for the given byte offset in src.
func lineNumber(src []byte, offset int) int {
	return strings.Count(string(src[:offset]), "\n") + 1
}

// lineAt returns the trimmed text of the 1-based line from src.
func lineAt(src []byte, line int) string {
	lines := strings.SplitN(string(src), "\n", line+1)
	if line-1 < len(lines) {
		return strings.TrimSpace(lines[line-1])
	}
	return ""
}

// regexRule builds a Rule whose Check scans raw source bytes with re.
func regexRule(category string, severity Severity, message, fix string, re *regexp.Regexp) Rule {
	return Rule{
		Category:    category,
		Severity:    severity,
		Description: message,
		Fix:         fix,
		Check: func(path string, src []byte, _ *ast.File, _ *token.FileSet) []Finding {
			var findings []Finding
			for _, loc := range re.FindAllIndex(src, -1) {
				line := lineNumber(src, loc[0])
				findings = append(findings, Finding{
					File:     path,
					Line:     line,
					Category: category,
					Severity: severity,
					Message:  message,
					Fix:      fix,
					Evidence: lineAt(src, line),
				})
			}
			return findings
		},
	}
}

// DefaultRules returns all built-in OWASP rules covering A02, A03, A05, A09,
// and A10.
func DefaultRules() []Rule {
	return []Rule{
		// ── A02 Cryptographic Failures ──────────────────────────────────────

		regexRule(
			"A02", SeverityCritical,
			"Weak hash: MD5 is not collision-resistant. Use crypto/bcrypt or argon2 for passwords.",
			"Replace md5 with crypto/bcrypt (passwords) or sha256/sha512 (integrity checks).",
			regexp.MustCompile(`md5\.New\(\)|md5\.Sum\(`),
		),

		regexRule(
			"A02", SeverityHigh,
			"Weak hash: SHA-1 is deprecated for security use. Use SHA-256 or stronger.",
			"Replace sha1 with crypto/sha256 or crypto/sha512.",
			regexp.MustCompile(`sha1\.New\(\)|sha1\.Sum\(`),
		),

		// A02: hardcoded secrets — AST-based
		{
			Category:    "A02",
			Severity:    SeverityHigh,
			Description: "Potential hardcoded secret",
			Fix:         "Load secrets from environment variables or a secrets manager; never hardcode credentials.",
			Check:       checkHardcodedSecrets,
		},

		// A02: InsecureSkipVerify — AST-based
		{
			Category:    "A02",
			Severity:    SeverityCritical,
			Description: "TLS verification disabled. Never use InsecureSkipVerify in production.",
			Fix:         "Remove InsecureSkipVerify or set it to false. Use proper certificate validation.",
			Check:       checkInsecureSkipVerify,
		},

		// ── A03 Injection ───────────────────────────────────────────────────

		regexRule(
			"A03", SeverityCritical,
			"Possible SQL injection via string formatting. Use parameterized queries.",
			"Replace fmt.Sprintf for SQL with database/sql parameterized queries (db.Query/db.Exec with ? placeholders).",
			regexp.MustCompile(`(?i)fmt\.Sprintf\s*\(\s*"[^"]*(?:SELECT|INSERT|UPDATE|DELETE)`),
		),

		regexRule(
			"A03", SeverityHigh,
			"Possible command injection. Validate and sanitize command arguments.",
			"Avoid building exec.Command arguments via string concatenation. Use a fixed command with a validated argument list.",
			regexp.MustCompile(`exec\.Command\([^)]*\+`),
		),

		regexRule(
			"A03", SeverityMedium,
			"Unsafe HTML/JS/URL cast bypasses template auto-escaping.",
			"Avoid template.HTML/JS/URL conversions unless the value is from a fully trusted source. Prefer contextual auto-escaping.",
			regexp.MustCompile(`template\.(HTML|JS|URL)\(`),
		),

		// ── A05 Security Misconfiguration ───────────────────────────────────

		// A05: pprof import — AST-based
		{
			Category:    "A05",
			Severity:    SeverityMedium,
			Description: `pprof profiling endpoint imported. Guard with a build tag or remove in production.`,
			Fix:         `Move the _ "net/http/pprof" import to a file with //go:build debug or equivalent.`,
			Check:       checkPprofImport,
		},

		regexRule(
			"A05", SeverityMedium,
			"Wildcard CORS origin. Restrict to known origins in production.",
			"Replace the wildcard with an explicit allowlist of permitted origins.",
			regexp.MustCompile(`(?i)(AllowedOrigins\s*[=:]\s*\[?"?\*"?|Access-Control-Allow-Origin[^"]*"\*")`),
		),

		// ── A09 Logging Failures ─────────────────────────────────────────────

		regexRule(
			"A09", SeverityHigh,
			"Potential sensitive data logged. Remove or mask sensitive fields.",
			"Never log raw passwords, tokens, or secrets. Use a structured logger with field redaction.",
			regexp.MustCompile(`(?i)(log\.Printf|log\.Println|fmt\.Fprintf\s*\(\s*os\.Stderr)[^\n]*(password|secret|token)`),
		),

		// ── A10 SSRF ────────────────────────────────────────────────────────

		regexRule(
			"A10", SeverityMedium,
			"Unvalidated URL in HTTP request. Validate against an allowlist of permitted hosts.",
			"Parse the URL with url.Parse, verify the host against an explicit allowlist, then make the request.",
			// Match http.Get/Post/NewRequest where the first argument starts
			// with a non-quote character (i.e. a variable, not a string literal).
			regexp.MustCompile(`http\.(Get|Post|NewRequest)\s*\(\s*[^"'\s]`),
		),
	}
}

// ── AST-based check helpers ──────────────────────────────────────────────────

// sensitiveNameRe matches variable/constant names that suggest credential storage.
var sensitiveNameRe = regexp.MustCompile(`(?i)(password|secret|token|apikey|api_key)`)

// checkHardcodedSecrets looks for non-empty string literals assigned to
// variables or constants whose names contain sensitive keywords.
func checkHardcodedSecrets(path string, src []byte, file *ast.File, fset *token.FileSet) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding

	ast.Inspect(file, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.ValueSpec:
			// var / const declarations: var password = "hunter2"
			for i, name := range v.Names {
				if !sensitiveNameRe.MatchString(name.Name) {
					continue
				}
				if i >= len(v.Values) {
					continue
				}
				lit, ok := v.Values[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val := strings.Trim(lit.Value, `"`)
				if val == "" || val == "****" {
					continue
				}
				pos := fset.Position(v.Pos())
				findings = append(findings, Finding{
					File:     path,
					Line:     pos.Line,
					Category: "A02",
					Severity: SeverityHigh,
					Message:  "Potential hardcoded secret",
					Fix:      "Load secrets from environment variables or a secrets manager; never hardcode credentials.",
					Evidence: lineAt(src, pos.Line),
				})
			}

		case *ast.AssignStmt:
			// Simple assignments: password = "hunter2"
			for i, lhs := range v.Lhs {
				ident, ok := lhs.(*ast.Ident)
				if !ok || !sensitiveNameRe.MatchString(ident.Name) {
					continue
				}
				if i >= len(v.Rhs) {
					continue
				}
				lit, ok := v.Rhs[i].(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val := strings.Trim(lit.Value, `"`)
				if val == "" || val == "****" {
					continue
				}
				pos := fset.Position(v.Pos())
				findings = append(findings, Finding{
					File:     path,
					Line:     pos.Line,
					Category: "A02",
					Severity: SeverityHigh,
					Message:  "Potential hardcoded secret",
					Fix:      "Load secrets from environment variables or a secrets manager; never hardcode credentials.",
					Evidence: lineAt(src, pos.Line),
				})
			}
		}
		return true
	})
	return findings
}

// checkInsecureSkipVerify finds `InsecureSkipVerify: true` in struct literals.
func checkInsecureSkipVerify(path string, src []byte, file *ast.File, fset *token.FileSet) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok {
			return true
		}
		for _, elt := range lit.Elts {
			kv, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := kv.Key.(*ast.Ident)
			if !ok || key.Name != "InsecureSkipVerify" {
				continue
			}
			ident, ok := kv.Value.(*ast.Ident)
			if !ok || ident.Name != "true" {
				continue
			}
			pos := fset.Position(kv.Pos())
			findings = append(findings, Finding{
				File:     path,
				Line:     pos.Line,
				Category: "A02",
				Severity: SeverityCritical,
				Message:  "TLS verification disabled. Never use InsecureSkipVerify in production.",
				Fix:      "Remove InsecureSkipVerify or set it to false. Use proper certificate validation.",
				Evidence: lineAt(src, pos.Line),
			})
		}
		return true
	})
	return findings
}

// checkPprofImport finds blank imports of "net/http/pprof" without a build tag.
func checkPprofImport(path string, src []byte, file *ast.File, fset *token.FileSet) []Finding {
	if file == nil {
		return nil
	}
	var findings []Finding
	for _, imp := range file.Imports {
		if imp.Path == nil {
			continue
		}
		importPath := strings.Trim(imp.Path.Value, `"`)
		if importPath != "net/http/pprof" {
			continue
		}
		// Only flag blank imports (_).
		if imp.Name == nil || imp.Name.Name != "_" {
			continue
		}
		pos := fset.Position(imp.Pos())
		findings = append(findings, Finding{
			File:     path,
			Line:     pos.Line,
			Category: "A05",
			Severity: SeverityMedium,
			Message:  "pprof profiling endpoint imported. Guard with a build tag or remove in production.",
			Fix:      `Move the _ "net/http/pprof" import to a file with //go:build debug or equivalent.`,
			Evidence: lineAt(src, pos.Line),
		})
	}
	return findings
}

// ── Scanner ──────────────────────────────────────────────────────────────────

// ScanFile reads a single Go source file, parses it, runs all rules, and
// returns deduplicated findings.
func ScanFile(path string, rules []Rule) ([]Finding, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	fset := token.NewFileSet()
	// ParseComments so that build-tag comments are available to future rules.
	// Errors are intentionally ignored: astFile will be nil on total parse
	// failure, and rules must handle nil gracefully.
	astFile, _ := parser.ParseFile(fset, path, src, parser.ParseComments)

	seen := make(map[string]bool)
	var findings []Finding
	for _, rule := range rules {
		for _, f := range rule.Check(path, src, astFile, fset) {
			key := strings.Join([]string{
				f.File,
				strconv.Itoa(f.Line),
				f.Category,
				f.Message,
			}, "\x00")
			if !seen[key] {
				seen[key] = true
				findings = append(findings, f)
			}
		}
	}
	return findings, nil
}

// ScanDirectory walks dir recursively, skipping vendor/, .git/, and
// node_modules/, and calls ScanFile on every .go file.
func ScanDirectory(dir string, rules []Rule) ([]Finding, error) {
	var all []Finding
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "vendor", ".git", "node_modules":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		findings, err := ScanFile(path, rules)
		if err != nil {
			// Non-fatal: skip unreadable or unparseable files.
			return nil
		}
		all = append(all, findings...)
		return nil
	})
	return all, err
}
