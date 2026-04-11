package tools

import "strings"

// trimSnippet truncates s to at most maxLen bytes, appending "…" if truncated.
// Package-private helper shared with review.go (and indirectly with knowledge.go
// via review.go) so that learning-style code snippets are capped to a
// predictable size before being stored.
func trimSnippet(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "…"
}

// fileGlobFor derives a file glob from a Go source basename. Domain-specific
// suffixes get a targeted glob; generic files get `*.go`. Package-private
// helper shared with review.go and knowledge.go.
func fileGlobFor(base string) string {
	if base == "" {
		return "*.go"
	}
	// Strip the .go extension for suffix inspection.
	stem := strings.TrimSuffix(base, ".go")

	// Bare-word stems that map to a domain glob even without an underscore prefix.
	// "middleware" is intentionally absent — it's too generic as a bare word.
	bareWordGlobs := map[string]string{
		"handler":    "*_handler.go",
		"handlers":   "*_handler.go",
		"server":     "*_server.go",
		"client":     "*_client.go",
		"repo":       "*_repo.go",
		"repository": "*_repository.go",
		"service":    "*_service.go",
		"model":      "*_model.go",
		"controller": "*_controller.go",
	}
	if glob, ok := bareWordGlobs[stem]; ok {
		return glob
	}

	domainSuffixes := []string{
		"_handler", "_handlers",
		"_test",
		"_server", "_client",
		"_middleware",
		"_controller",
		"_repository", "_repo",
		"_service",
		"_model",
		"_mock",
		"_gen", "_generated",
	}
	for _, suffix := range domainSuffixes {
		if strings.HasSuffix(stem, suffix) {
			return "*" + suffix + ".go"
		}
	}
	return "*.go"
}
