// Package audit provides structured, append-only audit logging for go-guardian MCP tool calls.
//
// Each MCP tool invocation produces one JSON log line. Raw arguments are never
// logged; only a SHA-256 digest is recorded so that sensitive path patterns
// cannot be reconstructed from the log file.
//
// Log destinations:
//   - Always: ~/.go-guardian/audit.log (0600, append-only)
//   - If KUBERNETES_SERVICE_HOST is set: also os.Stdout (captured by Fluentd/Loki)
//
// Security events (HMAC violations, rate-limit spikes) go to a separate
// ~/.go-guardian/security-events.log (0600) so they can be monitored
// independently without noise from routine tool calls.
package audit

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// logDir returns the go-guardian data directory (~/.go-guardian).
func logDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("audit: user home dir: %w", err)
	}
	return filepath.Join(home, ".go-guardian"), nil
}

// openAppendOnly opens (or creates) path for append-only writes at mode 0600.
// The parent directory must already exist.
func openAppendOnly(path string) (*os.File, error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return nil, fmt.Errorf("audit: open %s: %w", path, err)
	}
	return f, nil
}

// toolEntry is the JSON shape written for every MCP tool call.
type toolEntry struct {
	TS         string  `json:"ts"`
	Tool       string  `json:"tool"`
	ArgsDigest string  `json:"args_digest"`
	DurationMS int64   `json:"duration_ms"`
	Error      *string `json:"error"`
}

// securityEntry is the JSON shape written for security events.
type securityEntry struct {
	TS    string         `json:"ts"`
	Event string         `json:"event"`
	Extra map[string]any `json:"extra,omitempty"`
}

// AuditLogger writes structured JSON audit lines to disk and, in Kubernetes,
// also to stdout. It is safe for concurrent use.
type AuditLogger struct {
	mu            sync.Mutex
	auditLog      *log.Logger
	securityLog   *log.Logger
	stdoutLog     *log.Logger // non-nil only in Kubernetes
	auditFile     *os.File
	securityFile  *os.File

	// Rate-limit event tracking for the 5-minute spike detector.
	rlMu         sync.Mutex
	rlWindowStart time.Time
	rlCount       int64
}

// New creates an AuditLogger that writes to ~/.go-guardian/audit.log and
// ~/.go-guardian/security-events.log. Both files are created with mode 0600.
// The ~/.go-guardian directory must already exist (created by db.NewStore).
func New() (*AuditLogger, error) {
	dir, err := logDir()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("audit: mkdir %s: %w", dir, err)
	}

	auditFile, err := openAppendOnly(filepath.Join(dir, "audit.log"))
	if err != nil {
		return nil, err
	}

	securityFile, err := openAppendOnly(filepath.Join(dir, "security-events.log"))
	if err != nil {
		_ = auditFile.Close()
		return nil, err
	}

	al := &AuditLogger{
		auditFile:    auditFile,
		securityFile: securityFile,
		auditLog:     log.New(auditFile, "", 0),
		securityLog:  log.New(securityFile, "", 0),
		rlWindowStart: time.Now(),
	}

	// In Kubernetes, also emit to stdout so the container log aggregator
	// (Fluentd, Loki, etc.) picks up structured JSON lines automatically.
	if os.Getenv("KUBERNETES_SERVICE_HOST") != "" {
		al.stdoutLog = log.New(os.Stdout, "", 0)
	}

	return al, nil
}

// Close flushes and closes the underlying log files. Call this on shutdown.
func (a *AuditLogger) Close() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	var errs []error
	if err := a.auditFile.Close(); err != nil {
		errs = append(errs, err)
	}
	if err := a.securityFile.Close(); err != nil {
		errs = append(errs, err)
	}
	if len(errs) > 0 {
		return fmt.Errorf("audit: close: %v", errs)
	}
	return nil
}

// DigestArgs computes the SHA-256 digest of JSON-encoded args.
// Use this in tool handlers to produce the argsDigest parameter.
//
//	digest := audit.DigestArgs(req.Params.Arguments)
//	al.LogToolCall(ctx, "learn_from_lint", digest, durationMs, err)
func DigestArgs(args any) string {
	b, err := json.Marshal(args)
	if err != nil {
		return "sha256:error"
	}
	h := sha256.Sum256(b)
	return fmt.Sprintf("sha256:%x", h)
}

