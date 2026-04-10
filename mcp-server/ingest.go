package main

import (
	"flag"
	"fmt"
	"io"
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
