package main

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/kengou/go-guardian/mcp-server/db"
)

// seedAdminProject creates a .go-guardian/ directory under root, opens the
// learning store once (triggering the seed loader so admin.handleDashboard
// has real rows to return), and returns the DB path. Helper name is distinct
// from seedProject/seedInboxProject/seedRenovateProject because all four
// live in package main.
func seedAdminProject(t *testing.T, root string) string {
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
	// Insert a known lint pattern so total_patterns > 0 in the dashboard probe.
	// InsertLintPattern takes positional args: rule, fileGlob, dontCode, doCode, source.
	if err := store.InsertLintPattern(
		"TEST-ADMIN-CLI", "*.go", "x := 1", "const x = 1", "test",
	); err != nil {
		_ = store.Close()
		t.Fatalf("seed InsertLintPattern: %v", err)
	}
	_ = store.Close()
	return dbPath
}

// freeTCPPort picks an unused loopback port by binding, reading the assigned
// port, and immediately closing the listener. Used by the "on-demand start"
// subtest to pick a port --port can request. There is a narrow TOCTOU window
// between Close and the dispatcher's Listen; the window is acceptable for
// integration tests in a developer sandbox.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("freeTCPPort: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return port
}

// runAdminWithContext spawns dispatchAdminWithContext in a goroutine and
// returns a control handle. The caller waits for the onReady signal (address
// captured), performs probes, then calls cancel(). The returned WaitExit
// blocks until the dispatcher returns and yields its exit code plus stderr.
type adminRun struct {
	addrCh chan string
	exitCh chan int
	stderr *syncBuffer
	stdout *syncBuffer
	cancel context.CancelFunc
}

// syncBuffer is a byte buffer with a mutex so concurrent dispatcher writes
// and test reads don't race. bytes.Buffer is not safe for concurrent use.
type syncBuffer struct {
	mu  sync.Mutex
	buf []byte
}

func (s *syncBuffer) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.buf = append(s.buf, p...)
	return len(p), nil
}

func (s *syncBuffer) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return string(s.buf)
}

func runAdminWithContext(t *testing.T, args ...string) *adminRun {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	run := &adminRun{
		addrCh: make(chan string, 1),
		exitCh: make(chan int, 1),
		stderr: &syncBuffer{},
		stdout: &syncBuffer{},
		cancel: cancel,
	}
	onReady := func(addr string) {
		// Non-blocking send: if nobody is listening we discard — the test
		// cancelled before bind. Should not happen in practice.
		select {
		case run.addrCh <- addr:
		default:
		}
	}
	go func() {
		exit := dispatchAdminWithContext(ctx, args, run.stdout, run.stderr, onReady)
		run.exitCh <- exit
	}()
	return run
}

// waitForReady blocks until the dispatcher reports its bound address or the
// timeout elapses. On timeout the test fails with the stderr captured so far.
func (r *adminRun) waitForReady(t *testing.T, timeout time.Duration) string {
	t.Helper()
	select {
	case addr := <-r.addrCh:
		return addr
	case <-time.After(timeout):
		r.cancel()
		t.Fatalf("admin dispatcher did not report ready within %s; stderr=%s", timeout, r.stderr.String())
		return ""
	}
}

// adminExitTimeout is the maximum time the admin dispatcher has to return
// after cancellation (or after a self-terminating failure like port-bind).
// A generous 5s covers the shutdown grace period plus serve goroutine drain.
const adminExitTimeout = 5 * time.Second

// waitForExit blocks until the dispatcher returns. Must be called after
// cancel() has been invoked (or after a code path that causes the dispatcher
// to exit on its own, e.g. a port-bind failure).
func (r *adminRun) waitForExit(t *testing.T) int {
	t.Helper()
	select {
	case exit := <-r.exitCh:
		return exit
	case <-time.After(adminExitTimeout):
		t.Fatalf("admin dispatcher did not exit within %s; stderr=%s", adminExitTimeout, r.stderr.String())
		return -1
	}
}

