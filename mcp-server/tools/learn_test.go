package tools

import "testing"

// TestFileGlobFor covers the fileGlobFor helper that derives a file glob from
// a Go source basename. The helper is shared across review.go and knowledge.go;
// the rest of the original learn_from_lint pipeline has been removed in the
// MCP-tool-removal cutover, so only this test remains.
func TestFileGlobFor(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"handler.go", "*_handler.go"},
		{"user_handler.go", "*_handler.go"},
		{"foo_test.go", "*_test.go"},
		{"auth_middleware.go", "*_middleware.go"},
		{"middleware.go", "*.go"}, // stem "middleware" has no domain suffix
		{"server.go", "*_server.go"},
		{"main.go", "*.go"},
		{"", "*.go"},
		{"repo.go", "*_repo.go"},
		{"db_mock.go", "*_mock.go"},
		{"mock_db.go", "*.go"}, // stem ends in "_db", not "_mock"
	}

	for _, tc := range cases {
		t.Run(tc.base, func(t *testing.T) {
			got := fileGlobFor(tc.base)
			if got != tc.want {
				t.Errorf("fileGlobFor(%q): want %q, got %q", tc.base, tc.want, got)
			}
		})
	}
}
