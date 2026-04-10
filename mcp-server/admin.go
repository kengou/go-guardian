package main

import (
	"context"
	"fmt"
	"io"
)

// dispatchAdmin implements `go-guardian admin [--port N] [--open]`.
// It binds the admin HTTP server in the foreground, prints the reachable URL,
// and blocks until SIGINT/SIGTERM shuts the server down gracefully.
//
// This is the production entry point. Tests call dispatchAdminWithContext
// directly to bypass the signal path.
func dispatchAdmin(args []string, stdout, stderr io.Writer) int {
	fmt.Fprintln(stderr, "go-guardian admin: not yet implemented")
	return 1
}

// dispatchAdminWithContext is the internal, context-aware implementation that
// dispatchAdmin wraps. Tests call it directly with a cancellable context in
// place of the real signal path and with an onReady callback that fires once
// the listener has bound successfully (the callback receives the bound
// "host:port" address). Production callers pass context.Background() and
// onReady == nil.
func dispatchAdminWithContext(
	ctx context.Context,
	args []string,
	stdout, stderr io.Writer,
	onReady func(addr string),
) int {
	fmt.Fprintln(stderr, "go-guardian admin: not yet implemented")
	return 1
}
