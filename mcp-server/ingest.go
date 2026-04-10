package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ingestOptions is the parsed CLI flag state for an `ingest` invocation.
type ingestOptions struct {
	dbPath    string
	inboxDir  string
	sessionID string
}

// parseIngestArgs parses ingest-subcommand flags into ingestOptions. It
// mirrors the dispatchHealthcheck / parseScanArgs convention: use a
// ContinueOnError flag.FlagSet so a parse failure returns exit code 2
// without panicking.
//
// Flags:
//
//	--db          path to the SQLite learning database
//	--inbox-dir   inbox directory (defaults to <db-dir>/inbox)
//	--session-id  override session ID (defaults to env var or session-id file)
func parseIngestArgs(args []string, stderr io.Writer) (ingestOptions, int) {
	fs := flag.NewFlagSet("ingest", flag.ContinueOnError)
	fs.SetOutput(stderr)

	dbPath := fs.String("db", ".go-guardian/guardian.db", "path to the SQLite learning database")
	inboxDir := fs.String("inbox-dir", "", "inbox directory (defaults to <db-dir>/inbox)")
	sessionID := fs.String("session-id", "", "override session ID for finding docs (defaults to GO_GUARDIAN_SESSION_ID or <db-dir>/session-id)")

	if err := fs.Parse(args); err != nil {
		return ingestOptions{}, 2
	}

	opts := ingestOptions{
		dbPath:    *dbPath,
		inboxDir:  *inboxDir,
		sessionID: *sessionID,
	}
	if strings.TrimSpace(opts.inboxDir) == "" {
		opts.inboxDir = filepath.Join(filepath.Dir(opts.dbPath), "inbox")
	}
	return opts, 0
}

// dispatchIngest implements `go-guardian ingest`. Skeleton only — Task 5
// replaces the body with the full parse → route → move pipeline.
func dispatchIngest(args []string, stdout, stderr io.Writer) int {
	opts, exit := parseIngestArgs(args, stderr)
	if exit != 0 {
		return exit
	}
	_ = opts
	_ = stdout
	fmt.Fprintln(stderr, "go-guardian ingest: skeleton — not wired (Task 1)")
	return 1
}

// inboxDoc is a parsed agent-inbox markdown document. fields holds the
// YAML frontmatter as flattened string key/value pairs (block-scalar values
// are joined with newlines). kind is convenience shorthand for fields["kind"].
type inboxDoc struct {
	sourcePath string
	kind       string
	fields     map[string]string
	body       string
}

// parseInboxDoc reads an inbox document and parses its frontmatter + body.
// It supports a small subset of YAML sufficient for the inbox format:
//
//   - plain scalar:      key: value
//   - block scalar:      key: |
//                          multi
//                          line
//   - quoted scalar:     key: "value with : colon"
//
// It is intentionally not a general YAML parser — inbox docs are written
// either by the ingest integration test or by agents following the same
// narrow subset, and pulling in a YAML library would balloon the binary
// for zero benefit.
func parseInboxDoc(path string) (*inboxDoc, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	text := string(data)

	// Strip optional UTF-8 BOM.
	text = strings.TrimPrefix(text, "\ufeff")

	if !strings.HasPrefix(text, "---\n") && !strings.HasPrefix(text, "---\r\n") {
		return nil, fmt.Errorf("missing opening --- delimiter")
	}
	text = strings.TrimPrefix(text, "---\n")
	text = strings.TrimPrefix(text, "---\r\n")

	// Find closing delimiter at the start of a line.
	closeIdx := strings.Index(text, "\n---\n")
	closeLen := 5
	if closeIdx < 0 {
		// Try CRLF variant.
		closeIdx = strings.Index(text, "\n---\r\n")
		closeLen = 6
	}
	if closeIdx < 0 {
		// Also accept a trailing --- with no following newline (end of file).
		if strings.HasSuffix(text, "\n---") {
			closeIdx = len(text) - 4
			closeLen = 4
		}
	}
	if closeIdx < 0 {
		return nil, fmt.Errorf("missing closing --- delimiter")
	}
	frontmatter := text[:closeIdx+1] // include the trailing newline of the last key
	body := ""
	if closeIdx+closeLen < len(text) {
		body = text[closeIdx+closeLen:]
	}
	body = strings.TrimLeft(body, "\r\n")

	fields, err := parseInboxFrontmatter(frontmatter)
	if err != nil {
		return nil, fmt.Errorf("parse frontmatter: %w", err)
	}

	doc := &inboxDoc{
		sourcePath: path,
		kind:       strings.TrimSpace(fields["kind"]),
		fields:     fields,
		body:       body,
	}
	if doc.kind == "" {
		return nil, fmt.Errorf("missing 'kind' field")
	}
	return doc, nil
}

// parseInboxFrontmatter parses the narrow YAML subset described in
// parseInboxDoc's doc comment. It returns a flat key/value map; block
// scalars are joined with "\n" and their trailing newline is stripped.
func parseInboxFrontmatter(text string) (map[string]string, error) {
	fields := map[string]string{}
	lines := strings.Split(text, "\n")

	i := 0
	for i < len(lines) {
		raw := lines[i]
		line := strings.TrimRight(raw, "\r")
		if strings.TrimSpace(line) == "" {
			i++
			continue
		}
		// Reject indented lines at the top level — they must be part of a
		// block scalar and thus handled below.
		if line[0] == ' ' || line[0] == '\t' {
			return nil, fmt.Errorf("unexpected indentation on line %d: %q", i+1, line)
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			return nil, fmt.Errorf("missing ':' on line %d: %q", i+1, line)
		}
		key := strings.TrimSpace(line[:colon])
		if key == "" {
			return nil, fmt.Errorf("empty key on line %d: %q", i+1, line)
		}
		rest := strings.TrimSpace(line[colon+1:])

		if rest == "|" || rest == "|-" {
			// Block scalar: consume indented lines until dedent or EOF.
			var sb strings.Builder
			i++
			indent := ""
			for i < len(lines) {
				bl := strings.TrimRight(lines[i], "\r")
				if bl == "" {
					sb.WriteString("\n")
					i++
					continue
				}
				if bl[0] != ' ' && bl[0] != '\t' {
					break
				}
				if indent == "" {
					// Detect indent from first indented line.
					j := 0
					for j < len(bl) && (bl[j] == ' ' || bl[j] == '\t') {
						j++
					}
					indent = bl[:j]
				}
				trimmed := strings.TrimPrefix(bl, indent)
				sb.WriteString(trimmed)
				sb.WriteString("\n")
				i++
			}
			val := sb.String()
			val = strings.TrimRight(val, "\n")
			fields[key] = val
			continue
		}
		// Plain scalar — strip optional surrounding double quotes.
		if len(rest) >= 2 && rest[0] == '"' && rest[len(rest)-1] == '"' {
			rest = rest[1 : len(rest)-1]
		}
		fields[key] = rest
		i++
	}
	return fields, nil
}