func TestAdminSubcommands_IntegrationScenarios(t *testing.T) {
	t.Run("Scenario: No admin server runs in the default interactive session", func(t *testing.T) {
		// Source-level assertion: main.go must not contain any code that
		// starts an admin HTTP server outside dispatchAdmin. Any reference
		// in main.go to adminPort, admin.New, or "GO_GUARDIAN_ADMIN_PORT"
		// as a port source is a regression of the on-demand contract.
		//
		// This test flips GREEN when Task 2 deletes the always-on block.
		data, err := os.ReadFile("main.go")
		if err != nil {
			t.Fatalf("read main.go: %v", err)
		}
		body := string(data)
		forbidden := []string{
			"GO_GUARDIAN_ADMIN_PORT",
			"admin.New(",
			"adminSrv.ListenAndServe",
			`flag.Bool("no-admin"`,
		}
		for _, needle := range forbidden {
			if strings.Contains(body, needle) {
				t.Errorf("main.go must not reference %q after admin-cli migration; "+
					"the admin HTTP server is on-demand only (see mcp-server/admin.go)", needle)
			}
		}
	})

	t.Run("Scenario: The user starts the admin dashboard on demand", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedAdminProject(t, root)

		port := freeTCPPort(t)
		run := runAdminWithContext(t, "--db", dbPath, "--port", itoa(port))

		addr := run.waitForReady(t, 5*time.Second)
		if addr == "" {
			t.Fatal("waitForReady returned empty addr")
		}
		// Probe the dashboard endpoint — confirms the server is reachable.
		resp, err := httpGet("http://"+addr+"/api/v1/dashboard", 3*time.Second)
		if err != nil {
			run.cancel()
			t.Fatalf("dashboard probe failed: %v; stderr=%s", err, run.stderr.String())
		}
		if resp.StatusCode != http.StatusOK {
			run.cancel()
			t.Fatalf("dashboard probe status=%d, want 200", resp.StatusCode)
		}
		_ = resp.Body.Close()

		run.cancel()
		exit := run.waitForExit(t)
		if exit != 0 {
			t.Errorf("admin exit=%d after cancel, want 0; stderr=%s", exit, run.stderr.String())
		}
	})

	t.Run("Scenario: The on-demand admin dashboard displays learning data", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedAdminProject(t, root)

		port := freeTCPPort(t)
		run := runAdminWithContext(t, "--db", dbPath, "--port", itoa(port))
		addr := run.waitForReady(t, 5*time.Second)

		resp, err := httpGet("http://"+addr+"/api/v1/dashboard", 3*time.Second)
		if err != nil {
			run.cancel()
			t.Fatalf("dashboard probe failed: %v", err)
		}
		defer func() { _ = resp.Body.Close() }()

		var payload map[string]any
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			run.cancel()
			t.Fatalf("decode dashboard payload: %v", err)
		}
		// Assert the payload contains the aggregate fields served by
		// admin.handleDashboard — these cover "learned patterns",
		// "session findings", and "aggregate statistics". JSON field names
		// are snake_case per the dashboardResponse struct tags in
		// admin/handlers.go.
		for _, key := range []string{"total_patterns", "total_anti_patterns", "recent_learning_count"} {
			if _, ok := payload[key]; !ok {
				t.Errorf("dashboard payload missing field %q; got keys: %v", key, keysOf(payload))
			}
		}
		// The seeded lint pattern must be counted.
		if tp, ok := payload["total_patterns"].(float64); !ok || tp < 1 {
			t.Errorf("total_patterns=%v, want >= 1 (seeded by seedAdminProject)", payload["total_patterns"])
		}

		run.cancel()
		if exit := run.waitForExit(t); exit != 0 {
			t.Errorf("admin exit=%d after cancel, want 0", exit)
		}
	})

	t.Run("Scenario: Stopping the admin command releases the dashboard", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedAdminProject(t, root)

		port := freeTCPPort(t)
		run := runAdminWithContext(t, "--db", dbPath, "--port", itoa(port))
		addr := run.waitForReady(t, 5*time.Second)

		// Confirm reachable before stop.
		if _, err := httpGet("http://"+addr+"/api/v1/dashboard", 3*time.Second); err != nil {
			run.cancel()
			t.Fatalf("dashboard probe (pre-stop) failed: %v", err)
		}

		// Stop via context cancel (stand-in for SIGINT).
		run.cancel()
		if exit := run.waitForExit(t); exit != 0 {
			t.Errorf("admin exit=%d after cancel, want 0", exit)
		}

		// Dashboard must now be unreachable. Poll briefly (listener close is
		// synchronous inside Shutdown but the OS may take a moment).
		deadline := time.Now().Add(2 * time.Second)
		for time.Now().Before(deadline) {
			_, err := httpGet("http://"+addr+"/api/v1/dashboard", 300*time.Millisecond)
			if err != nil {
				return // Good: no lingering server.
			}
			time.Sleep(50 * time.Millisecond)
		}
		t.Errorf("dashboard still reachable at %s after cancel + shutdown; expected connection refused", addr)
	})

	t.Run("Scenario: The admin dashboard refuses to bind a port that is already in use", func(t *testing.T) {
		root := t.TempDir()
		dbPath := seedAdminProject(t, root)

		// Pre-bind a listener and KEEP IT OPEN for the duration of the test.
		blocker, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatalf("pre-bind listener: %v", err)
		}
		defer func() { _ = blocker.Close() }()
		port := blocker.Addr().(*net.TCPAddr).Port

		run := runAdminWithContext(t, "--db", dbPath, "--port", itoa(port))

		// The dispatcher must return non-zero before reporting ready.
		exit := run.waitForExit(t)
		if exit == 0 {
			t.Errorf("admin exit=0 on port conflict, want non-zero; stderr=%s", run.stderr.String())
		}
		// The error message must identify the conflict in a user-actionable way.
		stderr := run.stderr.String()
		if !strings.Contains(stderr, "port") && !strings.Contains(stderr, "address already in use") && !strings.Contains(stderr, "bind") {
			t.Errorf("port-conflict stderr does not name the conflict; got: %s", stderr)
		}
	})
}

// Small utilities to keep the test file self-contained.

// itoa formats an int without pulling in strconv at the call site.
func itoa(n int) string {
	return formatInt(n)
}

// formatInt is a minimal int→decimal formatter. Avoids pulling strconv just
// for this one call site and keeps the test helpers obvious.
func formatInt(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// httpGet wraps http.Client.Get with a short timeout. The test runs entirely
// on loopback so the timeout is just a safety net against a hung server.
func httpGet(url string, timeout time.Duration) (*http.Response, error) {
	client := &http.Client{Timeout: timeout}
	return client.Get(url)
}

// keysOf returns the keys of m in no particular order — used only for
// human-readable assertion failure messages.
func keysOf(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
