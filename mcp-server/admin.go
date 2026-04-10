package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"syscall"
	"time"

	"github.com/kengou/go-guardian/mcp-server/admin"
	"github.com/kengou/go-guardian/mcp-server/db"
)

// defaultAdminPort is the port dispatchAdmin binds when --port is not set.
// It matches the port documented in docs/QUICKSTART.md as the conventional
// GO_GUARDIAN_ADMIN_PORT example, preserving continuity with the previous
// always-on deployment.
const defaultAdminPort = 8080

// adminShutdownGrace is the maximum time the server has to finish in-flight
// requests during graceful shutdown before http.Server.Shutdown returns with
// an error. Keep it short — the admin UI only serves read-only dashboard
// queries; no long-running writes to flush.
const adminShutdownGrace = 3 * time.Second

// dispatchAdmin implements `go-guardian admin [--db <path>] [--port N] [--open]`.
// It is the production entry point: it installs SIGINT/SIGTERM handlers and
// blocks until one fires. Tests call dispatchAdminWithContext directly to
// avoid the signal path.
func dispatchAdmin(args []string, stdout, stderr io.Writer) int {
	// Translate OS signals into context cancellation so the code path is the
	// same in production and in tests.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return dispatchAdminWithContext(ctx, args, stdout, stderr, nil)
}

// dispatchAdminWithContext is the context-aware implementation that
// dispatchAdmin wraps. The test suite calls it directly with a cancellable
// context (replacing the signal path) and with an onReady callback that
// receives the bound "host:port" address once net.Listen returns. Production
// callers pass nil for onReady.
func dispatchAdminWithContext(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	onReady func(addr string),
) int {
	// NOTE: use "flags" (not "fs") for the FlagSet variable name because
	// this file imports "io/fs" and a local "fs" would shadow it, breaking
	// the fs.Sub / fs.FS references further down.
	flags := flag.NewFlagSet("admin", flag.ContinueOnError)
	flags.SetOutput(stderr)

	dbPath := flags.String("db", ".go-guardian/guardian.db", "path to the SQLite learning database")
	port := flags.Int("port", defaultAdminPort, "TCP port to bind the admin HTTP server on (localhost only)")
	openBrowser := flags.Bool("open", false, "open the admin URL in the system browser after bind succeeds")

	if err := flags.Parse(args); err != nil {
		return 2
	}
	if len(flags.Args()) != 0 {
		fmt.Fprintln(stderr, "go-guardian admin: unexpected positional arguments")
		return 2
	}
	if *port < 0 || *port > 65535 {
		fmt.Fprintf(stderr, "go-guardian admin: --port %d out of range [0, 65535]\n", *port)
		return 2
	}

	// Ensure the .go-guardian/ directory exists so db.NewStore can create
	// guardian.db on a fresh project. 0o700 matches the rest of the codebase.
	if err := os.MkdirAll(filepath.Dir(*dbPath), 0o700); err != nil {
		fmt.Fprintf(stderr, "go-guardian admin: mkdir %s: %v\n", filepath.Dir(*dbPath), err)
		return 1
	}

	store, err := db.NewStore(*dbPath)
	if err != nil {
		fmt.Fprintf(stderr, "go-guardian admin: open db %s: %v\n", *dbPath, err)
		return 1
	}
	defer store.Close()

	// Read session ID from .go-guardian/session-id (same convention main.go
	// uses for the MCP stdio server). Missing file is fine — admin.handleDashboard
	// degrades gracefully when sessionID == "".
	sessionID := readAdminSessionID(*dbPath)

	// Build the embedded SPA filesystem. If the UI assets are not present
	// (developer build with no frontend), construct the server anyway — the
	// API endpoints still work; only the HTML UI is missing.
	var staticFS fs.FS
	if sub, subErr := fs.Sub(admin.UIAssets, "ui/dist"); subErr == nil {
		staticFS = sub
	}

	srv := admin.New(store, staticFS, sessionID)

	// Bind the listener up front so port conflicts are reported BEFORE any
	// goroutine starts. This makes the error path atomic: the store is
	// already open, but no HTTP state has been mutated and no background
	// work is in flight, so deferring store.Close() is sufficient cleanup.
	addr := "127.0.0.1:" + strconv.Itoa(*port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		fmt.Fprintf(stderr, "go-guardian admin: bind %s: %v\n", addr, err)
		return 1
	}
	// listener ownership passes to http.Server.Serve; we do not Close() it
	// here. Serve returns http.ErrServerClosed after Shutdown, which closes
	// the listener as part of its internal teardown.

	boundAddr := listener.Addr().String()
	url := "http://" + boundAddr

	fmt.Fprintf(stdout, "admin UI: %s\n", url)
	fmt.Fprintln(stdout, "press Ctrl-C to stop")

	httpSrv := &http.Server{
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
	}

	serveErrCh := make(chan error, 1)
	go func() {
		serveErrCh <- httpSrv.Serve(listener)
	}()

	// Fire the test hook AFTER the listener is bound and Serve has been
	// kicked off. Production callers pass nil.
	if onReady != nil {
		onReady(boundAddr)
	}

	if *openBrowser {
		if oErr := openInBrowser(url); oErr != nil {
			fmt.Fprintf(stderr, "go-guardian admin: --open failed (continuing): %v\n", oErr)
		}
	}

	// Block until context cancellation (signal or test cancel) OR Serve
	// returns unexpectedly (listener EOF, internal error).
	select {
	case <-ctx.Done():
		// Expected termination path. Fall through to shutdown.
	case serveErr := <-serveErrCh:
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			fmt.Fprintf(stderr, "go-guardian admin: serve error: %v\n", serveErr)
			return 1
		}
		// Serve returned ErrServerClosed without a prior Shutdown call.
		// Nothing to clean up — the listener is already closed.
		return 0
	}

	// Graceful shutdown with a bounded timeout. Use a fresh context because
	// the parent ctx has already been cancelled.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), adminShutdownGrace)
	defer cancel()

	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		fmt.Fprintf(stderr, "go-guardian admin: graceful shutdown error: %v\n", err)
		// Force-close any remaining connections.
		_ = httpSrv.Close()
	}

	// Drain the serve goroutine so we do not leak it past dispatcher return.
	// Shutdown causes Serve to return http.ErrServerClosed, which we ignore.
	<-serveErrCh

	fmt.Fprintln(stdout, "admin UI: stopped")
	return 0
}

// readAdminSessionID reads the session ID from .go-guardian/session-id next
// to the DB. Missing file or read errors return "" (admin handlers degrade
// gracefully when sessionID is empty).
func readAdminSessionID(dbPath string) string {
	sidPath := filepath.Join(filepath.Dir(dbPath), "session-id")
	data, err := os.ReadFile(sidPath)
	if err != nil {
		return ""
	}
	// Trim trailing newline and whitespace without pulling strings just for this.
	end := len(data)
	for end > 0 && (data[end-1] == '\n' || data[end-1] == '\r' || data[end-1] == ' ' || data[end-1] == '\t') {
		end--
	}
	return string(data[:end])
}

// openInBrowser launches the platform browser opener for url. It is
// fire-and-forget: the command is started but not waited on. A start error
// (opener binary missing) is returned to the caller for logging; runtime
// failures of the spawned opener are invisible (which is correct — a browser
// that refuses to open should not fail `go-guardian admin`).
func openInBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", url)
	default:
		return fmt.Errorf("unsupported GOOS %q for --open", runtime.GOOS)
	}
	return cmd.Start()
}
