package admin

import (
	"io"
	"io/fs"
	"net/http"

	"github.com/kengou/go-guardian/mcp-server/db"
	"github.com/kengou/go-guardian/mcp-server/tools"
)

// Server is an embedded admin HTTP server that serves the SPA frontend
// and exposes API endpoints for the go-guardian admin dashboard.
type Server struct {
	store          *db.Store
	mux            *http.ServeMux
	sessionID      string
	prefetchStatus *tools.PrefetchStatus
}

// AdminOption configures optional Server settings.
type AdminOption func(*Server)

// WithPrefetchStatus attaches a shared prefetch status tracker.
func WithPrefetchStatus(ps *tools.PrefetchStatus) AdminOption {
	return func(s *Server) { s.prefetchStatus = ps }
}

// New creates a new admin Server. staticFS provides the embedded frontend
// assets (passed from main.go via embed.FS). API routes are registered
// under /api/v1/. An optional sessionID may be provided to enable
// session-aware dashboard data.
func New(store *db.Store, staticFS fs.FS, sessionID string, opts ...AdminOption) *Server {
	s := &Server{
		store:     store,
		mux:       http.NewServeMux(),
		sessionID: sessionID,
	}
	for _, opt := range opts {
		opt(s)
	}

	// API routes.
	s.mux.HandleFunc("GET /api/v1/activity", s.handleActivity)
	s.mux.HandleFunc("GET /api/v1/dashboard", s.handleDashboard)
	s.mux.HandleFunc("GET /api/v1/trends", s.handleTrends)
	s.mux.HandleFunc("GET /api/v1/prefetch-status", s.handlePrefetchStatus)

	// Pattern management routes.
	s.mux.HandleFunc("GET /api/v1/patterns", s.handleListPatterns)
	s.mux.HandleFunc("GET /api/v1/patterns/{id}", s.handleGetPattern)
	s.mux.HandleFunc("PUT /api/v1/patterns/{id}", s.handleUpdatePattern)
	s.mux.HandleFunc("DELETE /api/v1/patterns/{id}", s.handleDeletePattern)
	s.mux.HandleFunc("POST /api/v1/patterns/{id}/restore", s.handleRestorePattern)
	s.mux.HandleFunc("GET /api/v1/patterns/{id}/history", s.handlePatternHistory)
	s.mux.HandleFunc("GET /api/v1/suggestions", s.handleSuggestions)

	// Domain browser routes.
	s.mux.HandleFunc("GET /api/v1/session-findings", s.handleSessionFindings)
	s.mux.HandleFunc("GET /api/v1/owasp", s.handleOWASP)
	s.mux.HandleFunc("GET /api/v1/vulnerabilities", s.handleVulnerabilities)
	s.mux.HandleFunc("GET /api/v1/renovate", s.handleRenovate)

	// SPA fallback: try to serve a static file; if not found, serve index.html.
	s.mux.Handle("/", spaHandler(staticFS))

	return s
}

// ServeHTTP implements http.Handler so the Server can be used directly
// with httptest or composed into larger routers.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

// ListenAndServe starts the admin HTTP server on the given address.
func (s *Server) ListenAndServe(addr string) error {
	return http.ListenAndServe(addr, s.mux)
}

// spaHandler returns an http.Handler that tries to serve static files from
// the given filesystem. If the requested file does not exist, it falls back
// to serving index.html (standard SPA behavior for client-side routing).
func spaHandler(staticFS fs.FS) http.Handler {
	fileServer := http.FileServer(http.FS(staticFS))

	// Pre-read index.html so we can serve it directly on fallback
	// without going through http.FileServer (which may redirect).
	indexHTML, _ := fs.ReadFile(staticFS, "index.html")

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Try to open the requested path in the filesystem.
		path := r.URL.Path
		if path == "/" {
			path = "index.html"
		} else if len(path) > 0 && path[0] == '/' {
			path = path[1:]
		}

		f, err := staticFS.Open(path)
		if err != nil {
			// File not found: serve index.html directly for client-side routing.
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			if indexHTML != nil {
				w.Write(indexHTML)
			} else {
				// Fallback: try to read index.html on demand.
				if idx, openErr := staticFS.Open("index.html"); openErr == nil {
					defer idx.Close()
					io.Copy(w, idx)
				} else {
					http.NotFound(w, r)
				}
			}
			return
		}
		f.Close()

		// File exists: serve it normally via the file server.
		fileServer.ServeHTTP(w, r)
	})
}