// LogToolCall records one MCP tool invocation. It is safe to call from multiple
// goroutines. ctx is accepted for future tracing integration but is not used yet.
func (a *AuditLogger) LogToolCall(_ context.Context, toolName, argsDigest string, durationMS int64, callErr error) {
	entry := toolEntry{
		TS:         time.Now().UTC().Format(time.RFC3339),
		Tool:       toolName,
		ArgsDigest: argsDigest,
		DurationMS: durationMS,
	}
	if callErr != nil {
		s := callErr.Error()
		entry.Error = &s
	}

	line := marshalLine(entry)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.auditLog.Println(line)
	if a.stdoutLog != nil {
		a.stdoutLog.Println(line)
	}
}

// LogDBIntegrityViolation records an HMAC integrity failure for a database row.
// It writes to both audit.log (ERROR level) and security-events.log.
// The return value is always nil — the caller should return an empty/safe result
// to avoid surfacing tampered data.
func (a *AuditLogger) LogDBIntegrityViolation(table string, rowID int64) {
	extra := map[string]any{
		"table":  table,
		"row_id": rowID,
	}
	secEvent := securityEntry{
		TS:    time.Now().UTC().Format(time.RFC3339),
		Event: "db_integrity_violation",
		Extra: extra,
	}
	// Also emit an ERROR-tagged line to audit.log so it appears in the normal stream.
	auditEvent := map[string]any{
		"ts":     secEvent.TS,
		"level":  "ERROR",
		"event":  "db_integrity_violation",
		"table":  table,
		"row_id": rowID,
	}

	secLine := marshalLine(secEvent)
	auditLine := marshalLine(auditEvent)

	a.mu.Lock()
	defer a.mu.Unlock()
	a.auditLog.Println(auditLine)
	a.securityLog.Println(secLine)
	if a.stdoutLog != nil {
		// Emit the security event to stdout as well so Loki can alert on it.
		a.stdoutLog.Println(secLine)
	}
}

// LogRateLimitExceeded records a token-bucket rejection. If more than 10 events
// occur within a 5-minute window it also writes to security-events.log.
func (a *AuditLogger) LogRateLimitExceeded(toolName string, callerPID int) {
	ts := time.Now().UTC()

	// Bump the rate-limit counter and check the spike threshold.
	a.rlMu.Lock()
	if ts.Sub(a.rlWindowStart) > 5*time.Minute {
		// New window.
		a.rlWindowStart = ts
		atomic.StoreInt64(&a.rlCount, 0)
	}
	count := atomic.AddInt64(&a.rlCount, 1)
	a.rlMu.Unlock()

	entry := map[string]any{
		"ts":         ts.Format(time.RFC3339),
		"event":      "rate_limit_exceeded",
		"tool":       toolName,
		"caller_pid": callerPID,
	}
	line := marshalLine(entry)

	a.mu.Lock()
	a.auditLog.Println(line)
	if a.stdoutLog != nil {
		a.stdoutLog.Println(line)
	}
	a.mu.Unlock()

	// Spike threshold: > 10 events in the current 5-minute window.
	if count > 10 {
		spike := securityEntry{
			TS:    ts.Format(time.RFC3339),
			Event: "rate_limit_spike",
			Extra: map[string]any{
				"tool":         toolName,
				"count_in_5m":  count,
				"caller_pid":   callerPID,
			},
		}
		spikeLine := marshalLine(spike)
		a.mu.Lock()
		a.securityLog.Println(spikeLine)
		if a.stdoutLog != nil {
			a.stdoutLog.Println(spikeLine)
		}
		a.mu.Unlock()
	}
}

// marshalLine serialises v to a compact JSON string. On error it falls back to
// a plain-text sentinel so the log line is never silently dropped.
func marshalLine(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		return `{"error":"audit marshal failed"}`
	}
	return string(b)
}

